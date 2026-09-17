package evidence //nolint:testpackage // Compaction fixtures exercise private atomic archive boundaries.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeHistoryFixture(t *testing.T, root, name string, value Summary, modified time.Time) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, name+".summary.json")
	if err = os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
}

func TestReadHistoryAcceptsLegacyMultilineArchive(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	finished := now.Add(-48 * time.Hour)
	summary := Summary{
		SchemaVersion: 5, RunID: "legacy-pretty", Source: "hippo", ResourceTier: "standard",
		TaskClass: "ephemeral", Outcome: "passed", FinishedAt: finished.Format(time.RFC3339Nano),
	}
	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "history", "2026-09-15.jsonl.gz")
	if err = atomicGzipWrite(archive, [][]byte{encoded}, finished); err != nil {
		t.Fatal(err)
	}

	rows, err := ReadHistory(root, Query{Since: HistoryRetention, Now: now, Source: "hippo"})
	if err != nil || len(rows) != 1 || rows[0].RunID != summary.RunID {
		t.Fatalf("history rows=%+v error=%v", rows, err)
	}
}

func TestCleanupCompactsRawAndPriorDaySummaries(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour)
	writeHistoryFixture(t, root, "run-one", Summary{
		SchemaVersion: 5, RunID: "run-one", Source: "hippo", ResourceTier: "standard",
		TaskClass: "ephemeral", Outcome: "passed", FinishedAt: old.Format(time.RFC3339Nano),
	}, old)
	raw := filepath.Join(root, "run-one.jsonl")
	if err := os.WriteFile(raw, []byte("{\"sample\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(raw, old, old); err != nil {
		t.Fatal(err)
	}

	if err := Cleanup(root, now); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(root, "history", "2026-09-15.jsonl.gz")
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("history archive: %v", err)
	}
	var archive bytes.Buffer
	if err := CopyGzip(&archive, archivePath); err != nil {
		t.Fatalf("expand history archive: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(archive.Bytes()), []byte("\n"))
	if len(lines) != 1 || !json.Valid(lines[0]) {
		t.Fatalf("history archive is not one compact JSON row: lines=%d", len(lines))
	}
	if _, err := os.Stat(filepath.Join(root, "raw", "run-one.jsonl.gz")); err != nil {
		t.Fatalf("raw archive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "run-one.summary.json")); !os.IsNotExist(err) {
		t.Fatalf("source summary still exists: %v", err)
	}

	rows, err := ReadHistory(root, Query{Since: 30 * 24 * time.Hour, Now: now, Source: "hippo"})
	if err != nil || len(rows) != 1 || rows[0].RunID != "run-one" {
		t.Fatalf("history rows=%+v error=%v", rows, err)
	}
}

func TestCleanupDropsExpiredMalformedSummaryBeforeCompaction(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	path := filepath.Join(root, "expired.summary.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	expired := now.Add(-HistoryRetention - time.Hour)
	if err := os.Chtimes(path, expired, expired); err != nil {
		t.Fatal(err)
	}

	if err := Cleanup(root, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expired malformed summary still exists: %v", err)
	}
}

func TestPromotionRequiresLastHealthyOverlapsAcrossThreeSources(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	minimum := int64(12 * 1024 * 1024 * 1024)
	pressure := 1
	for index := range 25 {
		finished := now.Add(-time.Duration(25-index) * time.Minute)
		writeHistoryFixture(t, root, "run-"+time.Duration(index).String(), Summary{
			SchemaVersion: 5, RunID: "run-" + time.Duration(index).String(),
			Source: []string{"hippo", "rhino", "ose-public"}[index%3], ResourceTier: "standard",
			TaskClass: "ephemeral", Outcome: "passed", FinishedAt: finished.Format(time.RFC3339Nano),
			PeakOwnerCount: 2, AvailableNonCompressedEstimateMinBytes: &minimum,
			MemoryPressureLevelMax: &pressure, CPUUtilizationP95Percent: 70,
		}, finished)
	}
	criteria := PromotionCriteria{
		CompletedRuns: 25, MinimumSources: 3, MinimumAvailableMemoryBytes: 10 * 1024 * 1024 * 1024,
		MaximumCPUP95Percent: 75,
	}
	evaluation, err := EvaluatePromotion(root, criteria, now)
	if err != nil || !evaluation.Eligible || evaluation.QualifyingRuns != 25 || evaluation.Sources != 3 {
		t.Fatalf("evaluation=%+v error=%v", evaluation, err)
	}

	unsafe := now.Add(time.Minute)
	writeHistoryFixture(t, root, "run-unsafe", Summary{
		SchemaVersion: 5, RunID: "run-unsafe", Source: "hippo", ResourceTier: "heavy",
		TaskClass: "ephemeral", Outcome: "pressure-shed", FinishedAt: unsafe.Format(time.RFC3339Nano),
		PeakOwnerCount: 2, AvailableNonCompressedEstimateMinBytes: &minimum,
		MemoryPressureLevelMax: &pressure, CPUUtilizationP95Percent: 70,
	}, unsafe)
	evaluation, err = EvaluatePromotion(root, criteria, unsafe.Add(time.Minute))
	if err != nil || evaluation.Eligible || evaluation.Reason != "recent-overlap-unhealthy" {
		t.Fatalf("unsafe evaluation=%+v error=%v", evaluation, err)
	}
}
