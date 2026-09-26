package support

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/status"
	"github.com/wahidyankf/hippo/tests/contract"
)

const (
	admissionDeadlineReason = "admission-deadline"
	// heldLockConfig is a schema-2 reservation configuration a run can wait
	// under with its own --wait-for-admission, so a scenario chooses the wait.
	heldLockConfig = `{"schemaVersion":2,"coordination":{"maxCpu":4,"maxMemoryMiB":1024}}`
	// heldLockSignalDelay is how long a run is left blocked on the held lock
	// before it is signalled. Nothing it does before the lock checks for
	// cancellation, and the lock is held throughout, so any delay lands the
	// signal on the admission wait; this one only leaves room to start.
	heldLockSignalDelay = time.Second
)

func (driver *Driver) heldLockBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^a reservation root whose coordination lock another admission holds$`, driver.heldCoordinationLockV082),
		step(`^a guarded run waits (\d+) milliseconds for admission$`, driver.waitThroughHeldLockV082),
		step(`^it exits 124 naming hippo\.limit\.capacity-deferred and no child starts$`, driver.requireHeldLockDeferralV082),
		step(`^its receipt records never-started admission-deadline$`, driver.requireDeadlineReceiptV082),
		step(`^a guarded run waiting for that lock receives SIGINT$`, driver.signalHeldLockWaiterV082),
	}
}

// heldCoordinationLockV082 stages a reservation root, and a checkout whose
// configuration lets a run choose its own wait, whose coordination lock
// another admission holds for the whole scenario.
func (driver *Driver) heldCoordinationLockV082() error {
	if err := driver.prepareInterruption(runCommandName); err != nil {
		return err
	}
	root := driver.interruption.root
	driver.interruption.workingDir = filepath.Join(root, "checkout")
	if err := os.MkdirAll(driver.interruption.workingDir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(
		filepath.Join(driver.interruption.workingDir, interruptionConfigName), []byte(heldLockConfig), 0o600,
	); err != nil {
		return err
	}
	if err := driver.holdCoordinationLock(root); err != nil {
		return err
	}
	if driver.mode == contract.E2E {
		return driver.compiledBinary()
	}

	return nil
}

func (driver *Driver) heldLockRunArguments(wait string) []string {
	return append([]string{
		runCommandName, sourceFlagName, interruptedRunSource, diskPathFlag, driver.interruption.root,
		"--config", filepath.Join(driver.interruption.workingDir, interruptionConfigName),
		"--cwd", driver.interruption.workingDir, waitForAdmissionFlag, wait,
	}, append(reservationArguments(), "--", "/bin/sh", "-c", `printf started > "$CHILD_MARKER"`)...)
}

func (driver *Driver) waitThroughHeldLockV082(milliseconds string) error {
	return driver.runRefusal(
		driver.heldLockRunArguments(milliseconds+"ms"), &sequenceCollector{samples: driver.samples},
	)
}

func (driver *Driver) requireHeldLockDeferralV082() error {
	line := "hippo: [" + string(status.CodeLimitCapacityDeferred) + "]"
	if driver.exitCode != status.LimitShed || !strings.Contains(driver.errorOutput, line) {
		return fmt.Errorf("exit %d, want %d naming %s: %q",
			driver.exitCode, status.LimitShed, status.CodeLimitCapacityDeferred, driver.errorOutput)
	}
	if _, err := os.Stat(driver.interruption.childMarker); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("the deferred run started its child: %w", err)
	}

	return nil
}

func (driver *Driver) requireDeadlineReceiptV082() error {
	directory := filepath.Join(driver.interruption.root, "receipts")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("the deferral wrote no receipt: %w", err)
	}
	for _, entry := range entries {
		data, readError := os.ReadFile(filepath.Join(directory, entry.Name()))
		if readError != nil {
			return readError
		}
		var receipt guard.SafetyReceipt
		if json.Unmarshal(data, &receipt) != nil || receipt.Source != interruptedRunSource {
			continue
		}
		if receipt.State != neverStartedState || receipt.Reason != admissionDeadlineReason {
			return fmt.Errorf("the receipt records %s/%s, want %s/%s",
				receipt.State, receipt.Reason, neverStartedState, admissionDeadlineReason)
		}

		return nil
	}

	return fmt.Errorf("no receipt from %s among %d entries", interruptedRunSource, len(entries))
}

// signalHeldLockWaiterV082 starts a run that waits up to a minute for the held
// lock and signals it while it waits.
func (driver *Driver) signalHeldLockWaiterV082() error {
	arguments := driver.heldLockRunArguments("1m")
	if driver.mode == contract.E2E {
		return driver.signalCompiledAfter(exec.Command(driver.binary, arguments...), syscall.SIGINT, heldLockSignalDelay)
	}
	ctx, interrupt := context.WithCancelCause(context.Background())
	defer interrupt(nil)
	timer := time.AfterFunc(heldLockSignalDelay, func() { interrupt(status.Interruption{Signal: syscall.SIGINT}) })
	defer timer.Stop()
	driver.runInterruptible(ctx, arguments, &sequenceCollector{samples: driver.samples}, func(time.Duration) {})

	return nil
}

// signalCompiledAfter starts a compiled HIPPO, signals it after delay, and
// records the status a shell would see. A run that ends before the signal is
// reported with what it printed, since the scenario never reached its wait.
func (driver *Driver) signalCompiledAfter(command *exec.Cmd, signal syscall.Signal, delay time.Duration) error {
	command.Env = driver.interruptionEnvironment()
	command.Dir = driver.interruption.root
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command.Stdout, command.Stderr = stdout, stderr
	if err := command.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()
	var waitError error
	select {
	case waitError = <-exited:
		return fmt.Errorf("the run ended before it was signalled: %w: %q", waitError, stderr.String())
	case <-time.After(delay):
	}
	if err := command.Process.Signal(signal); err != nil {
		return err
	}
	select {
	case waitError = <-exited:
	case <-time.After(interruptionTimeLimit):
		_ = command.Process.Kill()

		return errors.New("the signalled run did not exit")
	}
	driver.exitCode, driver.output, driver.errorOutput = 0, stdout.String(), stderr.String()
	if exitError := (&exec.ExitError{}); errors.As(waitError, &exitError) {
		driver.exitCode = exitError.ExitCode()
	} else if waitError != nil {
		return waitError
	}

	return nil
}
