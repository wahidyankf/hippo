package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/evidence"
	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/host"
	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

func parseRollingDuration(value string) (time.Duration, error) {
	if days, found := strings.CutSuffix(value, "d"); found {
		parsed, err := time.ParseDuration(days + "h")
		if err != nil {
			return 0, err
		}

		return parsed * 24, nil
	}

	return time.ParseDuration(value)
}

// historyTaskClasses are the classes a recorded run can carry. release is
// among them because runs recorded before run refused it stay in history for
// the whole retention window.
var historyTaskClasses = []string{
	string(policy.TaskEphemeral), string(policy.TaskService), string(policy.TaskTransactional), string(policy.TaskRelease),
}

// historyFilterMistake refuses a filter value no recorded run can carry. An
// empty answer to it would read as "nothing matched" when the question could
// never have matched anything, which hides a typo behind a valid result.
func historyFilterMistake(options historyOptions) error {
	if err := identity.ValidateOverrides(options.source, nil); err != nil {
		return status.Fail(status.CodeArgsInvalid, "--source: %v", err)
	}
	if options.taskClass != "" && !slices.Contains(historyTaskClasses, options.taskClass) {
		return status.Fail(status.CodeArgsInvalid, "--class must be one of %s", strings.Join(historyTaskClasses, ", "))
	}
	if _, known := guard.DefaultResourceTiers()[options.resourceTier]; options.resourceTier != "" && !known {
		return status.Fail(status.CodeArgsInvalid, "--resource-tier must be light, standard, or heavy")
	}
	if outcomes := guard.RunOutcomes(); options.outcome != "" && !slices.Contains(outcomes, options.outcome) {
		return status.Fail(status.CodeArgsInvalid, "--outcome must be one of %s", strings.Join(outcomes, ", "))
	}

	return nil
}

func (application Application) history(options historyOptions) (int, error) {
	if options.jsonOutput && options.jsonLines {
		return 0, status.Fail(status.CodeArgsInvalid, "--json and --jsonl are mutually exclusive")
	}
	if err := historyFilterMistake(options); err != nil {
		return 0, err
	}
	since, err := parseRollingDuration(options.since)
	if err != nil || since <= 0 || since > evidence.HistoryRetention {
		return 0, status.Fail(status.CodeArgsInvalid, "since must be positive and no more than 30d")
	}
	tags, err := identity.ParseTags(options.tags)
	if err != nil {
		return 0, status.Fail(status.CodeArgsInvalid, "%v", err)
	}
	root := host.DefaultEvidenceRoot(environmentMap(application.Environment))
	rows, err := evidence.ReadHistory(root, evidence.Query{
		Since: since, Now: application.Now(), Source: options.source, Tags: tags,
		Class: options.taskClass, Tier: options.resourceTier, Outcome: options.outcome,
	})
	if err != nil {
		return 0, status.Fail(status.CodeEvidenceUnreadable, "reading run history: %v", err)
	}
	// An empty history is an answer, and `1` is how this contract says a run
	// completed with nothing to report. A caller scripting against it can tell
	// "no runs matched" from "hippo could not look" without reading a word.
	emptyResult := 0
	if len(rows) == 0 {
		emptyResult = 1
	}
	if options.jsonOutput {
		encoded, encodeError := json.Marshal(struct {
			SchemaVersion int                `json:"schemaVersion"`
			Since         string             `json:"since"`
			Rows          []evidence.Summary `json:"rows"`
		}{SchemaVersion: 1, Since: options.since, Rows: rows})
		if encodeError != nil {
			return 1, encodeError
		}
		_, err = fmt.Fprintln(application.Stdout, string(encoded))

		return emptyResult, err
	}
	for _, row := range rows {
		if options.jsonLines {
			encoded, encodeError := json.Marshal(row)
			if encodeError != nil {
				return 1, encodeError
			}
			if _, err = fmt.Fprintln(application.Stdout, string(encoded)); err != nil {
				return 1, err
			}

			continue
		}
		if _, err = fmt.Fprintf(
			application.Stdout,
			"finished=%s run=%s source=%s class=%s tier=%s outcome=%s count=%d\n",
			row.FinishedAt, row.RunID, row.Source, row.TaskClass, row.ResourceTier, row.Outcome, max(row.AggregateCount, 1),
		); err != nil {
			return 1, err
		}
	}

	return emptyResult, nil
}

func (application Application) watch(ctx context.Context, options watchOptions) (int, error) {
	if options.interval <= 0 {
		return 0, status.Fail(status.CodeArgsInvalid, "interval must be positive")
	}
	prior := ""
	for {
		output := &bytes.Buffer{}
		view := application
		view.Stdout = output
		code, err := view.status(ctx, options.statusOptions)
		if stoppedByCaller(ctx, err) {
			return 0, nil
		}
		if err != nil || code != 0 {
			return code, err
		}
		if output.String() != prior {
			if _, err = application.Stdout.Write(output.Bytes()); err != nil {
				return 1, err
			}
			prior = output.String()
		}
		if err = waitForContext(ctx, options.interval, application.Sleep); err != nil {
			if stoppedByCaller(ctx, err) {
				return 0, nil
			}

			return 1, err
		}
	}
}

// stoppedByCaller reports whether an observer loop failed only because its
// caller cancelled it. watch and monitor run until they are stopped, so a
// stop is their end wherever it lands: while waiting, or while a sample is
// being collected. The entry point turns a signal-caused stop into 128+N; a
// failure hippo classified while stopping is still its own answer.
func stoppedByCaller(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() == nil {
		return false
	}
	_, classified := errors.AsType[status.Failure](err)

	return !classified
}
