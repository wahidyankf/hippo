package support

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/bootstrap"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/tests/contract"
)

// The two stopped profiles the status scenario arranges. Each one is stopped by
// samples that only the in-process boundary can stage, which is why the
// compiled binary is exempt.
const (
	storageStatusHost  = "a status host with its disk below the storage floor"
	pressureStatusHost = "a status host whose capacity is deferred by critical pressure"
)

func (driver *Driver) statusExitAddBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^(a status host with its disk below the storage floor|a status host whose capacity is deferred by critical pressure)$`, driver.stageStatusHost),
		step(`^JSON status is requested for that host$`, driver.requestStatusForStagedHost),
		step(`^the profile publishes exitCode (73|75) as an integer$`, driver.requireProfileExitCodeInteger),
	}
}

// stageStatusHost records which stopped profile the scenario arranges; the status run happens in the When step.
func (driver *Driver) stageStatusHost(condition string) error {
	driver.addCLI.statusHost = condition

	return nil
}

// requestStatusForStagedHost runs status with JSON output on two samples that
// stop the profile for the staged condition, in an isolated state root.
func (driver *Driver) requestStatusForStagedHost() error {
	if driver.mode == contract.E2E {
		return errors.New("the end-to-end boundary cannot stage a stopped profile; its scenario is exempt")
	}
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	var first, second policy.Sample
	switch driver.addCLI.statusHost {
	case storageStatusHost:
		exhausted := capacitySample(policy.GiB, 512*policy.MiB)
		exhausted.DiskFreeBytes = new(200 * policy.MiB)
		exhausted.DiskTotalBytes = new(policy.GiB)
		first, second = exhausted, exhausted
	case pressureStatusHost:
		critical := healthySample(time.Unix(0, 0))
		level := 4
		critical.MemoryPressureLevel = &level
		first, second = critical, critical
	default:
		return errors.New("no stopped profile was staged")
	}

	var stdout, stderr bytes.Buffer
	code, runError := bootstrap.WithDefaults(cli.Application{
		Stdout:      &stdout,
		Stderr:      &stderr,
		Environment: []string{hippoRootEnvironment + "=" + root, isolatedPath},
		Collector:   &sequenceCollector{samples: []policy.Sample{first, second}},
		Sleep:       func(time.Duration) {},
	}).Run(context.Background(), []string{statusCommandName, jsonFlag, diskPathFlag, root})
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()

	return runError
}

// requireProfileExitCodeInteger holds the status payload's profile.exitCode to the
// bare integer the scenario names, so a reason is never published as a word.
func (driver *Driver) requireProfileExitCodeInteger(code string) error {
	var payload struct {
		Profile struct {
			ExitCode json.RawMessage `json:"exitCode"`
		} `json:"profile"`
	}
	if err := json.Unmarshal([]byte(driver.output), &payload); err != nil {
		return fmt.Errorf("status output is not JSON: %w: %q", err, driver.output)
	}
	if string(payload.Profile.ExitCode) != code {
		return fmt.Errorf("profile exitCode is %s, want the integer %s: %q", payload.Profile.ExitCode, code, driver.output)
	}

	return nil
}
