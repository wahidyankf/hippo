package cli //nolint:testpackage // Host and evidence refusals are injected through the application's own seams.

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/host"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

// unreadableHost is the real collector with every host file and probe
// refused, the way an unreadable /proc entry or a denied sysctl fails.
func unreadableHost() host.SystemCollector {
	refused := func(path string) error { return &fs.PathError{Op: "open", Path: path, Err: syscall.EACCES} }

	return host.SystemCollector{
		ReadFile: func(path string) ([]byte, error) { return nil, refused(path) },
		Run:      func(_ context.Context, name string, _ ...string) ([]byte, error) { return nil, refused(name) },
	}
}

// readOnlyDirectory makes a directory hippo can list but not write, and
// restores it so the test's own cleanup can remove it.
func readOnlyDirectory(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o700) })
	if probe, err := os.CreateTemp(path, "probe"); err == nil {
		_ = probe.Close()
		t.Skip("this user can write a read-only directory, so a refused write cannot be staged")
	}
}

func requireReason(t *testing.T, code int, err error, stderr string, want status.Code) {
	t.Helper()
	if code != status.GuardFailed {
		t.Fatalf("exit %d with %v, want %d naming %s: %q", code, err, status.GuardFailed, want, stderr)
	}
	if !strings.Contains(stderr, "hippo: ["+string(want)+"]") {
		t.Fatalf("the failure does not name %s: %q", want, stderr)
	}
}

func TestStatusNamesAnUnreadableHost(t *testing.T) {
	t.Parallel()

	stderr := &bytes.Buffer{}
	application := Application{
		Stdout: &bytes.Buffer{}, Stderr: stderr, Environment: []string{"HIPPO_ROOT=" + t.TempDir()},
		Collector: unreadableHost(), Sleep: func(time.Duration) {},
	}
	code, err := application.Run(context.Background(), []string{"status", "--disk-path", t.TempDir()})
	requireReason(t, code, err, stderr.String(), status.CodeHostUnreadable)
}

func TestRunNamesAnUnreadableHostBeforeLaunch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	childMarker := filepath.Join(root, "child-started")
	stderr := &bytes.Buffer{}
	application := Application{
		Stdout: &bytes.Buffer{}, Stderr: stderr,
		Environment: []string{"HIPPO_ROOT=" + root, "CHILD_MARKER=" + childMarker},
		Collector:   unreadableHost(), Sleep: func(time.Duration) {},
	}
	code, err := application.Run(context.Background(), []string{
		"run", "--disk-path", root, "--", "/bin/sh", "-c", `printf started > "$CHILD_MARKER"`,
	})
	requireReason(t, code, err, stderr.String(), status.CodeHostUnreadable)
	if _, statError := os.Stat(childMarker); !os.IsNotExist(statError) {
		t.Fatalf("a run with an unreadable host started its child: %v", statError)
	}
}

// failsAfter reports a stable host for its first healthy collections, then
// fails the way the system collector reports a host it can no longer read.
type failsAfter struct {
	sample  policy.Sample
	healthy int
	calls   *int
}

func (collector failsAfter) Collect(_ context.Context, previous policy.CPUState, _ string) (policy.Reading, error) {
	*collector.calls++
	if *collector.calls > collector.healthy {
		return policy.Reading{}, status.Fail(status.CodeHostUnreadable, "collecting host evidence: read Linux memory: permission denied")
	}

	return policy.Reading{CPUState: previous, Sample: collector.sample}, nil
}

func TestReleaseCheckNamesAnUnreadableHostInsteadOfDeferring(t *testing.T) {
	// A host hippo cannot read is not a busy host. Reporting it as a capacity
	// deferral tells the caller to retry into the same unreadable host.
	t.Parallel()

	now := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	calls := 0
	stderr := &bytes.Buffer{}
	application := Application{
		Stdout: &bytes.Buffer{}, Stderr: stderr, Environment: []string{"HIPPO_ROOT=" + t.TempDir()},
		Collector: failsAfter{sample: stableDevelopmentSample(now), healthy: 1, calls: &calls},
		Now:       func() time.Time { return now }, Sleep: func(time.Duration) {},
	}
	code, err := application.Run(context.Background(), []string{"release", "check", "--disk-path", t.TempDir()})
	requireReason(t, code, err, stderr.String(), status.CodeHostUnreadable)
	if calls < 2 {
		t.Fatalf("the check never reached its consecutive samples: %d collections", calls)
	}
}

func TestRunNamesARefusedEvidenceRootBeforeLaunch(t *testing.T) {
	// A state root hippo cannot create is the evidence root refusing the write
	// hippo needs before it can admit anything.
	t.Parallel()

	parent := filepath.Join(t.TempDir(), "locked")
	readOnlyDirectory(t, parent)
	root := filepath.Join(parent, "state")
	childMarker := filepath.Join(t.TempDir(), "child-started")
	now := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	stderr := &bytes.Buffer{}
	application := Application{
		Stdout: &bytes.Buffer{}, Stderr: stderr,
		Environment: []string{"HIPPO_ROOT=" + root, "CHILD_MARKER=" + childMarker},
		Collector:   stableCollector{sample: stableDevelopmentSample(now)}, Now: func() time.Time { return now },
		Sleep: func(time.Duration) {},
	}
	code, err := application.Run(context.Background(), []string{
		"run", "--disk-path", parent, "--", "/bin/sh", "-c", `printf started > "$CHILD_MARKER"`,
	})
	requireReason(t, code, err, stderr.String(), status.CodeEvidenceUnwritable)
	if _, statError := os.Stat(childMarker); !os.IsNotExist(statError) {
		t.Fatalf("a run whose evidence root refused writes started its child: %v", statError)
	}
}

func TestARefusedReceiptOutranksTheSignalThatStoppedAQueuedRun(t *testing.T) {
	// The never-started receipt is what a caller reads before requeueing. If
	// the evidence root refuses it, hippo failed at something the caller relies
	// on, and saying only that the run was interrupted would hide that.
	root := t.TempDir()
	workingDirectory := t.TempDir()
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
	readOnlyDirectory(t, filepath.Join(root, "receipts"))

	now := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	ctx, interrupt := context.WithCancelCause(context.Background())
	defer interrupt(nil)
	stderr := &bytes.Buffer{}
	application := Application{
		Stdout: &bytes.Buffer{}, Stderr: stderr, Environment: []string{"HIPPO_ROOT=" + root},
		Collector: stableCollector{sample: stableDevelopmentSample(now)}, Now: func() time.Time { return now },
		Sleep: func(time.Duration) { interrupt(status.Interruption{Signal: syscall.SIGINT}) },
	}
	code, err := application.Run(ctx, []string{
		"run", "--config", writeAdaptiveCLIConfig(t, workingDirectory), "--cwd", workingDirectory,
		"--source", "waiter", "--resource-tier", "light", "--", "/bin/sh", "-c", "exit 0",
	})
	requireReason(t, code, err, stderr.String(), status.CodeEvidenceUnwritable)
}

func TestAHostLostAfterLaunchStaysASupervisionFailure(t *testing.T) {
	// hippo.host.unreadable says nothing was started. Once the child runs, a
	// host hippo can no longer read is hippo losing supervision of work that
	// began, and the caller must recover it rather than simply rerun it.
	t.Parallel()

	root := t.TempDir()
	childMarker := filepath.Join(root, "child-started")
	now := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	calls := 0
	stderr := &bytes.Buffer{}
	application := Application{
		Stdout: &bytes.Buffer{}, Stderr: stderr,
		Environment: []string{"HIPPO_ROOT=" + root, "CHILD_MARKER=" + childMarker},
		// One probe and the consecutive admission samples, then the host
		// stops answering while the child runs.
		Collector: failsAfter{sample: stableDevelopmentSample(now), healthy: policy.DefaultPolicy().ConsecutiveCPUSamples + 1, calls: &calls},
		Now:       func() time.Time { return now }, Sleep: func(time.Duration) {},
	}
	code, err := application.Run(context.Background(), []string{
		"run", "--disk-path", root, "--", "/bin/sh", "-c", `printf started > "$CHILD_MARKER"; sleep 30`,
	})
	requireReason(t, code, err, stderr.String(), status.CodeSupervisionFailed)
	if _, statError := os.Stat(childMarker); statError != nil {
		t.Fatalf("the child never started, so this is not the after-launch path: %v", statError)
	}
}

// hookedCollector reports a stable host and calls onCall with the number of
// each collection before answering it, so a test can change the world at a
// known point in admission sampling.
type hookedCollector struct {
	sample policy.Sample
	calls  *int
	onCall func(call int)
}

func (collector hookedCollector) Collect(_ context.Context, previous policy.CPUState, _ string) (policy.Reading, error) {
	*collector.calls++
	collector.onCall(*collector.calls)

	return policy.Reading{CPUState: previous, Sample: collector.sample}, nil
}

// requireAdmissionFailedSummary checks that source's one lifetime summary
// records that hippo itself stopped the run before its child started.
func requireAdmissionFailedSummary(t *testing.T, root, source string) {
	t.Helper()

	if outcomes := summaryOutcomes(t, root); len(outcomes) != 1 || outcomes[source] != "admission-failed" {
		t.Fatalf("the run hippo stopped before launch summarized as %v, want %s=admission-failed", outcomes, source)
	}
}

func TestAHostUnreadableDuringAdmissionIsSummarizedAsAdmissionFailed(t *testing.T) {
	// The run exits naming hippo.host.unreadable. Its summary used to call
	// the same run a capacity deferral, which it was not: hippo failed.
	t.Parallel()

	root := t.TempDir()
	now := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	calls := 0
	stderr := &bytes.Buffer{}
	application := Application{
		Stdout: &bytes.Buffer{}, Stderr: stderr, Environment: []string{"HIPPO_ROOT=" + root},
		// The command's own probe and the guard's first sample succeed; the
		// host then stops answering before admission completes.
		Collector: failsAfter{sample: stableDevelopmentSample(now), healthy: 2, calls: &calls},
		Now:       func() time.Time { return now }, Sleep: func(time.Duration) {},
	}
	code, err := application.Run(context.Background(), []string{
		"run", "--source", "sampler", "--disk-path", root, "--", "/bin/sh", "-c", "exit 0",
	})
	requireReason(t, code, err, stderr.String(), status.CodeHostUnreadable)
	requireAdmissionFailedSummary(t, root, "sampler")
}

func TestARefusedReceiptDuringAdmissionIsSummarizedAsAdmissionFailed(t *testing.T) {
	// A signal stops the sampling run, and the evidence root then refuses its
	// never-started receipt. The refusal outranks the signal in the exit
	// status, and the summary must say the same thing.
	root := t.TempDir()
	readOnlyDirectory(t, filepath.Join(root, "receipts"))
	now := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	ctx, interrupt := context.WithCancelCause(context.Background())
	defer interrupt(nil)
	stderr := &bytes.Buffer{}
	application := Application{
		Stdout: &bytes.Buffer{}, Stderr: stderr, Environment: []string{"HIPPO_ROOT=" + root},
		Collector: stableCollector{sample: stableDevelopmentSample(now)}, Now: func() time.Time { return now },
		Sleep: func(time.Duration) { interrupt(status.Interruption{Signal: syscall.SIGTERM}) },
	}
	code, err := application.Run(ctx, []string{
		"run", "--source", "sampler", "--disk-path", root, "--", "/bin/sh", "-c", "exit 0",
	})
	requireReason(t, code, err, stderr.String(), status.CodeEvidenceUnwritable)
	requireAdmissionFailedSummary(t, root, "sampler")
}

func TestALaunchThatFailsAfterAdmissionSamplingIsSummarizedAsAdmissionFailed(t *testing.T) {
	// The command was runnable when hippo checked it and stopped being
	// runnable while hippo sampled the host, so the launch itself fails. No
	// child started, and hippo, not the host, is why.
	t.Parallel()

	root := t.TempDir()
	script := filepath.Join(t.TempDir(), "payload")
	childMarker := filepath.Join(root, "child-started")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf started > \"$CHILD_MARKER\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	calls := 0
	stderr := &bytes.Buffer{}
	application := Application{
		Stdout: &bytes.Buffer{}, Stderr: stderr,
		Environment: []string{"HIPPO_ROOT=" + root, "CHILD_MARKER=" + childMarker},
		Collector: hookedCollector{sample: stableDevelopmentSample(now), calls: &calls, onCall: func(call int) {
			// The second collection is the guard's first admission sample.
			if call == 2 {
				_ = os.Chmod(script, 0o600)
			}
		}},
		Now: func() time.Time { return now }, Sleep: func(time.Duration) {},
	}
	code, err := application.Run(context.Background(), []string{"run", "--source", "launcher", "--disk-path", root, "--", script})
	requireReason(t, code, err, stderr.String(), status.CodeSupervisionFailed)
	if _, statError := os.Stat(childMarker); !os.IsNotExist(statError) {
		t.Fatalf("a launch that failed started its child: %v", statError)
	}
	requireAdmissionFailedSummary(t, root, "launcher")
}
