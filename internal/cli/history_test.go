package cli //nolint:testpackage // Private parser and injected watch timing are intentional unit seams.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

type stableCollector struct {
	sample policy.Sample
}

func (collector stableCollector) Collect(_ context.Context, previous policy.CPUState, _ string) (policy.Reading, error) {
	return policy.Reading{CPUState: previous, Sample: collector.sample}, nil
}

func TestParseRollingDuration(t *testing.T) {
	t.Parallel()

	for value, expected := range map[string]time.Duration{
		"12h": 12 * time.Hour,
		"7d":  7 * 24 * time.Hour,
		"30d": 30 * 24 * time.Hour,
	} {
		actual, err := parseRollingDuration(value)
		if err != nil || actual != expected {
			t.Fatalf("%s = %s, %v", value, actual, err)
		}
	}
}

func TestHistoryJSONFiltersSourceAndTag(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	for index, source := range []string{"hippo", "rhino"} {
		row := evidence.Summary{
			SchemaVersion: 5, RunID: source, Source: source, Tags: map[string]string{"checkout": "worktree"},
			TaskClass: "ephemeral", ResourceTier: "standard", Outcome: "passed",
			FinishedAt: now.Add(time.Duration(index) * time.Minute).Format(time.RFC3339Nano),
		}
		data, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, source+".summary.json"), append(data, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	output := &bytes.Buffer{}
	application := Application{
		Stdout: output, Environment: []string{"HIPPO_ROOT=" + root}, Now: func() time.Time { return now.Add(time.Hour) },
	}
	code, err := application.Run(context.Background(), []string{
		"history", "--since", "30d", "--source", "hippo", "--tag", "checkout=worktree", "--json",
	})
	if err != nil || code != 0 || !strings.Contains(output.String(), `"runId":"hippo"`) ||
		strings.Contains(output.String(), `"runId":"rhino"`) {
		t.Fatalf("code=%d output=%s error=%v", code, output, err)
	}
}

func TestWatchEmitsOnlyChangedStatusSnapshots(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	available, disk, cpu, pressure := 20*policy.GiB, 100*policy.GiB, 20.0, 1
	sample := policy.Sample{
		SchemaVersion: 3, MeasuredAt: now.Format(time.RFC3339Nano), Platform: "darwin",
		Capabilities: []string{"memory-pressure"}, EffectiveMemoryLimitBytes: 32 * policy.GiB,
		PhysicalMemoryBytes: 32 * policy.GiB, AvailableMemoryBytes: &available,
		AvailableParallelism: 8, CPUUtilizationPercent: &cpu, MemoryPressureLevel: &pressure,
		DiskFreeBytes: &disk, SwapState: "idle",
	}
	ctx, cancel := context.WithCancel(context.Background())
	sleeps := 0
	output := &bytes.Buffer{}
	application := Application{
		Stdout: output, Stderr: &bytes.Buffer{}, Environment: []string{"HIPPO_ROOT=" + root},
		Collector: stableCollector{sample: sample}, Now: func() time.Time { return now },
		Sleep: func(time.Duration) {
			sleeps++
			if sleeps == 2 {
				cancel()
			}
		},
	}
	code, err := application.Run(ctx, []string{"watch", "--json", "--interval", "1s"})
	if err != nil || code != 0 || strings.Count(strings.TrimSpace(output.String()), "\n") != 0 ||
		!strings.Contains(output.String(), `"schemaVersion":5`) {
		t.Fatalf("code=%d output=%q sleeps=%d error=%v", code, output.String(), sleeps, err)
	}
}

func TestHistoryReportsAnUnreadableArchiveAsUnreadable(t *testing.T) {
	// Reading the evidence root and writing to it fail for different reasons
	// and are fixed differently, so a corrupt archive must not be reported as
	// a refused write.
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "history"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "history", "2026-09-16.jsonl.gz"), []byte("not gzip"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	stderr := &bytes.Buffer{}
	application := Application{
		Stdout: &bytes.Buffer{}, Stderr: stderr,
		Environment: []string{"HIPPO_ROOT=" + root}, Now: func() time.Time { return now },
	}
	code, _ := application.Run(context.Background(), []string{"history", "--since", "30d"})
	if code != status.GuardFailed {
		t.Fatalf("exit %d, want %d: %q", code, status.GuardFailed, stderr.String())
	}
	if !strings.Contains(stderr.String(), "hippo: [hippo.evidence.unreadable] reading run history:") {
		t.Fatalf("an unreadable archive was not reported as hippo.evidence.unreadable: %q", stderr.String())
	}
}
