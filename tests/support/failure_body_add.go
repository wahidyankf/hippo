package support

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/bootstrap"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/tests/contract"
)

// The refusal conditions the failure-body scenario arranges. Each one reaches
// the public run command, and each needs a host or a child that only the
// in-process boundary can stage, which is why the compiled binary is exempt.
const (
	storageFloorRefusal     = "a host whose disk is below the storage floor"
	capacityDeferralRefusal = "a shared root whose capacity never frees for a waiting owner"
	pressureShedRefusal     = "an admitted child that critical host pressure sheds"
	usageMistakeRefusal     = "a mistyped flag on the run command"
)

func (driver *Driver) failureBodyAddBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^(a host whose disk is below the storage floor|a shared root whose capacity never frees for a waiting owner|an admitted child that critical host pressure sheds|a mistyped flag on the run command)$`, driver.stageRefusal),
		step(`^the run is refused and its failure body is read$`, driver.refuseRunAndReadBody),
		step(`^the failure body names (hippo\.[a-z.-]+) with retryable (true|false)$`, driver.requireFailureBodyRetryable),
	}
}

// stageRefusal records which refusal the scenario arranges; the run happens in the When step.
func (driver *Driver) stageRefusal(condition string) error {
	driver.addCLI.refusal = condition

	return nil
}

// refuseRunAndReadBody runs the public run command with JSON output for the
// staged refusal, so the failure body is the caller's view of it.
func (driver *Driver) refuseRunAndReadBody() error {
	if driver.mode == contract.E2E {
		return errors.New("the end-to-end boundary cannot stage a refusal condition; its scenario is exempt")
	}
	switch driver.addCLI.refusal {
	case storageFloorRefusal:
		return driver.refuseOnStorageFloor()
	case capacityDeferralRefusal:
		if err := driver.permanentlyDeferringRoot(); err != nil {
			return err
		}

		return driver.runWaitingForAdmission(time.Second, 3, "--output", "json")
	case pressureShedRefusal:
		return driver.refuseOnPressureShed()
	case usageMistakeRefusal:
		return driver.refuseOnUsageMistake()
	}

	return fmt.Errorf("no refusal condition was staged")
}

// refusalRoot makes a state root and a schema-one configuration for one refusal run.
func (driver *Driver) refusalRoot() (string, string, error) {
	root, err := driver.temporaryRoot()
	if err != nil {
		return "", "", err
	}
	configPath := filepath.Join(root, "hippo.config.json")
	if err = os.WriteFile(configPath, []byte(schemaOneDocument), 0o600); err != nil {
		return "", "", err
	}

	return root, configPath, nil
}

// runRefusal runs the public command boundary in process with a scripted host
// and records its exit status and both streams.
func (driver *Driver) runScriptedRefusal(root string, samples []policy.Sample, arguments ...string) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code, _ := bootstrap.WithDefaults(cli.Application{
		Stdout:      stdout,
		Stderr:      stderr,
		Environment: []string{hippoRootEnvironment + "=" + root, isolatedPath},
		Collector:   &sequenceCollector{samples: samples},
		Sleep:       func(time.Duration) {},
	}).Run(context.Background(), arguments)
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()
}

// refuseOnStorageFloor scripts a disk below the 256 MiB floor for an ephemeral run.
func (driver *Driver) refuseOnStorageFloor() error {
	root, configPath, err := driver.refusalRoot()
	if err != nil {
		return err
	}
	exhausted := capacitySample(policy.GiB, 512*policy.MiB)
	exhausted.DiskFreeBytes = new(200 * policy.MiB)
	exhausted.DiskTotalBytes = new(policy.GiB)
	driver.runScriptedRefusal(root, []policy.Sample{exhausted},
		runCommandName, "--output", "json", configFlag, configPath, diskPathFlag, root,
		taskClassFlag, taskClassEphemeral, "--", shellPath, "-c", "exit 0")

	return nil
}

// refuseOnPressureShed admits a child on four healthy samples and then holds
// critical memory pressure, so the guard sheds the child it started.
func (driver *Driver) refuseOnPressureShed() error {
	root, configPath, err := driver.refusalRoot()
	if err != nil {
		return err
	}
	samples := make([]policy.Sample, 0, 5)
	for range 4 {
		samples = append(samples, healthySample(time.Now()))
	}
	critical := healthySample(time.Now())
	level := 4
	critical.MemoryPressureLevel = &level
	driver.runScriptedRefusal(root, append(samples, critical),
		runCommandName, "--output", "json", configFlag, configPath, diskPathFlag, root,
		"--", shellPath, "-c", "sleep 10")

	return nil
}

// refuseOnUsageMistake gives the run command a flag it does not accept.
func (driver *Driver) refuseOnUsageMistake() error {
	root, configPath, err := driver.refusalRoot()
	if err != nil {
		return err
	}
	driver.runScriptedRefusal(root, []policy.Sample{healthySample(time.Now())},
		runCommandName, "--output", "json", configFlag, configPath, "--no-such-flag")

	return nil
}

// requireFailureBodyRetryable holds the JSON failure body's code and retryable
// field to the values the scenario names.
func (driver *Driver) requireFailureBodyRetryable(code, retryable string) error {
	line, err := driver.failureBodyLine()
	if err != nil {
		return err
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if err = json.Unmarshal([]byte(line), &body); err != nil {
		return err
	}
	want := retryable == "true"
	if body.Error.Code != code || body.Error.Retryable != want {
		return fmt.Errorf("failure body names %q retryable %t, want %s retryable %t: %s",
			body.Error.Code, body.Error.Retryable, code, want, line)
	}

	return nil
}
