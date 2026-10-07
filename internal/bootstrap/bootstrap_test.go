package bootstrap_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/bootstrap"
	"github.com/wahidyankf/hippo/internal/policy"
)

type countingCollector struct {
	calls int
	now   time.Time
}

func (collector *countingCollector) Collect(_ context.Context, previous policy.CPUState, _ string) (policy.Reading, error) {
	collector.calls++
	available, disk, cpu, pressure := 20*policy.GiB, 100*policy.GiB, 20.0, 1
	return policy.Reading{CPUState: previous, Sample: policy.Sample{
		SchemaVersion: 3, MeasuredAt: collector.now.UTC().Format(time.RFC3339Nano), Platform: "darwin",
		Capabilities: []string{"memory-pressure"}, EffectiveMemoryLimitBytes: 32 * policy.GiB,
		PhysicalMemoryBytes: 32 * policy.GiB, AvailableMemoryBytes: &available,
		AvailableParallelism: 8, CPUUtilizationPercent: &cpu, MemoryPressureLevel: &pressure,
		DiskFreeBytes: &disk, SwapState: "idle",
	}}, nil
}

func TestBootstrapUsesPauseChangedBeforeRun(t *testing.T) {
	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	collector := &countingCollector{now: now}
	boundary := bootstrap.WithDefaults(cli.Application{
		Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Collector: collector,
		Environment: []string{"HIPPO_ROOT=" + t.TempDir(), "HOME=" + t.TempDir()},
		Now:         func() time.Time { return now },
	})
	pauses := 0
	boundary.Sleep = func(duration time.Duration) {
		pauses++
		now = now.Add(duration)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
	defer cancel()
	code, err := boundary.Run(ctx, []string{"status", "--json"})
	if code != 0 || err != nil || pauses != 1 || collector.calls != 2 {
		t.Fatalf("status after changing pause: exit=%d error=%v pauses=%d samples=%d, want 0/nil/1/2", code, err, pauses, collector.calls)
	}
}

func TestBootstrapUsesCollectorClockAndStreamsChangedBeforeRun(t *testing.T) {
	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	original, replacement := &countingCollector{now: now}, &countingCollector{now: now}
	oldOutput, newOutput := &bytes.Buffer{}, &bytes.Buffer{}
	boundary := bootstrap.WithDefaults(cli.Application{
		Stdout: oldOutput, Stderr: &bytes.Buffer{}, Collector: original, Sleep: func(time.Duration) {},
		Environment: []string{"HIPPO_ROOT=" + t.TempDir(), "HOME=" + t.TempDir()},
		Now:         func() time.Time { return now },
	})
	clockCalls := 0
	boundary.Collector = replacement
	boundary.Stdout = newOutput
	boundary.Now = func() time.Time {
		clockCalls++
		return now
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	code, err := boundary.Run(ctx, []string{"status", "--json"})
	if code != 0 || err != nil || original.calls != 0 || replacement.calls != 2 || clockCalls == 0 {
		t.Fatalf("status after changing dependencies: exit=%d error=%v old/new samples=%d/%d clock=%d", code, err, original.calls, replacement.calls, clockCalls)
	}
	if oldOutput.Len() != 0 || newOutput.Len() == 0 {
		t.Fatal("status did not use the replacement output stream")
	}
}

func TestBootstrapPreservesCustomMonitorCallbacks(t *testing.T) {
	for _, when := range []string{"before composition", "after composition"} {
		t.Run(when, func(t *testing.T) {
			root := t.TempDir()
			collector := &countingCollector{}
			calls := 0
			monitor := func(ctx context.Context, config application.MonitorConfig) error {
				calls++
				if ctx.Err() != nil || config.Collector != collector {
					t.Error("custom monitor lost its context or configured collector")
				}
				return nil
			}
			input := cli.Application{
				Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Collector: collector,
				Environment: []string{"HIPPO_ROOT=" + root, "HOME=" + root},
			}
			if when == "before composition" {
				input.MonitorRelease = monitor
			}
			boundary := bootstrap.WithDefaults(input)
			if when == "after composition" {
				boundary.MonitorRelease = monitor
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			code, err := boundary.Run(ctx, []string{
				"release", "monitor", "--output", filepath.Join(root, "raw.jsonl"),
				"--summary", filepath.Join(root, "summary.json"), "--deployment-root", root,
				"--health-url", "http://127.0.0.1:9/health", "--routed-origin", "https://example.test",
			})
			if code != 0 || err != nil || calls != 1 || collector.calls != 0 {
				t.Fatalf("custom monitor: exit=%d error=%v callbacks=%d host samples=%d", code, err, calls, collector.calls)
			}
		})
	}
}
