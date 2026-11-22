package runtime //nolint:testpackage // Floor leg adds the missing unit branches, as run_test.go does.

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/policy"
)

// floorStatusRoot seeds a reservation-mode root with live held owners.
func floorStatusRoot(t *testing.T, tokens ...string) (string, []func()) {
	t.Helper()
	root := t.TempDir()
	floorWriteMarker(t, root, "reservation")
	ledger := floorEmptyLedger()
	ledger.NextSequence = uint64(len(tokens)) + 2
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
		ledger.Owners = append(ledger.Owners, ReservationOwner{
			Token: token, PID: os.Getpid(), Class: "ephemeral", Profile: "floor",
			Requested: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Allocated: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Sequence:  uint64(index + 1), MaxOwners: 1,
			IdentityDevice: device, IdentityInode: inode,
		})
	}
	floorWriteLedger(t, root, ledger)

	return root, releases
}

func TestFloorExclusiveStatusWithoutMarker(t *testing.T) {
	totals, err := ExclusiveStatus(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("ExclusiveStatus without a marker: %v", err)
	}
	if totals.Mode != coordinationModeExclusive {
		t.Fatalf("ExclusiveStatus without a marker mode = %q", totals.Mode)
	}
}

func TestFloorExclusiveStatusRefusesReservationMode(t *testing.T) {
	root := t.TempDir()
	floorWriteMarker(t, root, "reservation")
	if _, err := ExclusiveStatus(context.Background(), root); err == nil {
		t.Fatal("ExclusiveStatus in reservation mode = nil, want error")
	}
}

func TestFloorExclusiveStatusReportsLockFailure(t *testing.T) {
	root := t.TempDir()
	floorWriteMarker(t, root, "exclusive")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ExclusiveStatus(ctx, root); err == nil {
		t.Fatal("ExclusiveStatus with a cancelled context = nil, want error")
	}
}

func TestFloorExclusiveStatusReportsChangedMarker(t *testing.T) {
	root := t.TempDir()
	floorWriteMarker(t, root, "exclusive")
	lock, err := openCoordinationLock(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := system.Production.Files.Flock(lock, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := ExclusiveStatus(context.Background(), root)
		done <- err
	}()
	time.Sleep(100 * time.Millisecond)
	writeFloorMarker(t, root, "[[[")
	_ = system.Production.Files.Flock(lock, syscall.LOCK_UN)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("ExclusiveStatus across a marker change = nil, want error")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ExclusiveStatus did not return after the marker changed")
	}
}

func TestFloorExclusiveStatusReportsHeavySessionDisagreement(t *testing.T) {
	root := t.TempDir()
	floorWriteMarker(t, root, "exclusive")
	if _, err := writeSessionRecord(root, floorLiveOwner(os.Getpid(), floorToken)); err != nil {
		t.Fatal(err)
	}
	heavyPath := filepath.Join(root, "heavy.lock")
	if err := os.Mkdir(heavyPath, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := writeLeaseOwner(heavyPath, floorLiveOwner(1, floorToken)); err != nil {
		t.Fatal(err)
	}
	if _, err := ExclusiveStatus(context.Background(), root); err == nil {
		t.Fatal("ExclusiveStatus with disagreeing ownership = nil, want error")
	}
}

func TestFloorExclusiveStatusNamesLegacyHeavy(t *testing.T) {
	root := t.TempDir()
	floorWriteMarker(t, root, "exclusive")
	heavyPath := filepath.Join(root, "heavy.lock")
	if err := os.Mkdir(heavyPath, 0o750); err != nil {
		t.Fatal(err)
	}
	owner := NewExclusiveSessionRecord(os.Getpid(), "legacy!", policy.TaskTransactional, 0, 0)
	if err := writeLeaseOwner(heavyPath, owner); err != nil {
		t.Fatal(err)
	}
	totals, err := ExclusiveStatus(context.Background(), root)
	if err != nil {
		t.Fatalf("ExclusiveStatus with a legacy heavy owner: %v", err)
	}
	if totals.LegacyEntries != 1 || totals.Transactional != 1 {
		t.Fatalf("ExclusiveStatus legacy totals = %+v, want one legacy transactional owner", totals)
	}
}

func TestFloorExclusiveStatusRefusesUnreadableInventory(t *testing.T) {
	root := t.TempDir()
	floorWriteMarker(t, root, "exclusive")
	if err := os.WriteFile(filepath.Join(root, "sessions"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ExclusiveStatus(context.Background(), root); err == nil {
		t.Fatal("ExclusiveStatus with a file sessions inventory = nil, want error")
	}
}

func TestFloorExclusiveStatusSessionMatrix(t *testing.T) {
	seed := func(t *testing.T, name, content string) string {
		t.Helper()
		root := t.TempDir()
		floorWriteMarker(t, root, "exclusive")
		if err := os.MkdirAll(filepath.Join(root, "sessions"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "sessions", name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return root
	}
	t.Run("missing schema", func(t *testing.T) {
		root := seed(t, floorToken+".json", `{"pid":1,"token":"`+floorToken+`","class":"ephemeral"}`)
		if _, err := ExclusiveStatus(context.Background(), root); err == nil {
			t.Fatal("ExclusiveStatus with a schemaless record = nil, want error")
		}
	})
	t.Run("invalid record", func(t *testing.T) {
		root := seed(t, floorToken+".json", `{"schemaVersion":1,"pid":1,"token":"`+floorDead+`","class":"ephemeral"}`)
		if _, err := ExclusiveStatus(context.Background(), root); err == nil {
			t.Fatal("ExclusiveStatus with a mismatched record = nil, want error")
		}
	})
	t.Run("unverifiable identity", func(t *testing.T) {
		root := seed(t, floorToken+".json", `{"schemaVersion":1,"pid":1,"token":"`+floorToken+`","class":"ephemeral","identityDevice":5}`)
		if _, err := ExclusiveStatus(context.Background(), root); err == nil {
			t.Fatal("ExclusiveStatus with a mismatched identity = nil, want error")
		}
	})
}

func TestFloorExclusiveStatusHeavyMatrix(t *testing.T) {
	seed := func(t *testing.T) (string, string) {
		t.Helper()
		root := t.TempDir()
		floorWriteMarker(t, root, "exclusive")
		heavyPath := filepath.Join(root, "heavy.lock")
		if err := os.Mkdir(heavyPath, 0o750); err != nil {
			t.Fatal(err)
		}
		return root, heavyPath
	}
	t.Run("uninspectable lock", func(t *testing.T) {
		root, _ := seed(t)
		if err := os.RemoveAll(filepath.Join(root, "heavy.lock")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("heavy.lock", filepath.Join(root, "heavy.lock")); err != nil {
			t.Fatal(err)
		}
		if _, err := ExclusiveStatus(context.Background(), root); err == nil {
			t.Fatal("ExclusiveStatus with a looping heavy lock = nil, want error")
		}
	})
	t.Run("undecodable owner", func(t *testing.T) {
		root, heavyPath := seed(t)
		if err := os.WriteFile(filepath.Join(heavyPath, "owner.json"), []byte("[[["), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ExclusiveStatus(context.Background(), root); err == nil {
			t.Fatal("ExclusiveStatus with garbage heavy owner = nil, want error")
		}
	})
	t.Run("future schema", func(t *testing.T) {
		root, heavyPath := seed(t)
		owner := floorLiveOwner(os.Getpid(), floorToken)
		owner.SchemaVersion = 2
		if err := writeLeaseOwner(heavyPath, owner); err != nil {
			t.Fatal(err)
		}
		if _, err := ExclusiveStatus(context.Background(), root); err == nil {
			t.Fatal("ExclusiveStatus with schema 2 heavy owner = nil, want error")
		}
	})
	t.Run("unverifiable identity", func(t *testing.T) {
		root, heavyPath := seed(t)
		owner := floorLiveOwner(os.Getpid(), floorToken)
		owner.IdentityDevice = 5
		if err := writeLeaseOwner(heavyPath, owner); err != nil {
			t.Fatal(err)
		}
		if _, err := ExclusiveStatus(context.Background(), root); err == nil {
			t.Fatal("ExclusiveStatus with mismatched heavy identity = nil, want error")
		}
	})
}

func TestFloorReservationStatusReportsPruneFailure(t *testing.T) {
	root, releases := floorStatusRoot(t, floorToken)
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	if err := os.RemoveAll(filepath.Join(root, "owner-metadata")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "owner-metadata"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReservationStatus(context.Background(), root); err == nil {
		t.Fatal("ReservationStatus with a file metadata inventory = nil, want error")
	}
}

func TestFloorReservationStatusReportsOwnerEntryFailure(t *testing.T) {
	root, releases := floorStatusRoot(t, floorToken)
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	if err := os.MkdirAll(filepath.Join(root, "owner-metadata"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "owner-metadata", floorToken+".json"), []byte("[[["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReservationStatus(context.Background(), root); err == nil {
		t.Fatal("ReservationStatus with garbage owner metadata = nil, want error")
	}
}

func TestFloorReservationStatusReportsWaiterEntryFailure(t *testing.T) {
	root := t.TempDir()
	floorWriteMarker(t, root, "reservation")
	identity, err := openReservationIdentity(system.Production, root, floorDead)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = identity.Close()
		_ = removeReservationIdentity(root, floorDead)
	}()
	device, inode, err := reservationIdentityMetadata(identity)
	if err != nil {
		t.Fatal(err)
	}
	ledger := floorEmptyLedger()
	ledger.NextSequence = 3
	ledger.Waiters = []reservationWaiter{{
		Token: floorDead, PID: os.Getpid(), Class: "ephemeral", Profile: "floor",
		Requested: ReservationVector{CPU: 1, MemoryBytes: 268435456},
		Sequence:  1, MaxOwners: 1,
		IdentityDevice: device, IdentityInode: inode,
	}}
	floorWriteLedger(t, root, ledger)
	if err := os.MkdirAll(filepath.Join(root, "owner-metadata"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "owner-metadata", floorDead+".json"), []byte("[[["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReservationStatus(context.Background(), root); err == nil {
		t.Fatal("ReservationStatus with garbage waiter metadata = nil, want error")
	}
}

func TestFloorReservationOwnerPeakMatrix(t *testing.T) {
	t.Run("nil session is zero", func(t *testing.T) {
		if peak, err := ReservationOwnerPeak(context.Background(), t.TempDir(), nil); peak != 0 || err != nil {
			t.Fatalf("ReservationOwnerPeak(nil) = %d, %v, want 0, nil", peak, err)
		}
	})
	t.Run("unreadable ledger", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(reservationLedgerPath(root), 0o750); err != nil {
			t.Fatal(err)
		}
		if _, err := ReservationOwnerPeak(context.Background(), root, &Session{Token: floorToken}); err == nil {
			t.Fatal("ReservationOwnerPeak with a directory ledger = nil, want error")
		}
	})
	t.Run("unverifiable ledger", func(t *testing.T) {
		root := t.TempDir()
		floorWriteLedger(t, root, floorEmptyLedger())
		if err := os.MkdirAll(filepath.Join(root, "reservation-identities"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(root, "reservation-identities", floorToken+".lock"), 0o750); err != nil {
			t.Fatal(err)
		}
		ledger := floorEmptyLedger()
		ledger.Owners = []ReservationOwner{{
			Token: floorToken, PID: os.Getpid(), Class: "ephemeral", Profile: "floor",
			Requested: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Allocated: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Sequence:  1, MaxOwners: 1,
		}}
		floorWriteLedger(t, root, ledger)
		if _, err := ReservationOwnerPeak(context.Background(), root, &Session{Token: floorToken}); err == nil {
			t.Fatal("ReservationOwnerPeak with a directory lock = nil, want error")
		}
	})
	t.Run("disappeared owner", func(t *testing.T) {
		root := t.TempDir()
		floorWriteLedger(t, root, floorEmptyLedger())
		if _, err := ReservationOwnerPeak(context.Background(), root, &Session{Token: floorToken}); err == nil {
			t.Fatal("ReservationOwnerPeak with a missing owner = nil, want error")
		}
	})
}

func TestFloorReservationVictimPresentMatrix(t *testing.T) {
	t.Run("unreadable ledger", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(reservationLedgerPath(root), 0o750); err != nil {
			t.Fatal(err)
		}
		_, port := floorDoublesSystem()
		victim := ReservationOwner{Token: floorToken}
		if _, err := reservationVictimPresent(port, context.Background(), root, victim); err == nil {
			t.Fatal("reservationVictimPresent with a directory ledger = nil, want error")
		}
	})
	t.Run("unverifiable ledger", func(t *testing.T) {
		root := t.TempDir()
		floorWriteLedger(t, root, floorEmptyLedger())
		if err := os.MkdirAll(filepath.Join(root, "reservation-identities"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(root, "reservation-identities", floorToken+".lock"), 0o750); err != nil {
			t.Fatal(err)
		}
		ledger := floorEmptyLedger()
		ledger.Owners = []ReservationOwner{{
			Token: floorToken, PID: os.Getpid(), Class: "ephemeral", Profile: "floor",
			Requested: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Allocated: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Sequence:  1, MaxOwners: 1,
		}}
		floorWriteLedger(t, root, ledger)
		victim := ReservationOwner{Token: floorToken}
		if _, err := reservationVictimPresent(system.Production, context.Background(), root, victim); err == nil {
			t.Fatal("reservationVictimPresent with a directory lock = nil, want error")
		}
	})
	t.Run("absent victim", func(t *testing.T) {
		root := t.TempDir()
		identity, err := openReservationIdentity(system.Production, root, floorDead)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = identity.Close()
			_ = removeReservationIdentity(root, floorDead)
		}()
		device, inode, err := reservationIdentityMetadata(identity)
		if err != nil {
			t.Fatal(err)
		}
		ledger := floorEmptyLedger()
		ledger.Owners = []ReservationOwner{{
			Token: floorDead, PID: os.Getpid(), Class: "ephemeral", Profile: "floor",
			Requested: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Allocated: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Sequence:  1, MaxOwners: 1,
			IdentityDevice: device, IdentityInode: inode,
		}}
		floorWriteLedger(t, root, ledger)
		victim := ReservationOwner{Token: floorToken}
		present, err := reservationVictimPresent(system.Production, context.Background(), root, victim)
		if err != nil || present {
			t.Fatalf("reservationVictimPresent for another token = %v, %v, want false, nil", present, err)
		}
	})
	t.Run("changed victim", func(t *testing.T) {
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
		ledger.Owners = []ReservationOwner{{
			Token: floorToken, PID: os.Getpid(), Class: "ephemeral", Profile: "floor",
			Requested: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Allocated: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Sequence:  1, MaxOwners: 1,
			IdentityDevice: device, IdentityInode: inode,
		}}
		floorWriteLedger(t, root, ledger)
		victim := ReservationOwner{Token: floorToken, ProcessGroup: 999}
		if _, err := reservationVictimPresent(system.Production, context.Background(), root, victim); err == nil {
			t.Fatal("reservationVictimPresent with a changed victim = nil, want error")
		}
	})
}

func TestFloorReservationSheddingSelectionMatrix(t *testing.T) {
	t.Run("nil session selects nothing", func(t *testing.T) {
		selected, cause, err := ReservationSheddingSelection(t.TempDir(), nil)
		if err != nil || selected || cause != ShedCauseNone {
			t.Fatalf("ReservationSheddingSelection(nil) = %v, %v, %v", selected, cause, err)
		}
	})
	t.Run("unreadable ledger", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(reservationLedgerPath(root), 0o750); err != nil {
			t.Fatal(err)
		}
		if _, _, err := ReservationSheddingSelection(root, &Session{Token: floorToken}); err == nil {
			t.Fatal("ReservationSheddingSelection with a directory ledger = nil, want error")
		}
	})
	t.Run("unverifiable ledger", func(t *testing.T) {
		root := t.TempDir()
		floorWriteLedger(t, root, floorEmptyLedger())
		if err := os.MkdirAll(filepath.Join(root, "reservation-identities"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(root, "reservation-identities", floorToken+".lock"), 0o750); err != nil {
			t.Fatal(err)
		}
		ledger := floorEmptyLedger()
		ledger.Owners = []ReservationOwner{{
			Token: floorToken, PID: os.Getpid(), Class: "ephemeral", Profile: "floor",
			Requested: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Allocated: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Sequence:  1, MaxOwners: 1,
		}}
		floorWriteLedger(t, root, ledger)
		if _, _, err := ReservationSheddingSelection(root, &Session{Token: floorToken}); err == nil {
			t.Fatal("ReservationSheddingSelection with a directory lock = nil, want error")
		}
	})
	t.Run("disappeared owner", func(t *testing.T) {
		root := t.TempDir()
		floorWriteLedger(t, root, floorEmptyLedger())
		if _, _, err := ReservationSheddingSelection(root, &Session{Token: floorToken}); err == nil {
			t.Fatal("ReservationSheddingSelection with a missing owner = nil, want error")
		}
	})
}

func TestFloorSelectPressureVictimMatrix(t *testing.T) {
	t.Run("unopenable lock", func(t *testing.T) {
		marker, err := os.CreateTemp(t.TempDir(), "root-*")
		if err != nil {
			t.Fatal(err)
		}
		_ = marker.Close()
		if _, _, err := selectPressureVictim(system.Production, marker.Name(), ShedCausePressure, false); err == nil {
			t.Fatal("selectPressureVictim under a file = nil, want error")
		}
	})
	t.Run("unreadable ledger", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(reservationLedgerPath(root), 0o750); err != nil {
			t.Fatal(err)
		}
		if _, _, err := selectPressureVictim(system.Production, root, ShedCausePressure, false); err == nil {
			t.Fatal("selectPressureVictim with a directory ledger = nil, want error")
		}
	})
	t.Run("unverifiable ledger", func(t *testing.T) {
		root := t.TempDir()
		floorWriteLedger(t, root, floorEmptyLedger())
		if err := os.MkdirAll(filepath.Join(root, "reservation-identities"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(root, "reservation-identities", floorToken+".lock"), 0o750); err != nil {
			t.Fatal(err)
		}
		ledger := floorEmptyLedger()
		ledger.Owners = []ReservationOwner{{
			Token: floorToken, PID: os.Getpid(), Class: "ephemeral", Profile: "floor",
			Requested: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Allocated: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Sequence:  1, MaxOwners: 1,
		}}
		floorWriteLedger(t, root, ledger)
		if _, _, err := selectPressureVictim(system.Production, root, ShedCausePressure, false); err == nil {
			t.Fatal("selectPressureVictim with a directory lock = nil, want error")
		}
	})
}
