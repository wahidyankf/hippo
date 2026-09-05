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
	"time"

	"golang.org/x/sys/unix"
)

const (
	coordinationSchemaVersion   = 1
	coordinationModeExclusive   = "exclusive"
	coordinationModeReservation = "reservation"
	coordinationPollInterval    = 10 * time.Millisecond
)

var errCoordinationDeferred = errors.New("shared coordination deferred admission")

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

func releaseCoordinationLock(lock *os.File) error {
	if lock == nil {
		return nil
	}

	return errors.Join(
		unix.Flock(int(lock.Fd()), unix.LOCK_UN),
		lock.Close(),
	)
}

func acquireCoordinationLock(ctx context.Context, root string, wait time.Duration) (*os.File, error) {
	lock, err := openCoordinationLock(root)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(wait)
	for {
		err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			_ = lock.Close()

			return nil, err
		}
		if wait == 0 || !time.Now().Before(deadline) {
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
			_ = lock.Close()

			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func lockCoordinationForRelease(root string) (*os.File, error) {
	lock, err := openCoordinationLock(root)
	if err != nil {
		return nil, err
	}

	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX); err != nil {
		_ = lock.Close()

		return nil, err
	}

	return lock, nil
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
