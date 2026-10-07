package evidence //nolint:testpackage // Compaction fixtures exercise private atomic archive boundaries.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	domain "github.com/wahidyankf/hippo/internal/domain/evidence"

	"github.com/wahidyankf/hippo/internal/policy"
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
		TaskClass: policy.RecordedClass(policy.TaskEphemeral), Outcome: Recorded(OutcomePassed), FinishedAt: finished.Format(time.RFC3339Nano),
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
		TaskClass: policy.RecordedClass(policy.TaskEphemeral), Outcome: Recorded(OutcomePassed), FinishedAt: old.Format(time.RFC3339Nano),
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
			TaskClass: policy.RecordedClass(policy.TaskEphemeral), Outcome: Recorded(OutcomePassed), FinishedAt: finished.Format(time.RFC3339Nano),
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
		TaskClass: policy.RecordedClass(policy.TaskEphemeral), Outcome: Recorded(OutcomePressureShed), FinishedAt: unsafe.Format(time.RFC3339Nano),
		PeakOwnerCount: 2, AvailableNonCompressedEstimateMinBytes: &minimum,
		MemoryPressureLevelMax: &pressure, CPUUtilizationP95Percent: 70,
	}, unsafe)
	evaluation, err = EvaluatePromotion(root, criteria, unsafe.Add(time.Minute))
	if err != nil || evaluation.Eligible || evaluation.Reason != "recent-overlap-unhealthy" {
		t.Fatalf("unsafe evaluation=%+v error=%v", evaluation, err)
	}
}

func TestHistoryListsAnUnknownOutcomeAsRecordedAndNeverPromotesOnIt(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	finished := now.Add(-time.Minute)
	minimum := int64(12 * 1024 * 1024 * 1024)
	pressure := 1
	healthy := Summary{
		SchemaVersion: 5, RunID: "run-healthy", Source: "hippo", ResourceTier: "standard", TaskClass: policy.RecordedClass(policy.TaskEphemeral),
		Outcome: Recorded(OutcomePassed), BudgetOutcome: RecordedBudget(BudgetOutcomeAdmitted),
		FinishedAt: finished.Format(time.RFC3339Nano), PeakOwnerCount: 2, AvailableNonCompressedEstimateMinBytes: &minimum,
		MemoryPressureLevelMax: &pressure, CPUUtilizationP95Percent: 70,
	}
	criteria := PromotionCriteria{
		CompletedRuns: 1, MinimumSources: 1, MinimumAvailableMemoryBytes: 10 * 1024 * 1024 * 1024,
		MaximumCPUP95Percent: 75,
	}

	// The control: the same run, recorded as a run this version knows passed.
	control := t.TempDir()
	writeHistoryFixture(t, control, "run-healthy", healthy, finished)
	evaluation, err := EvaluatePromotion(control, criteria, now)
	if err != nil || !evaluation.Eligible {
		t.Fatalf("a passed overlap is not eligible: evaluation=%+v error=%v", evaluation, err)
	}

	// The same bytes with the one word a later version might write instead.
	encoded, err := json.Marshal(healthy)
	if err != nil {
		t.Fatal(err)
	}
	future := bytes.Replace(encoded, []byte(`"outcome":"passed"`), []byte(`"outcome":"future-outcome"`), 1)
	if bytes.Equal(future, encoded) {
		t.Fatalf("the fixture does not record the outcome as passed: %s", encoded)
	}
	root := t.TempDir()
	path := filepath.Join(root, "run-healthy.summary.json")
	if err = os.WriteFile(path, append(future, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(path, finished, finished); err != nil {
		t.Fatal(err)
	}

	rows, err := ReadHistory(root, Query{Since: HistoryRetention, Now: now})
	if err != nil || len(rows) != 1 {
		t.Fatalf("history rows=%+v error=%v", rows, err)
	}
	if rows[0].Outcome.String() != "future-outcome" || rows[0].Outcome.Outcome() != OutcomeUnknown ||
		rows[0].BudgetOutcome.String() != "admitted" || rows[0].BudgetOutcome.BudgetOutcome() != BudgetOutcomeAdmitted {
		t.Fatalf("the row reads outcome=%q (%d) budget=%q, want the recorded words", rows[0].Outcome.String(),
			rows[0].Outcome.Outcome(), rows[0].BudgetOutcome.String())
	}
	listed, err := json.Marshal(rows[0])
	if err != nil || !bytes.Contains(listed, []byte(`"outcome":"future-outcome"`)) ||
		!bytes.Contains(listed, []byte(`"budgetOutcome":"admitted"`)) {
		t.Errorf("history lists the row as %s (%v), want the recorded words back", listed, err)
	}
	if rows, err = ReadHistory(root, Query{Since: HistoryRetention, Now: now, Outcome: OutcomePassed}); err != nil || len(rows) != 0 {
		t.Errorf("the passed filter listed %+v (%v), want no row for an outcome it does not know", rows, err)
	}
	evaluation, err = EvaluatePromotion(root, criteria, now)
	if err != nil || evaluation.Eligible || evaluation.Reason != "recent-overlap-unhealthy" {
		t.Errorf("an unknown outcome counted toward promotion: evaluation=%+v error=%v", evaluation, err)
	}
}

func TestAggregationKeepsACancelledRunApartFromADeferral(t *testing.T) {
	// A cancellation, a failed admission, and a capacity deferral never
	// started anything, but they are different events, and aggregating the
	// oldest day must not merge them or the aggregate would report one as
	// another.
	rows := domain.AggregateHistoryRows([]Summary{
		{Source: "repo", TaskClass: policy.RecordedClass(policy.TaskEphemeral), ResourceTier: "light", Outcome: Recorded(OutcomeAdmissionCancelled)},
		{Source: "repo", TaskClass: policy.RecordedClass(policy.TaskEphemeral), ResourceTier: "light", Outcome: Recorded(OutcomeAdmissionCancelled)},
		{Source: "repo", TaskClass: policy.RecordedClass(policy.TaskEphemeral), ResourceTier: "light", Outcome: Recorded(OutcomeCapacityDeferred)},
		{Source: "repo", TaskClass: policy.RecordedClass(policy.TaskEphemeral), ResourceTier: "light", Outcome: Recorded(OutcomeAdmissionFailed)},
	})
	counts := map[string]int{}
	for _, row := range rows {
		counts[row.Outcome.String()] += row.AggregateCount
	}
	if len(rows) != 3 || counts["admission-cancelled"] != 2 || counts["capacity-deferred"] != 1 ||
		counts["admission-failed"] != 1 {
		t.Fatalf("aggregation merged or lost outcomes: %+v", counts)
	}
	if !domain.MatchesQuery(rows[0], Query{Outcome: rows[0].Outcome.Outcome()}) ||
		domain.MatchesQuery(Summary{Outcome: Recorded(OutcomeAdmissionCancelled)}, Query{Outcome: OutcomeCapacityDeferred}) ||
		domain.MatchesQuery(Summary{Outcome: Recorded(OutcomeAdmissionFailed)}, Query{Outcome: OutcomeCapacityDeferred}) {
		t.Fatal("the outcome filter does not separate a cancellation from a deferral")
	}
}

// classHistoryFixture stages three current summaries in a root of their own: one whose class this version has no
// member for, one it has, and one that records no class.
func classHistoryFixture(t *testing.T, now time.Time) string {
	t.Helper()
	root := t.TempDir()
	finished := now.Add(-time.Minute).Format(time.RFC3339Nano)
	for run, class := range map[string]string{"run-batch": "batch", "run-ephemeral": "ephemeral", "run-unrecorded": ""} {
		document := map[string]any{
			"schemaVersion": 5, "runId": run, "source": "hippo", "outcome": "passed", "finishedAt": finished,
		}
		if class != "" {
			document["taskClass"] = class
		}
		encoded, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, run+".summary.json"), append(encoded, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	return root
}

func TestHistoryListsAClassThisVersionHasNoMemberForAsRecorded(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	rows, err := ReadHistory(classHistoryFixture(t, now), Query{Since: HistoryRetention, Now: now})
	if err != nil || len(rows) != 3 {
		t.Fatalf("history rows=%+v error=%v, want all three, none refused", rows, err)
	}
	byRun := map[string]Summary{}
	for _, row := range rows {
		byRun[row.RunID] = row
	}
	if batch := byRun["run-batch"].TaskClass; batch.String() != "batch" {
		t.Errorf("an unknown class reads %q, want the recorded text", batch.String())
	} else if class, known := batch.TaskClass(); known || class != "" {
		t.Errorf("an unknown class read as the member %q, want no member", class)
	}
	if class, known := byRun["run-ephemeral"].TaskClass.TaskClass(); !known || class != policy.TaskEphemeral {
		t.Errorf("a recorded ephemeral class read as %q (known=%t)", class, known)
	}
	if unrecorded := byRun["run-unrecorded"].TaskClass; !unrecorded.IsZero() || unrecorded.String() != "" {
		t.Errorf("a summary with no class reads %q, want nothing recorded", unrecorded.String())
	}
	for run, want := range map[string]string{"run-batch": `"taskClass":"batch"`, "run-ephemeral": `"taskClass":"ephemeral"`} {
		if listed, marshalError := json.Marshal(byRun[run]); marshalError != nil || !bytes.Contains(listed, []byte(want)) {
			t.Errorf("history lists %s as %s (%v), want %s back", run, listed, marshalError, want)
		}
	}
	if listed, marshalError := json.Marshal(byRun["run-unrecorded"]); marshalError != nil || bytes.Contains(listed, []byte("taskClass")) {
		t.Errorf("history lists a summary with no class as %s (%v), want the field omitted", listed, marshalError)
	}
}

// A filter takes a class this version has a member for, so it selects exactly the rows recorded as that member and can
// never select a class it has none for.
func TestHistoryClassFilterSelectsOnlyAKnownClass(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	root := classHistoryFixture(t, now)
	for _, test := range []struct {
		class policy.TaskClass
		want  []string
	}{
		{policy.TaskEphemeral, []string{"run-ephemeral"}},
		{policy.TaskService, []string{}},
	} {
		rows, err := ReadHistory(root, Query{Since: HistoryRetention, Now: now, Class: test.class})
		got := make([]string, 0, len(rows))
		for _, row := range rows {
			got = append(got, row.RunID)
		}
		if err != nil || !slices.Equal(got, test.want) {
			t.Errorf("the %s filter listed %v (%v), want %v", test.class, got, err, test.want)
		}
	}
}

func TestAggregationKeepsAClassItHasNoMemberForApartAndAsRecorded(t *testing.T) {
	rows := domain.AggregateHistoryRows([]Summary{
		{Source: "repo", TaskClass: policy.RecordedClass("batch"), ResourceTier: "light", Outcome: Recorded(OutcomePassed)},
		{Source: "repo", TaskClass: policy.RecordedClass("batch"), ResourceTier: "light", Outcome: Recorded(OutcomePassed)},
		{Source: "repo", TaskClass: policy.RecordedClass(policy.TaskEphemeral), ResourceTier: "light", Outcome: Recorded(OutcomePassed)},
	})
	counts := map[string]int{}
	for _, row := range rows {
		counts[row.TaskClass.String()] += row.AggregateCount
	}
	if len(rows) != 2 || counts["batch"] != 2 || counts["ephemeral"] != 1 {
		t.Fatalf("aggregation merged or lost classes: %+v over %d rows", counts, len(rows))
	}
}
