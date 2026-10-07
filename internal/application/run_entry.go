package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

// RunConfiguration supplies optional identity discovery and diagnostic paths.
type RunConfiguration interface {
	ConfigurationProvider
	IdentityPresent(environment map[string]string, workingDirectory string) bool
	DisplayPath(workingDirectory, path string) string
}

// RunRequest carries parsed guarded-work arguments.
type RunRequest struct {
	Command                                                            []string
	Class                                                              policy.TaskClass
	ConfigPath, WorkingDir, DiskPath, LeaseOwner, ResourceTier, Source string
	RequestedProfile                                                   policy.ProfileName
	LeasePort, LeaseMinimum, LeaseMaximum, ReserveCPU                  int
	ReserveMemoryMiB                                                   int64
	Tags, ConcurrencyEnvironment                                       []string
	WaitForAdmission                                                   time.Duration
	ObserveChild                                                       func(int)
}

// RunEntryServices coordinates configuration, observation and guarded execution.
type RunEntryServices struct {
	Configuration  RunConfiguration
	Observation    ObservationServices
	Run            RunServices
	Collector      policy.Collector
	Environment    []string
	Now            func() time.Time
	Sleep          func(time.Duration)
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	PortLeaseRoot  string
}

func environmentMap(environment []string) map[string]string {
	result := map[string]string{}
	for _, entry := range environment {
		for i := range entry {
			if entry[i] == '=' {
				result[entry[:i]] = entry[i+1:]
				break
			}
		}
	}
	return result
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

// runIdentity resolves the labels a run carries. A run that needs no identity
// — no labels asked for, no identity file present, schema below 3 — is
// unlabeled rather than refused.
func (application RunEntryServices) runIdentity(options RunRequest, schemaVersion int) (identity.Value, error) {
	identityPresent := application.Configuration.IdentityPresent(environmentMap(application.Environment), options.WorkingDir)
	if schemaVersion < 3 && options.Source == "" && len(options.Tags) == 0 && !identityPresent {
		return identity.Value{SchemaVersion: identity.SchemaVersion, Source: "unlabeled", Tags: map[string]string{}}, nil
	}
	runIdentity, identityPath, err := application.Configuration.Identity(environmentMap(application.Environment), options.WorkingDir, options.Source, options.Tags)
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
			"run identity file %s is invalid: %v; fix or remove it", application.Configuration.DisplayPath(options.WorkingDir, identityPath), err)
	}

	return runIdentity, nil
}

// Execute prepares policy, identity, promotion and allocation before running one workload.
func (application RunEntryServices) Execute(ctx context.Context, options RunRequest) (int, error) { //nolint:cyclop,funlen,gocognit // Admission, identity, tier, and guarded-lifecycle setup must stay in one auditable pre-launch path.
	if options.WorkingDir != "" {
		absolute, err := application.Configuration.AbsolutePath(options.WorkingDir)
		if err != nil {
			return 1, err
		}

		options.WorkingDir = absolute
	}

	root := application.Configuration.StateRoot(environmentMap(application.Environment))
	if root == "" {
		return 1, errors.New("resource evidence root is unavailable")
	}

	configuration, configError := application.Configuration.Load(options.ConfigPath, environmentMap(application.Environment))
	if configError != nil {
		return 0, status.Fail(status.CodeConfigUnreadable, "resource configuration: %v", configError)
	}
	// Whether the wait can apply depends only on the schema and the flags, so
	// it is decided before identity, host evidence, or coordination state.
	if waitError := admissionWaitConflict(
		configuration.Coordination.SchemaVersion, options.ResourceTier, options.WaitForAdmission,
	); waitError != nil {
		return 0, waitError
	}
	runIdentity, identityError := application.runIdentity(options, configuration.Coordination.SchemaVersion)
	if identityError != nil {
		return 0, identityError
	}

	probeDiskPath := options.DiskPath
	if probeDiskPath == "" {
		probeDiskPath = options.WorkingDir
	}
	if probeDiskPath == "" {
		probeDiskPath = "."
	}

	probe, collectError := application.Collector.Collect(ctx, nil, probeDiskPath)
	if collectError != nil {
		return 1, collectError
	}

	taskClass := options.Class
	resolution, resolveError := configuration.Catalog.Resolve(options.RequestedProfile, taskClass, probe.Sample)
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
	promotion, promotionError := application.Observation.EvaluatePromotion(root, configuration.Coordination, liveAssessment, application.Now())
	if promotionError != nil {
		return 1, fmt.Errorf("evaluate owner promotion: %w", promotionError)
	}
	if configuration.Coordination.SchemaVersion >= 3 {
		coordination, coordinationError := application.Observation.Snapshot(ctx, root, configuration.Coordination.Mode)
		if coordinationError != nil {
			return CoordinationFailure("verify schema-3 activation", coordinationError)
		}
		if coordination.LegacyEntries > 0 {
			return 0, policy.Stopped(policy.ReasonProtocolMismatch, fmt.Errorf(
				"schema 3 activation requires legacy owners and waiters to drain (remaining=%d)",
				coordination.LegacyEntries,
			))
		}
	}

	reservationPolicy := coordination.ReservationPolicy{
		Enabled:         configuration.Coordination.Mode == "reservation",
		MaxCPU:          configuration.Coordination.MaxCPU,
		MaxMemoryBytes:  configuration.Coordination.MaxMemoryBytes,
		MaxActiveOwners: promotion.EffectiveOwners,
		OwnerShares:     configuration.Coordination.OwnerShares,
		Tiers:           configuration.Coordination.Tiers,
	}
	if reservationPolicy.Enabled && len(reservationPolicy.Tiers) == 0 {
		reservationPolicy.Tiers = coordination.DefaultResourceTiers()
	}
	if options.ReserveCPU < 0 || options.ReserveMemoryMiB < 0 {
		return 0, status.Fail(status.CodeArgsInvalid, "reservation flags must be nonnegative")
	}
	if !reservationPolicy.Enabled && (options.ReserveCPU != 0 || options.ReserveMemoryMiB != 0) {
		return 0, status.Fail(status.CodeArgsInvalid, "explicit reservations require schema 2 coordination")
	}
	reservationPlan := coordination.ReservationPlan{}
	admissionWait := resolution.Policy.LeaseWait
	if reservationPolicy.Enabled { //nolint:nestif // Tier and legacy reservation paths deliberately converge before guarded launch.
		reservationMemoryBytes, conversionError := policy.MiBToBytes(options.ReserveMemoryMiB)
		if conversionError != nil {
			return 0, status.Fail(status.CodeArgsInvalid, "%v", conversionError)
		}
		if configuration.Coordination.SchemaVersion >= 3 && options.ResourceTier == "" {
			return 0, status.Fail(status.CodeArgsInvalid, "schema 3 requires --resource-tier")
		}
		if options.ResourceTier != "" {
			reservationPlan, admissionWait, resolveError = coordination.PlanTierReservation(
				probe.Sample,
				resolution,
				reservationPolicy,
				options.ResourceTier,
				options.ReserveCPU,
				reservationMemoryBytes,
			)
		} else {
			reservationPlan, resolveError = coordination.PlanReservation(
				probe.Sample,
				resolution,
				reservationPolicy,
				options.ReserveCPU,
				reservationMemoryBytes,
			)
			if options.WaitForAdmission > 0 {
				admissionWait = options.WaitForAdmission
			}
		}
		if resolveError != nil {
			return 0, policy.Stopped(policy.ReasonReplanRequired, resolveError)
		}
	}
	resolution.Policy.LeaseWait = admissionWait

	config := RunConfig{
		Command:                options.Command[0],
		Arguments:              options.Command[1:],
		TaskClass:              taskClass,
		WorkingDirectory:       options.WorkingDir,
		Environment:            application.Environment,
		EvidenceRoot:           root,
		DiskPath:               options.DiskPath,
		LeasePort:              options.LeasePort,
		LeaseOwner:             options.LeaseOwner,
		LeaseMinimum:           options.LeaseMinimum,
		LeaseMaximum:           options.LeaseMaximum,
		PortLeaseRoot:          application.PortLeaseRoot,
		ConcurrencyEnvironment: options.ConcurrencyEnvironment,
		Collector:              application.Collector,
		Policy:                 resolution.Policy,
		Resolution:             resolution,
		ReservationPolicy:      reservationPolicy,
		ReservationPlan:        reservationPlan,
		ReservationMetadata: coordination.ReservationMetadata{
			Source: runIdentity.Source, Tags: runIdentity.Tags, Tier: options.ResourceTier,
		},
		EmergencyAvailableMemoryBytes: configuration.Coordination.EmergencyAvailableMemoryBytes,
		ConfigHash:                    configuration.Hash,
		Sleep:                         application.Sleep,
		Now:                           application.Now,
		ChildStdin:                    application.Stdin,
		ChildStdout:                   application.Stdout,
		ChildStderr:                   application.Stderr,
		Stderr:                        application.Stderr,
		ObserveChildStatus:            options.ObserveChild,
	}
	return application.Run.Run(ctx, config)
}
