package runtime //nolint:testpackage // Floor leg adds the missing unit branches, as run_test.go does.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/wahidyankf/hippo/internal/adapters/system"
)

func TestFloorReservationStatusReportsLockFailure(t *testing.T) {
	root := t.TempDir()
	floorWriteMarker(t, root, "reservation")
	if err := os.Mkdir(filepath.Join(root, "coordination.lock"), 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := ReservationStatus(context.Background(), root); err == nil {
		t.Fatal("ReservationStatus with a directory coordination lock = nil, want error")
	}
}

func TestFloorReservationStatusReportsLedgerFailure(t *testing.T) {
	root := t.TempDir()
	floorWriteMarker(t, root, "reservation")
	if err := os.Mkdir(reservationLedgerPath(root), 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := ReservationStatus(context.Background(), root); err == nil {
		t.Fatal("ReservationStatus with a directory ledger = nil, want error")
	}
}

func TestFloorReservationStatusReportsWaitingOverflow(t *testing.T) {
	root := t.TempDir()
	floorWriteMarker(t, root, "reservation")
	tokens := []string{
		floorToken,
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	huge := ReservationVector{CPU: int(^uint(0)>>1) / 2, MemoryBytes: 8 * 1024 * 1024 * 1024}
	ledger := floorEmptyLedger()
	ledger.Capacity = ReservationVector{CPU: int(^uint(0) >> 1), MemoryBytes: int64(^uint64(0) >> 1)}
	ledger.NextSequence = uint64(len(tokens)) + 1
	releases := make([]func(), 0, len(tokens))
	for index, token := range tokens {
		identity, err := openReservationIdentity(system.Production, root, token)
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, func() {
			_ = identity.Close()
			_ = removeReservationIdentity(root, token)
		})
		device, inode, err := reservationIdentityMetadata(identity)
		if err != nil {
			t.Fatal(err)
		}
		ledger.Waiters = append(ledger.Waiters, reservationWaiter{
			Token: token, PID: os.Getpid(), Class: "ephemeral", Profile: "floor",
			Requested: huge, Sequence: uint64(index + 1), MaxOwners: 1,
			IdentityDevice: device, IdentityInode: inode,
		})
	}
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	floorWriteLedger(t, root, ledger)
	if _, err := ReservationStatus(context.Background(), root); err == nil {
		t.Fatal("ReservationStatus with an overflowing waiting total = nil, want error")
	}
}
