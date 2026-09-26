package cli //nolint:testpackage // Interruption is injected through the application's own context and seams.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/evidence"
	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/policy"
	releaseguard "github.com/wahidyankf/hippo/internal/release"
	"github.com/wahidyankf/hippo/internal/status"
)

// interruptingCollector reports a stable host until its interruptAt-th
// collection, which cancels the invocation the way the process entry does for
// a signal and fails the way a collection cut short by cancellation fails.
type interruptingCollector struct {
	sample      policy.Sample
	calls       *int
	interruptAt int
	interrupt   context.CancelCauseFunc
	signal      syscall.Signal
	// failure is what the interrupted collection returns. A real probe killed
	// by the cancelled context fails with its own error rather than with the
	// context's, so both shapes must read as the caller's stop.
	failure error
}

func (collector interruptingCollector) Collect(ctx context.Context, previous policy.CPUState, _ string) (policy.Reading, error) {
	*collector.calls++
	if *collector.calls == collector.interruptAt {
		collector.interrupt(status.Interruption{Signal: collector.signal})
		if collector.failure != nil {
			return policy.Reading{}, collector.failure
		}

		return policy.Reading{}, ctx.Err()
	}

	return policy.Reading{CPUState: previous, Sample: collector.sample}, nil
}

func TestWatchInterruptedWhileSamplingExitsWithTheSignalStatus(t *testing.T) {
	// A signal that lands while watch collects a sample is the same request
	// to stop as one that lands between samples. It must not surface as
	// hippo failing: the caller asked for the stop.
	t.Parallel()

	now := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	ctx, interrupt := context.WithCancelCause(context.Background())
	defer interrupt(nil)
	calls := 0
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	application := Application{
		Stdout: stdout, Stderr: stderr, Environment: []string{"HIPPO_ROOT=" + t.TempDir()},
		// Each status snapshot collects twice, so the third collection is the
		// first sample of the second snapshot.
		Collector: interruptingCollector{
			sample: stableDevelopmentSample(now), calls: &calls, interruptAt: 3,
			interrupt: interrupt, signal: syscall.SIGINT,
		},
		Now: func() time.Time { return now }, Sleep: func(time.Duration) {},
	}
	code, err := application.Run(ctx, []string{"watch", "--json", "--interval", "1s"})
	if code != 130 || err != nil {
		t.Fatalf("watch interrupted while sampling exited %d with %v, want 130: %q", code, err, stderr.String())
	}
	if strings.Contains(stderr.String(), "hippo:") {
		t.Fatalf("an interrupted watch reported a hippo failure: %q", stderr.String())
	}
	if strings.Count(strings.TrimSpace(stdout.String()), "\n") != 0 || !strings.Contains(stdout.String(), `"schemaVersion":5`) {
		t.Fatalf("watch lost the snapshot it printed before the signal: %q", stdout.String())
	}
}

func TestWatchInterruptedBetweenSamplesExitsWithTheSignalStatus(t *testing.T) {
	// Between samples watch is only waiting. A signal there ends it the way a
	// shell reports any signal-ended process, never as a result.
	t.Parallel()

	now := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	ctx, interrupt := context.WithCancelCause(context.Background())
	defer interrupt(nil)
	sleeps := 0
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	application := Application{
		Stdout: stdout, Stderr: stderr, Environment: []string{"HIPPO_ROOT=" + t.TempDir()},
		Collector: stableCollector{sample: stableDevelopmentSample(now)}, Now: func() time.Time { return now },
		// The first pause is inside the status snapshot; the second is the
		// interval watch waits between snapshots.
		Sleep: func(time.Duration) {
			sleeps++
			if sleeps == 2 {
				interrupt(status.Interruption{Signal: syscall.SIGTERM})
			}
		},
	}
	code, err := application.Run(ctx, []string{"watch", "--json", "--interval", "1s"})
	if code != 143 || err != nil {
		t.Fatalf("watch interrupted between samples exited %d with %v, want 143: %q", code, err, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("an interrupted watch wrote a diagnostic: %q", stderr.String())
	}
}

func TestMonitorInterruptedWhileSamplingExitsWithTheSignalStatus(t *testing.T) {
	// monitor runs until it is stopped, exactly like watch, and a stop that
	// cuts a probe short must read as the stop and not as a monitoring fault.
	t.Parallel()

	now := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	ctx, interrupt := context.WithCancelCause(context.Background())
	defer interrupt(nil)
	calls := 0
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	application := Application{
		Stdout: stdout, Stderr: stderr, Environment: []string{"HIPPO_ROOT=" + t.TempDir()},
		Collector: interruptingCollector{
			sample: stableDevelopmentSample(now), calls: &calls, interruptAt: 2,
			interrupt: interrupt, signal: syscall.SIGINT,
			failure: errors.New("available memory estimate is unavailable"),
		},
		Now: func() time.Time { return now },
	}
	code, err := application.Run(ctx, []string{"monitor", "--interval", "1ms"})
	if code != 130 || err != nil {
		t.Fatalf("monitor interrupted while sampling exited %d with %v, want 130: %q", code, err, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("an interrupted monitor wrote a diagnostic: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "state=") {
		t.Fatalf("monitor lost the state it printed before the signal: %q", stdout.String())
	}
}

func TestReleaseMonitorEndsWithTheSignalStatusOnlyWhenASignalEndsIt(t *testing.T) {
	// release monitor without a duration runs until it is stopped and writes
	// its summary on the way out. Stopped by a signal, it reports that signal;
	// stopped by its own --duration-ms, it finished what it was asked to do.
	t.Parallel()

	for _, testCase := range []struct {
		name     string
		duration string
		signal   syscall.Signal
		want     int
	}{
		{name: "signal", duration: "0", signal: syscall.SIGTERM, want: 143},
		{name: "duration", duration: "1", want: 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			ctx, interrupt := context.WithCancelCause(context.Background())
			defer interrupt(nil)
			summaryWritten := false
			stderr := &bytes.Buffer{}
			application := Application{
				Stdout: &bytes.Buffer{}, Stderr: stderr, Environment: []string{"HIPPO_ROOT=" + root},
				Collector: stableCollector{},
				MonitorRelease: func(monitorContext context.Context, _ releaseguard.MonitorConfig) error {
					if testCase.signal != 0 {
						interrupt(status.Interruption{Signal: testCase.signal})
					}
					<-monitorContext.Done()
					summaryWritten = true

					return nil
				},
			}
			code, err := application.Run(ctx, []string{
				"release", "monitor", "--output", root + "/raw.jsonl", "--summary", root + "/summary.json",
				"--deployment-root", root, "--health-url", "http://127.0.0.1:9/health",
				"--routed-origin", "https://example.test", "--duration-ms", testCase.duration,
			})
			if code != testCase.want || err != nil || stderr.String() != "" {
				t.Fatalf("release monitor ended by %s exited %d with %v, want %d: %q",
					testCase.name, code, err, testCase.want, stderr.String())
			}
			if !summaryWritten {
				t.Fatal("release monitor did not finish its capture before exiting")
			}
		})
	}
}

func TestInterruptedQueuedRunExitsWithTheSignalStatusAndKeepsItsReceipt(t *testing.T) {
	// A run still waiting in the reservation FIFO never started its child. A
	// signal there is the caller cancelling work that never began, so it gets
	// the signal status rather than hippo's own failure, and the never-started
	// receipt still says the payload did not run.
	root := t.TempDir()
	workingDirectory := t.TempDir()
	// Two small owners fill both the ledger and the owner limit, so the run
	// under test can only queue behind them.
	for _, source := range []string{"first-holder", "second-holder"} {
		holder, err := guard.AcquireReservationWithOptions(
			context.Background(), root, "", policy.TaskEphemeral, "balanced", "",
			guard.ReservationPlan{
				Capacity:  guard.ReservationVector{CPU: 2, MemoryBytes: 2 * policy.GiB},
				Requested: guard.ReservationVector{CPU: 1, MemoryBytes: policy.GiB},
				Allocated: guard.ReservationVector{CPU: 1, MemoryBytes: policy.GiB},
			},
			3, 0, guard.ReservationAdmissionOptions{Metadata: guard.ReservationMetadata{Source: source, Tier: "light"}},
		)
		if err != nil || holder == nil {
			t.Fatalf("hold the reservation capacity: session=%v error=%v", holder != nil, err)
		}
		t.Cleanup(func() { _ = guard.ReleaseReservation(root, holder) })
	}

	childMarker := filepath.Join(root, "child-started")
	now := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	ctx, interrupt := context.WithCancelCause(context.Background())
	defer interrupt(nil)
	stderr := &bytes.Buffer{}
	application := Application{
		Stdout: &bytes.Buffer{}, Stderr: stderr,
		Environment: []string{"HIPPO_ROOT=" + root, "CHILD_MARKER=" + childMarker},
		Collector:   stableCollector{sample: stableDevelopmentSample(now)}, Now: func() time.Time { return now },
		// The first pause is the queued waiter's retry wait.
		Sleep: func(time.Duration) { interrupt(status.Interruption{Signal: syscall.SIGINT}) },
	}
	code, runError := application.Run(ctx, []string{
		"run", "--config", writeAdaptiveCLIConfig(t, workingDirectory), "--cwd", workingDirectory,
		"--source", "waiter", "--resource-tier", "light",
		"--", "/bin/sh", "-c", `printf started > "$CHILD_MARKER"`,
	})
	if code != 130 || runError != nil {
		t.Fatalf("an interrupted queued run exited %d with %v, want 130: %q", code, runError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "HIPPO queued run=") {
		t.Fatalf("the run was never queued, so this is not the waiter's path: %q", stderr.String())
	}
	if strings.Contains(stderr.String(), "hippo:") {
		t.Fatalf("an interrupted waiter reported a hippo failure: %q", stderr.String())
	}
	if _, statError := os.Stat(childMarker); !os.IsNotExist(statError) {
		t.Fatalf("the interrupted waiter started its child: %v", statError)
	}
	requireNeverStartedReceipt(t, root, "waiter", "admission-cancelled")
	// A waiter collects no host evidence while it queues, so it has no
	// lifetime to summarize; the receipt is its whole record.
	if outcomes := summaryOutcomes(t, root); len(outcomes) != 0 {
		t.Fatalf("a waiter that collected no evidence wrote summaries: %v", outcomes)
	}
}

func TestInterruptedHostAdmissionWaitKeepsANeverStartedReceipt(t *testing.T) {
	// After the reservation is granted, the guard still samples the host until
	// admission is safe. A signal during that wait also cancels work that never
	// started, and it must leave the same never-started receipt a queued waiter
	// leaves, so a caller reading receipts can requeue it once.
	root := t.TempDir()
	childMarker := filepath.Join(root, "child-started")
	now := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	ctx, interrupt := context.WithCancelCause(context.Background())
	defer interrupt(nil)
	stderr := &bytes.Buffer{}
	application := Application{
		Stdout: &bytes.Buffer{}, Stderr: stderr,
		Environment: []string{"HIPPO_ROOT=" + root, "CHILD_MARKER=" + childMarker},
		Collector:   stableCollector{sample: stableDevelopmentSample(now)}, Now: func() time.Time { return now },
		// Admission needs consecutive samples, so the first pause is the wait
		// between the first and second host sample.
		Sleep: func(time.Duration) { interrupt(status.Interruption{Signal: syscall.SIGTERM}) },
	}
	code, runError := application.Run(ctx, []string{
		"run", "--source", "sampler", "--disk-path", root,
		"--", "/bin/sh", "-c", `printf started > "$CHILD_MARKER"`,
	})
	if code != 143 || runError != nil {
		t.Fatalf("an interrupted admission wait exited %d with %v, want 143: %q", code, runError, stderr.String())
	}
	if _, statError := os.Stat(childMarker); !os.IsNotExist(statError) {
		t.Fatalf("the interrupted admission wait started its child: %v", statError)
	}
	requireNeverStartedReceipt(t, root, "sampler", "admission-cancelled")
	// The samples it took are summarized under the outcome its receipt names:
	// a cancellation, not a capacity deferral.
	if outcomes := summaryOutcomes(t, root); len(outcomes) != 1 || outcomes["sampler"] != "admission-cancelled" {
		t.Fatalf("the interrupted admission wait summarized as %v, want sampler=admission-cancelled", outcomes)
	}
}

// summaryOutcomes reads the lifetime summaries under root, by source.
func summaryOutcomes(t *testing.T, root string) map[string]string {
	t.Helper()

	rows, err := evidence.ReadHistory(root, evidence.Query{})
	if err != nil {
		t.Fatal(err)
	}
	outcomes := map[string]string{}
	for _, row := range rows {
		outcomes[row.Source] = row.Outcome
	}

	return outcomes
}

// requireNeverStartedReceipt finds the one receipt source wrote and checks it
// records a run that never started, for the given reason.
func requireNeverStartedReceipt(t *testing.T, root, source, reason string) {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(root, "receipts"))
	if err != nil {
		t.Fatalf("no receipts were written: %v", err)
	}
	for _, entry := range entries {
		data, readError := os.ReadFile(filepath.Join(root, "receipts", entry.Name()))
		if readError != nil {
			t.Fatal(readError)
		}
		var receipt guard.SafetyReceipt
		if json.Unmarshal(data, &receipt) != nil || receipt.Source != source {
			continue
		}
		if receipt.State != "never-started" || receipt.Reason != reason {
			t.Fatalf("receipt %s records %s/%s, want never-started/%s", entry.Name(), receipt.State, receipt.Reason, reason)
		}

		return
	}
	t.Fatalf("no receipt from %s among %d entries", source, len(entries))
}
