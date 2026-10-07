package cli //nolint:testpackage // Application dependencies are injected at the CLI boundary.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	resourceconfig "github.com/wahidyankf/hippo/internal/adapters/config"
	"github.com/wahidyankf/hippo/internal/adapters/evidence"
	"github.com/wahidyankf/hippo/internal/adapters/health"
	"github.com/wahidyankf/hippo/tests/support/runtimewiring"

	guard "github.com/wahidyankf/hippo/internal/adapters/runtime"
	app "github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

func TestStatusExposesLiveExclusiveOwnerWithoutMutation(t *testing.T) {
	root := t.TempDir()
	owner, err := runtimewiring.AcquireSession(context.Background(), root, "", policy.TaskEphemeral, time.Second)
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
	application := wiredApplication(Application{
		Stdout: stdout, Stderr: &bytes.Buffer{}, Environment: []string{"HIPPO_ROOT=" + root},
		Collector: stableCollector{sample: stableDevelopmentSample(now)}, Now: func() time.Time { return now },
		Sleep: func(time.Duration) {},
	})
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
	// Both are refusals before anything is started, so both exit 125. What
	// tells them apart is the reason, which is the layer that exists to carry
	// exactly this distinction.
	for _, testCase := range []struct {
		name, contents string
		exitCode       int
		code           status.Code
	}{
		{
			name: "malformed", contents: `{"schemaVersion":1`, exitCode: status.GuardFailed,
			code: status.CodeSupervisionFailed,
		},
		{
			name: "future", contents: `{"schemaVersion":2}`, exitCode: status.GuardFailed,
			code: status.CodeCoordinationProtocolMismatch,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			owner, err := runtimewiring.AcquireSession(context.Background(), root, "", policy.TaskService, time.Second)
			if err != nil || owner == nil {
				t.Fatalf("acquire exclusive owner: session=%v error=%v", owner != nil, err)
			}
			defer func() { _ = guard.ReleaseSession(root, owner) }()
			contents := []byte(testCase.contents + "\n")
			if err = os.WriteFile(owner.RecordPath, contents, 0o600); err != nil {
				t.Fatal(err)
			}

			now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
			application := wiredApplication(Application{
				Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Environment: []string{"HIPPO_ROOT=" + root},
				Collector: stableCollector{sample: stableDevelopmentSample(now)}, Now: func() time.Time { return now },
				Sleep: func(time.Duration) {},
			})
			code, statusError := application.Run(context.Background(), []string{"status", "--json"})
			if code != testCase.exitCode || statusError == nil {
				t.Fatalf("status code=%d want=%d error=%v", code, testCase.exitCode, statusError)
			}
			var failure status.Failure
			if !errors.As(statusError, &failure) || failure.Code != testCase.code {
				t.Fatalf("status reason=%v want=%s", statusError, testCase.code)
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
	owner, err := runtimewiring.AcquireReservation(
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
	application := wiredApplication(Application{
		Stdout: &bytes.Buffer{}, Stderr: stderr,
		Environment: []string{"HIPPO_ROOT=" + root, "CHILD_MARKER=" + childMarker},
		Collector:   stableCollector{sample: stableDevelopmentSample(now)}, Now: func() time.Time { return now },
	})
	code, runError := application.Run(context.Background(), []string{
		"run", "--config", writeAdaptiveCLIConfig(t, workingDirectory), "--cwd", workingDirectory,
		"--resource-tier", "light", "--", "/bin/sh", "-c", `printf started > "$CHILD_MARKER"`,
	})
	if code != status.GuardFailed || runError == nil || !strings.Contains(runError.Error(), "legacy") {
		t.Fatalf("code=%d stderr=%q error=%v", code, stderr.String(), runError)
	}
	if !strings.Contains(runError.Error(), string(status.CodeCoordinationProtocolMismatch)) {
		t.Fatalf("the refusal does not name the protocol mismatch: %v", runError)
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
	application := wiredApplication(Application{
		Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Environment: []string{"HIPPO_ROOT=" + root},
		Collector: stableCollector{sample: stableDevelopmentSample(now)}, Now: func() time.Time { return now },
		Sleep: func(time.Duration) {},
	})
	code, statusError := application.Run(context.Background(), []string{"status", "--json"})
	if code != status.GuardFailed || statusError == nil {
		t.Fatalf("code=%d error=%v", code, statusError)
	}
	var failure status.Failure
	if !errors.As(statusError, &failure) || failure.Code != status.CodeCoordinationProtocolMismatch {
		t.Fatalf("the refusal does not name the protocol mismatch: %v", statusError)
	}
	after, readError := os.ReadFile(ledgerPath)
	if readError != nil || !bytes.Equal(after, ledger) {
		t.Fatalf("future ledger changed: %q error=%v", after, readError)
	}
}

// escalatingCollector reports a healthy host until admission, then critical
// memory pressure, so the guard admits its child and must shed it.
type escalatingCollector struct {
	calls   *int
	healthy int
}

func (collector escalatingCollector) Collect(_ context.Context, previous policy.CPUState, _ string) (policy.Reading, error) {
	*collector.calls++
	sample := policy.Sample{
		SchemaVersion: 3, MeasuredAt: time.Now().UTC().Format(time.RFC3339Nano), Platform: "darwin",
		Capabilities:              []string{"compressor", "memory-pressure", "swap"},
		EffectiveMemoryLimitBytes: 32 * policy.GiB, PhysicalMemoryBytes: 32 * policy.GiB,
		AvailableMemoryBytes: new(12 * policy.GiB), AvailableNonCompressedEstimateBytes: new(12 * policy.GiB),
		MemoryPressureLevel: new(1), CompressorAvailable: new(true), CompressorPayloadBytes: new(7 * policy.GiB),
		AvailableParallelism: 8, CPUUtilizationPercent: new(20.0),
		DiskFreeBytes: new(40 * policy.GiB), DiskTotalBytes: new(512 * policy.GiB), PageSizeBytes: new(int64(16_384)),
		SwapIns: new(int64(10)), SwapOuts: new(int64(20)), SwapFreeBytes: new(2 * policy.GiB), SwapState: "idle",
	}
	if *collector.calls > collector.healthy {
		critical := 4
		sample.MemoryPressureLevel = &critical
	}

	return policy.Reading{CPUState: previous, Sample: sample}, nil
}

func TestPressureShedNamesItsOwnReason(t *testing.T) {
	// Host pressure that stops a started child is a different event from a
	// capacity deferral that never started one. Both are limits and return
	// 124, but the reason is what a consumer branches on, so a shed must not
	// name the deferral.
	root := t.TempDir()
	stderr := &bytes.Buffer{}
	calls := 0
	application := wiredApplication(Application{
		Stdout: &bytes.Buffer{}, Stderr: stderr,
		Environment: []string{"HIPPO_ROOT=" + root},
		Collector:   escalatingCollector{calls: &calls, healthy: 4},
	})
	code, runError := application.Run(context.Background(), []string{
		"run", "--disk-path", root, "--", "/bin/sh", "-c", "sleep 10",
	})
	if code != status.LimitShed || runError != nil {
		t.Fatalf("code=%d error=%v stderr=%q", code, runError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "hippo: ["+string(status.CodeLimitPressureShed)+"]") {
		t.Fatalf("a pressure shed did not name %s: %q", status.CodeLimitPressureShed, stderr.String())
	}
	if strings.Contains(stderr.String(), string(status.CodeLimitCapacityDeferred)) {
		t.Fatalf("a pressure shed named the capacity deferral: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "the payload ran") {
		t.Fatalf("a pressure shed did not say its payload ran: %q", stderr.String())
	}
}

func TestSchemaTwoRejectsATierCombinedWithAnAdmissionWait(t *testing.T) {
	// A tier carries its own queue deadline. Under schema 2 the wait flag used
	// to be dropped silently when a tier was given, so the caller waited for the
	// tier's deadline instead of the one it asked for. Schema 3 already refuses
	// the combination; schema 2 must refuse it the same way, before enqueue.
	root := t.TempDir()
	workingDirectory := t.TempDir()
	configPath := filepath.Join(workingDirectory, "hippo.local.json")
	if err := os.WriteFile(configPath, []byte(`{"schemaVersion":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	childMarker := filepath.Join(root, "child-started")
	now := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	stderr := &bytes.Buffer{}
	application := wiredApplication(Application{
		Stdout: &bytes.Buffer{}, Stderr: stderr,
		Environment: []string{"HIPPO_ROOT=" + root, "CHILD_MARKER=" + childMarker},
		Collector:   stableCollector{sample: stableDevelopmentSample(now)}, Now: func() time.Time { return now },
		Sleep: func(time.Duration) {},
	})
	code, runError := application.Run(context.Background(), []string{
		"run", "--config", configPath, "--cwd", workingDirectory, "--disk-path", root,
		"--resource-tier", "light", "--wait-for-admission", "3s",
		"--", "/bin/sh", "-c", `printf started > "$CHILD_MARKER"`,
	})
	if code != status.CallerError {
		t.Fatalf("a tier with an admission wait was not a usage error: code=%d error=%v stderr=%q",
			code, runError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "hippo: ["+string(status.CodeArgsInvalid)+"]") ||
		!strings.Contains(stderr.String(), "--resource-tier") {
		t.Fatalf("the refusal did not name the argument mistake: %q", stderr.String())
	}
	if _, statError := os.Stat(childMarker); !os.IsNotExist(statError) {
		t.Fatalf("the refused run started its child: %v", statError)
	}
	totals, statusError := guard.ReservationStatus(context.Background(), root)
	if statusError != nil || totals.ActiveOwners != 0 || totals.WaitingOwners != 0 {
		t.Fatalf("the refused run touched the ledger: totals=%+v error=%v", totals, statusError)
	}
}

// statusProfile is the decision and exit code status JSON publishes for the
// resolved profile. The exit code stays an integer in the JSON: it is the
// number v0.8.4 published, whatever type the resolution holds it as.
type statusProfile struct {
	Decision  string `json:"decision"`
	ExitCode  int    `json:"exitCode"`
	Retryable bool   `json:"retryable"`
}

func TestStatusJSONKeepsEachResolutionsV084DecisionAndExitCode(t *testing.T) {
	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name      string
		sample    func(*policy.Sample)
		decision  string
		exitCode  int
		retryable bool
		// strictProfile asks for a configured profile that refuses every fallback.
		strictProfile bool
	}{
		{name: "normal pressure", sample: func(*policy.Sample) {}, decision: "run", exitCode: 0},
		{
			name: "a stable warning", decision: "wait", exitCode: 75, retryable: true,
			sample: func(sample *policy.Sample) { sample.MemoryPressureLevel = new(2) },
		},
		{
			name: "free disk below the 256 MiB floor", decision: "cleanup", exitCode: 73,
			sample: func(sample *policy.Sample) { sample.DiskFreeBytes = new(200 * policy.MiB) },
		},
		{
			// 10 GiB is below DefaultPolicy's 30 GiB disk reserve and above the one the
			// resolved profile derives, so only the resolution's own policy admits it.
			name: "free disk between the resolved profile's reserve and the default policy's", decision: "run", exitCode: 0,
			sample: func(sample *policy.Sample) { sample.DiskFreeBytes = new(10 * policy.GiB) },
		},
		{
			name: "a strict profile that does not fit", decision: "replan", exitCode: 78, strictProfile: true,
			sample: func(sample *policy.Sample) { sample.AvailableMemoryBytes = new(policy.GiB) },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			sample := stableDevelopmentSample(now)
			test.sample(&sample)
			stdout := &bytes.Buffer{}
			application := wiredApplication(Application{
				Stdout: stdout, Stderr: &bytes.Buffer{}, Environment: []string{"HIPPO_ROOT=" + t.TempDir()},
				Collector: stableCollector{sample: sample}, Now: func() time.Time { return now },
				Sleep: func(time.Duration) {},
			})
			arguments := []string{"status", "--json"}
			if test.strictProfile {
				configPath := filepath.Join(t.TempDir(), "hippo.local.json")
				strict := `{"schemaVersion":1,"defaultProfile":"local","profiles":{"local":{"extends":"constrained","strict":true}}}`
				if err := os.WriteFile(configPath, []byte(strict), 0o600); err != nil {
					t.Fatal(err)
				}
				arguments = append(arguments, "--config", configPath)
			}
			code, err := application.Run(context.Background(), arguments)
			var payload struct {
				Profile statusProfile `json:"profile"`
			}
			if code != 0 || err != nil || json.Unmarshal(stdout.Bytes(), &payload) != nil ||
				payload.Profile != (statusProfile{Decision: test.decision, ExitCode: test.exitCode, Retryable: test.retryable}) {
				t.Fatalf("status code=%d error=%v profile=%+v, want %s with exit code %d", code, err, payload.Profile, test.decision, test.exitCode)
			}
		})
	}
}

// A coordination mode the configuration does not accept is an unreadable configuration, wherever the document is
// refused: exit 125 naming hippo.config.unreadable. An empty mode is accepted, as it always was.
func TestStatusRefusesAnUnknownCoordinationModeAsAnUnreadableConfiguration(t *testing.T) {
	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		mode    string
		refused bool
	}{
		{"exclusive", true}, {"batch", true}, {"Reservation", true}, {"", false}, {"reservation", false},
	} {
		t.Run(test.mode, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "hippo.machine.json")
			document := strings.Replace(adaptiveConfigFixture, `"mode": "reservation"`, `"mode": "`+test.mode+`"`, 1)
			if err := os.WriteFile(configPath, []byte(document), 0o600); err != nil {
				t.Fatal(err)
			}
			application := wiredApplication(Application{
				Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Environment: []string{"HIPPO_ROOT=" + t.TempDir()},
				Collector: stableCollector{sample: stableDevelopmentSample(now)}, Now: func() time.Time { return now },
				Sleep: func(time.Duration) {},
			})
			code, err := application.Run(context.Background(), []string{"status", "--json", "--config", configPath})
			if !test.refused {
				if code != 0 || err != nil {
					t.Fatalf("mode %q: status code=%d error=%v, want the configuration to load", test.mode, code, err)
				}

				return
			}
			var failure status.Failure
			if code != status.GuardFailed || !errors.As(err, &failure) || failure.Code != status.CodeConfigUnreadable ||
				!strings.Contains(err.Error(), strconv.Quote(test.mode)) {
				t.Fatalf("mode %q: status code=%d error=%v, want %d naming %s and the mode", test.mode, code, err, status.GuardFailed, status.CodeConfigUnreadable)
			}
		})
	}
}

func TestAResolutionPublishesTheIntegerV084CarriedForItsReason(t *testing.T) {
	for _, row := range []struct {
		reason policy.Reason
		want   string
	}{
		{policy.ReasonStorageBlocked, "73"}, {policy.ReasonCapacityDeferred, "75"}, {policy.ReasonReplanRequired, "78"},
	} {
		encoded, err := json.Marshal(policy.Resolution{Reason: row.reason})
		var published struct {
			ExitCode json.Number `json:"exitCode"`
		}
		if err != nil || json.Unmarshal(encoded, &published) != nil || published.ExitCode.String() != row.want {
			t.Errorf("reason %d publishes %s (%v), want exitCode %s", row.reason, encoded, err, row.want)
		}
	}
}

func TestStatusDecisionFollowsTheAdmissionPath(t *testing.T) {
	running := policy.Resolution{Decision: policy.DecisionRun}
	cleanup := policy.Resolution{Decision: policy.DecisionCleanup, Reason: policy.ReasonStorageBlocked}
	replan := policy.Resolution{Decision: policy.DecisionReplan, Reason: policy.ReasonReplanRequired}

	// Each row is one of status's published answers, as AC-14 lists them, and the
	// path that produces it. Degraded is a wait: under any warning decision stays wait.
	for _, test := range []struct {
		name       string
		resolution policy.Resolution
		path       policy.AdmissionPath
		want       statusProfile
	}{
		{"normal pressure", running, policy.AdmissionNormal, statusProfile{Decision: "run", ExitCode: 0}},
		{"a wait", running, policy.AdmissionWait, statusProfile{Decision: "wait", ExitCode: 75, Retryable: true}},
		{"a degraded admission", running, policy.AdmissionDegraded, statusProfile{Decision: "wait", ExitCode: 75, Retryable: true}},
		{"free disk below the floor", running, policy.AdmissionCleanup, statusProfile{Decision: "cleanup", ExitCode: 73}},
		{"a resolution already at cleanup", cleanup, policy.AdmissionCleanup, statusProfile{Decision: "cleanup", ExitCode: 73}},
		{"a strict profile that does not fit", replan, policy.AdmissionReplan, statusProfile{Decision: "replan", ExitCode: 78}},
	} {
		t.Run(test.name, func(t *testing.T) {
			decided, err := app.WithAssessmentDecision(test.resolution, test.path)
			encoded, encodeError := json.Marshal(decided)
			var published statusProfile
			if err != nil || encodeError != nil || json.Unmarshal(encoded, &published) != nil || published != test.want {
				t.Errorf("path %d publishes %+v (%v), want %+v", test.path, published, err, test.want)
			}
		})
	}
}

func TestStatusRefusesAnUnsetAdmissionPathInsteadOfDefaulting(t *testing.T) {
	resolution := policy.Resolution{Decision: policy.DecisionRun}
	decided, err := app.WithAssessmentDecision(resolution, policy.AdmissionUnset)
	if err == nil || decided.Decision != resolution.Decision || decided.Reason != policy.ReasonNone || decided.Retryable {
		t.Errorf("an unset path decided %+v (%v), want a refusal that leaves the resolution as it was", decided, err)
	}
}

// TestRunClassAcceptsOnlyTheClassesRunMayGuard pins what run's --class flag may name: an empty flag means ephemeral,
// the three classes run guards pass through, and release, which only the release commands guard, is the caller's
// mistake, as is any text that names no class.
func TestRunClassAcceptsOnlyTheClassesRunMayGuard(t *testing.T) {
	for _, test := range []struct {
		flag string
		want policy.TaskClass
	}{
		{"", policy.TaskEphemeral},
		{string(policy.TaskEphemeral), policy.TaskEphemeral},
		{string(policy.TaskService), policy.TaskService},
		{string(policy.TaskTransactional), policy.TaskTransactional},
	} {
		class, err := runClass(test.flag)
		if err != nil || class != test.want {
			t.Errorf("runClass(%q) = %q (%v), want %q", test.flag, class, err, test.want)
		}
	}
	for _, flag := range []string{string(policy.TaskRelease), "batch", "Ephemeral", " service"} {
		class, err := runClass(flag)
		var failure status.Failure
		if !errors.As(err, &failure) || failure.Code != status.CodeArgsInvalid || class != "" {
			t.Errorf("runClass(%q) = %q (%v), want a %s refusal and no class", flag, class, err, status.CodeArgsInvalid)
		}
	}
}

func wiredApplication(application Application) Application {
	if application.Stdin == nil {
		application.Stdin = os.Stdin
	}
	if application.Stdout == nil {
		application.Stdout = os.Stdout
	}
	if application.Stderr == nil {
		application.Stderr = os.Stderr
	}

	if application.Environment == nil {
		application.Environment = os.Environ()
	}

	if application.Now == nil {
		application.Now = time.Now
	}

	if application.Version == "" {
		application.Version = Version
	}
	if application.Commit == "" {
		application.Commit = Commit
	}
	if application.Collector == nil {
		application.Collector = defaultCollector(application.Version, application.Environment)
	}

	application.Configuration = resourceconfig.Provider{}
	engine := guard.NewEngine()
	application.RunServices = app.RunServices{Coordination: engine, Workload: engine, Ports: engine, Evidence: engine, Clock: guard.RunClock{}}
	application.ReleaseServices = app.ReleaseServices{Configuration: application.Configuration, Collector: application.Collector, Environment: environmentMap(application.Environment), Clock: guard.ReleaseClock{NowFunc: application.Now, Pause: application.Sleep}, Health: health.Probe{}, Evidence: evidence.ReleaseRepository{}}
	if application.MonitorRelease == nil {
		application.MonitorRelease = application.ReleaseServices.RunMonitor
	}
	application.Observer = app.ObservationServices{Configuration: application.Configuration, Collector: application.Collector, Runtime: engine, Evidence: evidence.HistoryRepository{}, Clock: guard.ObservationClock{NowFunc: application.Now, Pause: application.Sleep}, Environment: environmentMap(application.Environment)}
	application.RunEntry = app.RunEntryServices{Configuration: application.Configuration, Observation: application.Observer, Run: application.RunServices, Collector: application.Collector, Environment: application.Environment, Now: application.Now, Sleep: application.Sleep, Stdin: application.Stdin, Stdout: application.Stdout, Stderr: application.Stderr, PortLeaseRoot: resourceconfig.PortLeaseRoot(environmentMap(application.Environment))}
	return application
}
