package runtime

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/internal/application"

	"github.com/wahidyankf/hippo/internal/adapters/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
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
	ObserveChildStatus func(int)
	// RetirementConfirmation, when positive, replaces the default window a
	// forced stop waits for the killed process group to retire. A caller that
	// knows its host can stall longer than the default sets it, so the stop is
	// judged by the group's real exit. Zero keeps the default.
	RetirementConfirmation   time.Duration
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
	return application.EnvironmentValue(environment, name)
}

func withEnvironment(environment []string, name, value string) []string {
	return application.WithEnvironment(environment, name, value)
}

func withEnvironmentIfMissing(environment []string, name, value string) []string {
	return application.WithEnvironmentIfMissing(environment, name, value)
}

// ReservationEnvironment delegates portable allocation-derived environment policy.
func ReservationEnvironment(environment []string, resolution policy.Resolution, allocation ReservationVector, requested []string) ([]string, error) {
	return application.ReservationEnvironment(environment, resolution, allocation, requested)
}

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

var errChildRetirementUnconfirmed = application.ErrChildRetirementUnconfirmed

// childRetirementConfirmation bounds the wait for a killed process group to
// retire. It is deliberately not the caller's termination grace: that grace is
// a shutdown policy for a cooperating child, while this window measures how
// long the operating system needs to kill, orphan-reparent, and reap a group.
// Deriving one from the other lets an aggressive grace report a healthy forced
// stop as unconfirmed, which keeps the reservation owned and leaks capacity
// from a coordination root every repository shares.
const childRetirementConfirmation = 2 * time.Second

func terminateAndWait(lifetime *supervisedLifetime, grace, confirmation time.Duration) (error, error) {
	if confirmation <= 0 {
		confirmation = childRetirementConfirmation
	}

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
		postKill := time.NewTimer(max(grace, confirmation))
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

	return terminateAndWait(lifetime, config.Policy.TerminationGrace, config.RetirementConfirmation)
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

func executableGuardPath() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}

	return path
}

// serialWriter is one of several writers that share a single lock.
type serialWriter struct {
	lock   *sync.Mutex
	writer io.Writer
}

func (writer serialWriter) Write(data []byte) (int, error) {
	writer.lock.Lock()
	defer writer.lock.Unlock()

	return writer.writer.Write(data)
}

// serializeWriters puts every output that is not a file behind one lock. A
// child writes a file directly, but reaches any other writer through a copying
// goroutine, which would otherwise write the same writer hippo's own lines go
// to at the same moment.
func serializeWriters(config *RunConfig) {
	lock := &sync.Mutex{}
	for _, target := range []*io.Writer{&config.Stderr, &config.ChildStdout, &config.ChildStderr} {
		if _, file := (*target).(*os.File); !file {
			*target = serialWriter{lock: lock, writer: *target}
		}
	}
}
