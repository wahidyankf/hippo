package support

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/cli"
	"github.com/wahidyankf/hippo/internal/policy"
)

// releaseCheckFailureSamples stages each way a release stability check can
// fail after its admission probe passed. The first sample is the healthy
// probe; the collector then repeats the last sample for every later reading.
func releaseCheckFailureSamples(condition string) ([]policy.Sample, int, error) {
	base := time.Unix(0, 0)
	probe := healthySample(base)
	failing := healthySample(base.Add(time.Second))
	switch condition {
	case "memory pressure never clears":
		failing.MemoryPressureLevel = new(4)
	case "CPU use never settles":
		failing.CPUUtilizationPercent = new(99.0)
	case "free disk is below the release reserve":
		failing.DiskFreeBytes = new(2 * policy.GiB)
	case "host evidence stops after the first sample":
		return []policy.Sample{probe}, 1, nil
	default:
		return nil, 0, fmt.Errorf("unknown release check condition %q", condition)
	}

	return []policy.Sample{probe, failing}, 0, nil
}

func (driver *Driver) releaseCheckHost(condition string) error {
	samples, failureFrom, err := releaseCheckFailureSamples(condition)
	if err != nil {
		return err
	}
	driver.samples = samples
	driver.releaseCheckFailureFrom = failureFrom

	return nil
}

// checkReleaseThroughCLI runs release check in process with the staged
// samples, so the sentence under test is the one the command writes. The
// pause is a no-op: a CPU that never settles would otherwise spend fifteen
// seconds of real time being sampled.
func (driver *Driver) checkReleaseThroughCLI() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	driver.exitCode, _ = (cli.Application{
		Stdout: stdout, Stderr: stderr,
		Environment: []string{"HIPPO_ROOT=" + root, "HOME=" + root, "PATH=/usr/bin:/bin"},
		Collector:   &sequenceCollector{samples: driver.samples, failureFrom: driver.releaseCheckFailureFrom},
		Sleep:       func(time.Duration) {},
	}).Run(context.Background(), []string{releaseCommandName, releaseCheckName, diskPathFlag, root})
	driver.output, driver.errorOutput = stdout.String(), stderr.String()

	return nil
}

func (driver *Driver) requireOneLineReleaseCheckFailure(status, code, reason string) error {
	expected, err := strconv.Atoi(status)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSuffix(driver.errorOutput, "\n"), "\n")
	switch {
	case driver.exitCode != expected:
		return fmt.Errorf("release check exited %d, want %d: stderr=%q", driver.exitCode, expected, driver.errorOutput)
	case len(lines) != 1:
		return fmt.Errorf("release check wrote %d diagnostic lines, want one: stderr=%q", len(lines), driver.errorOutput)
	case !strings.HasPrefix(lines[0], "hippo: ["+code+"]"):
		return fmt.Errorf("release check did not name %s: stderr=%q", code, driver.errorOutput)
	case !strings.Contains(lines[0], reason):
		return fmt.Errorf("release check did not say %q: stderr=%q", reason, driver.errorOutput)
	case driver.output != "":
		return fmt.Errorf("release check wrote to stdout: %q", driver.output)
	}

	return nil
}
