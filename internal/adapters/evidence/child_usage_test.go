package evidence //nolint:testpackage // Compaction fixtures exercise private atomic archive boundaries.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	domain "github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
)

// TestCompactionKeepsChildUsage stages a passed summary that finished before
// today, runs the same retention pass a guarded run runs at finalize, and reads
// the history back. The compacted archive must still carry both child usage
// scalars.
func TestCompactionKeepsChildUsage(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	cpu, wall := 0.9, 1.2
	encoded, err := json.Marshal(domain.Summary{
		SchemaVersion: 5, RunID: "compacted", Source: "hippo", TaskClass: policy.RecordedClass(policy.TaskEphemeral),
		Outcome: domain.Recorded(domain.OutcomePassed), FinishedAt: now.Add(-36 * time.Hour).Format(time.RFC3339Nano),
		ChildCPUSeconds: &cpu, ChildWallSeconds: &wall,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "compacted.summary.json"), append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = Cleanup(root, now); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "compacted.summary.json")); !os.IsNotExist(statErr) {
		t.Fatalf("the prior-day summary was not compacted: %v", statErr)
	}
	rows, err := ReadHistory(root, Query{Since: domain.HistoryRetention, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.RunID != "compacted" {
			continue
		}
		if row.ChildCPUSeconds == nil || *row.ChildCPUSeconds != 0.9 || row.ChildWallSeconds == nil || *row.ChildWallSeconds != 1.2 {
			t.Fatalf("compaction dropped child usage: %+v", row)
		}

		return
	}
	t.Fatalf("compacted row is missing from history: %+v", rows)
}
