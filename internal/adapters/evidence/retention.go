package evidence

import (
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
)

const (
	maximumInactiveBytes          int64 = 50 * 1024 * 1024
	atomicWriteTemporaryRetention       = time.Hour
)

func atomicWriteTemporary(name string) bool {
	return strings.HasPrefix(name, ".coordination-mode-") && strings.HasSuffix(name, ".tmp") ||
		strings.HasPrefix(name, ".reservations-") && strings.HasSuffix(name, ".tmp") ||
		strings.HasPrefix(name, ".summary-") && strings.HasSuffix(name, ".tmp") ||
		strings.HasPrefix(name, ".history-") && strings.HasSuffix(name, ".tmp") ||
		strings.HasPrefix(name, ".owner-metadata-") && strings.HasSuffix(name, ".tmp")
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

// livePID reports whether a process with the identifier exists, or exists under another owner. A signal of zero
// is only a probe, so no process is affected.
func livePID(port system.System, pid int) bool {
	if pid <= 0 {
		return false
	}

	err := port.Processes.Kill(pid, 0)

	return err == nil || errors.Is(err, syscall.EPERM)
}

func activePrefixes(port system.System, entries []fs.DirEntry, root string) (map[string]bool, error) {
	active := map[string]bool{}

	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".active.json") {
			continue
		}

		path := filepath.Join(root, entry.Name())
		data, err := port.Files.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}

		var marker activeMarker
		if json.Unmarshal(data, &marker) == nil && marker.SchemaVersion == 1 && livePID(port, marker.PID) {
			prefix := strings.TrimSuffix(path, ".active.json") + "."
			active[prefix] = true

			continue
		}

		if err := port.Files.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
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

//nolint:cyclop,gocognit // One pass classifies every protected, active, expired, and capped evidence entry.
func cleanupLocked(port system.System, root string, now time.Time, preserve ...string) error {
	entries, err := port.Files.ReadDir(root)
	if err != nil {
		return err
	}

	active, err := activePrefixes(port, entries, root)
	if err != nil {
		return err
	}

	preserved := map[string]bool{}
	for _, path := range preserve {
		preserved[path] = true
	}
	if err = compactLocked(port, root, now, active, preserved); err != nil {
		return err
	}
	entries, err = port.Files.ReadDir(root)
	if err != nil {
		return err
	}

	retained := []retainedFile{}

	for _, entry := range entries {
		if !entry.Type().IsRegular() || runtimeInternalFile(entry.Name()) || strings.HasSuffix(entry.Name(), ".active.json") {
			continue
		}

		path := filepath.Join(root, entry.Name())
		info, infoError := entry.Info()
		if errors.Is(infoError, fs.ErrNotExist) {
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
			if removeError := port.Files.Remove(path); removeError != nil && !errors.Is(removeError, fs.ErrNotExist) {
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
			if removeError := port.Files.Remove(path); removeError != nil && !errors.Is(removeError, fs.ErrNotExist) {
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
		if removeError := port.Files.Remove(entry.path); removeError != nil && !errors.Is(removeError, fs.ErrNotExist) {
			return removeError
		}

		total -= entry.size
	}

	return nil
}

// WriteRefused reports whether err is the evidence root refusing a write:
// permission denied, a read-only file system, or no space or quota left. Those
// are fixed on the root, not by retrying, and are distinct from evidence that
// was written earlier and can no longer be read.
func WriteRefused(err error) bool {
	return errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EROFS) ||
		errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EDQUOT)
}

// Cleanup removes expired evidence and prunes inactive files to the global cap.
// Active streams are excluded from that cap because each has its own hard limit.
func Cleanup(root string, now time.Time, preserve ...string) error {
	return cleanup(system.Production, root, now, preserve...)
}

func cleanup(port system.System, root string, now time.Time, preserve ...string) error {
	if root == "" {
		return errors.New("evidence root is empty")
	}

	if err := port.Files.MkdirAll(root, 0o700); err != nil {
		return err
	}
	if err := port.Files.Chmod(root, 0o700); err != nil {
		return err
	}

	return withWriterLock(port, root, func() error {
		return cleanupLocked(port, root, now, preserve...)
	})
}
