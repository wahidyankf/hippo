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
	"github.com/wahidyankf/hippo/internal/status"
)

const (
	// StorageBlockedExitCode indicates cleanup is required before retrying.
	StorageBlockedExitCode = 73
	// CapacityDeferredExitCode indicates transient pressure that should be retried.
	// It is also the shed cause the reservation ledger records for pressure
	// other than storage.
	CapacityDeferredExitCode = 75
	// PressureShedExitCode reports a started child shed under host pressure
	// other than storage. Like the others it never leaves the process: the
	// command-line boundary turns it into 124 naming hippo.limit.pressure-shed,
	// so a shed is never mistaken for a deferral that started nothing.
	PressureShedExitCode     = 74
	outcomeTaskFailed        = "task-failed"
	outcomeSupervisionFailed = "supervision-failed"
	outcomeEmergencyStop     = "emergency-safety-stop"
	outcomePressureShed      = "pressure-shed"
	outcomeStorageShed       = "storage-shed"
)

// RunOutcomes is every outcome a run's lifetime summary can record. history
// uses it to tell a filter no run can match from a question with an empty
// answer.
func RunOutcomes() []string {
	return []string{
		"passed", outcomeTaskFailed, outcomeSupervisionFailed, outcomePressureShed, outcomeStorageShed,
		outcomeEmergencyStop, "capacity-deferred", "storage-blocked",
	}
}

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
	ReservationMetadata                   ReservationMetadata
	EmergencyAvailableMemoryBytes         int64
	EvidenceLimits                        evidence.Limits
	ConfigHash                            string
	Now                                   func() time.Time
	// ObserveChildStatus, when set, is called with the status a started
	// child produced, and only then. It is how the boundary tells a status
	// the child chose from one hippo chose: hippo reports a reason for its
	// own refusals, and passes a child's status through untouched and
	// unexplained, which is the only honest thing to do with it.
	ObserveChildStatus       func(int)
	Sleep                    func(time.Duration)
	ChildStdin               io.Reader
	ChildStdout, ChildStderr io.Writer
	Stderr                   io.Writer
	startLifetime            func(context.Context, RunConfig, string, []string, ...*os.File) (*supervisedLifetime, error)
	stopLifetime             func(*supervisedLifetime, time.Duration) (error, error)
}

// environmentValue resolves name against the duplicate-key semantics the child
// actually observes: os/exec dedupes Cmd.Env keeping the last occurrence, and
// withEnvironment produces that same layout by stripping prior matches before
// appending. Reading the first match instead would let an ambient value shadow a
// caller's explicit override — append(os.Environ(), "K=v") is the ordinary way to
// set one variable — and the guard would then admit against a value the child
// never sees.
func environmentValue(environment []string, name string) string {
	prefix := name + "="
	value := ""
	for _, entry := range environment {
		if len(entry) >= len(prefix) && entry[:len(prefix)] == prefix {
			value = entry[len(prefix):]
		}
	}

	return value
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

// checkCommandIsRunnable answers, before hippo admits anything, whether the
// command it was asked to guard can be run at all. A caller that misspelled a
// command deserves to hear that immediately, in the words every shell already
// uses for it, rather than after hippo has queued, reserved, and launched a
// supervisor for a program that was never there.
//
// It is a check and not a guarantee: a file can vanish between here and exec.
// That race resolves to the launcher's existing status, which is the honest
// answer when hippo genuinely cannot tell what happened.
func checkCommandIsRunnable(command string) *status.Failure {
	resolved := command
	if !strings.ContainsRune(command, os.PathSeparator) {
		found, lookError := exec.LookPath(command)
		if lookError != nil {
			if errors.Is(lookError, exec.ErrNotFound) {
				failure := status.Fail(status.CodeChildNotFound, "%s: command not found", command)

				return &failure
			}

			failure := status.Fail(status.CodeChildNotExecutable, "%s: %v", command, lookError)

			return &failure
		}
		resolved = found
	}

	info, statError := os.Stat(resolved)
	switch {
	case errors.Is(statError, os.ErrNotExist):
		failure := status.Fail(status.CodeChildNotFound, "%s: no such file or directory", command)

		return &failure
	case statError != nil:
		failure := status.Fail(status.CodeChildNotExecutable, "%s: %v", command, statError)

		return &failure
	case info.IsDir():
		failure := status.Fail(status.CodeChildNotExecutable, "%s: is a directory", command)

		return &failure
	case info.Mode().Perm()&0o111 == 0:
		failure := status.Fail(status.CodeChildNotExecutable, "%s: permission denied", command)

		return &failure
	}

	return nil
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

// childStatus is waitStatusCode plus the note to the boundary that a child,
// not hippo, decided this status.
func childStatus(config RunConfig, err error) int {
	status := waitStatusCode(err)
	if config.ObserveChildStatus != nil {
		config.ObserveChildStatus(status)
	}

	return status
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

func resolveActivationFailure(
	config RunConfig,
	runID string,
	activationError error,
	stopLifetime func() (error, error),
) (int, error) {
	_, stopError := stopLifetime()
	receiptReason := "coordination-activation-failure"
	if errors.Is(activationError, errCoordinationDeferred) {
		receiptReason = "coordination-contention"
	}
	receiptError := writeSafetyReceipt(
		config.EvidenceRoot, runID, "started-activation-failure", receiptReason,
		config.ReservationMetadata, config.TaskClass, config.Now(),
	)
	// The payload already started, so lock contention is not a retryable
	// admission deferral. Owned cleanup completes before a stable failure
	// is returned and the receipt records that launch occurred. HIPPO failed
	// while starting the child, so the caller gets HIPPO's own failure status
	// and reason rather than 1, which only ever reports a result.
	failure := status.Fail(
		status.CodeSupervisionFailed, "task failed after launch: %v",
		errors.Join(activationError, stopError, receiptError),
	)

	return failure.Status(), failure
}

// refusedEvidenceWrite names a write the evidence root refused before any child
// started with its published reason, so the caller fixes the root rather than
// reading a supervision failure. Any other failure keeps its own shape.
func refusedEvidenceWrite(action string, err error) error {
	if err == nil || !evidence.WriteRefused(err) {
		return err
	}

	return status.Fail(status.CodeEvidenceUnwritable, "%s: %v", action, err)
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

// noteDeferralf reports why admission was deferred. The run returns after this
// notice; callers use the recorded receipt to decide whether a later retry is
// safe instead of blindly replaying a 124 naming hippo.limit.capacity-deferred.
func (config RunConfig) noteDeferralf(format string, arguments ...any) {
	_, _ = fmt.Fprintf(config.Stderr, format, arguments...)
}

// Run admits, supervises, and records one child process without touching unrelated processes.
func Run(ctx context.Context, config RunConfig) (exitCode int, returnError error) {
	if err := ctx.Err(); err != nil {
		return 1, err
	}
	if config.Command == "" {
		return 1, errors.New("guarded command is empty")
	}
	if failure := checkCommandIsRunnable(config.Command); failure != nil {
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
		return 1, refusedEvidenceWrite("preparing the evidence root", err)
	}

	var session *Session
	var err error
	if config.ReservationPolicy.Enabled {
		pause := func(ctx context.Context, duration time.Duration) error {
			if config.Sleep == nil {
				return waitForContext(ctx, duration, nil)
			}
			config.Sleep(duration)

			return ctx.Err()
		}
		session, err = AcquireReservationWithOptions(
			ctx,
			config.EvidenceRoot,
			environmentValue(config.Environment, "HIPPO_SESSION"),
			config.TaskClass,
			config.Resolution.ResolvedProfile,
			config.ConfigHash,
			config.ReservationPlan,
			config.ReservationPolicy.MaxActiveOwners,
			config.Policy.LeaseWait,
			ReservationAdmissionOptions{
				Metadata: config.ReservationMetadata,
				Now:      config.Now,
				Pause:    pause,
				Heartbeat: func(status ReservationWaitStatus) {
					_, _ = fmt.Fprintf(
						config.Stderr,
						"HIPPO queued run=%s position=%d remaining=%s.\n",
						status.RunID, status.Position, status.Remaining.Round(time.Second),
					)
				},
			},
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
		if errors.Is(err, ErrCoordinationProtocolMismatch) {
			config.noteDeferralf("HIPPO protocol mismatch: %s.\n", err)

			return policy.ProtocolMismatchExitCode, nil
		}
		if errors.Is(err, ErrReservationDeferred) {
			config.noteDeferralf("HIPPO deferred task: reservation capacity remained exhausted through the bounded wait.\n")

			return CapacityDeferredExitCode, nil
		}
		if errors.Is(err, errCoordinationDeferred) {
			config.noteDeferralf("HIPPO deferred task: %s.\n", err)

			return CapacityDeferredExitCode, nil
		}

		return 1, err
	}
	if session == nil {
		if config.ReservationPolicy.Enabled {
			config.noteDeferralf("HIPPO deferred task: reservation capacity remained exhausted through the bounded wait.\n")
		} else {
			config.noteDeferralf(
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
			config.noteDeferralf("HIPPO deferred task: %s.\n", err)

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

			return childStatus(config, waitError), nil
		case <-ctx.Done():
			waitError, stopError := stopLifetime()
			if stopError != nil {
				return 1, stopError
			}

			return childStatus(config, waitError), nil
		}
	}

	writer, err := NewEvidenceWriter(
		config.EvidenceRoot,
		EvidenceIdentifier("development-"+string(config.TaskClass), config.Now(), os.Getpid()),
		config.EvidenceLimits,
	)
	if err != nil {
		return 1, refusedEvidenceWrite("opening the evidence stream", err)
	}

	writer.SetIdentity(config.ReservationMetadata)
	writer.SetContext(config.Resolution, config.ConfigHash)
	if config.ReservationPolicy.Enabled {
		totals, statusError := ReservationStatus(ctx, config.EvidenceRoot)
		// A peer holding the shared lock here is contention, and no child has
		// started yet. The caller receives exit 124 naming
		// hippo.limit.capacity-deferred with a never-started receipt.
		if errors.Is(statusError, errCoordinationDeferred) {
			config.noteDeferralf("HIPPO deferred task: %s.\n", statusError)

			return CapacityDeferredExitCode, nil
		}
		if statusError != nil {
			return 1, statusError
		}
		writer.SetReservationContext(session, totals.ActiveOwners, "admitted")
	}
	outcome := "capacity-deferred"
	launched, finalized := false, false
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
			if !launched {
				returnError = refusedEvidenceWrite("recording the lifetime summary", finalizeError)
			}
			exitCode = 1
		}
	}()

	deadline := config.Now().Add(config.Policy.AdmissionWindow)
	var previous policy.CPUState
	samples := []policy.Sample{}
	admitted, degraded := false, false
	// A caller that cancels while the host is still being sampled cancels
	// work that never started, exactly as one that cancels a queued waiter
	// does, and leaves the same receipt so it may requeue once.
	cancelledBeforeLaunch := func(cause error) (int, error) {
		receiptError := writeSafetyReceipt(
			config.EvidenceRoot, writer.summary.RunID, "never-started", "admission-cancelled",
			config.ReservationMetadata, config.TaskClass, config.Now(),
		)

		return 1, errors.Join(cause, refusedEvidenceWrite("writing the never-started receipt", receiptError))
	}

	for {
		if err := ctx.Err(); err != nil {
			return cancelledBeforeLaunch(err)
		}

		reading, collectError := config.Collector.Collect(ctx, previous, config.DiskPath)
		if collectError != nil {
			if ctx.Err() != nil {
				return cancelledBeforeLaunch(ctx.Err())
			}

			return 1, collectError
		}

		previous = reading.CPUState
		samples = append(samples, reading.Sample)
		if appendError := writer.Append(reading.Sample); appendError != nil {
			return 1, refusedEvidenceWrite("recording a host sample", appendError)
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
			return cancelledBeforeLaunch(err)
		}
	}

	if !admitted {
		config.noteDeferralf("HIPPO deferred task: safe admission was not reached.\n")
		if receiptError := writeSafetyReceipt(
			config.EvidenceRoot, writer.summary.RunID, "never-started", "host-admission",
			config.ReservationMetadata, config.TaskClass, config.Now(),
		); receiptError != nil {
			return 1, refusedEvidenceWrite("writing the never-started receipt", receiptError)
		}

		return CapacityDeferredExitCode, nil
	}

	lifetime, launchError := launchConfiguredLifetime(ctx, config, session, portLease)
	if launchError != nil {
		if lifetime != nil {
			ownershipRetired = false
		}

		return 1, launchError
	}
	launched = true
	ownershipRetired = false
	stopLifetime := func() (error, error) {
		waitError, stopError := stopConfiguredLifetime(config, lifetime)
		ownershipRetired = !errors.Is(stopError, errChildRetirementUnconfirmed)

		return waitError, stopError
	}
	if config.ReservationPolicy.Enabled {
		if activationError := ActivateReservation(config.EvidenceRoot, session, lifetime.processGroup); activationError != nil { //nolint:contextcheck // Activation owns a bounded atomic coordination transaction.
			outcome = outcomeTaskFailed

			return resolveActivationFailure(config, writer.summary.RunID, activationError, stopLifetime)
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

			return childStatus(config, waitError), nil

		case <-ctx.Done():
			waitError, stopError := stopLifetime()
			outcome = outcomeTaskFailed
			if stopError != nil {
				return 1, stopError
			}

			return childStatus(config, waitError), nil

		case <-ticker.C:
			if config.ReservationPolicy.Enabled { //nolint:nestif // Owner-side marked shedding must remain ahead of fresh pressure collection.
				selected, selectedExit, selectionError := ReservationSheddingSelection(config.EvidenceRoot, session) //nolint:contextcheck // Owner mark observation remains bounded independently of sampling cancellation.
				// A contended shared root defers this observation to the next
				// sample instead of costing the caller a healthy child.
				if selectionError != nil && !errors.Is(selectionError, errCoordinationDeferred) {
					outcome = outcomeSupervisionFailed
					_, stopError := stopLifetime()

					return 1, errors.Join(selectionError, stopError)
				}
				if selectionError == nil && selected {
					outcome = outcomePressureShed
					if config.TaskClass == policy.TaskTransactional {
						outcome = outcomeEmergencyStop
					}
					if selectedExit == StorageBlockedExitCode {
						outcome = outcomeStorageShed
					}
					_, _ = fmt.Fprintln(config.Stderr, "HIPPO shedding this selected child from its owning guard.")
					_, stopError := stopLifetime()
					if outcome == outcomeEmergencyStop {
						stopError = errors.Join(stopError, writeSafetyReceipt(
							config.EvidenceRoot, session.Token, "started-safety-stop", "emergency-pressure",
							config.ReservationMetadata, config.TaskClass, config.Now(),
						))
					}

					return callerShedCode(selectedExit), stopError
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

					return childStatus(config, waitError), nil
				}

				outcome = outcomeSupervisionFailed
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
					shedCode, outcome = StorageBlockedExitCode, outcomeStorageShed
				} else {
					outcome = outcomePressureShed
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
					var victim ReservationOwner
					var selected bool
					var selectionError error
					if emergency {
						victim, selected, selectionError = SelectEmergencyPressureVictim(config.EvidenceRoot, shedCode) //nolint:contextcheck // Selection is one bounded locked evaluation.
					} else {
						victim, selected, selectionError = SelectPressureVictim(config.EvidenceRoot, shedCode) //nolint:contextcheck // Selection is one bounded locked evaluation.
					}
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
					if victim.Class == policy.TaskTransactional {
						outcome = outcomeEmergencyStop
						_, _ = fmt.Fprintln(config.Stderr, "HIPPO emergency safety stop selected transactional work; it will not retry automatically.")
					}
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
				if outcome == outcomeEmergencyStop {
					stopError = errors.Join(stopError, writeSafetyReceipt(
						config.EvidenceRoot, session.Token, "started-safety-stop", "emergency-pressure",
						config.ReservationMetadata, config.TaskClass, config.Now(),
					))
				}

				return callerShedCode(shedCode), stopError
			}
		}
	}
}

// callerShedCode turns the shed cause the ledger records into the status the
// guard returns for a child it stopped. Storage keeps its own status; any
// other pressure is a shed, not the capacity deferral the ledger shares a code
// with.
func callerShedCode(shedCode int) int {
	if shedCode == CapacityDeferredExitCode {
		return PressureShedExitCode
	}

	return shedCode
}

func executableGuardPath() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}

	return path
}
