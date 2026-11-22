package runtime //nolint:testpackage // Floor leg adds the missing unit branches, as run_test.go does.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
)

func TestFloorReconcileReservationLedgerMatrix(t *testing.T) {
	_, port := floorDoublesSystem()
	t.Run("unverifiable identity", func(t *testing.T) {
		root := t.TempDir()
		locked := filepath.Join(root, "reservation-identities", floorToken+".lock")
		if err := os.MkdirAll(filepath.Dir(locked), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(locked, 0o750); err != nil {
			t.Fatal(err)
		}
		ledger := floorEmptyLedger()
		ledger.Waiters = []reservationWaiter{{
			Token: floorToken, PID: os.Getpid(), Class: "ephemeral", Profile: "floor",
			Requested: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Sequence:  1, MaxOwners: 1,
		}}
		if err := reconcileReservationLedger(port, root, &ledger); err == nil {
			t.Fatal("reconcileReservationLedger with a directory lock = nil, want error")
		}
	})
	t.Run("stale write failure", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(reservationLedgerPath(root), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(reservationLedgerPath(root), "junk"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		ledger := floorEmptyLedger()
		ledger.Owners = []ReservationOwner{{
			Token: floorToken, PID: os.Getpid(), Class: "ephemeral", Profile: "floor",
			Requested: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Allocated: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Sequence:  1, MaxOwners: 1,
		}}
		if err := reconcileReservationLedger(system.Production, root, &ledger); err == nil {
			t.Fatal("reconcileReservationLedger with an unwritable ledger = nil, want error")
		}
	})
	t.Run("stale removal failure", func(t *testing.T) {
		root := t.TempDir()
		floorWriteLedger(t, root, floorEmptyLedger())
		if err := os.WriteFile(filepath.Join(root, "owner-metadata"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		ledger := floorEmptyLedger()
		ledger.Owners = []ReservationOwner{{
			Token: floorToken, PID: os.Getpid(), Class: "ephemeral", Profile: "floor",
			Requested: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Allocated: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Sequence:  1, MaxOwners: 1,
		}}
		if err := reconcileReservationLedger(system.Production, root, &ledger); err == nil {
			t.Fatal("reconcileReservationLedger with an unremovable stale identity = nil, want error")
		}
	})
}

func TestFloorEnsureReservationCoordinationMatrix(t *testing.T) {
	_, port := floorDoublesSystem()
	t.Run("unreadable marker", func(t *testing.T) {
		marker, err := os.CreateTemp(t.TempDir(), "root-*")
		if err != nil {
			t.Fatal(err)
		}
		_ = marker.Close()
		if err := ensureReservationCoordination(port, marker.Name()); err == nil {
			t.Fatal("ensureReservationCoordination under a file = nil, want error")
		}
	})
	t.Run("looping heavy lock", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Symlink("heavy.lock", filepath.Join(root, "heavy.lock")); err != nil {
			t.Fatal(err)
		}
		if err := ensureReservationCoordination(port, root); err == nil {
			t.Fatal("ensureReservationCoordination with a looping heavy lock = nil, want error")
		}
	})
}

func TestFloorInheritedReservationWithoutCandidate(t *testing.T) {
	_, port := floorDoublesSystem()
	if session, err := inheritedReservation(port, t.TempDir(), "", floorEmptyLedger()); session != nil || err != nil {
		t.Fatalf("inheritedReservation without a candidate = %v, %v, want nil, nil", session, err)
	}
}

func TestFloorInheritedReservationReportsUnverifiableIdentity(t *testing.T) {
	root := t.TempDir()
	locked := filepath.Join(root, "reservation-identities", floorToken+".lock")
	if err := os.MkdirAll(filepath.Dir(locked), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(locked, 0o750); err != nil {
		t.Fatal(err)
	}
	_, port := floorDoublesSystem()
	ledger := floorEmptyLedger()
	ledger.Owners = []ReservationOwner{{
		Token: floorToken, PID: os.Getpid(), Class: "ephemeral", Profile: "floor",
		Requested: ReservationVector{CPU: 1, MemoryBytes: 268435456},
		Allocated: ReservationVector{CPU: 1, MemoryBytes: 268435456},
		Sequence:  1, MaxOwners: 1,
	}}
	if _, err := inheritedReservation(port, root, floorToken, ledger); err == nil {
		t.Fatal("inheritedReservation with a directory lock = nil, want error")
	}
}

func TestFloorNeverStartedAtCoordinationLockMatrix(t *testing.T) {
	now := time.Now()
	t.Run("foreign error passes through", func(t *testing.T) {
		boom := errors.New("floor: foreign lock error")
		if err := neverStartedAtCoordinationLock(
			system.Production, t.TempDir(), floorToken, ReservationMetadata{}, "ephemeral", now, boom,
		); !errors.Is(err, boom) {
			t.Fatalf("neverStartedAtCoordinationLock with a foreign error = %v, want it back", err)
		}
	})
	t.Run("cancelled admission with a refused receipt", func(t *testing.T) {
		marker, err := os.CreateTemp(t.TempDir(), "root-*")
		if err != nil {
			t.Fatal(err)
		}
		_ = marker.Close()
		metadata := ReservationMetadata{Source: "hippo"}
		err = neverStartedAtCoordinationLock(
			system.Production, marker.Name(), floorToken, metadata, "ephemeral", now, context.Canceled,
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("neverStartedAtCoordinationLock on cancellation = %v, want cancellation", err)
		}
	})
}

func TestFloorRetainReservationWaiterUntilCleanupNilIsNoOp(t *testing.T) {
	_, port := floorDoublesSystem()
	retainReservationWaiterUntilCleanup(port, t.TempDir(), floorToken, nil)
}

func TestFloorRetainReservationWaiterUntilCleanupRetries(t *testing.T) {
	root := t.TempDir()
	floorWriteLedger(t, root, floorEmptyLedger())
	identity, err := openReservationIdentity(system.Production, root, floorToken)
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.OpenFile(filepath.Join(root, "reservation-identities", floorToken+".lock"), os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	retainReservationWaiterUntilCleanup(system.Production, root, floorToken, second)
	timer := time.AfterFunc(100*time.Millisecond, func() { _ = identity.Close() })
	defer timer.Stop()
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, statErr := os.Stat(filepath.Join(root, "reservation-identities", floorToken+".lock"))
		if os.IsNotExist(statErr) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("retained waiter did not finish after the identity was released")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestFloorRemoveReservationWaiterMatrix(t *testing.T) {
	t.Run("lock failure", func(t *testing.T) {
		marker, err := os.CreateTemp(t.TempDir(), "root-*")
		if err != nil {
			t.Fatal(err)
		}
		_ = marker.Close()
		if err := removeReservationWaiterWithAdvisories(
			system.Production, marker.Name(), floorToken, time.Second, nil,
		); err == nil {
			t.Fatal("removeReservationWaiterWithAdvisories under a file = nil, want error")
		}
	})
	t.Run("ledger failure", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(reservationLedgerPath(root), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := removeReservationWaiterWithAdvisories(
			system.Production, root, floorToken, time.Second, nil,
		); err == nil {
			t.Fatal("removeReservationWaiterWithAdvisories with a directory ledger = nil, want error")
		}
	})
}

func TestFloorActivateReservationMatrix(t *testing.T) {
	t.Run("nil session is nil", func(t *testing.T) {
		if err := ActivateReservation(t.TempDir(), nil, 1); err != nil {
			t.Fatalf("ActivateReservation(nil) = %v, want nil", err)
		}
	})
	t.Run("lock failure", func(t *testing.T) {
		marker, err := os.CreateTemp(t.TempDir(), "root-*")
		if err != nil {
			t.Fatal(err)
		}
		_ = marker.Close()
		if err := ActivateReservation(marker.Name(), &Session{Token: floorToken}, 1); err == nil {
			t.Fatal("ActivateReservation under a file = nil, want error")
		}
	})
	t.Run("ledger failure", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(reservationLedgerPath(root), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := ActivateReservation(root, &Session{Token: floorToken}, 1); err == nil {
			t.Fatal("ActivateReservation with a directory ledger = nil, want error")
		}
	})
	t.Run("disappeared owner", func(t *testing.T) {
		root := t.TempDir()
		floorWriteLedger(t, root, floorEmptyLedger())
		if err := ActivateReservation(root, &Session{Token: floorToken}, 1); err == nil {
			t.Fatal("ActivateReservation with a missing owner = nil, want error")
		}
	})
}

func TestFloorReleaseReservationIdentityNilIsNil(t *testing.T) {
	if err := releaseReservationIdentity(t.TempDir(), nil); err != nil {
		t.Fatalf("releaseReservationIdentity(nil) = %v, want nil", err)
	}
	if err := abandonReservationIdentity(nil); err != nil {
		t.Fatalf("abandonReservationIdentity(nil) = %v, want nil", err)
	}
}

func TestFloorRetainReleaseForReconciliationMatrix(t *testing.T) {
	t.Run("unretainable identity", func(t *testing.T) {
		closed, err := os.CreateTemp(t.TempDir(), "identity-*.lock")
		if err != nil {
			t.Fatal(err)
		}
		name := closed.Name()
		_ = closed.Close()
		closed, err = os.OpenFile(name, os.O_RDONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		_ = closed.Close()
		session := &Session{Token: floorToken, identityLock: closed}
		if err := retainReleaseForReconciliation(
			system.Production, context.Background(), t.TempDir(), session, errCoordinationDeferred,
		); err == nil {
			t.Fatal("retainReleaseForReconciliation with a closed identity = nil, want error")
		}
	})
	t.Run("deferred cleanup wraps the deferral", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "reservation-identities"), 0o750); err != nil {
			t.Fatal(err)
		}
		lockPath := filepath.Join(root, "reservation-identities", floorToken+".lock")
		anchorPath := filepath.Join(root, "reservation-identities", floorToken+".anchor")
		seed, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		_ = seed.Close()
		if err := os.Link(lockPath, anchorPath); err != nil {
			t.Fatal(err)
		}
		var info syscall.Stat_t
		if err := syscall.Stat(lockPath, &info); err != nil {
			t.Fatal(err)
		}
		ledger := floorEmptyLedger()
		ledger.NextSequence = 2
		ledger.Owners = []ReservationOwner{{
			Token: floorToken, PID: os.Getpid(), Class: "ephemeral", Profile: "floor",
			Requested: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Allocated: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Sequence:  1, MaxOwners: 1,
			IdentityDevice: info.Dev, IdentityInode: info.Ino,
		}}
		floorWriteLedger(t, root, ledger)
		identity, err := os.OpenFile(lockPath, os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		session := &Session{Token: floorToken, identityLock: identity}
		err = retainReleaseForReconciliation(
			system.Production, context.Background(), root, session, errCoordinationDeferred,
		)
		if !errors.Is(err, ErrCoordinationCleanupDeferred) {
			t.Fatalf("retainReleaseForReconciliation on a deferral = %v, want deferred cleanup", err)
		}
		deadline := time.Now().Add(10 * time.Second)
		for {
			_, statErr := os.Stat(filepath.Join(root, "reservation-identities", floorToken+".lock"))
			if os.IsNotExist(statErr) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("retained owner did not finish releasing")
			}
			time.Sleep(20 * time.Millisecond)
		}
	})
}

func TestFloorReleaseReservationContextNilIsNil(t *testing.T) {
	if err := releaseReservationContext(system.Production, context.Background(), t.TempDir(), nil, true); err != nil {
		t.Fatalf("releaseReservationContext(nil) = %v, want nil", err)
	}
}

func TestFloorReleaseReservationOwnershipLockedMatrix(t *testing.T) {
	_, port := floorDoublesSystem()
	t.Run("unreadable ledger", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(reservationLedgerPath(root), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := releaseReservationOwnershipLocked(port, root, &Session{Token: floorToken}); err == nil {
			t.Fatal("releaseReservationOwnershipLocked with a directory ledger = nil, want error")
		}
	})
	t.Run("unwritable ledger", func(t *testing.T) {
		root := t.TempDir()
		owner := floorReservationOwner(t, root, floorToken, 1)
		ledger := floorEmptyLedger()
		ledger.NextSequence = 2
		ledger.Owners = []ReservationOwner{owner}
		floorWriteLedger(t, root, ledger)
		if err := os.Chmod(root, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(root, 0o750) })
		if err := releaseReservationOwnershipLocked(port, root, &Session{Token: floorToken}); err == nil {
			t.Fatal("releaseReservationOwnershipLocked under a read-only root = nil, want error")
		}
	})
	t.Run("unreadable marker is joined", func(t *testing.T) {
		root := t.TempDir()
		owner := floorReservationOwner(t, root, floorToken, 1)
		ledger := floorEmptyLedger()
		ledger.NextSequence = 2
		ledger.Owners = []ReservationOwner{owner}
		floorWriteLedger(t, root, ledger)
		identity, err := os.OpenFile(
			filepath.Join(root, "reservation-identities", floorToken+".lock"), os.O_RDWR, 0o600,
		)
		if err != nil {
			t.Fatal(err)
		}
		writeFloorMarker(t, root, "[[[")
		session := &Session{Token: floorToken, identityLock: identity}
		if err := releaseReservationOwnershipLocked(system.Production, root, session); err == nil {
			t.Fatal("releaseReservationOwnershipLocked with a corrupt marker = nil, want error")
		}
	})
}
