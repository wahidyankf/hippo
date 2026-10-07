package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/policy"
)

const (
	// RawRetention is the rolling lifetime of compressed raw evidence.
	RawRetention = 7 * 24 * time.Hour
	// HistoryRetention is the rolling lifetime of queryable summary evidence.
	HistoryRetention = 30 * 24 * time.Hour
	// RawMaximumBytes is the shared compressed raw-evidence cap.
	RawMaximumBytes = int64(512 * 1024 * 1024)
	// HistoryMaxBytes is the shared compacted-summary cap.
	HistoryMaxBytes = int64(128 * 1024 * 1024)
)

// Summary is the queryable, privacy-safe subset shared by current and archived evidence.
type Summary struct {
	SchemaVersion                          int                      `json:"schemaVersion"`
	RunID                                  string                   `json:"runId,omitempty"`
	StartedAt                              string                   `json:"startedAt,omitempty"`
	FinishedAt                             string                   `json:"finishedAt,omitempty"`
	Source                                 string                   `json:"source,omitempty"`
	Tags                                   map[string]string        `json:"tags,omitempty"`
	ResourceTier                           string                   `json:"resourceTier,omitempty"`
	TaskClass                              policy.RecordedTaskClass `json:"taskClass,omitzero"`
	Outcome                                RecordedOutcome          `json:"outcome,omitzero"`
	AvailableNonCompressedEstimateMinBytes *int64                   `json:"availableNonCompressedEstimateMinBytes,omitempty"`
	MemoryPressureLevelMax                 *int                     `json:"memoryPressureLevelMax,omitempty"`
	CPUUtilizationP95Percent               float64                  `json:"cpuUtilizationP95Percent,omitempty"`
	SwapOutsDelta                          int64                    `json:"swapOutsDelta,omitempty"`
	PeakOwnerCount                         int                      `json:"peakOwnerCount,omitempty"`
	BudgetOutcome                          RecordedBudgetOutcome    `json:"budgetOutcome,omitzero"`
	AggregateCount                         int                      `json:"aggregateCount,omitempty"`
}

// Query selects history rows without exposing commands, arguments, or paths.
// An Outcome of OutcomeUnset selects every outcome, and an empty Class every class.
type Query struct {
	Since   time.Duration
	Now     time.Time
	Source  string
	Tags    map[string]string
	Class   policy.TaskClass
	Tier    string
	Outcome Outcome
}

// PromotionCriteria defines the evidence required to open the optional owner slot.
type PromotionCriteria struct {
	CompletedRuns               int
	MinimumSources              int
	MinimumAvailableMemoryBytes int64
	MaximumCPUP95Percent        float64
}

// PromotionEvaluation explains whether completed evidence permits owner three.
type PromotionEvaluation struct {
	Eligible       bool   `json:"eligible"`
	QualifyingRuns int    `json:"qualifyingRuns"`
	Sources        int    `json:"sources"`
	Reason         string `json:"reason"`
}

// SummaryTime returns the parsed lifetime end or a caller-provided fallback.
func SummaryTime(summary Summary, fallback time.Time) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, summary.FinishedAt); err == nil {
		return parsed
	}

	return fallback
}

// MatchesClass matches only a known recorded class or the empty wildcard.
func MatchesClass(recorded policy.RecordedTaskClass, wanted policy.TaskClass) bool {
	if wanted == "" {
		return true
	}
	class, known := recorded.TaskClass()

	return known && class == wanted
}

// MatchesQuery applies the validated history query to one summary.
func MatchesQuery(summary Summary, query Query) bool {
	if query.Source != "" && summary.Source != query.Source || !MatchesClass(summary.TaskClass, query.Class) ||
		query.Tier != "" && summary.ResourceTier != query.Tier ||
		query.Outcome != OutcomeUnset && summary.Outcome.Outcome() != query.Outcome {
		return false
	}
	for key, value := range query.Tags {
		if summary.Tags[key] != value {
			return false
		}
	}
	if query.Since > 0 {
		measured := SummaryTime(summary, time.Time{})
		if measured.IsZero() || measured.Before(query.Now.Add(-query.Since)) {
			return false
		}
	}

	return true
}

// HealthyPromotionSummary checks the aggregate safety conditions for owner promotion.
func HealthyPromotionSummary(summary Summary, criteria PromotionCriteria) bool {
	if summary.Outcome.Outcome() != OutcomePassed || summary.AggregateCount != 0 {
		return false
	}
	minimum := summary.AvailableNonCompressedEstimateMinBytes

	return minimum != nil && *minimum >= criteria.MinimumAvailableMemoryBytes &&
		(summary.MemoryPressureLevelMax == nil || *summary.MemoryPressureLevelMax <= 1) &&
		summary.CPUUtilizationP95Percent <= criteria.MaximumCPUP95Percent && summary.SwapOutsDelta == 0
}

// EvaluatePromotion checks the newest overlapping runs; one unhealthy run closes the gate.
func EvaluatePromotion(rows []Summary, criteria PromotionCriteria) (PromotionEvaluation, error) {
	overlaps := make([]Summary, 0, len(rows))
	for _, summary := range rows {
		if summary.PeakOwnerCount >= 2 && summary.AggregateCount == 0 {
			overlaps = append(overlaps, summary)
		}
	}
	if len(overlaps) < criteria.CompletedRuns {
		return PromotionEvaluation{
			QualifyingRuns: len(overlaps), Reason: "insufficient-overlap-runs",
		}, nil
	}
	recent := overlaps[len(overlaps)-criteria.CompletedRuns:]
	sources := map[string]bool{}
	for _, summary := range recent {
		if !HealthyPromotionSummary(summary, criteria) {
			return PromotionEvaluation{
				QualifyingRuns: len(recent), Sources: len(sources), Reason: "recent-overlap-unhealthy",
			}, nil
		}
		if summary.Source != "" && summary.Source != "unlabeled" {
			sources[summary.Source] = true
		}
	}
	if len(sources) < criteria.MinimumSources {
		return PromotionEvaluation{
			QualifyingRuns: len(recent), Sources: len(sources), Reason: "insufficient-sources",
		}, nil
	}

	return PromotionEvaluation{
		Eligible: true, QualifyingRuns: len(recent), Sources: len(sources), Reason: "healthy-evidence",
	}, nil
}

// AggregateHistoryRows groups summaries by source, exact tags, class, tier, and outcome.
// Each synthetic aggregate combines represented run counts, safety extrema, and swap-out deltas.
func AggregateHistoryRows(rows []Summary) []Summary {
	groups := map[string]Summary{}
	for _, row := range rows {
		tagKeys := make([]string, 0, len(row.Tags))
		for tagKey := range row.Tags {
			tagKeys = append(tagKeys, tagKey)
		}
		sort.Strings(tagKeys)
		parts := make([]string, 0, 4+2*len(tagKeys))
		parts = append(parts, row.Source, row.TaskClass.String(), row.ResourceTier, row.Outcome.String())
		for _, tagKey := range tagKeys {
			parts = append(parts, tagKey, row.Tags[tagKey])
		}
		key := strings.Join(parts, "\x00")
		aggregate := groups[key]
		if aggregate.AggregateCount == 0 {
			aggregate = Summary{
				SchemaVersion: 5, Source: row.Source, Tags: row.Tags, TaskClass: row.TaskClass,
				ResourceTier: row.ResourceTier, Outcome: row.Outcome,
				StartedAt: row.StartedAt, FinishedAt: row.FinishedAt,
			}
			hash := sha256.Sum256([]byte(key))
			aggregate.RunID = "aggregate-" + hex.EncodeToString(hash[:8])
		}
		count := max(row.AggregateCount, 1)
		aggregate.AggregateCount += count
		if row.AvailableNonCompressedEstimateMinBytes != nil &&
			(aggregate.AvailableNonCompressedEstimateMinBytes == nil ||
				*row.AvailableNonCompressedEstimateMinBytes < *aggregate.AvailableNonCompressedEstimateMinBytes) {
			value := *row.AvailableNonCompressedEstimateMinBytes
			aggregate.AvailableNonCompressedEstimateMinBytes = &value
		}
		if row.MemoryPressureLevelMax != nil &&
			(aggregate.MemoryPressureLevelMax == nil || *row.MemoryPressureLevelMax > *aggregate.MemoryPressureLevelMax) {
			value := *row.MemoryPressureLevelMax
			aggregate.MemoryPressureLevelMax = &value
		}
		aggregate.CPUUtilizationP95Percent = max(aggregate.CPUUtilizationP95Percent, row.CPUUtilizationP95Percent)
		aggregate.SwapOutsDelta += row.SwapOutsDelta
		aggregate.PeakOwnerCount = max(aggregate.PeakOwnerCount, row.PeakOwnerCount)
		if SummaryTime(row, time.Time{}).After(SummaryTime(aggregate, time.Time{})) {
			aggregate.FinishedAt = row.FinishedAt
		}
		groups[key] = aggregate
	}
	result := make([]Summary, 0, len(groups))
	for _, aggregate := range groups {
		result = append(result, aggregate)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].RunID < result[right].RunID })

	return result
}
