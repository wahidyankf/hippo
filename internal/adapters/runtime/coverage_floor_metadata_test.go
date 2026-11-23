package runtime //nolint:testpackage // Floor leg adds the missing unit branches, as run_test.go does.

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
)

var errFloorFileRefused = errors.New("floor: file operation refused")

type floorFailChmodFile struct{ *os.File }

func (file floorFailChmodFile) Chmod(fs.FileMode) error { return errFloorFileRefused }

type floorFailWriteFile struct{ *os.File }

func (file floorFailWriteFile) Write(_ []byte) (int, error) { return 0, errFloorFileRefused }

type floorFailSyncFile struct{ *os.File }

func (file floorFailSyncFile) Sync() error { return errFloorFileRefused }

type floorFailCloseFile struct{ *os.File }

func (file floorFailCloseFile) Close() error { return errFloorFileRefused }

// floorProductionWithTemp returns Production with CreateTemp replaced by wrap,
// so file-method failures are observable without touching the kernel.
func floorProductionWithTemp(t *testing.T, wrap func(file *os.File) system.File) system.System {
	t.Helper()
	port := system.Production
	port.Files.CreateTemp = func(dir, pattern string) (system.File, error) {
		backing, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		t.Cleanup(func() {
			_ = backing.Close()
			_ = os.Remove(backing.Name())
		})
		if wrap == nil {
			return backing, nil
		}

		return wrap(backing), nil
	}

	return port
}

func floorMetadata() ReservationMetadata {
	return ReservationMetadata{Source: "hippo", Tags: map[string]string{"plan": "floor"}, Tier: "light"}
}

func floorMetadataValue(token string) reservationMetadataFile {
	return reservationMetadataFile{
		SchemaVersion: reservationMetadataSchemaVersion, RunID: token, Source: "hippo",
		Minimum:      ReservationVector{CPU: 1, MemoryBytes: 268435456},
		Maximum:      ReservationVector{CPU: 1, MemoryBytes: 268435456},
		RegisteredAt: time.Unix(1700000000, 0).UTC().Format(time.RFC3339Nano),
		Deadline:     time.Unix(1700003600, 0).UTC().Format(time.RFC3339Nano),
	}
}

func TestFloorWriteReservationMetadataMatrix(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	t.Run("invalid identity", func(t *testing.T) {
		_, port := floorDoublesSystem()
		metadata := ReservationMetadata{Source: "has space!"}
		err := writeReservationMetadata(
			port, t.TempDir(), floorToken, metadata,
			ReservationVector{}, ReservationVector{}, now, now,
		)
		if err == nil {
			t.Fatal("writeReservationMetadata with a bad source = nil, want error")
		}
	})
	t.Run("unwritable directory", func(t *testing.T) {
		marker, err := os.CreateTemp(t.TempDir(), "root-*")
		if err != nil {
			t.Fatal(err)
		}
		_ = marker.Close()
		err = writeReservationMetadata(
			system.Production, marker.Name(), floorToken, floorMetadata(),
			ReservationVector{}, ReservationVector{}, now, now,
		)
		if err == nil {
			t.Fatal("writeReservationMetadata under a file = nil, want error")
		}
	})
	t.Run("uncreatable temporary", func(t *testing.T) {
		port := system.Production
		port.Files.CreateTemp = func(string, string) (system.File, error) {
			return nil, errFloorFileRefused
		}
		err := writeReservationMetadata(
			port, t.TempDir(), floorToken, floorMetadata(),
			ReservationVector{}, ReservationVector{}, now, now,
		)
		if !errors.Is(err, errFloorFileRefused) {
			t.Fatalf("writeReservationMetadata without temp = %v, want refusal", err)
		}
	})
	t.Run("chmod failure", func(t *testing.T) {
		root := t.TempDir()
		port := floorProductionWithTemp(t, func(file *os.File) system.File {
			return floorFailChmodFile{file}
		})
		err := writeReservationMetadata(
			port, root, floorToken, floorMetadata(),
			ReservationVector{}, ReservationVector{}, now, now,
		)
		if !errors.Is(err, errFloorFileRefused) {
			t.Fatalf("writeReservationMetadata with failing chmod = %v, want refusal", err)
		}
	})
	t.Run("encode failure", func(t *testing.T) {
		root := t.TempDir()
		port := floorProductionWithTemp(t, func(file *os.File) system.File {
			return floorFailWriteFile{file}
		})
		err := writeReservationMetadata(
			port, root, floorToken, floorMetadata(),
			ReservationVector{}, ReservationVector{}, now, now,
		)
		if !errors.Is(err, errFloorFileRefused) {
			t.Fatalf("writeReservationMetadata with failing writes = %v, want refusal", err)
		}
	})
	t.Run("sync failure", func(t *testing.T) {
		root := t.TempDir()
		port := floorProductionWithTemp(t, func(file *os.File) system.File {
			return floorFailSyncFile{file}
		})
		err := writeReservationMetadata(
			port, root, floorToken, floorMetadata(),
			ReservationVector{}, ReservationVector{}, now, now,
		)
		if !errors.Is(err, errFloorFileRefused) {
			t.Fatalf("writeReservationMetadata with failing sync = %v, want refusal", err)
		}
	})
	t.Run("remove failure joins", func(t *testing.T) {
		root := t.TempDir()
		port := system.Production
		port.Files.Remove = func(string) error { return errFloorFileRefused }
		err := writeReservationMetadata(
			port, root, floorToken, floorMetadata(),
			ReservationVector{}, ReservationVector{}, now, now,
		)
		if !errors.Is(err, errFloorFileRefused) {
			t.Fatalf("writeReservationMetadata with failing remove = %v, want refusal", err)
		}
	})
}

func TestFloorReadReservationMetadataMatrix(t *testing.T) {
	_, port := floorDoublesSystem()
	t.Run("invalid token", func(t *testing.T) {
		if _, _, err := readReservationMetadata(port, t.TempDir(), "../escape"); err == nil {
			t.Fatal("readReservationMetadata on an invalid token = nil, want error")
		}
	})
	t.Run("unreadable file", func(t *testing.T) {
		marker, err := os.CreateTemp(t.TempDir(), "root-*")
		if err != nil {
			t.Fatal(err)
		}
		_ = marker.Close()
		if _, _, err := readReservationMetadata(system.Production, marker.Name(), floorToken); err == nil {
			t.Fatal("readReservationMetadata under a file = nil, want error")
		}
	})
	t.Run("undecodable file", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "owner-metadata"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "owner-metadata", floorToken+".json"), []byte("[[["), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := readReservationMetadata(system.Production, root, floorToken); err == nil {
			t.Fatal("readReservationMetadata with garbage = nil, want error")
		}
	})
	t.Run("mismatched identity", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "owner-metadata"), 0o750); err != nil {
			t.Fatal(err)
		}
		value := floorMetadataValue(floorDead)
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "owner-metadata", floorToken+".json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := readReservationMetadata(system.Production, root, floorToken); err == nil {
			t.Fatal("readReservationMetadata with a mismatched run = nil, want error")
		}
	})
	t.Run("invalid identity", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "owner-metadata"), 0o750); err != nil {
			t.Fatal(err)
		}
		value := floorMetadataValue(floorToken)
		value.Source = "has space!"
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "owner-metadata", floorToken+".json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := readReservationMetadata(system.Production, root, floorToken); err == nil {
			t.Fatal("readReservationMetadata with a bad source = nil, want error")
		}
	})
}

func TestFloorRemoveReservationMetadataInvalidToken(t *testing.T) {
	_, port := floorDoublesSystem()
	if err := removeReservationMetadata(port, t.TempDir(), "../escape"); err == nil {
		t.Fatal("removeReservationMetadata on an invalid token = nil, want error")
	}
}

func TestFloorPruneReservationMetadataMatrix(t *testing.T) {
	ledgerWith := func(tokens ...string) reservationLedger {
		ledger := floorEmptyLedger()
		for index, token := range tokens {
			ledger.Owners = append(ledger.Owners, ReservationOwner{
				Token: token, PID: os.Getpid(), Class: "ephemeral", Profile: "floor",
				Requested: ReservationVector{CPU: 1, MemoryBytes: 268435456},
				Allocated: ReservationVector{CPU: 1, MemoryBytes: 268435456},
				Sequence:  uint64(index + 1), MaxOwners: 1,
			})
		}
		return ledger
	}
	t.Run("unreadable inventory", func(t *testing.T) {
		marker, err := os.CreateTemp(t.TempDir(), "root-*")
		if err != nil {
			t.Fatal(err)
		}
		_ = marker.Close()
		if err := pruneReservationMetadata(system.Production, marker.Name(), floorEmptyLedger()); err == nil {
			t.Fatal("pruneReservationMetadata under a file = nil, want error")
		}
	})
	t.Run("skips live and irregular entries", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "owner-metadata"), 0o750); err != nil {
			t.Fatal(err)
		}
		for name, content := range map[string]string{
			"notes.txt":          "x",
			floorToken + ".json": "x",
		} {
			if err := os.WriteFile(filepath.Join(root, "owner-metadata", name), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Mkdir(filepath.Join(root, "owner-metadata", "sub"), 0o750); err != nil {
			t.Fatal(err)
		}
		ledger := floorEmptyLedger()
		ledger.Owners = ledgerWith(floorToken).Owners
		ledger.Waiters = []reservationWaiter{{
			Token: floorDead, PID: os.Getpid(), Class: "ephemeral", Profile: "floor",
			Requested: ReservationVector{CPU: 1, MemoryBytes: 268435456},
			Sequence:  2, MaxOwners: 1,
		}}
		if err := pruneReservationMetadata(system.Production, root, ledger); err != nil {
			t.Fatalf("pruneReservationMetadata with live and irregular entries: %v", err)
		}
	})
	t.Run("unremovable entry", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "owner-metadata"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "owner-metadata", floorDead+".json"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(root, "owner-metadata"), 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "owner-metadata"), 0o750) })
		if err := pruneReservationMetadata(system.Production, root, floorEmptyLedger()); err == nil {
			t.Fatal("pruneReservationMetadata under a read-only inventory = nil, want error")
		}
	})
}

func TestFloorWriteSafetyReceiptMatrix(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	metadata := ReservationMetadata{Source: "hippo"}
	t.Run("unwritable directory", func(t *testing.T) {
		marker, err := os.CreateTemp(t.TempDir(), "root-*")
		if err != nil {
			t.Fatal(err)
		}
		_ = marker.Close()
		if err := writeSafetyReceipt(
			system.Production, marker.Name(), floorToken, "never-started", "reason", metadata, "ephemeral", now,
		); err == nil {
			t.Fatal("writeSafetyReceipt under a file = nil, want error")
		}
	})
	t.Run("uncreatable temporary", func(t *testing.T) {
		port := system.Production
		port.Files.CreateTemp = func(string, string) (system.File, error) {
			return nil, errFloorFileRefused
		}
		if err := writeSafetyReceipt(
			port, t.TempDir(), floorToken, "never-started", "reason", metadata, "ephemeral", now,
		); !errors.Is(err, errFloorFileRefused) {
			t.Fatalf("writeSafetyReceipt without temp = %v, want refusal", err)
		}
	})
	t.Run("chmod failure", func(t *testing.T) {
		root := t.TempDir()
		port := floorProductionWithTemp(t, func(file *os.File) system.File {
			return floorFailChmodFile{file}
		})
		if err := writeSafetyReceipt(
			port, root, floorToken, "never-started", "reason", metadata, "ephemeral", now,
		); !errors.Is(err, errFloorFileRefused) {
			t.Fatalf("writeSafetyReceipt with failing chmod = %v, want refusal", err)
		}
	})
	t.Run("encode failure", func(t *testing.T) {
		root := t.TempDir()
		port := floorProductionWithTemp(t, func(file *os.File) system.File {
			return floorFailWriteFile{file}
		})
		if err := writeSafetyReceipt(
			port, root, floorToken, "never-started", "reason", metadata, "ephemeral", now,
		); !errors.Is(err, errFloorFileRefused) {
			t.Fatalf("writeSafetyReceipt with failing writes = %v, want refusal", err)
		}
	})
	t.Run("sync failure", func(t *testing.T) {
		root := t.TempDir()
		port := floorProductionWithTemp(t, func(file *os.File) system.File {
			return floorFailSyncFile{file}
		})
		if err := writeSafetyReceipt(
			port, root, floorToken, "never-started", "reason", metadata, "ephemeral", now,
		); !errors.Is(err, errFloorFileRefused) {
			t.Fatalf("writeSafetyReceipt with failing sync = %v, want refusal", err)
		}
	})
	t.Run("close failure", func(t *testing.T) {
		root := t.TempDir()
		port := floorProductionWithTemp(t, func(file *os.File) system.File {
			return floorFailCloseFile{file}
		})
		if err := writeSafetyReceipt(
			port, root, floorToken, "never-started", "reason", metadata, "ephemeral", now,
		); !errors.Is(err, errFloorFileRefused) {
			t.Fatalf("writeSafetyReceipt with failing close = %v, want refusal", err)
		}
	})
	t.Run("remove failure joins", func(t *testing.T) {
		root := t.TempDir()
		port := system.Production
		port.Files.Remove = func(string) error { return errFloorFileRefused }
		if err := writeSafetyReceipt(
			port, root, floorToken, "never-started", "reason", metadata, "ephemeral", now,
		); !errors.Is(err, errFloorFileRefused) {
			t.Fatalf("writeSafetyReceipt with failing remove = %v, want refusal", err)
		}
	})
	t.Run("happy path persists", func(t *testing.T) {
		root := t.TempDir()
		if err := writeSafetyReceipt(
			system.Production, root, floorToken, "never-started", "reason", metadata, "ephemeral", now,
		); err != nil {
			t.Fatalf("writeSafetyReceipt: %v", err)
		}
		entries, err := os.ReadDir(filepath.Join(root, "receipts"))
		if err != nil || len(entries) != 1 {
			t.Fatalf("receipts = %v, %v, want one persisted receipt", entries, err)
		}
	})
}
