package support

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wahidyankf/hippo/internal/cli"
	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

// healthyAdmissionSamples gives the collector a steady healthy reading. The
// collector clamps to its last sample, so one attempt or twenty all see the same
// host, and the scenario is about the retry rather than about changing pressure.
func (driver *Driver) healthyAdmissionSamples() {
	base := time.Now()
	driver.samples = []policy.Sample{
		healthySample(base),
		healthySample(base.Add(time.Millisecond)),
		healthySample(base.Add(2 * time.Millisecond)),
	}
}

const coordinationModeMarker = "coordination-mode.json"

func (driver *Driver) capacityBlockedRoot() error {
	if err := driver.emptyCoordinationRoot(); err != nil {
		return err
	}
	driver.configPath = filepath.Join(driver.leaseRoot, "hippo.json")
	if err := os.WriteFile(driver.configPath, []byte(`{"schemaVersion":2,"coordination":{"maxCpu":1,"maxMemoryMiB":256,"maxActiveOwners":2}}`), 0o600); err != nil {
		return err
	}
	plan := guard.ReservationPlan{
		Capacity:  guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
		Requested: guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
		Allocated: guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
	}
	session, err := guard.AcquireReservation(
		context.Background(), driver.leaseRoot, "", policy.TaskEphemeral, "balanced", "", plan, 2, 0,
	)
	if err != nil {
		return err
	}
	driver.admissionSession = session

	return nil
}

// deferringRootFreeingCapacity fills the reservation pool, then arranges for
// the one registered waiter to receive capacity from its deterministic pause.
func (driver *Driver) deferringRootFreeingCapacity() error {
	if err := driver.capacityBlockedRoot(); err != nil {
		return err
	}

	driver.healthyAdmissionSamples()
	driver.admissionUnblockAfter = 1

	return nil
}

// permanentlyDeferringRoot never frees capacity, so the only thing that ends the
// retry loop is the caller's own budget.
func (driver *Driver) permanentlyDeferringRoot() error {
	if err := driver.capacityBlockedRoot(); err != nil {
		return err
	}

	driver.healthyAdmissionSamples()
	driver.admissionUnblockAfter = 0

	return nil
}

// admittingRootWithFailingChild leaves the root free, so the owner is admitted on
// its first attempt and the child's own failure is the only outcome to report.
func (driver *Driver) admittingRootWithFailingChild() error {
	if err := driver.capacityBlockedRoot(); err != nil {
		return err
	}
	if err := guard.ReleaseReservation(driver.leaseRoot, driver.admissionSession); err != nil {
		return err
	}
	driver.admissionSession = nil

	driver.healthyAdmissionSamples()
	driver.admissionUnblockAfter = 0

	return nil
}

// deferralNoticePrefix is the notice a waiting caller would otherwise receive
// once per attempt.
const deferralNoticePrefix = "HIPPO deferred task:"

// deferringRootFreeingCapacityOntoExhaustedStorage defers the first attempt and
// then frees capacity onto a host whose disk sits under the floor. The second
// attempt therefore gets past coordination and reaches the storage check, which
// is the point: by then the caller has asked for the deferral notice to be quiet,
// and the storage notice still has to be said out loud.
func (driver *Driver) deferringRootFreeingCapacityOntoExhaustedStorage() error {
	if err := driver.capacityBlockedRoot(); err != nil {
		return err
	}

	base := time.Now()
	blocked := healthySample(base)
	blocked.DiskFreeBytes = new(200 * policy.MiB)
	blocked.DiskTotalBytes = new(policy.GiB)

	// The CLI resolves a profile before it ever reaches the guard, and a host that
	// is already blocked there never gets as far as a deferral. So the readings
	// start healthy and only fall under the floor once the guard is sampling.
	driver.samples = []policy.Sample{
		healthySample(base),
		healthySample(base.Add(time.Millisecond)),
		healthySample(base.Add(2 * time.Millisecond)),
		blocked, blocked, blocked,
	}
	driver.admissionUnblockAfter = 1

	return nil
}

// requireDeferralReportedOnce proves the notice is reported per waiting run
// rather than per attempt. It asserts the attempt count first, because one notice
// across one attempt would say nothing at all.
func (driver *Driver) requireDeferralReportedOnce() error {
	if driver.admissionAttempts < 3 {
		return fmt.Errorf(
			"only %d queue polls were made, too few to prove a bounded wait",
			driver.admissionAttempts,
		)
	}

	if notices := strings.Count(driver.errorOutput, deferralNoticePrefix); notices != 1 {
		return fmt.Errorf(
			"the deferral was reported %d times across %d attempts, want once: %s",
			notices, driver.admissionAttempts, driver.errorOutput,
		)
	}

	if heartbeats := strings.Count(driver.errorOutput, "HIPPO queued run="); heartbeats != 1 {
		return fmt.Errorf("queue heartbeats=%d, want one change report: %s", heartbeats, driver.errorOutput)
	}
	if runs := driver.childRunCount(); runs != 0 {
		return fmt.Errorf("deferred waiter launched %d payloads, want zero", runs)
	}

	return nil
}

// requireBlockedNoticeSurvivesQuieting proves quieting is scoped to the deferral.
// A caller that silenced the whole stream would pass the counting scenario and
// fail here, which is the only reason this scenario exists.
func (driver *Driver) requireBlockedNoticeSurvivesQuieting() error {
	// Both sheds exit 124; the reason is what says which one, so the exit
	// alone is no longer enough to prove this scenario measured storage.
	if driver.exitCode != status.LimitShed ||
		!strings.Contains(driver.errorOutput, string(status.CodeLimitStorageBlocked)) {
		return fmt.Errorf(
			"exit=%d want %d and %s: stderr=%s",
			driver.exitCode, status.LimitShed, status.CodeLimitStorageBlocked, driver.errorOutput,
		)
	}

	if !strings.Contains(driver.errorOutput, "HIPPO blocked task:") {
		return fmt.Errorf("quieting the deferral also silenced the storage notice: %s", driver.errorOutput)
	}

	if notices := strings.Count(driver.errorOutput, deferralNoticePrefix); notices != 0 {
		return fmt.Errorf(
			"the admitted waiter reported %d deferrals, want none: %s",
			notices, driver.errorOutput,
		)
	}

	return nil
}

// runWaitingForAdmission drives the public CLI with a wait budget. The injected
// sleep is the scenario's clock: it counts queue polls, advances the budget by
// exactly what admission asked to wait, and frees capacity at the arranged
// poll, so the outcome never depends on how fast the host happens to be.
func (driver *Driver) runWaitingForAdmission(budget time.Duration, childExit int) error {
	root := driver.leaseRoot
	runs := filepath.Join(root, "child-runs")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}

	var elapsed atomic.Int64

	base := time.Now()
	driver.admissionAttempts = 0
	driver.admissionElapsed = 0

	application := cli.Application{
		Stdout:      stdout,
		Stderr:      stderr,
		Environment: []string{"HIPPO_ROOT=" + root, "HIPPO_CHILD_RUNS=" + runs},
		Collector:   &sequenceCollector{samples: driver.samples},
		Now:         func() time.Time { return base.Add(time.Duration(elapsed.Load())) },
	}
	// This sleep serves the queue pause and the guard's own sampling alike. It
	// advances the scenario clock and frees
	// capacity at the arranged point; the assertions below read behaviour instead.
	application.Sleep = func(duration time.Duration) {
		driver.admissionAttempts++
		elapsed.Add(int64(duration))
		if driver.admissionUnblockAfter > 0 && driver.admissionAttempts >= driver.admissionUnblockAfter &&
			driver.admissionSession != nil {
			_ = guard.ReleaseReservation(root, driver.admissionSession)
			driver.admissionSession = nil
		}
	}

	code, err := application.Run(context.Background(), []string{
		runCommandName,
		taskClassFlag, taskClassEphemeral,
		configFlag, driver.configPath,
		diskPathFlag, ".",
		"--wait-for-admission", budget.String(),
		"--reserve-cpu", "1", "--reserve-memory-mib", "256",
		"--", shellPath, "-c",
		fmt.Sprintf(`printf x >> "$HIPPO_CHILD_RUNS"; exit %d`, childExit),
	})

	driver.exitCode = code
	driver.output = stdout.String()
	driver.errorOutput = stderr.String()
	driver.admissionElapsed = time.Duration(elapsed.Load())

	return err
}

// childRunCount reports how many times the guarded payload actually executed.
func (driver *Driver) childRunCount() int {
	data, err := os.ReadFile(filepath.Join(driver.leaseRoot, "child-runs"))
	if err != nil {
		return 0
	}

	return len(data)
}

func (driver *Driver) requireRetriedThenAdmitted() error {
	if driver.admissionSession != nil {
		return errors.New("capacity was never freed, so this proves nothing about queueing")
	}
	if driver.exitCode != 3 {
		return fmt.Errorf("exit=%d want the admitted child's own code 3: stderr=%s", driver.exitCode, driver.errorOutput)
	}
	if runs := driver.childRunCount(); runs != 1 {
		return fmt.Errorf("the payload ran %d times, want exactly one admitted run", runs)
	}

	return nil
}

func (driver *Driver) requireDeferralSurvivesTheBudget() error {
	if driver.exitCode != status.LimitShed ||
		!strings.Contains(driver.errorOutput, string(status.CodeLimitCapacityDeferred)) {
		return fmt.Errorf(
			"exit=%d want %d and %s: stderr=%s",
			driver.exitCode, status.LimitShed, status.CodeLimitCapacityDeferred, driver.errorOutput,
		)
	}
	if driver.admissionElapsed < time.Second {
		return fmt.Errorf("the owner surrendered after %s without spending its one second budget", driver.admissionElapsed)
	}
	if runs := driver.childRunCount(); runs != 0 {
		return fmt.Errorf("a permanently deferred owner ran its payload %d times", runs)
	}

	return nil
}

func (driver *Driver) requireFailingChildReportedUnchanged() error {
	// The shared When step runs a child that exits 3. What separates this scenario
	// from the deferral one is not the code but that it was never retried.
	if driver.exitCode != 3 {
		return fmt.Errorf("exit=%d want the child's own failing code 3: stderr=%s", driver.exitCode, driver.errorOutput)
	}
	// A loop that retried a non-deferral would keep relaunching an admitting root
	// until the budget ran out, so a single run is what proves it stopped at once.
	if runs := driver.childRunCount(); runs != 1 {
		return fmt.Errorf("the payload ran %d times, want exactly one", runs)
	}

	return nil
}
