package support

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/internal/conformance"
	"github.com/wahidyankf/hippo/internal/status"
	"github.com/wahidyankf/hippo/tests/contract"
)

// conformanceGateReport is what the harness says about a gate a signal
// stopped; it must survive the signal.
const conformanceGateReport = `consumer "consumer-1" gate`

func (driver *Driver) conformanceSignalBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^a conformance run whose first consumer gate is still running$`, driver.blockedConformanceRunV082),
		step(`^hippo-conformance receives (SIGINT|SIGTERM)$`, driver.signalConformanceRunV082),
		step(`^hippo-conformance exits (130|143) and still reports the stopped gate$`, driver.requireConformanceSignalStatusV082),
	}
}

func (driver *Driver) blockedConformanceRunV082() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	manifest, _, err := conformanceFixtureV04(root)
	if err != nil {
		return err
	}
	driver.interruption.root = root
	driver.interruption.childMarker = filepath.Join(root, "gate-started")
	manifest.Consumers[0].Gates = []conformance.Command{{Arguments: []string{
		shellPath, "-c", `printf started > "$1"; exec sleep 30`, conformanceLabel, driver.interruption.childMarker,
	}}}
	driver.interruption.command = filepath.Join(root, "manifest.json")

	return writeConformanceManifestV04(driver.interruption.command, manifest)
}

// waitForGate reports whether the blocked gate started within the limit.
func (driver *Driver) waitForGate() bool {
	deadline := time.Now().Add(interruptionTimeLimit)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(driver.interruption.childMarker); err == nil {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}

	return false
}

// signalConformanceRunV082 stops the run once its gate is running. Through the
// compiled command the signal is a real one; in process it is the
// cancellation the command's entry makes of one.
func (driver *Driver) signalConformanceRunV082(name string) error {
	if driver.mode == contract.E2E {
		return driver.signalCompiledConformance(signalNamed(name))
	}
	ctx, interrupt := context.WithCancelCause(context.Background())
	defer interrupt(nil)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	done := make(chan int, 1)
	go func() { done <- conformance.Main(ctx, []string{driver.interruption.command}, stdout, stderr) }()
	if !driver.waitForGate() {
		interrupt(nil)
		<-done

		return fmt.Errorf("the conformance gate never started: %q", stderr.String())
	}
	interrupt(status.Interruption{Signal: signalNamed(name)})
	driver.exitCode = <-done
	driver.output, driver.errorOutput = stdout.String(), stderr.String()

	return nil
}

func (driver *Driver) signalCompiledConformance(signal syscall.Signal) error {
	moduleRoot, err := moduleRootV04()
	if err != nil {
		return err
	}
	binary := filepath.Join(driver.interruption.root, "hippo-conformance")
	build := exec.Command("go", "build", "-o", binary, "./cmd/hippo-conformance")
	build.Dir = moduleRoot
	if output, buildError := build.CombinedOutput(); buildError != nil {
		return fmt.Errorf("build hippo-conformance: %s: %w", output, buildError)
	}
	command := exec.Command(binary, driver.interruption.command)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command.Stdout, command.Stderr = stdout, stderr
	if err = command.Start(); err != nil {
		return err
	}
	if !driver.waitForGate() {
		_ = command.Process.Kill()
		_ = command.Wait()

		return fmt.Errorf("the conformance gate never started: %q", stderr.String())
	}
	if err = command.Process.Signal(signal); err != nil {
		return err
	}
	waitError := command.Wait()
	driver.exitCode = 0
	if exitError := (&exec.ExitError{}); errors.As(waitError, &exitError) {
		driver.exitCode = exitError.ExitCode()
		if waitStatus, ok := exitError.Sys().(syscall.WaitStatus); ok && waitStatus.Signaled() {
			driver.exitCode = 128 + int(waitStatus.Signal())
		}
	} else if waitError != nil {
		return waitError
	}
	driver.output, driver.errorOutput = stdout.String(), stderr.String()

	return nil
}

func (driver *Driver) requireConformanceSignalStatusV082(want string) error {
	expected, err := strconv.Atoi(want)
	if err != nil {
		return err
	}
	if driver.exitCode != expected {
		return fmt.Errorf("the stopped conformance run exited %d, want %d: %q", driver.exitCode, expected, driver.errorOutput)
	}
	if !strings.Contains(driver.errorOutput, conformanceGateReport) {
		return fmt.Errorf("the stopped conformance run hid what it found: %q", driver.errorOutput)
	}

	return nil
}
