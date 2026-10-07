package application

import (
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

// HistoryFlags carries invocation text until it is parsed into an evidence query.
type HistoryFlags struct {
	Since, Source, Tier, ClassFlag, OutcomeFlag string
	Tags                                        []string
}

// HistoryResult holds rows and the requested rolling interval.
type HistoryResult struct {
	Since string
	Rows  []evidence.Summary
}

// ParseRollingDuration reads a duration with optional whole-day suffix.
func ParseRollingDuration(value string) (time.Duration, error) {
	if days, found := strings.CutSuffix(value, "d"); found {
		parsed, err := time.ParseDuration(days + "h")
		if err != nil {
			return 0, err
		}

		return parsed * 24, nil
	}

	return time.ParseDuration(value)
}

// classNames lists every class a recorded run can carry, as the --class filter
// names them. release is among them because runs recorded before run refused it
// stay in history for the whole retention window.
func classNames() []string {
	classes := policy.TaskClasses()
	names := make([]string, 0, len(classes))
	for _, class := range classes {
		names = append(names, string(class))
	}

	return names
}

// outcomeNames lists every outcome a run's lifetime summary can record, as the
// --outcome filter names them. history uses it to tell a filter no run can
// match from a question with an empty answer.
func outcomeNames() []string {
	outcomes := evidence.Outcomes()
	names := make([]string, 0, len(outcomes))
	for _, outcome := range outcomes {
		names = append(names, outcome.String())
	}

	return names
}

// historyFilters are the filters history takes that name a closed set, read from
// their flags.
type historyFilters struct {
	class   policy.TaskClass
	outcome evidence.Outcome
}

// readHistoryFilters refuses a filter value no recorded run can carry, and reads
// the rest: --class as the class it names, or the empty class for no filter, and
// --outcome as the outcome it names, or OutcomeUnset for no filter. An empty
// answer to a refused value would read as "nothing matched" when the question
// could never have matched anything, which hides a typo behind a valid result.
// The checks run in the order the flags are documented: source, class, resource
// tier, outcome.
func readHistoryFilters(request HistoryFlags) (historyFilters, error) {
	if err := identity.ValidateOverrides(request.Source, nil); err != nil {
		return historyFilters{}, status.Fail(status.CodeArgsInvalid, "--source: %v", err)
	}
	class, err := classFilter(request.ClassFlag)
	if err != nil {
		return historyFilters{}, err
	}
	if _, known := coordination.DefaultResourceTiers()[request.Tier]; request.Tier != "" && !known {
		return historyFilters{}, status.Fail(status.CodeArgsInvalid, "--resource-tier must be light, standard, or heavy")
	}
	outcome, err := outcomeFilter(request.OutcomeFlag)
	if err != nil {
		return historyFilters{}, err
	}

	return historyFilters{class: class, outcome: outcome}, nil
}

// classFilter reads the --class flag: the class it names, or the empty class for
// no filter. A class no recorded run can carry is a usage mistake, for the
// reason readHistoryFilters gives.
func classFilter(flag string) (policy.TaskClass, error) {
	if flag == "" {
		return "", nil
	}
	class, err := policy.ParseTaskClass(flag)
	if err != nil {
		return "", status.Fail(status.CodeArgsInvalid, "--class must be one of %s", strings.Join(classNames(), ", "))
	}

	return class, nil
}

// outcomeFilter reads the --outcome flag: the outcome it names, or
// OutcomeUnset for no filter. A word no run records is a usage mistake, for the
// reason readHistoryFilters gives.
func outcomeFilter(flag string) (evidence.Outcome, error) {
	if flag == "" {
		return evidence.OutcomeUnset, nil
	}
	outcome, err := evidence.ParseOutcome(flag)
	if err != nil {
		return evidence.OutcomeUnset, status.Fail(
			status.CodeArgsInvalid, "--outcome must be one of %s", strings.Join(outcomeNames(), ", "),
		)
	}

	return outcome, nil
}

// prepareHistoryQuery turns invocation text into the domain's closed-set query before effects begin.
func prepareHistoryQuery(request HistoryFlags) (evidence.Query, error) {
	filters, err := readHistoryFilters(request)
	if err != nil {
		return evidence.Query{}, err
	}
	since, err := ParseRollingDuration(request.Since)
	if err != nil || since <= 0 || since > evidence.HistoryRetention {
		return evidence.Query{}, status.Fail(status.CodeArgsInvalid, "since must be positive and no more than 30d")
	}
	tags, err := identity.ParseTags(request.Tags)
	if err != nil {
		return evidence.Query{}, status.Fail(status.CodeArgsInvalid, "%v", err)
	}
	return evidence.Query{
		Since: since, Source: request.Source, Tags: tags,
		Class: filters.class, Tier: request.Tier, Outcome: filters.outcome,
	}, nil
}

// History prepares a typed query and reads its matching retained rows.
func (services ObservationServices) History(request HistoryFlags) (HistoryResult, int, error) {
	query, err := prepareHistoryQuery(request)
	if err != nil {
		return HistoryResult{}, 0, err
	}
	root := services.Configuration.StateRoot(services.Environment)
	query.Now = services.Clock.Now()
	rows, err := services.Evidence.History(root, query)
	if err != nil {
		return HistoryResult{}, 0, status.Fail(status.CodeEvidenceUnreadable, "reading run history: %v", err)
	}
	// An empty history is an answer, and `1` is how this contract says a run
	// completed with nothing to report. A caller scripting against it can tell
	// "no runs matched" from "hippo could not look" without reading a word.
	emptyResult := 0
	if len(rows) == 0 {
		emptyResult = 1
	}

	return HistoryResult{Since: request.Since, Rows: rows}, emptyResult, nil
}
