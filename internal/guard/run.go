package guard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/internal/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
)

const (
	// StorageBlockedExitCode indicates cleanup is required before retrying.
	StorageBlockedExitCode = 73
	// CapacityDeferredExitCode indicates transient pressure that should be retried.
	CapacityDeferredExitCode = 75
	outcomeTaskFailed        = "task-failed"
	outcomeSupervisionFailed = "supervision-failed"
)

// RunConfig describes one guarded child process and its resource policy.
type RunConfig struct {
	Command                               string
	Arguments                             []string
	TaskClass                             policy.TaskClass
	WorkingDirectory                      string
	Environment                           []string
	EvidenceRoot                          string
	DiskPath                              string
	LeasePort, LeaseMinimum, LeaseMaximum int
	LeaseOwner                            string
	ConcurrencyEnvironment                []string
	PortLeaseRoot                         string
	Collector                             policy.Collector
	Policy                                policy.Policy
	Resolution                            policy.Resolution
	ReservationPolicy                     ReservationPolicy
	ReservationPlan                       ReservationPlan
	EvidenceLimits                        evidence.Limits
	ConfigHash                            string
	Now                                   func() time.Time
	Sleep                                 func(time.Duration)
	ChildStdin                            io.Reader
	ChildStdout, ChildStderr              io.Writer
	Stderr                                io.Writer
	startLifetime                         func(context.Context, RunConfig, string, []string, ...*os.File) (*supervisedLifetime, error)
	stopLifetime                          func(*supervisedLifetime, time.Duration) (error, error)
}

func environmentValue(environment []string, name string) string {
	prefix := name + "="
	for _, entry := range environment {
		if len(entry) >= len(prefix) && entry[:len(prefix)] == prefix {
			return entry[len(prefix):]
		}
	}

	return ""
}

func withEnvironment(environment []string, name, value string) []string {
	result := make([]string, 0, len(environment)+1)
	prefix := name + "="
	for _, entry := range environment {
		if len(entry) < len(prefix) || entry[:len(prefix)] != prefix {
			result = append(result, entry)
		}
	}

	return append(result, prefix+value)
}

func withEnvironmentIfMissing(environment []string, name, value string) []string {
	if environmentValue(environment, name) != "" {
		return environment
	}

	return withEnvironment(environment, name, value)
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

func normalizeConcurrencyEnvironment(names []string) ([]string, error) {
	seen := map[string]bool{}
	result := make([]string, 0, len(names))

	for _, name := range names {
		if !validEnvironmentName(name) {
			return nil, fmt.Errorf("concurrency environment name %q is not a POSIX identifier", name)
		}
		if reservedConcurrencyEnvironment(name) {
			return nil, fmt.Errorf("concurrency environment name %q is reserved", name)
		}
		if !seen[name] {
			result = append(result, name)
			seen[name] = true
		}
	}

	return result, nil
}

func resolvedEnvironment(environment []string, resolution policy.Resolution, forceConcurrency bool, names []string) []string {
	if resolution.ResolvedProfile == "" || resolution.Concurrency <= 0 {
		return environment
	}

	concurrency := strconv.Itoa(resolution.Concurrency)
	environment = withEnvironment(environment, "HIPPO_PROFILE", resolution.ResolvedProfile)
	environment = withEnvironment(environment, "HIPPO_CONCURRENCY", concurrency)
	for _, name := range names {
		if forceConcurrency {
			environment = withEnvironment(environment, name, concurrency)
		} else {
			environment = withEnvironmentIfMissing(environment, name, concurrency)
		}
	}

	return environment
}

// ReservationEnvironment exports a fixed allocation and safely clamps mapped worker variables.
func ReservationEnvironment(
	environment []string,
	resolution policy.Resolution,
	allocation ReservationVector,
	names []string,
) ([]string, error) {
	if allocation.CPU < MinimumReservationCPU || allocation.MemoryBytes < MinimumReservationMemoryBytes {
		return nil, errors.New("reservation allocation is below the immutable floor")
	}

	concurrency := strconv.Itoa(allocation.CPU)
	environment = withEnvironment(environment, "HIPPO_PROFILE", resolution.ResolvedProfile)
	environment = withEnvironment(environment, "HIPPO_CONCURRENCY", concurrency)
	environment = withEnvironment(environment, "HIPPO_RESERVED_MEMORY_BYTES", strconv.FormatInt(allocation.MemoryBytes, 10))
	for _, name := range names {
		value := environmentValue(environment, name)
		if value == "" {
			environment = withEnvironment(environment, name, concurrency)

			continue
		}
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("concurrency environment %q must be a positive integer", name)
		}
		if parsed > allocation.CPU {
			environment = withEnvironment(environment, name, concurrency)
		}
	}

	return environment, nil
}

func waitStatusCode(err error) int {
	if err == nil {
		return 0
	}

	if exitError, ok := errors.AsType[*exec.ExitError](err); ok {
		if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
			if status.Signaled() {
				return 128 + int(status.Signal())
			}
			return status.ExitStatus()
		}
	}

	return 1
}

func signalGroup(processGroup int, signal syscall.Signal) error {
	if processGroup <= 0 {
		return errors.New("child process group is unavailable")
	}

	groupError := syscall.Kill(-processGroup, signal)
	// A group signal reports ESRCH once the group is gone and, on Darwin, EPERM
	// while its members have exited but have not been reaped yet: no member is
	// left that this process may signal. Neither answer says the payload is
	// still running, and a stop the child already completed is not a
	// supervision failure. Liveness is decided by the authoritative lifetime
	// exit, never by the delivery result of a signal.
	if groupError == nil || errors.Is(groupError, syscall.ESRCH) || errors.Is(groupError, syscall.EPERM) {
		return nil
	}

	return groupError
}

func waitReservationVictimRelease(root string, victim ReservationOwner, timeout time.Duration) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), max(timeout, time.Millisecond))
	defer cancel()
	for {
		present, err := reservationVictimPresent(ctx, root, victim)
		if err != nil || !present {
			return !present, err
		}
		if err = waitForContext(ctx, coordinationPollInterval, nil); err != nil {
			return false, nil
		}
	}
}

// WaitPressureVictimRelease observes a selected remote owner through a bounded
// deadline. The remote selector never signals another guard's process group;
// only the owning guard may stop, reap, and release its supervised child.
func WaitPressureVictimRelease(root string, victim ReservationOwner, timeout time.Duration) error {
	released, observeError := waitReservationVictimRelease(root, victim, timeout)
	if !released && observeError == nil {
		observeError = errors.New("selected reservation victim remained owned after bounded observation")
	}

	return observeError
}

var errChildRetirementUnconfirmed = errors.New("child process-group retirement remained unconfirmed")

// childRetirementConfirmation bounds the wait for a killed process group to
// retire. It is deliberately not the caller's termination grace: that grace is
// a shutdown policy for a cooperating child, while this window measures how
// long the operating system needs to kill, orphan-reparent, and reap a group.
// Deriving one from the other lets an aggressive grace report a healthy forced
// stop as unconfirmed, which keeps the reservation owned and leaks capacity
// from a coordination root every repository shares.
const childRetirementConfirmation = 2 * time.Second

func terminateAndWait(lifetime *supervisedLifetime, grace time.Duration) (error, error) {
	select {
	case waitError := <-lifetime.exited:
		return waitError, nil
	default:
	}

	signalError := signalGroup(lifetime.processGroup, syscall.SIGTERM)
	timer := time.NewTimer(grace)
	defer timer.Stop()

	select {
	case waitError := <-lifetime.exited:
		return waitError, signalError
	case <-timer.C:
		killError := signalGroup(lifetime.processGroup, syscall.SIGKILL)
		postKill := time.NewTimer(max(grace, childRetirementConfirmation))
		defer postKill.Stop()
		select {
		case waitError := <-lifetime.exited:
			return waitError, errors.Join(signalError, killError)
		case <-postKill.C:
			return nil, errors.Join(signalError, killError, errChildRetirementUnconfirmed)
		}
	}
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

func launchConfiguredLifetime(
	ctx context.Context,
	config RunConfig,
	session *Session,
	portLease *PortLease,
) (*supervisedLifetime, error) {
	environment := withEnvironment(config.Environment, "HIPPO_SESSION", session.Token)
	executable, err := exec.LookPath(config.Command)
	if err != nil {
		return nil, err
	}
	environment = withEnvironment(environment, "HIPPO_BIN", executableGuardPath())
	var reservationIdentity, portIdentity *os.File
	if session != nil {
		reservationIdentity = session.identityLock
	}
	if portLease != nil {
		portIdentity = portLease.identityLock
	}

	starter := startSupervisedLifetime
	if config.startLifetime != nil {
		starter = config.startLifetime
	}

	return starter(ctx, config, executable, environment, reservationIdentity, portIdentity)
}

func stopConfiguredLifetime(config RunConfig, lifetime *supervisedLifetime) (error, error) {
	if config.stopLifetime != nil {
		return config.stopLifetime(lifetime, config.Policy.TerminationGrace)
	}

	return terminateAndWait(lifetime, config.Policy.TerminationGrace)
}

// Run admits, supervises, and records one child process without touching unrelated processes.
func Run(ctx context.Context, config RunConfig) (exitCode int, returnError error) {
	if err := ctx.Err(); err != nil {
		return 1, err
	}
	if config.Command == "" {
		return 1, errors.New("guarded command is empty")
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

	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Stderr == nil {
		config.Stderr = os.Stderr
	}
	if config.ChildStdin == nil {
		config.ChildStdin = os.Stdin
	}
	if config.ChildStdout == nil {
		config.ChildStdout = os.Stdout
	}
	if config.ChildStderr == nil {
		config.ChildStderr = os.Stderr
	}
	if config.Environment == nil {
		config.Environment = os.Environ()
	}

	concurrencyEnvironment, concurrencyError := normalizeConcurrencyEnvironment(config.ConcurrencyEnvironment)
	if concurrencyError != nil {
		if config.ReservationPolicy.Enabled {
			return policy.ReplanRequiredExitCode, concurrencyError
		}

		return 1, concurrencyError
	}
	config.ConcurrencyEnvironment = concurrencyEnvironment
	if !config.ReservationPolicy.Enabled {
		config.Environment = resolvedEnvironment(config.Environment, config.Resolution, false, config.ConcurrencyEnvironment)
	}

	if config.DiskPath == "" {
		config.DiskPath = config.WorkingDirectory
	}
	if config.DiskPath == "" {
		config.DiskPath = "."
	}

	if err := evidence.Cleanup(config.EvidenceRoot, config.Now()); err != nil {
		return 1, err
	}

	var session *Session
	var err error
	if config.ReservationPolicy.Enabled {
		session, err = AcquireReservation(
			ctx,
			config.EvidenceRoot,
			environmentValue(config.Environment, "HIPPO_SESSION"),
			config.TaskClass,
			config.Resolution.ResolvedProfile,
			config.ConfigHash,
			config.ReservationPlan,
			config.ReservationPolicy.MaxActiveOwners,
			config.Policy.LeaseWait,
		)
	} else {
		session, err = AcquireSession(
			ctx,
			config.EvidenceRoot,
			environmentValue(config.Environment, "HIPPO_SESSION"),
			config.TaskClass,
			config.Policy.LeaseWait,
		)
	}
	if err != nil {
		if errors.Is(err, ErrReservationReplan) {
			return policy.ReplanRequiredExitCode, nil
		}
		if errors.Is(err, ErrReservationDeferred) {
			_, _ = fmt.Fprintln(config.Stderr, "HIPPO deferred task: reservation capacity remained exhausted through the bounded wait.")

			return CapacityDeferredExitCode, nil
		}
		if errors.Is(err, errCoordinationDeferred) {
			_, _ = fmt.Fprintf(config.Stderr, "HIPPO deferred task: %s.\n", err)

			return CapacityDeferredExitCode, nil
		}

		return 1, err
	}
	if session == nil {
		if config.ReservationPolicy.Enabled {
			_, _ = fmt.Fprintln(config.Stderr, "HIPPO deferred task: reservation capacity remained exhausted through the bounded wait.")
		} else {
			_, _ = fmt.Fprintf(
				config.Stderr,
				"HIPPO deferred task: %s; it must exit before this work is admitted.\n",
				DescribeHeavyLease(config.EvidenceRoot),
			)
		}

		return CapacityDeferredExitCode, nil
	}
	if config.ReservationPolicy.Enabled {
		config.Environment, err = ReservationEnvironment(
			config.Environment,
			config.Resolution,
			session.Allocation,
			config.ConcurrencyEnvironment,
		)
		if err != nil {
			_ = ReleaseReservation(config.EvidenceRoot, session) //nolint:contextcheck // Rejected environment cleanup owns its bounded release context.

			return policy.ReplanRequiredExitCode, err
		}
		config.Resolution.Concurrency = session.Allocation.CPU
	}
	ownershipRetired := true

	defer func() { //nolint:contextcheck // Ownership release runs after caller cancellation and deliberately uses its own bounded context.
		if !ownershipRetired {
			if abandonError := abandonReservationIdentity(session); returnError == nil && abandonError != nil {
				returnError = abandonError
			}

			return
		}
		var releaseError error
		if config.ReservationPolicy.Enabled {
			releaseError = ReleaseReservation(config.EvidenceRoot, session)
		} else {
			releaseError = ReleaseSession(config.EvidenceRoot, session)
		}
		if returnError == nil && releaseError != nil {
			// A peer holding the shared lock at cleanup time already left a
			// reconcilable owner mark, so the caller's completed work must not
			// be reported as a failure.
			if errors.Is(releaseError, ErrCoordinationCleanupDeferred) {
				_, _ = fmt.Fprintf(config.Stderr, "HIPPO deferred coordination cleanup: %s.\n", releaseError)

				return
			}
			returnError = releaseError
			if exitCode != StorageBlockedExitCode && exitCode != CapacityDeferredExitCode {
				exitCode = 1
			}
		}
	}()

	var portLease *PortLease
	if config.LeasePort != 0 { //nolint:nestif // Lease acquisition retains atomic rollback for each ownership component.
		root := config.PortLeaseRoot
		if root == "" {
			root = filepath.Join(os.TempDir(), "hippo-port-leases")
		}

		portLease, err = AcquirePortLease(root, config.LeasePort, config.LeaseOwner, config.LeaseMinimum, config.LeaseMaximum)
		if errors.Is(err, errCoordinationDeferred) {
			_, _ = fmt.Fprintf(config.Stderr, "HIPPO deferred task: %s.\n", err)

			return CapacityDeferredExitCode, nil
		}
		if err != nil {
			return 1, err
		}

		defer func() {
			if !ownershipRetired {
				if abandonError := abandonPortLeaseIdentity(portLease); returnError == nil && abandonError != nil {
					returnError = abandonError
				}

				return
			}
			if releaseError := ReleasePortLease(root, portLease); returnError == nil && releaseError != nil {
				returnError = releaseError
				exitCode = 1
			}
		}()
	}

	if session.Inherited {
		lifetime, launchError := launchConfiguredLifetime(ctx, config, session, portLease)
		if launchError != nil {
			if lifetime != nil {
				ownershipRetired = false
			}

			return 1, launchError
		}
		ownershipRetired = false
		stopLifetime := func() (error, error) {
			waitError, stopError := stopConfiguredLifetime(config, lifetime)
			ownershipRetired = !errors.Is(stopError, errChildRetirementUnconfirmed)

			return waitError, stopError
		}
		select {
		case waitError := <-lifetime.exited:
			ownershipRetired = true

			return waitStatusCode(waitError), nil
		case <-ctx.Done():
			waitError, stopError := stopLifetime()
			if stopError != nil {
				return 1, stopError
			}

			return waitStatusCode(waitError), nil
		}
	}

	writer, err := NewEvidenceWriter(
		config.EvidenceRoot,
		EvidenceIdentifier("development-"+string(config.TaskClass), config.Now(), os.Getpid()),
		config.EvidenceLimits,
	)
	if err != nil {
		return 1, err
	}

	writer.SetContext(config.Resolution, config.ConfigHash)
	if config.ReservationPolicy.Enabled {
		totals, statusError := ReservationStatus(ctx, config.EvidenceRoot)
		// A peer holding the shared lock here is contention, and no child has
		// started yet, so the caller receives the retryable deferral exit.
		if errors.Is(statusError, errCoordinationDeferred) {
			_, _ = fmt.Fprintf(config.Stderr, "HIPPO deferred task: %s.\n", statusError)

			return CapacityDeferredExitCode, nil
		}
		if statusError != nil {
			return 1, statusError
		}
		writer.SetReservationContext(session, totals.ActiveOwners, "admitted")
	}
	outcome := "capacity-deferred"
	finalized := false
	finalize := func() error { //nolint:contextcheck // Evidence finalization deliberately uses the bounded ownership lifecycle, not caller cancellation.
		if finalized {
			return nil
		}
		finalized = true
		if config.ReservationPolicy.Enabled {
			peakOwners, peakError := ReservationOwnerPeak(context.Background(), config.EvidenceRoot, session)
			// The owner peak is evidence metadata: a contended shared root costs
			// the summary one observation, never the completed run its outcome.
			if peakError != nil && !errors.Is(peakError, errCoordinationDeferred) {
				return peakError
			}
			if peakError == nil {
				writer.ObserveReservationOwners(peakOwners)
			}
		}
		_, finalizeError := writer.Finalize(config.TaskClass, outcome, 0)
		cleanupError := evidence.Cleanup(config.EvidenceRoot, config.Now())

		return errors.Join(finalizeError, cleanupError)
	}

	defer func() {
		if finalizeError := finalize(); returnError == nil && finalizeError != nil {
			returnError = finalizeError
			exitCode = 1
		}
	}()

	deadline := config.Now().Add(config.Policy.AdmissionWindow)
	var previous policy.CPUState
	samples := []policy.Sample{}
	admitted, degraded := false, false

	for {
		if err := ctx.Err(); err != nil {
			return 1, err
		}

		reading, collectError := config.Collector.Collect(ctx, previous, config.DiskPath)
		if collectError != nil {
			return 1, collectError
		}

		previous = reading.CPUState
		samples = append(samples, reading.Sample)
		if appendError := writer.Append(reading.Sample); appendError != nil {
			return 1, appendError
		}

		assessment := policy.ResourceAssessment(samples, config.Policy)
		if assessment.StorageBlocked {
			outcome = "storage-blocked"
			_, _ = fmt.Fprintf(config.Stderr, "HIPPO blocked task: %s; storage inspection or cleanup is required.\n", assessment.Reason)

			return StorageBlockedExitCode, nil
		}

		if policy.AdmissionReady(samples, config.Policy) {
			admitted = true
			break
		}

		if config.TaskClass == policy.TaskEphemeral &&
			config.Resolution.ResolvedProfile == "balanced" &&
			policy.WarningAdmissionReady(samples, config.Policy) {
			admitted, degraded = true, true
			if !config.ReservationPolicy.Enabled {
				config.Resolution.Concurrency = 1
				config.Environment = resolvedEnvironment(config.Environment, config.Resolution, true, config.ConcurrencyEnvironment)
			}
			writer.SetContext(config.Resolution, config.ConfigHash)
			_, _ = fmt.Fprintln(config.Stderr, "HIPPO admitting ephemeral child under stable macOS warning pressure with concurrency 1.")

			break
		}

		if !config.Now().Before(deadline) {
			break
		}

		if err := waitForContext(ctx, config.Policy.SampleInterval, config.Sleep); err != nil {
			return 1, err
		}
	}

	if !admitted {
		_, _ = fmt.Fprintln(config.Stderr, "HIPPO deferred task: safe admission was not reached.")

		return CapacityDeferredExitCode, nil
	}

	lifetime, launchError := launchConfiguredLifetime(ctx, config, session, portLease)
	if launchError != nil {
		if lifetime != nil {
			ownershipRetired = false
		}

		return 1, launchError
	}
	ownershipRetired = false
	stopLifetime := func() (error, error) {
		waitError, stopError := stopConfiguredLifetime(config, lifetime)
		ownershipRetired = !errors.Is(stopError, errChildRetirementUnconfirmed)

		return waitError, stopError
	}
	if config.ReservationPolicy.Enabled {
		if activationError := ActivateReservation(config.EvidenceRoot, session, lifetime.processGroup); activationError != nil { //nolint:contextcheck // Activation owns a bounded atomic coordination transaction.
			_, stopError := stopLifetime()
			// Every repository on the host shares one coordination root, so a
			// peer holding its lock through this bounded window is ordinary
			// contention. The caller must receive the retryable deferral exit
			// instead of a generic failure it cannot classify.
			if errors.Is(activationError, errCoordinationDeferred) && stopError == nil {
				_, _ = fmt.Fprintf(config.Stderr, "HIPPO deferred task: %s.\n", activationError)

				return CapacityDeferredExitCode, nil
			}

			return 1, errors.Join(activationError, stopError)
		}
	}

	ticker := time.NewTicker(config.Policy.SampleInterval)
	defer ticker.Stop()
	var warningSince *time.Time

	for {
		select {
		case waitError := <-lifetime.exited:
			ownershipRetired = true
			if waitError == nil {
				outcome = "passed"
			} else {
				outcome = outcomeTaskFailed
			}

			return waitStatusCode(waitError), nil

		case <-ctx.Done():
			waitError, stopError := stopLifetime()
			outcome = outcomeTaskFailed
			if stopError != nil {
				return 1, stopError
			}

			return waitStatusCode(waitError), nil

		case <-ticker.C:
			if config.ReservationPolicy.Enabled {
				selected, selectedExit, selectionError := ReservationSheddingSelection(config.EvidenceRoot, session) //nolint:contextcheck // Owner mark observation remains bounded independently of sampling cancellation.
				// A contended shared root defers this observation to the next
				// sample instead of costing the caller a healthy child.
				if selectionError != nil && !errors.Is(selectionError, errCoordinationDeferred) {
					outcome = outcomeSupervisionFailed
					_, stopError := stopLifetime()

					return 1, errors.Join(selectionError, stopError)
				}
				if selectionError == nil && selected {
					outcome = "pressure-shed"
					if selectedExit == StorageBlockedExitCode {
						outcome = "storage-shed"
					}
					_, _ = fmt.Fprintln(config.Stderr, "HIPPO shedding this selected child from its owning guard.")
					_, stopError := stopLifetime()

					return selectedExit, stopError
				}
			}
			reading, collectError := config.Collector.Collect(ctx, previous, config.DiskPath)
			if collectError != nil {
				if ctx.Err() != nil {
					waitError, stopError := stopLifetime()
					outcome = outcomeTaskFailed
					if stopError != nil {
						return 1, stopError
					}

					return waitStatusCode(waitError), nil
				}

				outcome = outcomeSupervisionFailed
				_, stopError := stopLifetime()

				return 1, errors.Join(collectError, stopError)
			}

			previous = reading.CPUState
			samples = append(samples, reading.Sample)
			limit := int(config.Policy.TrendWindow/config.Policy.SampleInterval) + 2
			if len(samples) > limit {
				samples = samples[len(samples)-limit:]
			}

			if appendError := writer.Append(reading.Sample); appendError != nil {
				outcome = outcomeSupervisionFailed
				_, stopError := stopLifetime()

				return 1, errors.Join(appendError, stopError)
			}
			if config.ReservationPolicy.Enabled {
				peakOwners, statusError := ReservationOwnerPeak(ctx, config.EvidenceRoot, session)
				// The owner peak is evidence metadata. A contended shared root,
				// or a caller cancelling mid-observation, costs this sample its
				// observation: never the child, and never the caller's exit.
				if statusError != nil && !errors.Is(statusError, errCoordinationDeferred) && ctx.Err() == nil {
					outcome = outcomeSupervisionFailed
					_, stopError := stopLifetime()

					return 1, errors.Join(statusError, stopError)
				}
				if statusError == nil {
					writer.ObserveReservationOwners(peakOwners)
				}
			}

			assessment := policy.ResourceAssessment(samples, config.Policy)
			stableDegradedWarning := degraded && policy.WarningAdmissionReady(samples, config.Policy)
			if assessment.State == policy.StateNormal || stableDegradedWarning {
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
				shedCode := CapacityDeferredExitCode
				if assessment.StorageBlocked {
					shedCode, outcome = StorageBlockedExitCode, "storage-shed"
				} else {
					outcome = "pressure-shed"
				}

				if config.ReservationPolicy.Enabled {
					victim, selected, selectionError := SelectPressureVictim(config.EvidenceRoot, shedCode) //nolint:contextcheck // Selection is one bounded locked evaluation.
					// Pressure persists across samples, so a contended shared
					// root re-elects on the next one rather than shedding work
					// no one has been selected for.
					if errors.Is(selectionError, errCoordinationDeferred) {
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
					if victim.Token != session.Token {
						observation := 2*config.Policy.SampleInterval + 2*config.Policy.TerminationGrace
						if remoteError := WaitPressureVictimRelease(config.EvidenceRoot, victim, observation); remoteError != nil { //nolint:contextcheck // Remote observation has its own bounded deadline.
							_, stopError := stopLifetime()

							return 1, errors.Join(remoteError, stopError)
						}

						continue // The next pressure decision requires a fresh host sample after owned release.
					}
				} else {
					_, _ = fmt.Fprintf(config.Stderr, "HIPPO shedding %s child after %s.\n", config.TaskClass, assessment.Reason)
				}
				_, stopError := stopLifetime()

				return shedCode, stopError
			}
		}
	}
}

func executableGuardPath() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}

	return path
}
