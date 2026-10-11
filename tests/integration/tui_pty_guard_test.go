package integration_test

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	runtimeadapter "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/tests/support"
)

func TestTUIRunnerPreservesExistingGuardedJob(t *testing.T) {
	script, err := exec.LookPath("script")
	if err != nil {
		t.Fatal("isolated PTY requires the existing script utility")
	}
	guarded := startTUIGuardedJob(t)
	runTUIRunnerPTY(t, script, "q")
	select {
	case result := <-guarded.done:
		guarded.done <- result
		t.Fatalf("TUI retired the existing guarded job: exit=%d error=%v", result.code, result.err)
	default:
	}
	if channelClosed(guarded.cancelled) || syscall.Kill(guarded.processID, 0) != nil {
		t.Fatal("existing guarded process did not survive TUI return")
	}
	t.Logf("existing guarded process %d remained alive after isolated q; fixture cleanup owns its cancellation", guarded.processID)
}

type tuiGuardResult struct {
	code int
	err  error
}

type tuiGuardedJob struct {
	cancelled <-chan struct{}
	done      chan tuiGuardResult
	processID int
}

func startTUIGuardedJob(t *testing.T) tuiGuardedJob {
	t.Helper()
	root := t.TempDir()
	ready := filepath.Join(root, "guarded-pid")
	ctx, cancel := context.WithTimeout(t.Context(), livenessLimit)
	done := make(chan tuiGuardResult, 1)
	base := time.Now()
	collector := &integrationCollector{samples: []policy.Sample{integrationSample(base), integrationSample(base.Add(time.Millisecond)), integrationSample(base.Add(2 * time.Millisecond))}}
	go func() {
		code, err := support.RunGuard(ctx, runtimeadapter.RunConfig{
			Command: "/bin/sh", Arguments: []string{"-c", `printf '%s' "$$" > "$HIPPO_TUI_GUARD_PID"; exec /bin/sleep 30`},
			Environment: append(os.Environ(), "HIPPO_TUI_GUARD_PID="+ready),
			ChildStdin:  strings.NewReader(""), ChildStdout: io.Discard, ChildStderr: io.Discard, Stderr: io.Discard,
			TaskClass: policy.TaskEphemeral, EvidenceRoot: filepath.Join(root, "evidence"), DiskPath: ".",
			Collector: collector, Policy: fastPolicy(), Now: time.Now,
			Resolution: policy.Resolution{RequestedProfile: "balanced", ResolvedProfile: "balanced", Concurrency: 4},
		})
		done <- tuiGuardResult{code: code, err: err}
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(livenessLimit):
			t.Error("owned guarded fixture did not join after cancellation")
		}
	})
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case result := <-done:
			done <- result
			t.Fatalf("guarded fixture did not launch: exit=%d error=%v", result.code, result.err)
		case <-ctx.Done():
			t.Fatal("guarded fixture did not become ready")
		case <-ticker.C:
			data, err := os.ReadFile(ready)
			if err != nil {
				continue
			}
			processID, err := strconv.Atoi(string(data))
			if err == nil && processID > 0 {
				return tuiGuardedJob{cancelled: ctx.Done(), done: done, processID: processID}
			}
		}
	}
}

func channelClosed(channel <-chan struct{}) bool {
	select {
	case <-channel:
		return true
	default:
		return false
	}
}
