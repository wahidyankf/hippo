package runtime

import (
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/evidence"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	domain "github.com/wahidyankf/hippo/internal/domain/evidence"
)

// EvidenceSummary is the portable completed run summary.
type EvidenceSummary = domain.RunSummary

// EvidenceWriter translates opaque runtime ownership into portable evidence metadata.
type EvidenceWriter struct{ *evidence.RunWriter }

// NewEvidenceWriter opens one adapter-owned evidence lifetime.
func NewEvidenceWriter(root, identifier string, limits evidence.Limits) (*EvidenceWriter, error) {
	writer, err := evidence.NewRunWriter(root, identifier, limits)
	if err != nil {
		return nil, err
	}
	return &EvidenceWriter{writer}, nil
}

// SetReservationContext passes allocation values without exposing runtime identity.
func (writer *EvidenceWriter) SetReservationContext(session *Session, peak int, outcome evidence.BudgetOutcome) {
	if writer == nil || session == nil {
		return
	}
	writer.RunWriter.SetReservationContext(&coordination.LeaseMetadata{Requested: session.Requested, Allocation: session.Allocation, WaitDuration: session.WaitDuration}, peak, outcome)
}

// EvidenceIdentifier returns a private filesystem-safe run name.
func EvidenceIdentifier(prefix string, now time.Time, pid int) string {
	return evidence.Identifier(prefix, now, pid)
}
