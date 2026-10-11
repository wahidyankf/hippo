package evidence

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/identity"
	"golang.org/x/sys/unix"
)

const maximumDisplayBytes = 512

var displayTokenPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

type displayRecord struct {
	SchemaVersion int    `json:"schemaVersion"`
	RunID         string `json:"runId"`
	Label         string `json:"label"`
}

// DisplayRepository reads optional display context without affecting accounting.
type DisplayRepository struct{}

func openDisplayDirectory(root string, create bool) (*os.File, error) {
	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = unix.Close(rootFD) }()
	if create {
		if err = unix.Mkdirat(rootFD, "run-display", 0o700); err != nil && !errors.Is(err, unix.EEXIST) {
			return nil, err
		}
	}
	directoryFD, err := unix.Openat(rootFD, "run-display", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	directory := os.NewFile(uintptr(directoryFD), "run-display")
	if create {
		if err = directory.Chmod(0o700); err != nil {
			return nil, errors.Join(err, directory.Close())
		}
	}
	return directory, nil
}

// ReadDisplay returns one advisory result for each requested complete run ID.
func (DisplayRepository) ReadDisplay(ctx context.Context, root string, runIDs []string) (map[string]application.DisplayAnnotation, error) {
	result := make(map[string]application.DisplayAnnotation, len(runIDs))
	directory, directoryError := openDisplayDirectory(root, false)
	if directory != nil {
		defer func() { _ = directory.Close() }()
	}
	for _, runID := range runIDs {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if !displayTokenPattern.MatchString(runID) {
			result[runID] = application.DisplayAnnotation{Availability: application.DisplayInvalid}
			continue
		}
		if directoryError != nil {
			availability := application.DisplayUnreadable
			if errors.Is(directoryError, os.ErrNotExist) {
				availability = application.DisplayMissing
			}
			result[runID] = application.DisplayAnnotation{Availability: availability}
			continue
		}
		result[runID] = readDisplay(directory, runID)
	}
	if errors.Is(directoryError, os.ErrNotExist) {
		return result, nil
	}
	return result, directoryError
}

func readDisplay(directory *os.File, runID string) application.DisplayAnnotation {
	descriptor, err := unix.Openat(int(directory.Fd()), runID+".json", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		availability := application.DisplayUnreadable
		if errors.Is(err, os.ErrNotExist) {
			availability = application.DisplayMissing
		}
		return application.DisplayAnnotation{Availability: availability}
	}
	file := os.NewFile(uintptr(descriptor), "display annotation")
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return application.DisplayAnnotation{Availability: application.DisplayUnreadable}
	}
	data, err := io.ReadAll(io.LimitReader(file, maximumDisplayBytes+1))
	if err = errors.Join(err, file.Close()); err != nil {
		return application.DisplayAnnotation{Availability: application.DisplayUnreadable}
	}
	if len(data) > maximumDisplayBytes {
		return application.DisplayAnnotation{Availability: application.DisplayInvalid}
	}
	return decodeDisplay(data, runID)
}

func decodeDisplay(data []byte, runID string) application.DisplayAnnotation {
	record, err := parseDisplayRecord(data)
	if err != nil || record.RunID != runID || record.SchemaVersion <= 0 {
		return application.DisplayAnnotation{Availability: application.DisplayInvalid}
	}
	if record.SchemaVersion != 1 {
		return application.DisplayAnnotation{Availability: application.DisplayUnsupported}
	}
	label, err := identity.ParseDisplayLabel(record.Label)
	if err != nil {
		return application.DisplayAnnotation{Availability: application.DisplayInvalid}
	}
	return application.DisplayAnnotation{Label: label, Availability: application.DisplayAvailable}
}

func parseDisplayRecord(data []byte) (displayRecord, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return displayRecord{}, errors.New("display annotation must be an object")
	}
	seen := map[string]bool{}
	var record displayRecord
	for decoder.More() {
		value, readError := decoder.Token()
		key, valid := value.(string)
		if readError != nil || !valid || seen[key] {
			return displayRecord{}, errors.New("display annotation field is invalid or duplicated")
		}
		seen[key] = true
		switch key {
		case "schemaVersion":
			err = decoder.Decode(&record.SchemaVersion)
		case "runId":
			err = decoder.Decode(&record.RunID)
		case "label":
			err = decoder.Decode(&record.Label)
		default:
			return displayRecord{}, errors.New("display annotation field is unknown")
		}
		if err != nil {
			return displayRecord{}, err
		}
	}
	if _, err = decoder.Token(); err != nil {
		return displayRecord{}, err
	}
	if err = decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) || len(seen) != 3 {
		return displayRecord{}, errors.New("display annotation must contain exactly one complete record")
	}
	return record, nil
}

// WriteDisplay atomically publishes private schema-one context for an owned token.
// The runtime caller holds the coordination lock before making that token visible.
func WriteDisplay(root, runID string, label identity.DisplayLabel) (returnError error) {
	if !displayTokenPattern.MatchString(runID) {
		return errors.New("display annotation token is invalid")
	}
	if _, err := identity.ParseDisplayLabel(label.String()); err != nil {
		return err
	}
	directory, err := openDisplayDirectory(root, true)
	if err != nil {
		return err
	}
	defer func() { returnError = errors.Join(returnError, directory.Close()) }()
	random := make([]byte, 8)
	if _, err = rand.Read(random); err != nil {
		return err
	}
	temporaryName := ".run-display-" + runID + "-" + hex.EncodeToString(random) + ".tmp"
	fd, err := unix.Openat(int(directory.Fd()), temporaryName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return err
	}
	temporary := os.NewFile(uintptr(fd), "display annotation temporary")
	defer func() {
		if temporary != nil {
			returnError = errors.Join(returnError, temporary.Close())
		}
		if removeError := unix.Unlinkat(int(directory.Fd()), temporaryName, 0); !errors.Is(removeError, unix.ENOENT) {
			returnError = errors.Join(returnError, removeError)
		}
	}()
	if err = temporary.Chmod(0o600); err != nil {
		return err
	}
	if err = json.NewEncoder(temporary).Encode(displayRecord{SchemaVersion: 1, RunID: runID, Label: label.String()}); err != nil {
		return err
	}
	if err = temporary.Sync(); err != nil {
		return err
	}
	err = temporary.Close()
	temporary = nil
	if err != nil {
		return err
	}
	return unix.Renameat(int(directory.Fd()), temporaryName, int(directory.Fd()), runID+".json")
}

func removeDisplayAt(directory *os.File, name string) error {
	var info unix.Stat_t
	if err := unix.Fstatat(int(directory.Fd()), name, &info, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return err
	}
	if info.Mode&unix.S_IFMT != unix.S_IFREG {
		return errors.New("display annotation is not a regular file")
	}
	err := unix.Unlinkat(int(directory.Fd()), name, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	return err
}

// RemoveDisplay removes only a regular annotation for a validated owned token.
func RemoveDisplay(root, runID string) (returnError error) {
	if !displayTokenPattern.MatchString(runID) {
		return errors.New("display annotation token is invalid")
	}
	directory, err := openDisplayDirectory(root, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { returnError = errors.Join(returnError, directory.Close()) }()
	return removeDisplayAt(directory, runID+".json")
}

// DisplayTokens enumerates supported, strictly valid sidecars for orphan census.
// Unknown, invalid, and unreadable annotations cannot establish protocol ownership.
func DisplayTokens(root string) (_ []string, returnError error) {
	directory, err := openDisplayDirectory(root, false)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { returnError = errors.Join(returnError, directory.Close()) }()
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	tokens := make([]string, 0, len(entries))
	for _, entry := range entries {
		runID, matched := strings.CutSuffix(entry.Name(), ".json")
		if matched && entry.Type().IsRegular() && displayTokenPattern.MatchString(runID) && readDisplay(directory, runID).Availability == application.DisplayAvailable {
			tokens = append(tokens, runID)
		}
	}
	return tokens, nil
}

// PruneDisplay removes proven-stale annotations and expired regular owned temporaries.
// The caller supplies a complete liveness proof and holds the coordination lock.
func PruneDisplay(root string, provenStaleRunIDs []string, now time.Time) (returnError error) {
	directory, err := openDisplayDirectory(root, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { returnError = errors.Join(returnError, directory.Close()) }()
	for _, runID := range provenStaleRunIDs {
		if !displayTokenPattern.MatchString(runID) {
			return errors.New("display annotation stale token is invalid")
		}
		returnError = errors.Join(returnError, removeDisplayAt(directory, runID+".json"))
	}
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return errors.Join(returnError, err)
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !displayTemporary(entry.Name()) {
			continue
		}
		var info unix.Stat_t
		infoError := unix.Fstatat(int(directory.Fd()), entry.Name(), &info, unix.AT_SYMLINK_NOFOLLOW)
		if infoError != nil {
			if errors.Is(infoError, unix.ENOENT) {
				continue
			}
			returnError = errors.Join(returnError, infoError)
			continue
		}
		if info.Mode&unix.S_IFMT != unix.S_IFREG {
			continue
		}
		if now.Sub(time.Unix(info.Mtim.Sec, info.Mtim.Nsec)) >= atomicWriteTemporaryRetention {
			returnError = errors.Join(returnError, removeDisplayAt(directory, entry.Name()))
		}
	}
	return returnError
}

func displayTemporary(name string) bool {
	value, prefix := strings.CutPrefix(name, ".run-display-")
	value, suffix := strings.CutSuffix(value, ".tmp")
	if !prefix || !suffix || len(value) != 49 || value[32] != '-' || !displayTokenPattern.MatchString(value[:32]) {
		return false
	}
	_, err := hex.DecodeString(value[33:])
	return err == nil
}

var _ application.DisplayAnnotations = DisplayRepository{}
