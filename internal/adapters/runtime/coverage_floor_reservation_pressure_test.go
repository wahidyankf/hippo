package runtime //nolint:testpackage // Floor leg adds the missing unit branches, as run_test.go does.

import (
	"context"
	"os"
	"testing"

	"github.com/wahidyankf/hippo/internal/adapters/system"
)

func TestFloorSelectPressureVictimRefusesInvalidCause(t *testing.T) {
	if _, _, err := SelectPressureVictim(t.TempDir(), ShedCauseNone); err == nil {
		t.Fatal("SelectPressureVictim without a cause = nil, want error")
	}
}

func TestFloorSelectPressureVictimReportsLedgerWriteFailure(t *testing.T) {
	root := t.TempDir()
	identity, err := openReservationIdentity(system.Production, root, floorToken)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = identity.Close()
		_ = removeReservationIdentity(root, floorToken)
	}()
	device, inode, err := reservationIdentityMetadata(identity)
	if err != nil {
		t.Fatal(err)
	}
	ledger := floorEmptyLedger()
	ledger.NextSequence = 2
	ledger.Owners = []ReservationOwner{{
		Token: floorToken, PID: os.Getpid(), Class: "ephemeral", Profile: "floor",
		Requested: ReservationVector{CPU: 1, MemoryBytes: 268435456},
		Allocated: ReservationVector{CPU: 1, MemoryBytes: 268435456},
		Sequence:  1, MaxOwners: 1, ProcessGroup: os.Getpid(),
		IdentityDevice: device, IdentityInode: inode,
	}}
	floorWriteLedger(t, root, ledger)
	held, err := acquireCoordinationLock(system.Production, context.Background(), root, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := releaseCoordinationLock(system.Production, held); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o750) })
	if _, _, err := SelectPressureVictim(root, ShedCausePressure); err == nil {
		t.Fatal("SelectPressureVictim under a read-only root = nil, want error")
	}
}
