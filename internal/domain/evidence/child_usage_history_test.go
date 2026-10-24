package evidence_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/domain/evidence"
)

func TestSummaryRoundTripKeepsChildUsage(t *testing.T) {
	cpu, wall := 0.9, 1.2
	encoded, err := json.Marshal(evidence.Summary{
		SchemaVersion: 5, RunID: "run", ChildCPUSeconds: &cpu, ChildWallSeconds: &wall,
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded evidence.Summary
	if err = json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ChildCPUSeconds == nil || *decoded.ChildCPUSeconds != 0.9 || decoded.ChildWallSeconds == nil || *decoded.ChildWallSeconds != 1.2 {
		t.Fatalf("round trip lost child usage: %s", encoded)
	}
	for _, field := range []string{`"childCpuSeconds":0.9`, `"childWallSeconds":1.2`} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("summary does not encode %s: %s", field, encoded)
		}
	}
}

func TestSummaryWithoutChildUsageDecodesUnmeasured(t *testing.T) {
	var decoded evidence.Summary
	if err := json.Unmarshal([]byte(`{"schemaVersion":5,"runId":"old","outcome":"passed"}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ChildCPUSeconds != nil || decoded.ChildWallSeconds != nil {
		t.Fatalf("a summary without the fields decoded usage: %+v", decoded)
	}
}

func TestSummaryWithoutChildUsageStillQualifiesForPromotion(t *testing.T) {
	criteria := evidence.PromotionCriteria{
		CompletedRuns: 2, MinimumSources: 1, MinimumAvailableMemoryBytes: 1 << 30, MaximumCPUP95Percent: 50,
	}
	const legacy = `{"schemaVersion":5,"runId":"legacy","source":"hippo","outcome":"passed","taskClass":"ephemeral",` +
		`"availableNonCompressedEstimateMinBytes":4294967296,"memoryPressureLevelMax":1,"cpuUtilizationP95Percent":10,` +
		`"peakOwnerCount":2}`
	var summary evidence.Summary
	if err := json.Unmarshal([]byte(legacy), &summary); err != nil {
		t.Fatal(err)
	}
	if !evidence.HealthyPromotionSummary(summary, criteria) {
		t.Fatalf("a summary without child usage no longer qualifies: %+v", summary)
	}
	cpu, wall := 0.9, 1.2
	summary.ChildCPUSeconds, summary.ChildWallSeconds = &cpu, &wall
	if !evidence.HealthyPromotionSummary(summary, criteria) {
		t.Fatalf("child usage changed the promotion verdict: %+v", summary)
	}
}

func TestAggregateHistoryRowsOmitChildUsage(t *testing.T) {
	first, second := 0.5, 0.75
	wallFirst, wallSecond := 1.0, 2.0
	rows := []evidence.Summary{
		{SchemaVersion: 5, RunID: "a", Source: "hippo", Outcome: evidence.Recorded(evidence.OutcomePassed), ChildCPUSeconds: &first, ChildWallSeconds: &wallFirst},
		{SchemaVersion: 5, RunID: "b", Source: "hippo", Outcome: evidence.Recorded(evidence.OutcomePassed), ChildCPUSeconds: &second, ChildWallSeconds: &wallSecond},
	}
	aggregates := evidence.AggregateHistoryRows(rows)
	if len(aggregates) != 1 || aggregates[0].AggregateCount != 2 {
		t.Fatalf("aggregates = %+v, want one aggregate of two runs", aggregates)
	}
	if aggregates[0].ChildCPUSeconds != nil || aggregates[0].ChildWallSeconds != nil {
		t.Fatalf("aggregate carries per-run child usage: %+v", aggregates[0])
	}
}
