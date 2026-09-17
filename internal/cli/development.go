package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	resourceconfig "github.com/wahidyankf/hippo/internal/config"
	"github.com/wahidyankf/hippo/internal/evidence"
	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/host"
	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/internal/policy"
)

const reservationCoordinationMode = "reservation"

type promotionStatus struct {
	evidence.PromotionEvaluation

	BaseOwners      int `json:"baseOwners"`
	MaximumOwners   int `json:"maximumOwners"`
	EffectiveOwners int `json:"effectiveOwners"`
}

func evaluatePromotion(
	root string,
	coordination resourceconfig.Coordination,
	assessment policy.Assessment,
	now time.Time,
) (promotionStatus, error) {
	status := promotionStatus{
		BaseOwners: coordination.MaxActiveOwners, MaximumOwners: coordination.MaxActiveOwners,
		EffectiveOwners: coordination.MaxActiveOwners,
	}
	if coordination.SchemaVersion < 3 {
		status.Reason = "not-configured"

		return status, nil
	}
	status.BaseOwners = coordination.BaseActiveOwners
	status.EffectiveOwners = coordination.BaseActiveOwners
	criteria := evidence.PromotionCriteria{
		CompletedRuns:               coordination.Promotion.CompletedRuns,
		MinimumSources:              coordination.Promotion.MinimumSources,
		MinimumAvailableMemoryBytes: coordination.Promotion.MinimumAvailableMemoryBytes,
		MaximumCPUP95Percent:        coordination.Promotion.MaximumCPUP95Percent,
	}
	evaluation, err := evidence.EvaluatePromotion(root, criteria, now)
	if err != nil {
		return promotionStatus{}, err
	}
	status.PromotionEvaluation = evaluation
	if assessment.State == policy.StateNormal && evaluation.Eligible {
		status.EffectiveOwners = coordination.MaxActiveOwners
	} else if assessment.State != policy.StateNormal {
		status.Reason = "live-pressure-" + string(assessment.State)
	}

	return status, nil
}

func (application Application) version(options versionOptions) (int, error) {
	if options.jsonOutput {
		encoded, err := json.Marshal(struct {
			SchemaVersion int    `json:"schemaVersion"`
			Version       string `json:"version"`
			Commit        string `json:"commit"`
		}{
			SchemaVersion: 1,
			Version:       application.Version,
			Commit:        application.Commit,
		})
		if err != nil {
			return 1, fmt.Errorf("encode version JSON: %w", err)
		}

		_, err = fmt.Fprintln(application.Stdout, string(encoded))

		return 0, err
	}

	_, err := fmt.Fprintf(application.Stdout, "%s (%s)\n", application.Version, application.Commit)

	return 0, err
}

func withAssessmentDecision(resolution policy.Resolution, assessment policy.Assessment) policy.Resolution {
	if resolution.ExitCode != 0 {
		return resolution
	}

	if assessment.StorageBlocked {
		resolution.Decision, resolution.ExitCode = policy.DecisionCleanup, guard.StorageBlockedExitCode
	} else if assessment.State != policy.StateNormal {
		resolution.Decision, resolution.ExitCode, resolution.Retryable = policy.DecisionWait, guard.CapacityDeferredExitCode, true
	}

	return resolution
}

func statusCoordination(ctx context.Context, root, configuredMode string) (guard.ReservationTotals, error) {
	totals := guard.ReservationTotals{SchemaVersion: 5, Mode: configuredMode}
	if root == "" {
		return totals, nil
	}
	activeMode, present, err := guard.ActiveCoordinationMode(root)
	if err != nil {
		return guard.ReservationTotals{}, err
	}
	if !present {
		return totals, nil
	}
	if activeMode != reservationCoordinationMode {
		return guard.ExclusiveStatus(ctx, root)
	}

	return guard.ReservationStatus(ctx, root)
}

func coordinationStatusExitCode(err error) int {
	if guard.IsCoordinationProtocolMismatch(err) {
		return policy.ProtocolMismatchExitCode
	}

	return 1
}

func tagsMatch(actual, expected map[string]string) bool {
	for key, value := range expected {
		if actual[key] != value {
			return false
		}
	}

	return true
}

func filterCoordinationRows(totals guard.ReservationTotals, source string, tags map[string]string) guard.ReservationTotals {
	if source == "" && len(tags) == 0 {
		return totals
	}
	filter := func(entries []guard.ReservationEntry) []guard.ReservationEntry {
		result := make([]guard.ReservationEntry, 0, len(entries))
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

func (application Application) status(ctx context.Context, options statusOptions) (int, error) {
	configuration, configError := application.loadConfig(options.configPath)
	if configError != nil {
		return policy.ReplanRequiredExitCode, fmt.Errorf("resource configuration: %w", configError)
	}
	filterTags, filterError := identity.ParseTags(options.tags)
	if filterError != nil {
		return policy.ReplanRequiredExitCode, filterError
	}
	first, err := application.Collector.Collect(ctx, nil, options.diskPath)
	if err != nil {
		return 1, err
	}

	if err := waitForContext(ctx, time.Second, application.Sleep); err != nil {
		return 1, err
	}

	second, err := application.Collector.Collect(ctx, first.CPUState, options.diskPath)
	if err != nil {
		return 1, err
	}

	resolution, resolveError := configuration.Catalog.Resolve(options.requestedProfile, policy.TaskEphemeral, second.Sample)
	if resolveError != nil {
		return policy.ReplanRequiredExitCode, resolveError
	}

	assessment := policy.ResourceAssessment([]policy.Sample{first.Sample, second.Sample}, resolution.Policy)
	resolution = withAssessmentDecision(resolution, assessment)
	root := host.DefaultEvidenceRoot(environmentMap(application.Environment))
	promotion, promotionError := evaluatePromotion(root, configuration.Coordination, assessment, application.Now())
	if promotionError != nil {
		return 1, fmt.Errorf("evaluate owner promotion: %w", promotionError)
	}

	if options.jsonOutput {
		coordination, coordinationError := statusCoordination(ctx, root, configuration.Coordination.Mode)
		if coordinationError != nil {
			return coordinationStatusExitCode(coordinationError), fmt.Errorf("read coordination status: %w", coordinationError)
		}
		payload := struct {
			policy.Sample

			SchemaVersion int                     `json:"schemaVersion"`
			Resource      policy.Assessment       `json:"resource"`
			Profile       policy.Resolution       `json:"profile"`
			Coordination  guard.ReservationTotals `json:"coordination"`
			Promotion     promotionStatus         `json:"promotion"`
			ConfigHash    string                  `json:"configHash,omitempty"`
		}{second.Sample, 5, assessment, resolution, filterCoordinationRows(coordination, options.source, filterTags), promotion, configuration.Hash}
		encoded, marshalError := json.Marshal(payload)
		if marshalError != nil {
			return 1, fmt.Errorf("encode status JSON: %w", marshalError)
		}

		_, err = fmt.Fprintln(application.Stdout, string(encoded))

		return 0, err
	}

	available, disk, cpu := unavailableValue, unavailableValue, unavailableValue
	availableBytes := second.Sample.AvailableMemoryBytes
	if availableBytes == nil {
		availableBytes = second.Sample.AvailableNonCompressedEstimateBytes
	}

	if availableBytes != nil {
		available = fmt.Sprintf("%.2f", float64(*availableBytes)/float64(policy.GiB))
	}
	if second.Sample.DiskFreeBytes != nil {
		disk = fmt.Sprintf("%.2f", float64(*second.Sample.DiskFreeBytes)/float64(policy.GiB))
	}
	if second.Sample.CPUUtilizationPercent != nil {
		cpu = fmt.Sprintf("%.1f%%", *second.Sample.CPUUtilizationPercent)
	}

	coordination, coordinationError := statusCoordination(ctx, root, configuration.Coordination.Mode)
	if coordinationError != nil {
		return coordinationStatusExitCode(coordinationError), fmt.Errorf("read coordination status: %w", coordinationError)
	}
	coordination = filterCoordinationRows(coordination, options.source, filterTags)
	_, err = fmt.Fprintf(
		application.Stdout,
		"state=%s reason=%s profile=%s concurrency=%d swap=%s availableGiB=%s diskFreeGiB=%s cpu=%s owners=%d waiters=%d ownerLimit=%d promotion=%s\n",
		assessment.State,
		assessment.Reason,
		resolution.ResolvedProfile,
		resolution.Concurrency,
		second.Sample.SwapState,
		available,
		disk,
		cpu,
		coordination.ActiveOwners,
		coordination.WaitingOwners,
		promotion.EffectiveOwners,
		promotion.Reason,
	)
	if err != nil {
		return 1, err
	}
	for _, entry := range append(coordination.Owners, coordination.Waiters...) {
		if _, err = fmt.Fprintf(
			application.Stdout,
			"%s run=%s position=%d source=%s class=%s tier=%s cpu=%d memoryMiB=%d deadline=%s\n",
			entry.State, entry.RunID, entry.Position, entry.Source, entry.Class, entry.Tier,
			max(entry.Allocated.CPU, entry.Requested.CPU),
			max(entry.Allocated.MemoryBytes, entry.Requested.MemoryBytes)/policy.MiB,
			entry.Deadline,
		); err != nil {
			return 1, err
		}
	}

	return 0, nil
}

func waitForContext(ctx context.Context, duration time.Duration, pause func(time.Duration)) error {
	if pause != nil {
		pause(duration)

		return ctx.Err()
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func writeMonitorTransition(
	destination io.Writer,
	jsonOutput bool,
	sample policy.Sample,
	assessment policy.Assessment,
	resolution policy.Resolution,
) error {
	if !jsonOutput {
		_, err := fmt.Fprintf(
			destination,
			"%s state=%s reason=%s profile=%s swap=%s\n",
			sample.MeasuredAt,
			assessment.State,
			assessment.Reason,
			resolution.ResolvedProfile,
			sample.SwapState,
		)

		return err
	}

	encoded, err := json.Marshal(struct {
		SchemaVersion int          `json:"schemaVersion"`
		MeasuredAt    string       `json:"measuredAt"`
		State         policy.State `json:"state"`
		Reason        string       `json:"reason"`
		Profile       string       `json:"profile"`
		SwapState     string       `json:"swapState"`
	}{1, sample.MeasuredAt, assessment.State, assessment.Reason, resolution.ResolvedProfile, sample.SwapState})
	if err != nil {
		return fmt.Errorf("encode monitor transition JSON: %w", err)
	}

	_, err = fmt.Fprintln(destination, string(encoded))

	return err
}

func (application Application) monitor(ctx context.Context, options monitorOptions) (int, error) {
	if options.interval <= 0 {
		return 1, errors.New("interval must be positive")
	}

	configuration, configError := application.loadConfig(options.configPath)
	if configError != nil {
		return policy.ReplanRequiredExitCode, fmt.Errorf("resource configuration: %w", configError)
	}

	var previous policy.CPUState

	samples := []policy.Sample{}
	prior := ""

	observe := func() error {
		reading, err := application.Collector.Collect(ctx, previous, options.diskPath)
		if err != nil {
			return err
		}

		previous = reading.CPUState
		samples = append(samples, reading.Sample)
		if len(samples) > 17 {
			samples = samples[len(samples)-17:]
		}

		resolution, resolveError := configuration.Catalog.Resolve(options.requestedProfile, policy.TaskEphemeral, reading.Sample)
		if resolveError != nil {
			return resolveError
		}

		assessment := policy.ResourceAssessment(samples, resolution.Policy)
		state := string(assessment.State) + ":" + assessment.Reason + ":" + resolution.ResolvedProfile

		if state != prior {
			if err := writeMonitorTransition(application.Stdout, options.jsonOutput, reading.Sample, assessment, resolution); err != nil {
				return err
			}

			prior = state
		}

		return nil
	}

	if err := observe(); err != nil {
		return 1, err
	}

	ticker := time.NewTicker(options.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return 0, nil
		case <-ticker.C:
			// Cancellation and a tick routinely become ready together, because the
			// interval is short and a loaded host leaves this loop unscheduled across
			// both. Go then selects uniformly, and servicing the tick collects through
			// an already-cancelled context, which turns a deliberate stop into a
			// monitoring failure. Stopping is what the caller asked for, so it wins.
			select {
			case <-ctx.Done():
				return 0, nil
			default:
			}

			if err := observe(); err != nil {
				return 1, err
			}
		}
	}
}

func (application Application) run(ctx context.Context, options runOptions) (int, error) { //nolint:cyclop,funlen,gocognit,gocyclo // Admission, identity, tier, and guarded-lifecycle setup must stay in one auditable pre-launch path.
	if options.workingDir != "" {
		absolute, err := filepath.Abs(options.workingDir)
		if err != nil {
			return 1, err
		}

		options.workingDir = absolute
	}

	root := host.DefaultEvidenceRoot(environmentMap(application.Environment))
	if root == "" {
		return 1, errors.New("resource evidence root is unavailable")
	}

	configuration, configError := application.loadConfig(options.configPath)
	if configError != nil {
		return policy.ReplanRequiredExitCode, fmt.Errorf("resource configuration: %w", configError)
	}
	environment := environmentMap(application.Environment)
	identityPath := identity.Path(environment, options.workingDir)
	runIdentity := identity.Value{
		SchemaVersion: identity.SchemaVersion, Source: "unlabeled", Tags: map[string]string{},
	}
	_, identityStatError := os.Stat(identityPath)
	identityRequired := configuration.Coordination.SchemaVersion >= 3 || options.source != "" || len(options.tags) > 0 ||
		identityStatError == nil
	if identityRequired {
		runIdentity, configError = identity.Load(identityPath, options.source, options.tags)
		if configError != nil {
			return policy.ReplanRequiredExitCode, fmt.Errorf("run identity: %w", configError)
		}
	}

	probeDiskPath := options.diskPath
	if probeDiskPath == "" {
		probeDiskPath = options.workingDir
	}
	if probeDiskPath == "" {
		probeDiskPath = "."
	}

	probe, collectError := application.Collector.Collect(ctx, nil, probeDiskPath)
	if collectError != nil {
		return 1, collectError
	}

	taskClass := policy.TaskClass(options.class)
	resolution, resolveError := configuration.Catalog.Resolve(options.requestedProfile, taskClass, probe.Sample)
	if resolveError != nil {
		return policy.ReplanRequiredExitCode, resolveError
	}
	if resolution.ExitCode != 0 {
		_, _ = fmt.Fprintf(
			application.Stderr,
			"HIPPO decision=%s requested=%s resolved=%s.\n",
			resolution.Decision,
			resolution.RequestedProfile,
			resolution.ResolvedProfile,
		)

		return resolution.ExitCode, nil
	}
	liveAssessment := policy.ResourceAssessment([]policy.Sample{probe.Sample}, resolution.Policy)
	promotion, promotionError := evaluatePromotion(root, configuration.Coordination, liveAssessment, application.Now())
	if promotionError != nil {
		return 1, fmt.Errorf("evaluate owner promotion: %w", promotionError)
	}
	if configuration.Coordination.SchemaVersion >= 3 {
		coordination, coordinationError := statusCoordination(ctx, root, configuration.Coordination.Mode)
		if coordinationError != nil {
			if guard.IsCoordinationProtocolMismatch(coordinationError) {
				return policy.ProtocolMismatchExitCode, fmt.Errorf("verify schema-3 activation: %w", coordinationError)
			}

			return 1, fmt.Errorf("verify schema-3 activation: %w", coordinationError)
		}
		if coordination.LegacyEntries > 0 {
			return policy.ProtocolMismatchExitCode, fmt.Errorf(
				"schema 3 activation requires legacy owners and waiters to drain (remaining=%d)",
				coordination.LegacyEntries,
			)
		}
	}

	reservationPolicy := guard.ReservationPolicy{
		Enabled:         configuration.Coordination.Mode == reservationCoordinationMode,
		MaxCPU:          configuration.Coordination.MaxCPU,
		MaxMemoryBytes:  configuration.Coordination.MaxMemoryBytes,
		MaxActiveOwners: promotion.EffectiveOwners,
		OwnerShares:     configuration.Coordination.OwnerShares,
		Tiers:           configuration.Coordination.Tiers,
	}
	if reservationPolicy.Enabled && len(reservationPolicy.Tiers) == 0 {
		reservationPolicy.Tiers = guard.DefaultResourceTiers()
	}
	if options.reserveCPU < 0 || options.reserveMemoryMiB < 0 {
		return policy.ReplanRequiredExitCode, errors.New("reservation flags must be nonnegative")
	}
	if !reservationPolicy.Enabled && (options.reserveCPU != 0 || options.reserveMemoryMiB != 0) {
		return policy.ReplanRequiredExitCode, errors.New("explicit reservations require schema 2 coordination")
	}
	reservationPlan := guard.ReservationPlan{}
	admissionWait := resolution.Policy.LeaseWait
	if reservationPolicy.Enabled { //nolint:nestif // Tier and legacy reservation paths deliberately converge before guarded launch.
		reservationMemoryBytes, conversionError := policy.MiBToBytes(options.reserveMemoryMiB)
		if conversionError != nil {
			return policy.ReplanRequiredExitCode, conversionError
		}
		if configuration.Coordination.SchemaVersion >= 3 && options.resourceTier == "" {
			return policy.ReplanRequiredExitCode, errors.New("schema 3 requires --resource-tier")
		}
		if configuration.Coordination.SchemaVersion >= 3 && options.waitForAdmission != 0 {
			return policy.ReplanRequiredExitCode, errors.New("schema 3 queue deadlines come from --resource-tier")
		}
		if options.resourceTier != "" {
			reservationPlan, admissionWait, resolveError = guard.PlanTierReservation(
				probe.Sample,
				resolution,
				reservationPolicy,
				options.resourceTier,
				options.reserveCPU,
				reservationMemoryBytes,
			)
		} else {
			reservationPlan, resolveError = guard.PlanReservation(
				probe.Sample,
				resolution,
				reservationPolicy,
				options.reserveCPU,
				reservationMemoryBytes,
			)
			if options.waitForAdmission > 0 {
				admissionWait = options.waitForAdmission
			}
		}
		if resolveError != nil {
			return policy.ReplanRequiredExitCode, resolveError
		}
	}
	resolution.Policy.LeaseWait = admissionWait

	config := guard.RunConfig{
		Command:                options.command[0],
		Arguments:              options.command[1:],
		TaskClass:              taskClass,
		WorkingDirectory:       options.workingDir,
		Environment:            application.Environment,
		EvidenceRoot:           root,
		DiskPath:               options.diskPath,
		LeasePort:              options.leasePort,
		LeaseOwner:             options.leaseOwner,
		LeaseMinimum:           options.leaseMinimum,
		LeaseMaximum:           options.leaseMaximum,
		ConcurrencyEnvironment: options.concurrencyEnvironment,
		Collector:              application.Collector,
		Policy:                 resolution.Policy,
		Resolution:             resolution,
		ReservationPolicy:      reservationPolicy,
		ReservationPlan:        reservationPlan,
		ReservationMetadata: guard.ReservationMetadata{
			Source: runIdentity.Source, Tags: runIdentity.Tags, Tier: options.resourceTier,
		},
		EmergencyAvailableMemoryBytes: configuration.Coordination.EmergencyAvailableMemoryBytes,
		ConfigHash:                    configuration.Hash,
		Sleep:                         application.Sleep,
		Now:                           application.Now,
		ChildStdin:                    application.Stdin,
		ChildStdout:                   application.Stdout,
		ChildStderr:                   application.Stderr,
		Stderr:                        application.Stderr,
	}
	return guard.Run(ctx, config)
}
