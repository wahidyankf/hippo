package evidence_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
)

func historyContractSummary() evidence.Summary {
	return evidence.Summary{SchemaVersion: 5, RunID: "run-one", Source: "hippo", Tags: map[string]string{"role": "test", "kind": "build"}, ResourceTier: "standard", TaskClass: policy.RecordedClass(policy.TaskEphemeral), Outcome: evidence.Recorded(evidence.OutcomePassed), StartedAt: "2026-10-01T00:00:00Z", FinishedAt: "2026-10-01T01:00:00Z", AvailableNonCompressedEstimateMinBytes: new(10 * policy.GiB), MemoryPressureLevelMax: new(1), CPUUtilizationP95Percent: 10, SwapOutsDelta: 2, PeakOwnerCount: 2}
}

func TestHistoryContractAggregationPreservesRepresentedRunsAndSafetyMetrics(t *testing.T) {
	first := historyContractSummary()
	second := historyContractSummary()
	second.RunID = "run-two"
	second.Tags = map[string]string{"kind": "build", "role": "test"}
	second.FinishedAt = "2026-10-01T03:00:00Z"
	second.AvailableNonCompressedEstimateMinBytes = new(8 * policy.GiB)
	second.MemoryPressureLevelMax = new(4)
	second.CPUUtilizationP95Percent = 60
	second.SwapOutsDelta = 5
	second.PeakOwnerCount = 3
	compacted := historyContractSummary()
	compacted.RunID = "prior-aggregate"
	compacted.AggregateCount = 5
	compacted.FinishedAt = "2026-10-01T02:00:00Z"
	compacted.AvailableNonCompressedEstimateMinBytes = new(12 * policy.GiB)
	compacted.MemoryPressureLevelMax = new(2)
	compacted.CPUUtilizationP95Percent = 20
	compacted.SwapOutsDelta = 7
	compacted.PeakOwnerCount = 1
	rows := []evidence.Summary{first, second, compacted}
	before, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	aggregated := evidence.AggregateHistoryRows(rows)
	if len(aggregated) != 1 {
		t.Fatalf("same identity became %d groups", len(aggregated))
	}
	result := aggregated[0]
	assertHistoryContractAggregateIdentity(t, result, first, 7)
	if result.FinishedAt != second.FinishedAt || *result.AvailableNonCompressedEstimateMinBytes != 8*policy.GiB || *result.MemoryPressureLevelMax != 4 || result.CPUUtilizationP95Percent != 60 || result.SwapOutsDelta != 14 || result.PeakOwnerCount != 3 {
		t.Fatalf("aggregate lost safety extrema/counts: %+v", result)
	}
	after, err := json.Marshal(rows)
	if err != nil || string(after) != string(before) {
		t.Fatalf("aggregation changed source summaries: before=%s after=%s error=%v", before, after, err)
	}
	*second.AvailableNonCompressedEstimateMinBytes = 99 * policy.GiB
	*second.MemoryPressureLevelMax = 99
	if *result.AvailableNonCompressedEstimateMinBytes != 8*policy.GiB || *result.MemoryPressureLevelMax != 4 {
		t.Fatal("aggregate safety metrics alias mutable input readings")
	}
}

func assertHistoryContractAggregateIdentity(t *testing.T, actual, source evidence.Summary, count int) {
	t.Helper()
	if actual.SchemaVersion != 5 || !strings.HasPrefix(actual.RunID, "aggregate-") || actual.RunID == source.RunID || actual.Source != source.Source || !reflect.DeepEqual(actual.Tags, source.Tags) || actual.TaskClass != source.TaskClass || actual.ResourceTier != source.ResourceTier || actual.Outcome != source.Outcome || actual.AggregateCount != count {
		t.Fatalf("aggregate grouping/represented count changed: actual=%+v source=%+v", actual, source)
	}
}

func TestHistoryContractAggregationSeparatesEveryIdentityDimension(t *testing.T) {
	rows := make([]evidence.Summary, 0, 7)
	rows = append(rows, historyContractSummary())
	for _, dimension := range []string{"source", "tag-value", "tag-key", "class", "tier", "outcome"} {
		row := historyContractSummary()
		switch dimension {
		case "source":
			row.Source = "rhino"
		case "tag-value":
			row.Tags = map[string]string{"role": "release", "kind": "build"}
		case "tag-key":
			row.Tags = map[string]string{"task": "test", "kind": "build"}
		case "class":
			row.TaskClass = policy.RecordedClass(policy.TaskService)
		case "tier":
			row.ResourceTier = "light"
		case "outcome":
			row.Outcome = evidence.Recorded(evidence.OutcomeTaskFailed)
		}
		rows = append(rows, row)
	}
	forward := evidence.AggregateHistoryRows(rows)
	slices.Reverse(rows)
	reverse := evidence.AggregateHistoryRows(rows)
	if len(forward) != 7 || !reflect.DeepEqual(forward, reverse) {
		t.Fatalf("grouping/order depends on input order: forward=%+v reverse=%+v", forward, reverse)
	}
	ids := map[string]bool{}
	for index, row := range forward {
		if ids[row.RunID] || row.AggregateCount != 1 || index > 0 && row.RunID < forward[index-1].RunID {
			t.Fatalf("unstable, duplicate, or incorrect aggregate identities: %+v", forward)
		}
		ids[row.RunID] = true
	}
}

func TestHistoryContractAggregationKeepsUnavailableMetricsUnknown(t *testing.T) {
	row := historyContractSummary()
	row.AvailableNonCompressedEstimateMinBytes = nil
	row.MemoryPressureLevelMax = nil
	row.FinishedAt = "invalid"
	result := evidence.AggregateHistoryRows([]evidence.Summary{row, row})
	if len(result) != 1 || result[0].AggregateCount != 2 || result[0].AvailableNonCompressedEstimateMinBytes != nil || result[0].MemoryPressureLevelMax != nil || result[0].FinishedAt != "invalid" {
		t.Fatalf("unknown evidence became metrics or timestamps: %+v", result)
	}
	empty := evidence.AggregateHistoryRows(nil)
	if empty == nil || len(empty) != 0 {
		t.Fatalf("empty history aggregation=%v", empty)
	}
}

func TestHistoryContractSummaryTimeUsesOnlyValidEvidenceOrInjectedFallback(t *testing.T) {
	fallback := time.Unix(123, 0).UTC()
	for _, text := range []string{"", "invalid", "2026-10-01T03:00:00.123Z"} {
		t.Run(text, func(t *testing.T) {
			summary := evidence.Summary{FinishedAt: text}
			actual := evidence.SummaryTime(summary, fallback)
			want := fallback
			if text == "2026-10-01T03:00:00.123Z" {
				want = time.Date(2026, time.October, 1, 3, 0, 0, 123000000, time.UTC)
			}
			if !actual.Equal(want) {
				t.Fatalf("summary time=%v want=%v", actual, want)
			}
		})
	}
}

func TestHistoryContractQueryRequiresEveryRequestedFilter(t *testing.T) {
	row := historyContractSummary()
	now := time.Date(2026, time.October, 2, 1, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name  string
		query evidence.Query
		want  bool
	}{
		{name: "wildcard", want: true},
		{name: "all-match", query: evidence.Query{Since: 24 * time.Hour, Now: now, Source: "hippo", Tags: map[string]string{"role": "test"}, Class: policy.TaskEphemeral, Tier: "standard", Outcome: evidence.OutcomePassed}, want: true},
		{name: "source", query: evidence.Query{Source: "rhino"}},
		{name: "class", query: evidence.Query{Class: policy.TaskService}},
		{name: "tier", query: evidence.Query{Tier: "light"}},
		{name: "outcome", query: evidence.Query{Outcome: evidence.OutcomeTaskFailed}},
		{name: "tag-value", query: evidence.Query{Tags: map[string]string{"role": "release"}}},
		{name: "tag-key", query: evidence.Query{Tags: map[string]string{"task": "test"}}},
		{name: "expired", query: evidence.Query{Since: 24*time.Hour - time.Nanosecond, Now: now}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if actual := evidence.MatchesQuery(row, test.query); actual != test.want {
				t.Fatalf("query=%+v matched=%t want=%t", test.query, actual, test.want)
			}
		})
	}
	for _, text := range []string{"", "invalid"} {
		row.FinishedAt = text
		if evidence.MatchesQuery(row, evidence.Query{Since: time.Hour, Now: now}) {
			t.Fatalf("unknown lifetime date matched bounded history: %q", text)
		}
	}
}

func TestHistoryContractFutureRecordedWordsSurviveUnfilteredQueries(t *testing.T) {
	var row evidence.Summary
	if err := json.Unmarshal([]byte(`{"schemaVersion":5,"source":"hippo","taskClass":"future-class","outcome":"future-outcome","budgetOutcome":"future-budget"}`), &row); err != nil {
		t.Fatal(err)
	}
	if !evidence.MatchesQuery(row, evidence.Query{}) || evidence.MatchesClass(row.TaskClass, policy.TaskEphemeral) || evidence.MatchesQuery(row, evidence.Query{Outcome: evidence.OutcomePassed}) {
		t.Fatalf("future words were filtered as known defaults: %+v", row)
	}
	encoded, err := json.Marshal(row)
	want := `{"schemaVersion":5,"source":"hippo","taskClass":"future-class","outcome":"future-outcome","budgetOutcome":"future-budget"}`
	if err != nil || string(encoded) != want {
		t.Fatalf("future row wire text changed: %s error=%v", encoded, err)
	}
}

func promotionContractCriteria() evidence.PromotionCriteria {
	return evidence.PromotionCriteria{CompletedRuns: 2, MinimumSources: 2, MinimumAvailableMemoryBytes: 10 * policy.GiB, MaximumCPUP95Percent: 75}
}

func TestHistoryContractPromotionRequiresRawPassedHealthySummaries(t *testing.T) {
	for _, condition := range []string{"healthy", "unknown-pressure", "failed", "unknown-outcome", "aggregate", "missing-memory", "low-memory", "pressure", "cpu", "swap"} {
		t.Run(condition, func(t *testing.T) {
			row := historyContractSummary()
			want := condition == "healthy" || condition == "unknown-pressure"
			switch condition {
			case "unknown-pressure":
				row.MemoryPressureLevelMax = nil
			case "failed":
				row.Outcome = evidence.Recorded(evidence.OutcomeTaskFailed)
			case "unknown-outcome":
				if err := row.Outcome.UnmarshalText([]byte("future-outcome")); err != nil {
					t.Fatal(err)
				}
			case "aggregate":
				row.AggregateCount = 2
			case "missing-memory":
				row.AvailableNonCompressedEstimateMinBytes = nil
			case "low-memory":
				row.AvailableNonCompressedEstimateMinBytes = new(9 * policy.GiB)
			case "pressure":
				row.MemoryPressureLevelMax = new(2)
			case "cpu":
				row.CPUUtilizationP95Percent = 76
			case "swap":
				row.SwapOutsDelta = 1
			}
			if condition != "swap" {
				row.SwapOutsDelta = 0
			}
			if actual := evidence.HealthyPromotionSummary(row, promotionContractCriteria()); actual != want {
				t.Fatalf("promotion safety accepted=%t want=%t row=%+v", actual, want, row)
			}
		})
	}
}

func TestHistoryContractPromotionUsesOnlyNewestRawOverlapsAndDistinctSources(t *testing.T) {
	for _, scenario := range []string{"insufficient-overlap", "unhealthy-recent", "unlabeled-sources", "duplicate-source", "healthy-newest"} {
		t.Run(scenario, func(t *testing.T) {
			rows, want := promotionContractRows(scenario)
			actual, err := evidence.EvaluatePromotion(rows, promotionContractCriteria())
			if err != nil || actual != want {
				t.Fatalf("promotion=%+v want=%+v error=%v", actual, want, err)
			}
		})
	}
}

func promotionContractRows(scenario string) ([]evidence.Summary, evidence.PromotionEvaluation) {
	first := historyContractSummary()
	first.SwapOutsDelta = 0
	second := first
	second.Source = "rhino"
	switch scenario {
	case "insufficient-overlap":
		second.PeakOwnerCount = 1
		compacted := first
		compacted.AggregateCount = 20
		compacted.PeakOwnerCount = 3
		return []evidence.Summary{first, second, compacted}, evidence.PromotionEvaluation{QualifyingRuns: 1, Reason: "insufficient-overlap-runs"}
	case "unhealthy-recent":
		second.Outcome = evidence.Recorded(evidence.OutcomeTaskFailed)
		return []evidence.Summary{first, second}, evidence.PromotionEvaluation{QualifyingRuns: 2, Sources: 1, Reason: "recent-overlap-unhealthy"}
	case "unlabeled-sources":
		first.Source = ""
		second.Source = "unlabeled"
		return []evidence.Summary{first, second}, evidence.PromotionEvaluation{QualifyingRuns: 2, Sources: 0, Reason: "insufficient-sources"}
	case "duplicate-source":
		second.Source = first.Source
		return []evidence.Summary{first, second}, evidence.PromotionEvaluation{QualifyingRuns: 2, Sources: 1, Reason: "insufficient-sources"}
	default:
		older := first
		older.Outcome = evidence.Recorded(evidence.OutcomeTaskFailed)
		isolated := first
		isolated.PeakOwnerCount = 1
		return []evidence.Summary{older, first, isolated, second}, evidence.PromotionEvaluation{Eligible: true, QualifyingRuns: 2, Sources: 2, Reason: "healthy-evidence"}
	}
}
