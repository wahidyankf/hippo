package support

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	evidenceadapter "github.com/wahidyankf/hippo/internal/adapters/evidence"
	runtimeadapter "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/bootstrap"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
	"github.com/wahidyankf/hippo/tests/contract"
	"github.com/wahidyankf/hippo/tests/support/runtimewiring"
)

// observationCompatibilityBindings covers mixed display-annotation versions
// and the stable JSON projection through the public status and watch commands.
// Display annotations ride the observation snapshot but never reach the stable
// projection, so the JSON responses stay comparable while labels change.
func (driver *Driver) observationCompatibilityBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^old and current owners sharing unchanged coordination schemas$`, driver.prepareMixedVersionOwners),
		step(`^current observation reads the owners and their queued work$`, driver.readMixedVersionOwners),
		step(`^owners without display annotations remain visible with unavailable labels$`, driver.requireMixedVersionLabels),
		step(`^admission accounting and FIFO ordering remain unchanged$`, driver.requireMixedVersionAccounting),
		step(`^stable observation evidence whose display annotation changes between requests$`, driver.prepareCompatibilityEvidence),
		step(`^JSON status and watch inspect that evidence$`, driver.inspectCompatibilityEvidence),
		step(`^status retains schema version 5 without display annotation fields$`, driver.requireCompatibilityStatus),
		step(`^watch emits only the first unchanged schema five snapshot$`, driver.requireCompatibilityWatch),
	}
}

func mixedVersionPlan() coordination.ReservationPlan {
	return coordination.ReservationPlan{
		Capacity:  coordination.ReservationVector{CPU: 2, MemoryBytes: 2 * policy.GiB},
		Requested: coordination.ReservationVector{CPU: 1, MemoryBytes: policy.GiB},
		Allocated: coordination.ReservationVector{CPU: 1, MemoryBytes: policy.GiB},
	}
}

func (driver *Driver) prepareMixedVersionOwners() error {
	if err := driver.emptyCoordinationRoot(); err != nil {
		return err
	}
	plan := mixedVersionPlan()
	ctx, cancel := context.WithTimeout(context.Background(), usageAttemptBound)
	defer cancel()
	oldOwner, err := runtimewiring.AcquireReservation(ctx, driver.leaseRoot, "", policy.TaskEphemeral, "balanced", "old-hash", plan, 3, 0)
	if err != nil {
		return err
	}
	currentOwner, err := runtimewiring.AcquireReservation(ctx, driver.leaseRoot, "", policy.TaskEphemeral, "balanced", "current-hash", plan, 3, 0)
	if err != nil {
		return err
	}
	driver.ownedMixedSessions = []*runtimeadapter.Session{oldOwner, currentOwner}
	label, err := identity.ParseDisplayLabel("current-owner")
	if err != nil {
		return err
	}
	return evidenceadapter.WriteDisplay(driver.leaseRoot, currentOwner.Token, label)
}

func (driver *Driver) readMixedVersionOwners() error {
	return driver.readCompatibilityStatus()
}

func (driver *Driver) requireMixedVersionLabels() error {
	owners, _, err := driver.compatibilityOwners()
	if err != nil {
		return err
	}
	if len(owners) != 2 {
		return fmt.Errorf("mixed owners visible = %d, want two", len(owners))
	}
	return driver.requireNoAnnotationFields()
}

func (driver *Driver) requireMixedVersionAccounting() error {
	owners, totals, err := driver.compatibilityOwners()
	if err != nil {
		return err
	}
	if len(owners) != 2 || owners[0]["runId"] == owners[1]["runId"] {
		return fmt.Errorf("mixed owners lost distinct FIFO order: %v", owners)
	}
	for _, key := range []string{allocatedField, "capacity"} {
		if totals[key] == nil {
			return fmt.Errorf("mixed accounting lost %s totals", key)
		}
	}
	return nil
}

func (driver *Driver) prepareCompatibilityEvidence() error {
	if err := driver.emptyCoordinationRoot(); err != nil {
		return err
	}
	plan := mixedVersionPlan()
	ctx, cancel := context.WithTimeout(context.Background(), usageAttemptBound)
	defer cancel()
	owner, err := runtimewiring.AcquireReservation(ctx, driver.leaseRoot, "", policy.TaskEphemeral, "balanced", "stable-hash", plan, 3, 0)
	if err != nil {
		return err
	}
	driver.ownedMixedSessions = []*runtimeadapter.Session{owner}
	label, err := identity.ParseDisplayLabel("first-owner")
	if err != nil {
		return err
	}
	return evidenceadapter.WriteDisplay(driver.leaseRoot, owner.Token, label)
}

func (driver *Driver) inspectCompatibilityEvidence() error {
	if err := driver.readCompatibilityStatus(); err != nil {
		return err
	}
	driver.compatibilityBodies = append(driver.compatibilityBodies, driver.output)
	label, err := identity.ParseDisplayLabel("changed-1")
	if err != nil {
		return err
	}
	if len(driver.ownedMixedSessions) != 1 {
		return errors.New("compatibility evidence lost its owner")
	}
	if err := evidenceadapter.WriteDisplay(driver.leaseRoot, driver.ownedMixedSessions[0].Token, label); err != nil {
		return err
	}
	if err := driver.readCompatibilityStatus(); err != nil {
		return err
	}
	driver.compatibilityBodies = append(driver.compatibilityBodies, driver.output)
	return driver.readCompatibilityWatch()
}

func stableCoordination(body string) (string, error) {
	var document map[string]any
	if err := json.Unmarshal([]byte(body), &document); err != nil {
		return "", err
	}
	if document["schemaVersion"] != float64(5) {
		return "", errors.New("status schema changed")
	}
	for _, private := range []string{"first-owner", "changed-", "current-owner", "displayAnnotation", "availability", "label"} {
		if strings.Contains(body, private) {
			return "", fmt.Errorf("status leaked annotation field %q", private)
		}
	}
	stable, err := json.Marshal(document["coordination"])
	if err != nil {
		return "", err
	}
	return string(stable), nil
}

func (driver *Driver) requireCompatibilityStatus() error {
	if len(driver.compatibilityBodies) != 2 {
		return errors.New("compatibility evidence lacks two status inspections")
	}
	var stable string
	for _, body := range driver.compatibilityBodies {
		projection, err := stableCoordination(body)
		if err != nil {
			return err
		}
		if stable == "" {
			stable = projection
		} else if projection != stable {
			return errors.New("changed annotation altered the stable status comparison")
		}
	}
	return nil
}

func (driver *Driver) requireCompatibilityWatch() error {
	return driver.requireCompatibilityWatchBodies()
}

func (driver *Driver) requireCompatibilityWatchBodies() error {
	if len(driver.compatibilityWatch) == 0 {
		return errors.New("watch emitted no snapshot")
	}
	stable, err := stableCoordination(driver.compatibilityBodies[0])
	if err != nil {
		return err
	}
	for _, body := range driver.compatibilityWatch {
		projection, err := stableCoordination(body)
		if err != nil {
			return err
		}
		if projection != stable {
			return errors.New("watch emitted a snapshot beyond the first unchanged one")
		}
	}
	return nil
}

func (driver *Driver) readCompatibilityStatus() error {
	arguments := []string{statusCommandName, jsonFlag, diskPathFlag, "."}
	root := driver.leaseRoot
	environment := []string{"HIPPO_ROOT=" + root, "HOME=" + root, isolatedPath}
	ctx, cancel := context.WithTimeout(context.Background(), usageAttemptBound)
	defer cancel()
	if driver.mode == contract.E2E {
		previous, present := os.LookupEnv(hippoRootEnvironment)
		_ = os.Setenv(hippoRootEnvironment, root)
		defer func() {
			if present {
				_ = os.Setenv(hippoRootEnvironment, previous)
			} else {
				_ = os.Unsetenv(hippoRootEnvironment)
			}
		}()
		return driver.runCompatibilityCLI(ctx, arguments, environment)
	}
	return driver.runCompatibilityInProcess(ctx, root, environment, arguments)
}

func (driver *Driver) runCompatibilityCLI(ctx context.Context, arguments, environment []string) error {
	command := exec.CommandContext(ctx, driver.binary, arguments...)
	command.Env = environment
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command.Stdout, command.Stderr = stdout, stderr
	runError := command.Run()
	driver.output, driver.errorOutput = stdout.String(), stderr.String()
	exitError := &exec.ExitError{}
	switch {
	case runError == nil:
	case errors.As(runError, &exitError):
		driver.exitCode = exitError.ExitCode()
	default:
		return runError
	}
	if driver.exitCode != status.CallerError && driver.exitCode != 0 {
		return fmt.Errorf("status exited %d: %q", driver.exitCode, driver.errorOutput)
	}
	driver.exitCode = 0
	return nil
}

func (driver *Driver) runCompatibilityInProcess(ctx context.Context, root string, environment, arguments []string) error {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return err
	}
	if err = os.Chdir(root); err != nil {
		return err
	}
	defer func() { _ = os.Chdir(workingDirectory) }()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code, err := bootstrap.WithDefaults(cli.Application{
		Stdout: stdout, Stderr: stderr, Environment: environment,
	}).Run(ctx, arguments)
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()
	if code != 0 {
		return fmt.Errorf("status exited %d: %q", code, stderr.String())
	}
	return err
}

func (driver *Driver) readCompatibilityWatch() error {
	arguments := []string{watchCommandName, jsonFlag, diskPathFlag, ".", "--interval", "1s"}
	root := driver.leaseRoot
	environment := []string{"HIPPO_ROOT=" + root, "HOME=" + root, isolatedPath}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	if driver.mode == contract.E2E {
		driver.runCompatibilityWatchBinary(ctx, root, environment, arguments, &stdout, &stderr)
	} else {
		workingDirectory, err := os.Getwd()
		if err != nil {
			return err
		}
		if err = os.Chdir(root); err != nil {
			return err
		}
		defer func() { _ = os.Chdir(workingDirectory) }()
		_, _ = bootstrap.WithDefaults(cli.Application{
			Stdout: &stdout, Stderr: &stderr, Environment: environment,
		}).Run(ctx, arguments)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(stdout.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		driver.compatibilityWatch = append(driver.compatibilityWatch, line)
	}
	return nil
}

func (driver *Driver) runCompatibilityWatchBinary(ctx context.Context, root string, environment, arguments []string, stdout, stderr *bytes.Buffer) {
	previous, present := os.LookupEnv(hippoRootEnvironment)
	_ = os.Setenv(hippoRootEnvironment, root)
	defer func() {
		if present {
			_ = os.Setenv(hippoRootEnvironment, previous)
		} else {
			_ = os.Unsetenv(hippoRootEnvironment)
		}
	}()
	command := exec.CommandContext(ctx, driver.binary, arguments...)
	command.Env = environment
	command.Stdout, command.Stderr = stdout, stderr
	_ = command.Run()
}

func (driver *Driver) compatibilityOwners() ([]map[string]any, map[string]any, error) {
	var document map[string]any
	if err := json.Unmarshal([]byte(driver.output), &document); err != nil {
		return nil, nil, err
	}
	coordination, _ := document["coordination"].(map[string]any)
	if coordination == nil {
		return nil, nil, errors.New("status lacks coordination totals")
	}
	rows, _ := coordination["owners"].([]any)
	owners := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		record, ok := row.(map[string]any)
		if !ok {
			return nil, nil, errors.New("status owner row is not an object")
		}
		owners = append(owners, record)
	}
	return owners, coordination, nil
}

func (driver *Driver) requireNoAnnotationFields() error {
	for _, private := range []string{"displayAnnotation", "availability", "\"label\"", "first-owner", "current-owner", "changed-"} {
		if strings.Contains(driver.output, private) {
			return fmt.Errorf("observation leaked annotation field %q", private)
		}
	}
	return nil
}
