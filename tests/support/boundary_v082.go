package support

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/cli"
	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

// Configuration documents a boundary run can be given. Schema 1 keeps the
// compatibility coordination, so an admission decision is reached without a
// reservation; schema 2 turns reservations on.
const (
	boundarySchemaOne = schemaOneDocument
	boundarySchemaTwo = `{"schemaVersion":2}`
	reserveCPUFlag    = "--reserve-cpu"
	reserveMemoryFlag = "--reserve-memory-mib"
)

// boundaryOutcome is what a caller of the public command line sees: the status,
// what was written to stderr, and whether the guarded payload ever ran.
type boundaryOutcome struct {
	exitCode     int
	stderr       string
	childStarted bool
}

// runGuardedAtBoundary runs one `hippo run` through the public command-line
// application against root. Many scenarios arrange an internal outcome and
// observe it below the boundary, where the numbers the guard and policy layers
// use among themselves are still visible. A caller never sees those numbers,
// so a scenario that promises a caller a status and a reason runs this too:
// only the boundary proves which of the closed statuses and reasons came out.
//
// The configuration is explicit and the child's working directory is a
// scratch directory, so nothing this checkout tracks is discovered.
func runGuardedAtBoundary(root, configDocument string, samples []policy.Sample, arguments ...string) (boundaryOutcome, error) {
	scratch, err := os.MkdirTemp("", "hippo-boundary-")
	if err != nil {
		return boundaryOutcome{}, err
	}
	defer func() { _ = os.RemoveAll(scratch) }()

	configPath := filepath.Join(scratch, "hippo.local.json")
	if err = os.WriteFile(configPath, []byte(configDocument), 0o600); err != nil {
		return boundaryOutcome{}, err
	}
	marker := filepath.Join(scratch, "child-started")

	command := make([]string, 0, 9+len(arguments)+4)
	command = append(command, runCommandName, configFlag, configPath, "--cwd", scratch, diskPathFlag, scratch)
	command = append(command, arguments...)
	command = append(command, "--", shellPath, "-c", childStartedScript)

	if len(samples) == 0 {
		samples = []policy.Sample{healthySample(time.Now())}
	}
	stderr := &bytes.Buffer{}
	code, _ := (cli.Application{
		Stdin:       strings.NewReader(""),
		Stdout:      &bytes.Buffer{},
		Stderr:      stderr,
		Environment: []string{hippoRootEnvironment + "=" + root, "CHILD_MARKER=" + marker, "PATH=/usr/bin:/bin:/usr/sbin:/sbin"},
		Collector:   &sequenceCollector{samples: samples},
		Sleep:       func(time.Duration) {},
	}).Run(context.Background(), command)

	_, statError := os.Stat(marker)

	return boundaryOutcome{exitCode: code, stderr: stderr.String(), childStarted: statError == nil}, nil
}

// requireReason holds the outcome to one closed status and the reason line a
// caller greps for, and to a payload that never ran.
func (outcome boundaryOutcome) requireReason(exitCode int, reason status.Code) error {
	line := "hippo: [" + string(reason) + "]"
	if outcome.exitCode != exitCode || !strings.Contains(outcome.stderr, line) || outcome.childStarted {
		return fmt.Errorf(
			"a caller saw exit %d (payload ran: %t) and stderr %q, want exit %d naming %s before any payload",
			outcome.exitCode, outcome.childStarted, outcome.stderr, exitCode, reason,
		)
	}

	return nil
}

// requireReasonAtBoundary runs the boundary and holds it to one status and reason.
func requireReasonAtBoundary(
	root, configDocument string, samples []policy.Sample, exitCode int, reason status.Code, arguments ...string,
) error {
	outcome, err := runGuardedAtBoundary(root, configDocument, samples, arguments...)
	if err != nil {
		return err
	}

	return outcome.requireReason(exitCode, reason)
}

// requireSupervisionFailedAtBoundary is the caller's view of compatibility
// state reservation admission cannot verify: exit 125 naming
// hippo.supervision.failed, with the state it could not verify left byte for
// byte as it was.
func requireSupervisionFailedAtBoundary(root, statePath string, state []byte) error {
	if err := requireReasonAtBoundary(
		root, boundarySchemaTwo, nil, status.GuardFailed, status.CodeSupervisionFailed, reservationArguments()...,
	); err != nil {
		return err
	}
	after, err := os.ReadFile(statePath)
	if err != nil || !bytes.Equal(after, state) {
		return fmt.Errorf("the public run changed the state it could not verify: %q: %w", after, err)
	}

	return nil
}

// reservationArguments request the smallest explicit reservation, so a
// schema-2 boundary run reaches reservation admission without planning one.
func reservationArguments() []string {
	return []string{reserveCPUFlag, "1", reserveMemoryFlag, "256"}
}

// requireNothingEnqueued proves a refusal came before the queue: no owner and
// no waiter was ever recorded for it.
func requireNothingEnqueued(root string) error {
	totals, err := guard.ReservationStatus(context.Background(), root)
	if err != nil {
		return err
	}
	if totals.ActiveOwners != 0 || totals.WaitingOwners != 0 {
		return fmt.Errorf("the refused request reached the queue: totals=%+v", totals)
	}

	return nil
}

// requireFloorMemoryReplansAtBoundary asks the public command line for less
// memory than the reservation floor, which no profile can admit as asked.
func (driver *Driver) requireFloorMemoryReplansAtBoundary() error {
	if err := requireReasonAtBoundary(
		driver.evidenceRoot, boundarySchemaTwo, nil, status.GuardFailed, status.CodePolicyReplanRequired,
		reserveCPUFlag, "1", reserveMemoryFlag, "255",
	); err != nil {
		return err
	}

	return requireNothingEnqueued(driver.evidenceRoot)
}

// requireNegativeCPUIsUsageMistakeAtBoundary asks for a CPU count below one.
// The flag reads zero as "plan it automatically", so the only CPU request
// below one a caller can type is a negative one, and that is a mistake in the
// invocation rather than a request some other plan could admit.
func (driver *Driver) requireNegativeCPUIsUsageMistakeAtBoundary() error {
	if err := requireReasonAtBoundary(
		driver.evidenceRoot, boundarySchemaTwo, nil, status.CallerError, status.CodeArgsInvalid,
		reserveCPUFlag, "-1", reserveMemoryFlag, "256",
	); err != nil {
		return err
	}

	return requireNothingEnqueued(driver.evidenceRoot)
}

// requireAdmissionReasonAtBoundary runs the scenario's host samples through
// the public command line, so an admission decision taken from them is held
// to the status and reason a caller receives.
func (driver *Driver) requireAdmissionReasonAtBoundary(exitCode int, reason status.Code) error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	class := string(driver.taskClass)
	if class == "" {
		class = taskClassEphemeral
	}

	return requireReasonAtBoundary(root, boundarySchemaOne, driver.samples, exitCode, reason, taskClassFlag, class)
}

// requireImpossibleReplanAtBoundary asks the public command line for more CPU
// than the healthy host it samples has at all: no wait could admit it, so the
// caller is told to replan and nothing joins the queue.
func requireImpossibleReplanAtBoundary(root string) error {
	if err := requireReasonAtBoundary(
		root, boundarySchemaTwo, nil, status.GuardFailed, status.CodePolicyReplanRequired,
		reserveCPUFlag, "64", reserveMemoryFlag, "256",
	); err != nil {
		return err
	}

	return requireNothingEnqueued(root)
}

// requireNearlySpentBudgetDefersOnCapacity binds the race a timed wait can
// only hit by luck. The bounded wait's last passes reach the coordination lock
// with nanoseconds of budget left; a lock that lost to its own expired timer
// there turned a capacity deferral into a coordination deferral that writes no
// never-started receipt. Both regressions pin that pass deterministically: an
// injected clock parks the wait one nanosecond short of its deadline, and an
// uncontended lock is taken with a one-nanosecond budget.
func requireNearlySpentBudgetDefersOnCapacity() error {
	return errors.Join(
		runGoRegressionV10("./tests/unit", "TestBoundedWaitDefersOnCapacityWhenBudgetIsNearlySpent"),
		runInternalGuardRegressionV04("TestCoordinationLockGrantsFreeRootWhenBudgetIsNearlySpent"),
	)
}

// requireCapacityDeferredAtBoundary asks the public command line for a
// reservation that fits the configured budget but not what a live owner left
// of it. The bounded wait elapses, and the caller is told capacity deferred
// the work, with a new never-started receipt saying nothing ran.
func requireCapacityDeferredAtBoundary(root string) error {
	before, err := neverStartedReceipts(root)
	if err != nil {
		return err
	}
	if err = requireReasonAtBoundary(
		root, `{"schemaVersion":2,"coordination":{"maxCpu":4,"maxMemoryMiB":1024}}`, nil,
		status.LimitShed, status.CodeLimitCapacityDeferred,
		append(reservationArguments(), "--wait-for-admission", "30ms")...,
	); err != nil {
		return err
	}
	after, err := neverStartedReceipts(root)
	if err != nil {
		return err
	}
	if after <= before {
		return fmt.Errorf("the deferral wrote no new never-started receipt: before=%d after=%d", before, after)
	}

	return nil
}

// neverStartedReceipts counts the schema-1 receipts that record work which
// never started.
func neverStartedReceipts(root string) (int, error) {
	entries, err := os.ReadDir(filepath.Join(root, "receipts"))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		data, readError := os.ReadFile(filepath.Join(root, "receipts", entry.Name()))
		if readError != nil {
			return 0, readError
		}
		var receipt guard.SafetyReceipt
		if json.Unmarshal(data, &receipt) == nil && receipt.SchemaVersion == 1 && receipt.State == "never-started" {
			count++
		}
	}

	return count, nil
}

// requirePressureShedAtBoundary holds the public command line to what a shed
// looks like to its caller. The shed scenarios drive the guard directly,
// because only there can synthetic pressure be injected while the child's
// lifecycle is controlled, and the guard answers with an internal status no
// caller ever sees. The boundary regression runs `hippo run` through the
// command-line application, admits a child, sheds it under critical pressure,
// and requires exit 124 naming hippo.limit.pressure-shed rather than the
// capacity deferral that shares its status. Whichever pressure caused a shed,
// it leaves the guard as that one internal status, so this is the boundary
// both shed scenarios cross.
func requirePressureShedAtBoundary() error {
	return runGoRegressionV10("./internal/cli", "TestPressureShedNamesItsOwnReason")
}

// requireShedReasonsAtBoundary holds both shed statuses the guard returns to
// what a caller receives: its storage status as 124 naming
// hippo.limit.storage-blocked, and its other-pressure status as 124 naming
// hippo.limit.pressure-shed. The guard regression proves which internal status
// each shed returns; these prove how the command line reports each one.
func requireShedReasonsAtBoundary() error {
	root, err := os.MkdirTemp("", "hippo-boundary-root-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(root) }()
	exhausted := capacitySample(policy.GiB, 512*policy.MiB)
	exhausted.DiskFreeBytes = new(200 * policy.MiB)
	exhausted.DiskTotalBytes = new(policy.GiB)

	return errors.Join(
		requireReasonAtBoundary(
			root, boundarySchemaOne, []policy.Sample{exhausted}, status.LimitShed, status.CodeLimitStorageBlocked,
			taskClassFlag, taskClassEphemeral,
		),
		requirePressureShedAtBoundary(),
	)
}
