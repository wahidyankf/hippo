package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/status"
)

func (boundary Application) history(options historyOptions) (int, error) {
	if options.jsonOutput && options.jsonLines {
		return 0, status.Fail(status.CodeArgsInvalid, "--json and --jsonl are mutually exclusive")
	}
	result, emptyResult, err := boundary.Observer.History(application.HistoryFlags{Since: options.since, Source: options.source, Tags: options.tags, ClassFlag: options.classFlag, Tier: options.resourceTier, OutcomeFlag: options.outcomeFlag})
	if err != nil {
		return emptyResult, err
	}
	rows := result.Rows
	if options.jsonOutput {
		encoded, encodeError := json.Marshal(struct {
			SchemaVersion int                `json:"schemaVersion"`
			Since         string             `json:"since"`
			Rows          []evidence.Summary `json:"rows"`
		}{SchemaVersion: 1, Since: options.since, Rows: rows})
		if encodeError != nil {
			return 1, encodeError
		}
		_, err = fmt.Fprintln(boundary.Stdout, string(encoded))

		return emptyResult, err
	}
	for _, row := range rows {
		if options.jsonLines {
			encoded, encodeError := json.Marshal(row)
			if encodeError != nil {
				return 1, encodeError
			}
			if _, err = fmt.Fprintln(boundary.Stdout, string(encoded)); err != nil {
				return 1, err
			}

			continue
		}
		if _, err = fmt.Fprintf(
			boundary.Stdout,
			"finished=%s run=%s source=%s class=%s tier=%s outcome=%s count=%d\n",
			row.FinishedAt, row.RunID, row.Source, row.TaskClass, row.ResourceTier, row.Outcome, max(row.AggregateCount, 1),
		); err != nil {
			return 1, err
		}
	}

	return emptyResult, nil
}

func (boundary Application) watch(ctx context.Context, options watchOptions) (int, error) {
	return boundary.Observer.Watch(ctx, statusRequest(options.statusOptions), options.interval, func(view application.StatusView) ([]byte, error) {
		output := &bytes.Buffer{}
		_, err := renderStatus(output, options.jsonOutput, view)
		return output.Bytes(), err
	}, func(value []byte) error { _, err := boundary.Stdout.Write(value); return err })
}
