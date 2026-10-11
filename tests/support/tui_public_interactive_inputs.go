package support

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	runtimeadapter "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/internal/policy"
	"golang.org/x/sys/unix"
)

// RetireOwnedRunning changes a genuine held reservation, not a displayed row.
// Paused FIFO actors remain held and cannot be admitted by this release.
func (fixture *PublicTUILedger) RetireOwnedRunning(ctx context.Context, runID string) error {
	fixture.closeMu.Lock()
	defer fixture.closeMu.Unlock()
	for index, owner := range fixture.owners {
		if owner.Token != runID {
			continue
		}
		if err := runtimeadapter.ReleaseReservationContext(ctx, fixture.root, owner); err != nil {
			return err
		}
		fixture.owners = append(fixture.owners[:index], fixture.owners[index+1:]...)
		for rowIndex, row := range fixture.expected.Owners {
			if row.RunID == runID {
				fixture.expected.Owners = append(fixture.expected.Owners[:rowIndex], fixture.expected.Owners[rowIndex+1:]...)
				fixture.expected.ActiveOwners--
				fixture.expected.Ephemeral--
				fixture.expected.Allocated.CPU -= row.Allocated.CPU
				fixture.expected.Allocated.MemoryBytes -= row.Allocated.MemoryBytes
				break
			}
		}
		// The real engine removes the empty epoch ledger after its last identity
		// retires. Its capacity is unavailable/zero; all other liveness checks stay.
		if len(fixture.expected.Owners) == 0 && len(fixture.expected.Waiters) == 0 {
			fixture.expected.Capacity = coordination.ReservationVector{}
		}
		return fixture.VerifyLiveness(ctx)
	}
	return errors.New("refuse retirement of a foreign public running identity")
}

// StageOwnedText writes only the acquired owner's advisory inputs. The control
// category deliberately creates a malformed optional label, so the reader must
// refuse it safely; it does not inject a product snapshot or terminal output.
func (fixture *PublicTUILedger) StageOwnedText(kind string) error {
	if len(fixture.expected.Owners) == 0 {
		return errors.New("text fixture needs one held owner")
	}
	row := &fixture.expected.Owners[0]
	var source string
	switch kind {
	case "an overlong unbroken value":
		source = strings.Repeat("a", 64)
	case "wide Unicode characters":
		source = strings.Repeat("\u754c", 16)
	case "combining Unicode characters":
		source = strings.Repeat("e\u0301", 16)
	case "ANSI escapes and control bytes":
		data, err := json.Marshal(map[string]any{schemaVersionField: 1, tuiRunIDField: row.RunID, tuiLabelField: "unchecked\x1b[31m\x00\r\nvalue"})
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(fixture.root, "run-display", row.RunID+".json"), data, 0o600)
	default:
		return errors.New("unknown original untrusted text category")
	}
	if err := identity.Validate(identity.Value{SchemaVersion: identity.SchemaVersion, Source: source, Tags: row.Tags}); err != nil {
		return err
	}
	path := filepath.Join(fixture.root, "owner-metadata", row.RunID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		return err
	}
	record["source"] = source
	data, err = json.Marshal(record)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	row.Source = source
	return nil
}

// PrepareLargeLedger represents a persisted historical epoch with 1000 owners
// and 1000 FIFO identities. Every record has its own real held Flock inode and
// same-inode anchor. It does not claim these were admitted by the capped engine,
// or that this epoch budget is safe capacity on the current host.
func (fixture *PublicTUILedger) PrepareLargeLedger(ctx context.Context) error {
	if len(fixture.owners) != 1 || len(fixture.waiters) != 0 {
		return errors.New("large ledger requires the isolated one-owner initialization")
	}
	// Last-owner retirement removes the authentic empty epoch marker.
	mode, present, err := runtimeadapter.ActiveCoordinationMode(fixture.root)
	if err != nil || !present || mode != reservationMode {
		return errors.Join(errors.New("large epoch requires its genuine reservation marker"), err)
	}
	marker, err := os.ReadFile(filepath.Join(fixture.root, coordinationModeMarker))
	if err != nil {
		return err
	}
	if err := fixture.RetireOwnedRunning(ctx, fixture.owners[0].Token); err != nil {
		return err
	}
	var limit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &limit); err != nil {
		return err
	}
	if limit.Cur < 2200 {
		return fmt.Errorf("large original public fixture needs 2000 held identity descriptors plus overhead; actual soft limit=%d", limit.Cur)
	}
	directory := filepath.Join(fixture.root, "reservation-identities")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	vector := coordination.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB}
	capacity := coordination.ReservationVector{CPU: 1000, MemoryBytes: 1000 * vector.MemoryBytes}
	ledger := coordination.Ledger{SchemaVersion: coordination.LedgerSchemaVersion, Capacity: capacity, NextSequence: 2000}
	fixture.expected = coordination.ReservationTotals{SchemaVersion: 5, Mode: reservationMode, Capacity: capacity, ActiveOwners: 1000, WaitingOwners: 1000, Ephemeral: 1000, LegacyEntries: 2000, Allocated: capacity, Waiting: capacity}
	for index := range 2000 {
		if err := ctx.Err(); err != nil {
			return err
		}
		runID, stat, err := fixture.prepareLargeIdentity(directory)
		if err != nil {
			return err
		}
		sequence := uint64(index + 1)
		if index < 1000 {
			ledger.Owners = append(ledger.Owners, coordination.ReservationOwner{Token: runID, PID: os.Getpid(), Class: policy.TaskEphemeral, Profile: profileBalanced, Requested: vector, Allocated: vector, Sequence: sequence, MaxOwners: 20, PeakOwners: 20, IdentityDevice: nativeTUIDevice(stat), IdentityInode: stat.Ino})
			fixture.expected.Owners = append(fixture.expected.Owners, coordination.ReservationEntry{RunID: runID, State: tuiActiveState, Class: policy.TaskEphemeral, Profile: profileBalanced, Requested: vector, Allocated: vector, Legacy: true})
		} else {
			ledger.Waiters = append(ledger.Waiters, coordination.Waiter{Token: runID, PID: os.Getpid(), Class: policy.TaskEphemeral, Profile: profileBalanced, Requested: vector, Sequence: sequence, MaxOwners: 20, IdentityDevice: nativeTUIDevice(stat), IdentityInode: stat.Ino})
			fixture.expected.Waiters = append(fixture.expected.Waiters, coordination.ReservationEntry{RunID: runID, State: "waiting", Position: index - 999, Class: policy.TaskEphemeral, Profile: profileBalanced, Requested: vector, Legacy: true})
		}
	}
	if err := coordination.ValidateLedger(ledger); err != nil {
		return err
	}
	data, err := json.Marshal(ledger)
	if err != nil {
		return err
	}
	if err := fixture.publishLargeEpoch(ctx, data, marker); err != nil {
		return err
	}
	return fixture.VerifyLiveness(ctx)
}

// publishLargeEpoch serializes the genuinely held identity ledger and its
// original validated marker. It never publishes a marker before its ledger.
func (fixture *PublicTUILedger) publishLargeEpoch(ctx context.Context, data, marker []byte) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(fixture.root, "coordination.lock"), os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, lock.Close()) }()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return err
	}
	defer func() { result = errors.Join(result, unix.Flock(int(lock.Fd()), unix.LOCK_UN)) }()
	if _, present, err := runtimeadapter.ActiveCoordinationMode(fixture.root); err != nil || present {
		return errors.Join(errors.New("refuse replacement of another published epoch"), err)
	}
	if err := os.WriteFile(filepath.Join(fixture.root, "reservations.json"), data, 0o600); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(fixture.root, coordinationModeMarker), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeError := file.Write(marker)
	return errors.Join(writeError, file.Close())
}

func (fixture *PublicTUILedger) closeLargeLedger() error {
	// All mass records belong to these held descriptors. Clear only this epoch's
	// own records before dropping identities; waiter handles retire before owners.
	empty := coordination.Ledger{SchemaVersion: coordination.LedgerSchemaVersion, Capacity: fixture.expected.Capacity, NextSequence: 2000}
	data, err := json.Marshal(empty)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(fixture.root, "reservations.json"), data, 0o600); err != nil {
		return err
	}
	var result error
	for _, span := range [][2]int{{min(1000, len(fixture.largeIdentities)), len(fixture.largeIdentities)}, {0, min(1000, len(fixture.largeIdentities))}} {
		for index := span[0]; index < span[1]; index++ {
			file := fixture.largeIdentities[index]
			if file != nil {
				result = errors.Join(result, file.Close())
				fixture.largeIdentities[index] = nil
			}
		}
	}
	fixture.largeIdentities = nil
	return result
}

func (fixture *PublicTUILedger) prepareLargeIdentity(directory string) (string, syscall.Stat_t, error) {
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", syscall.Stat_t{}, err
	}
	runID := hex.EncodeToString(tokenBytes)
	path := filepath.Join(directory, runID+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return "", syscall.Stat_t{}, err
	}
	fixture.largeIdentities = append(fixture.largeIdentities, file)
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return "", syscall.Stat_t{}, err
	}
	if err := os.Link(path, filepath.Join(directory, runID+".anchor")); err != nil {
		return "", syscall.Stat_t{}, err
	}
	var stat syscall.Stat_t
	if err := syscall.Fstat(int(file.Fd()), &stat); err != nil {
		return "", syscall.Stat_t{}, err
	}
	// Supported POSIX device/inode fields are nonnegative kernel identities.
	if stat.Dev <= 0 || stat.Ino == 0 {
		return "", syscall.Stat_t{}, errors.New("large peer has no authentic device/inode")
	}
	return runID, stat, nil
}
