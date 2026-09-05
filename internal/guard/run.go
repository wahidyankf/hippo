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
	EvidenceLimits                        evidence.Limits
	ConfigHash                            string
	Now                                   func() time.Time
	Sleep                                 func(time.Duration)
	ChildStdin                            io.Reader
	ChildStdout, ChildStderr              io.Writer
	Stderr                                io.Writer
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
	switch name {
	case "HIPPO_BIN", "HIPPO_CONCURRENCY", "HIPPO_PROFILE", "HIPPO_SESSION":
		return true
	default:
		return false
	}
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

func signalGroup(process *os.Process, signal syscall.Signal) error {
	if process == nil {
		return nil
	}

	groupError := syscall.Kill(-process.Pid, signal)
	if groupError == nil || errors.Is(groupError, syscall.ESRCH) {
		return nil
	}

	processError := process.Signal(signal)
	if errors.Is(processError, os.ErrProcessDone) {
		return nil
	}

	return errors.Join(groupError, processError)
}

func directChild(ctx context.Context, config RunConfig, environment []string) int {
	command := exec.CommandContext(ctx, config.Command, config.Arguments...)
	command.Dir = config.WorkingDirectory
	command.Env = environment
	command.Stdin, command.Stdout, command.Stderr = config.ChildStdin, config.ChildStdout, config.ChildStderr
	return waitStatusCode(command.Run())
}

func terminateAndWait(command *exec.Cmd, exited <-chan error, grace time.Duration) (error, error) {
	select {
	case waitError := <-exited:
		return waitError, nil
	default:
	}

	signalError := signalGroup(command.Process, syscall.SIGTERM)
	timer := time.NewTimer(grace)
	defer timer.Stop()

	select {
	case waitError := <-exited:
		return waitError, signalError
	case <-timer.C:
		killError := signalGroup(command.Process, syscall.SIGKILL)
		waitError := <-exited

		return waitError, errors.Join(signalError, killError)
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
		return 1, concurrencyError
	}
	config.ConcurrencyEnvironment = concurrencyEnvironment
	config.Environment = resolvedEnvironment(config.Environment, config.Resolution, false, config.ConcurrencyEnvironment)

	if config.DiskPath == "" {
		config.DiskPath = config.WorkingDirectory
	}
	if config.DiskPath == "" {
		config.DiskPath = "."
	}

	if err := evidence.Cleanup(config.EvidenceRoot, config.Now()); err != nil {
		return 1, err
	}

	session, err := AcquireSession(
		ctx,
		config.EvidenceRoot,
		environmentValue(config.Environment, "HIPPO_SESSION"),
		config.TaskClass,
		config.Policy.LeaseWait,
	)
	if err != nil {
		if errors.Is(err, errCoordinationDeferred) {
			_, _ = fmt.Fprintf(config.Stderr, "HIPPO deferred task: %s.\n", err)

			return CapacityDeferredExitCode, nil
		}

		return 1, err
	}
	if session == nil {
		_, _ = fmt.Fprintf(
			config.Stderr,
			"HIPPO deferred task: %s; it must exit before this work is admitted.\n",
			DescribeHeavyLease(config.EvidenceRoot),
		)

		return CapacityDeferredExitCode, nil
	}

	defer func() {
		if releaseError := ReleaseSession(config.EvidenceRoot, session); returnError == nil && releaseError != nil {
			returnError = releaseError
			exitCode = 1
		}
	}()

	var portLease *PortLease
	if config.LeasePort != 0 {
		root := config.PortLeaseRoot
		if root == "" {
			root = filepath.Join(os.TempDir(), "hippo-port-leases")
		}

		portLease, err = AcquirePortLease(root, config.LeasePort, config.LeaseOwner, config.LeaseMinimum, config.LeaseMaximum)
		if err != nil {
			return 1, err
		}

		defer func() {
			if releaseError := ReleasePortLease(root, portLease); returnError == nil && releaseError != nil {
				returnError = releaseError
				exitCode = 1
			}
		}()
	}

	if session.Inherited {
		return directChild(ctx, config, config.Environment), nil
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
	outcome := "capacity-deferred"
	finalized := false
	finalize := func() error {
		if finalized {
			return nil
		}
		finalized = true
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
			config.Resolution.Concurrency = 1
			config.Environment = resolvedEnvironment(config.Environment, config.Resolution, true, config.ConcurrencyEnvironment)
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

	environment := withEnvironment(config.Environment, "HIPPO_SESSION", session.Token)
	executable, lookupError := exec.LookPath(config.Command)
	if lookupError != nil {
		return 1, lookupError
	}

	guardPath := executableGuardPath()
	environment = withEnvironment(environment, "HIPPO_BIN", guardPath)
	command := exec.Command(executable, config.Arguments...)
	command.Dir = config.WorkingDirectory
	command.Env = environment
	command.Stdin, command.Stdout, command.Stderr = config.ChildStdin, config.ChildStdout, config.ChildStderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if startError := command.Start(); startError != nil {
		return 1, startError
	}

	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()

	ticker := time.NewTicker(config.Policy.SampleInterval)
	defer ticker.Stop()
	var warningSince *time.Time

	for {
		select {
		case waitError := <-exited:
			if waitError == nil {
				outcome = "passed"
			} else {
				outcome = outcomeTaskFailed
			}

			return waitStatusCode(waitError), nil

		case <-ctx.Done():
			waitError, stopError := terminateAndWait(command, exited, config.Policy.TerminationGrace)
			outcome = outcomeTaskFailed
			if stopError != nil {
				return 1, stopError
			}

			return waitStatusCode(waitError), nil

		case <-ticker.C:
			reading, collectError := config.Collector.Collect(ctx, previous, config.DiskPath)
			if collectError != nil {
				if ctx.Err() != nil {
					waitError, stopError := terminateAndWait(command, exited, config.Policy.TerminationGrace)
					outcome = outcomeTaskFailed
					if stopError != nil {
						return 1, stopError
					}

					return waitStatusCode(waitError), nil
				}

				outcome = "supervision-failed"
				_, stopError := terminateAndWait(command, exited, config.Policy.TerminationGrace)

				return 1, errors.Join(collectError, stopError)
			}

			previous = reading.CPUState
			samples = append(samples, reading.Sample)
			limit := int(config.Policy.TrendWindow/config.Policy.SampleInterval) + 2
			if len(samples) > limit {
				samples = samples[len(samples)-limit:]
			}

			if appendError := writer.Append(reading.Sample); appendError != nil {
				outcome = "supervision-failed"
				_, stopError := terminateAndWait(command, exited, config.Policy.TerminationGrace)

				return 1, errors.Join(appendError, stopError)
			}

			assessment := policy.ResourceAssessment(samples, config.Policy)
			stableDegradedWarning := degraded && policy.WarningAdmissionReady(samples, config.Policy)
			if assessment.State == policy.StateNormal || stableDegradedWarning {
				warningSince = nil
			} else if assessment.State == policy.StateWarning && warningSince == nil {
				value := config.Now()
				warningSince = &value
			}

			if config.TaskClass == policy.TaskTransactional {
				continue
			}

			grace := config.Policy.EphemeralWarningGrace
			if config.TaskClass == policy.TaskService {
				grace = config.Policy.ServiceWarningGrace
			}

			if assessment.State == policy.StateCritical || (warningSince != nil && config.Now().Sub(*warningSince) >= grace) {
				shedCode := CapacityDeferredExitCode
				if assessment.StorageBlocked {
					shedCode, outcome = StorageBlockedExitCode, "storage-shed"
				} else {
					outcome = "pressure-shed"
				}

				_, _ = fmt.Fprintf(config.Stderr, "HIPPO shedding %s child after %s.\n", config.TaskClass, assessment.Reason)
				_, stopError := terminateAndWait(command, exited, config.Policy.TerminationGrace)

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
