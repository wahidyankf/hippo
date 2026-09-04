package integration_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/policy"
)

type integrationCollector struct {
	samples     []policy.Sample
	index       int
	cancel      context.CancelFunc
	cancelAfter int
}

func (collector *integrationCollector) Collect(_ context.Context, previous policy.CPUState, _ string) (policy.Reading, error) {
	index := min(collector.index, len(collector.samples)-1)
	collector.index++
	if collector.cancel != nil && collector.index >= collector.cancelAfter {
		collector.cancel()
	}

	return policy.Reading{
		CPUState: previous,
		Sample:   collector.samples[index],
	}, nil
}

func integrationSample(measuredAt time.Time) policy.Sample {
	available, payload, disk, page, swaps := 12*policy.GiB, 7*policy.GiB, 40*policy.GiB, int64(16_384), int64(0)
	level, compressor, cpu := 1, true, 10.0

	return policy.Sample{
		SchemaVersion:                       2,
		MeasuredAt:                          measuredAt.UTC().Format(time.RFC3339Nano),
		AvailableNonCompressedEstimateBytes: &available,
		MemoryPressureLevel:                 &level,
		CompressorAvailable:                 &compressor,
		CompressorPayloadBytes:              &payload,
		AvailableParallelism:                12,
		CPUUtilizationPercent:               &cpu,
		DiskFreeBytes:                       &disk,
		PageSizeBytes:                       &page,
		SwapOuts:                            &swaps,
	}
}

func fastPolicy() policy.Policy {
	policy := policy.DefaultPolicy()
	policy.SampleInterval = time.Millisecond
	policy.AdmissionWindow = time.Second
	policy.TerminationGrace = time.Millisecond
	policy.LeaseWait = time.Second

	return policy
}

func TestGuardPreservesChildExitAndWritesEvidence(t *testing.T) {
	base := time.Now()
	collector := &integrationCollector{samples: []policy.Sample{
		integrationSample(base),
		integrationSample(base.Add(time.Millisecond)),
		integrationSample(base.Add(2 * time.Millisecond)),
	}}

	code, err := guard.Run(context.Background(), guard.RunConfig{
		Command:      "/bin/sh",
		Arguments:    []string{"-c", "exit 17"},
		TaskClass:    "ephemeral",
		Environment:  os.Environ(),
		EvidenceRoot: t.TempDir(),
		DiskPath:     ".",
		Collector:    collector,
		Policy:       fastPolicy(),
		Sleep:        func(time.Duration) {},
		Now:          time.Now,
		Stderr:       &bytes.Buffer{},
	})

	if err != nil || code != 17 {
		t.Fatalf("exit=%d error=%v", code, err)
	}
}

func TestGuardReturnsStorageCodeBeforeStartingChild(t *testing.T) {
	sample := integrationSample(time.Now())
	disk := 29 * policy.GiB
	sample.DiskFreeBytes = &disk

	code, err := guard.Run(context.Background(), guard.RunConfig{
		Command:      "/bin/sh",
		Arguments:    []string{"-c", "exit 99"},
		TaskClass:    "ephemeral",
		Environment:  os.Environ(),
		EvidenceRoot: t.TempDir(),
		DiskPath:     ".",
		Collector:    &integrationCollector{samples: []policy.Sample{sample}},
		Policy:       fastPolicy(),
		Sleep:        func(time.Duration) {},
		Now:          time.Now,
		Stderr:       &bytes.Buffer{},
	})

	if err != nil || code != guard.StorageBlockedExitCode {
		t.Fatalf("exit=%d error=%v", code, err)
	}
}

func TestInheritedGuardRunsDirectlyAndKeepsPortLease(t *testing.T) {
	root, portRoot := t.TempDir(), t.TempDir()
	session, err := guard.AcquireSession(context.Background(), root, "", "ephemeral", time.Second)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = guard.ReleaseSession(root, session) }()

	code, err := guard.Run(context.Background(), guard.RunConfig{
		Command:       "/bin/sh",
		Arguments:     []string{"-c", "exit 0"},
		TaskClass:     "ephemeral",
		Environment:   []string{"HIPPO_SESSION=" + session.Token},
		EvidenceRoot:  root,
		DiskPath:      ".",
		LeasePort:     45_123,
		LeaseOwner:    "integration",
		LeaseMinimum:  45_000,
		LeaseMaximum:  46_000,
		PortLeaseRoot: portRoot,
		Collector:     &integrationCollector{samples: []policy.Sample{integrationSample(time.Now())}},
		Policy:        fastPolicy(),
		Sleep:         func(time.Duration) {},
		Now:           time.Now,
		Stderr:        &bytes.Buffer{},
	})

	if err != nil || code != 0 {
		t.Fatalf("exit=%d error=%v", code, err)
	}
}

func TestGuardShedsCriticalEphemeralChild(t *testing.T) {
	base := time.Now()
	healthy := integrationSample(base)
	critical := integrationSample(base.Add(3 * time.Millisecond))
	level := 4
	critical.MemoryPressureLevel = &level
	collector := &integrationCollector{samples: []policy.Sample{healthy, healthy, healthy, critical}}

	code, err := guard.Run(context.Background(), guard.RunConfig{
		Command:      "/bin/sh",
		Arguments:    []string{"-c", "sleep 5"},
		TaskClass:    "ephemeral",
		Environment:  os.Environ(),
		EvidenceRoot: t.TempDir(),
		DiskPath:     ".",
		Collector:    collector,
		Policy:       fastPolicy(),
		Sleep:        func(time.Duration) {},
		Now:          time.Now,
		Stderr:       &bytes.Buffer{},
	})

	if err != nil || code != guard.CapacityDeferredExitCode {
		t.Fatalf("exit=%d error=%v", code, err)
	}
}

func TestGuardInjectsResolvedConcurrencyWithoutOverwritingCaller(t *testing.T) {
	base := time.Now()
	collector := &integrationCollector{samples: []policy.Sample{
		integrationSample(base),
		integrationSample(base.Add(time.Millisecond)),
		integrationSample(base.Add(2 * time.Millisecond)),
	}}
	resolution := policy.Resolution{
		RequestedProfile: "balanced",
		ResolvedProfile:  "minimal",
		FallbackChain:    []string{"balanced", "constrained", "minimal"},
		Concurrency:      1,
	}
	command := `[ "$HIPPO_PROFILE" = minimal ] && [ "$HIPPO_CONCURRENCY" = 1 ] && [ "$TOOL_WORKERS" = 1 ] && [ "$CALLER_WORKERS" = 5 ]`
	environment := []string{"PATH=" + os.Getenv("PATH"), "CALLER_WORKERS=5"}

	code, err := guard.Run(context.Background(), guard.RunConfig{
		Command:                "/bin/sh",
		Arguments:              []string{"-c", command},
		TaskClass:              "ephemeral",
		Environment:            environment,
		ConcurrencyEnvironment: []string{"TOOL_WORKERS", "CALLER_WORKERS"},
		EvidenceRoot:           t.TempDir(),
		DiskPath:               ".",
		Collector:              collector,
		Policy:                 fastPolicy(),
		Resolution:             resolution,
		Sleep:                  func(time.Duration) {},
		Now:                    time.Now,
		Stderr:                 &bytes.Buffer{},
	})

	if err != nil || code != 0 {
		t.Fatalf("exit=%d error=%v", code, err)
	}
}

func TestGuardExportsChildSessionAndBinary(t *testing.T) {
	base := time.Now()
	collector := &integrationCollector{samples: []policy.Sample{
		integrationSample(base),
		integrationSample(base.Add(time.Millisecond)),
		integrationSample(base.Add(2 * time.Millisecond)),
	}}
	command := `[ -n "$HIPPO_SESSION" ] && [ -x "$HIPPO_BIN" ]`

	code, err := guard.Run(context.Background(), guard.RunConfig{
		Command:      "/bin/sh",
		Arguments:    []string{"-c", command},
		TaskClass:    "ephemeral",
		Environment:  []string{"PATH=" + os.Getenv("PATH")},
		EvidenceRoot: t.TempDir(),
		DiskPath:     ".",
		Collector:    collector,
		Policy:       fastPolicy(),
		Sleep:        func(time.Duration) {},
		Now:          time.Now,
		Stderr:       &bytes.Buffer{},
	})

	if err != nil || code != 0 {
		t.Fatalf("exit=%d error=%v", code, err)
	}
}

func TestInterruptedGuardSignalsOnceThenForceStops(t *testing.T) {
	base := time.Now()
	collector := &integrationCollector{samples: []policy.Sample{
		integrationSample(base),
		integrationSample(base.Add(time.Millisecond)),
		integrationSample(base.Add(2 * time.Millisecond)),
	}}
	policy := fastPolicy()
	policy.TerminationGrace = 200 * time.Millisecond
	marker := filepath.Join(t.TempDir(), "terminations")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(300*time.Millisecond, cancel)

	started := time.Now()
	_, err := guard.Run(ctx, guard.RunConfig{
		Command:      "/bin/sh",
		Arguments:    []string{"-c", `trap 'printf x >> "$GUARD_TERM_MARKER"' TERM; for attempt in $(seq 1 40); do sleep 0.05; done`},
		TaskClass:    "ephemeral",
		Environment:  append(os.Environ(), "GUARD_TERM_MARKER="+marker),
		EvidenceRoot: t.TempDir(),
		DiskPath:     ".",
		Collector:    collector,
		Policy:       policy,
		Sleep:        func(time.Duration) {},
		Now:          time.Now,
		Stderr:       &bytes.Buffer{},
	})

	elapsed := time.Since(started)

	if err != nil {
		t.Fatal(err)
	}

	delivered, readError := os.ReadFile(marker)
	if readError != nil {
		t.Fatalf("child recorded no termination signal: %v", readError)
	}
	if len(delivered) != 1 {
		t.Fatalf("guard delivered %d termination signals, want exactly 1", len(delivered))
	}

	if elapsed > 1500*time.Millisecond {
		t.Fatalf("a child ignoring SIGTERM was not force-stopped: guard returned after %s", elapsed)
	}
}
