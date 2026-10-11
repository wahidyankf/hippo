package integration_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/evidence"
	runtimeadapter "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/policy"
)

const (
	nativePublicationAdvisory = "HIPPO advisory: display label publication failed; monitoring may show an unavailable label.\n"
	nativeRemovalAdvisory     = "HIPPO advisory: display label cleanup failed; later reconciliation may be needed.\n"
)

func annotationOwnershipConfig(root string, stderr *bytes.Buffer) application.RunConfig {
	config := displayRegistrationConfig(root, true)
	config.ReservationPlan.Capacity = coordination.ReservationVector{CPU: 4, MemoryBytes: 2 * policy.GiB}
	config.Stderr = stderr
	return config
}

func TestAnnotationNativeForeignOwnerSurvivesOwnedRemovalFailure(t *testing.T) {
	root := t.TempDir()
	var diagnostics bytes.Buffer
	config := annotationOwnershipConfig(root, &diagnostics)
	engine := runtimeadapter.NewEngine()
	owned := displayRegister(t, engine, config)
	foreignEngine := runtimeadapter.NewEngine()
	foreign := displayRegister(t, foreignEngine, config)
	foreignPath := filepath.Join(root, "run-display", foreign.Lease.Token+".json")
	before, err := os.ReadFile(foreignPath)
	if err != nil {
		t.Fatal(err)
	}
	ownedPath := filepath.Join(root, "run-display", owned.Lease.Token+".json")
	if err = os.Remove(ownedPath); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(ownedPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = engine.Release(config, owned.Lease); err != nil {
		t.Fatalf("owned annotation failure changed release: %v", err)
	}
	totals, err := foreignEngine.Snapshot(t.Context(), root, "reservation")
	if err != nil || totals.ActiveOwners != 1 || totals.WaitingOwners != 0 || totals.Allocated != foreign.Lease.Allocation {
		t.Fatalf("foreign accounting changed: %+v %v", totals, err)
	}
	after, err := os.ReadFile(foreignPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("foreign annotation changed on owned removal fault: %v", err)
	}
	if diagnostics.String() != nativeRemovalAdvisory {
		t.Fatalf("retired owner removal failure was not safely reported once: %q", diagnostics.String())
	}
	if strings.Contains(diagnostics.String(), root) {
		t.Fatal("removal report leaked private root")
	}
	if err = os.Remove(ownedPath); err != nil {
		t.Fatal(err)
	}
}

func TestAnnotationNativeInheritedOwnerDoesNotMutateOrReport(t *testing.T) {
	for _, reservation := range []bool{false, true} {
		root := t.TempDir()
		var diagnostics bytes.Buffer
		config := displayRegistrationConfig(root, reservation)
		config.Stderr = &diagnostics
		ownerEngine := runtimeadapter.NewEngine()
		owner := displayRegister(t, ownerEngine, config)
		path := filepath.Join(root, "run-display", owner.Lease.Token+".json")
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		config.Environment = []string{"HIPPO_SESSION=" + owner.Lease.Token}
		nestedEngine := runtimeadapter.NewEngine()
		admission, err := nestedEngine.BeginAdmission(t.Context(), config)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(admission.Close)
		lease := admission.Admitted
		if lease == nil {
			result, attemptError := admission.Attempt(t.Context(), time.Second, time.Now())
			if attemptError != nil {
				t.Fatal(attemptError)
			}
			lease = result.Lease
		}
		if lease == nil || !lease.Inherited || lease.Token != owner.Lease.Token {
			t.Fatalf("native test did not inherit original owner: %+v", lease)
		}
		if err = nestedEngine.Release(config, lease); err != nil {
			t.Fatal(err)
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) || diagnostics.Len() != 0 {
			t.Fatalf("inherited mutation/report changed owner: %v stderr=%q", err, diagnostics.String())
		}
	}
}

func TestAnnotationNativeQueuedCloseReportsActualOwnedFaultAndPreservesOtherOwner(t *testing.T) {
	for _, operation := range []string{"write", "removal"} {
		t.Run(operation, func(t *testing.T) {
			checkAnnotationQueuedClose(t, operation)
		})
	}
}

type annotationCallerLifetimeWriter struct {
	mutex            sync.Mutex
	expired          bool
	callsAfterReturn int
	output           bytes.Buffer
}

func (writer *annotationCallerLifetimeWriter) Write(data []byte) (int, error) {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()
	if writer.expired {
		writer.callsAfterReturn++
		return 0, errors.New("caller stream expired")
	}
	return writer.output.Write(data)
}

func (writer *annotationCallerLifetimeWriter) expire() {
	writer.mutex.Lock()
	writer.expired = true
	writer.mutex.Unlock()
}

func (writer *annotationCallerLifetimeWriter) read() (string, int) {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()
	return writer.output.String(), writer.callsAfterReturn
}

func TestAnnotationNativeRetainedOwnerReconcilesWithoutExpiredCallerWrites(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "run-display"), []byte("private annotation blocker"), 0o600); err != nil {
		t.Fatal(err)
	}
	writer := &annotationCallerLifetimeWriter{}
	config := displayRegistrationConfig(root, true)
	config.Stderr = writer
	engine := runtimeadapter.NewEngine()
	admission, err := engine.BeginAdmission(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admission.Close)
	result, err := admission.Attempt(t.Context(), time.Second, time.Now())
	if err != nil || result.Lease == nil {
		t.Fatalf("retained-owner setup never admitted: %+v %v", result, err)
	}
	t.Cleanup(func() {
		if releaseError := engine.Release(config, result.Lease); releaseError != nil {
			t.Error(releaseError)
		}
	})
	before, calls := writer.read()
	if before != nativePublicationAdvisory || calls != 0 {
		t.Fatalf("foreground publication error missing or misframed: %q calls=%d", before, calls)
	}
	lock, err := os.OpenFile(filepath.Join(root, "coordination.lock"), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	if err = engine.Release(config, result.Lease); !errors.Is(err, coordination.ErrCoordinationCleanupDeferred) {
		t.Fatalf("contended owned release did not retain reconcilable ownership: %v", err)
	}
	writer.expire()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	waitForAnnotationRetainedRetirement(ctx, t, root, lock)
	after, expiredCalls := writer.read()
	if after != before || expiredCalls != 0 {
		t.Fatalf("background cleanup wrote to completed invocation: before=%q after=%q expired calls=%d", before, after, expiredCalls)
	}
	annotations, err := (evidence.DisplayRepository{}).ReadDisplay(context.Background(), root, []string{result.Lease.Token})
	if err == nil || annotations[result.Lease.Token].Availability != application.DisplayUnreadable {
		t.Fatal("retained background cleanup fault was not the actual blocked native directory")
	}
	if totals, readError := runtimeadapter.NewEngine().Snapshot(t.Context(), root, "reservation"); readError != nil || totals.ActiveOwners != 0 || totals.WaitingOwners != 0 {
		t.Fatalf("retained ownership did not retire normally: %+v %v", totals, readError)
	}
}

func checkAnnotationQueuedClose(t *testing.T, operation string) {
	t.Helper()
	root := t.TempDir()
	var diagnostics bytes.Buffer
	config := displayRegistrationConfig(root, true)
	config.Stderr = &diagnostics
	ownerEngine := runtimeadapter.NewEngine()
	owner := displayRegister(t, ownerEngine, config)
	ownerPath := filepath.Join(root, "run-display", owner.Lease.Token+".json")
	before, err := os.ReadFile(ownerPath)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "run-display")
	retained := filepath.Join(root, "retained-display")
	if operation == "write" {
		blockQueuedPublication(t, directory, retained)
	}
	waiterEngine := runtimeadapter.NewEngine()
	admission, err := waiterEngine.BeginAdmission(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admission.Close)
	result, err := admission.Attempt(t.Context(), time.Second, time.Now())
	if err != nil || !result.Waiting || result.Lease != nil || result.Status.RunID == "" || result.Status.Position != 1 {
		t.Fatalf("native waiter was not registered once at FIFO head: %+v %v", result, err)
	}
	if operation == "write" {
		restoreQueuedPublication(t, directory, retained)
	} else {
		blockQueuedRemoval(t, directory, result.Status.RunID)
	}
	admission.Close()
	admission.Close()
	totals, err := ownerEngine.Snapshot(t.Context(), root, "reservation")
	if err != nil || totals.WaitingOwners != 0 || totals.ActiveOwners != 1 || totals.Allocated != owner.Lease.Allocation {
		t.Fatalf("advisory queue cancellation changed accounting: %+v %v", totals, err)
	}
	after, err := os.ReadFile(ownerPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("waiter cancellation changed other owner's annotation: %v", err)
	}
	want := nativePublicationAdvisory
	if operation == "removal" {
		want = nativeRemovalAdvisory
	}
	if diagnostics.String() != want {
		t.Fatalf("queued mutation report=%q want once-only %q", diagnostics.String(), want)
	}
}

func blockQueuedPublication(t *testing.T, directory, retained string) {
	t.Helper()
	if err := os.Rename(directory, retained); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(directory, []byte("private waiter publication blocker"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func restoreQueuedPublication(t *testing.T, directory, retained string) {
	t.Helper()
	if err := os.Remove(directory); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(retained, directory); err != nil {
		t.Fatal(err)
	}
}

func blockQueuedRemoval(t *testing.T, directory, runID string) {
	t.Helper()
	path := filepath.Join(directory, runID+".json")
	if _, err := os.ReadFile(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if removeError := os.Remove(path); removeError != nil {
			t.Error(removeError)
		}
	})
}

func waitForAnnotationRetainedRetirement(ctx context.Context, t *testing.T, root string, lock *os.File) {
	t.Helper()
	for {
		_, statError := os.Stat(filepath.Join(root, "reservations.json"))
		if errors.Is(statError, os.ErrNotExist) {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("retained owner never reconciled after lock release")
		case <-time.After(time.Millisecond):
		}
	}
	// The final coordination lock round trip waits for the cleanup transaction,
	// including its native annotation removal attempt, to return.
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-time.After(20 * time.Millisecond):
	}
}
