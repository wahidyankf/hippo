package evidence

import domain "github.com/wahidyankf/hippo/internal/domain/evidence"

// Histogram is the evidence-domain value used by this adapter.
type Histogram = domain.Histogram

// Outcome is the evidence-domain value used by this adapter.
type Outcome = domain.Outcome

// RecordedOutcome is the evidence-domain value used by this adapter.
type RecordedOutcome = domain.RecordedOutcome

// BudgetOutcome is the evidence-domain value used by this adapter.
type BudgetOutcome = domain.BudgetOutcome

// RecordedBudgetOutcome is the evidence-domain value used by this adapter.
type RecordedBudgetOutcome = domain.RecordedBudgetOutcome

// Domain constants preserve the adapter boundary wire vocabulary.
const (
	RawRetention               = domain.RawRetention
	HistoryRetention           = domain.HistoryRetention
	RawMaximumBytes            = domain.RawMaximumBytes
	HistoryMaxBytes            = domain.HistoryMaxBytes
	OutcomeUnset               = domain.OutcomeUnset
	OutcomePassed              = domain.OutcomePassed
	OutcomeTaskFailed          = domain.OutcomeTaskFailed
	OutcomeSupervisionFailed   = domain.OutcomeSupervisionFailed
	OutcomePressureShed        = domain.OutcomePressureShed
	OutcomeStorageShed         = domain.OutcomeStorageShed
	OutcomeEmergencySafetyStop = domain.OutcomeEmergencySafetyStop
	OutcomeCapacityDeferred    = domain.OutcomeCapacityDeferred
	OutcomeStorageBlocked      = domain.OutcomeStorageBlocked
	OutcomeAdmissionCancelled  = domain.OutcomeAdmissionCancelled
	OutcomeAdmissionFailed     = domain.OutcomeAdmissionFailed
	OutcomeUnknown             = domain.OutcomeUnknown
	BudgetOutcomeUnset         = domain.BudgetOutcomeUnset
	BudgetOutcomeAdmitted      = domain.BudgetOutcomeAdmitted
	BudgetOutcomeUnknown       = domain.BudgetOutcomeUnknown
)

// NewHistogram constructs a pure bounded histogram.
func NewHistogram(maximum, resolution float64) *Histogram {
	return domain.NewHistogram(maximum, resolution)
}

// Outcomes returns the supported completed outcomes.
func Outcomes() []Outcome { return domain.Outcomes() }

// ParseOutcome validates a recorded outcome name.
func ParseOutcome(text string) (Outcome, error) { return domain.ParseOutcome(text) }

// Recorded preserves a known outcome in retained evidence.
func Recorded(outcome Outcome) RecordedOutcome { return domain.Recorded(outcome) }

// RecordedBudget preserves a known budget outcome.
func RecordedBudget(outcome BudgetOutcome) RecordedBudgetOutcome {
	return domain.RecordedBudget(outcome)
}

// Summary is the portable evidence-domain value.
type Summary = domain.Summary

// Query is the portable evidence-domain value.
type Query = domain.Query

// PromotionCriteria is the portable evidence-domain value.
type PromotionCriteria = domain.PromotionCriteria

// PromotionEvaluation is the portable evidence-domain value.
type PromotionEvaluation = domain.PromotionEvaluation

// Limits bounds a portable evidence stream request.
type Limits = domain.Limits

// DefaultLimits returns the standard private evidence bounds.
func DefaultLimits() Limits { return domain.DefaultLimits() }
