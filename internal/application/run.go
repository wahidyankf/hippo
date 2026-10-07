package application

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/domain/coordination"
	evidencedomain "github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

// EnvironmentValue resolves name against the duplicate-key semantics the child
// actually observes: os/exec dedupes Cmd.Env keeping the last occurrence, and
// WithEnvironment produces that same layout by stripping prior matches before
// appending. Reading the first match instead would let an ambient value shadow a
// caller's explicit override — append(os.Environ(), "K=v") is the ordinary way to
// set one variable — and the guard would then admit against a value the child
// never sees.
func EnvironmentValue(environment []string, name string) string {
	prefix := name + "="
	value := ""
	for _, entry := range environment {
		if len(entry) >= len(prefix) && entry[:len(prefix)] == prefix {
			value = entry[len(prefix):]
		}
	}

	return value
}

// WithEnvironment replaces all values of a named environment entry.
func WithEnvironment(environment []string, name, value string) []string {
	result := make([]string, 0, len(environment)+1)
	prefix := name + "="
	for _, entry := range environment {
		if len(entry) < len(prefix) || entry[:len(prefix)] != prefix {
			result = append(result, entry)
		}
	}

	return append(result, prefix+value)
}

// WithEnvironmentIfMissing preserves an existing entry or sets its default.
func WithEnvironmentIfMissing(environment []string, name, value string) []string {
	if EnvironmentValue(environment, name) != "" {
		return environment
	}

	return WithEnvironment(environment, name, value)
}

func isASCIILetter(character byte) bool {
	return character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z'
}

func validEnvironmentName(name string) bool {
	if name == "" || name[0] != '_' && !isASCIILetter(name[0]) {
		return false
	}

	for index := 1; index < len(name); index++ {
		character := name[index]
		if character != '_' && !isASCIILetter(character) && (character < '0' || character > '9') {
			return false
		}
	}

	return true
}

func reservedConcurrencyEnvironment(name string) bool {
	return strings.HasPrefix(name, "HIPPO_")
}

// NormalizeConcurrencyEnvironment validates and deduplicates caller-selected concurrency names.
func NormalizeConcurrencyEnvironment(names []string) ([]string, error) {
	seen := map[string]bool{}
	result := make([]string, 0, len(names))

	for _, name := range names {
		// Both are the caller naming something hippo cannot accept, which is a
		// usage mistake and not a request that a different host could admit.
		if !validEnvironmentName(name) {
			return nil, status.Fail(status.CodeArgsInvalid,
				"concurrency environment name %q is not a POSIX identifier", name)
		}
		if reservedConcurrencyEnvironment(name) {
			return nil, status.Fail(status.CodeArgsInvalid, "concurrency environment name %q is reserved", name)
		}
		if !seen[name] {
			result = append(result, name)
			seen[name] = true
		}
	}

	return result, nil
}

// ResolvedEnvironment applies fixed profile concurrency to the child environment.
func ResolvedEnvironment(environment []string, resolution policy.Resolution, forceConcurrency bool, names []string) []string {
	if resolution.ResolvedProfile == "" || resolution.Concurrency <= 0 {
		return environment
	}

	concurrency := strconv.Itoa(resolution.Concurrency)
	environment = WithEnvironment(environment, "HIPPO_PROFILE", string(resolution.ResolvedProfile))
	environment = WithEnvironment(environment, "HIPPO_CONCURRENCY", concurrency)
	for _, name := range names {
		if forceConcurrency {
			environment = WithEnvironment(environment, name, concurrency)
		} else {
			environment = WithEnvironmentIfMissing(environment, name, concurrency)
		}
	}

	return environment
}

// ReservationEnvironment exports a fixed allocation and safely clamps mapped worker variables.
func ReservationEnvironment(
	environment []string,
	resolution policy.Resolution,
	allocation coordination.ReservationVector,
	names []string,
) ([]string, error) {
	if allocation.CPU < coordination.MinimumReservationCPU || allocation.MemoryBytes < coordination.MinimumReservationMemoryBytes {
		return nil, errors.New("reservation allocation is below the immutable floor")
	}

	concurrency := strconv.Itoa(allocation.CPU)
	environment = WithEnvironment(environment, "HIPPO_PROFILE", string(resolution.ResolvedProfile))
	environment = WithEnvironment(environment, "HIPPO_CONCURRENCY", concurrency)
	environment = WithEnvironment(environment, "HIPPO_RESERVED_MEMORY_BYTES", strconv.FormatInt(allocation.MemoryBytes, 10))
	for _, name := range names {
		value := EnvironmentValue(environment, name)
		if value == "" {
			environment = WithEnvironment(environment, name, concurrency)

			continue
		}
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("concurrency environment %q must be a positive integer", name)
		}
		if parsed > allocation.CPU {
			environment = WithEnvironment(environment, name, concurrency)
		}
	}

	return environment, nil
}

// lostAfterLaunch keeps a failure after the child started the supervision
// failure it is. The reasons for an unreadable host or a refused evidence
// write say that nothing was started, and here something was.
func lostAfterLaunch(err error) error {
	if failure, classified := errors.AsType[status.Failure](err); classified {
		return errors.New(failure.Message)
	}

	return err
}

// FinalOutcome is the outcome a run's lifetime summary records. A run whose
// lifetime ended with no outcome decided is a supervision failure, never a
// deferral: the deferral was once the default every unlabelled path fell into,
// and a summary that said capacity-deferred about a run that was not deferred is
// what 41bda15 and 529b506 had to repair.
func FinalOutcome(outcome evidencedomain.Outcome) (evidencedomain.Outcome, error) {
	if outcome != evidencedomain.OutcomeUnset {
		return outcome, nil
	}

	return evidencedomain.OutcomeSupervisionFailed, status.Fail(
		status.CodeSupervisionFailed, "the run ended without deciding its outcome; its summary records %s",
		evidencedomain.OutcomeSupervisionFailed,
	)
}

// PromoteRelease decides what the caller sees once the ownership release has
// failed. A run that already failed keeps its own failure, and so does a stop
// that carries an error, as the (74, stopError) it replaces did. A run that said
// nothing beyond its reason reports the failure with exit status 1, except that a
// storage or capacity deferral keeps its reason beside the failure, as it kept
// its exit status: nothing started, so retrying after the cleanup is still right.
// A pressure shed, a replan, and a protocol mismatch do not; the failure is the
// news.
func PromoteRelease(exitCode int, returnError, releaseError error) (int, error) {
	if !policy.CarriesNoError(returnError) || releaseError == nil {
		return exitCode, returnError
	}
	if stop, bare := policy.BareStop(returnError); bare &&
		(stop.Reason == policy.ReasonStorageBlocked || stop.Reason == policy.ReasonCapacityDeferred) {
		return exitCode, policy.Stopped(stop.Reason, releaseError)
	}

	return 1, releaseError
}

// PromoteFinalize decides what the caller sees once the lifetime summary has
// been finalized. A run that already failed keeps its own failure, and so does a
// run whose summary was written. A stop with no error of its own counts as no
// failure of the run's own. Otherwise the summary's failure becomes the run's,
// with exit status 1; before anything launched, a write the evidence root
// refused is named as one.
//
// This is how the supervision failure finalOutcome returns for a run that ended
// with its outcome unset reaches the caller: it is the finalize error here, so
// the command-line boundary reports hippo.supervision.failed and exit 125.
func (services RunServices) PromoteFinalize(exitCode int, returnError, finalizeError error, launched bool) (int, error) {
	if !policy.CarriesNoError(returnError) || finalizeError == nil {
		return exitCode, returnError
	}
	if !launched {
		return 1, services.refusedEvidenceWrite("recording the lifetime summary", finalizeError)
	}

	return 1, finalizeError
}

// noteDeferralf reports why admission was deferred. The run returns after this
// notice; callers use the recorded receipt to decide whether a later retry is
// safe instead of blindly replaying a 124 naming hippo.limit.capacity-deferred.
func (config RunConfig) noteDeferralf(format string, arguments ...any) {
	_, _ = fmt.Fprintf(config.Stderr, format, arguments...)
}

func childStatus(config RunConfig, completion ChildCompletion) int {
	if config.ObserveChildStatus != nil {
		config.ObserveChildStatus(completion.ExitCode)
	}
	return completion.ExitCode
}

func (services RunServices) refusedEvidenceWrite(action string, err error) error {
	if err == nil || !services.Evidence.WriteRefused(err) {
		return err
	}
	return status.Fail(status.CodeEvidenceUnwritable, "%s: %v", action, err)
}

func (services RunServices) resolveActivationFailure(config RunConfig, runID string, activationError error, stop func() (ChildCompletion, error)) (int, error) {
	_, stopError := stop()
	reason := "coordination-activation-failure"
	if errors.Is(activationError, coordination.ErrCoordinationDeferred) {
		reason = "coordination-contention"
	}
	receiptError := services.Evidence.Receipt(config.EvidenceRoot, runID, "started-activation-failure", reason, config.ReservationMetadata, config.TaskClass, config.Now())
	failure := status.Fail(status.CodeSupervisionFailed, "task failed after launch: %v", errors.Join(activationError, stopError, receiptError))
	return failure.Status(), failure
}

// WaitVictimRelease observes a selected remote owner until retirement or deadline.
func (services RunServices) WaitVictimRelease(parent context.Context, root string, victim coordination.ReservationOwner, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), max(timeout, time.Millisecond))
	defer cancel()
	for {
		present, err := services.Coordination.VictimPresent(ctx, root, victim)
		if err != nil || !present {
			return err
		}
		if err = services.Clock.Wait(ctx, 10*time.Millisecond, nil); err != nil {
			return errors.New("selected reservation victim remained owned after bounded observation")
		}
	}
}

// Run admits, supervises, and records one child process without touching unrelated processes.
func (services RunServices) Run(ctx context.Context, config RunConfig) (exitCode int, returnError error) {
	if err := ctx.Err(); err != nil {
		return 1, err
	}
	if config.Command == "" {
		return 1, errors.New("guarded command is empty")
	}
	if failure := services.Workload.ValidateCommand(config.Command); failure != nil {
		return failure.Status(), *failure
	}
	if config.TaskClass == "" {
		config.TaskClass = policy.TaskEphemeral
	}
	if config.TaskClass != policy.TaskEphemeral && config.TaskClass != policy.TaskService && config.TaskClass != policy.TaskTransactional {
		return 1, errors.New("class must be ephemeral, service, or transactional")
	}

	if config.Policy.SampleInterval == 0 {
		config.Policy = policy.DefaultPolicy()
	}
	if config.Policy.SampleInterval <= 0 || config.Policy.TerminationGrace < 0 || config.Policy.AdmissionWindow < 0 || config.Policy.LeaseWait < 0 {
		return 1, errors.New("resource policy durations are invalid")
	}

	if config.Collector == nil {
		return 1, errors.New("host collector is required")
	}

	config = services.Workload.Normalize(config)
	if config.Now == nil {
		config.Now = services.Clock.Now
	}

	concurrencyEnvironment, concurrencyError := NormalizeConcurrencyEnvironment(config.ConcurrencyEnvironment)
	if concurrencyError != nil {
		if config.ReservationPolicy.Enabled {
			return 0, policy.Stopped(policy.ReasonReplanRequired, concurrencyError)
		}

		return 1, concurrencyError
	}
	config.ConcurrencyEnvironment = concurrencyEnvironment
	if !config.ReservationPolicy.Enabled {
		config.Environment = ResolvedEnvironment(config.Environment, config.Resolution, false, config.ConcurrencyEnvironment)
	}

	if config.DiskPath == "" {
		config.DiskPath = config.WorkingDirectory
	}
	if config.DiskPath == "" {
		config.DiskPath = "."
	}

	if err := services.Evidence.Cleanup(config.EvidenceRoot, config.Now()); err != nil {
		return 1, services.refusedEvidenceWrite("preparing the evidence root", err)
	}

	session, err := services.acquire(ctx, config)
	if err != nil {
		if errors.Is(err, coordination.ErrReservationReplan) {
			return 0, policy.Stopped(policy.ReasonReplanRequired, nil)
		}
		if errors.Is(err, coordination.ErrCoordinationProtocolMismatch) {
			config.noteDeferralf("HIPPO protocol mismatch: %s.\n", err)

			return 0, policy.Stopped(policy.ReasonProtocolMismatch, nil)
		}
		if errors.Is(err, coordination.ErrReservationDeferred) {
			config.noteDeferralf("HIPPO deferred task: reservation capacity remained exhausted through the bounded wait.\n")

			return 0, policy.Stopped(policy.ReasonCapacityDeferred, nil)
		}
		if errors.Is(err, coordination.ErrCoordinationDeferred) {
			config.noteDeferralf("HIPPO deferred task: %s.\n", err)

			return 0, policy.Stopped(policy.ReasonCapacityDeferred, nil)
		}

		// Coordination state lives in the evidence root, and nothing has
		// started yet, so a refused lock, ledger, identity, or session write
		// is that root refusing a write.
		return 1, services.refusedEvidenceWrite("joining shared coordination", err)
	}
	if session == nil {
		if config.ReservationPolicy.Enabled {
			config.noteDeferralf("HIPPO deferred task: reservation capacity remained exhausted through the bounded wait.\n")
		} else {
			config.noteDeferralf(
				"HIPPO deferred task: %s; it must exit before this work is admitted.\n",
				services.Coordination.DescribeHeavy(config.EvidenceRoot),
			)
		}

		return 0, policy.Stopped(policy.ReasonCapacityDeferred, nil)
	}
	if config.ReservationPolicy.Enabled {
		config.Environment, err = ReservationEnvironment(
			config.Environment,
			config.Resolution,
			session.Allocation,
			config.ConcurrencyEnvironment,
		)
		if err != nil {
			_ = services.Coordination.Release(config, session)

			return 0, policy.Stopped(policy.ReasonReplanRequired, err)
		}
		config.Resolution.Concurrency = session.Allocation.CPU
	}
	ownershipRetired := true

	defer func() {
		if !ownershipRetired {
			if abandonError := services.Coordination.Abandon(session); policy.CarriesNoError(returnError) && abandonError != nil {
				returnError = abandonError
			}

			return
		}
		releaseError := services.Coordination.Release(config, session)
		// A peer holding the shared lock at cleanup time already left a
		// reconcilable owner mark, so the caller's completed work must not be
		// reported as a failure.
		if errors.Is(releaseError, coordination.ErrCoordinationCleanupDeferred) && policy.CarriesNoError(returnError) {
			_, _ = fmt.Fprintf(config.Stderr, "HIPPO deferred coordination cleanup: %s.\n", releaseError)

			return
		}
		exitCode, returnError = PromoteRelease(exitCode, returnError, releaseError)
	}()

	var portLease *PortLease
	if config.LeasePort != 0 { //nolint:nestif // Lease acquisition retains atomic rollback for each ownership component.
		portLease, err = services.AcquirePort(config)

		if errors.Is(err, coordination.ErrCoordinationDeferred) {
			config.noteDeferralf("HIPPO deferred task: %s.\n", err)

			return 0, policy.Stopped(policy.ReasonCapacityDeferred, nil)
		}
		if err != nil && services.Evidence.WriteRefused(err) {
			// Nothing has started, and the lease root is not the state root,
			// so a refused lease write has its own reason.
			return 1, status.Fail(status.CodeLeaseUnwritable, "acquiring the port lease: %v", err)
		}
		if err != nil {
			return 1, err
		}

		defer func() {
			if !ownershipRetired {
				if abandonError := services.Ports.AbandonPort(portLease); policy.CarriesNoError(returnError) && abandonError != nil {
					returnError = abandonError
				}

				return
			}
			if releaseError := services.Ports.ReleasePort(config, portLease); policy.CarriesNoError(returnError) && releaseError != nil {
				returnError = releaseError
				exitCode = 1
			}
		}()
	}

	if session.Inherited {
		lifetime, launchError := services.Workload.Start(ctx, config, session, portLease)
		if launchError != nil {
			if lifetime != nil {
				ownershipRetired = false
			}

			return 1, launchError
		}
		ownershipRetired = false
		stopLifetime := func() (ChildCompletion, error) {
			waitError, stopError := services.Workload.Stop(config, lifetime)
			ownershipRetired = !errors.Is(stopError, ErrChildRetirementUnconfirmed)

			return waitError, stopError
		}
		select {
		case waitError := <-lifetime.Done:
			ownershipRetired = true

			return childStatus(config, waitError), nil
		case <-ctx.Done():
			waitError, stopError := stopLifetime()
			if stopError != nil {
				return 1, stopError
			}

			return childStatus(config, waitError), nil
		}
	}

	writer, err := services.Evidence.Open(config)
	if err != nil {
		return 1, services.refusedEvidenceWrite("opening the evidence stream", err)
	}

	writer.SetIdentity(config.ReservationMetadata)
	writer.SetContext(config.Resolution, config.ConfigHash)
	if config.ReservationPolicy.Enabled {
		totals, statusError := services.Coordination.Snapshot(ctx, config.EvidenceRoot, "reservation")
		// A peer holding the shared lock here is contention, and no child has
		// started yet. The caller receives exit 124 naming
		// hippo.limit.capacity-deferred with a never-started receipt.
		if errors.Is(statusError, coordination.ErrCoordinationDeferred) {
			config.noteDeferralf("HIPPO deferred task: %s.\n", statusError)

			return 0, policy.Stopped(policy.ReasonCapacityDeferred, nil)
		}
		if statusError != nil {
			return 1, statusError
		}
		writer.SetReservationContext(session, totals.ActiveOwners, evidencedomain.BudgetOutcomeAdmitted)
	}
	// No outcome is decided yet: every way out of this lifetime names its own,
	// and finalOutcome refuses to record one that never did.
	var outcome evidencedomain.Outcome
	launched, finalized := false, false
	finalize := func() error {
		if finalized {
			return nil
		}
		finalized = true
		if config.ReservationPolicy.Enabled {
			peakOwners, peakError := services.Coordination.OwnerPeak(context.WithoutCancel(ctx), config.EvidenceRoot, session)
			// The owner peak is evidence metadata: a contended shared root costs
			// the summary one observation, never the completed run its outcome.
			if peakError != nil && !errors.Is(peakError, coordination.ErrCoordinationDeferred) {
				return peakError
			}
			if peakError == nil {
				writer.ObserveReservationOwners(peakOwners)
			}
		}
		recorded, outcomeError := FinalOutcome(outcome)
		finalizeError := writer.Finalize(config.TaskClass, recorded, 0)
		cleanupError := services.Evidence.Cleanup(config.EvidenceRoot, config.Now())

		return errors.Join(outcomeError, finalizeError, cleanupError)
	}

	defer func() { exitCode, returnError = services.PromoteFinalize(exitCode, returnError, finalize(), launched) }()

	deadline := config.Now().Add(config.Policy.AdmissionWindow)
	var previous policy.CPUState
	samples := []policy.Sample{}
	// A caller that cancels while the host is still being sampled cancels
	// work that never started, exactly as one that cancels a queued waiter
	// does, and leaves the same receipt so it may requeue once.
	cancelledBeforeLaunch := func(cause error) (int, error) {
		outcome = evidencedomain.OutcomeAdmissionCancelled
		// The receipt's reason is the outcome's own word, so the two records agree.
		receiptError := services.Evidence.Receipt(
			config.EvidenceRoot, writer.RunID, "never-started", outcome.String(),
			config.ReservationMetadata, config.TaskClass, config.Now(),
		)
		if receiptError != nil {
			outcome = evidencedomain.OutcomeAdmissionFailed
		}

		return 1, errors.Join(cause, services.refusedEvidenceWrite("writing the never-started receipt", receiptError))
	}

	// The admission window closed before the host was safe: the work never
	// started, and the receipt says so for a caller deciding whether to retry.
	deferAtTheDeadline := func() (int, error) {
		outcome = evidencedomain.OutcomeCapacityDeferred
		config.noteDeferralf("HIPPO deferred task: safe admission was not reached.\n")
		if receiptError := services.Evidence.Receipt(
			config.EvidenceRoot, writer.RunID, "never-started", "host-admission",
			config.ReservationMetadata, config.TaskClass, config.Now(),
		); receiptError != nil {
			outcome = evidencedomain.OutcomeAdmissionFailed

			return 1, services.refusedEvidenceWrite("writing the never-started receipt", receiptError)
		}

		return 0, policy.Stopped(policy.ReasonCapacityDeferred, nil)
	}

admission:
	for {
		if err := ctx.Err(); err != nil {
			return cancelledBeforeLaunch(err)
		}

		reading, collectError := config.Collector.Collect(ctx, previous, config.DiskPath)
		if collectError != nil {
			if ctx.Err() != nil {
				return cancelledBeforeLaunch(ctx.Err())
			}
			outcome = evidencedomain.OutcomeAdmissionFailed

			return 1, collectError
		}

		previous = reading.CPUState
		samples = append(samples, reading.Sample)
		if appendError := writer.Append(reading.Sample); appendError != nil {
			outcome = evidencedomain.OutcomeAdmissionFailed

			return 1, services.refusedEvidenceWrite("recording a host sample", appendError)
		}

		path, decisionError := policy.DecideAdmission(policy.AdmissionInput{
			Resolution: config.Resolution, TaskClass: config.TaskClass, Samples: samples,
			Policy: config.Policy, Window: policy.WindowSampling,
		})
		// A refused decision is a fault whatever path came beside its error, and no path at
		// all is a fault too: refuse both here, before any path is acted on.
		if decisionError != nil || path == policy.AdmissionUnset {
			outcome = evidencedomain.OutcomeAdmissionFailed

			return 1, status.Fail(status.CodeSupervisionFailed, "the admission decision was refused: %v", decisionError)
		}

		switch path {
		case policy.AdmissionCleanup:
			outcome = evidencedomain.OutcomeStorageBlocked
			// A resolution already at cleanup decided its own path before any sample was
			// read, so its reason, not the assessment of a host that may be healthy, is why.
			cause := policy.ResourceAssessment(samples, config.Policy).Reason
			if config.Resolution.Decision == policy.DecisionCleanup {
				cause = config.Resolution.Reason.String()
			}
			_, _ = fmt.Fprintf(config.Stderr, "HIPPO blocked task: %s; storage inspection or cleanup is required.\n", cause)

			return 0, policy.Stopped(policy.ReasonStorageBlocked, nil)
		case policy.AdmissionReplan:
			outcome = evidencedomain.OutcomeAdmissionFailed

			return 0, policy.Stopped(policy.ReasonReplanRequired, nil)
		case policy.AdmissionNormal:
			break admission
		case policy.AdmissionDegraded:
			if !config.ReservationPolicy.Enabled {
				config.Resolution.Concurrency = 1
				config.Environment = ResolvedEnvironment(config.Environment, config.Resolution, true, config.ConcurrencyEnvironment)
			}
			writer.SetContext(config.Resolution, config.ConfigHash)
			_, _ = fmt.Fprintln(config.Stderr, "HIPPO admitting ephemeral child under stable macOS warning pressure with concurrency 1.")

			break admission
		case policy.AdmissionWait:
			if !config.Now().Before(deadline) {
				return deferAtTheDeadline()
			}
		case policy.AdmissionUnset:
			// Refused above, with the error DecideAdmission returns beside it.
		}

		if err := services.Clock.Wait(ctx, config.Policy.SampleInterval, config.Sleep); err != nil {
			return cancelledBeforeLaunch(err)
		}
	}

	lifetime, launchError := services.Workload.Start(ctx, config, session, portLease)
	if launchError != nil {
		outcome = evidencedomain.OutcomeAdmissionFailed
		if lifetime != nil {
			ownershipRetired = false
		}

		return 1, launchError
	}
	launched = true
	ownershipRetired = false
	stopLifetime := func() (ChildCompletion, error) {
		waitError, stopError := services.Workload.Stop(config, lifetime)
		ownershipRetired = !errors.Is(stopError, ErrChildRetirementUnconfirmed)

		return waitError, stopError
	}
	if config.ReservationPolicy.Enabled {
		if activationError := services.Workload.Activate(config, session, lifetime); activationError != nil {
			outcome = evidencedomain.OutcomeTaskFailed

			return services.resolveActivationFailure(config, writer.RunID, activationError, stopLifetime)
		}
	}

	ticker := services.Clock.Ticker(config.Policy.SampleInterval)
	defer ticker.Stop()
	var warningSince *time.Time

	for {
		select {
		case waitError := <-lifetime.Done:
			ownershipRetired = true
			if !waitError.Failed {
				outcome = evidencedomain.OutcomePassed
			} else {
				outcome = evidencedomain.OutcomeTaskFailed
			}

			return childStatus(config, waitError), nil

		case <-ctx.Done():
			waitError, stopError := stopLifetime()
			outcome = evidencedomain.OutcomeTaskFailed
			if stopError != nil {
				return 1, stopError
			}

			return childStatus(config, waitError), nil

		case <-ticker.Ticks:
			if config.ReservationPolicy.Enabled { //nolint:nestif // Owner-side marked shedding must remain ahead of fresh pressure collection.
				selected, selectedCause, selectionError := services.Coordination.SheddingSelection(config.EvidenceRoot, session)
				// A contended shared root defers this observation to the next
				// sample instead of costing the caller a healthy child.
				if selectionError != nil && !errors.Is(selectionError, coordination.ErrCoordinationDeferred) {
					outcome = evidencedomain.OutcomeSupervisionFailed
					_, stopError := stopLifetime()

					return 1, errors.Join(selectionError, stopError)
				}
				if selectionError == nil && selected {
					outcome = evidencedomain.OutcomePressureShed
					if config.TaskClass == policy.TaskTransactional {
						outcome = evidencedomain.OutcomeEmergencySafetyStop
					}
					if selectedCause == coordination.ShedCauseStorage {
						outcome = evidencedomain.OutcomeStorageShed
					}
					_, _ = fmt.Fprintln(config.Stderr, "HIPPO shedding this selected child from its owning guard.")
					_, stopError := stopLifetime()
					if outcome == evidencedomain.OutcomeEmergencySafetyStop {
						stopError = errors.Join(stopError, services.Evidence.Receipt(
							config.EvidenceRoot, session.Token, "started-safety-stop", "emergency-pressure",
							config.ReservationMetadata, config.TaskClass, config.Now(),
						))
					}

					return 0, policy.Stopped(selectedCause.Reason(), stopError)
				}
			}
			reading, collectError := config.Collector.Collect(ctx, previous, config.DiskPath)
			if collectError != nil {
				if ctx.Err() != nil {
					waitError, stopError := stopLifetime()
					outcome = evidencedomain.OutcomeTaskFailed
					if stopError != nil {
						return 1, stopError
					}

					return childStatus(config, waitError), nil
				}

				outcome = evidencedomain.OutcomeSupervisionFailed
				_, stopError := stopLifetime()

				return 1, errors.Join(lostAfterLaunch(collectError), stopError)
			}

			previous = reading.CPUState
			samples = append(samples, reading.Sample)
			limit := int(config.Policy.TrendWindow/config.Policy.SampleInterval) + 2
			if len(samples) > limit {
				samples = samples[len(samples)-limit:]
			}

			if appendError := writer.Append(reading.Sample); appendError != nil {
				outcome = evidencedomain.OutcomeSupervisionFailed
				_, stopError := stopLifetime()

				return 1, errors.Join(appendError, stopError)
			}
			if config.ReservationPolicy.Enabled {
				peakOwners, statusError := services.Coordination.OwnerPeak(ctx, config.EvidenceRoot, session)
				// The owner peak is evidence metadata. A contended shared root,
				// or a caller cancelling mid-observation, costs this sample its
				// observation: never the child, and never the caller's exit.
				if statusError != nil && !errors.Is(statusError, coordination.ErrCoordinationDeferred) && ctx.Err() == nil {
					outcome = evidencedomain.OutcomeSupervisionFailed
					_, stopError := stopLifetime()

					return 1, errors.Join(statusError, stopError)
				}
				if statusError == nil {
					writer.ObserveReservationOwners(peakOwners)
				}
			}

			assessment := policy.ResourceAssessment(samples, config.Policy)
			// A stable macOS warning never counts toward the grace of a running
			// ephemeral child whose profile may use degraded admission, however
			// that child was admitted: the same warning would admit it now.
			stableWarning := policy.SparesStableWarning(config.TaskClass, config.Resolution, samples, config.Policy)
			if assessment.State == policy.StateNormal || stableWarning {
				warningSince = nil
			} else if assessment.State == policy.StateWarning && warningSince == nil {
				value := config.Now()
				warningSince = &value
			}

			if config.TaskClass == policy.TaskTransactional && !config.ReservationPolicy.Enabled {
				continue
			}

			grace := config.Policy.EphemeralWarningGrace
			if config.TaskClass == policy.TaskService {
				grace = config.Policy.ServiceWarningGrace
			}

			if assessment.State == policy.StateCritical || (warningSince != nil && config.Now().Sub(*warningSince) >= grace) { //nolint:nestif // Pressure outcome, atomic victim election, and owned reaping remain one lifecycle branch.
				shedCause := coordination.ShedCausePressure
				if assessment.StorageBlocked {
					shedCause, outcome = coordination.ShedCauseStorage, evidencedomain.OutcomeStorageShed
				} else {
					outcome = evidencedomain.OutcomePressureShed
				}

				if config.ReservationPolicy.Enabled {
					emergency := false
					available := reading.Sample.AvailableMemoryBytes
					if available == nil {
						available = reading.Sample.AvailableNonCompressedEstimateBytes
					}
					if config.EmergencyAvailableMemoryBytes > 0 && available != nil &&
						*available < config.EmergencyAvailableMemoryBytes {
						emergency = true
					}
					if config.EmergencyAvailableMemoryBytes > 0 && assessment.State == policy.StateCritical &&
						!assessment.StorageBlocked {
						emergency = true
					}
					var victim coordination.ReservationOwner
					var selected bool
					var selectionError error
					if emergency {
						victim, selected, selectionError = services.Coordination.SelectVictim(config.EvidenceRoot, shedCause, true)
					} else {
						victim, selected, selectionError = services.Coordination.SelectVictim(config.EvidenceRoot, shedCause, false)
					}
					// Pressure persists across samples, so a contended shared
					// root re-elects on the next one rather than shedding work
					// no one has been selected for.
					if errors.Is(selectionError, coordination.ErrCoordinationDeferred) {
						continue
					}
					if selectionError != nil {
						_, stopError := stopLifetime()

						return 1, errors.Join(selectionError, stopError)
					}
					if !selected {
						continue
					}
					_, _ = fmt.Fprintf(config.Stderr, "HIPPO shedding selected %s child after %s.\n", victim.Class, assessment.Reason)
					if victim.Class == policy.TaskTransactional {
						outcome = evidencedomain.OutcomeEmergencySafetyStop
						_, _ = fmt.Fprintln(config.Stderr, "HIPPO emergency safety stop selected transactional work; it will not retry automatically.")
					}
					if victim.Token != session.Token {
						observation := 2*config.Policy.SampleInterval + 2*config.Policy.TerminationGrace
						if remoteError := services.WaitVictimRelease(ctx, config.EvidenceRoot, victim, observation); remoteError != nil {
							_, stopError := stopLifetime()

							return 1, errors.Join(remoteError, stopError)
						}

						continue // The next pressure decision requires a fresh host sample after owned release.
					}
				} else {
					_, _ = fmt.Fprintf(config.Stderr, "HIPPO shedding %s child after %s.\n", config.TaskClass, assessment.Reason)
				}
				_, stopError := stopLifetime()
				if outcome == evidencedomain.OutcomeEmergencySafetyStop {
					stopError = errors.Join(stopError, services.Evidence.Receipt(
						config.EvidenceRoot, session.Token, "started-safety-stop", "emergency-pressure",
						config.ReservationMetadata, config.TaskClass, config.Now(),
					))
				}

				return 0, policy.Stopped(shedCause.Reason(), stopError)
			}
		}
	}
}
