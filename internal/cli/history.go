package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/evidence"
	"github.com/wahidyankf/hippo/internal/host"
	"github.com/wahidyankf/hippo/internal/identity"
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

func (application Application) history(options historyOptions) (int, error) {
	if options.jsonOutput && options.jsonLines {
		return 1, errors.New("--json and --jsonl are mutually exclusive")
	}
	since, err := parseRollingDuration(options.since)
	if err != nil || since <= 0 || since > evidence.HistoryRetention {
		return 1, errors.New("since must be positive and no more than 30d")
	}
	tags, err := identity.ParseTags(options.tags)
	if err != nil {
		return 1, err
	}
	root := host.DefaultEvidenceRoot(environmentMap(application.Environment))
	rows, err := evidence.ReadHistory(root, evidence.Query{
		Since: since, Now: application.Now(), Source: options.source, Tags: tags,
		Class: options.taskClass, Tier: options.resourceTier, Outcome: options.outcome,
	})
	if err != nil {
		return 1, err
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

		return 0, err
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

	return 0, nil
}

func (application Application) watch(ctx context.Context, options watchOptions) (int, error) {
	if options.interval <= 0 {
		return 1, errors.New("interval must be positive")
	}
	prior := ""
	for {
		output := &bytes.Buffer{}
		view := application
		view.Stdout = output
		code, err := view.status(ctx, options.statusOptions)
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
			if ctx.Err() != nil {
				//nolint:nilerr // Cancellation is the successful termination contract for watch.
				return 0, nil
			}

			return 1, err
		}
	}
}
