package guard

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/wahidyankf/hippo/internal/policy"
)

// SafetyReceipt records a queue or supervision terminal state without private command data.
type SafetyReceipt struct {
	SchemaVersion int               `json:"schemaVersion"`
	RunID         string            `json:"runId"`
	RecordedAt    string            `json:"recordedAt"`
	State         string            `json:"state"`
	Reason        string            `json:"reason"`
	Source        string            `json:"source,omitempty"`
	Tags          map[string]string `json:"tags,omitempty"`
	ResourceTier  string            `json:"resourceTier,omitempty"`
	TaskClass     policy.TaskClass  `json:"taskClass"`
}

func writeSafetyReceipt(
	root, runID, state, reason string,
	metadata ReservationMetadata,
	class policy.TaskClass,
	now time.Time,
) (returnError error) {
	if metadata.Source == "" {
		return nil
	}
	directory := filepath.Join(root, "receipts")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	receipt := SafetyReceipt{
		SchemaVersion: 1, RunID: runID, RecordedAt: now.UTC().Format(time.RFC3339Nano),
		State: state, Reason: reason, Source: metadata.Source, Tags: metadata.Tags,
		ResourceTier: metadata.Tier, TaskClass: class,
	}
	temporary, err := os.CreateTemp(directory, ".receipt-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			returnError = errors.Join(returnError, temporary.Close())
		}
		if removeError := os.Remove(temporaryPath); !errors.Is(removeError, os.ErrNotExist) {
			returnError = errors.Join(returnError, removeError)
		}
	}()
	if err = temporary.Chmod(0o600); err != nil {
		return err
	}
	if err = json.NewEncoder(temporary).Encode(receipt); err != nil {
		return err
	}
	if err = temporary.Sync(); err != nil {
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	closed = true
	destination := filepath.Join(directory, now.UTC().Format("20060102T150405.000000000Z")+"-"+runID+".json")

	return os.Rename(temporaryPath, destination)
}
