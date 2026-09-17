package evidence

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
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
	SchemaVersion                          int               `json:"schemaVersion"`
	RunID                                  string            `json:"runId,omitempty"`
	StartedAt                              string            `json:"startedAt,omitempty"`
	FinishedAt                             string            `json:"finishedAt,omitempty"`
	Source                                 string            `json:"source,omitempty"`
	Tags                                   map[string]string `json:"tags,omitempty"`
	ResourceTier                           string            `json:"resourceTier,omitempty"`
	TaskClass                              string            `json:"taskClass,omitempty"`
	Outcome                                string            `json:"outcome,omitempty"`
	AvailableNonCompressedEstimateMinBytes *int64            `json:"availableNonCompressedEstimateMinBytes,omitempty"`
	MemoryPressureLevelMax                 *int              `json:"memoryPressureLevelMax,omitempty"`
	CPUUtilizationP95Percent               float64           `json:"cpuUtilizationP95Percent,omitempty"`
	SwapOutsDelta                          int64             `json:"swapOutsDelta,omitempty"`
	PeakOwnerCount                         int               `json:"peakOwnerCount,omitempty"`
	BudgetOutcome                          string            `json:"budgetOutcome,omitempty"`
	AggregateCount                         int               `json:"aggregateCount,omitempty"`
}

// Query selects history rows without exposing commands, arguments, or paths.
type Query struct {
	Since   time.Duration
	Now     time.Time
	Source  string
	Tags    map[string]string
	Class   string
	Tier    string
	Outcome string
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

func summaryTime(summary Summary, fallback time.Time) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, summary.FinishedAt); err == nil {
		return parsed
	}

	return fallback
}

func readSummaryFile(path string, fallback time.Time) (Summary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Summary{}, err
	}
	var summary Summary
	if err = json.Unmarshal(data, &summary); err != nil {
		return Summary{}, err
	}
	if summary.FinishedAt == "" && !fallback.IsZero() {
		summary.FinishedAt = fallback.UTC().Format(time.RFC3339Nano)
	}

	return summary, nil
}

func readGzipSummaries(path string, fallback time.Time) ([]Summary, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close() //nolint:errcheck // Read result already carries scanner/decompressor errors.
	reader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer reader.Close() //nolint:errcheck // Read result already carries scanner/decompressor errors.
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	rows := []Summary{}
	for scanner.Scan() {
		var summary Summary
		if err = json.Unmarshal(scanner.Bytes(), &summary); err != nil {
			return nil, err
		}
		if summary.FinishedAt == "" && !fallback.IsZero() {
			summary.FinishedAt = fallback.UTC().Format(time.RFC3339Nano)
		}
		rows = append(rows, summary)
	}

	return rows, scanner.Err()
}

func archiveFallback(name string) time.Time {
	day := strings.TrimSuffix(name, ".jsonl.gz")
	parsed, _ := time.Parse("2006-01-02", day)

	return parsed
}

func matchesQuery(summary Summary, query Query) bool {
	if query.Source != "" && summary.Source != query.Source || query.Class != "" && summary.TaskClass != query.Class ||
		query.Tier != "" && summary.ResourceTier != query.Tier || query.Outcome != "" && summary.Outcome != query.Outcome {
		return false
	}
	for key, value := range query.Tags {
		if summary.Tags[key] != value {
			return false
		}
	}
	if query.Since > 0 {
		measured := summaryTime(summary, time.Time{})
		if measured.IsZero() || measured.Before(query.Now.Add(-query.Since)) {
			return false
		}
	}

	return true
}

// ReadHistory reads current summaries and compacted daily archives.
func ReadHistory(root string, query Query) ([]Summary, error) {
	if query.Now.IsZero() {
		query.Now = time.Now()
	}
	rows := []Summary{}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return rows, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".summary.json") {
			continue
		}
		info, infoError := entry.Info()
		if infoError != nil {
			return nil, infoError
		}
		summary, readError := readSummaryFile(filepath.Join(root, entry.Name()), info.ModTime())
		if readError != nil {
			return nil, readError
		}
		if matchesQuery(summary, query) {
			rows = append(rows, summary)
		}
	}
	historyEntries, err := os.ReadDir(filepath.Join(root, "history"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, entry := range historyEntries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".jsonl.gz") {
			continue
		}
		archived, readError := readGzipSummaries(
			filepath.Join(root, "history", entry.Name()), archiveFallback(entry.Name()),
		)
		if readError != nil {
			return nil, readError
		}
		for _, summary := range archived {
			if matchesQuery(summary, query) {
				rows = append(rows, summary)
			}
		}
	}
	sort.Slice(rows, func(left, right int) bool {
		return summaryTime(rows[left], time.Time{}).Before(summaryTime(rows[right], time.Time{}))
	})

	return rows, nil
}

func healthyPromotionSummary(summary Summary, criteria PromotionCriteria) bool {
	return summary.Outcome == "passed" && summary.AggregateCount == 0 &&
		summary.AvailableNonCompressedEstimateMinBytes != nil &&
		*summary.AvailableNonCompressedEstimateMinBytes >= criteria.MinimumAvailableMemoryBytes &&
		(summary.MemoryPressureLevelMax == nil || *summary.MemoryPressureLevelMax <= 1) &&
		summary.CPUUtilizationP95Percent <= criteria.MaximumCPUP95Percent && summary.SwapOutsDelta == 0 &&
		summary.BudgetOutcome != "pressure-shed" && summary.BudgetOutcome != "storage-shed"
}

// EvaluatePromotion checks the newest overlapping runs; one unhealthy run closes the gate.
func EvaluatePromotion(root string, criteria PromotionCriteria, now time.Time) (PromotionEvaluation, error) {
	rows, err := ReadHistory(root, Query{Since: HistoryRetention, Now: now})
	if err != nil {
		return PromotionEvaluation{}, err
	}
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
		if !healthyPromotionSummary(summary, criteria) {
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

func atomicGzipWrite(path string, rows [][]byte, modified time.Time) (returnError error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".history-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			returnError = errors.Join(returnError, temporary.Close())
		}
		if removeError := os.Remove(temporaryPath); !errors.Is(removeError, os.ErrNotExist) {
			returnError = errors.Join(returnError, removeError)
		}
	}()
	if err = temporary.Chmod(0o600); err != nil {
		return err
	}
	compressed := gzip.NewWriter(temporary)
	for _, row := range rows {
		if _, err = compressed.Write(append(row, '\n')); err != nil {
			return err
		}
	}
	if err = compressed.Close(); err != nil {
		return err
	}
	if err = temporary.Sync(); err != nil {
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	closed = true
	if err = os.Rename(temporaryPath, path); err != nil {
		return err
	}
	if !modified.IsZero() {
		return os.Chtimes(path, modified, modified)
	}

	return nil
}

func compressRawFile(source, destination string, modified time.Time) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err = atomicGzipWrite(destination, [][]byte{data}, modified); err != nil {
		return err
	}

	return os.Remove(source)
}

func summaryArchiveDay(path string, info os.FileInfo) (time.Time, error) {
	summary, err := readSummaryFile(path, info.ModTime())
	if err != nil {
		return time.Time{}, err
	}
	measured := summaryTime(summary, info.ModTime())

	return time.Date(measured.UTC().Year(), measured.UTC().Month(), measured.UTC().Day(), 0, 0, 0, 0, time.UTC), nil
}

func compactSummaries(root string, now time.Time, protected map[string]bool) error { //nolint:gocognit,cyclop // Atomic daily merge keeps discovery, deduplication, write, and source removal visibly ordered.
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	groups := map[string][]string{}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".summary.json") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if protected[path] {
			continue
		}
		info, infoError := entry.Info()
		if infoError != nil {
			return infoError
		}
		// Expired loose summaries cannot contribute to the rolling history. Drop
		// them by their filesystem age before decoding so one corrupt, already
		// expired file cannot block retention for the shared machine log.
		if now.Sub(info.ModTime()) > HistoryRetention {
			if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}

			continue
		}
		day, dayError := summaryArchiveDay(path, info)
		if dayError != nil {
			return dayError
		}
		if day.Before(today) {
			groups[day.Format("2006-01-02")] = append(groups[day.Format("2006-01-02")], path)
		}
	}
	for day, paths := range groups {
		destination := filepath.Join(root, "history", day+".jsonl.gz")
		rows := map[string][]byte{}
		if archived, readError := readGzipSummaries(destination, archiveFallback(filepath.Base(destination))); readError == nil {
			for _, summary := range archived {
				encoded, encodeError := json.Marshal(summary)
				if encodeError != nil {
					return encodeError
				}
				rows[summary.RunID] = encoded
			}
		} else if !errors.Is(readError, os.ErrNotExist) {
			return readError
		}
		for _, path := range paths {
			data, readError := os.ReadFile(path)
			if readError != nil {
				return readError
			}
			var summary Summary
			if readError = json.Unmarshal(data, &summary); readError != nil {
				return readError
			}
			key := summary.RunID
			if key == "" {
				hash := sha256.Sum256(data)
				key = hex.EncodeToString(hash[:])
			}
			rows[key] = data
		}
		keys := make([]string, 0, len(rows))
		for key := range rows {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		encoded := make([][]byte, 0, len(keys))
		for _, key := range keys {
			encoded = append(encoded, bytesTrimSpace(rows[key]))
		}
		modified, _ := time.Parse("2006-01-02", day)
		if err = atomicGzipWrite(destination, encoded, modified); err != nil {
			return err
		}
		for _, path := range paths {
			if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}

	return nil
}

func bytesTrimSpace(data []byte) []byte {
	return []byte(strings.TrimSpace(string(data)))
}

func compactRaw(root string, now time.Time, active, preserved map[string]bool) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.Contains(entry.Name(), ".jsonl") ||
			strings.HasSuffix(entry.Name(), ".summary.json") || strings.HasSuffix(entry.Name(), ".active.json") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if preserved[path] || protectedByActiveWriter(path, active) {
			continue
		}
		info, infoError := entry.Info()
		if infoError != nil {
			return infoError
		}
		// Daily compaction keeps today's completed stream directly readable and
		// moves only prior-day raw evidence into the bounded gzip window.
		if !info.ModTime().Before(today) {
			continue
		}
		destination := filepath.Join(root, "raw", entry.Name()+".gz")
		if err = compressRawFile(path, destination, info.ModTime()); err != nil {
			return err
		}
	}

	return nil
}

func pruneDirectory(path string, now time.Time, retention time.Duration, maximumBytes int64) error { //nolint:gocognit // Expiry, temporary handling, accounting, and oldest-first pruning form one directory transaction.
	entries, err := os.ReadDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	retained := []retainedFile{}
	var total int64
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		info, infoError := entry.Info()
		if infoError != nil {
			return infoError
		}
		entryPath := filepath.Join(path, entry.Name())
		if strings.HasPrefix(entry.Name(), ".") {
			if now.Sub(info.ModTime()) > atomicWriteTemporaryRetention {
				if err = os.Remove(entryPath); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
			continue
		}
		modified := info.ModTime()
		if strings.HasSuffix(entry.Name(), ".jsonl.gz") {
			if day := archiveFallback(entry.Name()); !day.IsZero() {
				modified = day
			}
		}
		if now.Sub(modified) > retention {
			if err = os.Remove(entryPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			continue
		}
		retained = append(retained, retainedFile{path: entryPath, size: info.Size(), modified: modified})
		total += info.Size()
	}
	sort.Slice(retained, func(left, right int) bool { return retained[left].modified.Before(retained[right].modified) })
	for _, entry := range retained {
		if total <= maximumBytes {
			break
		}
		if err = os.Remove(entry.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		total -= entry.size
	}

	return nil
}

func pruneTemporaryFiles(path string, now time.Time) error {
	entries, err := os.ReadDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, infoError := entry.Info()
		if infoError != nil {
			return infoError
		}
		if now.Sub(info.ModTime()) > atomicWriteTemporaryRetention {
			if err = os.Remove(filepath.Join(path, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}

	return nil
}

func aggregateHistoryRows(rows []Summary) []Summary {
	groups := map[string]Summary{}
	for _, row := range rows {
		tagKeys := make([]string, 0, len(row.Tags))
		for tagKey := range row.Tags {
			tagKeys = append(tagKeys, tagKey)
		}
		sort.Strings(tagKeys)
		parts := make([]string, 0, 4+2*len(tagKeys))
		parts = append(parts, row.Source, row.TaskClass, row.ResourceTier, row.Outcome)
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
		if summaryTime(row, time.Time{}).After(summaryTime(aggregate, time.Time{})) {
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

func pruneHistory(path string, now time.Time) error { //nolint:gocognit // The loop must remeasure after each atomic aggregation or removal.
	if err := pruneDirectory(path, now, HistoryRetention, int64(^uint64(0)>>1)); err != nil {
		return err
	}
	for {
		entries, err := os.ReadDir(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		retained := []retainedFile{}
		var total int64
		for _, entry := range entries {
			if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".jsonl.gz") {
				continue
			}
			info, infoError := entry.Info()
			if infoError != nil {
				return infoError
			}
			modified := archiveFallback(entry.Name())
			retained = append(retained, retainedFile{
				path: filepath.Join(path, entry.Name()), size: info.Size(), modified: modified,
			})
			total += info.Size()
		}
		if total <= HistoryMaxBytes || len(retained) == 0 {
			return nil
		}
		sort.Slice(retained, func(left, right int) bool { return retained[left].modified.Before(retained[right].modified) })
		oldest := retained[0]
		rows, readError := readGzipSummaries(oldest.path, oldest.modified)
		if readError != nil {
			return readError
		}
		alreadyAggregated := len(rows) > 0
		for _, row := range rows {
			alreadyAggregated = alreadyAggregated && row.AggregateCount > 0
		}
		if alreadyAggregated {
			if err = os.Remove(oldest.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}

			continue
		}
		aggregates := aggregateHistoryRows(rows)
		encoded := make([][]byte, 0, len(aggregates))
		for _, aggregate := range aggregates {
			row, encodeError := json.Marshal(aggregate)
			if encodeError != nil {
				return encodeError
			}
			encoded = append(encoded, row)
		}
		if err = atomicGzipWrite(oldest.path, encoded, oldest.modified); err != nil {
			return err
		}
	}
}

func compactLocked(root string, now time.Time, active, preserved map[string]bool) error {
	protected := map[string]bool{}
	for path := range preserved {
		protected[path] = true
	}
	for prefix := range active {
		protected[prefix] = true
	}
	if err := compactSummaries(root, now, protected); err != nil {
		return fmt.Errorf("compact summaries: %w", err)
	}
	if err := compactRaw(root, now, active, preserved); err != nil {
		return fmt.Errorf("compact raw evidence: %w", err)
	}
	if err := pruneDirectory(filepath.Join(root, "raw"), now, RawRetention, RawMaximumBytes); err != nil {
		return err
	}
	if err := pruneTemporaryFiles(filepath.Join(root, "owner-metadata"), now); err != nil {
		return err
	}
	if err := pruneDirectory(filepath.Join(root, "receipts"), now, HistoryRetention, HistoryMaxBytes); err != nil {
		return err
	}

	return pruneHistory(filepath.Join(root, "history"), now)
}

// CopyGzip expands one compressed raw stream for diagnostic tooling.
func CopyGzip(destination io.Writer, source string) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close() //nolint:errcheck // Copy/decompressor errors are returned.
	reader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer reader.Close() //nolint:errcheck // Copy/decompressor errors are returned.
	written, err := io.CopyN(destination, reader, RawMaximumBytes+1)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	if written > RawMaximumBytes {
		return errors.New("raw gzip expands beyond the shared evidence cap")
	}

	return nil
}
