package application

import (
	"context"

	"github.com/wahidyankf/hippo/internal/identity"
)

// DisplayAvailability describes an advisory annotation independently of accounting.
type DisplayAvailability uint8

const (
	// DisplayUnavailable is absent or unknown annotation availability.
	DisplayUnavailable DisplayAvailability = iota
	// DisplayAvailable identifies a validated, matching annotation.
	DisplayAvailable
	// DisplayMissing means no annotation was published for the live run.
	DisplayMissing
	// DisplayInvalid means the annotation failed bounded syntax or identity validation.
	DisplayInvalid
	// DisplayUnsupported means the annotation uses an unsupported positive schema.
	DisplayUnsupported
	// DisplayUnreadable means the annotation could not be read safely.
	DisplayUnreadable
)

// String returns only safe fixed availability words, including unknown values.
func (availability DisplayAvailability) String() string {
	switch availability {
	case DisplayAvailable:
		return "available"
	case DisplayMissing:
		return "missing"
	case DisplayInvalid:
		return "invalid"
	case DisplayUnsupported:
		return "unsupported"
	case DisplayUnreadable:
		return "unreadable"
	case DisplayUnavailable:
		return "unavailable"
	default:
		return "unavailable"
	}
}

// DisplayAnnotation contains only validated display context and its availability.
type DisplayAnnotation struct {
	Label        identity.DisplayLabel `json:"Label"`
	Availability DisplayAvailability   `json:"Availability"`
}

// DisplayAnnotations batch-reads advisory labels for complete live run identifiers.
type DisplayAnnotations interface {
	ReadDisplay(ctx context.Context, root string, runIDs []string) (map[string]DisplayAnnotation, error)
}
