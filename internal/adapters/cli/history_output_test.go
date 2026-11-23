package cli //nolint:testpackage // Output refusal and encoding failures are injected through the application's own writers and evidence port.

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domainevidence "github.com/wahidyankf/hippo/internal/domain/evidence"
)

// historyRows is an evidence history whose rows are the ones a test supplies.
type historyRows []domainevidence.Summary

func (rows historyRows) History(string, domainevidence.Query) ([]domainevidence.Summary, error) {
	return rows, nil
}

func writeOneHistoryRow(t *testing.T, root string, now time.Time) {
	t.Helper()
	document := `{"schemaVersion":5,"runId":"known","source":"hippo","taskClass":"ephemeral",` +
		`"resourceTier":"standard","outcome":"passed","finishedAt":"` + now.Format(time.RFC3339Nano) + `"}` + "\n"
	if err := os.WriteFile(filepath.Join(root, "known.summary.json"), []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryTextReportsAWriterThatRefusesARow(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	writeOneHistoryRow(t, root, now)
	stderr := &bytes.Buffer{}
	application := wiredApplication(Application{
		Stdout: &refusingWriter{}, Stderr: stderr, Environment: []string{"HIPPO_ROOT=" + root},
		Now: func() time.Time { return now.Add(time.Hour) },
	})
	code, _ := application.Run(context.Background(), []string{"history", "--since", "30d"})
	if code == 0 || !strings.Contains(stderr.String(), "stdout refused the write") {
		t.Fatalf("code=%d stderr=%q, want the refused row reported", code, stderr)
	}
}

func TestHistoryJSONLinesReportsAWriterThatRefusesARow(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	writeOneHistoryRow(t, root, now)
	stderr := &bytes.Buffer{}
	application := wiredApplication(Application{
		Stdout: &refusingWriter{}, Stderr: stderr, Environment: []string{"HIPPO_ROOT=" + root},
		Now: func() time.Time { return now.Add(time.Hour) },
	})
	code, _ := application.Run(context.Background(), []string{"history", "--since", "30d", "--jsonl"})
	if code == 0 || !strings.Contains(stderr.String(), "stdout refused the write") {
		t.Fatalf("code=%d stderr=%q, want the refused JSON line reported", code, stderr)
	}
}

func TestHistoryReportsARowThatCannotBeEncodedInEachJSONForm(t *testing.T) {
	// A row whose p95 CPU reading is not a number has no JSON form. Both the
	// document and the line form must report it rather than print a partial row.
	t.Parallel()

	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	for name, arguments := range map[string][]string{
		"document": {"history", "--since", "30d", "--json"},
		"lines":    {"history", "--since", "30d", "--jsonl"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
			application := wiredApplication(Application{
				Stdout: stdout, Stderr: stderr, Environment: []string{"HIPPO_ROOT=" + t.TempDir()},
				Now: func() time.Time { return now.Add(time.Hour) },
			})
			application.Observer.Evidence = historyRows{{
				SchemaVersion: 5, RunID: "unencodable", Source: "hippo", FinishedAt: now.Format(time.RFC3339Nano),
				CPUUtilizationP95Percent: math.NaN(),
			}}
			code, _ := application.Run(context.Background(), arguments)
			if code == 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "unsupported value") {
				t.Fatalf("code=%d stdout=%q stderr=%q, want an encoding failure and no partial output", code, stdout, stderr)
			}
		})
	}
}
