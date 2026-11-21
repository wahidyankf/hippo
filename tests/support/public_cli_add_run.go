package support

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/bootstrap"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
	"github.com/wahidyankf/hippo/tests/contract"
)

// childWorkingDirectoryScript writes the child's physical working directory into
// the marker the scenario names, so the scenario can compare it with --cwd.
const childWorkingDirectoryScript = `pwd -P > "$CHILD_MARKER"`

// unmeasurableDiskName is a path under the scenario root that is never created.
const unmeasurableDiskName = "unmeasurable-disk"

// unmeasurableDiskCollector stands for a host whose free space cannot be read at
// one path, and reads every other path as healthy. A run that ignored the path
// it was given would read the healthy path and start its child.
type unmeasurableDiskCollector struct {
	unreadable string
}

func (collector unmeasurableDiskCollector) Collect(_ context.Context, previous policy.CPUState, diskPath string) (policy.Reading, error) {
	if diskPath == collector.unreadable {
		return policy.Reading{}, status.Fail(status.CodeHostUnreadable, "collecting host evidence: free space at the disk path cannot be read")
	}

	return policy.Reading{CPUState: previous, Sample: healthySample(time.Now())}, nil
}

func (driver *Driver) publicCLIAddRunBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^run starts a child in the directory named by --cwd$`, driver.runChildInCwd),
		step(`^the child reports the directory named by --cwd$`, driver.requireChildInCwd),
		step(`^run is requested with a disk path that cannot be measured$`, driver.runWithUnmeasurableDisk),
		step(`^it exits 125 naming hippo\.host\.unreadable and the child never starts$`, driver.requireUnmeasurableDiskRefused),
	}
}

// runGuardedChild runs one guarded command: in process at the unit and
// integration boundaries, through the compiled binary at the end-to-end one.
// The child may write to the marker, which CHILD_MARKER names.
func (driver *Driver) runGuardedCommand(root, marker string, arguments ...string) error {
	values := map[string]string{hippoRootEnvironment: root, "CHILD_MARKER": marker}
	if driver.mode == contract.E2E {
		return driver.runBinaryWith(values, arguments...)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code, _ := bootstrap.WithDefaults(cli.Application{
		Stdout: stdout, Stderr: stderr, Environment: append(environmentPairs(values), isolatedPath),
		Collector: &sequenceCollector{samples: []policy.Sample{healthySample(time.Now()), healthySample(time.Now())}},
		Sleep:     func(time.Duration) {},
	}).Run(context.Background(), arguments)
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()

	return nil
}

// runChildInCwd guards a child that records its working directory, with --cwd
// naming a directory of the scenario's own.
func (driver *Driver) runChildInCwd() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	driver.leaseRoot = root
	work := filepath.Join(root, "work")
	if err = os.Mkdir(work, 0o700); err != nil {
		return err
	}
	marker := filepath.Join(root, "child-directory")
	driver.addCLI.cwdDirectory, driver.addCLI.childMarker = work, marker

	return driver.runGuardedCommand(root, marker,
		runCommandName, sourceFlagName, hippoFixtureName, diskPathFlag, root, "--cwd", work,
		"--", shellPath, "-c", childWorkingDirectoryScript)
}

func (driver *Driver) requireChildInCwd() error {
	if driver.exitCode != 0 {
		return fmt.Errorf("run exit=%d stderr=%q", driver.exitCode, driver.errorOutput)
	}
	recorded, err := os.ReadFile(driver.addCLI.childMarker)
	if err != nil {
		return fmt.Errorf("the child recorded no directory: %w", err)
	}
	want, err := filepath.EvalSymlinks(driver.addCLI.cwdDirectory)
	if err != nil {
		return err
	}
	if got := strings.TrimSpace(string(recorded)); got != want {
		return fmt.Errorf("child ran in %q, want the --cwd directory %q", got, want)
	}

	return nil
}

// runWithUnmeasurableDisk guards a child with a --disk-path that does not
// exist. End to end, the compiled binary measures the real host at that path.
// Integration also measures the live host, in process. Unit reads the path
// through the collector double above.
func (driver *Driver) runWithUnmeasurableDisk() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	driver.leaseRoot = root
	unreadable := filepath.Join(root, unmeasurableDiskName)
	marker := filepath.Join(root, "child-started")
	driver.addCLI.childMarker = marker
	arguments := []string{runCommandName, sourceFlagName, hippoFixtureName, diskPathFlag, unreadable, "--", shellPath, "-c", childStartedScript}
	if driver.mode == contract.E2E {
		return driver.runGuardedCommand(root, marker, arguments...)
	}
	var collector policy.Collector
	if driver.mode == contract.Unit {
		collector = unmeasurableDiskCollector{unreadable: unreadable}
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code, _ := bootstrap.WithDefaults(cli.Application{
		Stdout: stdout, Stderr: stderr,
		Environment: []string{hippoRootEnvironment + "=" + root, "CHILD_MARKER=" + marker, isolatedPath},
		Collector:   collector,
		Sleep:       func(time.Duration) {},
	}).Run(context.Background(), arguments)
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()

	return nil
}

func (driver *Driver) requireUnmeasurableDiskRefused() error {
	line := "hippo: [" + string(status.CodeHostUnreadable) + "]"
	if driver.exitCode != status.GuardFailed || !strings.Contains(driver.errorOutput, line) {
		return fmt.Errorf("exit=%d stderr=%q, want exit 125 naming %s", driver.exitCode, driver.errorOutput, status.CodeHostUnreadable)
	}
	if _, err := os.Stat(driver.addCLI.childMarker); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("the child started before the disk path was measured: %w", err)
	}

	return nil
}
