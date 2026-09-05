package guard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const (
	coordinationSchemaVersion   = 1
	coordinationModeExclusive   = "exclusive"
	coordinationModeReservation = "reservation"
	coordinationPollInterval    = 10 * time.Millisecond
	coordinationLifecycleWait   = 100 * time.Millisecond
	coordinationSelectionWait   = 500 * time.Millisecond
	// Reading the shared root is an inspection every repository performs while
	// its peers hold the lock for their own bounded transactions, so it waits
	// out ordinary contention instead of reporting it as a coordination error.
	coordinationObservationWait = 2 * time.Second
)

var errCoordinationDeferred = errors.New("shared coordination deferred admission")

// ErrCoordinationCleanupDeferred reports that ownership cleanup could not take
// the shared lock but left a reconcilable owner mark behind. The work itself
// succeeded, so callers surface this as a note rather than a failure.
var ErrCoordinationCleanupDeferred = errors.New("shared coordination deferred ownership cleanup")

type coordinationProcessIdentity struct {
	device uint64
	inode  uint64
}

type coordinationProcessGate struct {
	identity   coordinationProcessIdentity
	available  chan struct{}
	references int
}

var coordinationProcessGates = struct { //nolint:gochecknoglobals // Process-local flock serialization is shared by every guard entry point.
	sync.Mutex

	byIdentity map[coordinationProcessIdentity]*coordinationProcessGate
	held       map[*os.File]*coordinationProcessGate
}{
	byIdentity: make(map[coordinationProcessIdentity]*coordinationProcessGate),
	held:       make(map[*os.File]*coordinationProcessGate),
}

// IsCoordinationDeferred reports whether another compatible owner should be retried with exit 75.
func IsCoordinationDeferred(err error) bool {
	return errors.Is(err, errCoordinationDeferred)
}

type coordinationMarker struct {
	SchemaVersion int    `json:"schemaVersion"`
	Mode          string `json:"mode"`
}

func coordinationMarkerPath(root string) string {
	return filepath.Join(root, "coordination-mode.json")
}

func openCoordinationLock(root string) (*os.File, error) {
	return os.OpenFile(filepath.Join(root, "coordination.lock"), os.O_CREATE|os.O_RDWR, 0o600)
}

func filesystemIdentifier(value any) (uint64, error) {
	return strconv.ParseUint(fmt.Sprint(value), 10, 64)
}

func acquireCoordinationProcessGate(ctx context.Context, lock *os.File, wait time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var info unix.Stat_t
	if err := unix.Fstat(int(lock.Fd()), &info); err != nil {
		return err
	}
	device, err := filesystemIdentifier(info.Dev)
	if err != nil {
		return err
	}
	inode, err := filesystemIdentifier(info.Ino)
	if err != nil {
		return err
	}
	identity := coordinationProcessIdentity{device: device, inode: inode}
	coordinationProcessGates.Lock()
	gate := coordinationProcessGates.byIdentity[identity]
	if gate == nil {
		gate = &coordinationProcessGate{identity: identity, available: make(chan struct{}, 1)}
		gate.available <- struct{}{}
		coordinationProcessGates.byIdentity[identity] = gate
	}
	gate.references++
	coordinationProcessGates.Unlock()

	releaseReference := func() {
		coordinationProcessGates.Lock()
		gate.references--
		if gate.references == 0 {
			delete(coordinationProcessGates.byIdentity, gate.identity)
		}
		coordinationProcessGates.Unlock()
	}
	if wait <= 0 {
		select {
		case <-gate.available:
			coordinationProcessGates.Lock()
			coordinationProcessGates.held[lock] = gate
			coordinationProcessGates.Unlock()

			return nil
		default:
			releaseReference()

			return fmt.Errorf("%w: another admission is updating the shared root", errCoordinationDeferred)
		}
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-gate.available:
		coordinationProcessGates.Lock()
		coordinationProcessGates.held[lock] = gate
		coordinationProcessGates.Unlock()

		return nil
	case <-ctx.Done():
		releaseReference()

		return ctx.Err()
	case <-timer.C:
		releaseReference()

		return fmt.Errorf("%w: another admission is updating the shared root", errCoordinationDeferred)
	}
}

func releaseCoordinationProcessGate(lock *os.File) {
	coordinationProcessGates.Lock()
	gate := coordinationProcessGates.held[lock]
	delete(coordinationProcessGates.held, lock)
	if gate != nil {
		gate.references--
		gate.available <- struct{}{}
		if gate.references == 0 {
			delete(coordinationProcessGates.byIdentity, gate.identity)
		}
	}
	coordinationProcessGates.Unlock()
}

func releaseCoordinationLock(lock *os.File) error {
	if lock == nil {
		return nil
	}
	unlockError := unix.Flock(int(lock.Fd()), unix.LOCK_UN)
	closeError := lock.Close()
	releaseCoordinationProcessGate(lock)

	return errors.Join(unlockError, closeError)
}

func acquireCoordinationLock(ctx context.Context, root string, wait time.Duration) (*os.File, error) {
	lock, err := openCoordinationLock(root)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(wait)
	if err = acquireCoordinationProcessGate(ctx, lock, wait); err != nil {
		_ = lock.Close()

		return nil, err
	}
	for {
		err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			releaseCoordinationProcessGate(lock)
			_ = lock.Close()

			return nil, err
		}
		if wait == 0 || !time.Now().Before(deadline) {
			releaseCoordinationProcessGate(lock)
			_ = lock.Close()

			return nil, fmt.Errorf("%w: another admission is updating the shared root", errCoordinationDeferred)
		}

		remaining := time.Until(deadline)
		delay := min(coordinationPollInterval, remaining)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			releaseCoordinationProcessGate(lock)
			_ = lock.Close()

			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func lockCoordinationForRelease(root string) (*os.File, error) {
	return acquireCoordinationLock(context.Background(), root, coordinationLifecycleWait)
}

func readCoordinationMarker(root string) (coordinationMarker, bool, error) {
	data, err := os.ReadFile(coordinationMarkerPath(root))
	if errors.Is(err, os.ErrNotExist) {
		return coordinationMarker{}, false, nil
	}
	if err != nil {
		return coordinationMarker{}, false, err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	var marker coordinationMarker
	token, err := decoder.Token()
	if err != nil {
		return coordinationMarker{}, false, fmt.Errorf("decode coordination marker: %w", err)
	}
	delimiter, ok := token.(json.Delim)
	if !ok || delimiter != '{' {
		return coordinationMarker{}, false, errors.New("coordination marker must be a JSON object")
	}

	seen := map[string]bool{}
	for decoder.More() {
		keyToken, keyError := decoder.Token()
		if keyError != nil {
			return coordinationMarker{}, false, fmt.Errorf("decode coordination marker field: %w", keyError)
		}
		key, keyOK := keyToken.(string)
		if !keyOK {
			return coordinationMarker{}, false, errors.New("coordination marker field name must be a string")
		}
		if seen[key] {
			return coordinationMarker{}, false, fmt.Errorf("duplicate coordination marker field %q", key)
		}
		seen[key] = true

		switch key {
		case "schemaVersion":
			err = decoder.Decode(&marker.SchemaVersion)
		case "mode":
			err = decoder.Decode(&marker.Mode)
		default:
			return coordinationMarker{}, false, fmt.Errorf("unknown coordination marker field %q", key)
		}
		if err != nil {
			return coordinationMarker{}, false, fmt.Errorf("decode coordination marker field %q: %w", key, err)
		}
	}

	if _, err = decoder.Token(); err != nil {
		return coordinationMarker{}, false, fmt.Errorf("decode coordination marker end: %w", err)
	}
	if err = decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return coordinationMarker{}, false, errors.New("coordination marker must contain one JSON value")
	}
	if marker.SchemaVersion != coordinationSchemaVersion {
		return coordinationMarker{}, false, fmt.Errorf("unsupported coordination marker schema %d", marker.SchemaVersion)
	}
	if marker.Mode != coordinationModeExclusive && marker.Mode != coordinationModeReservation {
		return coordinationMarker{}, false, fmt.Errorf("unsupported coordination mode %q", marker.Mode)
	}

	return marker, true, nil
}

// ActiveCoordinationMode returns the validated runtime marker when one exists.
func ActiveCoordinationMode(root string) (string, bool, error) {
	marker, present, err := readCoordinationMarker(root)
	if err != nil || !present {
		return "", present, err
	}

	return marker.Mode, true, nil
}

func writeCoordinationMarker(root string, marker coordinationMarker) (returnError error) {
	temporary, err := os.CreateTemp(root, ".coordination-mode-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		if closeError := temporary.Close(); returnError == nil && closeError != nil {
			returnError = closeError
		}
		if removeError := os.Remove(temporaryPath); returnError == nil && removeError != nil && !errors.Is(removeError, os.ErrNotExist) {
			returnError = removeError
		}
	}()

	if err = temporary.Chmod(0o600); err != nil {
		return err
	}
	if err = json.NewEncoder(temporary).Encode(marker); err != nil {
		return err
	}
	if err = temporary.Sync(); err != nil {
		return err
	}

	return os.Rename(temporaryPath, coordinationMarkerPath(root))
}

func ensureExclusiveCoordination(root string) error {
	marker, present, err := readCoordinationMarker(root)
	if err != nil {
		return err
	}
	if !present {
		return writeCoordinationMarker(root, coordinationMarker{
			SchemaVersion: coordinationSchemaVersion,
			Mode:          coordinationModeExclusive,
		})
	}
	if marker.Mode == coordinationModeReservation {
		return fmt.Errorf("%w: reservation mode is active", errCoordinationDeferred)
	}

	return nil
}
