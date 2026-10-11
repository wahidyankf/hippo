package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

// CoordinationSnapshot observes and reconciles the shared coordination state atomically.
type CoordinationSnapshot interface {
	Snapshot(ctx context.Context, root, mode string) (coordination.ReservationTotals, error)
}

// ObservationHistory reads retained evidence without owning its use-case decisions.
type ObservationHistory interface {
	History(root string, query evidence.Query) ([]evidence.Summary, error)
}

// ObservationClock supplies observation timestamps, waits, and ticks.
type ObservationClock = ReleaseClock

// ObservationServices wires the capabilities used by status, history, watch, and monitor.
type ObservationServices struct {
	Configuration ConfigurationProvider
	Collector     policy.Collector
	Runtime       CoordinationSnapshot
	Evidence      ObservationHistory
	Clock         ObservationClock
	Environment   map[string]string
	Annotations   DisplayAnnotations
}

// StatusRequest is a validated-at-entry status observation request.
type StatusRequest struct {
	ConfigPath, DiskPath, Source string
	RequestedProfile             policy.ProfileName
	Tags                         []string
}

// PromotionStatus describes the currently effective owner limit.
type PromotionStatus struct {
	evidence.PromotionEvaluation

	BaseOwners      int `json:"baseOwners"`
	MaximumOwners   int `json:"maximumOwners"`
	EffectiveOwners int `json:"effectiveOwners"`
}

// StatusView is the portable schema-5 status snapshot rendered by driving adapters.
type StatusView struct {
	policy.Sample

	SchemaVersion int                            `json:"schemaVersion"`
	Resource      policy.Assessment              `json:"resource"`
	Profile       policy.Resolution              `json:"profile"`
	Coordination  coordination.ReservationTotals `json:"coordination"`
	Promotion     PromotionStatus                `json:"promotion"`
	ConfigHash    string                         `json:"configHash,omitempty"`
}

// EvaluatePromotion applies retained evidence and live pressure to owner limits.
func (services ObservationServices) EvaluatePromotion(
	root string,
	configuration coordination.Configuration,
	assessment policy.Assessment,
	now time.Time,
) (PromotionStatus, error) {
	status := PromotionStatus{
		BaseOwners: configuration.MaxActiveOwners, MaximumOwners: configuration.MaxActiveOwners,
		EffectiveOwners: configuration.MaxActiveOwners,
	}
	if configuration.SchemaVersion < 3 {
		status.Reason = "not-configured"

		return status, nil
	}
	status.BaseOwners = configuration.BaseActiveOwners
	status.EffectiveOwners = configuration.BaseActiveOwners
	criteria := evidence.PromotionCriteria{
		CompletedRuns:               configuration.Promotion.CompletedRuns,
		MinimumSources:              configuration.Promotion.MinimumSources,
		MinimumAvailableMemoryBytes: configuration.Promotion.MinimumAvailableMemoryBytes,
		MaximumCPUP95Percent:        configuration.Promotion.MaximumCPUP95Percent,
	}
	rows, err := services.Evidence.History(root, evidence.Query{Since: evidence.HistoryRetention, Now: now})
	if err != nil {
		return PromotionStatus{}, err
	}
	evaluation, err := evidence.EvaluatePromotion(rows, criteria)
	if err != nil {
		return PromotionStatus{}, err
	}
	status.PromotionEvaluation = evaluation
	if assessment.State == policy.StateNormal && evaluation.Eligible {
		status.EffectiveOwners = configuration.MaxActiveOwners
	} else if assessment.State != policy.StateNormal {
		status.Reason = "live-pressure-" + string(assessment.State)
	}

	return status, nil
}

// decideStatus assesses the two samples status takes against the resolution's
// own policy, as status always has, and sets the resolution's decision from the
// admission path they take. A decision that cannot be made is a fault, never a
// default.
func decideStatus(resolution policy.Resolution, first, second policy.Sample) (policy.Resolution, policy.Assessment, error) {
	samples := []policy.Sample{first, second}
	assessment := policy.ResourceAssessment(samples, resolution.Policy)
	path, decisionError := policy.DecideAdmission(policy.AdmissionInput{
		Resolution: resolution, TaskClass: policy.TaskEphemeral, Samples: samples,
		Policy: resolution.Policy, Window: policy.WindowSnapshot,
	})
	if decisionError == nil {
		resolution, decisionError = WithAssessmentDecision(resolution, path)
	}
	if decisionError != nil {
		return resolution, assessment, status.Fail(status.CodeSupervisionFailed, "decide admission: %v", decisionError)
	}

	return resolution, assessment, nil
}

// WithAssessmentDecision sets the decision status publishes for the admission
// path the host evidence took. A normal path keeps the resolution as resolved.
// A wait, and a degraded admission, which status never reaches from its two
// samples, are the documented "under any warning decision stays wait": a
// retryable deferral. A blocked disk is cleanup for storage, which a resolution
// already at cleanup already says, and a replan keeps the resolution that chose
// it. An unset path decides nothing and is refused, never defaulted.
func WithAssessmentDecision(resolution policy.Resolution, path policy.AdmissionPath) (policy.Resolution, error) {
	switch path {
	case policy.AdmissionNormal, policy.AdmissionReplan:
		return resolution, nil
	case policy.AdmissionWait, policy.AdmissionDegraded:
		resolution.Decision, resolution.Reason, resolution.Retryable = policy.DecisionWait, policy.ReasonCapacityDeferred, true

		return resolution, nil
	case policy.AdmissionCleanup:
		resolution.Decision, resolution.Reason = policy.DecisionCleanup, policy.ReasonStorageBlocked

		return resolution, nil
	case policy.AdmissionUnset:
	}

	return resolution, errors.New("admission path is unset")
}

// CoordinationFailure is what a failed read of the shared root's coordination
// state returns: a protocol mismatch stops the work for that reason, anything
// else is a failure of the read. The action says what was being done.
func CoordinationFailure(action string, err error) (int, error) {
	failure := fmt.Errorf("%s: %w", action, err)
	if errors.Is(err, coordination.ErrCoordinationProtocolMismatch) {
		return 0, policy.Stopped(policy.ReasonProtocolMismatch, failure)
	}

	return 1, failure
}

func tagsMatch(actual, expected map[string]string) bool {
	for key, value := range expected {
		if actual[key] != value {
			return false
		}
	}

	return true
}

// FilterCoordinationRows projects matching rows while preserving global resource totals.
func FilterCoordinationRows(totals coordination.ReservationTotals, source string, tags map[string]string) coordination.ReservationTotals {
	if source == "" && len(tags) == 0 {
		return totals
	}
	filter := func(entries []coordination.ReservationEntry) []coordination.ReservationEntry {
		result := make([]coordination.ReservationEntry, 0, len(entries))
		for _, entry := range entries {
			if source != "" && entry.Source != source || !tagsMatch(entry.Tags, tags) {
				continue
			}
			result = append(result, entry)
		}

		return result
	}
	totals.Owners = filter(totals.Owners)
	totals.Waiters = filter(totals.Waiters)

	return totals
}

// Snapshot observes the runtime primitive without splitting its transaction.
func (services ObservationServices) Snapshot(ctx context.Context, root, mode string) (coordination.ReservationTotals, error) {
	return services.Runtime.Snapshot(ctx, root, mode)
}

// Observe projects the shared two-sample observation into the stable status schema.
func (services ObservationServices) Observe(ctx context.Context, request StatusRequest) (StatusView, int, error) {
	prepared, err := PrepareObservation(request)
	if err != nil {
		return StatusView{}, 0, err
	}
	snapshot, code, err := services.ObserveSnapshot(ctx, prepared)
	return snapshot.Status, code, err
}

// ObserveSnapshot reloads configuration and performs the shared observation workflow.
func (services ObservationServices) ObserveSnapshot(ctx context.Context, request ObservationRequest) (ObservationSnapshot, int, error) {
	configuration, configError := services.Configuration.Load(request.configPath, services.Environment)
	if configError != nil {
		return ObservationSnapshot{}, 0, status.Fail(status.CodeConfigUnreadable, "resource configuration: %v", configError)
	}
	first, err := services.Collector.Collect(ctx, nil, request.diskPath)
	if err != nil {
		return ObservationSnapshot{}, 1, err
	}

	first.Sample = cloneObservationSample(first.Sample)
	first.CPUState = slices.Clone(first.CPUState)

	if err := services.Clock.Wait(ctx, time.Second); err != nil {
		return ObservationSnapshot{}, 1, err
	}

	second, err := services.Collector.Collect(ctx, first.CPUState, request.diskPath)
	if err != nil {
		return ObservationSnapshot{}, 1, err
	}

	second.Sample = cloneObservationSample(second.Sample)

	resolution, resolveError := configuration.Catalog.Resolve(request.requestedProfile, policy.TaskEphemeral, second.Sample)
	if resolveError != nil {
		return ObservationSnapshot{}, 0, policy.Stopped(policy.ReasonReplanRequired, resolveError)
	}

	resolution, assessment, decisionError := decideStatus(resolution, first.Sample, second.Sample)
	if decisionError != nil {
		return ObservationSnapshot{}, 0, decisionError
	}
	root := services.Configuration.StateRoot(services.Environment)
	promotion, promotionError := services.EvaluatePromotion(root, configuration.Coordination, assessment, services.Clock.Now())
	if promotionError != nil {
		return ObservationSnapshot{}, 1, fmt.Errorf("evaluate owner promotion: %w", promotionError)
	}

	totals, snapshotError := services.Snapshot(ctx, root, configuration.Coordination.Mode)
	if snapshotError != nil {
		code, failure := CoordinationFailure("read coordination status", snapshotError)
		return ObservationSnapshot{}, code, failure
	}
	view := StatusView{Sample: second.Sample, SchemaVersion: 5, Resource: assessment, Profile: resolution, Coordination: FilterCoordinationRows(totals, request.source, request.tags), Promotion: promotion, ConfigHash: configuration.Hash}
	return services.projectObservation(ctx, root, view), 0, nil
}
