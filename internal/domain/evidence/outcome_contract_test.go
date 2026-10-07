package evidence_test

import (
	"encoding/json"
	"testing"

	"github.com/wahidyankf/hippo/internal/domain/evidence"
)

func TestOutcomeContractInvalidMembersRemainDiagnosticValues(t *testing.T) {
	for _, test := range []struct {
		outcome evidence.Outcome
		want    string
	}{{evidence.OutcomeUnset, "Outcome(0)"}, {evidence.Outcome(255), "Outcome(255)"}} {
		if actual := test.outcome.String(); actual != test.want {
			t.Fatalf("invalid outcome display=%q want=%q", actual, test.want)
		}
		if _, err := json.Marshal(test.outcome); err == nil {
			t.Fatalf("diagnostic-only outcome serialized: %s", test.want)
		}
	}
	for _, test := range []struct {
		outcome evidence.BudgetOutcome
		want    string
	}{{evidence.BudgetOutcomeUnset, "BudgetOutcome(0)"}, {evidence.BudgetOutcome(255), "BudgetOutcome(255)"}} {
		if actual := test.outcome.String(); actual != test.want {
			t.Fatalf("invalid budget display=%q want=%q", actual, test.want)
		}
		if _, err := json.Marshal(test.outcome); err == nil {
			t.Fatalf("diagnostic-only budget serialized: %s", test.want)
		}
	}
}

func TestOutcomeContractRecordedBudgetKeepsKnownUnknownAndClearedStates(t *testing.T) {
	for _, test := range []struct {
		text string
		want evidence.BudgetOutcome
	}{{"admitted", evidence.BudgetOutcomeAdmitted}, {"future-budget", evidence.BudgetOutcomeUnknown}, {"", evidence.BudgetOutcomeUnset}} {
		t.Run(test.text, func(t *testing.T) {
			recorded := evidence.RecordedBudget(evidence.BudgetOutcomeAdmitted)
			if err := recorded.UnmarshalText([]byte(test.text)); err != nil {
				t.Fatal(err)
			}
			if recorded.BudgetOutcome() != test.want || recorded.String() != test.text || recorded.IsZero() != (test.text == "") {
				t.Fatalf("recorded budget lost decoded member/text: member=%d text=%q zero=%t", recorded.BudgetOutcome(), recorded.String(), recorded.IsZero())
			}
			text, err := recorded.MarshalText()
			if err != nil || string(text) != test.text {
				t.Fatalf("recorded budget wire=%q error=%v", text, err)
			}
			row := evidence.Summary{SchemaVersion: 5, BudgetOutcome: recorded}
			encoded, err := json.Marshal(row)
			want := `{"schemaVersion":5}`
			if test.text != "" {
				want = `{"schemaVersion":5,"budgetOutcome":"` + test.text + `"}`
			}
			if err != nil || string(encoded) != want {
				t.Fatalf("budget summary=%s want=%s error=%v", encoded, want, err)
			}
		})
	}
}
