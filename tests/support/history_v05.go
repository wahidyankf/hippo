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
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/internal/cli"
	"github.com/wahidyankf/hippo/internal/evidence"
	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

const (
	hippoFixtureName = "hippo"
	standardTierName = "standard"
	worktreeTagValue = "worktree"
)

func (driver *Driver) labeledReservationOwnerV05() error {
	if err := driver.emptyCoordinationRoot(); err != nil {
		return err
	}
	plan := guard.ReservationPlan{
		Capacity:  guard.ReservationVector{CPU: 4, MemoryBytes: 8 * policy.GiB},
		Requested: guard.ReservationVector{CPU: 2, MemoryBytes: 3 * policy.GiB},
		Allocated: guard.ReservationVector{CPU: 2, MemoryBytes: 3 * policy.GiB},
		Minimum:   guard.ReservationVector{CPU: 2, MemoryBytes: 3 * policy.GiB},
		Maximum:   guard.ReservationVector{CPU: 4, MemoryBytes: 6 * policy.GiB},
		Tier:      standardTierName,
	}
	session, err := guard.AcquireReservationWithOptions(
		context.Background(), driver.leaseRoot, "", policy.TaskEphemeral, "balanced", "hash", plan, 2, time.Minute,
		guard.ReservationAdmissionOptions{Metadata: guard.ReservationMetadata{
			Source: hippoFixtureName, Tags: map[string]string{"checkout": worktreeTagValue}, Tier: standardTierName,
		}},
	)
	if err != nil {
		return err
	}
	driver.admissionSession = session
	base := time.Now()
	driver.samples = []policy.Sample{healthySample(base), healthySample(base.Add(time.Second))}

	return nil
}

func (driver *Driver) filteredLabeledStatusV05() error {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code, err := (cli.Application{
		Stdout: stdout, Stderr: stderr, Environment: []string{"HIPPO_ROOT=" + driver.leaseRoot},
		Collector: &sequenceCollector{samples: driver.samples}, Sleep: func(time.Duration) {},
	}).Run(context.Background(), []string{
		"status", jsonFlag, "--source", hippoFixtureName, tagFlagName, "checkout=worktree",
	})
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()

	return err
}

func (driver *Driver) requireFilteredLabeledStatusV05() error {
	if driver.exitCode != 0 {
		return fmt.Errorf("status exit=%d stderr=%s", driver.exitCode, driver.errorOutput)
	}
	var payload struct {
		SchemaVersion int `json:"schemaVersion"`
		Coordination  struct {
			Owners []guard.ReservationEntry `json:"owners"`
		} `json:"coordination"`
	}
	if err := json.Unmarshal([]byte(driver.output), &payload); err != nil {
		return err
	}
	if payload.SchemaVersion != 5 || len(payload.Coordination.Owners) != 1 {
		return fmt.Errorf("unexpected labeled status: %s", driver.output)
	}
	owner := payload.Coordination.Owners[0]
	if owner.Source != hippoFixtureName || owner.Tier != standardTierName || owner.Tags["checkout"] != worktreeTagValue {
		return fmt.Errorf("unexpected owner labels: %+v", owner)
	}
	for _, forbidden := range []string{"command", "argument", "workingDirectory", driver.leaseRoot} {
		if strings.Contains(driver.output, forbidden) {
			return fmt.Errorf("status exposed forbidden %q: %s", forbidden, driver.output)
		}
	}

	return nil
}

func (driver *Driver) labeledHistoryV05() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	driver.leaseRoot = root
	now := time.Now().UTC()
	for _, source := range []string{hippoFixtureName, "rhino"} {
		row := evidence.Summary{
			SchemaVersion: 5, RunID: source, Source: source, Tags: map[string]string{"checkout": worktreeTagValue},
			TaskClass: "ephemeral", ResourceTier: standardTierName, Outcome: "passed",
			FinishedAt: now.Format(time.RFC3339Nano),
		}
		encoded, encodeError := json.Marshal(row)
		if encodeError != nil {
			return encodeError
		}
		if err = os.WriteFile(filepath.Join(root, source+".summary.json"), append(encoded, '\n'), 0o600); err != nil {
			return err
		}
	}

	return nil
}

func (driver *Driver) filteredHistoryV05() error {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code, err := (cli.Application{
		Stdout: stdout, Stderr: stderr, Environment: []string{"HIPPO_ROOT=" + driver.leaseRoot},
	}).Run(context.Background(), []string{historyCommandName, sinceFlagName, "30d", "--source", hippoFixtureName, jsonFlag})
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()

	return err
}

func (driver *Driver) requireFilteredHistoryV05() error {
	if driver.exitCode != 0 {
		return errors.New(driver.errorOutput)
	}
	if !strings.Contains(driver.output, `"runId":"hippo"`) || strings.Contains(driver.output, `"runId":"rhino"`) {
		return fmt.Errorf("history filter mismatch: %s", driver.output)
	}
	for _, forbidden := range []string{"command", "argument", "workingDirectory", driver.leaseRoot} {
		if strings.Contains(driver.output, forbidden) {
			return fmt.Errorf("history exposed forbidden %q: %s", forbidden, driver.output)
		}
	}

	return nil
}

func (driver *Driver) stableWatchV05() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	driver.leaseRoot = root
	base := time.Now().UTC()
	driver.samples = []policy.Sample{healthySample(base), healthySample(base)}

	return nil
}

func (driver *Driver) jsonWatchV05() error {
	ctx, interrupt := context.WithCancelCause(context.Background())
	defer interrupt(nil)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	sleeps := 0
	code, err := (cli.Application{
		Stdout: stdout, Stderr: stderr, Environment: []string{"HIPPO_ROOT=" + driver.leaseRoot},
		Collector: &sequenceCollector{samples: driver.samples},
		Sleep: func(time.Duration) {
			sleeps++
			if sleeps == 2 {
				interrupt(status.Interruption{Signal: syscall.SIGINT})
			}
		},
	}).Run(ctx, []string{"watch", jsonFlag, "--interval", "1s"})
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()

	return err
}

func (driver *Driver) requireJSONWatchV05() error {
	if driver.exitCode != 130 || driver.errorOutput != "" {
		return fmt.Errorf("watch exit=%d stderr=%s, want 130 and no diagnostic", driver.exitCode, driver.errorOutput)
	}
	trimmed := strings.TrimSpace(driver.output)
	if strings.Count(trimmed, "\n") != 0 || !strings.Contains(trimmed, `"schemaVersion":5`) {
		return fmt.Errorf("unexpected watch snapshots: %q", driver.output)
	}

	return nil
}

func requireEmergencyPressureV05(root string) error {
	transactional, err := acquireV04Victim(root, policy.TaskTransactional, 25_001)
	if err != nil {
		return err
	}
	defer func() { _ = guard.ReleaseReservation(root, transactional) }()
	service, err := acquireV04Victim(root, policy.TaskService, 25_002)
	if err != nil {
		return err
	}
	defer func() { _ = guard.ReleaseReservation(root, service) }()
	ephemeral, err := acquireV04Victim(root, policy.TaskEphemeral, 25_003)
	if err != nil {
		return err
	}
	defer func() { _ = guard.ReleaseReservation(root, ephemeral) }()

	victim, selected, err := guard.SelectEmergencyPressureVictim(root, guard.CapacityDeferredExitCode)
	if err != nil || !selected || victim.Token != ephemeral.Token {
		return fmt.Errorf("emergency ephemeral victim=%+v selected=%v error=%w", victim, selected, err)
	}
	if err = guard.ReleaseReservation(root, ephemeral); err != nil {
		return err
	}
	victim, selected, err = guard.SelectEmergencyPressureVictim(root, guard.CapacityDeferredExitCode)
	if err != nil || !selected || victim.Token != service.Token {
		return fmt.Errorf("emergency service victim=%+v selected=%v error=%w", victim, selected, err)
	}
	if err = guard.ReleaseReservation(root, service); err != nil {
		return err
	}
	if _, selected, err = guard.SelectPressureVictim(root, guard.CapacityDeferredExitCode); err != nil || selected {
		return fmt.Errorf("ordinary pressure selected transaction: selected=%v error=%w", selected, err)
	}
	victim, selected, err = guard.SelectEmergencyPressureVictim(root, guard.CapacityDeferredExitCode)
	if err != nil || !selected || victim.Token != transactional.Token {
		return fmt.Errorf("emergency transaction victim=%+v selected=%v error=%w", victim, selected, err)
	}

	return runInternalGuardRegressionV04("TestRunEmergencyTransactionalStopWritesReceiptWithoutRetry")
}

// corruptHistoryArchiveBytes is what the corrupt archive holds, so the
// assertion can prove the query left it exactly as it was.
const (
	corruptHistoryArchiveBytes = "not gzip"
	historyArchiveDirectory    = "history"
)

func (driver *Driver) corruptHistoryArchive() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	driver.leaseRoot = root
	if err = os.Mkdir(filepath.Join(root, historyArchiveDirectory), 0o700); err != nil {
		return err
	}
	archive := time.Now().UTC().AddDate(0, 0, -1).Format(time.DateOnly) + ".jsonl.gz"

	return os.WriteFile(filepath.Join(root, historyArchiveDirectory, archive), []byte(corruptHistoryArchiveBytes), 0o600)
}

func (driver *Driver) queryHistory() error {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code, _ := (cli.Application{
		Stdout: stdout, Stderr: stderr, Environment: []string{"HIPPO_ROOT=" + driver.leaseRoot},
	}).Run(context.Background(), []string{historyCommandName, sinceFlagName, "30d"})
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()

	return nil
}

func (driver *Driver) requireUnreadableHistory() error {
	if driver.exitCode != status.GuardFailed || !strings.Contains(driver.errorOutput, "hippo: [hippo.evidence.unreadable]") {
		return fmt.Errorf("exit=%d stderr=%q", driver.exitCode, driver.errorOutput)
	}
	archives, err := filepath.Glob(filepath.Join(driver.leaseRoot, historyArchiveDirectory, "*.jsonl.gz"))
	if err != nil || len(archives) != 1 {
		return fmt.Errorf("archives=%v error=%w", archives, err)
	}
	data, err := os.ReadFile(archives[0])
	if err != nil || string(data) != corruptHistoryArchiveBytes {
		return fmt.Errorf("the query changed the corrupt archive: %q error=%w", data, err)
	}

	return nil
}
