package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/policy"
)

func integrationReservationPlan(cpu int, memory int64) guard.ReservationPlan {
	vector := guard.ReservationVector{CPU: cpu, MemoryBytes: memory}

	return guard.ReservationPlan{
		Capacity:  guard.ReservationVector{CPU: 4, MemoryBytes: policy.GiB},
		Requested: vector, Allocated: vector,
	}
}

func TestCompiledReservationRejectsHIPPOConcurrencyMappingBeforeChild(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "hippo")
	build := exec.Command("go", "build", "-o", binary, "./cmd/hippo")
	build.Dir = integrationModuleRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build HIPPO: %s: %v", output, err)
	}
	configPath := filepath.Join(root, "hippo.local.json")
	if err := os.WriteFile(configPath, []byte(`{"schemaVersion":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"HIPPO_ROOT", "HIPPO_RESERVED_MEMORY_BYTES", "HIPPO_CONFIG", "HIPPO_DEFAULT_CONFIG"} {
		marker := filepath.Join(root, name+"-started")
		command := exec.Command(
			binary, "run", "--config", configPath, "--disk-path", ".",
			"--reserve-cpu", "1", "--reserve-memory-mib", "256", "--concurrency-env", name,
			"--", "/bin/sh", "-c", `printf started > "$CHILD_MARKER"`,
		)
		command.Env = append(os.Environ(), "HIPPO_ROOT="+filepath.Join(root, "shared"), "CHILD_MARKER="+marker)
		var output bytes.Buffer
		command.Stdout, command.Stderr = &output, &output
		err := command.Run()
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) || exitError.ExitCode() != policy.ReplanRequiredExitCode {
			t.Fatalf("mapping %s exit=%v output=%q", name, err, output.String())
		}
		if _, statError := os.Stat(marker); !errors.Is(statError, os.ErrNotExist) {
			t.Fatalf("mapping %s started child", name)
		}
	}
}

func TestReservationRecoversOwnerWhosePIDWasReused(t *testing.T) {
	if os.Getenv("HIPPO_STALE_RESERVATION_HELPER") == "1" {
		root := os.Getenv("HIPPO_STALE_RESERVATION_ROOT")
		session, err := guard.AcquireReservation(
			context.Background(), root, "", policy.TaskEphemeral, "balanced", "",
			integrationReservationPlan(4, policy.GiB), 20, 0,
		)
		if err != nil || session == nil {
			t.Fatalf("helper reservation failed: session=%+v error=%v", session, err)
		}

		return
	}

	root := t.TempDir()
	command := exec.Command(os.Args[0], "-test.run=^TestReservationRecoversOwnerWhosePIDWasReused$") //nolint:gosec // The current signed test binary is the intentional subprocess boundary.
	command.Env = append(os.Environ(), "HIPPO_STALE_RESERVATION_HELPER=1", "HIPPO_STALE_RESERVATION_ROOT="+root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("stale owner helper failed: %s: %v", output, err)
	}

	ledgerPath := filepath.Join(root, "reservations.json")
	data, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	var ledger map[string]any
	if err = json.Unmarshal(data, &ledger); err != nil {
		t.Fatal(err)
	}
	owners, ok := ledger["owners"].([]any)
	if !ok || len(owners) != 1 {
		t.Fatalf("unexpected stale ledger %s", data)
	}
	ownerRecord, ok := owners[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected stale owner %T", owners[0])
	}
	ownerRecord["pid"] = os.Getpid()
	data, err = json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(ledgerPath, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	session, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskTransactional, "minimal", "",
		integrationReservationPlan(4, policy.GiB), 20, 0,
	)
	if err != nil || session == nil {
		t.Fatalf("stale PID-reuse record retained capacity: session=%+v error=%v", session, err)
	}
	if err = guard.ReleaseReservation(root, session); err != nil {
		t.Fatal(err)
	}
}

func TestOnlyOneConcurrentOwnerWinsTheLastVectorSlot(t *testing.T) {
	root := t.TempDir()
	owner, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskService, "balanced", "",
		integrationReservationPlan(3, 768*policy.MiB), 20, 0,
	)
	if err != nil || owner == nil {
		t.Fatalf("initial reservation: %+v %v", owner, err)
	}

	const contenders = 12
	start := make(chan struct{})
	results := make(chan *guard.Session, contenders)
	errorsFound := make(chan error, contenders)
	var group sync.WaitGroup
	for range contenders {
		group.Go(func() {
			<-start
			session, acquireError := guard.AcquireReservation(
				context.Background(), root, "", policy.TaskEphemeral, "balanced", "",
				integrationReservationPlan(1, 256*policy.MiB), 20, 100*time.Millisecond,
			)
			results <- session
			errorsFound <- acquireError
		})
	}
	close(start)
	group.Wait()
	close(results)
	close(errorsFound)

	for acquireError := range errorsFound {
		if acquireError != nil && !guard.IsCoordinationDeferred(acquireError) && !errors.Is(acquireError, guard.ErrReservationDeferred) {
			t.Fatal(acquireError)
		}
	}
	winners := []*guard.Session{}
	for session := range results {
		if session != nil {
			winners = append(winners, session)
		}
	}
	if len(winners) != 1 {
		t.Fatalf("atomic last slot admitted %d contenders", len(winners))
	}
	totals, err := guard.ReservationStatus(context.Background(), root)
	if err != nil || totals.Allocated.CPU != 4 || totals.Allocated.MemoryBytes != policy.GiB {
		t.Fatalf("non-atomic totals %+v error=%v", totals, err)
	}
	if err = guard.ReleaseReservation(root, winners[0]); err != nil {
		t.Fatal(err)
	}
	if err = guard.ReleaseReservation(root, owner); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentVictimSelectionAndReleaseKeepsLedgerConsistent(t *testing.T) {
	for iteration := range 20 {
		root := t.TempDir()
		service, err := guard.AcquireReservation(
			context.Background(), root, "", policy.TaskService, "balanced", "",
			integrationReservationPlan(1, 256*policy.MiB), 20, 0,
		)
		if err != nil {
			t.Fatal(err)
		}
		ephemeral, err := guard.AcquireReservation(
			context.Background(), root, "", policy.TaskEphemeral, "balanced", "",
			integrationReservationPlan(1, 256*policy.MiB), 20, 0,
		)
		if err != nil {
			t.Fatal(err)
		}
		if err = guard.ActivateReservation(root, service, 30_000+iteration*2); err != nil {
			t.Fatal(err)
		}
		if err = guard.ActivateReservation(root, ephemeral, 30_001+iteration*2); err != nil {
			t.Fatal(err)
		}

		errorsFound := make(chan error, 2)
		var group sync.WaitGroup
		group.Go(func() {
			_, _, selectionError := guard.SelectPressureVictim(root, guard.CapacityDeferredExitCode)
			errorsFound <- selectionError
		})
		group.Go(func() { errorsFound <- guard.ReleaseReservation(root, ephemeral) })
		group.Wait()
		close(errorsFound)
		for operationError := range errorsFound {
			if operationError != nil {
				t.Fatal(operationError)
			}
		}
		totals, statusError := guard.ReservationStatus(context.Background(), root)
		if statusError != nil || totals.ActiveOwners != 1 || totals.Allocated.CPU != 1 || totals.Allocated.MemoryBytes != 256*policy.MiB {
			t.Fatalf("iteration %d left inconsistent totals %+v error=%v", iteration, totals, statusError)
		}
		if err = guard.ReleaseReservation(root, service); err != nil {
			t.Fatal(err)
		}
	}
}

func TestConcurrentVictimSelectorsCannotCascadeBeforeOwnedRelease(t *testing.T) { //nolint:gocognit // The race regression keeps both selectors, owner observation, and post-release election in one lifecycle.
	for iteration := range 25 {
		root := t.TempDir()
		service, err := guard.AcquireReservation(
			context.Background(), root, "", policy.TaskService, "balanced", "",
			integrationReservationPlan(1, 256*policy.MiB), 20, 0,
		)
		if err != nil {
			t.Fatal(err)
		}
		ephemeral, err := guard.AcquireReservation(
			context.Background(), root, "", policy.TaskEphemeral, "balanced", "",
			integrationReservationPlan(1, 256*policy.MiB), 20, 0,
		)
		if err != nil {
			t.Fatal(err)
		}
		if err = guard.ActivateReservation(root, service, 40_000+iteration*2); err != nil {
			t.Fatal(err)
		}
		if err = guard.ActivateReservation(root, ephemeral, 40_001+iteration*2); err != nil {
			t.Fatal(err)
		}

		const selectors = 12
		start := make(chan struct{})
		type selectionResult struct {
			victim   guard.ReservationOwner
			selected bool
		}
		results := make(chan selectionResult, selectors)
		errorsFound := make(chan error, selectors)
		var group sync.WaitGroup
		for range selectors {
			group.Go(func() {
				<-start
				victim, selected, selectionError := guard.SelectPressureVictim(root, guard.CapacityDeferredExitCode)
				results <- selectionResult{victim: victim, selected: selected}
				errorsFound <- selectionError
			})
		}
		close(start)
		group.Wait()
		close(results)
		close(errorsFound)
		for selectionError := range errorsFound {
			if selectionError != nil {
				t.Fatalf("iteration %d selector: %v", iteration, selectionError)
			}
		}
		victims := make([]guard.ReservationOwner, 0, 1)
		for result := range results {
			if result.selected {
				victims = append(victims, result.victim)
			}
		}
		if len(victims) != 1 || victims[0].Token != ephemeral.Token {
			t.Fatalf("iteration %d selected victims %+v", iteration, victims)
		}
		if victim, selected, selectionError := guard.SelectPressureVictim(root, guard.CapacityDeferredExitCode); selectionError != nil || selected {
			t.Fatalf("iteration %d cascaded before release: victim=%+v selected=%v error=%v", iteration, victim, selected, selectionError)
		}
		if err = guard.ReleaseReservation(root, ephemeral); err != nil {
			t.Fatal(err)
		}
		victim, selected, selectionError := guard.SelectPressureVictim(root, guard.CapacityDeferredExitCode)
		if selectionError != nil || !selected || victim.Token != service.Token {
			t.Fatalf("iteration %d did not select service after release: victim=%+v selected=%v error=%v", iteration, victim, selected, selectionError)
		}
		if err = guard.ReleaseReservation(root, service); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRemoteObservationTreatsReleasedOwnerAsCompleteWithoutSignaling(t *testing.T) {
	root := t.TempDir()
	session, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, "balanced", "",
		integrationReservationPlan(1, 256*policy.MiB), 20, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("sh", "-c", `while :; do sleep 1; done`)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()
	t.Cleanup(func() {
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	})
	if err = guard.ActivateReservation(root, session, command.Process.Pid); err != nil {
		t.Fatal(err)
	}
	victim, selected, err := guard.SelectPressureVictim(root, guard.CapacityDeferredExitCode)
	if err != nil || !selected {
		t.Fatalf("select victim: selected=%v error=%v", selected, err)
	}
	if err = guard.ReleaseReservation(root, session); err != nil {
		t.Fatal(err)
	}
	if err = guard.WaitPressureVictimRelease(root, victim, 20*time.Millisecond); err != nil {
		t.Fatalf("released ownership did not complete remote observation: %v", err)
	}
	select {
	case waitError := <-exited:
		t.Fatalf("unowned process group was signaled: %v", waitError)
	case <-time.After(40 * time.Millisecond):
	}
}

func TestRemoteSelectorNeverSignalsUnresponsiveOwner(t *testing.T) {
	root := t.TempDir()
	session, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, "balanced", "",
		integrationReservationPlan(1, 256*policy.MiB), 20, 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("sh", "-c", `while :; do sleep 1; done`)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = guard.ReleaseReservation(root, session)
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		_ = command.Wait()
	})
	if err = guard.ActivateReservation(root, session, command.Process.Pid); err != nil {
		t.Fatal(err)
	}
	victim, selected, err := guard.SelectPressureVictim(root, guard.CapacityDeferredExitCode)
	if err != nil || !selected {
		t.Fatalf("select victim: selected=%v error=%v", selected, err)
	}
	if err = guard.WaitPressureVictimRelease(root, victim, 20*time.Millisecond); err == nil {
		t.Fatal("unresponsive remote owner unexpectedly completed bounded observation")
	}
	if err = syscall.Kill(command.Process.Pid, 0); err != nil {
		t.Fatalf("remote selector signaled another owner's child: %v", err)
	}
	if next, nextSelected, nextError := guard.SelectPressureVictim(root, guard.StorageBlockedExitCode); nextError != nil || nextSelected {
		t.Fatalf("unresponsive selected owner did not remain barrier: victim=%+v selected=%v error=%v", next, nextSelected, nextError)
	}
}
