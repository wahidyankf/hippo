package evidence_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/domain/evidence"
)

// wireOutcomes is every outcome a run's lifetime summary can record, with the
// word v0.8.4 writes for it, in the order Outcomes lists them.
var wireOutcomes = []struct {
	outcome evidence.Outcome
	wire    string
}{
	{evidence.OutcomePassed, "passed"},
	{evidence.OutcomeTaskFailed, "task-failed"},
	{evidence.OutcomeSupervisionFailed, "supervision-failed"},
	{evidence.OutcomePressureShed, "pressure-shed"},
	{evidence.OutcomeStorageShed, "storage-shed"},
	{evidence.OutcomeEmergencySafetyStop, "emergency-safety-stop"},
	{evidence.OutcomeCapacityDeferred, "capacity-deferred"},
	{evidence.OutcomeStorageBlocked, "storage-blocked"},
	{evidence.OutcomeAdmissionCancelled, "admission-cancelled"},
	{evidence.OutcomeAdmissionFailed, "admission-failed"},
}

func TestOutcomeKeepsItsV084WireString(t *testing.T) {
	t.Parallel()

	for _, row := range wireOutcomes {
		encoded, err := row.outcome.MarshalText()
		if err != nil || string(encoded) != row.wire {
			t.Errorf("%d encodes as %q (%v), want %q", row.outcome, encoded, err, row.wire)
		}
		if row.outcome.String() != row.wire {
			t.Errorf("%d reads %q, want %q", row.outcome, row.outcome.String(), row.wire)
		}
	}
}

func TestOutcomesListsTheTenWritableMembersInOrder(t *testing.T) {
	t.Parallel()

	listed := evidence.Outcomes()
	if len(listed) != len(wireOutcomes) {
		t.Fatalf("Outcomes lists %d members, want %d", len(listed), len(wireOutcomes))
	}
	for position, row := range wireOutcomes {
		if listed[position] != row.outcome {
			t.Errorf("Outcomes()[%d] = %q, want %q", position, listed[position], row.outcome)
		}
	}
}

// TestOutcomesListsEveryMemberBetweenUnsetAndUnknown walks the declared members
// themselves, not a table kept beside them, so a member added to the constants
// can be neither left out of Outcomes nor left without a wire word.
func TestOutcomesListsEveryMemberBetweenUnsetAndUnknown(t *testing.T) {
	t.Parallel()

	listed := evidence.Outcomes()
	members := 0
	for outcome := evidence.OutcomeUnset + 1; outcome < evidence.OutcomeUnknown; outcome++ {
		members++
		if _, err := outcome.MarshalText(); err != nil {
			t.Errorf("member %d has no wire word: %v", outcome, err)
		}
		if !slices.Contains(listed, outcome) {
			t.Errorf("member %q is missing from Outcomes()", outcome)
		}
	}
	if members == 0 {
		t.Fatal("no member lies between OutcomeUnset and OutcomeUnknown")
	}
	if len(listed) != members {
		t.Errorf("Outcomes() lists %d outcomes, want the %d members between OutcomeUnset and OutcomeUnknown",
			len(listed), members)
	}
	for position, outcome := range listed {
		if outcome <= evidence.OutcomeUnset || outcome >= evidence.OutcomeUnknown {
			t.Errorf("Outcomes()[%d] = %d lies outside the members between OutcomeUnset and OutcomeUnknown",
				position, outcome)
		}
		if slices.Index(listed, outcome) != position {
			t.Errorf("Outcomes()[%d] = %q is listed twice", position, outcome)
		}
	}
}

func TestOutcomeEncoderRefusesWhatNoRunWrites(t *testing.T) {
	t.Parallel()

	// The unset zero value is a run that never decided how it ended, unknown is
	// a word only a reader produces, and the third is no member at all.
	for _, outcome := range []evidence.Outcome{evidence.OutcomeUnset, evidence.OutcomeUnknown, evidence.Outcome(200)} {
		if encoded, err := outcome.MarshalText(); err == nil {
			t.Errorf("outcome %d encoded as %q, want a refusal", outcome, encoded)
		}
	}
	if _, err := json.Marshal(struct {
		Outcome evidence.Outcome `json:"outcome"`
	}{}); err == nil {
		t.Error("a summary whose outcome is unset was encoded")
	}
}

func TestParseOutcomeIsStrict(t *testing.T) {
	t.Parallel()

	for _, row := range wireOutcomes {
		if parsed, err := evidence.ParseOutcome(row.wire); err != nil || parsed != row.outcome {
			t.Errorf("ParseOutcome(%q) = %d (%v), want %d", row.wire, parsed, err, row.outcome)
		}
	}
	for _, text := range []string{"", "future-outcome", "Passed", " passed", "unknown", "unset"} {
		parsed, err := evidence.ParseOutcome(text)
		if err == nil || parsed != evidence.OutcomeUnset || !strings.Contains(err.Error(), text) {
			t.Errorf("ParseOutcome(%q) = %d (%v), want the unset outcome and an error naming it", text, parsed, err)
		}
	}
}

func TestOutcomeDecodesStrictly(t *testing.T) {
	t.Parallel()

	var summary struct {
		Outcome evidence.Outcome `json:"outcome"`
	}
	if err := json.Unmarshal([]byte(`{"outcome":"storage-blocked"}`), &summary); err != nil ||
		summary.Outcome != evidence.OutcomeStorageBlocked {
		t.Errorf("decoded %d (%v), want the storage-blocked outcome", summary.Outcome, err)
	}
	if err := json.Unmarshal([]byte(`{"outcome":"future-outcome"}`), &summary); err == nil {
		t.Error("an outcome this version does not know decoded without a refusal")
	}
}

func TestRecordedOutcomeKeepsWhatWasRecorded(t *testing.T) {
	t.Parallel()

	type row struct {
		Outcome evidence.RecordedOutcome `json:"outcome,omitzero"`
	}
	for _, text := range []string{"passed", "capacity-deferred", "future-outcome", "Passed", "unknown"} {
		var decoded row
		if err := json.Unmarshal([]byte(`{"outcome":"`+text+`"}`), &decoded); err != nil {
			t.Fatalf("%q did not decode: %v", text, err)
		}
		if decoded.Outcome.String() != text {
			t.Errorf("%q was recorded as %q", text, decoded.Outcome.String())
		}
		encoded, err := json.Marshal(decoded)
		if err != nil || string(encoded) != `{"outcome":"`+text+`"}` {
			t.Errorf("%q round trips as %s (%v)", text, encoded, err)
		}
	}
}

func TestRecordedOutcomeNamesTheMemberItKnows(t *testing.T) {
	t.Parallel()

	known := []string{}
	for _, text := range []string{"passed", "task-failed", "future-outcome", "Passed", "unknown", "unset"} {
		var decoded evidence.RecordedOutcome
		if err := decoded.UnmarshalText([]byte(text)); err != nil {
			t.Fatalf("%q did not decode: %v", text, err)
		}
		if decoded.Outcome() != evidence.OutcomeUnknown {
			known = append(known, text)
		}
	}
	if !slices.Equal(known, []string{"passed", "task-failed"}) {
		t.Errorf("a known member was claimed for %v, want only passed and task-failed", known)
	}
	var future evidence.RecordedOutcome
	if err := future.UnmarshalText([]byte("future-outcome")); err != nil || future.Outcome() != evidence.OutcomeUnknown {
		t.Errorf("an outcome this version does not know read as %d (%v), want the unknown member", future.Outcome(), err)
	}
	for _, row := range wireOutcomes {
		recorded := evidence.Recorded(row.outcome)
		if recorded.Outcome() != row.outcome || recorded.String() != row.wire {
			t.Errorf("Recorded(%d) names %d and %q, want %d and %q", row.outcome, recorded.Outcome(), recorded.String(), row.outcome, row.wire)
		}
	}
}

func TestRecordedOutcomeIsOmittedOnlyWhenNothingWasRecorded(t *testing.T) {
	t.Parallel()

	type row struct {
		Outcome evidence.RecordedOutcome `json:"outcome,omitzero"`
	}
	var nothing evidence.RecordedOutcome
	if !nothing.IsZero() || !evidence.Recorded(evidence.OutcomeUnset).IsZero() || !evidence.Recorded(evidence.OutcomeUnknown).IsZero() {
		t.Error("an empty recording is not reported as zero")
	}
	if encoded, err := json.Marshal(row{}); err != nil || string(encoded) != `{}` {
		t.Errorf("an empty recording encodes as %s (%v), want {}", encoded, err)
	}
	var decoded row
	if err := json.Unmarshal([]byte(`{"outcome":""}`), &decoded); err != nil || !decoded.Outcome.IsZero() {
		t.Errorf("an empty recorded word decoded as %q (%v), want nothing recorded", decoded.Outcome.String(), err)
	}
	if evidence.Recorded(evidence.OutcomePassed).IsZero() {
		t.Error("a recorded outcome is reported as empty")
	}
}

func TestBudgetOutcomeKeepsItsV084WireString(t *testing.T) {
	t.Parallel()

	encoded, err := evidence.BudgetOutcomeAdmitted.MarshalText()
	if err != nil || string(encoded) != "admitted" {
		t.Errorf("the admitted budget outcome encodes as %q (%v), want admitted", encoded, err)
	}
	for _, outcome := range []evidence.BudgetOutcome{evidence.BudgetOutcomeUnset, evidence.BudgetOutcome(200)} {
		if encoded, err = outcome.MarshalText(); err == nil {
			t.Errorf("budget outcome %d encoded as %q, want a refusal", outcome, encoded)
		}
	}
	type summary struct {
		Budget evidence.BudgetOutcome `json:"budgetOutcome,omitempty"`
	}
	if encoded, err = json.Marshal(summary{}); err != nil || string(encoded) != `{}` {
		t.Errorf("an unset budget outcome encodes as %s (%v), want it omitted", encoded, err)
	}
	var decoded summary
	if err = json.Unmarshal([]byte(`{"budgetOutcome":"admitted"}`), &decoded); err != nil ||
		decoded.Budget != evidence.BudgetOutcomeAdmitted {
		t.Errorf("the admitted budget outcome decoded as %d (%v)", decoded.Budget, err)
	}
	if err = json.Unmarshal([]byte(`{"budgetOutcome":"pressure-shed"}`), &decoded); err == nil {
		t.Error("a budget outcome no run writes decoded without a refusal")
	}
}

func TestRecordedBudgetOutcomeKeepsWhatWasRecorded(t *testing.T) {
	t.Parallel()

	type row struct {
		Budget evidence.RecordedBudgetOutcome `json:"budgetOutcome,omitzero"`
	}
	for _, text := range []string{"admitted", "future-budget"} {
		var decoded row
		if err := json.Unmarshal([]byte(`{"budgetOutcome":"`+text+`"}`), &decoded); err != nil {
			t.Fatalf("%q did not decode: %v", text, err)
		}
		if decoded.Budget.String() != text {
			t.Errorf("%q was recorded as %q", text, decoded.Budget.String())
		}
		encoded, err := json.Marshal(decoded)
		if err != nil || string(encoded) != `{"budgetOutcome":"`+text+`"}` {
			t.Errorf("%q round trips as %s (%v)", text, encoded, err)
		}
	}
	if encoded, err := json.Marshal(row{}); err != nil || string(encoded) != `{}` {
		t.Errorf("an empty recording encodes as %s (%v), want {}", encoded, err)
	}
	if evidence.RecordedBudget(evidence.BudgetOutcomeAdmitted).String() != "admitted" ||
		!evidence.RecordedBudget(evidence.BudgetOutcomeUnset).IsZero() {
		t.Error("RecordedBudget does not name the member it was given")
	}
}
