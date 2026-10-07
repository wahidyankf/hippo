package evidence

import "fmt"

// Outcome is how a guarded run's lifetime ended, as its summary records it. The
// zero value is OutcomeUnset: a run that has not decided how it ended, which no
// summary may record.
type Outcome uint8

const (
	// OutcomeUnset is the zero value, a run whose outcome is not decided yet.
	OutcomeUnset Outcome = iota
	// OutcomePassed is a child that exited 0.
	OutcomePassed
	// OutcomeTaskFailed is a child that exited nonzero, was stopped by a signal to
	// HIPPO, or failed its activation.
	OutcomeTaskFailed
	// OutcomeSupervisionFailed is a running child HIPPO lost supervision of and
	// stopped.
	OutcomeSupervisionFailed
	// OutcomePressureShed is a child shed under host pressure other than storage.
	OutcomePressureShed
	// OutcomeStorageShed is a child shed because the disk floor was crossed.
	OutcomeStorageShed
	// OutcomeEmergencySafetyStop is transactional work stopped past the emergency
	// floor.
	OutcomeEmergencySafetyStop
	// OutcomeCapacityDeferred is a run whose safe host admission was not reached
	// before the admission deadline.
	OutcomeCapacityDeferred
	// OutcomeStorageBlocked is a run the disk floor refused before launch.
	OutcomeStorageBlocked
	// OutcomeAdmissionCancelled is a run a signal or other cancellation stopped
	// while it sampled the host.
	OutcomeAdmissionCancelled
	// OutcomeAdmissionFailed is a run HIPPO itself stopped after host sampling
	// began and before its child started.
	OutcomeAdmissionFailed
	// OutcomeUnknown is what a reader reports for a recorded word this version has
	// no member for. No writer produces it. It ends the members a run records: a
	// new member is declared before it, which is all Outcomes needs to list it.
	OutcomeUnknown
)

// Outcomes returns every outcome a run's lifetime summary can record, in the
// order the history filter lists them. It is every member declared between
// OutcomeUnset and OutcomeUnknown, so no list kept beside the constants can
// fall behind them.
func Outcomes() []Outcome {
	outcomes := make([]Outcome, 0, int(OutcomeUnknown)-int(OutcomeUnset)-1)
	for outcome := OutcomeUnset + 1; outcome < OutcomeUnknown; outcome++ {
		outcomes = append(outcomes, outcome)
	}

	return outcomes
}

// MarshalText writes the wire string a summary records for the outcome. It
// refuses OutcomeUnset and OutcomeUnknown, which no run records, and any value
// that is no member at all.
func (outcome Outcome) MarshalText() ([]byte, error) {
	switch outcome {
	case OutcomePassed:
		return []byte("passed"), nil
	case OutcomeTaskFailed:
		return []byte("task-failed"), nil
	case OutcomeSupervisionFailed:
		return []byte("supervision-failed"), nil
	case OutcomePressureShed:
		return []byte("pressure-shed"), nil
	case OutcomeStorageShed:
		return []byte("storage-shed"), nil
	case OutcomeEmergencySafetyStop:
		return []byte("emergency-safety-stop"), nil
	case OutcomeCapacityDeferred:
		return []byte("capacity-deferred"), nil
	case OutcomeStorageBlocked:
		return []byte("storage-blocked"), nil
	case OutcomeAdmissionCancelled:
		return []byte("admission-cancelled"), nil
	case OutcomeAdmissionFailed:
		return []byte("admission-failed"), nil
	case OutcomeUnset, OutcomeUnknown:
		// Neither is a word a run records; the refusal below says so.
	}

	return nil, fmt.Errorf("outcome %d is not one a run records", uint8(outcome))
}

// String is the wire string of a recordable outcome, and Outcome(n) for any
// other value.
func (outcome Outcome) String() string {
	text, err := outcome.MarshalText()
	if err != nil {
		return fmt.Sprintf("Outcome(%d)", uint8(outcome))
	}

	return string(text)
}

// ParseOutcome reads the wire string of one outcome a run records, and refuses
// every other text, including the empty one. It asks each outcome in Outcomes
// for its string, so MarshalText stays the only table of wire words.
func ParseOutcome(text string) (Outcome, error) {
	for _, outcome := range Outcomes() {
		if outcome.String() == text {
			return outcome, nil
		}
	}

	return OutcomeUnset, fmt.Errorf("unknown outcome %q", text)
}

// UnmarshalText decodes strictly: text that ParseOutcome refuses is refused. It
// is for input this version defines, never for evidence a run recorded, which
// may carry a word this version has no member for. A reader of recorded
// evidence must decode into RecordedOutcome, which keeps such a word, or one
// unfamiliar summary fails the whole read (D6).
func (outcome *Outcome) UnmarshalText(text []byte) error {
	parsed, err := ParseOutcome(string(text))
	if err != nil {
		return err
	}
	*outcome = parsed

	return nil
}

// RecordedOutcome is an outcome as a summary recorded it: the member this
// version knows it as, and the text that was recorded. A word this version has
// no member for reads as OutcomeUnknown with its text kept, so a reader lists
// what a newer or older version wrote instead of failing or guessing.
type RecordedOutcome struct {
	outcome Outcome
	text    string
}

// Recorded returns the recording of one outcome a run writes. OutcomeUnset,
// OutcomeUnknown, and a non-member record nothing.
func Recorded(outcome Outcome) RecordedOutcome {
	text, err := outcome.MarshalText()
	if err != nil {
		return RecordedOutcome{outcome: OutcomeUnset, text: ""}
	}

	return RecordedOutcome{outcome: outcome, text: string(text)}
}

// Outcome is the member the recorded text names, OutcomeUnknown for text this
// version has no member for, and OutcomeUnset when nothing was recorded.
func (recorded RecordedOutcome) Outcome() Outcome {
	return recorded.outcome
}

// String is the text that was recorded.
func (recorded RecordedOutcome) String() string {
	return recorded.text
}

// IsZero reports that nothing was recorded, so an omitzero tag leaves the field
// out as omitempty left out an empty string.
func (recorded RecordedOutcome) IsZero() bool {
	return recorded.text == ""
}

// MarshalText writes back the text that was recorded.
func (recorded RecordedOutcome) MarshalText() ([]byte, error) {
	return []byte(recorded.text), nil
}

// UnmarshalText never fails: a known word yields its member, any other word
// yields OutcomeUnknown, and either keeps the text.
func (recorded *RecordedOutcome) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*recorded = RecordedOutcome{outcome: OutcomeUnset, text: ""}

		return nil
	}
	member := OutcomeUnknown
	if parsed, err := ParseOutcome(string(text)); err == nil {
		member = parsed
	}
	*recorded = RecordedOutcome{outcome: member, text: string(text)}

	return nil
}

// BudgetOutcome is how a run's reservation budget was settled, as its summary
// records it. The zero value is BudgetOutcomeUnset, which a summary omits.
type BudgetOutcome uint8

const (
	// BudgetOutcomeUnset is the zero value, a run with no reservation budget.
	BudgetOutcomeUnset BudgetOutcome = iota
	// BudgetOutcomeAdmitted is a run the reservation budget admitted. It is the
	// only budget outcome any release of HIPPO has written.
	BudgetOutcomeAdmitted
	// BudgetOutcomeUnknown is what a reader reports for a recorded word this
	// version has no member for. No writer produces it.
	BudgetOutcomeUnknown
)

// MarshalText writes the wire string a summary records for the budget outcome.
// It refuses BudgetOutcomeUnset and BudgetOutcomeUnknown, which no run records,
// and any value that is no member at all.
func (budget BudgetOutcome) MarshalText() ([]byte, error) {
	switch budget {
	case BudgetOutcomeAdmitted:
		return []byte("admitted"), nil
	case BudgetOutcomeUnset, BudgetOutcomeUnknown:
		// Neither is a word a run records; the refusal below says so.
	}

	return nil, fmt.Errorf("budget outcome %d is not one a run records", uint8(budget))
}

// String is the wire string of a recordable budget outcome, and
// BudgetOutcome(n) for any other value.
func (budget BudgetOutcome) String() string {
	text, err := budget.MarshalText()
	if err != nil {
		return fmt.Sprintf("BudgetOutcome(%d)", uint8(budget))
	}

	return string(text)
}

// parseBudgetOutcome reads the wire string of the one budget outcome a run
// records, and refuses every other text.
func parseBudgetOutcome(text string) (BudgetOutcome, error) {
	if text == BudgetOutcomeAdmitted.String() {
		return BudgetOutcomeAdmitted, nil
	}

	return BudgetOutcomeUnset, fmt.Errorf("unknown budget outcome %q", text)
}

// UnmarshalText decodes strictly: any text but the one a run writes is refused.
// It is for input this version defines, never for evidence a run recorded,
// which may carry a word this version has no member for. A reader of recorded
// evidence must decode into RecordedBudgetOutcome, which keeps such a word, or
// one unfamiliar summary fails the whole read (D6).
func (budget *BudgetOutcome) UnmarshalText(text []byte) error {
	parsed, err := parseBudgetOutcome(string(text))
	if err != nil {
		return err
	}
	*budget = parsed

	return nil
}

// RecordedBudgetOutcome is a budget outcome as a summary recorded it, built as
// RecordedOutcome is: a word this version has no member for reads as
// BudgetOutcomeUnknown with its text kept.
type RecordedBudgetOutcome struct {
	budget BudgetOutcome
	text   string
}

// RecordedBudget returns the recording of one budget outcome a run writes.
// BudgetOutcomeUnset, BudgetOutcomeUnknown, and a non-member record nothing.
func RecordedBudget(budget BudgetOutcome) RecordedBudgetOutcome {
	text, err := budget.MarshalText()
	if err != nil {
		return RecordedBudgetOutcome{budget: BudgetOutcomeUnset, text: ""}
	}

	return RecordedBudgetOutcome{budget: budget, text: string(text)}
}

// BudgetOutcome is the member the recorded text names, BudgetOutcomeUnknown for
// text this version has no member for, and BudgetOutcomeUnset when nothing was
// recorded.
func (recorded RecordedBudgetOutcome) BudgetOutcome() BudgetOutcome {
	return recorded.budget
}

// String is the text that was recorded.
func (recorded RecordedBudgetOutcome) String() string {
	return recorded.text
}

// IsZero reports that nothing was recorded, so an omitzero tag leaves the field
// out as omitempty left out an empty string.
func (recorded RecordedBudgetOutcome) IsZero() bool {
	return recorded.text == ""
}

// MarshalText writes back the text that was recorded.
func (recorded RecordedBudgetOutcome) MarshalText() ([]byte, error) {
	return []byte(recorded.text), nil
}

// UnmarshalText never fails: a known word yields its member, any other word
// yields BudgetOutcomeUnknown, and either keeps the text.
func (recorded *RecordedBudgetOutcome) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*recorded = RecordedBudgetOutcome{budget: BudgetOutcomeUnset, text: ""}

		return nil
	}
	member := BudgetOutcomeUnknown
	if parsed, err := parseBudgetOutcome(string(text)); err == nil {
		member = parsed
	}
	*recorded = RecordedBudgetOutcome{budget: member, text: string(text)}

	return nil
}
