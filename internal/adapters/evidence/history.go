package evidence

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	domain "github.com/wahidyankf/hippo/internal/domain/evidence"
)

func readSummaryFile(port system.System, path string, fallback time.Time) (Summary, error) {
	data, err := port.Files.ReadFile(path)
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

func readGzipSummaries(port system.System, path string, fallback time.Time) ([]Summary, error) {
	file, err := port.Files.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close() //nolint:errcheck // Read result already carries scanner/decompressor errors.
	reader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer reader.Close() //nolint:errcheck // Read result already carries scanner/decompressor errors.
	decoder := json.NewDecoder(reader)
	rows := []Summary{}
	for {
		var summary Summary
		if err = decoder.Decode(&summary); errors.Is(err, io.EOF) {
			return rows, nil
		}
		if err != nil {
			return nil, err
		}
		if summary.FinishedAt == "" && !fallback.IsZero() {
			summary.FinishedAt = fallback.UTC().Format(time.RFC3339Nano)
		}
		rows = append(rows, summary)
	}
}

func archiveFallback(name string) time.Time {
	day := strings.TrimSuffix(name, ".jsonl.gz")
	parsed, _ := time.Parse("2006-01-02", day)

	return parsed
}

// ReadHistory reads current summaries and compacted daily archives.
func ReadHistory(root string, query Query) ([]Summary, error) {
	return readHistory(system.Production, root, query)
}

func readHistory(port system.System, root string, query Query) ([]Summary, error) {
	if query.Now.IsZero() {
		query.Now = port.Clock.Now()
	}
	rows := []Summary{}
	entries, err := port.Files.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
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
		summary, readError := readSummaryFile(port, filepath.Join(root, entry.Name()), info.ModTime())
		if readError != nil {
			return nil, readError
		}
		if domain.MatchesQuery(summary, query) {
			rows = append(rows, summary)
		}
	}
	historyEntries, err := port.Files.ReadDir(filepath.Join(root, "history"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, entry := range historyEntries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".jsonl.gz") {
			continue
		}
		archived, readError := readGzipSummaries(
			port, filepath.Join(root, "history", entry.Name()), archiveFallback(entry.Name()),
		)
		if readError != nil {
			return nil, readError
		}
		for _, summary := range archived {
			if domain.MatchesQuery(summary, query) {
				rows = append(rows, summary)
			}
		}
	}
	sort.Slice(rows, func(left, right int) bool {
		return domain.SummaryTime(rows[left], time.Time{}).Before(domain.SummaryTime(rows[right], time.Time{}))
	})

	return rows, nil
}

// domain.HealthyPromotionSummary reports whether one run counts toward opening the
// optional owner slot. Only a recorded OutcomePassed does: a run that was shed,
// deferred, or recorded under a word this version does not know never counts,
// whatever its budget outcome says, so the budget outcome is not consulted.
func atomicGzipWrite(port system.System, path string, rows [][]byte, modified time.Time) (returnError error) {
	if err := port.Files.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := port.Files.CreateTemp(filepath.Dir(path), ".history-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			returnError = errors.Join(returnError, temporary.Close())
		}
		if removeError := port.Files.Remove(temporaryPath); !errors.Is(removeError, fs.ErrNotExist) {
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
	if err = port.Files.Rename(temporaryPath, path); err != nil {
		return err
	}
	if !modified.IsZero() {
		return port.Files.Chtimes(path, modified, modified)
	}

	return nil
}

func compressRawFile(port system.System, source, destination string, modified time.Time) error {
	data, err := port.Files.ReadFile(source)
	if err != nil {
		return err
	}
	if err = atomicGzipWrite(port, destination, [][]byte{data}, modified); err != nil {
		return err
	}

	return port.Files.Remove(source)
}

func summaryArchiveDay(port system.System, path string, info fs.FileInfo) (time.Time, error) {
	summary, err := readSummaryFile(port, path, info.ModTime())
	if err != nil {
		return time.Time{}, err
	}
	measured := domain.SummaryTime(summary, info.ModTime())

	return time.Date(measured.UTC().Year(), measured.UTC().Month(), measured.UTC().Day(), 0, 0, 0, 0, time.UTC), nil
}

//nolint:gocognit,cyclop // Atomic daily merge stays visibly ordered.
func compactSummaries(
	port system.System, root string, now time.Time, protected map[string]bool,
) error {
	entries, err := port.Files.ReadDir(root)
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
			if err = port.Files.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}

			continue
		}
		day, dayError := summaryArchiveDay(port, path, info)
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
		fallback := archiveFallback(filepath.Base(destination))
		archived, readError := readGzipSummaries(port, destination, fallback)
		if readError == nil {
			for _, summary := range archived {
				// A decoded summary holds only recorded words and finite numbers, so encoding it cannot fail.
				encoded, _ := json.Marshal(summary) //nolint:errchkjson // A decoded summary holds only recorded words and finite numbers.
				rows[summary.RunID] = encoded
			}
		} else if !errors.Is(readError, fs.ErrNotExist) {
			return readError
		}
		for _, path := range paths {
			data, readError := port.Files.ReadFile(path)
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
			// Decoded from one summary, this value encodes for the same reason the archived rows above do.
			encoded, _ := json.Marshal(summary) //nolint:errchkjson // Decoded from one summary, it holds only recorded words and finite numbers.
			rows[key] = encoded
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
		if err = atomicGzipWrite(port, destination, encoded, modified); err != nil {
			return err
		}
		for _, path := range paths {
			if err = port.Files.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	}

	return nil
}

func bytesTrimSpace(data []byte) []byte {
	return []byte(strings.TrimSpace(string(data)))
}

func compactRaw(port system.System, root string, now time.Time, active, preserved map[string]bool) error {
	entries, err := port.Files.ReadDir(root)
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
		if err = compressRawFile(port, path, destination, info.ModTime()); err != nil {
			return err
		}
	}

	return nil
}

//nolint:gocognit // Expiry, temporary handling, accounting, and oldest-first pruning form one directory transaction.
func pruneDirectory(
	port system.System, path string, now time.Time, retention time.Duration, maximumBytes int64,
) error {
	entries, err := port.Files.ReadDir(path)
	if errors.Is(err, fs.ErrNotExist) {
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
				if err = port.Files.Remove(entryPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
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
			if err = port.Files.Remove(entryPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
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
		if err = port.Files.Remove(entry.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		total -= entry.size
	}

	return nil
}

func pruneTemporaryFiles(port system.System, path string, now time.Time) error {
	entries, err := port.Files.ReadDir(path)
	if errors.Is(err, fs.ErrNotExist) {
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
			if err = port.Files.Remove(filepath.Join(path, entry.Name())); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	}

	return nil
}

//nolint:gocognit // The loop must remeasure after each atomic aggregation or removal.
func pruneHistory(port system.System, path string, now time.Time) error {
	if err := pruneDirectory(port, path, now, HistoryRetention, int64(^uint64(0)>>1)); err != nil {
		return err
	}
	for {
		entries, err := port.Files.ReadDir(path)
		if errors.Is(err, fs.ErrNotExist) {
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
		rows, readError := readGzipSummaries(port, oldest.path, oldest.modified)
		if readError != nil {
			return readError
		}
		alreadyAggregated := len(rows) > 0
		for _, row := range rows {
			alreadyAggregated = alreadyAggregated && row.AggregateCount > 0
		}
		if alreadyAggregated {
			if err = port.Files.Remove(oldest.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}

			continue
		}
		aggregates := domain.AggregateHistoryRows(rows)
		encoded := make([][]byte, 0, len(aggregates))
		for _, aggregate := range aggregates {
			// An aggregate takes recorded words and the largest of finite decoded numbers, so encoding it cannot fail.
			row, _ := json.Marshal(aggregate) //nolint:errchkjson // An aggregate takes recorded words and the largest of finite decoded numbers.
			encoded = append(encoded, row)
		}
		if err = atomicGzipWrite(port, oldest.path, encoded, oldest.modified); err != nil {
			return err
		}
	}
}

func compactLocked(port system.System, root string, now time.Time, active, preserved map[string]bool) error {
	protected := map[string]bool{}
	for path := range preserved {
		protected[path] = true
	}
	for prefix := range active {
		protected[prefix] = true
	}
	if err := compactSummaries(port, root, now, protected); err != nil {
		return fmt.Errorf("compact summaries: %w", err)
	}
	if err := compactRaw(port, root, now, active, preserved); err != nil {
		return fmt.Errorf("compact raw evidence: %w", err)
	}
	if err := pruneDirectory(port, filepath.Join(root, "raw"), now, RawRetention, RawMaximumBytes); err != nil {
		return err
	}
	if err := pruneTemporaryFiles(port, filepath.Join(root, "owner-metadata"), now); err != nil {
		return err
	}
	if err := pruneDirectory(port, filepath.Join(root, "receipts"), now, HistoryRetention, HistoryMaxBytes); err != nil {
		return err
	}

	return pruneHistory(port, filepath.Join(root, "history"), now)
}

// CopyGzip expands one compressed raw stream for diagnostic tooling.
func CopyGzip(destination io.Writer, source string) error {
	return copyGzip(system.Production, destination, source)
}

func copyGzip(port system.System, destination io.Writer, source string) error {
	file, err := port.Files.OpenFile(source, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close() //nolint:errcheck // Copy/decompressor errors are returned.
	reader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer reader.Close() //nolint:errcheck // Copy/decompressor errors are returned.
	_, err = io.CopyN(destination, reader, RawMaximumBytes+1)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}

	return errors.New("raw gzip expands beyond the shared evidence cap")
}

// EvaluatePromotion reads retained rows and delegates their pure evaluation.
func EvaluatePromotion(root string, criteria PromotionCriteria, now time.Time) (PromotionEvaluation, error) {
	return evaluatePromotion(system.Production, root, criteria, now)
}

func evaluatePromotion(
	port system.System, root string, criteria PromotionCriteria, now time.Time,
) (PromotionEvaluation, error) {
	rows, err := readHistory(port, root, Query{Since: HistoryRetention, Now: now})
	if err != nil {
		return PromotionEvaluation{}, err
	}
	return domain.EvaluatePromotion(rows, criteria)
}
