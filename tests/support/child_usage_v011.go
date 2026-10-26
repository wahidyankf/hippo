package support

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/bootstrap"

	guard "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/tests/contract"
)

// Child usage is the CPU and wall time of the process tree a guarded run
// waited for. The payloads below keep one CPU-bound helper busy for one
// CPU-second. The helper and sleep use absolute paths, so no payload needs PATH.
const (
	cpuBusyHelper = "/usr/bin/yes"
	// busyRound keeps the helper running until the helper has itself used one
	// CPU-second, then reaps it: the helper is then a waited descendant of the
	// guard's child. A wall-clock bound would not do, because a helper timed by
	// the wall clock gets only the share of a core a contended host leaves it.
	// ps reports the helper's own CPU time, whole seconds on Linux and hundredths
	// on macOS, and the payload polls it until it reads at least one second. The
	// poll is capped, so a host without ps fails the assertion rather than hangs.
	busyRound = cpuBusyHelper + " >/dev/null & p=$!; n=0; while [ $n -lt 1200 ]; do " +
		`set -- $(/bin/ps -o time= -p $p); case $1 in 0:00.*|00:00:00|"") ;; *) break;; esac; ` +
		"n=$((n+1)); /bin/sleep 0.05; done; kill $p; wait $p 2>/dev/null"
	// grandchildRound runs the busy round in a nested shell the payload waits for.
	grandchildRound = "/bin/sh -c '" + busyRound + "'"
	// unreapedRound starts its helper from a subshell that exits at once, so the
	// helper is no process's child within the waited tree and the shell cannot
	// reap it. A helper the shell started itself would be reaped, and its CPU
	// carried into the shell's usage, whenever a group signal killed the helper
	// before the shell. A shed then leaves the helper's CPU unrecorded however
	// the host schedules the signals.
	unreapedRound = "(" + cpuBusyHelper + " >/dev/null &); /bin/sleep 30"
	// shedHealthySamples keeps the guard healthy for about one second at the
	// fixture's one-millisecond sample interval before it reports critical pressure.
	shedHealthySamples = 1000
	// childUsageRunID and predatingRunID name the staged summaries the
	// compaction and old-summary scenarios read back by identifier.
	childUsageRunID  = "child-usage-compacted"
	predatingRunID   = "child-usage-predates"
	resourceTierFlag = "--resource-tier"
	cpuSecondsField  = "childCpuSeconds"
	wallSecondsField = "childWallSeconds"
	// previousDayAge places a staged summary's lifetime before today in any zone.
	previousDayAge = 36 * time.Hour
)

// childUsageRow is the part of one history row the child-usage scenarios read,
// with every field the row carries kept for the privacy assertions.
type childUsageRow struct {
	RunID   string   `json:"runId"`
	Outcome string   `json:"outcome"`
	CPU     *float64 `json:"childCpuSeconds"`
	Wall    *float64 `json:"childWallSeconds"`
	fields  map[string]json.RawMessage
}

func (driver *Driver) childUsageBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^an empty evidence root for guarded runs$`, driver.emptyGuardedRunRoot),
		step(`^a guarded child keeps one CPU busy for about one second$`, func() error {
			return driver.runGuardedChild(busyRound + "; exit 0")
		}),
		step(`^a guarded child waits for a grandchild that keeps one CPU busy for about one second$`, func() error {
			return driver.runGuardedChild(grandchildRound + "; exit 0")
		}),
		step(`^a guarded child keeps one CPU busy for about one second and then exits 3$`, func() error {
			return driver.runGuardedChild(busyRound + "; exit 3")
		}),
		step(`^a guarded child starts a CPU-busy grandchild that the guard never waits for and critical pressure sheds it$`, driver.shedGuardedChild),
		step(`^a passed run from the previous day that recorded its child usage$`, driver.stagePreviousDayUsageSummary),
		step(`^a later guarded run completes$`, func() error { return driver.runGuardedChild("exit 0") }),
		step(`^a passed run whose summary predates child usage fields$`, driver.stagePredatingSummary),
		step(`^the history row records at least 0\.8 seconds of child CPU and at least 1 second of child wall time$`, driver.requirePassedRowUsage),
		step(`^the history row records at least 0\.8 seconds of child CPU$`, driver.requireWaitedRowUsage),
		step(`^the history row outcome reads task-failed and records at least 0\.8 seconds of child CPU$`, driver.requireFailedRowUsage),
		step(`^the history row reads pressure-shed and records less than 0\.5 seconds of child CPU$`, driver.requireShedRowLowerBound),
		step(`^the history row still records at least 0\.8 seconds of child CPU and at least 1 second of child wall time$`, driver.requireCompactedRowUsage),
		step(`^the history row reads passed and carries no child usage fields$`, driver.requirePredatingRowWithoutUsage),
		step(`^the history row carries no command, argument, working directory, or path$`, driver.requireUsageRowPrivacy),
		step(`^its lifetime summary carries no child usage fields$`, driver.requireNoUsageInInterruptedSummary),
	}
}

// emptyGuardedRunRoot gives the scenario a fresh evidence root for its guarded runs.
func (driver *Driver) emptyGuardedRunRoot() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	driver.leaseRoot = root

	return nil
}

// runGuardedChild runs the guard over the scenario's evidence root with a shell
// payload that executes script.
func (driver *Driver) runGuardedChild(script string) error {
	return driver.runGuardInRoot(driver.leaseRoot, runCommandName, diskPathFlag, ".", resourceTierFlag, standardTierName, "--", shellPath, "-c", script)
}

// runGuardInRoot runs the guard with root as its evidence root: through the
// compiled binary at the end-to-end boundary, and in process otherwise. Neither
// samples the live host: admission is not these scenarios' subject, and a
// runner's own memory state would otherwise defer a run their inputs admit.
func (driver *Driver) runGuardInRoot(root string, arguments ...string) error {
	if driver.mode == e2eMode {
		return driver.runBinaryInRoot(root, arguments...)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	base := time.Now()
	code, _ := bootstrap.WithDefaults(cli.Application{
		Stdout: stdout, Stderr: stderr, Environment: []string{hippoRootEnvironment + "=" + root},
		Collector: &sequenceCollector{samples: []policy.Sample{
			healthySample(base), healthySample(base.Add(time.Millisecond)), healthySample(base.Add(2 * time.Millisecond)),
		}},
		Sleep: func(time.Duration) {},
	}).Run(context.Background(), arguments)
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()

	return nil
}

// shedGuardedChild runs a payload that leaves its helper unreaped, with healthy
// samples for about a second and then critical memory pressure. The guard must
// shed the payload's group and exit with the pressure-shed status. The sample
// sequence is injected, so this scenario is in-process only.
func (driver *Driver) shedGuardedChild() error {
	samples := make([]policy.Sample, 0, shedHealthySamples+1)
	base := time.Now()
	for range shedHealthySamples {
		samples = append(samples, healthySample(base))
	}
	critical := healthySample(base)
	critical.MemoryPressureLevel = new(4)
	samples = append(samples, critical)
	stderr := &bytes.Buffer{}
	code, err := RunGuard(context.Background(), guard.RunConfig{
		Command: shellPath, Arguments: []string{"-c", unreapedRound}, TaskClass: taskClassEphemeral,
		Environment: os.Environ(), EvidenceRoot: driver.leaseRoot, DiskPath: ".",
		Collector: &sequenceCollector{samples: samples}, Policy: fastBehaviourPolicy(),
		RetirementConfirmation: fixtureLivenessWait, Sleep: func(time.Duration) {}, Now: time.Now, Stderr: stderr,
	})
	driver.errorOutput = stderr.String()
	if err = driver.recordGuardResult(code, err); err != nil {
		return err
	}
	if driver.reason != policy.ReasonPressureShed {
		return fmt.Errorf("shed run exit=%d reason=%v, want %v: %s", code, driver.reason, policy.ReasonPressureShed, stderr.String())
	}

	return nil
}

// stagePreviousDayUsageSummary leaves a passed summary that recorded child
// usage and finished before today, so the next guarded run compacts it.
func (driver *Driver) stagePreviousDayUsageSummary() error {
	if err := driver.emptyGuardedRunRoot(); err != nil {
		return err
	}

	return writeChildUsageSummary(driver.leaseRoot, childUsageRunID, time.Now().Add(-previousDayAge), map[string]any{
		cpuSecondsField: 0.9, wallSecondsField: 1.2,
	})
}

// stagePredatingSummary leaves a passed summary written before child usage existed.
func (driver *Driver) stagePredatingSummary() error {
	if err := driver.emptyGuardedRunRoot(); err != nil {
		return err
	}

	return writeChildUsageSummary(driver.leaseRoot, predatingRunID, time.Now(), nil)
}

// writeChildUsageSummary writes one current summary in the shape a guarded run
// finalizes, with the given child-usage fields added.
func writeChildUsageSummary(root, runID string, finished time.Time, fields map[string]any) error {
	values := map[string]any{
		schemaVersionField: 5, runIDField: runID, "source": hippoFixtureName,
		"taskClass": string(policy.TaskEphemeral), "resourceTier": standardTierName, "outcome": "passed",
		"finishedAt": finished.UTC().Format(time.RFC3339Nano),
	}
	maps.Copy(values, fields)
	encoded, err := json.Marshal(values)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(root, runID+".summary.json"), append(encoded, '\n'), 0o600)
}

// childUsageRows decodes every row the last history query listed.
func (driver *Driver) childUsageRows() ([]childUsageRow, error) {
	if driver.exitCode != 0 {
		return nil, fmt.Errorf("history exit=%d stderr=%s", driver.exitCode, driver.errorOutput)
	}
	var payload struct {
		Rows []json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal([]byte(driver.output), &payload); err != nil {
		return nil, err
	}
	rows := make([]childUsageRow, 0, len(payload.Rows))
	for _, raw := range payload.Rows {
		var row childUsageRow
		if err := json.Unmarshal(raw, &row); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &row.fields); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}

	return rows, nil
}

// childUsageRowFor returns the one row the last history query listed for runID,
// or the only row when runID is empty.
func (driver *Driver) childUsageRowFor(runID string) (childUsageRow, error) {
	rows, err := driver.childUsageRows()
	if err != nil {
		return childUsageRow{}, err
	}
	var matched []childUsageRow
	for _, row := range rows {
		if runID == "" || row.RunID == runID {
			matched = append(matched, row)
		}
	}
	if len(matched) != 1 {
		return childUsageRow{}, fmt.Errorf("history rows %+v, want exactly one row for %q", rows, runID)
	}

	return matched[0], nil
}

// minimumChildCPUSeconds is the CPU a passed or failed run must show. Its busy
// helper ends only after a full CPU-second, so the bar sits just below that to
// cover how a kernel rounds the usage it reports.
const minimumChildCPUSeconds = 0.8

// Field-name and output fragments that the privacy check forbids. Naming them
// keeps the same literal from repeating across this file.
const (
	forbiddenCommandField        = "command"
	forbiddenArgumentField       = "argument"
	forbiddenDirectoryField      = "directory"
	forbiddenWorkingDirectoryKey = "workingDirectory"
)

// requireChildUsage checks a row's outcome, at least minimumChildCPUSeconds of
// CPU, and at least minimumWall seconds of wall time.
func requireChildUsage(row childUsageRow, outcome string, minimumWall float64) error {
	if row.Outcome != outcome {
		return fmt.Errorf("history row outcome %q, want %q: %+v", row.Outcome, outcome, row)
	}
	if row.CPU == nil || *row.CPU < minimumChildCPUSeconds {
		return fmt.Errorf("history row child CPU %v, want at least %v seconds: %+v", row.CPU, minimumChildCPUSeconds, row)
	}
	if minimumWall > 0 && (row.Wall == nil || *row.Wall < minimumWall) {
		return fmt.Errorf("history row child wall %v, want at least %v seconds: %+v", row.Wall, minimumWall, row)
	}

	return nil
}

func (driver *Driver) requirePassedRowUsage() error {
	row, err := driver.childUsageRowFor("")
	if err != nil {
		return err
	}

	return requireChildUsage(row, "passed", 1)
}

func (driver *Driver) requireWaitedRowUsage() error {
	row, err := driver.childUsageRowFor("")
	if err != nil {
		return err
	}

	return requireChildUsage(row, "passed", 0)
}

func (driver *Driver) requireFailedRowUsage() error {
	row, err := driver.childUsageRowFor("")
	if err != nil {
		return err
	}

	return requireChildUsage(row, "task-failed", 0)
}

// requireShedRowLowerBound checks that a shed run recorded its usage, and that
// the unreaped helper's CPU is missing from it: the value stays below what the
// helper burned before the shed.
func (driver *Driver) requireShedRowLowerBound() error {
	row, err := driver.childUsageRowFor("")
	if err != nil {
		return err
	}
	if row.Outcome != "pressure-shed" || row.CPU == nil || *row.CPU >= 0.5 {
		return fmt.Errorf("shed history row %+v, want pressure-shed with less than 0.5 seconds of child CPU", row)
	}

	return nil
}

func (driver *Driver) requireCompactedRowUsage() error {
	row, err := driver.childUsageRowFor(childUsageRunID)
	if err != nil {
		return err
	}

	return requireChildUsage(row, "passed", 1)
}

func (driver *Driver) requirePredatingRowWithoutUsage() error {
	row, err := driver.childUsageRowFor(predatingRunID)
	if err != nil {
		return err
	}
	if row.Outcome != "passed" || row.CPU != nil || row.Wall != nil {
		return fmt.Errorf("predating history row %+v, want passed with no child usage", row)
	}

	return nil
}

// requireUsageRowPrivacy extends the history privacy assertion to the child
// usage row: only the two scalars are new, and no command, argument, working
// directory, or path reaches the output.
func (driver *Driver) requireUsageRowPrivacy() error {
	row, err := driver.childUsageRowFor("")
	if err != nil {
		return err
	}
	for key := range row.fields {
		for _, forbidden := range []string{forbiddenCommandField, forbiddenArgumentField, "argv", "args", forbiddenDirectoryField, "cwd", "path"} {
			if strings.Contains(strings.ToLower(key), forbidden) {
				return fmt.Errorf("history row exposed forbidden field %q: %s", key, driver.output)
			}
		}
	}
	for _, forbidden := range []string{shellPath, cpuBusyHelper, driver.leaseRoot, "argv", forbiddenWorkingDirectoryKey} {
		if strings.Contains(driver.output, forbidden) {
			return fmt.Errorf("history exposed forbidden %q: %s", forbidden, driver.output)
		}
	}

	return nil
}

// requireNoUsageInInterruptedSummary reads the summaries of the run hippo
// stopped before launch, and requires that neither child usage field is present.
func (driver *Driver) requireNoUsageInInterruptedSummary() error {
	summaries, err := filepath.Glob(filepath.Join(driver.interruption.root, "*.summary.json"))
	if err != nil {
		return fmt.Errorf("glob interrupted summaries under the run's root: %w", err)
	}
	if len(summaries) == 0 {
		return errors.New("no interrupted summaries under the run's root")
	}
	for _, summary := range summaries {
		data, readError := os.ReadFile(summary)
		if readError != nil {
			return readError
		}
		for _, field := range []string{cpuSecondsField, wallSecondsField} {
			if strings.Contains(string(data), field) {
				return fmt.Errorf("a never-started run's summary carries %s: %s", field, data)
			}
		}
	}

	return nil
}
