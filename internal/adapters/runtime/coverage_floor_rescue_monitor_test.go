package runtime //nolint:testpackage // Floor leg adds the missing unit branches, as run_test.go does.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
)

// floorRescueOwnerLedger seeds a ledger holding one live owner whose identity
// files exist on disk, mirroring the deferred-cleanup matrix. The returned
// handle is owned by the caller until it is handed to the rescue monitor.
func floorRescueOwnerLedger(t *testing.T, root string) {
	t.Helper()
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
}

// floorPollForRemoval waits until every path is gone, which is how a test
// observes a monitor shutdown without touching the handle it stole.
func floorPollForRemoval(t *testing.T, paths ...string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for _, path := range paths {
		for {
			_, statErr := os.Stat(path)
			if os.IsNotExist(statErr) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("monitor did not finish: %s still present", path)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func TestFloorRescueMonitorRetriesContestedIdentity(t *testing.T) {
	root := t.TempDir()
	floorRescueOwnerLedger(t, root)
	lockPath := filepath.Join(root, "reservation-identities", floorToken+".lock")
	identity, err := os.OpenFile(lockPath, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	// The contender is a separate open of the same path, so its exclusive
	// lock conflicts with the monitor's even in this process. The test owns
	// the contender outright; the monitor never sees it.
	contender, err := os.OpenFile(lockPath, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(contender.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			_ = syscall.Flock(int(contender.Fd()), syscall.LOCK_UN)
		}
		_ = contender.Close()
	}()
	session := &Session{Token: floorToken, identityLock: identity}
	err = retainReleaseForReconciliation(
		system.Production, context.Background(), root, session, errCoordinationDeferred,
	)
	// The initial retain probe runs while the contender holds the lock, so
	// the rescue reports the retain failure. Ownership already moved: the
	// session no longer references the stolen handle, and the monitor keeps
	// retrying it instead of the test closing a borrowed handle.
	if err == nil || !strings.Contains(err.Error(), "reservation identity ownership could not be retained") {
		t.Fatalf("retainReleaseForReconciliation on a contested identity = %v, want the retain failure", err)
	}
	if session.identityLock != nil {
		t.Fatal("rescue left the stolen identity handle referenced by the session")
	}
	time.Sleep(200 * time.Millisecond)
	if err := syscall.Flock(int(contender.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	released = true
	floorPollForRemoval(t, lockPath, reservationLedgerPath(root))
}
