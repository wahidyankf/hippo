package guard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/internal/policy"
)

const reservationMetadataSchemaVersion = 1

// ReservationMetadata is privacy-safe context supplied before queue registration.
type ReservationMetadata struct {
	Source string
	Tags   map[string]string
	Tier   string
}

type reservationMetadataFile struct {
	SchemaVersion int               `json:"schemaVersion"`
	RunID         string            `json:"runId"`
	Source        string            `json:"source"`
	Tags          map[string]string `json:"tags,omitempty"`
	Tier          string            `json:"tier,omitempty"`
	Minimum       ReservationVector `json:"minimum"`
	Maximum       ReservationVector `json:"maximum"`
	RegisteredAt  string            `json:"registeredAt"`
	Deadline      string            `json:"deadline"`
}

// ReservationEntry is one privacy-safe live queue or owner row.
type ReservationEntry struct {
	RunID        string            `json:"runId"`
	State        string            `json:"state"`
	Position     int               `json:"position,omitempty"`
	Class        policy.TaskClass  `json:"class"`
	Profile      string            `json:"profile"`
	Source       string            `json:"source,omitempty"`
	Tags         map[string]string `json:"tags,omitempty"`
	Tier         string            `json:"tier,omitempty"`
	Requested    ReservationVector `json:"requested"`
	Allocated    ReservationVector `json:"allocated,omitzero"`
	Minimum      ReservationVector `json:"minimum,omitzero"`
	Maximum      ReservationVector `json:"maximum,omitzero"`
	RegisteredAt string            `json:"registeredAt,omitempty"`
	Deadline     string            `json:"deadline,omitempty"`
	Legacy       bool              `json:"legacy,omitempty"`
}

func reservationMetadataPath(root, token string) (string, error) {
	if !sessionTokenPattern.MatchString(token) {
		return "", errors.New("invalid reservation metadata token")
	}

	return filepath.Join(root, "owner-metadata", token+".json"), nil
}

func writeReservationMetadata(
	root, token string,
	metadata ReservationMetadata,
	minimum, maximum ReservationVector,
	registeredAt, deadline time.Time,
) (returnError error) {
	if err := identity.Validate(identity.Value{
		SchemaVersion: identity.SchemaVersion, Source: metadata.Source, Tags: metadata.Tags,
	}); err != nil {
		return err
	}
	path, err := reservationMetadataPath(root, token)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	value := reservationMetadataFile{
		SchemaVersion: reservationMetadataSchemaVersion,
		RunID:         token, Source: metadata.Source, Tags: metadata.Tags, Tier: metadata.Tier,
		Minimum: minimum, Maximum: maximum,
		RegisteredAt: registeredAt.UTC().Format(time.RFC3339Nano),
		Deadline:     deadline.UTC().Format(time.RFC3339Nano),
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".owner-metadata-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		returnError = errors.Join(returnError, temporary.Close())
		if removeError := os.Remove(temporaryPath); !errors.Is(removeError, os.ErrNotExist) {
			returnError = errors.Join(returnError, removeError)
		}
	}()
	if err = temporary.Chmod(0o600); err != nil {
		return err
	}
	if err = json.NewEncoder(temporary).Encode(value); err != nil {
		return err
	}
	if err = temporary.Sync(); err != nil {
		return err
	}

	return os.Rename(temporaryPath, path)
}

func readReservationMetadata(root, token string) (reservationMetadataFile, bool, error) {
	path, err := reservationMetadataPath(root, token)
	if err != nil {
		return reservationMetadataFile{}, false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return reservationMetadataFile{}, false, nil
	}
	if err != nil {
		return reservationMetadataFile{}, false, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value reservationMetadataFile
	if err = decoder.Decode(&value); err != nil {
		return reservationMetadataFile{}, false, err
	}
	if value.SchemaVersion != reservationMetadataSchemaVersion || value.RunID != token {
		return reservationMetadataFile{}, false, errors.New("reservation metadata identity is invalid")
	}
	if err = identity.Validate(identity.Value{
		SchemaVersion: identity.SchemaVersion, Source: value.Source, Tags: value.Tags,
	}); err != nil {
		return reservationMetadataFile{}, false, fmt.Errorf("reservation metadata: %w", err)
	}

	return value, true, nil
}

func removeReservationMetadata(root, token string) error {
	path, err := reservationMetadataPath(root, token)
	if err != nil {
		return err
	}
	if err = os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}

func pruneReservationMetadata(root string, ledger reservationLedger) error {
	directory := filepath.Join(root, "owner-metadata")
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	live := map[string]bool{}
	for _, owner := range ledger.Owners {
		live[owner.Token] = true
	}
	for _, waiter := range ledger.Waiters {
		live[waiter.Token] = true
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		token := strings.TrimSuffix(entry.Name(), ".json")
		if live[token] || !sessionTokenPattern.MatchString(token) {
			continue
		}
		if err = os.Remove(filepath.Join(directory, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	return nil
}

func reservationEntry(
	root, token, state string,
	position int,
	class policy.TaskClass,
	profile string,
	requested, allocated ReservationVector,
) (ReservationEntry, error) {
	entry := ReservationEntry{
		RunID: token, State: state, Position: position, Class: class, Profile: profile,
		Requested: requested, Allocated: allocated,
	}
	metadata, present, err := readReservationMetadata(root, token)
	if err != nil {
		return ReservationEntry{}, err
	}
	if !present {
		entry.Legacy = true

		return entry, nil
	}
	entry.Source, entry.Tags, entry.Tier = metadata.Source, metadata.Tags, metadata.Tier
	entry.Minimum, entry.Maximum = metadata.Minimum, metadata.Maximum
	entry.RegisteredAt, entry.Deadline = metadata.RegisteredAt, metadata.Deadline

	return entry, nil
}
