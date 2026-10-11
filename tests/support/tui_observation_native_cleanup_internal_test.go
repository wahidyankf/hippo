package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	runtimeadapter "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/tests/contract"
)

func TestNeutralNativeObservationCleanupRetainsFailedRelease(t *testing.T) {
	driver := NewDriver(contract.Adapter{Name: contract.Integration})
	if err := driver.prepareTUIKnownRows(); err != nil {
		t.Fatal(err)
	}
	// The fault targets admitted owner retirement. Queued admission has a separate
	// upstream asynchronous recovery path, so this fixture holds only the owners.
	driver.tuiObservation.totals.Waiters = nil
	driver.tuiObservation.totals.Waiting = coordination.ReservationVector{}
	if err := driver.observeTUI(t.Context()); err != nil {
		driver.Close()
		t.Fatal(err)
	}
	native := driver.tuiObservation.native
	savedSessions := append([]*runtimeadapter.Session(nil), native.sessions...)
	ownedRoot, err := os.OpenRoot(native.root)
	if err != nil {
		driver.Close()
		t.Fatal(err)
	}
	defer func() {
		if err := ownedRoot.Close(); err != nil {
			t.Error(err)
		}
	}()
	ledger, err := ownedRoot.ReadFile("reservations.json")
	if err != nil {
		driver.Close()
		t.Fatal(err)
	}
	defer func() {
		if err := ownedRoot.WriteFile("reservations.json", ledger, 0o600); err != nil {
			t.Errorf("restore owned ledger: %v", err)
			return
		}
		// Preserve test-owned references even under the old implementation's RED,
		// so a failed regression never abandons the real acquired identity locks.
		if len(native.sessions) == 0 {
			native.sessions = savedSessions
		}
		driver.Close()
		if _, err := os.Stat(native.root); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("recovered cleanup retained fixture root: %v", err)
		}
	}()
	if err := ownedRoot.WriteFile("reservations.json", []byte("corrupt owned ledger\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Exercise the callback the driver actually registered for Close and Reset,
	// rather than proving only the private close method already returns an error.
	var caught any
	func() { defer func() { caught = recover() }(); driver.stops[len(driver.stops)-1]() }()
	failure, visible := caught.(error)
	var syntaxFailure *json.SyntaxError
	if !visible || !errors.As(failure, &syntaxFailure) {
		t.Errorf("actual driver cleanup hid the owned release failure: %v", caught)
	}
	if len(native.sessions) != len(savedSessions) {
		t.Errorf("failed cleanup lost owner handles: got %d want %d", len(native.sessions), len(savedSessions))
	}
	for index, session := range savedSessions {
		if index < len(native.sessions) && native.sessions[index] != session {
			t.Errorf("failed cleanup replaced owner handle %d", index)
		}
		identity, err := os.OpenFile(filepath.Join(native.root, "reservation-identities", session.Token+".lock"), os.O_RDWR, 0)
		if err != nil {
			t.Errorf("failed cleanup lost identity %s: %v", session.Token, err)
			continue
		}
		lockError := syscall.Flock(int(identity.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if !errors.Is(lockError, syscall.EWOULDBLOCK) {
			t.Errorf("failed cleanup lost owned identity lock %s: %v", session.Token, lockError)
		}
		if err := identity.Close(); err != nil {
			t.Error(err)
		}
	}
}

func TestNativeTUIWaiterCleanupPreservesIndependentFailure(t *testing.T) {
	failure := errors.New("waiter release failed")
	if got := unexpectedTUIWaiterCleanupError(errors.Join(context.Canceled, failure)); !errors.Is(got, failure) {
		t.Fatalf("cancellation hid independent failure: %v", got)
	}
	if got := unexpectedTUIWaiterCleanupError(fmt.Errorf("waiter cleanup: %w", errors.Join(context.Canceled, failure))); !errors.Is(got, failure) {
		t.Fatalf("wrapped cancellation hid independent failure: %v", got)
	}
	if got := unexpectedTUIWaiterCleanupError(fmt.Errorf("waiter canceled: %w", context.Canceled)); got != nil {
		t.Fatalf("expected cancellation became cleanup failure: %v", got)
	}
}
