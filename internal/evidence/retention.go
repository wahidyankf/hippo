package evidence

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

const (
	maximumInactiveBytes          int64 = 50 * 1024 * 1024
	atomicWriteTemporaryRetention       = time.Hour
)

func atomicWriteTemporary(name string) bool {
	return strings.HasPrefix(name, ".coordination-mode-") && strings.HasSuffix(name, ".tmp") ||
		strings.HasPrefix(name, ".reservations-") && strings.HasSuffix(name, ".tmp")
}

func runtimeInternalFile(name string) bool {
	return name == ".writers.lock" ||
		name == "coordination.lock" ||
		name == "coordination-mode.json" ||
		name == "reservations.json"
}

type retainedFile struct {
	path     string
	size     int64
	modified time.Time
}

func livePID(pid int) bool {
	if pid <= 0 {
		return false
	}

	err := syscall.Kill(pid, 0)

	return err == nil || errors.Is(err, syscall.EPERM)
}

func activePrefixes(entries []os.DirEntry, root string) (map[string]bool, error) {
	active := map[string]bool{}

	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".active.json") {
			continue
		}

		path := filepath.Join(root, entry.Name())
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}

		var marker activeMarker
		if json.Unmarshal(data, &marker) == nil && marker.SchemaVersion == 1 && livePID(marker.PID) {
			prefix := strings.TrimSuffix(path, ".active.json") + "."
			active[prefix] = true

			continue
		}

		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	return active, nil
}

func protectedByActiveWriter(path string, active map[string]bool) bool {
	for prefix := range active {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}

	return false
}

func cleanupLocked(root string, now time.Time, preserve ...string) error { //nolint:cyclop,gocognit // One pass classifies every protected, active, expired, and capped evidence entry.
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}

	active, err := activePrefixes(entries, root)
	if err != nil {
		return err
	}

	preserved := map[string]bool{}
	for _, path := range preserve {
		preserved[path] = true
	}

	retained := []retainedFile{}

	for _, entry := range entries {
		if !entry.Type().IsRegular() || runtimeInternalFile(entry.Name()) || strings.HasSuffix(entry.Name(), ".active.json") {
			continue
		}

		path := filepath.Join(root, entry.Name())
		info, infoError := entry.Info()
		if errors.Is(infoError, os.ErrNotExist) {
			continue
		}
		if infoError != nil {
			return infoError
		}
		if atomicWriteTemporary(entry.Name()) {
			age := now.Sub(info.ModTime())
			if age < 0 || age < atomicWriteTemporaryRetention {
				continue
			}
			if removeError := os.Remove(path); removeError != nil && !errors.Is(removeError, os.ErrNotExist) {
				return removeError
			}

			continue
		}

		retention := 7 * 24 * time.Hour
		if strings.HasSuffix(entry.Name(), ".summary.json") {
			retention = 30 * 24 * time.Hour
		}

		protected := preserved[path] || protectedByActiveWriter(path, active)
		if now.Sub(info.ModTime()) > retention && !protected {
			if removeError := os.Remove(path); removeError != nil && !errors.Is(removeError, os.ErrNotExist) {
				return removeError
			}

			continue
		}
		if protectedByActiveWriter(path, active) {
			continue
		}

		retained = append(retained, retainedFile{path: path, size: info.Size(), modified: info.ModTime()})
	}

	sort.Slice(retained, func(i, j int) bool { return retained[i].modified.Before(retained[j].modified) })

	var total int64
	for _, entry := range retained {
		total += entry.size
	}

	for _, entry := range retained {
		if total <= maximumInactiveBytes {
			break
		}
		if preserved[entry.path] {
			continue
		}
		if removeError := os.Remove(entry.path); removeError != nil && !errors.Is(removeError, os.ErrNotExist) {
			return removeError
		}

		total -= entry.size
	}

	return nil
}

// Cleanup removes expired evidence and prunes inactive files to the global cap.
// Active streams are excluded from that cap because each has its own hard limit.
func Cleanup(root string, now time.Time, preserve ...string) error {
	if root == "" {
		return errors.New("evidence root is empty")
	}

	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return err
	}

	return withWriterLock(root, func() error {
		return cleanupLocked(root, now, preserve...)
	})
}
