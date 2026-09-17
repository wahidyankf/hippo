package cli //nolint:testpackage // Application dependencies are injected at the CLI boundary.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/policy"
)

func TestStatusExposesLiveExclusiveOwnerWithoutMutation(t *testing.T) {
	root := t.TempDir()
	owner, err := guard.AcquireSession(context.Background(), root, "", policy.TaskEphemeral, time.Second)
	if err != nil || owner == nil {
		t.Fatalf("acquire exclusive owner: session=%v error=%v", owner != nil, err)
	}
	defer func() { _ = guard.ReleaseSession(root, owner) }()
	paths := []string{
		filepath.Join(root, "coordination-mode.json"),
		filepath.Join(root, "heavy.lock", "owner.json"),
		owner.RecordPath,
	}
	before := make(map[string][]byte, len(paths))
	for _, path := range paths {
		before[path], err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
	}

	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	stdout := &bytes.Buffer{}
	application := Application{
		Stdout: stdout, Stderr: &bytes.Buffer{}, Environment: []string{"HIPPO_ROOT=" + root},
		Collector: stableCollector{sample: stableDevelopmentSample(now)}, Now: func() time.Time { return now },
		Sleep: func(time.Duration) {},
	}
	code, statusError := application.Run(context.Background(), []string{"status", "--json"})
	if code != 0 || statusError != nil {
		t.Fatalf("status code=%d error=%v", code, statusError)
	}
	var payload struct {
		Coordination guard.ReservationTotals `json:"coordination"`
	}
	if err = json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	totals := payload.Coordination
	if totals.Mode != "exclusive" || totals.ActiveOwners != 1 || totals.Ephemeral != 1 ||
		totals.LegacyEntries != 1 || len(totals.Owners) != 1 || !totals.Owners[0].Legacy {
		t.Fatalf("exclusive status did not expose owner: %+v", totals)
	}
	for path, original := range before {
		after, readError := os.ReadFile(path)
		if readError != nil || !bytes.Equal(original, after) {
			t.Fatalf("status changed %s: error=%v", filepath.Base(path), readError)
		}
	}
}

func TestStatusClassifiesExclusiveCompatibilityState(t *testing.T) {
	for _, testCase := range []struct {
		name, contents string
		exitCode       int
	}{
		{name: "malformed", contents: `{"schemaVersion":1`, exitCode: 1},
		{name: "future", contents: `{"schemaVersion":2}`, exitCode: policy.ProtocolMismatchExitCode},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			owner, err := guard.AcquireSession(context.Background(), root, "", policy.TaskService, time.Second)
			if err != nil || owner == nil {
				t.Fatalf("acquire exclusive owner: session=%v error=%v", owner != nil, err)
			}
			defer func() { _ = guard.ReleaseSession(root, owner) }()
			contents := []byte(testCase.contents + "\n")
			if err = os.WriteFile(owner.RecordPath, contents, 0o600); err != nil {
				t.Fatal(err)
			}

			now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
			application := Application{
				Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Environment: []string{"HIPPO_ROOT=" + root},
				Collector: stableCollector{sample: stableDevelopmentSample(now)}, Now: func() time.Time { return now },
				Sleep: func(time.Duration) {},
			}
			code, statusError := application.Run(context.Background(), []string{"status", "--json"})
			if code != testCase.exitCode || statusError == nil {
				t.Fatalf("status code=%d want=%d error=%v", code, testCase.exitCode, statusError)
			}
			after, readError := os.ReadFile(owner.RecordPath)
			if readError != nil || !bytes.Equal(contents, after) {
				t.Fatalf("status changed invalid state: %q error=%v", after, readError)
			}
		})
	}
}

const adaptiveConfigFixture = `{
  "schemaVersion": 3,
  "coordination": {
    "mode": "reservation",
    "maxCpu": 8,
    "maxMemoryMiB": 16384,
    "baseActiveOwners": 2,
    "maxActiveOwners": 3,
    "promotion": {
      "completedRuns": 25,
      "minimumSources": 3,
      "minimumAvailableMemoryMiB": 10240,
      "maximumCpuP95Percent": 75
    },
    "emergencyAvailableMemoryMiB": 6144,
    "tiers": {
      "light": {"minimumCpu": 1, "maximumCpu": 2, "minimumMemoryMiB": 1024, "maximumMemoryMiB": 2048, "queueDeadline": "30m"},
      "standard": {"minimumCpu": 2, "maximumCpu": 4, "minimumMemoryMiB": 3072, "maximumMemoryMiB": 6144, "queueDeadline": "90m"},
      "heavy": {"minimumCpu": 4, "maximumCpu": 8, "minimumMemoryMiB": 8192, "maximumMemoryMiB": 16384, "queueDeadline": "4h"}
    }
  }
}`

func stableDevelopmentSample(now time.Time) policy.Sample {
	available, disk, cpu, pressure := 20*policy.GiB, 100*policy.GiB, 20.0, 1

	return policy.Sample{
		SchemaVersion: 3, MeasuredAt: now.UTC().Format(time.RFC3339Nano), Platform: "darwin",
		Capabilities: []string{"memory-pressure"}, EffectiveMemoryLimitBytes: 32 * policy.GiB,
		PhysicalMemoryBytes: 32 * policy.GiB, AvailableMemoryBytes: &available,
		AvailableParallelism: 8, CPUUtilizationPercent: &cpu, MemoryPressureLevel: &pressure,
		DiskFreeBytes: &disk, SwapState: "idle",
	}
}

func writeAdaptiveCLIConfig(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "hippo.machine.json")
	if err := os.WriteFile(path, []byte(adaptiveConfigFixture), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestSchemaThreeRefusesLiveLegacyOwnerWithProtocolMismatch(t *testing.T) {
	root := t.TempDir()
	plan := guard.ReservationPlan{
		Capacity:  guard.ReservationVector{CPU: 8, MemoryBytes: 16 * policy.GiB},
		Requested: guard.ReservationVector{CPU: 1, MemoryBytes: policy.GiB},
		Allocated: guard.ReservationVector{CPU: 1, MemoryBytes: policy.GiB},
	}
	owner, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, "balanced", "", plan, 20, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = guard.ReleaseReservation(root, owner) }()

	workingDirectory := t.TempDir()
	identityPath := filepath.Join(workingDirectory, "hippo.identity.json")
	if err = os.WriteFile(identityPath, []byte(`{"schemaVersion":1,"source":"cli-test"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	childMarker := filepath.Join(root, "child-started")
	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	stderr := &bytes.Buffer{}
	application := Application{
		Stdout: &bytes.Buffer{}, Stderr: stderr,
		Environment: []string{"HIPPO_ROOT=" + root, "CHILD_MARKER=" + childMarker},
		Collector:   stableCollector{sample: stableDevelopmentSample(now)}, Now: func() time.Time { return now },
	}
	code, runError := application.Run(context.Background(), []string{
		"run", "--config", writeAdaptiveCLIConfig(t, workingDirectory), "--cwd", workingDirectory,
		"--resource-tier", "light", "--", "/bin/sh", "-c", `printf started > "$CHILD_MARKER"`,
	})
	if code != policy.ProtocolMismatchExitCode || runError == nil || !strings.Contains(runError.Error(), "legacy") {
		t.Fatalf("code=%d stderr=%q error=%v", code, stderr.String(), runError)
	}
	if _, statError := os.Stat(childMarker); !os.IsNotExist(statError) {
		t.Fatalf("schema-three mismatch started its child: %v", statError)
	}
	totals, statusError := guard.ReservationStatus(context.Background(), root)
	if statusError != nil || totals.ActiveOwners != 1 || totals.WaitingOwners != 0 {
		t.Fatalf("legacy owner changed: totals=%+v error=%v", totals, statusError)
	}
}

func TestStatusReturnsProtocolMismatchForFutureLedger(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "coordination-mode.json"), []byte("{\"schemaVersion\":1,\"mode\":\"reservation\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger := []byte("{\"schemaVersion\":3,\"capacity\":{\"cpu\":0,\"memoryBytes\":0},\"nextSequence\":0,\"owners\":[],\"waiters\":[]}\n")
	ledgerPath := filepath.Join(root, "reservations.json")
	if err := os.WriteFile(ledgerPath, ledger, 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	application := Application{
		Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Environment: []string{"HIPPO_ROOT=" + root},
		Collector: stableCollector{sample: stableDevelopmentSample(now)}, Now: func() time.Time { return now },
		Sleep: func(time.Duration) {},
	}
	code, statusError := application.Run(context.Background(), []string{"status", "--json"})
	if code != policy.ProtocolMismatchExitCode || statusError == nil {
		t.Fatalf("code=%d error=%v", code, statusError)
	}
	after, readError := os.ReadFile(ledgerPath)
	if readError != nil || !bytes.Equal(after, ledger) {
		t.Fatalf("future ledger changed: %q error=%v", after, readError)
	}
}
