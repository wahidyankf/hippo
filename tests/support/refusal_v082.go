package support

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/internal/cli"
	"github.com/wahidyankf/hippo/internal/host"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
	"github.com/wahidyankf/hippo/tests/contract"
)

const (
	outcomeAdmissionFailedName = "admission-failed"
	refusedRunSource           = "refused-run"
	releaseCheckVerb           = "check"
	releaseCheckAsked          = "release check"
)

func (driver *Driver) refusalBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^host evidence that HIPPO cannot read$`, driver.unreadableHostV082),
		step(`^(status|run|release check) is requested against that host$`, driver.requestAgainstUnreadableHostV082),
		step(
			`^it exits 125 naming hippo\.(host\.unreadable|evidence\.unwritable) and no child starts$`,
			driver.requireRefusedBeforeLaunchV082,
		),
		step(`^an admitted child whose host evidence becomes unreadable while it runs$`, driver.hostLostAfterLaunchV082),
		step(`^the guard samples the host after launch$`, driver.sampleAfterLaunchV082),
		step(`^it exits 125 naming hippo\.supervision\.failed after the child started$`, driver.requireSupervisionLostV082),
		step(`^a state root HIPPO is not permitted to create$`, driver.uncreatableStateRootV082),
		step(`^a guarded run is requested with that state root$`, driver.runWithStateRootV082),
		step(`^a queued run whose receipt directory refuses writes$`, driver.refusedReceiptDirectoryV082),
		step(`^the queued run receives SIGINT before it is admitted$`, driver.signalQueuedRunV082),
		step(`^a guarded run that has begun sampling the host$`, driver.samplingRunV082),
		step(
			`^(its host evidence becomes unreadable|a signal stops it and its never-started receipt is refused|its command stops being executable) before its child starts$`,
			driver.failBeforeLaunchV082,
		),
		step(
			`^it exits 125 naming (hippo\.host\.unreadable|hippo\.evidence\.unwritable|hippo\.supervision\.failed), its child never starts, and its lifetime summary records the outcome admission-failed$`,
			driver.requireAdmissionFailedV082,
		),
	}
}

// failBeforeLaunchV082 stops a run hippo has begun admitting in one of the
// ways that are hippo's own failure rather than the host's capacity. The
// command's own probe is the first collection and the guard's first
// admission sample the second.
func (driver *Driver) failBeforeLaunchV082(failure string) error {
	sample := driver.samples[0]
	payload := filepath.Join(driver.interruption.root, "payload")
	if err := os.WriteFile(payload, []byte("#!/bin/sh\nprintf started > \"$CHILD_MARKER\"\n"), 0o700); err != nil {
		return err
	}
	arguments := []string{runCommandName, sourceFlagName, refusedRunSource, diskPathFlag, driver.interruption.root, "--", payload}
	ctx, interrupt := context.WithCancelCause(context.Background())
	defer interrupt(nil)
	var collector policy.Collector = &sequenceCollector{samples: driver.samples}
	sleep := func(time.Duration) {}
	switch failure {
	case "its host evidence becomes unreadable":
		collector = &failingAfter{sample: sample, healthy: 2}
	case "a signal stops it and its never-started receipt is refused":
		if err := driver.lockDirectory(filepath.Join(driver.interruption.root, "receipts")); err != nil {
			return err
		}
		sleep = func(time.Duration) { interrupt(status.Interruption{Signal: syscall.SIGTERM}) }
	default:
		collector = &launchBreaker{sample: sample, payload: payload}
	}
	driver.runInterruptible(ctx, arguments, collector, sleep)

	return nil
}

// launchBreaker reports a stable host and, at the guard's first admission
// sample, makes the command it is about to launch no longer executable.
type launchBreaker struct {
	sample  policy.Sample
	payload string
	calls   int
}

func (collector *launchBreaker) Collect(_ context.Context, previous policy.CPUState, _ string) (policy.Reading, error) {
	collector.calls++
	if collector.calls == 2 {
		if err := os.Chmod(collector.payload, 0o600); err != nil {
			return policy.Reading{}, err
		}
	}

	return policy.Reading{CPUState: previous, Sample: collector.sample}, nil
}

func (driver *Driver) requireAdmissionFailedV082(reason string) error {
	if driver.exitCode != status.GuardFailed || !strings.Contains(driver.errorOutput, "hippo: ["+reason+"]") {
		return fmt.Errorf("exit %d, want %d naming %s: %q", driver.exitCode, status.GuardFailed, reason, driver.errorOutput)
	}
	if _, err := os.Stat(driver.interruption.childMarker); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("a run hippo stopped before launch started its child: %w", err)
	}
	outcomes, err := driver.interruptedSummaries()
	if err != nil {
		return err
	}
	if len(outcomes) != 1 || outcomes[refusedRunSource] != outcomeAdmissionFailedName {
		return fmt.Errorf("the run summarized as %v, want %s=%s", outcomes, refusedRunSource, outcomeAdmissionFailedName)
	}

	return nil
}

// refusingHost is the real collector with every host file and probe refused,
// the way an unreadable /proc entry or a denied sysctl fails.
func refusingHost() host.SystemCollector {
	refused := func(path string) error { return &fs.PathError{Op: "open", Path: path, Err: syscall.EACCES} }

	return host.SystemCollector{
		ReadFile: func(path string) ([]byte, error) { return nil, refused(path) },
		Run:      func(_ context.Context, name string, _ ...string) ([]byte, error) { return nil, refused(name) },
	}
}

// lockDirectory makes path listable but not writable, and records it so
// cleanup can restore it before removing the scenario's temporary tree.
func (driver *Driver) lockDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o500); err != nil {
		return err
	}
	driver.interruption.locked = append(driver.interruption.locked, path)
	if probe, err := os.CreateTemp(path, "probe"); err == nil {
		_ = probe.Close()

		return errors.New("this user can write a read-only directory, so a refused write cannot be staged")
	}

	return nil
}

func (driver *Driver) unreadableHostV082() error {
	if err := driver.prepareInterruption(runCommandName); err != nil {
		return err
	}
	// Through the compiled binary the host evidence it cannot read is the
	// measured path itself; in process it is every host file and probe.
	driver.interruption.diskPath = driver.interruption.root
	if driver.mode == contract.E2E {
		driver.interruption.diskPath = filepath.Join(driver.interruption.root, "not-a-measured-path")

		return driver.compiledBinary()
	}

	return nil
}

func (driver *Driver) payloadArguments() []string {
	return []string{"--", shellPath, "-c", `printf started > "$CHILD_MARKER"`}
}

func (driver *Driver) requestAgainstUnreadableHostV082(command string) error {
	diskPath := driver.interruption.diskPath
	arguments := map[string][]string{
		statusCommandName: {statusCommandName, diskPathFlag, diskPath},
		runCommandName: append(
			[]string{runCommandName, sourceFlagName, refusedRunSource, diskPathFlag, diskPath}, driver.payloadArguments()...,
		),
		releaseCheckAsked: {releaseCommandName, releaseCheckVerb, diskPathFlag, diskPath},
	}[command]

	return driver.runRefusal(arguments, refusingHost())
}

// runRefusal runs one invocation in process, or through the compiled binary
// with only the scenario's environment, and records what it returned.
func (driver *Driver) runRefusal(arguments []string, collector policy.Collector) error {
	if driver.mode == contract.E2E {
		command := exec.Command(driver.binary, arguments...)
		command.Env = driver.interruptionEnvironment()
		command.Dir = filepath.Dir(driver.interruption.childMarker)
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		command.Stdout, command.Stderr = stdout, stderr
		runError := command.Run()
		driver.exitCode, driver.output, driver.errorOutput = 0, stdout.String(), stderr.String()
		if exitError := (&exec.ExitError{}); errors.As(runError, &exitError) {
			driver.exitCode = exitError.ExitCode()
		} else if runError != nil {
			return runError
		}

		return nil
	}
	driver.runInterruptible(context.Background(), arguments, collector, func(time.Duration) {})

	return nil
}

func (driver *Driver) requireRefusedBeforeLaunchV082(reason string) error {
	if driver.exitCode != status.GuardFailed {
		return fmt.Errorf("exit %d, want %d naming hippo.%s: %q", driver.exitCode, status.GuardFailed, reason, driver.errorOutput)
	}
	if !strings.Contains(driver.errorOutput, "hippo: [hippo."+reason+"]") {
		return fmt.Errorf("the failure does not name hippo.%s: %q", reason, driver.errorOutput)
	}
	if _, err := os.Stat(driver.interruption.childMarker); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("the refused run started its child: %w", err)
	}

	return nil
}

// failingAfter reports a stable host for its first healthy collections, then
// fails the way the system collector reports a host it can no longer read.
type failingAfter struct {
	sample  policy.Sample
	healthy int
	calls   int
}

func (collector *failingAfter) Collect(_ context.Context, previous policy.CPUState, _ string) (policy.Reading, error) {
	collector.calls++
	if collector.calls > collector.healthy {
		return policy.Reading{}, status.Fail(status.CodeHostUnreadable, "collecting host evidence: read Linux memory: permission denied")
	}

	return policy.Reading{CPUState: previous, Sample: collector.sample}, nil
}

func (driver *Driver) hostLostAfterLaunchV082() error {
	return driver.prepareInterruption(runCommandName)
}

func (driver *Driver) sampleAfterLaunchV082() error {
	// One probe and the consecutive admission samples; the host then stops
	// answering while the child runs.
	collector := &failingAfter{sample: driver.samples[0], healthy: policy.DefaultPolicy().ConsecutiveCPUSamples + 1}
	arguments := []string{
		runCommandName, diskPathFlag, driver.interruption.root,
		"--", shellPath, "-c", `printf started > "$CHILD_MARKER"; sleep 30`,
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code, _ := (cli.Application{
		Stdout: stdout, Stderr: stderr, Environment: driver.interruptionEnvironment(),
		Collector: collector, Sleep: func(time.Duration) {},
	}).Run(context.Background(), arguments)
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()

	return nil
}

func (driver *Driver) requireSupervisionLostV082() error {
	if driver.exitCode != status.GuardFailed || !strings.Contains(driver.errorOutput, "hippo: ["+string(status.CodeSupervisionFailed)+"]") {
		return fmt.Errorf("exit %d, want %d naming %s: %q",
			driver.exitCode, status.GuardFailed, status.CodeSupervisionFailed, driver.errorOutput)
	}
	if _, err := os.Stat(driver.interruption.childMarker); err != nil {
		return fmt.Errorf("the child never started, so this is not the after-launch path: %w", err)
	}

	return nil
}

func (driver *Driver) uncreatableStateRootV082() error {
	if err := driver.prepareInterruption(runCommandName); err != nil {
		return err
	}
	parent := filepath.Join(driver.interruption.root, "locked")
	if err := driver.lockDirectory(parent); err != nil {
		return err
	}
	// The child marker and measured path stay outside the refused root, so
	// only the state root itself is refused.
	driver.interruption.diskPath = driver.interruption.root
	driver.interruption.root = filepath.Join(parent, "state")
	if driver.mode == contract.E2E {
		return driver.compiledBinary()
	}

	return nil
}

func (driver *Driver) runWithStateRootV082() error {
	return driver.runRefusal(
		append([]string{
			runCommandName, sourceFlagName, refusedRunSource, diskPathFlag, driver.interruption.diskPath,
		}, driver.payloadArguments()...),
		&sequenceCollector{samples: driver.samples},
	)
}

func (driver *Driver) refusedReceiptDirectoryV082() error {
	if err := driver.filledReservationRootV082(); err != nil {
		return err
	}

	return driver.lockDirectory(filepath.Join(driver.interruption.root, "receipts"))
}
