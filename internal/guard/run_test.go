package guard //nolint:testpackage // Deterministic launcher and lifecycle seams are intentionally exercised inside the production package.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
	"golang.org/x/sys/unix" //nolint:depguard // Kernel-lock fixtures must exercise the same flock implementation as production.
)

// forcedStopExitCode is the shell convention for a child killed by SIGKILL.
const forcedStopExitCode = 137

type controlledRunCollector struct{}

func (*controlledRunCollector) Collect(context.Context, policy.CPUState, string) (policy.Reading, error) {
	now := time.Now().UTC()

	return policy.Reading{Sample: policy.Sample{
		SchemaVersion: 3, MeasuredAt: now.Format(time.RFC3339Nano), Platform: "darwin",
		Capabilities:              []string{"compressor", "memory-pressure", "swap"},
		EffectiveMemoryLimitBytes: 32 * policy.GiB, AvailableMemoryBytes: new(12 * policy.GiB),
		AvailableNonCompressedEstimateBytes: new(12 * policy.GiB), MemoryPressureLevel: new(1),
		CompressorAvailable: new(true), CompressorPayloadBytes: new(7 * policy.GiB), PhysicalMemoryBytes: 32 * policy.GiB,
		AvailableParallelism: 8, CPUUtilizationPercent: new(20.0), DiskFreeBytes: new(40 * policy.GiB),
		DiskTotalBytes: new(512 * policy.GiB), PageSizeBytes: new(int64(16_384)), SwapIns: new(int64(10)),
		SwapOuts: new(int64(20)), SwapFreeBytes: new(2 * policy.GiB), SwapState: "idle",
	}}, nil
}

func TestCoordinationLockSerializesSameProcessByRoot(t *testing.T) {
	root := t.TempDir()
	first, err := acquireCoordinationLock(context.Background(), root, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseCoordinationLock(first) }()

	started := make(chan struct{})
	acquired := make(chan *os.File, 1)
	failed := make(chan error, 1)
	go func() {
		close(started)
		lock, lockError := acquireCoordinationLock(context.Background(), root, 200*time.Millisecond)
		if lockError != nil {
			failed <- lockError

			return
		}
		acquired <- lock
	}()
	<-started
	select {
	case lock := <-acquired:
		_ = releaseCoordinationLock(lock)
		t.Fatal("same-process coordination acquisition bypassed the existing critical section")
	case err = <-failed:
		t.Fatalf("same-process coordination acquisition failed before release: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	if err = releaseCoordinationLock(first); err != nil {
		t.Fatal(err)
	}
	first = nil
	select {
	case lock := <-acquired:
		if err = releaseCoordinationLock(lock); err != nil {
			t.Fatal(err)
		}
	case err = <-failed:
		t.Fatalf("same-process coordination acquisition did not resume after release: %v", err)
	case <-time.After(time.Second):
		t.Fatal("same-process coordination acquisition remained blocked after release")
	}
}

func TestCoordinationLockAllowsDistinctRootsInParallel(t *testing.T) {
	first, err := acquireCoordinationLock(context.Background(), t.TempDir(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseCoordinationLock(first) }()

	start := time.Now()
	second, err := acquireCoordinationLock(context.Background(), t.TempDir(), 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err = releaseCoordinationLock(second); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed >= 40*time.Millisecond {
		t.Fatalf("distinct-root coordination was serialized: %s", elapsed)
	}
}

func TestCoordinationLockRejectsPreCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	lock, err := acquireCoordinationLock(ctx, t.TempDir(), time.Second)
	if lock != nil || !errors.Is(err, context.Canceled) {
		if lock != nil {
			_ = releaseCoordinationLock(lock)
		}
		t.Fatalf("pre-cancelled coordination acquisition succeeded: lock=%v error=%v", lock, err)
	}
}

func TestCoordinationLockUsesOneBoundedWaitBudget(t *testing.T) {
	root := t.TempDir()
	first, err := acquireCoordinationLock(context.Background(), root, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = releaseCoordinationLock(first) }()

	start := time.Now()
	second, err := acquireCoordinationLock(context.Background(), root, 30*time.Millisecond)
	if second != nil || !IsCoordinationDeferred(err) {
		if second != nil {
			_ = releaseCoordinationLock(second)
		}
		t.Fatalf("same-root deadline did not defer: lock=%v error=%v", second, err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("coordination layers exceeded one wait budget: %s", elapsed)
	}
}

func TestCoordinationIdentityProbeHelper(t *testing.T) {
	path := os.Getenv("HIPPO_COORDINATION_IDENTITY_PROBE")
	if path == "" {
		return
	}
	//nolint:gosec // The helper opens only the test-owned temporary path supplied by its parent fixture.
	identity, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = identity.Close() }()
	err = unix.Flock(int(identity.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return
	}
	if err == nil {
		_ = unix.Flock(int(identity.Fd()), unix.LOCK_UN)
		t.Fatal("reservation identity was not externally locked")
	}
	t.Fatal(err)
}

func probeReservationIdentityExternallyLocked(identityPath string) error {
	//nolint:gosec // The fixture invokes only the current test binary with fixed arguments.
	probe := exec.Command(os.Args[0], "-test.run=^TestCoordinationIdentityProbeHelper$")
	probe.Env = append(os.Environ(), "HIPPO_COORDINATION_IDENTITY_PROBE="+identityPath)
	if output, probeError := probe.CombinedOutput(); probeError != nil {
		return fmt.Errorf("reservation identity was not retained: %s: %w", output, probeError)
	}

	return nil
}

func assertReservationIdentityExternallyLocked(t *testing.T, identityPath string) {
	t.Helper()
	if err := probeReservationIdentityExternallyLocked(identityPath); err != nil {
		t.Fatal(err)
	}
}

// requireDeferredCleanupOutcome states the contract for a cancelled owner whose
// release found the shared lock held: the owner mark is retained for the
// automatic retry, so the caller sees its child's own outcome and no failure.
func requireDeferredCleanupOutcome(code int, runError error) error {
	if runError != nil {
		return fmt.Errorf("deferred coordination cleanup was reported as a failure: code=%d error=%w", code, runError)
	}
	if code != forcedStopExitCode {
		return fmt.Errorf("cancelled owner exited %d, want the force-stopped child code %d", code, forcedStopExitCode)
	}

	return nil
}

func TestOwnerCancellationRetainsIdentityAcrossSameProcessCoordination(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "child-pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	settings := policy.DefaultPolicy()
	settings.AdmissionWindow = 100 * time.Millisecond
	settings.LeaseWait = 50 * time.Millisecond
	settings.SampleInterval = 2 * time.Millisecond
	settings.TerminationGrace = 200 * time.Millisecond
	plan := ReservationPlan{
		Capacity:  ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
		Requested: ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
		Allocated: ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
	}
	type runResult struct {
		code int
		err  error
	}
	done := make(chan runResult, 1)
	go func() {
		code, runError := Run(ctx, RunConfig{
			Command: "/bin/sh", Arguments: []string{"-c", `printf '%s' "$$" > "$CHILD_PID"; trap '' TERM; while :; do sleep 0.01; done`},
			TaskClass: policy.TaskEphemeral, Environment: append(os.Environ(), "CHILD_PID="+marker), EvidenceRoot: root,
			Collector: &controlledRunCollector{}, Policy: settings,
			Resolution: policy.Resolution{RequestedProfile: "balanced", ResolvedProfile: "balanced", Concurrency: 1},
			ReservationPolicy: ReservationPolicy{
				Enabled: true, MaxCPU: plan.Capacity.CPU, MaxMemoryBytes: plan.Capacity.MemoryBytes, MaxActiveOwners: 1,
			},
			ReservationPlan: plan,
			ChildStdin:      bytes.NewBuffer(nil), ChildStdout: &bytes.Buffer{}, ChildStderr: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		})
		done <- runResult{code: code, err: runError}
	}()

	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("reserved child did not start")
		}
		time.Sleep(time.Millisecond)
	}
	ledgerPath := reservationLedgerPath(root)
	var before []byte
	var ledger reservationLedger
	deadline = time.Now().Add(time.Second)
	for {
		before, _ = os.ReadFile(ledgerPath)
		if json.Unmarshal(before, &ledger) == nil && len(ledger.Owners) == 1 && ledger.Owners[0].ProcessGroup > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("reservation owner did not activate")
		}
		time.Sleep(time.Millisecond)
	}
	identityPath, err := reservationIdentityPath(root, ledger.Owners[0].Token)
	if err != nil {
		t.Fatal(err)
	}
	assertReservationIdentityExternallyLocked(t, identityPath)

	coordination, err := acquireCoordinationLock(context.Background(), root, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	var run runResult
	select {
	case run = <-done:
		if outcomeError := requireDeferredCleanupOutcome(run.code, run.err); outcomeError != nil {
			_ = releaseCoordinationLock(coordination)
			t.Fatal(outcomeError)
		}
	case <-time.After(time.Second):
		_ = releaseCoordinationLock(coordination)
		t.Fatal("owner cancellation blocked on same-process coordination")
	}
	pidData, err := os.ReadFile(marker)
	if err != nil {
		_ = releaseCoordinationLock(coordination)
		t.Fatal(err)
	}
	childPID, err := strconv.Atoi(string(pidData))
	if err != nil {
		_ = releaseCoordinationLock(coordination)
		t.Fatal(err)
	}
	if signalError := syscall.Kill(-childPID, 0); !errors.Is(signalError, syscall.ESRCH) {
		_ = releaseCoordinationLock(coordination)
		t.Fatalf("cancelled child group remains live after reap: %v", signalError)
	}
	after, err := os.ReadFile(ledgerPath)
	if err != nil || !bytes.Equal(before, after) {
		_ = releaseCoordinationLock(coordination)
		t.Fatalf("release failure changed ledger bytes: equal=%v error=%v", bytes.Equal(before, after), err)
	}
	if probeError := probeReservationIdentityExternallyLocked(identityPath); probeError != nil {
		_ = releaseCoordinationLock(coordination)
		t.Fatalf("%v; release error=%v", probeError, run.err)
	}
	if err = releaseCoordinationLock(coordination); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(time.Second)
	for {
		totals, statusError := ReservationStatus(context.Background(), root)
		if statusError == nil && totals.ActiveOwners == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("automatic release retry did not finish: totals=%+v error=%v", totals, statusError)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestCompiledGuardHelper(t *testing.T) {
	mode := os.Getenv("HIPPO_COMPILED_GUARD_HELPER")
	if mode != "supervisor-death" && mode != "port-supervisor-death" && mode != "schema1-heavy" && mode != "schema1-service" {
		return
	}
	policyValue := policy.DefaultPolicy()
	policyValue.AdmissionWindow = 100 * time.Millisecond
	policyValue.LeaseWait = 100 * time.Millisecond
	policyValue.SampleInterval = 10 * time.Millisecond
	policyValue.TerminationGrace = 50 * time.Millisecond
	plan := ReservationPlan{
		Capacity:  ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
		Requested: ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
	}
	config := RunConfig{
		Command: "/bin/sh", Arguments: []string{"-c", `printf '%s' "$$" > "$LONG_CHILD_PID"; trap '' TERM; while :; do sleep 0.05; done`},
		TaskClass: policy.TaskEphemeral, EvidenceRoot: os.Getenv("HIPPO_COMPILED_GUARD_ROOT"),
		Environment: os.Environ(), Collector: &controlledRunCollector{}, Policy: policyValue,
		Resolution:        policy.Resolution{RequestedProfile: "minimal", ResolvedProfile: "minimal", Concurrency: 1},
		ReservationPolicy: ReservationPolicy{Enabled: true, MaxActiveOwners: 1}, ReservationPlan: plan,
		EvidenceLimits: evidence.DefaultLimits(),
	}
	if mode != "supervisor-death" { //nolint:nestif // The self-exec helper dispatches isolated process-boundary fixture modes.
		if mode == "port-supervisor-death" {
			port, parseError := strconv.Atoi(os.Getenv("HIPPO_COMPILED_GUARD_PORT"))
			if parseError != nil {
				t.Fatal(parseError)
			}
			config.LeasePort, config.LeaseMinimum, config.LeaseMaximum = port, port, port
			config.LeaseOwner = "fixture"
			config.PortLeaseRoot = os.Getenv("HIPPO_COMPILED_GUARD_PORT_ROOT")
		} else {
			config.ReservationPolicy = ReservationPolicy{}
			if mode == "schema1-service" {
				config.TaskClass = policy.TaskService
			}
		}
	}
	exitCode, err := Run(context.Background(), config)
	if err != nil || exitCode != 0 {
		t.Fatalf("compiled guard helper exit=%d error=%v", exitCode, err)
	}
}

func TestAwaitLifetimeHandshakeIsBounded(t *testing.T) {
	started := time.Now()
	_, err := awaitLifetimeHandshake(context.Background(), io.LimitReader(&neverLifetimeReader{}, 1), 10*time.Millisecond)
	if !errors.Is(err, errLifetimeHandshakeDeadline) {
		t.Fatalf("expected bounded handshake deadline, got %v", err)
	}
	if time.Since(started) > 250*time.Millisecond {
		t.Fatal("lifetime handshake exceeded its bounded deadline")
	}
}

func TestStalledLifetimeHandshakeRetainsOwnershipUntilLauncherExit(t *testing.T) {
	root := t.TempDir()
	reservationRoot := filepath.Join(root, "reservation")
	portRoot := filepath.Join(root, "ports")
	plan := ReservationPlan{
		Capacity:  ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
		Requested: ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
	}
	session, err := AcquireReservation(
		context.Background(), reservationRoot, "", policy.TaskEphemeral, "minimal", "fixture", plan, 1, time.Second,
	)
	if err != nil || session == nil {
		t.Fatalf("acquire reservation: %v", err)
	}
	const port = 23_747
	lease, err := AcquirePortLease(portRoot, port, "fixture", port, port)
	if err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(root, "stalled-launcher")
	stoppedMarker := filepath.Join(root, "launcher-stopped")
	if err = os.WriteFile(launcher, []byte("#!/bin/sh\n(sleep 0.05; : > \"$STALL_MARKER\") &\nkill -STOP $$\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	lifetime, launchError := startSupervisedLifetimeWithLauncher(
		context.Background(), RunConfig{}, "/bin/true", append(os.Environ(), "STALL_MARKER="+stoppedMarker), launcher, 10*time.Millisecond,
		session.identityLock, lease.identityLock,
	)
	if lifetime == nil || launchError == nil {
		t.Fatalf("stalled launcher result lifetime=%v error=%v", lifetime, launchError)
	}
	defer func() {
		if lifetime.command.Process != nil {
			_ = lifetime.command.Process.Kill()
		}
	}()
	if err = errors.Join(abandonReservationIdentity(session), abandonPortLeaseIdentity(lease)); err != nil {
		t.Fatal(err)
	}
	if totals, statusError := ReservationStatus(context.Background(), reservationRoot); statusError != nil || totals.ActiveOwners != 1 {
		t.Fatalf("stalled launcher reservation was released: totals=%+v error=%v", totals, statusError)
	}
	if competitor, acquireError := AcquirePortLease(portRoot, port, "competitor", port, port); acquireError == nil {
		_ = ReleasePortLease(portRoot, competitor)
		t.Fatal("stalled launcher port was released")
	}
	deadline := time.Now().Add(time.Second)
	for {
		if _, statError := os.Stat(stoppedMarker); statError == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("launcher did not reach its stopped state")
		}
		time.Sleep(time.Millisecond)
	}
	if err = lifetime.command.Process.Signal(syscall.SIGCONT); err != nil {
		t.Fatal(err)
	}
	select {
	case <-lifetime.exited:
	case <-time.After(time.Second):
		t.Fatal("resumed launcher did not exit")
	}
	if totals, statusError := ReservationStatus(context.Background(), reservationRoot); statusError != nil || totals.ActiveOwners != 0 {
		t.Fatalf("exited launcher reservation was not reclaimed: totals=%+v error=%v", totals, statusError)
	}
	competitor, err := AcquirePortLease(portRoot, port, "competitor", port, port)
	if err != nil {
		t.Fatalf("exited launcher port was not reclaimed: %v", err)
	}
	if err = ReleasePortLease(portRoot, competitor); err != nil {
		t.Fatal(err)
	}
}

func TestLifetimeLauncherRequiresParentCapability(t *testing.T) {
	if lifetimeLauncherInputsAuthorized([]string{"hippo", lifetimeLauncherArgument, "", "/bin/true"}, []string{
		lifetimeLauncherEnvironment + "=1",
		lifetimeCapabilityEnvironment + "=0123456789abcdef0123456789abcdef",
	}, false, false) {
		t.Fatal("launcher mode accepted an invocation without parent capability pipes")
	}
}

type neverLifetimeReader struct{}

func (*neverLifetimeReader) Read([]byte) (int, error) {
	select {}
}

func TestAbandonedLocalHandlesPermitLaterSameProcessReclaim(t *testing.T) {
	root := t.TempDir()
	reservationRoot := filepath.Join(root, "reservation")
	portRoot := filepath.Join(root, "ports")
	plan := ReservationPlan{
		Capacity:  ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
		Requested: ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
	}
	session, err := AcquireReservation(
		context.Background(), reservationRoot, "", policy.TaskEphemeral, "minimal", "fixture", plan, 1, time.Second,
	)
	if err != nil || session == nil {
		t.Fatalf("acquire reservation: %v", err)
	}
	const port = 23_744
	lease, err := AcquirePortLease(portRoot, port, "fixture", port, port)
	if err != nil {
		t.Fatalf("acquire port: %v", err)
	}
	holder := exec.Command("/bin/sh", "-c", "sleep 0.15")
	holder.ExtraFiles = []*os.File{session.identityLock, lease.identityLock}
	if err = holder.Start(); err != nil {
		t.Fatal(err)
	}
	if err = errors.Join(abandonReservationIdentity(session), abandonPortLeaseIdentity(lease)); err != nil {
		t.Fatalf("abandon local handles: %v", err)
	}
	if totals, statusError := ReservationStatus(context.Background(), reservationRoot); statusError != nil || totals.ActiveOwners != 1 {
		t.Fatalf("live holder reservation was not retained: totals=%+v error=%v", totals, statusError)
	}
	if competitor, acquireError := AcquirePortLease(portRoot, port, "competitor", port, port); acquireError == nil {
		_ = ReleasePortLease(portRoot, competitor)
		t.Fatal("live holder port was reclaimed")
	}
	if err = holder.Wait(); err != nil {
		t.Fatal(err)
	}
	if totals, statusError := ReservationStatus(context.Background(), reservationRoot); statusError != nil || totals.ActiveOwners != 0 {
		t.Fatalf("retired holder reservation was not reclaimed: totals=%+v error=%v", totals, statusError)
	}
	competitor, err := AcquirePortLease(portRoot, port, "competitor", port, port)
	if err != nil {
		t.Fatalf("retired holder port was not reclaimed: %v", err)
	}
	if err = ReleasePortLease(portRoot, competitor); err != nil {
		t.Fatal(err)
	}
}

func TestTokenizedPortIdentityCorruptionFailClosed(t *testing.T) {
	for _, fault := range []string{"missing", "replaced"} {
		t.Run(fault, func(t *testing.T) {
			root := t.TempDir()
			const port = 23_745
			lease, err := AcquirePortLease(root, port, "fixture", port, port)
			if err != nil {
				t.Fatal(err)
			}
			markerPath := filepath.Join(lease.Path, "owner.json")
			markerBefore, err := os.ReadFile(markerPath)
			if err != nil {
				t.Fatal(err)
			}
			identityPath := filepath.Join(lease.Path, "identity.lock")
			if fault == "replaced" {
				if err = os.Rename(identityPath, identityPath+".original"); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(identityPath, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			} else if err = os.Remove(identityPath); err != nil {
				t.Fatal(err)
			}
			if competitor, acquireError := AcquirePortLease(root, port, "competitor", port, port); acquireError == nil {
				_ = ReleasePortLease(root, competitor)
				t.Fatal("corrupt tokenized identity was reclaimed")
			}
			markerAfter, err := os.ReadFile(markerPath)
			if err != nil || string(markerAfter) != string(markerBefore) {
				t.Fatalf("corrupt tokenized marker changed: error=%v", err)
			}
			if err = ReleasePortLease(root, lease); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStaleSameProcessPortHandleCannotReleaseReplacement(t *testing.T) {
	root := t.TempDir()
	const port = 23_746
	oldLease, err := AcquirePortLease(root, port, "fixture", port, port)
	if err != nil {
		t.Fatal(err)
	}
	staleHandle := *oldLease
	if err = ReleasePortLease(root, oldLease); err != nil {
		t.Fatal(err)
	}
	replacement, err := AcquirePortLease(root, port, "fixture", port, port)
	if err != nil {
		t.Fatal(err)
	}
	if err = ReleasePortLease(root, &staleHandle); err == nil {
		t.Fatal("stale handle released a replacement lease")
	}
	if err = ReleasePortLease(root, replacement); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyTokenlessPortMarkerCompatibility(t *testing.T) {
	root := t.TempDir()
	const livePort = 23_748
	livePath := filepath.Join(root, "23748.lock")
	if err := os.Mkdir(livePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeLeaseOwner(livePath, leaseOwner{
		SchemaVersion: 1, PID: os.Getpid(), Port: livePort, Owner: "legacy",
	}); err != nil {
		t.Fatal(err)
	}
	if competitor, err := AcquirePortLease(root, livePort, "competitor", livePort, livePort); err == nil {
		_ = ReleasePortLease(root, competitor)
		t.Fatal("live tokenless port marker was reclaimed")
	}
	const stalePort = 23_749
	stalePath := filepath.Join(root, "23749.lock")
	if err := os.Mkdir(stalePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeLeaseOwner(stalePath, leaseOwner{
		SchemaVersion: 1, PID: 1 << 30, Port: stalePort, Owner: "legacy",
	}); err != nil {
		t.Fatal(err)
	}
	competitor, err := AcquirePortLease(root, stalePort, "competitor", stalePort, stalePort)
	if err != nil {
		t.Fatalf("positively stale tokenless port marker was not reclaimed: %v", err)
	}
	if err = ReleasePortLease(root, competitor); err != nil {
		t.Fatal(err)
	}
}

func TestLegacySchemaOnePIDOnlyOwnershipCompatibility(t *testing.T) { //nolint:cyclop,gocognit // Table-driven compatibility characterization intentionally checks live and stale heavy/service records together.
	const legacyToken = "0123456789abcdef0123456789abcdef"
	plan := ReservationPlan{
		Capacity:  ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
		Requested: ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
	}
	seed := func(t *testing.T, root, class string, pid int) string {
		t.Helper()
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := writeCoordinationMarker(root, coordinationMarker{
			SchemaVersion: coordinationSchemaVersion, Mode: coordinationModeExclusive,
		}); err != nil {
			t.Fatal(err)
		}
		owner := leaseOwner{
			SchemaVersion: 1, PID: pid, Token: legacyToken, Class: class,
		}
		if class == "heavy" {
			path := filepath.Join(root, "heavy.lock")
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := writeLeaseOwner(path, owner); err != nil {
				t.Fatal(err)
			}

			return filepath.Join(path, "owner.json")
		}
		path, err := writeSessionRecord(root, owner)
		if err != nil {
			t.Fatal(err)
		}

		return path
	}
	for _, class := range []string{"heavy", "service"} {
		t.Run(class, func(t *testing.T) {
			liveRoot := filepath.Join(t.TempDir(), "live")
			livePath := seed(t, liveRoot, class, os.Getpid())
			before, err := os.ReadFile(livePath)
			if err != nil {
				t.Fatal(err)
			}
			reservation, takeoverError := AcquireReservation(
				context.Background(), liveRoot, "", policy.TaskEphemeral, "minimal", "replacement", plan, 1, 0,
			)
			if reservation != nil {
				_ = ReleaseReservation(liveRoot, reservation)
			}
			after, readError := os.ReadFile(livePath)
			if reservation != nil || !IsCoordinationDeferred(takeoverError) || readError != nil || !bytes.Equal(before, after) {
				t.Fatalf("live legacy %s ownership was not retained: session=%v takeover=%v read=%v", class, reservation != nil, takeoverError, readError)
			}
			if class == "heavy" {
				competitor, acquireError := AcquireSession(context.Background(), liveRoot, "", policy.TaskTransactional, 0)
				if competitor != nil {
					_ = ReleaseSession(liveRoot, competitor)
				}
				if competitor != nil || acquireError != nil {
					t.Fatalf("live legacy heavy ownership admitted a competitor: %v", acquireError)
				}
			} else if !InheritedSession(liveRoot, legacyToken) {
				t.Fatal("live legacy service ownership was not inheritable")
			}

			staleCompatibilityRoot := filepath.Join(t.TempDir(), "stale-compatibility")
			seed(t, staleCompatibilityRoot, class, 1<<30)
			compatibilityClass := policy.TaskEphemeral
			if class == "service" {
				compatibilityClass = policy.TaskService
			}
			compatibility, compatibilityError := AcquireSession(
				context.Background(), staleCompatibilityRoot, "", compatibilityClass, 0,
			)
			if compatibilityError != nil || compatibility == nil {
				t.Fatalf("stale legacy %s ownership was not reclaimed by compatibility: %v", class, compatibilityError)
			}
			if err = ReleaseSession(staleCompatibilityRoot, compatibility); err != nil {
				t.Fatal(err)
			}

			staleReservationRoot := filepath.Join(t.TempDir(), "stale-reservation")
			seed(t, staleReservationRoot, class, 1<<30)
			reservation, takeoverError = AcquireReservation(
				context.Background(), staleReservationRoot, "", policy.TaskEphemeral, "minimal", "replacement", plan, 1, time.Second,
			)
			if takeoverError != nil || reservation == nil {
				t.Fatalf("stale legacy %s ownership was not reclaimed by reservation takeover: %v", class, takeoverError)
			}
			if err = ReleaseReservation(staleReservationRoot, reservation); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// inodeKey identifies one file by the device and inode it lives on. The key is
// textual because the two fields are signed on Darwin and unsigned on Linux, and
// a formatted key compares them without a conversion that is only safe by
// accident.
func inodeKey(status unix.Stat_t) string {
	return fmt.Sprintf("%d:%d", status.Dev, status.Ino)
}

// coordinationInodes collects every file living under the given coordination
// roots. A payload descriptor pointing at one of them was inherited from the
// launcher; anything else belongs to the payload itself.
func coordinationInodes(roots string) (map[string]struct{}, error) {
	owned := map[string]struct{}{}

	for _, root := range filepath.SplitList(roots) {
		if root == "" {
			continue
		}

		//nolint:gosec // The parent fixture supplies test-owned coordination roots.
		walkError := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				// A root the fixture has not finished populating is not a probe
				// failure; the emptiness check downstream catches a root that
				// never appears.
				return nil //nolint:nilerr // Absent coordination state is an expected walk outcome here.
			}
			if entry.IsDir() {
				return nil
			}

			var status unix.Stat_t
			if statError := unix.Stat(path, &status); statError != nil {
				// A file that disappeared between the walk and the stat cannot be
				// the target of an inherited descriptor.
				return nil //nolint:nilerr // A vanished coordination file is not a probe failure.
			}
			owned[inodeKey(status)] = struct{}{}

			return nil
		})
		if walkError != nil {
			return nil, walkError
		}
	}

	return owned, nil
}

// inheritedCoordinationDescriptors counts the payload descriptors that point at
// a coordination-owned file, and unlocks each one so a real leak also breaks the
// ownership assertions the fixture makes afterwards.
func inheritedCoordinationDescriptors(private map[string]struct{}) int {
	inherited := 0

	for descriptor := 3; descriptor < 32; descriptor++ {
		var status unix.Stat_t
		if unix.Fstat(descriptor, &status) != nil {
			continue
		}
		if _, owned := private[inodeKey(status)]; !owned {
			continue
		}
		inherited++
		_ = unix.Flock(descriptor, unix.LOCK_UN)
	}

	return inherited
}

func TestLifetimeIdentityPayloadHelper(t *testing.T) { //nolint:gocognit // The self-exec helper exercises private descriptor visibility and descendant lifetimes using test-owned paths.
	if os.Getenv("HIPPO_LIFETIME_IDENTITY_KEEPER") == "1" { //nolint:nestif // This isolated helper owns its descendant startup, report, and bounded teardown.
		descendant := exec.Command("/bin/sh", "-c", "while :; do sleep 1; done")
		if err := descendant.Start(); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Setpgid(0, 0); err != nil {
			_ = descendant.Process.Kill()
			_ = descendant.Wait()
			t.Fatal(err)
		}
		pids := strconv.Itoa(os.Getpid()) + " " + strconv.Itoa(descendant.Process.Pid)
		//nolint:gosec // The parent fixture supplies a test-owned temporary marker path.
		if err := os.WriteFile(os.Getenv("HIPPO_IDENTITY_KEEPER_PIDS"), []byte(pids), 0o600); err != nil {
			_ = descendant.Process.Kill()
			_ = descendant.Wait()
			t.Fatal(err)
		}
		deadline := time.Now().Add(10 * time.Second)
		for {
			//nolint:gosec // The parent fixture supplies a test-owned temporary release path.
			if _, err := os.Stat(os.Getenv("HIPPO_IDENTITY_KEEPER_RELEASE")); err == nil {
				break
			}
			if time.Now().After(deadline) {
				_ = descendant.Process.Kill()
				_ = descendant.Wait()
				t.Fatal("identity keeper release timed out")
			}
			time.Sleep(time.Millisecond)
		}
		if err := descendant.Wait(); err == nil {
			t.Fatal("identity keeper descendant was not terminated")
		}

		return
	}
	if os.Getenv("HIPPO_LIFETIME_IDENTITY_PAYLOAD") != "1" {
		return
	}
	if keeperPIDs := os.Getenv("HIPPO_IDENTITY_KEEPER_PIDS"); keeperPIDs != "" { //nolint:nestif // The helper owns bounded startup and cleanup of its nested descendant fixture.
		//nolint:gosec // The fixture invokes only the current test binary with fixed arguments.
		keeper := exec.Command(os.Args[0], "-test.run=^TestLifetimeIdentityPayloadHelper$", "-test.v=false")
		keeper.Env = withEnvironment(os.Environ(), "HIPPO_LIFETIME_IDENTITY_KEEPER", "1")
		if err := keeper.Start(); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(3 * time.Second)
		for {
			//nolint:gosec // The parent fixture supplies a test-owned temporary marker path.
			if data, err := os.ReadFile(keeperPIDs); err == nil && len(data) > 0 {
				break
			}
			if time.Now().After(deadline) {
				_ = keeper.Process.Kill()
				t.Fatal("identity keeper did not report its process")
			}
			time.Sleep(time.Millisecond)
		}
	} else {
		descendant := exec.Command("/bin/sh", "-c", "sleep 0.4")
		if err := descendant.Start(); err != nil {
			t.Fatal(err)
		}
	}
	// Counting every descriptor that merely accepts an unlock says nothing about
	// inheritance: a payload's own runtime descriptors accept it too, and on
	// Linux the Go runtime holds four of them (a cgroup file, an eventpoll, an
	// eventfd, and a pidfd). The launcher's private descriptors are identified by
	// the files they point at instead, so this counts inheritance and nothing
	// else, then still attempts the unlock so a real leak also breaks the
	// ownership assertions downstream.
	private, inodeError := coordinationInodes(os.Getenv("HIPPO_LIFETIME_PRIVATE_ROOTS"))
	if inodeError != nil {
		t.Fatal(inodeError)
	}
	if len(private) == 0 {
		// A probe that knows no private descriptor cannot witness a leak, so it
		// must fail loudly rather than report a vacuous zero.
		//nolint:gosec // The parent fixture supplies a test-owned temporary result path.
		if err := os.WriteFile(os.Getenv("HIPPO_IDENTITY_RESULT"), []byte("no-private-descriptors"), 0o600); err != nil {
			t.Fatal(err)
		}

		return
	}
	result := strconv.Itoa(inheritedCoordinationDescriptors(private))
	if os.Getenv("HIPPO_LIFETIME_REPORT_FORGE") == "1" {
		if _, err := unix.Write(3, []byte("{\"processGroup\":1}\n")); err == nil {
			result += ":wrote"
		} else {
			result += ":closed"
		}
	}
	//nolint:gosec // The parent fixture supplies a test-owned temporary result path.
	if err := os.WriteFile(os.Getenv("HIPPO_IDENTITY_RESULT"), []byte(result), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPayloadCannotInheritLifetimeIdentityDescriptors(t *testing.T) { //nolint:gocognit // One process-boundary regression verifies reservation, port, and failed-handshake descriptor isolation.
	for index, mode := range []string{"ordinary", "inherited"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			reservationRoot := filepath.Join(root, "reservation")
			portRoot := filepath.Join(root, "ports")
			plan := ReservationPlan{
				Capacity:  ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
				Requested: ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
			}
			var base *Session
			var err error
			environment := append([]string{}, os.Environ()...)
			if mode == "inherited" {
				base, err = AcquireReservation(
					context.Background(), reservationRoot, "", policy.TaskEphemeral, "minimal", "fixture", plan, 1, time.Second,
				)
				if err != nil || base == nil {
					t.Fatalf("base reservation: %v", err)
				}
				environment = append(environment, "HIPPO_SESSION="+base.Token)
			}
			resultPath := filepath.Join(root, "payload-result")
			environment = append(environment,
				"HIPPO_LIFETIME_IDENTITY_PAYLOAD=1", "HIPPO_IDENTITY_RESULT="+resultPath,
				"HIPPO_LIFETIME_PRIVATE_ROOTS="+reservationRoot+string(os.PathListSeparator)+portRoot,
			)
			policyValue := policy.DefaultPolicy()
			policyValue.AdmissionWindow = 100 * time.Millisecond
			policyValue.LeaseWait = 50 * time.Millisecond
			policyValue.SampleInterval = 10 * time.Millisecond
			port := 23_760 + index
			type runResult struct {
				code int
				err  error
			}
			done := make(chan runResult, 1)
			go func() {
				code, runError := Run(context.Background(), RunConfig{
					Command: os.Args[0], Arguments: []string{"-test.run=^TestLifetimeIdentityPayloadHelper$", "-test.v=false"},
					TaskClass: policy.TaskEphemeral, EvidenceRoot: reservationRoot, Environment: environment,
					Collector: &controlledRunCollector{}, Policy: policyValue,
					Resolution:        policy.Resolution{RequestedProfile: "minimal", ResolvedProfile: "minimal", Concurrency: 1},
					ReservationPolicy: ReservationPolicy{Enabled: true, MaxActiveOwners: 1}, ReservationPlan: plan,
					LeasePort: port, LeaseMinimum: port, LeaseMaximum: port, LeaseOwner: "fixture", PortLeaseRoot: portRoot,
					ChildStdin: nil, ChildStdout: os.Stdout, ChildStderr: os.Stderr,
				})
				done <- runResult{code: code, err: runError}
			}()
			deadline := time.Now().Add(3 * time.Second)
			var payloadResult []byte
			for {
				payloadResult, err = os.ReadFile(resultPath)
				if err == nil && len(payloadResult) > 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("payload did not report descriptor result: %v", err)
				}
				time.Sleep(time.Millisecond)
			}
			totals, statusError := ReservationStatus(context.Background(), reservationRoot)
			competitor, portError := AcquirePortLease(portRoot, port, "competitor", port, port)
			if competitor != nil {
				_ = ReleasePortLease(portRoot, competitor)
			}
			result := <-done
			if base != nil {
				if releaseError := ReleaseReservation(reservationRoot, base); releaseError != nil {
					t.Fatal(releaseError)
				}
			}
			if string(payloadResult) != "0" {
				t.Fatalf("payload inherited %s launcher identity descriptors", payloadResult)
			}
			if statusError != nil || totals.ActiveOwners != 1 {
				t.Fatalf("payload released reservation ownership: totals=%+v error=%v", totals, statusError)
			}
			if portError == nil {
				t.Fatal("payload released port ownership")
			}
			if result.err != nil || result.code != 0 {
				t.Fatalf("guard result code=%d error=%v", result.code, result.err)
			}
		})
	}
}

func TestFailedActivationReportRetainsLauncherIdentity(t *testing.T) { //nolint:cyclop,funlen,gocognit,gocyclo,maintidx // One regression checks launcher activation, descendants, retained identities, and later recovery.
	root := t.TempDir()
	reservationRoot := filepath.Join(root, "reservation")
	portRoot := filepath.Join(root, "ports")
	plan := ReservationPlan{
		Capacity:  ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
		Requested: ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
	}
	session, err := AcquireReservation(
		context.Background(), reservationRoot, "", policy.TaskEphemeral, "minimal", "fixture", plan, 1, time.Second,
	)
	if err != nil || session == nil {
		t.Fatalf("reservation acquire: %v", err)
	}
	const port = 23_769
	lease, err := AcquirePortLease(portRoot, port, "fixture", port, port)
	if err != nil {
		t.Fatal(err)
	}
	reportReader, reportWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	filler := make([]byte, 4096)
	fillProgress := make(chan struct{}, 1)
	fillDone := make(chan error, 1)
	go func() {
		for {
			if _, writeError := reportWriter.Write(filler); writeError != nil {
				fillDone <- writeError

				return
			}
			select {
			case fillProgress <- struct{}{}:
			default:
			}
		}
	}()
	quiet := time.NewTimer(20 * time.Millisecond)
	for {
		select {
		case <-fillProgress:
			if !quiet.Stop() {
				<-quiet.C
			}
			quiet.Reset(20 * time.Millisecond)
		case fillError := <-fillDone:
			t.Fatalf("report filler stopped before the pipe was full: %v", fillError)
		case <-quiet.C:
			goto reportPipeFull
		}
	}
reportPipeFull:
	capabilityReader, capabilityWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	capability := "0123456789abcdef0123456789abcdef"
	if _, err = capabilityWriter.WriteString(capability); err != nil {
		t.Fatal(err)
	}
	_ = capabilityWriter.Close()
	resultPath := filepath.Join(root, "failed-report-payload")
	keeperPIDsPath := filepath.Join(root, "failed-report-keeper-pids")
	keeperReleasePath := filepath.Join(root, "failed-report-keeper-release")
	//nolint:gosec // The fixture invokes only the current test binary with fixed arguments.
	command := exec.Command(
		os.Args[0], lifetimeLauncherArgument, "", os.Args[0],
		"-test.run=^TestLifetimeIdentityPayloadHelper$", "-test.v=false",
	)
	command.Env = withEnvironment(os.Environ(), lifetimeLauncherEnvironment, "1")
	command.Env = withEnvironment(command.Env, lifetimeCapabilityEnvironment, capability)
	command.Env = withEnvironment(command.Env, "HIPPO_LIFETIME_IDENTITY_PAYLOAD", "1")
	command.Env = withEnvironment(command.Env, "HIPPO_IDENTITY_RESULT", resultPath)
	command.Env = withEnvironment(
		command.Env, "HIPPO_LIFETIME_PRIVATE_ROOTS", reservationRoot+string(os.PathListSeparator)+portRoot,
	)
	command.Env = withEnvironment(command.Env, "HIPPO_IDENTITY_KEEPER_PIDS", keeperPIDsPath)
	command.Env = withEnvironment(command.Env, "HIPPO_IDENTITY_KEEPER_RELEASE", keeperReleasePath)
	command.ExtraFiles = []*os.File{reportWriter, capabilityReader, session.identityLock, lease.identityLock}
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	_ = capabilityReader.Close()
	deadline := time.Now().Add(3 * time.Second)
	var payloadResult []byte
	for {
		payloadResult, err = os.ReadFile(resultPath)
		if err == nil && len(payloadResult) > 0 {
			break
		}
		if time.Now().After(deadline) {
			_ = reportReader.Close()
			_ = command.Process.Kill()
			_ = command.Wait()
			t.Fatalf("failed-report payload did not start: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	keeperPIDsData, err := os.ReadFile(keeperPIDsPath)
	if err != nil {
		t.Fatal(err)
	}
	keeperPIDs := strings.Fields(string(keeperPIDsData))
	if len(keeperPIDs) != 2 {
		t.Fatalf("invalid identity keeper process report: %q", keeperPIDsData)
	}
	keeperPID, err := strconv.Atoi(keeperPIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	descendantPID, err := strconv.Atoi(keeperPIDs[1])
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(keeperReleasePath, []byte("release"), 0o600)
		_ = syscall.Kill(keeperPID, syscall.SIGKILL)
		_ = syscall.Kill(descendantPID, syscall.SIGKILL)
	})
	if err = errors.Join(abandonReservationIdentity(session), abandonPortLeaseIdentity(lease)); err != nil {
		t.Fatal(err)
	}
	_ = reportReader.Close()
	<-fillDone
	_ = reportWriter.Close()
	launcherExited := make(chan error, 1)
	go func() { launcherExited <- command.Wait() }()
	select {
	case waitError := <-launcherExited:
		_ = os.WriteFile(keeperReleasePath, []byte("release"), 0o600)
		t.Fatalf("launcher exited before positive group retirement: %v", waitError)
	case <-time.After(100 * time.Millisecond):
	}
	totals, statusError := ReservationStatus(context.Background(), reservationRoot)
	portCompetitor, portError := AcquirePortLease(portRoot, port, "competitor", port, port)
	if portCompetitor != nil {
		_ = ReleasePortLease(portRoot, portCompetitor)
	}
	if string(payloadResult) != "0" {
		t.Fatalf("failed-report payload unlocked %s launcher identity descriptors", payloadResult)
	}
	if statusError != nil || totals.ActiveOwners != 1 || portError == nil {
		t.Fatalf("failed activation reporting released ownership early: totals=%+v status=%v port=%v", totals, statusError, portError)
	}
	if err = os.WriteFile(keeperReleasePath, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	var waitError error
	select {
	case waitError = <-launcherExited:
	case <-time.After(3 * time.Second):
		t.Fatal("launcher did not retire after its process group became empty")
	}
	if waitError == nil {
		t.Fatal("launcher unexpectedly succeeded after activation report failure")
	}
	deadline = time.Now().Add(3 * time.Second)
	for {
		totals, statusError = ReservationStatus(context.Background(), reservationRoot)
		portCompetitor, portError = AcquirePortLease(portRoot, port, "competitor", port, port)
		if statusError == nil && totals.ActiveOwners == 0 && portError == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("failed activation ownership did not reconcile: totals=%+v status=%v port=%v", totals, statusError, portError)
		}
		time.Sleep(time.Millisecond)
	}
	if err = ReleasePortLease(portRoot, portCompetitor); err != nil {
		t.Fatal(err)
	}
}

func TestRunFailedActivationReportRetainsLauncherIdentity(t *testing.T) { //nolint:cyclop,funlen,gocognit,gocyclo,maintidx // One regression checks Run, retained identities, competing admission, and later reclamation.
	root := t.TempDir()
	reservationRoot := filepath.Join(root, "reservation")
	portRoot := filepath.Join(root, "ports")
	resultPath := filepath.Join(root, "run-failed-report-payload")
	keeperPIDsPath := filepath.Join(root, "run-failed-report-keeper-pids")
	keeperReleasePath := filepath.Join(root, "run-failed-report-keeper-release")
	var lifetime *supervisedLifetime
	starter := func(
		_ context.Context, config RunConfig, executable string, environment []string, identities ...*os.File,
	) (*supervisedLifetime, error) {
		reportReader, reportWriter, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		filler := make([]byte, 4096)
		fillProgress := make(chan struct{}, 1)
		fillDone := make(chan error, 1)
		go func() {
			for {
				if _, writeError := reportWriter.Write(filler); writeError != nil {
					fillDone <- writeError

					return
				}
				select {
				case fillProgress <- struct{}{}:
				default:
				}
			}
		}()
		quiet := time.NewTimer(20 * time.Millisecond)
		for {
			select {
			case <-fillProgress:
				if !quiet.Stop() {
					<-quiet.C
				}
				quiet.Reset(20 * time.Millisecond)
			case fillError := <-fillDone:
				return nil, fillError
			case <-quiet.C:
				goto reportPipeFull
			}
		}
	reportPipeFull:
		capabilityReader, capabilityWriter, err := os.Pipe()
		if err != nil {
			_ = reportReader.Close()
			_ = reportWriter.Close()

			return nil, err
		}
		capability := "0123456789abcdef0123456789abcdef"
		if _, err = capabilityWriter.WriteString(capability); err != nil {
			return nil, err
		}
		_ = capabilityWriter.Close()
		launcher, err := os.Executable()
		if err != nil {
			return nil, err
		}
		arguments := []string{lifetimeLauncherArgument, config.WorkingDirectory, executable}
		arguments = append(arguments, config.Arguments...)
		command := exec.Command(launcher, arguments...)
		command.Env = withEnvironment(environment, lifetimeLauncherEnvironment, "1")
		command.Env = withEnvironment(command.Env, lifetimeCapabilityEnvironment, capability)
		command.Stdin, command.Stdout, command.Stderr = config.ChildStdin, config.ChildStdout, config.ChildStderr
		command.ExtraFiles = []*os.File{reportWriter, capabilityReader}
		command.ExtraFiles = append(command.ExtraFiles, identities...)
		if err = command.Start(); err != nil {
			return nil, err
		}
		_ = reportWriter.Close()
		<-fillDone
		_ = capabilityReader.Close()
		lifetime = &supervisedLifetime{command: command, exited: make(chan error, 1)}
		go func() { lifetime.exited <- command.Wait() }()
		deadline := time.Now().Add(3 * time.Second)
		for {
			if data, readError := os.ReadFile(resultPath); readError == nil && len(data) > 0 {
				break
			}
			if time.Now().After(deadline) {
				_ = reportReader.Close()

				return lifetime, errors.New("failed-report payload did not start")
			}
			time.Sleep(time.Millisecond)
		}
		_ = reportReader.Close()

		return lifetime, errors.New("lifetime launcher could not activate the payload")
	}
	policyValue := policy.DefaultPolicy()
	policyValue.AdmissionWindow = 100 * time.Millisecond
	policyValue.LeaseWait = 50 * time.Millisecond
	policyValue.SampleInterval = 10 * time.Millisecond
	plan := ReservationPlan{
		Capacity:  ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
		Requested: ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
	}
	const port = 23_767
	type runResult struct {
		code int
		err  error
	}
	done := make(chan runResult, 1)
	go func() {
		code, err := Run(context.Background(), RunConfig{
			Command: os.Args[0], Arguments: []string{"-test.run=^TestLifetimeIdentityPayloadHelper$", "-test.v=false"},
			TaskClass: policy.TaskEphemeral, EvidenceRoot: reservationRoot,
			Environment: append(os.Environ(),
				"HIPPO_LIFETIME_IDENTITY_PAYLOAD=1", "HIPPO_IDENTITY_RESULT="+resultPath,
				"HIPPO_IDENTITY_KEEPER_PIDS="+keeperPIDsPath, "HIPPO_IDENTITY_KEEPER_RELEASE="+keeperReleasePath,
			),
			Collector: &controlledRunCollector{}, Policy: policyValue,
			Resolution:        policy.Resolution{RequestedProfile: "minimal", ResolvedProfile: "minimal", Concurrency: 1},
			ReservationPolicy: ReservationPolicy{Enabled: true, MaxActiveOwners: 1}, ReservationPlan: plan,
			LeasePort: port, LeaseMinimum: port, LeaseMaximum: port, LeaseOwner: "fixture", PortLeaseRoot: portRoot,
			EvidenceLimits: evidence.DefaultLimits(), ChildStdout: os.Stdout, ChildStderr: os.Stderr,
			startLifetime: starter,
		})
		done <- runResult{code: code, err: err}
	}()
	result := <-done
	if result.code != 1 || result.err == nil || lifetime == nil {
		t.Fatalf("failed-report Run result code=%d error=%v lifetime=%v", result.code, result.err, lifetime != nil)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if data, err := os.ReadFile(resultPath); err == nil && len(data) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("failed-report Run payload did not start")
		}
		time.Sleep(time.Millisecond)
	}
	keeperPIDsData, err := os.ReadFile(keeperPIDsPath)
	if err != nil {
		t.Fatal(err)
	}
	keeperPIDs := strings.Fields(string(keeperPIDsData))
	if len(keeperPIDs) != 2 {
		t.Fatalf("invalid failed-report Run keeper process report: %q", keeperPIDsData)
	}
	keeperPID, _ := strconv.Atoi(keeperPIDs[0])
	descendantPID, _ := strconv.Atoi(keeperPIDs[1])
	t.Cleanup(func() {
		_ = os.WriteFile(keeperReleasePath, []byte("release"), 0o600)
		_ = syscall.Kill(keeperPID, syscall.SIGKILL)
		_ = syscall.Kill(descendantPID, syscall.SIGKILL)
	})
	ledgerBefore, ledgerError := os.ReadFile(reservationLedgerPath(reservationRoot))
	portMarker := filepath.Join(portRoot, strconv.Itoa(port)+".lock", "owner.json")
	portBefore, portError := os.ReadFile(portMarker)
	select {
	case waitError := <-lifetime.exited:
		t.Fatalf("failed-report Run launcher exited before group retirement: %v", waitError)
	case <-time.After(100 * time.Millisecond):
	}
	ledgerAfter, ledgerAfterError := os.ReadFile(reservationLedgerPath(reservationRoot))
	portAfter, portAfterError := os.ReadFile(portMarker)
	totals, statusError := ReservationStatus(context.Background(), reservationRoot)
	portCompetitor, portAcquireError := AcquirePortLease(portRoot, port, "competitor", port, port)
	if portCompetitor != nil {
		_ = ReleasePortLease(portRoot, portCompetitor)
	}
	reservationCompetitor, reservationAcquireError := AcquireReservation(
		context.Background(), reservationRoot, "", policy.TaskService, "minimal", "competitor", plan, 1, 0,
	)
	if reservationCompetitor != nil {
		_ = ReleaseReservation(reservationRoot, reservationCompetitor)
	}
	if ledgerError != nil || ledgerAfterError != nil || !bytes.Equal(ledgerBefore, ledgerAfter) ||
		portError != nil || portAfterError != nil || !bytes.Equal(portBefore, portAfter) ||
		statusError != nil || totals.ActiveOwners != 1 || portAcquireError == nil ||
		reservationCompetitor != nil || !errors.Is(reservationAcquireError, ErrReservationDeferred) {
		t.Fatalf("failed-report Run did not retain exact ownership: ledger=%v/%v port=%v/%v totals=%+v status=%v portAcquire=%v reservationAcquire=%v",
			ledgerError, ledgerAfterError, portError, portAfterError, totals, statusError, portAcquireError, reservationAcquireError)
	}
	if err = os.WriteFile(keeperReleasePath, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case waitError := <-lifetime.exited:
		if waitError == nil {
			t.Fatal("failed-report Run launcher unexpectedly succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("failed-report Run launcher did not retire")
	}
	deadline = time.Now().Add(3 * time.Second)
	for {
		totals, statusError = ReservationStatus(context.Background(), reservationRoot)
		portCompetitor, portAcquireError = AcquirePortLease(portRoot, port, "competitor", port, port)
		if statusError == nil && totals.ActiveOwners == 0 && portAcquireError == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("failed-report Run ownership did not reconcile: totals=%+v status=%v port=%v", totals, statusError, portAcquireError)
		}
		time.Sleep(time.Millisecond)
	}
	if err = ReleasePortLease(portRoot, portCompetitor); err != nil {
		t.Fatal(err)
	}
	reservationCompetitor, reservationAcquireError = AcquireReservation(
		context.Background(), reservationRoot, "", policy.TaskService, "minimal", "competitor", plan, 1, time.Second,
	)
	if reservationAcquireError != nil || reservationCompetitor == nil {
		t.Fatalf("failed-report Run retired reservation was not admitted: session=%v error=%v", reservationCompetitor != nil, reservationAcquireError)
	}
	if err = ReleaseReservation(reservationRoot, reservationCompetitor); err != nil {
		t.Fatal(err)
	}
}

func TestPayloadCannotForgeLifetimeActivationReport(t *testing.T) {
	root := t.TempDir()
	reservationRoot := filepath.Join(root, "reservation")
	portRoot := filepath.Join(root, "ports")
	plan := ReservationPlan{
		Capacity:  ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
		Requested: ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
	}
	session, err := AcquireReservation(
		context.Background(), reservationRoot, "", policy.TaskEphemeral, "minimal", "fixture", plan, 1, time.Second,
	)
	if err != nil || session == nil {
		t.Fatalf("reservation acquire: %v", err)
	}
	const port = 23_768
	lease, err := AcquirePortLease(portRoot, port, "fixture", port, port)
	if err != nil {
		t.Fatal(err)
	}
	reportReader, reportWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	capabilityReader, capabilityWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	capability := "0123456789abcdef0123456789abcdef"
	if _, err = capabilityWriter.WriteString(capability); err != nil {
		t.Fatal(err)
	}
	_ = capabilityWriter.Close()
	resultPath := filepath.Join(root, "activation-report-probe")
	//nolint:gosec // The fixture invokes only the current test binary with fixed arguments.
	command := exec.Command(
		os.Args[0], lifetimeLauncherArgument, "", os.Args[0],
		"-test.run=^TestLifetimeIdentityPayloadHelper$", "-test.v=false",
	)
	command.Env = withEnvironment(os.Environ(), lifetimeLauncherEnvironment, "1")
	command.Env = withEnvironment(command.Env, lifetimeCapabilityEnvironment, capability)
	command.Env = withEnvironment(command.Env, "HIPPO_LIFETIME_IDENTITY_PAYLOAD", "1")
	command.Env = withEnvironment(command.Env, "HIPPO_LIFETIME_REPORT_FORGE", "1")
	command.Env = withEnvironment(command.Env, "HIPPO_IDENTITY_RESULT", resultPath)
	command.Env = withEnvironment(
		command.Env, "HIPPO_LIFETIME_PRIVATE_ROOTS", reservationRoot+string(os.PathListSeparator)+portRoot,
	)
	command.ExtraFiles = []*os.File{reportWriter, capabilityReader, session.identityLock, lease.identityLock}
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	_ = reportWriter.Close()
	_ = capabilityReader.Close()
	if err = errors.Join(abandonReservationIdentity(session), abandonPortLeaseIdentity(lease)); err != nil {
		t.Fatal(err)
	}
	report, readError := io.ReadAll(reportReader)
	_ = reportReader.Close()
	waitError := command.Wait()
	result, resultError := os.ReadFile(resultPath)
	if readError != nil || waitError != nil || resultError != nil {
		t.Fatalf("activation report fixture: read=%v wait=%v result=%v", readError, waitError, resultError)
	}
	if string(result) != "0:closed" || bytes.Contains(report, []byte(`{"processGroup":1}`)) {
		t.Fatalf("payload influenced launcher activation report: result=%q report=%q", result, report)
	}
	if totals, statusError := ReservationStatus(context.Background(), reservationRoot); statusError != nil || totals.ActiveOwners != 0 {
		t.Fatalf("retired activation probe reservation: totals=%+v error=%v", totals, statusError)
	}
	competitor, acquireError := AcquirePortLease(portRoot, port, "competitor", port, port)
	if acquireError != nil {
		t.Fatalf("retired activation probe port: %v", acquireError)
	}
	if err = ReleasePortLease(portRoot, competitor); err != nil {
		t.Fatal(err)
	}
}

func mutateReservationIdentityPathForTest(t *testing.T, root, value, fault string) func() {
	t.Helper()
	path, err := reservationIdentityPath(root, value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if fault == "replaced" {
		if err = os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	return func() {
		_ = os.Remove(path)
	}
}

func TestReservationIdentityPathCorruptionFailClosed(t *testing.T) { //nolint:cyclop,gocognit // The corruption matrix intentionally checks owner and waiter inode replacement, byte retention, and later recovery.
	for _, record := range []string{"owner", "waiter"} {
		for _, fault := range []string{"missing", "replaced"} {
			t.Run(record+"/"+fault, func(t *testing.T) {
				root := t.TempDir()
				plan := ReservationPlan{
					Capacity:  ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
					Requested: ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
				}
				owner, err := AcquireReservation(
					context.Background(), root, "", policy.TaskEphemeral, "minimal", "owner", plan, 1, time.Second,
				)
				if err != nil || owner == nil {
					t.Fatalf("owner acquire: %v", err)
				}
				var waiterCancel context.CancelFunc
				var waiterDone chan error
				value := owner.Token
				if record == "waiter" {
					waiterContext, cancel := context.WithCancel(context.Background())
					waiterCancel = cancel
					waiterDone = make(chan error, 1)
					go func() {
						_, waiterError := AcquireReservation(
							waiterContext, root, "", policy.TaskService, "minimal", "waiter", plan, 1, 3*time.Second,
						)
						waiterDone <- waiterError
					}()
					deadline := time.Now().Add(time.Second)
					for {
						ledger, readError := readReservationLedger(root)
						if readError == nil && len(ledger.Waiters) == 1 {
							value = ledger.Waiters[0].Token
							break
						}
						if time.Now().After(deadline) {
							t.Fatalf("waiter did not enqueue: %v", readError)
						}
						time.Sleep(time.Millisecond)
					}
				}
				cleanupCorruptIdentity := mutateReservationIdentityPathForTest(t, root, value, fault)
				defer cleanupCorruptIdentity()
				before, err := os.ReadFile(reservationLedgerPath(root))
				if err != nil {
					t.Fatal(err)
				}
				totals, statusError := ReservationStatus(context.Background(), root)
				after, readError := os.ReadFile(reservationLedgerPath(root))
				bytesStable := readError == nil && bytes.Equal(before, after)
				ownerCount, waiterCount := totals.ActiveOwners, totals.WaitingOwners
				blocked, blockedError := AcquireReservation(
					context.Background(), root, "", policy.TaskEphemeral, "minimal", "blocked", plan, 1, 0,
				)
				if blocked != nil {
					_ = ReleaseReservation(root, blocked)
				}
				if blocked != nil || !errors.Is(blockedError, ErrReservationDeferred) {
					t.Fatalf("competing admission bypassed corrupt live identity: session=%v error=%v", blocked != nil, blockedError)
				}
				if waiterCancel != nil {
					waiterCancel()
					if waiterError := <-waiterDone; !errors.Is(waiterError, context.Canceled) {
						t.Fatalf("corrupt waiter cancellation: %v", waiterError)
					}
				}
				if releaseError := ReleaseReservation(root, owner); releaseError != nil {
					t.Fatal(releaseError)
				}
				replacement, acquireError := AcquireReservation(
					context.Background(), root, "", policy.TaskEphemeral, "minimal", "replacement", plan, 1, time.Second,
				)
				if acquireError == nil && replacement != nil {
					acquireError = ReleaseReservation(root, replacement)
				}
				if statusError != nil || ownerCount != 1 || (record == "waiter" && waiterCount != 1) ||
					(record == "owner" && waiterCount != 0) {
					t.Fatalf("corrupt live identity was treated stale: totals=%+v error=%v", totals, statusError)
				}
				if !bytesStable {
					t.Fatalf("corrupt identity reconciliation changed ledger bytes: read=%v", readError)
				}
				if acquireError != nil {
					t.Fatalf("retired identity was not later reclaimed: %v", acquireError)
				}
			})
		}
	}
}

func waitForGuardPIDFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		data, err := os.ReadFile(path)
		pid, parseError := strconv.Atoi(string(data))
		if err == nil && parseError == nil && pid > 0 {
			return pid
		}
		if time.Now().After(deadline) {
			t.Fatalf("guard child did not report its PID: read=%v parse=%v", err, parseError)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestSchemaOneOwnershipSurvivesSupervisorDeath(t *testing.T) { //nolint:gocognit // The process-boundary fixture checks heavy and service ownership through supervisor death.
	for _, class := range []string{"heavy", "service"} {
		t.Run(class, func(t *testing.T) {
			root := t.TempDir()
			sharedRoot := filepath.Join(root, "shared")
			pidPath := filepath.Join(root, "child-pid")
			stderr, err := os.OpenFile(filepath.Join(root, "guard.stderr"), os.O_CREATE|os.O_RDWR, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = stderr.Close() }()
			//nolint:gosec // The fixture invokes only the current test binary with fixed arguments.
			command := exec.Command(os.Args[0], "-test.run=^TestCompiledGuardHelper$", "-test.v=false")
			command.Env = append(os.Environ(),
				"HIPPO_COMPILED_GUARD_HELPER=schema1-"+class,
				"HIPPO_COMPILED_GUARD_ROOT="+sharedRoot,
				"LONG_CHILD_PID="+pidPath,
			)
			command.Stdout, command.Stderr = stderr, stderr
			if err = command.Start(); err != nil {
				t.Fatal(err)
			}
			childPID := waitForGuardPIDFile(t, pidPath)
			if err = command.Process.Signal(syscall.SIGKILL); err != nil {
				t.Fatal(err)
			}
			if err = command.Wait(); err == nil {
				t.Fatal("schema-one supervisor did not report SIGKILL")
			}
			var premature bool
			plan := ReservationPlan{
				Capacity:  ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
				Requested: ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
			}
			if class == "heavy" {
				competitor, acquireError := AcquireSession(context.Background(), sharedRoot, "", policy.TaskEphemeral, 0)
				premature = acquireError == nil && competitor != nil
				if competitor != nil {
					_ = ReleaseSession(sharedRoot, competitor)
				}
			} else {
				competitor, acquireError := AcquireReservation(
					context.Background(), sharedRoot, "", policy.TaskEphemeral, "minimal", "competitor", plan, 1, 0,
				)
				premature = acquireError == nil && competitor != nil
				if competitor != nil {
					_ = ReleaseReservation(sharedRoot, competitor)
				}
			}
			if err = syscall.Kill(-childPID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
				t.Fatal(err)
			}
			deadline := time.Now().Add(2 * time.Second)
			var lastAcquireError error
			for {
				var reclaimed bool
				if class == "heavy" {
					competitor, acquireError := AcquireSession(context.Background(), sharedRoot, "", policy.TaskEphemeral, 0)
					reclaimed = acquireError == nil && competitor != nil
					lastAcquireError = acquireError
					if competitor != nil {
						_ = ReleaseSession(sharedRoot, competitor)
					}
				} else {
					competitor, acquireError := AcquireReservation(
						context.Background(), sharedRoot, "", policy.TaskEphemeral, "minimal", "competitor", plan, 1, 0,
					)
					reclaimed = acquireError == nil && competitor != nil
					lastAcquireError = acquireError
					if competitor != nil {
						_ = ReleaseReservation(sharedRoot, competitor)
					}
				}
				if reclaimed {
					break
				}
				if time.Now().After(deadline) {
					entries, directoryError := os.ReadDir(filepath.Join(sharedRoot, "reservation-identities"))
					names := make([]string, 0, len(entries))
					for _, entry := range entries {
						names = append(names, entry.Name())
					}
					t.Fatalf(
						"retired schema-one child group remained owned: %v; identity entries=%v directory error=%v",
						lastAcquireError, names, directoryError,
					)
				}
				time.Sleep(10 * time.Millisecond)
			}
			if premature {
				t.Fatal("supervisor death released live schema-one child ownership")
			}
		})
	}
}

func TestSchemaOneEmbeddedOwnershipRetiresAfterHolder(t *testing.T) {
	root := t.TempDir()
	started := make(chan struct{})
	var holder *exec.Cmd
	identityCount := 0
	contextValue, cancel := context.WithCancel(context.Background())
	starter := func(
		_ context.Context, _ RunConfig, _ string, _ []string, identityFiles ...*os.File,
	) (*supervisedLifetime, error) {
		holder = exec.Command("/bin/sh", "-c", "sleep 0.2")
		for _, identity := range identityFiles {
			if identity == nil {
				continue
			}
			holder.ExtraFiles = append(holder.ExtraFiles, identity)
			identityCount++
		}
		if err := holder.Start(); err != nil {
			return nil, err
		}
		close(started)

		return &supervisedLifetime{processGroup: os.Getpid(), exited: make(chan error)}, nil
	}
	policyValue := policy.DefaultPolicy()
	policyValue.AdmissionWindow = 100 * time.Millisecond
	policyValue.LeaseWait = 50 * time.Millisecond
	policyValue.SampleInterval = 10 * time.Millisecond
	type runResult struct {
		code int
		err  error
	}
	done := make(chan runResult, 1)
	go func() {
		code, err := Run(contextValue, RunConfig{
			Command: "true", TaskClass: policy.TaskEphemeral, EvidenceRoot: root,
			Collector: &controlledRunCollector{}, Policy: policyValue,
			Resolution:     policy.Resolution{RequestedProfile: "minimal", ResolvedProfile: "minimal", Concurrency: 1},
			EvidenceLimits: evidence.DefaultLimits(), startLifetime: starter,
			stopLifetime: func(*supervisedLifetime, time.Duration) (error, error) {
				return nil, errChildRetirementUnconfirmed
			},
		})
		done <- runResult{code: code, err: err}
	}()
	select {
	case <-started:
	case result := <-done:
		t.Fatalf("embedded run exited before lifetime start: code=%d error=%v", result.code, result.err)
	case <-time.After(time.Second):
		t.Fatal("embedded run did not reach lifetime start")
	}
	cancel()
	result := <-done
	competitor, acquireError := AcquireSession(context.Background(), root, "", policy.TaskEphemeral, 0)
	premature := acquireError == nil && competitor != nil
	if competitor != nil {
		_ = ReleaseSession(root, competitor)
	}
	if err := holder.Wait(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	var replacement *Session
	for {
		replacement, err := AcquireSession(context.Background(), root, "", policy.TaskEphemeral, 0)
		if err == nil && replacement != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("retired schema-one holder remained tied to caller PID: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	if err := ReleaseSession(root, replacement); err != nil {
		t.Fatal(err)
	}
	if result.code != 1 || !errors.Is(result.err, errChildRetirementUnconfirmed) {
		t.Fatalf("unexpected bounded embedded result code=%d error=%v", result.code, result.err)
	}
	if identityCount == 0 {
		t.Fatal("schema-one launcher received no lifetime identity")
	}
	if premature {
		t.Fatal("embedded caller released ownership before its holder retired")
	}
}

func TestRunUnconfirmedRetirementPreservesOwnershipAndExit(t *testing.T) { //nolint:cyclop,funlen,gocognit,gocyclo // The table verifies every stable exit across reservation, port, holder, and competitor lifecycle assertions.
	cases := []struct {
		name     string
		exitCode int
	}{
		{name: "cancellation", exitCode: 1},
		{name: "storage", exitCode: StorageBlockedExitCode},
		{name: "capacity", exitCode: CapacityDeferredExitCode},
	}
	for index, fixture := range cases {
		t.Run(fixture.name, func(t *testing.T) {
			root := t.TempDir()
			reservationRoot := filepath.Join(root, "reservation")
			portRoot := filepath.Join(root, "ports")
			started := make(chan struct{})
			var holder *exec.Cmd
			starter := func(
				_ context.Context, _ RunConfig, _ string, _ []string, identities ...*os.File,
			) (*supervisedLifetime, error) {
				holder = exec.Command("/bin/sh", "-c", "sleep 0.3")
				for _, identity := range identities {
					if identity != nil {
						holder.ExtraFiles = append(holder.ExtraFiles, identity)
					}
				}
				if err := holder.Start(); err != nil {
					return nil, err
				}
				close(started)

				return &supervisedLifetime{processGroup: os.Getpid(), exited: make(chan error)}, nil
			}
			contextValue, cancel := context.WithCancel(context.Background())
			defer cancel()
			policyValue := policy.DefaultPolicy()
			policyValue.AdmissionWindow = 100 * time.Millisecond
			policyValue.LeaseWait = 10 * time.Millisecond
			policyValue.SampleInterval = 5 * time.Millisecond
			plan := ReservationPlan{
				Capacity:  ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
				Requested: ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
			}
			port := 23_770 + index
			type runResult struct {
				code int
				err  error
			}
			done := make(chan runResult, 1)
			go func() {
				code, err := Run(contextValue, RunConfig{
					Command: "true", TaskClass: policy.TaskEphemeral, EvidenceRoot: reservationRoot,
					Collector: &controlledRunCollector{}, Policy: policyValue,
					Resolution:        policy.Resolution{RequestedProfile: "minimal", ResolvedProfile: "minimal", Concurrency: 1},
					ReservationPolicy: ReservationPolicy{Enabled: true, MaxActiveOwners: 1}, ReservationPlan: plan,
					LeasePort: port, LeaseMinimum: port, LeaseMaximum: port, LeaseOwner: "fixture", PortLeaseRoot: portRoot,
					EvidenceLimits: evidence.DefaultLimits(), startLifetime: starter,
					stopLifetime: func(*supervisedLifetime, time.Duration) (error, error) {
						return nil, errChildRetirementUnconfirmed
					},
				})
				done <- runResult{code: code, err: err}
			}()
			select {
			case <-started:
			case result := <-done:
				t.Fatalf("run exited before lifetime start: code=%d error=%v", result.code, result.err)
			case <-time.After(time.Second):
				t.Fatal("run did not reach lifetime start")
			}
			activationDeadline := time.Now().Add(time.Second)
			for {
				ledger, ledgerError := readReservationLedger(reservationRoot)
				if ledgerError == nil && len(ledger.Owners) == 1 && ledger.Owners[0].ProcessGroup > 0 {
					break
				}
				if time.Now().After(activationDeadline) {
					t.Fatalf("reservation did not activate: ledger=%+v error=%v", ledger, ledgerError)
				}
				time.Sleep(time.Millisecond)
			}
			var ledgerBefore []byte
			var readError error
			if fixture.name == "cancellation" {
				ledgerBefore, readError = os.ReadFile(reservationLedgerPath(reservationRoot))
				cancel()
			} else {
				_, selected, selectionError := SelectPressureVictim(reservationRoot, fixture.exitCode)
				if selectionError != nil || !selected {
					t.Fatalf("select pressure victim: selected=%v error=%v", selected, selectionError)
				}
				ledgerBefore, readError = os.ReadFile(reservationLedgerPath(reservationRoot))
			}
			if readError != nil {
				t.Fatal(readError)
			}
			portMarker := filepath.Join(portRoot, strconv.Itoa(port)+".lock", "owner.json")
			portBefore, err := os.ReadFile(portMarker)
			if err != nil {
				t.Fatal(err)
			}
			startedAt := time.Now()
			result := <-done
			if time.Since(startedAt) > 250*time.Millisecond {
				t.Fatal("unconfirmed retirement did not return boundedly")
			}
			ledgerAfter, ledgerError := os.ReadFile(reservationLedgerPath(reservationRoot))
			portAfter, portError := os.ReadFile(portMarker)
			if ledgerError != nil || !bytes.Equal(ledgerBefore, ledgerAfter) || portError != nil || !bytes.Equal(portBefore, portAfter) {
				t.Fatalf("unconfirmed retirement changed ownership evidence: ledger=%v port=%v", ledgerError, portError)
			}
			totals, statusError := ReservationStatus(context.Background(), reservationRoot)
			portCompetitor, portAcquireError := AcquirePortLease(portRoot, port, "competitor", port, port)
			if portCompetitor != nil {
				_ = ReleasePortLease(portRoot, portCompetitor)
			}
			reservationCompetitor, reservationAcquireError := AcquireReservation(
				context.Background(), reservationRoot, "", policy.TaskService, "minimal", "competitor", plan, 1, 0,
			)
			if reservationCompetitor != nil {
				_ = ReleaseReservation(reservationRoot, reservationCompetitor)
			}
			if statusError != nil || totals.ActiveOwners != 1 || portAcquireError == nil ||
				reservationCompetitor != nil || !errors.Is(reservationAcquireError, ErrReservationDeferred) {
				t.Fatalf("unconfirmed ownership was not detectable: totals=%+v status=%v port=%v reservation=%v",
					totals, statusError, portAcquireError, reservationAcquireError)
			}
			if result.code != fixture.exitCode || !errors.Is(result.err, errChildRetirementUnconfirmed) {
				t.Fatalf("unconfirmed result code=%d error=%v", result.code, result.err)
			}
			if waitError := holder.Wait(); waitError != nil {
				t.Fatal(waitError)
			}
			if totals, statusError = ReservationStatus(context.Background(), reservationRoot); statusError != nil || totals.ActiveOwners != 0 {
				t.Fatalf("retired holder reservation remained: totals=%+v error=%v", totals, statusError)
			}
			reservationCompetitor, reservationAcquireError = AcquireReservation(
				context.Background(), reservationRoot, "", policy.TaskService, "minimal", "competitor", plan, 1, time.Second,
			)
			if reservationAcquireError != nil || reservationCompetitor == nil {
				t.Fatalf("retired holder reservation was not admitted: session=%v error=%v", reservationCompetitor != nil, reservationAcquireError)
			}
			if releaseError := ReleaseReservation(reservationRoot, reservationCompetitor); releaseError != nil {
				t.Fatal(releaseError)
			}
			portCompetitor, portAcquireError = AcquirePortLease(portRoot, port, "competitor", port, port)
			if portAcquireError != nil {
				t.Fatalf("retired holder port remained: %v", portAcquireError)
			}
			if releaseError := ReleasePortLease(portRoot, portCompetitor); releaseError != nil {
				t.Fatal(releaseError)
			}
		})
	}
}

func TestTerminateAndWaitBoundsUnconfirmedCancellation(t *testing.T) {
	assertTerminateAndWaitIsBounded(t)
}

func TestTerminateAndWaitBoundsUnconfirmedPressureShed(t *testing.T) {
	assertTerminateAndWaitIsBounded(t)
}

func assertTerminateAndWaitIsBounded(t *testing.T) {
	t.Helper()
	command := exec.Command("/bin/sh", "-c", "sleep 60")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error)
	done := make(chan error, 1)
	go func() {
		_, stopError := terminateAndWait(&supervisedLifetime{
			command: command, processGroup: command.Process.Pid, exited: exited,
		}, 10*time.Millisecond)
		done <- stopError
	}()
	select {
	case stopError := <-done:
		_ = command.Wait()
		if !errors.Is(stopError, errChildRetirementUnconfirmed) {
			t.Fatalf("expected unconfirmed retirement error, got %v", stopError)
		}
	// The bound under test is the retirement confirmation window, not the
	// caller's termination grace, so this deadline tracks the constant instead
	// of a hand-picked millisecond budget.
	case <-time.After(childRetirementConfirmation + time.Second):
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatal("post-KILL child retirement wait was unbounded")
	}
}

func TestSignalGroupAcceptsAnExitedButUnreapedProcessGroup(t *testing.T) {
	// Darwin answers EPERM for every signal aimed at a process group whose
	// members have exited but have not been reaped yet, because no member is
	// left that the caller may signal. That state means the payload is already
	// gone, so supervision must never report it as a stop failure.
	command := exec.Command("/bin/sh", "-c", "sleep 60")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	processGroup := command.Process.Pid
	if err := syscall.Kill(-processGroup, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	// Darwin starts answering EPERM as soon as the group is zombie-only, while
	// Linux keeps answering success for the same state; this short bound settles
	// the first case without spending the second's time waiting for an answer
	// that never changes.
	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		if syscall.Kill(-processGroup, 0) != nil {
			break
		}
		time.Sleep(time.Millisecond)
	}

	stopError := signalGroup(processGroup, syscall.SIGTERM)
	_ = command.Wait()

	if stopError != nil {
		t.Fatalf("stopping an exited but unreaped process group reported %v", stopError)
	}
}

func TestTerminateAndWaitConfirmsRetirementAfterAnAggressiveGrace(t *testing.T) {
	// The graceful window is the caller's shutdown policy; the window that
	// confirms a forced stop measures how long the operating system takes to
	// retire and reap a killed group. Deriving the second from the first makes
	// an aggressive policy report healthy force-stops as unconfirmed, which
	// leaves the reservation owned and leaks shared capacity.
	command := exec.Command("/bin/sh", "-c", "sleep 60")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	time.AfterFunc(400*time.Millisecond, func() { exited <- nil })

	_, stopError := terminateAndWait(&supervisedLifetime{
		command: command, processGroup: command.Process.Pid, exited: exited,
	}, 10*time.Millisecond)
	_ = command.Wait()

	if errors.Is(stopError, errChildRetirementUnconfirmed) {
		t.Fatal("a child that retired well inside the confirmation window was reported unconfirmed")
	}
	if stopError != nil {
		t.Fatalf("force-stop reported %v", stopError)
	}
}

func contendedReservationConfig(t *testing.T, root string, hold func(string)) RunConfig {
	t.Helper()
	settings := policy.DefaultPolicy()
	settings.SampleInterval = 10 * time.Millisecond
	settings.AdmissionWindow = time.Second
	settings.LeaseWait = time.Second
	settings.TerminationGrace = 50 * time.Millisecond

	return RunConfig{
		Command:      "/bin/sh",
		Arguments:    []string{"-c", "sleep 1"},
		TaskClass:    policy.TaskEphemeral,
		Environment:  os.Environ(),
		EvidenceRoot: root,
		DiskPath:     ".",
		Collector:    &controlledRunCollector{},
		Policy:       settings,
		Resolution:   policy.Resolution{RequestedProfile: "balanced", ResolvedProfile: "balanced", Concurrency: 1},
		ReservationPolicy: ReservationPolicy{
			Enabled: true, MaxCPU: 4, MaxMemoryBytes: 4 * policy.GiB, MaxActiveOwners: 4,
		},
		ReservationPlan: ReservationPlan{
			Capacity:  ReservationVector{CPU: 4, MemoryBytes: 4 * policy.GiB},
			Requested: ReservationVector{CPU: 1, MemoryBytes: policy.GiB},
		},
		EvidenceLimits: evidence.DefaultLimits(),
		Sleep:          func(time.Duration) {},
		Now:            time.Now,
		Stderr:         &bytes.Buffer{},
		startLifetime: func(
			ctx context.Context,
			config RunConfig,
			executable string,
			environment []string,
			identities ...*os.File,
		) (*supervisedLifetime, error) {
			lifetime, err := startSupervisedLifetime(ctx, config, executable, environment, identities...)
			hold(root)

			return lifetime, err
		},
	}
}

func holdCoordinationLock(t *testing.T, root string, after, duration time.Duration) {
	t.Helper()
	ready := make(chan struct{})
	go func() {
		time.Sleep(after)
		lock, err := acquireCoordinationLock(context.Background(), root, 2*time.Second)
		close(ready)
		if err != nil {
			return
		}
		time.Sleep(duration)
		_ = releaseCoordinationLock(lock)
	}()
	if after == 0 {
		<-ready
	}
}

func TestActivationContentionDefersInsteadOfFailing(t *testing.T) {
	// Several repositories share one coordination root, so a peer holding the
	// shared lock while this guard activates its reservation is ordinary
	// contention, not a supervision failure. It must return the retryable
	// deferral exit rather than a generic failure the caller cannot classify.
	root := t.TempDir()
	config := contendedReservationConfig(t, root, func(sharedRoot string) {
		holdCoordinationLock(t, sharedRoot, 0, 400*time.Millisecond)
	})

	code, err := Run(context.Background(), config)
	if err != nil {
		t.Fatalf("contended activation exited %d and reported %v", code, err)
	}
	if code != CapacityDeferredExitCode {
		t.Fatalf("contended activation exited %d, want %d", code, CapacityDeferredExitCode)
	}
}

func TestSupervisionContentionDoesNotStopHealthyWork(t *testing.T) {
	// Once the child is running, every shared-root read is an observation. A
	// peer holding the lock through several sample intervals must never cost
	// the caller its healthy child.
	root := t.TempDir()
	config := contendedReservationConfig(t, root, func(sharedRoot string) {
		holdCoordinationLock(t, sharedRoot, 50*time.Millisecond, 400*time.Millisecond)
	})

	code, err := Run(context.Background(), config)
	if err != nil {
		t.Fatalf("contended supervision reported %v", err)
	}
	if code != 0 {
		t.Fatalf("contended supervision exited %d, want 0", code)
	}
}

func TestReservationStatusWaitsOutABusyCoordinationRoot(t *testing.T) {
	// Reading the shared root is an inspection every repository performs while
	// its peers hold the lock for their own bounded transactions. Ordinary
	// contention must not turn that read into a failure.
	root := t.TempDir()
	marker := coordinationMarker{SchemaVersion: 1, Mode: coordinationModeReservation}
	if err := writeCoordinationMarker(root, marker); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireCoordinationLock(context.Background(), root, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	time.AfterFunc(300*time.Millisecond, func() { _ = releaseCoordinationLock(lock) })

	totals, statusError := ReservationStatus(context.Background(), root)

	if statusError != nil {
		t.Fatalf("status of a briefly contended root reported %v", statusError)
	}
	if totals.Mode != "reservation" {
		t.Fatalf("status reported mode %q, want reservation", totals.Mode)
	}
}

func TestRunDefersWhenTheRequestedPortIsHeldByALiveOwner(t *testing.T) {
	// A service port held by a live peer is retryable lease pressure, which the
	// public contract reports as exit 75. A caller that cannot tell it apart
	// from a configuration or supervision failure has no basis to retry.
	leaseRoot := t.TempDir()
	const port = 45_213
	held, err := AcquirePortLease(leaseRoot, port, "holder", 45_000, 46_000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ReleasePortLease(leaseRoot, held) })

	settings := policy.DefaultPolicy()
	settings.SampleInterval = 10 * time.Millisecond
	settings.AdmissionWindow = 200 * time.Millisecond
	settings.LeaseWait = 100 * time.Millisecond

	code, runError := Run(context.Background(), RunConfig{
		Command:       "/bin/sh",
		Arguments:     []string{"-c", "exit 0"},
		TaskClass:     policy.TaskService,
		Environment:   os.Environ(),
		EvidenceRoot:  t.TempDir(),
		DiskPath:      ".",
		LeasePort:     port,
		LeaseOwner:    "contender",
		LeaseMinimum:  45_000,
		LeaseMaximum:  46_000,
		PortLeaseRoot: leaseRoot,
		Collector:     &controlledRunCollector{},
		Policy:        settings,
		Resolution:    policy.Resolution{RequestedProfile: "balanced", ResolvedProfile: "balanced", Concurrency: 1},
		Sleep:         func(time.Duration) {},
		Now:           time.Now,
		Stderr:        &bytes.Buffer{},
	})

	if runError != nil {
		t.Fatalf("a held service port reported %v", runError)
	}
	if code != CapacityDeferredExitCode {
		t.Fatalf("a held service port exited %d, want the retryable %d", code, CapacityDeferredExitCode)
	}
}
