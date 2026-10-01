package integration_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// evidenceDecidesAdmission is an admission window no runner exhausts. Every
// test here feeds evidence that admits after a fixed number of samples, and a
// one-second window also timed the runner: one too slow to take those samples
// inside it deferred the run with exit 75 before the behaviour under test began.
const evidenceDecidesAdmission = time.Hour

// livenessLimit bounds a wait for something that must happen: a child to
// become ready, a forced stop to be confirmed by the killed group's real exit.
// It only detects a wait that never ends, and a healthy run never spends it.
const livenessLimit = 30 * time.Second

func fastPolicy() policy.Policy {
	policy := policy.DefaultPolicy()
	policy.SampleInterval = time.Millisecond
	policy.AdmissionWindow = evidenceDecidesAdmission
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

func TestReservationCoordinationReturnsProtocolMismatchBeforeChildExecution(t *testing.T) {
	root := t.TempDir()
	marker := []byte("{\"schemaVersion\":1,\"mode\":\"reservation\"}\n")
	if err := os.WriteFile(filepath.Join(root, "coordination-mode.json"), marker, 0o600); err != nil {
		t.Fatal(err)
	}

	childMarker := filepath.Join(t.TempDir(), "child-started")
	stderr := &bytes.Buffer{}
	code, err := guard.Run(context.Background(), guard.RunConfig{
		Command:      "/bin/sh",
		Arguments:    []string{"-c", `printf started > "$CHILD_MARKER"`},
		TaskClass:    "ephemeral",
		Environment:  append(os.Environ(), "CHILD_MARKER="+childMarker),
		EvidenceRoot: root,
		DiskPath:     ".",
		Collector:    &integrationCollector{samples: []policy.Sample{integrationSample(time.Now())}},
		Policy:       fastPolicy(),
		Sleep:        func(time.Duration) {},
		Now:          time.Now,
		Stderr:       stderr,
	})

	if err != nil || code != policy.ProtocolMismatchExitCode {
		t.Fatalf("exit=%d error=%v", code, err)
	}
	if !strings.Contains(stderr.String(), "reservation mode is active") {
		t.Fatalf("missing actionable coordination diagnostic: %q", stderr.String())
	}
	if _, statError := os.Stat(childMarker); !errors.Is(statError, os.ErrNotExist) {
		t.Fatalf("deferred child executed: %v", statError)
	}
	data, readError := os.ReadFile(filepath.Join(root, "coordination-mode.json"))
	if readError != nil {
		t.Fatal(readError)
	}
	if !bytes.Equal(data, marker) {
		t.Fatalf("reservation marker changed from %q to %q", marker, data)
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
		// The shed is judged by the child's real exit, not by how quickly a
		// loaded host reaps it.
		RetirementConfirmation: livenessLimit,
		Sleep:                  func(time.Duration) {},
		Now:                    time.Now,
		Stderr:                 &bytes.Buffer{},
	})

	if err != nil || code != guard.PressureShedExitCode {
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
	childRoot := t.TempDir()
	marker := filepath.Join(childRoot, "terminations")
	ready := filepath.Join(childRoot, "trap-installed")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Interrupting on a wall clock does not promise the child is already ignoring
	// termination: on a loaded host the signal can land after the shell starts but
	// before it installs its trap, which kills the child by SIGTERM's default
	// action and leaves this test nothing to observe. Interrupt on the child's own
	// readiness mark so the premise holds every run.
	//
	// The child's own bound is a release mark this test writes on its clock,
	// well past the grace, never an iteration count: how long a count takes
	// depends on the shell, and a fast shell can finish one inside the grace,
	// which reports a correct guard as letting the child finish on its own.
	release := filepath.Join(childRoot, "release")
	runDone := make(chan struct{})
	released := make(chan struct{})
	go func() {
		defer close(released)

		awaitChildReadiness(ready)
		cancel()

		select {
		case <-time.After(livenessLimit):
		case <-runDone:
		}
		_ = os.WriteFile(release, []byte("r"), 0o600)
	}()
	defer func() {
		close(runDone)
		<-released
	}()

	// The child waits without forking. A forked foreground child shares the
	// payload process group, so one group SIGTERM kills it too and the shell then
	// runs its trap a second time to report the child that died from that signal,
	// which makes the trap count an unreliable witness for how often the guard
	// actually signalled. A builtin-only wait counts kernel deliveries exactly,
	// and its release stops a guard that never force-stops from leaving a
	// spinning orphan behind. A child that outlives the grace until that release
	// marks its own completion, which is how the test tells a force-stop from a
	// child that finished.
	completed := filepath.Join(childRoot, "completed")
	_, err := guard.Run(ctx, guard.RunConfig{
		Command:      "/bin/sh",
		Arguments:    []string{"-c", `trap 'printf x >> "$GUARD_TERM_MARKER"' TERM; printf r > "$GUARD_READY_MARKER"; while [ ! -e "$GUARD_RELEASE_MARKER" ]; do :; done; printf d > "$GUARD_DONE_MARKER"`},
		TaskClass:    "ephemeral",
		Environment:  append(os.Environ(), "GUARD_TERM_MARKER="+marker, "GUARD_READY_MARKER="+ready, "GUARD_RELEASE_MARKER="+release, "GUARD_DONE_MARKER="+completed),
		EvidenceRoot: t.TempDir(),
		DiskPath:     ".",
		Collector:    collector,
		Policy:       policy,
		// The deliberate forced stop is judged by the killed group's real
		// exit, not by how quickly a loaded host reaps it.
		RetirementConfirmation: livenessLimit,
		Sleep:                  func(time.Duration) {},
		Now:                    time.Now,
		Stderr:                 &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, readyError := os.Stat(ready); readyError != nil {
		t.Fatalf("child never installed its termination trap: %v", readyError)
	}

	delivered, readError := os.ReadFile(marker)
	if readError != nil {
		t.Fatalf("child recorded no termination signal: %v", readError)
	}
	if len(delivered) != 1 {
		t.Fatalf("guard delivered %d termination signals, want exactly 1", len(delivered))
	}

	if _, completedError := os.Stat(completed); completedError == nil {
		t.Fatal("a child ignoring SIGTERM finished on its own instead of being force-stopped")
	}
}

// awaitChildReadiness waits for the guarded child to publish its readiness mark.
// It also returns on its own deadline so a child that never becomes ready
// reaches an explicit assertion instead of hanging the suite.
func awaitChildReadiness(path string) {
	deadline := time.Now().Add(30 * time.Second)

	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}

		time.Sleep(time.Millisecond)
	}
}
