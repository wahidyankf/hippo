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
	"github.com/wahidyankf/hippo/internal/status"
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
		resolution, decisionError = withAssessmentDecision(resolution, path)
	}
	if decisionError != nil {
		return resolution, assessment, status.Fail(status.CodeSupervisionFailed, "decide admission: %v", decisionError)
	}

	return resolution, assessment, nil
}

// withAssessmentDecision sets the decision status publishes for the admission
// path the host evidence took. A normal path keeps the resolution as resolved.
// A wait, and a degraded admission, which status never reaches from its two
// samples, are the documented "under any warning decision stays wait": a
// retryable deferral. A blocked disk is cleanup for storage, which a resolution
// already at cleanup already says, and a replan keeps the resolution that chose
// it. An unset path decides nothing and is refused, never defaulted.
func withAssessmentDecision(resolution policy.Resolution, path policy.AdmissionPath) (policy.Resolution, error) {
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

// statusCoordination reads the shared root's coordination summary. Taking the
// shared lock, and reconciling what it finds, are writes into the state root;
// one the root refuses names the reason the state root's other refused writes
// name, since the remedy is the same and nothing was started.
func statusCoordination(ctx context.Context, root, configuredMode string) (guard.ReservationTotals, error) {
	totals, err := readStatusCoordination(ctx, root, configuredMode)
	if err != nil && evidence.WriteRefused(err) {
		return guard.ReservationTotals{}, status.Fail(status.CodeEvidenceUnwritable, "read coordination status: %v", err)
	}

	return totals, err
}

func readStatusCoordination(ctx context.Context, root, configuredMode string) (guard.ReservationTotals, error) {
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

// coordinationFailure is what a failed read of the shared root's coordination
// state returns: a protocol mismatch stops the work for that reason, anything
// else is a failure of the read. The action says what was being done.
func coordinationFailure(action string, err error) (int, error) {
	failure := fmt.Errorf("%s: %w", action, err)
	if guard.IsCoordinationProtocolMismatch(err) {
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
	// A malformed filter is the caller's mistake, found before anything is
	// read, exactly as run and history report their own malformed filters.
	if err := identity.ValidateOverrides(options.source, nil); err != nil {
		return 0, status.Fail(status.CodeArgsInvalid, "--source: %v", err)
	}
	filterTags, filterError := identity.ParseTags(options.tags)
	if filterError != nil {
		return 0, status.Fail(status.CodeArgsInvalid, "--tag: %v", filterError)
	}
	configuration, configError := application.loadConfig(options.configPath)
	if configError != nil {
		return 0, status.Fail(status.CodeConfigUnreadable, "resource configuration: %v", configError)
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

	resolution, resolveError := configuration.Catalog.Resolve(options.requestedProfile(), policy.TaskEphemeral, second.Sample)
	if resolveError != nil {
		return 0, policy.Stopped(policy.ReasonReplanRequired, resolveError)
	}

	resolution, assessment, decisionError := decideStatus(resolution, first.Sample, second.Sample)
	if decisionError != nil {
		return 0, decisionError
	}
	root := host.DefaultEvidenceRoot(environmentMap(application.Environment))
	promotion, promotionError := evaluatePromotion(root, configuration.Coordination, assessment, application.Now())
	if promotionError != nil {
		return 1, fmt.Errorf("evaluate owner promotion: %w", promotionError)
	}

	if options.jsonOutput {
		coordination, coordinationError := statusCoordination(ctx, root, configuration.Coordination.Mode)
		if coordinationError != nil {
			return coordinationFailure("read coordination status", coordinationError)
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
		return coordinationFailure("read coordination status", coordinationError)
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
		SchemaVersion int                `json:"schemaVersion"`
		MeasuredAt    string             `json:"measuredAt"`
		State         policy.State       `json:"state"`
		Reason        string             `json:"reason"`
		Profile       policy.ProfileName `json:"profile"`
		SwapState     string             `json:"swapState"`
	}{1, sample.MeasuredAt, assessment.State, assessment.Reason, resolution.ResolvedProfile, sample.SwapState})
	if err != nil {
		return fmt.Errorf("encode monitor transition JSON: %w", err)
	}

	_, err = fmt.Fprintln(destination, string(encoded))

	return err
}

func (application Application) monitor(ctx context.Context, options monitorOptions) (int, error) {
	if options.interval <= 0 {
		return 0, status.Fail(status.CodeArgsInvalid, "interval must be positive")
	}

	configuration, configError := application.loadConfig(options.configPath)
	if configError != nil {
		return 0, status.Fail(status.CodeConfigUnreadable, "resource configuration: %v", configError)
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

		resolution, resolveError := configuration.Catalog.Resolve(options.requestedProfile(), policy.TaskEphemeral, reading.Sample)
		if resolveError != nil {
			return resolveError
		}

		assessment := policy.ResourceAssessment(samples, resolution.Policy)
		state := string(assessment.State) + ":" + assessment.Reason + ":" + string(resolution.ResolvedProfile)

		if state != prior {
			if err := writeMonitorTransition(application.Stdout, options.jsonOutput, reading.Sample, assessment, resolution); err != nil {
				return err
			}

			prior = state
		}

		return nil
	}

	if err := observe(); err != nil {
		if stoppedByCaller(ctx, err) {
			return 0, nil
		}

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
				// A stop can also land while a probe is running, and a probe
				// its cancelled context killed reports its own error.
				if stoppedByCaller(ctx, err) {
					return 0, nil
				}

				return 1, err
			}
		}
	}
}

// admissionWaitConflict refuses a wait that could only be ignored. The flag
// bounds the schema-2 FIFO queue. Schema 1 has no such queue — its exclusive
// lease waits as long as the resolved profile says — a tier carries its own
// queue deadline, and schema 3 always uses one, so a --wait-for-admission
// under any of them would silently wait for a deadline the caller did not ask
// for.
func admissionWaitConflict(schemaVersion int, resourceTier string, wait time.Duration) error {
	switch {
	case wait == 0:
		return nil
	case schemaVersion < 2:
		return status.Fail(
			status.CodeArgsInvalid,
			"--wait-for-admission bounds the schema 2 reservation queue; schema 1 exclusive coordination has none",
		)
	case schemaVersion >= 3:
		return status.Fail(status.CodeArgsInvalid, "schema 3 queue deadlines come from --resource-tier")
	case resourceTier != "":
		return status.Fail(
			status.CodeArgsInvalid,
			"--wait-for-admission cannot be combined with --resource-tier; the tier sets the queue deadline",
		)
	default:
		return nil
	}
}

// runClass reads the --class flag of run: the class it names, or ephemeral for
// an empty one, which is what an unset class has always meant. A class run can
// never accept is the caller's mistake, found before anything else runs. Release
// work is guarded by the release commands, never by run.
func runClass(flag string) (policy.TaskClass, error) {
	if flag == "" {
		return policy.TaskEphemeral, nil
	}
	class, err := policy.ParseTaskClass(flag)
	if err != nil || class == policy.TaskRelease {
		return "", status.Fail(status.CodeArgsInvalid, "class must be ephemeral, service, or transactional")
	}

	return class, nil
}

// runArgumentMistake refuses flag values run can never accept. Each is the
// caller's mistake, so it is found here, before any configuration, host
// evidence, or coordination state is read, and reported as a usage mistake
// rather than as HIPPO failing.
func runArgumentMistake(options runOptions) error {
	if _, known := guard.DefaultResourceTiers()[options.resourceTier]; options.resourceTier != "" && !known {
		return status.Fail(status.CodeArgsInvalid, "resource tier must be light, standard, or heavy")
	}
	if options.waitForAdmission < 0 {
		return status.Fail(status.CodeArgsInvalid, "--wait-for-admission must not be negative")
	}
	if options.leasePort != 0 {
		if err := guard.ValidatePortLeaseRequest(
			options.leasePort, options.leaseOwner, options.leaseMinimum, options.leaseMaximum,
		); err != nil {
			return status.Fail(status.CodeArgsInvalid, "--lease-port: %v", err)
		}
	} else if options.leaseOwner != "" || options.leaseMinimum != 0 || options.leaseMaximum != 0 {
		// Without a port there is no lease for these to shape, and ignoring
		// them would let a caller believe a range or owner was enforced.
		return status.Fail(status.CodeArgsInvalid, "--lease-owner, --lease-min, and --lease-max require --lease-port")
	}
	if err := identity.ValidateOverrides(options.source, options.tags); err != nil {
		return status.Fail(status.CodeArgsInvalid, "%v", err)
	}

	return nil
}

// runIdentity resolves the labels a run carries. A run that needs no identity
// — no labels asked for, no identity file present, schema below 3 — is
// unlabeled rather than refused.
func (application Application) runIdentity(options runOptions, schemaVersion int) (identity.Value, error) {
	identityPath := identity.Path(environmentMap(application.Environment), options.workingDir)
	_, identityStatError := os.Stat(identityPath)
	identityPresent := identityStatError == nil
	if schemaVersion < 3 && options.source == "" && len(options.tags) == 0 && !identityPresent {
		return identity.Value{SchemaVersion: identity.SchemaVersion, Source: "unlabeled", Tags: map[string]string{}}, nil
	}
	runIdentity, err := identity.Load(identityPath, options.source, options.tags)
	if errors.Is(err, identity.ErrSourceRequired) {
		// Nothing names whose run this is. That is fixed by the invocation,
		// not by HIPPO's state, so it is a usage mistake.
		return identity.Value{}, status.Failure{
			Code: status.CodeArgsInvalid, Field: "--source",
			Message: "run identity has no source: pass --source or provide a hippo.identity.json identity file",
		}
	}
	if err != nil {
		// The overrides were validated as usage, so what remains is the
		// identity file itself: present, but unreadable or invalid.
		return identity.Value{}, status.Fail(status.CodeIdentityInvalid,
			"run identity file %s is invalid: %v; fix or remove it", displayedPath(options.workingDir, identityPath), err)
	}

	return runIdentity, nil
}

func (application Application) run(ctx context.Context, options runOptions) (int, error) { //nolint:cyclop,funlen,gocognit // Admission, identity, tier, and guarded-lifecycle setup must stay in one auditable pre-launch path.
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
		return 0, status.Fail(status.CodeConfigUnreadable, "resource configuration: %v", configError)
	}
	// Whether the wait can apply depends only on the schema and the flags, so
	// it is decided before identity, host evidence, or coordination state.
	if waitError := admissionWaitConflict(
		configuration.Coordination.SchemaVersion, options.resourceTier, options.waitForAdmission,
	); waitError != nil {
		return 0, waitError
	}
	runIdentity, identityError := application.runIdentity(options, configuration.Coordination.SchemaVersion)
	if identityError != nil {
		return 0, identityError
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

	taskClass := options.class
	resolution, resolveError := configuration.Catalog.Resolve(options.requestedProfile(), taskClass, probe.Sample)
	if resolveError != nil {
		return 0, policy.Stopped(policy.ReasonReplanRequired, resolveError)
	}
	if resolution.Reason != policy.ReasonNone {
		_, _ = fmt.Fprintf(
			application.Stderr,
			"HIPPO decision=%s requested=%s resolved=%s.\n",
			resolution.Decision,
			resolution.RequestedProfile,
			resolution.ResolvedProfile,
		)

		return 0, policy.Stopped(resolution.Reason, nil)
	}
	liveAssessment := policy.ResourceAssessment([]policy.Sample{probe.Sample}, resolution.Policy)
	promotion, promotionError := evaluatePromotion(root, configuration.Coordination, liveAssessment, application.Now())
	if promotionError != nil {
		return 1, fmt.Errorf("evaluate owner promotion: %w", promotionError)
	}
	if configuration.Coordination.SchemaVersion >= 3 {
		coordination, coordinationError := statusCoordination(ctx, root, configuration.Coordination.Mode)
		if coordinationError != nil {
			return coordinationFailure("verify schema-3 activation", coordinationError)
		}
		if coordination.LegacyEntries > 0 {
			return 0, policy.Stopped(policy.ReasonProtocolMismatch, fmt.Errorf(
				"schema 3 activation requires legacy owners and waiters to drain (remaining=%d)",
				coordination.LegacyEntries,
			))
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
		return 0, status.Fail(status.CodeArgsInvalid, "reservation flags must be nonnegative")
	}
	if !reservationPolicy.Enabled && (options.reserveCPU != 0 || options.reserveMemoryMiB != 0) {
		return 0, status.Fail(status.CodeArgsInvalid, "explicit reservations require schema 2 coordination")
	}
	reservationPlan := guard.ReservationPlan{}
	admissionWait := resolution.Policy.LeaseWait
	if reservationPolicy.Enabled { //nolint:nestif // Tier and legacy reservation paths deliberately converge before guarded launch.
		reservationMemoryBytes, conversionError := policy.MiBToBytes(options.reserveMemoryMiB)
		if conversionError != nil {
			return 0, status.Fail(status.CodeArgsInvalid, "%v", conversionError)
		}
		if configuration.Coordination.SchemaVersion >= 3 && options.resourceTier == "" {
			return 0, status.Fail(status.CodeArgsInvalid, "schema 3 requires --resource-tier")
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
			return 0, policy.Stopped(policy.ReasonReplanRequired, resolveError)
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
		PortLeaseRoot:          portLeaseRoot(application.Environment),
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
		ObserveChildStatus:            options.observeChild,
	}
	return guard.Run(ctx, config)
}

// displayedPath names a file relative to the working directory when it lies
// beneath or beside it, so a diagnostic points at the file the caller sees.
func displayedPath(workingDirectory, path string) string {
	base, err := filepath.Abs(workingDirectory)
	if err != nil {
		return path
	}
	relative, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}

	return relative
}

// portLeaseRoot is where port leases live: under the temporary directory, as
// the guard would find it, but read from the environment the command was
// given, so an injected environment chooses it the way a process's own does.
func portLeaseRoot(environment []string) string {
	temporary := environmentMap(environment)["TMPDIR"]
	if temporary == "" {
		temporary = os.TempDir()
	}

	return filepath.Join(temporary, "hippo-port-leases")
}
