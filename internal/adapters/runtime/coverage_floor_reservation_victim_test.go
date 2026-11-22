package runtime //nolint:testpackage // Floor leg adds the missing unit branches, as run_test.go does.

import (
	"context"
	"os"
	"testing"

	"github.com/wahidyankf/hippo/internal/adapters/system"
)

func TestFloorReservationVictimPresentReportsLockFailure(t *testing.T) {
	marker, err := os.CreateTemp(t.TempDir(), "root-*")
	if err != nil {
		t.Fatal(err)
	}
	_ = marker.Close()
	_, port := floorDoublesSystem()
	if _, err := reservationVictimPresent(
		port, context.Background(), marker.Name(), ReservationOwner{Token: floorToken},
	); err == nil {
		t.Fatal("reservationVictimPresent under a file = nil, want error")
	}
}

func TestFloorReservationVictimPresentConfirmsLiveVictim(t *testing.T) {
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
		Sequence:  1, MaxOwners: 1, ProcessGroup: 12345,
		Shedding: true, SheddingCause: ShedCausePressure,
		IdentityDevice: device, IdentityInode: inode,
	}}
	floorWriteLedger(t, root, ledger)
	victim := ReservationOwner{
		Token: floorToken, ProcessGroup: 12345,
		Shedding: true, SheddingCause: ShedCausePressure,
	}
	present, err := reservationVictimPresent(system.Production, context.Background(), root, victim)
	if err != nil || !present {
		t.Fatalf("reservationVictimPresent for a matching live victim = %v, %v, want true, nil", present, err)
	}
}
