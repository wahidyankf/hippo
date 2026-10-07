package application_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

type historyFilterCase struct {
	name    string
	class   policy.TaskClass
	outcome evidence.Outcome
}

func TestHistoryRequestPreparesTypedWildcardsAndEveryClosedSetMember(t *testing.T) {
	classes, outcomes := policy.TaskClasses(), evidence.Outcomes()
	cases := make([]historyFilterCase, 0, 1+len(classes)+len(outcomes))
	cases = append(cases, historyFilterCase{name: "no class or outcome filter", outcome: evidence.OutcomeUnset})
	for _, class := range classes {
		cases = append(cases, historyFilterCase{name: "class " + string(class), class: class, outcome: evidence.OutcomeUnset})
	}
	for _, outcome := range outcomes {
		cases = append(cases, historyFilterCase{name: "outcome " + outcome.String(), outcome: outcome})
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newObservationFixture()
			history := &observationContractHistory{}
			services := observationServices(fixture)
			services.Evidence = history
			outcomeFlag := ""
			if test.outcome != evidence.OutcomeUnset {
				outcomeFlag = test.outcome.String()
			}
			result, code, err := services.History(application.HistoryFlags{Since: "1h", ClassFlag: string(test.class), OutcomeFlag: outcomeFlag})
			if err != nil || code != 1 || result.Since != "1h" || len(result.Rows) != 0 {
				t.Fatalf("wildcard/member result=%+v code=%d error=%v", result, code, err)
			}
			if history.query.Class != test.class || history.query.Outcome != test.outcome || history.root != "shared" {
				t.Fatalf("typed query=%+v root=%q", history.query, history.root)
			}
		})
	}
}

func TestHistoryRequestValidationPrecedenceLeavesEffectsUntouched(t *testing.T) {
	cases := []struct {
		name    string
		request application.HistoryFlags
		prefix  string
	}{
		{"source before class", application.HistoryFlags{Source: "bad source", ClassFlag: "future", OutcomeFlag: "future", Since: "bad"}, "--source"},
		{"class before tier", application.HistoryFlags{ClassFlag: "future", Tier: "future", OutcomeFlag: "future", Since: "bad"}, "--class"},
		{"tier before outcome", application.HistoryFlags{Tier: "future", OutcomeFlag: "future", Since: "bad"}, "--resource-tier"},
		{"outcome before duration", application.HistoryFlags{OutcomeFlag: "future", Since: "bad"}, "--outcome"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newObservationFixture()
			_, code, err := observationServices(fixture).History(test.request)
			failure, classified := errors.AsType[status.Failure](err)
			if code != 0 || !classified || failure.Code != status.CodeArgsInvalid || !strings.HasPrefix(failure.Message, test.prefix) || len(fixture.calls) != 0 {
				t.Fatalf("code=%d error=%v effects=%v", code, err, fixture.calls)
			}
		})
	}
}
