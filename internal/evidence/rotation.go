package evidence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const (
	defaultChunkBytes  int64 = 400 * 1024
	defaultChunkCount        = 5
	maximumLiveWriters       = 20
)

// Limits bounds one live raw evidence stream.
type Limits struct {
	ChunkBytes int64
	Chunks     int
}

// DefaultLimits retains five recent 400 KiB chunks per live session.
func DefaultLimits() Limits {
	return Limits{ChunkBytes: defaultChunkBytes, Chunks: defaultChunkCount}
}

// Writer stores newline-delimited JSON in a bounded set of recent chunks.
// A Writer is owned by one goroutine and is not safe for concurrent use.
type Writer struct {
	path       string
	markerPath string
	file       *os.File
	written    int64
	limits     Limits
}

type activeMarker struct {
	SchemaVersion int `json:"schemaVersion"`
	PID           int `json:"pid"`
}

func withWriterLock(root string, operation func() error) error {
	lock, err := os.OpenFile(filepath.Join(root, ".writers.lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()

	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = unix.Flock(int(lock.Fd()), unix.LOCK_UN) }()

	return operation()
}

func normalizeLimits(limits Limits) (Limits, error) {
	defaults := DefaultLimits()
	if limits.ChunkBytes == 0 {
		limits.ChunkBytes = defaults.ChunkBytes
	}
	if limits.Chunks == 0 {
		limits.Chunks = defaults.Chunks
	}
	if limits.ChunkBytes < 0 || limits.Chunks < 1 {
		return Limits{}, errors.New("evidence rotation limits must be positive")
	}

	return limits, nil
}

func markerPath(path string) string {
	extension := filepath.Ext(path)
	base := path[:len(path)-len(extension)]

	return base + ".active.json"
}

func rotatedPath(path string, generation int) string {
	extension := filepath.Ext(path)
	base := path[:len(path)-len(extension)]

	return fmt.Sprintf("%s.%d%s", base, generation, extension)
}

// NewWriter creates an exclusive current chunk and its live-owner marker.
func NewWriter(path string, limits Limits) (*Writer, error) {
	limits, err := normalizeLimits(limits)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}

	marker := markerPath(path)
	var file *os.File
	err = withWriterLock(filepath.Dir(path), func() error {
		entries, readError := os.ReadDir(filepath.Dir(path))
		if readError != nil {
			return readError
		}

		active, activeError := activePrefixes(entries, filepath.Dir(path))
		if activeError != nil {
			return activeError
		}
		if len(active) >= maximumLiveWriters {
			return fmt.Errorf("live evidence session limit reached (%d)", maximumLiveWriters)
		}

		file, readError = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if readError != nil {
			return readError
		}

		encoded, encodeError := json.Marshal(activeMarker{SchemaVersion: 1, PID: os.Getpid()})
		if encodeError != nil {
			return encodeError
		}

		return os.WriteFile(marker, append(encoded, '\n'), 0o600)
	})
	if err != nil {
		if file != nil {
			_ = file.Close()
		}
		_ = os.Remove(path)
		_ = os.Remove(marker)

		return nil, err
	}

	return &Writer{path: path, markerPath: marker, file: file, limits: limits}, nil
}

func (writer *Writer) abandon() {
	if writer.file != nil {
		_ = writer.file.Close()
		writer.file = nil
	}
	_ = os.Remove(writer.markerPath)
}

func (writer *Writer) rotate() error {
	if err := writer.file.Close(); err != nil {
		return err
	}
	writer.file = nil

	oldest := rotatedPath(writer.path, writer.limits.Chunks-1)
	if err := os.Remove(oldest); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	for generation := writer.limits.Chunks - 2; generation >= 1; generation-- {
		from := rotatedPath(writer.path, generation)
		to := rotatedPath(writer.path, generation+1)
		if err := os.Rename(from, to); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	if writer.limits.Chunks > 1 {
		if err := os.Rename(writer.path, rotatedPath(writer.path, 1)); err != nil {
			return err
		}
	} else if err := os.Remove(writer.path); err != nil {
		return err
	}

	file, err := os.OpenFile(writer.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}

	writer.file = file
	writer.written = 0

	return nil
}

// AppendJSON writes one complete JSON line, rotating before it crosses the cap.
func (writer *Writer) AppendJSON(value any) error {
	if writer == nil || writer.file == nil {
		return errors.New("evidence writer is closed")
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')

	if int64(len(encoded)) > writer.limits.ChunkBytes {
		return errors.New("one evidence sample exceeds the chunk limit")
	}
	if writer.written > 0 && writer.written+int64(len(encoded)) > writer.limits.ChunkBytes {
		if err := writer.rotate(); err != nil {
			writer.abandon()

			return err
		}
	}

	written, err := writer.file.Write(encoded)
	writer.written += int64(written)

	return err
}

// Close closes the current chunk and removes its live-owner marker.
func (writer *Writer) Close() error {
	if writer == nil || writer.file == nil {
		return errors.New("evidence writer is closed")
	}

	closeError := writer.file.Close()
	writer.file = nil
	removeError := withWriterLock(filepath.Dir(writer.path), func() error {
		return os.Remove(writer.markerPath)
	})
	if errors.Is(removeError, os.ErrNotExist) {
		removeError = nil
	}

	return errors.Join(closeError, removeError)
}
