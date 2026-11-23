package runtime //nolint:testpackage // Floor leg adds the missing unit branches, as run_test.go does.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wahidyankf/hippo/internal/adapters/system"
)

// floorReservationOwner builds a ledger-valid owner with real identity metadata.
func floorReservationOwner(t *testing.T, root, token string, sequence uint64) ReservationOwner {
	t.Helper()
	identity, err := openReservationIdentity(system.Production, root, token)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = identity.Close()
		_ = removeReservationIdentity(root, token)
	})
	device, inode, err := reservationIdentityMetadata(identity)
	if err != nil {
		t.Fatal(err)
	}
	return ReservationOwner{
		Token: token, PID: os.Getpid(), Class: "ephemeral", Profile: "floor",
		Requested: ReservationVector{CPU: 1, MemoryBytes: 268435456},
		Allocated: ReservationVector{CPU: 1, MemoryBytes: 268435456},
		Sequence:  sequence, MaxOwners: 1,
		IdentityDevice: device, IdentityInode: inode,
	}
}

func floorWriteLedger(t *testing.T, root string, ledger reservationLedger) {
	t.Helper()
	data, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reservationLedgerPath(root), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func floorEmptyLedger() reservationLedger {
	return reservationLedger{SchemaVersion: reservationLedgerSchemaVersion, Capacity: ReservationVector{CPU: 8, MemoryBytes: 8 * 1024 * 1024 * 1024}, NextSequence: 3}
}

func floorWriteMarker(t *testing.T, root, mode string) {
	t.Helper()
	writeFloorMarker(t, root, `{"schemaVersion":1,"mode":"`+mode+`"}`)
}

func TestFloorOpenReservationIdentityMatrix(t *testing.T) {
	t.Run("invalid token", func(t *testing.T) {
		_, port := floorDoublesSystem()
		if _, err := openReservationIdentity(port, t.TempDir(), "../escape"); err == nil {
			t.Fatal("openReservationIdentity on an invalid token = nil, want error")
		}
	})
	t.Run("uncreatable directory", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "reservation-identities"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := openReservationIdentity(system.Production, root, floorToken); err == nil {
			t.Fatal("openReservationIdentity under a file = nil, want error")
		}
	})
	t.Run("held lock", func(t *testing.T) {
		host, port := floorDoublesSystem()
		host.Files.FlockErr = errors.New("floor: identity locked")
		if _, err := openReservationIdentity(port, t.TempDir(), floorToken); err == nil {
			t.Fatal("openReservationIdentity on a held lock = nil, want error")
		}
	})
	t.Run("anchored identity", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "reservation-identities"), 0o750); err != nil {
			t.Fatal(err)
		}
		anchor, err := reservationIdentityAnchorPath(root, floorToken)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(anchor, []byte("taken"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := openReservationIdentity(system.Production, root, floorToken); err == nil {
			t.Fatal("openReservationIdentity with a taken anchor = nil, want error")
		}
	})
}

func TestFloorObserveReservationIdentityMatrix(t *testing.T) {
	t.Run("unopenable candidate", func(t *testing.T) {
		marker, err := os.CreateTemp(t.TempDir(), "root-*")
		if err != nil {
			t.Fatal(err)
		}
		_ = marker.Close()
		if _, _, err := observeReservationIdentity(marker.Name() + "/missing"); err == nil {
			t.Fatal("observeReservationIdentity under a file = nil, want error")
		}
	})
	t.Run("missing candidate is absent", func(t *testing.T) {
		got, file, err := observeReservationIdentity(filepath.Join(t.TempDir(), "missing.lock"))
		if err != nil || got.Present || file != nil {
			t.Fatalf("observeReservationIdentity on a missing path = %+v, %v, %v", got, file, err)
		}
	})
}

func TestFloorReservationIdentityAliveMatrix(t *testing.T) {
	t.Run("invalid token", func(t *testing.T) {
		_, port := floorDoublesSystem()
		if _, err := reservationIdentityAlive(port, t.TempDir(), "../escape", 0, 0); err == nil {
			t.Fatal("reservationIdentityAlive on an invalid token = nil, want error")
		}
	})
	t.Run("unreadable anchor", func(t *testing.T) {
		root := t.TempDir()
		identity, err := openReservationIdentity(system.Production, root, floorToken)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = identity.Close()
			_ = removeReservationIdentity(root, floorToken)
		})
		anchor, err := reservationIdentityAnchorPath(root, floorToken)
		if err != nil {
			t.Fatal(err)
		}
		_ = os.Remove(anchor)
		if err := os.Mkdir(anchor, 0o750); err != nil {
			t.Fatal(err)
		}
		if _, err := reservationIdentityAlive(system.Production, root, floorToken, 0, 0); err == nil {
			t.Fatal("reservationIdentityAlive with a directory anchor = nil, want error")
		}
	})
	t.Run("missing metadata is an error", func(t *testing.T) {
		if _, _, err := reservationIdentityMetadata(nil); err == nil {
			t.Fatal("reservationIdentityMetadata(nil) = nil, want error")
		}
	})
}

func TestFloorConsumeReservationJSONMatrix(t *testing.T) {
	t.Run("nested object duplicate", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(reservationLedgerPath(root), []byte(`{"schemaVersion":1,"owners":[{"token":"x","token":"y"}]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		_, port := floorDoublesSystem()
		if _, err := readReservationLedger(port, root); err == nil {
			t.Fatal("readReservationLedger with a nested duplicate = nil, want error")
		}
	})
	t.Run("nested array duplicate", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(reservationLedgerPath(root), []byte(`[{"a":1,"a":2}]`), 0o600); err != nil {
			t.Fatal(err)
		}
		_, port := floorDoublesSystem()
		if _, err := readReservationLedger(port, root); err == nil {
			t.Fatal("readReservationLedger with an array duplicate = nil, want error")
		}
	})
	t.Run("trailing value", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(reservationLedgerPath(root), []byte(`{"schemaVersion":1} {}`), 0o600); err != nil {
			t.Fatal(err)
		}
		_, port := floorDoublesSystem()
		if _, err := readReservationLedger(port, root); err == nil {
			t.Fatal("readReservationLedger with a trailing value = nil, want error")
		}
	})
}

func TestFloorInitializeMissingReservationLedgerMatrix(t *testing.T) { //nolint:gocognit // One ledger-init matrix covers every unverifiable identity shape in place.
	_, port := floorDoublesSystem()
	t.Run("unreadable identities", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "reservation-identities"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := initializeMissingReservationLedger(port, root); err == nil {
			t.Fatal("initializeMissingReservationLedger under a file = nil, want error")
		}
	})
	t.Run("directory entry", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "reservation-identities", "sub"), 0o750); err != nil {
			t.Fatal(err)
		}
		if _, err := initializeMissingReservationLedger(port, root); err == nil {
			t.Fatal("initializeMissingReservationLedger with a directory entry = nil, want error")
		}
	})
	t.Run("invalid name", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "reservation-identities"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "reservation-identities", "bogus.txt"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := initializeMissingReservationLedger(port, root); err == nil {
			t.Fatal("initializeMissingReservationLedger with a bad name = nil, want error")
		}
	})
	t.Run("unknown extension", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "reservation-identities"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "reservation-identities", floorToken+".xyz"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := initializeMissingReservationLedger(port, root); err == nil {
			t.Fatal("initializeMissingReservationLedger with a bad extension = nil, want error")
		}
	})
	t.Run("incomplete pair", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "reservation-identities"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "reservation-identities", floorToken+".lock"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := initializeMissingReservationLedger(port, root); err == nil {
			t.Fatal("initializeMissingReservationLedger with a half pair = nil, want error")
		}
	})
	t.Run("unverifiable identity", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "reservation-identities"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(floorToken+".lock", filepath.Join(root, "reservation-identities", floorToken+".lock")); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "reservation-identities", floorToken+".anchor"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := initializeMissingReservationLedger(port, root); err == nil {
			t.Fatal("initializeMissingReservationLedger with a looping lock = nil, want error")
		}
	})
	t.Run("live owner blocks", func(t *testing.T) {
		root := t.TempDir()
		identity, err := openReservationIdentity(system.Production, root, floorToken)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = identity.Close()
			_ = removeReservationIdentity(root, floorToken)
		}()
		if _, err := initializeMissingReservationLedger(system.Production, root); err == nil {
			t.Fatal("initializeMissingReservationLedger with a live owner = nil, want error")
		}
	})
	t.Run("stale removal failure", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "reservation-identities"), 0o750); err != nil {
			t.Fatal(err)
		}
		for _, suffix := range []string{".lock", ".anchor"} {
			if err := os.Symlink("/nonexistent-floor-target", filepath.Join(root, "reservation-identities", floorToken+suffix)); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Chmod(filepath.Join(root, "reservation-identities"), 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "reservation-identities"), 0o750) })
		if _, err := initializeMissingReservationLedger(system.Production, root); err == nil {
			t.Fatal("initializeMissingReservationLedger with an unremovable stale identity = nil, want error")
		}
	})
}

func TestFloorReadReservationLedgerRefusesDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(reservationLedgerPath(root), 0o750); err != nil {
		t.Fatal(err)
	}
	_, port := floorDoublesSystem()
	if _, err := readReservationLedger(port, root); err == nil {
		t.Fatal("readReservationLedger on a directory = nil, want error")
	}
}

func TestFloorWriteReservationLedgerRefusesUnencodableLedger(t *testing.T) {
	ledger := floorEmptyLedger()
	ledger.Owners = []ReservationOwner{{
		Token: floorToken, PID: os.Getpid(), Class: "ephemeral", Profile: "floor",
		Requested: ReservationVector{CPU: 1, MemoryBytes: 268435456},
		Allocated: ReservationVector{CPU: 1, MemoryBytes: 268435456},
		Sequence:  1, MaxOwners: 1, Shedding: true, SheddingCause: ShedCause(255),
	}}
	if err := writeReservationLedger(t.TempDir(), ledger); err == nil {
		t.Fatal("writeReservationLedger with an unencodable ledger = nil, want error")
	}
}

func TestFloorWriteReservationLedgerRefusesFileRoot(t *testing.T) {
	marker, err := os.CreateTemp(t.TempDir(), "root-*")
	if err != nil {
		t.Fatal(err)
	}
	_ = marker.Close()
	if err := writeReservationLedger(marker.Name(), floorEmptyLedger()); err == nil {
		t.Fatal("writeReservationLedger under a file = nil, want error")
	}
}

func TestFloorRemoveReservationIdentityMatrix(t *testing.T) {
	t.Run("invalid token", func(t *testing.T) {
		if err := removeReservationIdentity(t.TempDir(), "../escape"); err == nil {
			t.Fatal("removeReservationIdentity on an invalid token = nil, want error")
		}
	})
	t.Run("unremovable entry", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "reservation-identities"), 0o750); err != nil {
			t.Fatal(err)
		}
		locked := filepath.Join(root, "reservation-identities", floorToken+".lock")
		if err := os.Mkdir(locked, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(locked, "child"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := removeReservationIdentity(root, floorToken); err == nil {
			t.Fatal("removeReservationIdentity on a non-empty directory = nil, want error")
		}
	})
}

func TestFloorReservationIdentityAnchorRefusesBadToken(t *testing.T) {
	if _, err := reservationIdentityAnchorPath(t.TempDir(), "../escape"); err == nil {
		t.Fatal("reservationIdentityAnchorPath on an invalid token = nil, want error")
	}
}

func TestFloorOpenReservationIdentityRefusesTakenAnchor(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "reservation-identities"), 0o750); err != nil {
		t.Fatal(err)
	}
	anchor, err := reservationIdentityAnchorPath(root, floorToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(anchor, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openReservationIdentity(system.Production, root, floorToken); err == nil {
		t.Fatal("openReservationIdentity with a taken anchor = nil, want error")
	}
}

func TestFloorReservationIdentityMetadataRefusesClosedIdentity(t *testing.T) {
	identity, err := os.CreateTemp(t.TempDir(), "identity-*.lock")
	if err != nil {
		t.Fatal(err)
	}
	name := identity.Name()
	_ = identity.Close()
	identity, err = os.OpenFile(name, os.O_RDONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_ = identity.Close()
	if _, _, err := reservationIdentityMetadata(identity); err == nil {
		t.Fatal("reservationIdentityMetadata on a closed identity = nil, want error")
	}
}

func TestFloorConsumeReservationJSONRefusesNonStringKey(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(reservationLedgerPath(root), []byte(`{{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, port := floorDoublesSystem()
	if _, err := readReservationLedger(port, root); err == nil {
		t.Fatal("readReservationLedger with a non-string ledger key = nil, want error")
	}
}

func TestFloorRemoveReservationIdentityRefusesBadToken(t *testing.T) {
	if err := removeReservationIdentity(t.TempDir(), "../escape"); err == nil {
		t.Fatal("removeReservationIdentity on an invalid token = nil, want error")
	}
}
