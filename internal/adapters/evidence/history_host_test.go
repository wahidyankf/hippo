package evidence //nolint:testpackage // Host-seam tests drive the unexported history and compaction functions that take the system port.

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
	domain "github.com/wahidyankf/hippo/internal/domain/evidence"
)

const (
	redHistoryDay = redRoot + "/history/2026-11-20.jsonl.gz"
	redLooseRun   = redRoot + "/run.summary.json"
)

// redCompactionHost returns a host whose evidence root and its sibling directories exist.
func redCompactionHost() *systemtest.Host {
	host := redHost()
	redDirs(host, redRoot, redRoot+"/history", redRoot+"/raw", redRoot+"/receipts", redRoot+"/owner-metadata")

	return host
}

func TestParseOutcomeReadsOnlyRecordableWords(t *testing.T) {
	if outcome, err := ParseOutcome("passed"); err != nil || outcome != OutcomePassed {
		t.Fatalf("passed parsed as %v with %v", outcome, err)
	}
	if _, err := ParseOutcome("unrecorded-word"); err == nil {
		t.Fatal("an unrecorded outcome word was accepted")
	}
}

func TestReadSummaryFileReportsAReadFailure(t *testing.T) {
	port := redHost().Port()
	port.Files.ReadFile = func(string) ([]byte, error) { return nil, errRed }
	if _, err := readSummaryFile(port, redLooseRun, time.Time{}); !errors.Is(err, errRed) {
		t.Fatalf("summary read error = %v, want the injected failure", err)
	}
}

func TestReadSummaryFileRejectsMalformedJSON(t *testing.T) {
	host := redHost()
	host.Files.Contents[redLooseRun] = []byte("{")
	if _, err := readSummaryFile(host.Port(), redLooseRun, time.Time{}); err == nil {
		t.Fatal("a malformed summary was accepted")
	}
}

func TestReadSummaryFileTakesTheFallbackFinishTimeWhenTheRecordHasNone(t *testing.T) {
	host := redHost()
	host.Files.Contents[redLooseRun] = []byte(`{"schemaVersion":5,"runId":"run"}`)
	fallback := redStart.Add(-time.Hour)
	summary, err := readSummaryFile(host.Port(), redLooseRun, fallback)
	if err != nil || summary.FinishedAt != fallback.Format(time.RFC3339Nano) {
		t.Fatalf("finish time = %q with %v, want the fallback", summary.FinishedAt, err)
	}
}

func TestReadGzipSummariesReportsAnOpenFailure(t *testing.T) {
	port := redHost().Port()
	port.Files.OpenFile = func(string, int, fs.FileMode) (system.File, error) { return nil, errRed }
	if _, err := readGzipSummaries(port, redHistoryDay, time.Time{}); !errors.Is(err, errRed) {
		t.Fatalf("archive open error = %v, want the injected failure", err)
	}
}

func TestReadGzipSummariesRejectsAnUnreadableRecord(t *testing.T) {
	host := redHost()
	host.Files.Contents[redHistoryDay] = redGzip(t, []byte("{"))
	if _, err := readGzipSummaries(host.Port(), redHistoryDay, time.Time{}); err == nil {
		t.Fatal("an unreadable archive record was accepted")
	}
}

func TestReadGzipSummariesTakesTheArchiveDayWhenARecordHasNoFinishTime(t *testing.T) {
	host := redHost()
	day := time.Date(2026, time.November, 20, 0, 0, 0, 0, time.UTC)
	host.Files.Contents[redHistoryDay] = redGzip(t, []byte(`{"schemaVersion":5,"runId":"run"}`))
	rows, err := readGzipSummaries(host.Port(), redHistoryDay, day)
	if err != nil || len(rows) != 1 || rows[0].FinishedAt != day.Format(time.RFC3339Nano) {
		t.Fatalf("archive rows = %+v with %v, want the archive day as the finish time", rows, err)
	}
}

func TestReadHistoryUsesTheHostClockWhenTheQueryHasNoNow(t *testing.T) {
	host := redHost()
	redDirs(host, redRoot)
	redPlace(host, redLooseRun, redMarshal(t, redSummary("run", redStart.Add(-time.Hour))), redStart)
	query := Query{Since: HistoryRetention}

	rows, err := readHistory(host.Port(), redRoot, query)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows at the host clock = %d with %v, want the recent run", len(rows), err)
	}
	host.Clock.Current = redStart.Add(60 * 24 * time.Hour)
	rows, err = readHistory(host.Port(), redRoot, query)
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows after sixty days = %d with %v, want none inside the window", len(rows), err)
	}
}

func TestReadHistoryReturnsNoRowsWhenTheRootIsMissing(t *testing.T) {
	rows, err := readHistory(redHost().Port(), "/hippo/missing", Query{Now: redStart})
	if err != nil || len(rows) != 0 {
		t.Fatalf("missing root read as %d rows with %v", len(rows), err)
	}
}

func TestReadHistoryReportsAListingFailure(t *testing.T) {
	host := redCompactionHost()
	port := host.Port()
	redReadDirFails(&port, redRoot, errRed)
	if _, err := readHistory(port, redRoot, Query{Now: redStart}); !errors.Is(err, errRed) {
		t.Fatalf("root listing error = %v, want the injected failure", err)
	}
}

func TestReadHistoryReportsAnEntryInfoFailure(t *testing.T) {
	host := redCompactionHost()
	redPlace(host, redLooseRun, redMarshal(t, redSummary("run", redStart)), redStart)
	port := host.Port()
	redEntryInfo(&port, redLooseRun, func(fs.DirEntry) (fs.FileInfo, error) { return nil, errRed })
	if _, err := readHistory(port, redRoot, Query{Now: redStart}); !errors.Is(err, errRed) {
		t.Fatalf("entry info error = %v, want the injected failure", err)
	}
}

func TestReadHistoryReportsAnUnreadableSummary(t *testing.T) {
	host := redCompactionHost()
	redPlace(host, redLooseRun, redMarshal(t, redSummary("run", redStart)), redStart)
	port := host.Port()
	redReadFile(&port, redLooseRun, 0, nil, errRed)
	if _, err := readHistory(port, redRoot, Query{Now: redStart}); !errors.Is(err, errRed) {
		t.Fatalf("summary read error = %v, want the injected failure", err)
	}
}

func TestReadHistoryReportsAHistoryListingFailure(t *testing.T) {
	host := redCompactionHost()
	port := host.Port()
	redReadDirFails(&port, redRoot+"/history", errRed)
	if _, err := readHistory(port, redRoot, Query{Now: redStart}); !errors.Is(err, errRed) {
		t.Fatalf("history listing error = %v, want the injected failure", err)
	}
}

func TestReadHistorySkipsHistoryEntriesThatAreNotArchives(t *testing.T) {
	host := redCompactionHost()
	archive := redRoot + "/history/2026-11-20.jsonl.gz"
	host.Files.Contents[archive] = redGzip(t, redMarshal(t, redSummary("archived", redStart.Add(-48*time.Hour))))
	host.Files.Contents[redRoot+"/history/notes.txt"] = []byte("not an archive")
	host.Files.Dirs[redRoot+"/history/nested"] = true

	rows, err := readHistory(host.Port(), redRoot, Query{Since: HistoryRetention, Now: redStart})
	if err != nil || len(rows) != 1 || rows[0].RunID != "archived" {
		t.Fatalf("rows = %+v with %v, want only the archived run", rows, err)
	}
}

func TestAtomicGzipWriteReportsAMakeDirectoryFailure(t *testing.T) {
	port := redHost().Port()
	port.Files.MkdirAll = func(string, fs.FileMode) error { return errRed }
	if err := atomicGzipWrite(port, redHistoryDay, nil, time.Time{}); !errors.Is(err, errRed) {
		t.Fatalf("directory error = %v, want the injected failure", err)
	}
}

func TestAtomicGzipWriteReportsATemporaryCreationFailure(t *testing.T) {
	port := redHost().Port()
	port.Files.CreateTemp = func(string, string) (system.File, error) { return nil, errRed }
	if err := atomicGzipWrite(port, redHistoryDay, nil, time.Time{}); !errors.Is(err, errRed) {
		t.Fatalf("temporary error = %v, want the injected failure", err)
	}
}

func TestAtomicGzipWriteRemovesItsTemporaryWhenChmodFails(t *testing.T) {
	host := redHost()
	port := host.Port()
	redWrapOpenFiles(&port, redFileFaults{chmod: errRed})
	if err := atomicGzipWrite(port, redHistoryDay, nil, time.Time{}); !errors.Is(err, errRed) {
		t.Fatalf("chmod error = %v, want the injected failure", err)
	}
	for name := range host.Files.Contents {
		if strings.HasPrefix(name, redRoot+"/history/.history-") {
			t.Fatalf("temporary %s survived the failed write", name)
		}
	}
}

func TestAtomicGzipWriteJoinsATemporaryRemovalFailure(t *testing.T) {
	host := redHost()
	port := host.Port()
	removeErr := errors.New("temporary removal refused")
	redWrapOpenFiles(&port, redFileFaults{chmod: errRed})
	port.Files.Remove = func(string) error { return removeErr }
	err := atomicGzipWrite(port, redHistoryDay, nil, time.Time{})
	if !errors.Is(err, errRed) || !errors.Is(err, removeErr) {
		t.Fatalf("write error = %v, want both the chmod and the removal failures", err)
	}
}

func TestAtomicGzipWriteReportsAWriteFailure(t *testing.T) {
	port := redHost().Port()
	redWrapOpenFiles(&port, redFileFaults{writeFrom: 1})
	if err := atomicGzipWrite(port, redHistoryDay, [][]byte{[]byte("{}")}, time.Time{}); !errors.Is(err, errRed) {
		t.Fatalf("write error = %v, want the injected failure", err)
	}
}

func TestAtomicGzipWriteReportsACompressorCloseFailure(t *testing.T) {
	port := redHost().Port()
	redWrapOpenFiles(&port, redFileFaults{writeFrom: 2})
	if err := atomicGzipWrite(port, redHistoryDay, [][]byte{[]byte("{}")}, time.Time{}); !errors.Is(err, errRed) {
		t.Fatalf("compressor close error = %v, want the injected failure", err)
	}
}

func TestAtomicGzipWriteReportsSyncAndCloseFailures(t *testing.T) {
	tests := []struct {
		name   string
		faults redFileFaults
	}{
		{name: "sync", faults: redFileFaults{sync: errRed}},
		{name: "close", faults: redFileFaults{closeErr: errRed}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			port := redHost().Port()
			redWrapOpenFiles(&port, test.faults)
			if err := atomicGzipWrite(port, redHistoryDay, nil, time.Time{}); !errors.Is(err, errRed) {
				t.Fatalf("%s error = %v, want the injected failure", test.name, err)
			}
		})
	}
}

func TestAtomicGzipWriteReportsARenameFailure(t *testing.T) {
	port := redHost().Port()
	port.Files.Rename = func(string, string) error { return errRed }
	if err := atomicGzipWrite(port, redHistoryDay, nil, time.Time{}); !errors.Is(err, errRed) {
		t.Fatalf("rename error = %v, want the injected failure", err)
	}
}

func TestAtomicGzipWriteSetsTheModificationTimeOnlyWhenOneIsGiven(t *testing.T) {
	host := redHost()
	if err := atomicGzipWrite(host.Port(), redHistoryDay, nil, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, stamped := host.Files.Modified[redHistoryDay]; stamped {
		t.Fatal("a zero modification time stamped the archive")
	}
	modified := redStart.Add(-24 * time.Hour)
	if err := atomicGzipWrite(host.Port(), redHistoryDay, nil, modified); err != nil {
		t.Fatal(err)
	}
	if !host.Files.Modified[redHistoryDay].Equal(modified) {
		t.Fatalf("archive stamped %v, want %v", host.Files.Modified[redHistoryDay], modified)
	}
}

func TestAtomicGzipWriteReportsAModificationTimeFailure(t *testing.T) {
	port := redHost().Port()
	port.Files.Chtimes = func(string, time.Time, time.Time) error { return errRed }
	if err := atomicGzipWrite(port, redHistoryDay, nil, redStart); !errors.Is(err, errRed) {
		t.Fatalf("chtimes error = %v, want the injected failure", err)
	}
}

func TestCompressRawFileReportsAReadFailure(t *testing.T) {
	host := redCompactionHost()
	host.Files.Contents[redRoot+"/run.jsonl"] = []byte("{}\n")
	port := host.Port()
	port.Files.ReadFile = func(string) ([]byte, error) { return nil, errRed }
	if err := compressRawFile(port, redRoot+"/run.jsonl", redRoot+"/raw/run.jsonl.gz", redStart); !errors.Is(err, errRed) {
		t.Fatalf("raw read error = %v, want the injected failure", err)
	}
}

func TestCompressRawFileReportsACompressionFailure(t *testing.T) {
	host := redCompactionHost()
	host.Files.Contents[redRoot+"/run.jsonl"] = []byte("{}\n")
	port := host.Port()
	port.Files.MkdirAll = func(string, fs.FileMode) error { return errRed }
	if err := compressRawFile(port, redRoot+"/run.jsonl", redRoot+"/raw/run.jsonl.gz", redStart); !errors.Is(err, errRed) {
		t.Fatalf("compression error = %v, want the injected failure", err)
	}
}

func TestSummaryArchiveDayReportsAnUnreadableSummary(t *testing.T) {
	host := redCompactionHost()
	host.Files.Contents[redLooseRun] = []byte("{")
	info, err := host.Port().Files.Stat(redLooseRun)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = summaryArchiveDay(host.Port(), redLooseRun, info); err == nil {
		t.Fatal("an unreadable summary had an archive day")
	}
}

func TestCompactSummariesReportsAListingFailure(t *testing.T) {
	host := redCompactionHost()
	port := host.Port()
	redReadDirFails(&port, redRoot, errRed)
	if err := compactSummaries(port, redRoot, redStart, map[string]bool{}); !errors.Is(err, errRed) {
		t.Fatalf("listing error = %v, want the injected failure", err)
	}
}

func TestCompactSummariesLeavesProtectedSummariesAlone(t *testing.T) {
	host := redCompactionHost()
	protected := redRoot + "/keep.summary.json"
	host.Files.Contents[protected] = []byte("{")
	if err := compactSummaries(host.Port(), redRoot, redStart, map[string]bool{protected: true}); err != nil {
		t.Fatal(err)
	}
	if _, kept := host.Files.Contents[protected]; !kept {
		t.Fatal("a protected summary was compacted")
	}
}

func TestCompactSummariesReportsAnEntryInfoFailure(t *testing.T) {
	host := redCompactionHost()
	redPlace(host, redLooseRun, redMarshal(t, redSummary("run", redStart)), redStart)
	port := host.Port()
	redEntryInfo(&port, redLooseRun, func(fs.DirEntry) (fs.FileInfo, error) { return nil, errRed })
	if err := compactSummaries(port, redRoot, redStart, map[string]bool{}); !errors.Is(err, errRed) {
		t.Fatalf("entry info error = %v, want the injected failure", err)
	}
}

func TestCompactSummariesReportsAFailedRemovalOfAnExpiredSummary(t *testing.T) {
	host := redCompactionHost()
	expired := redStart.Add(-(domain.HistoryRetention + time.Hour))
	redPlace(host, redLooseRun, redMarshal(t, redSummary("run", expired)), expired)
	port := host.Port()
	port.Files.Remove = func(string) error { return errRed }
	if err := compactSummaries(port, redRoot, redStart, map[string]bool{}); !errors.Is(err, errRed) {
		t.Fatalf("expired removal error = %v, want the injected failure", err)
	}
}

func TestCompactSummariesReportsAnUnreadableRecentSummary(t *testing.T) {
	host := redCompactionHost()
	redPlace(host, redLooseRun, []byte("{"), redStart)
	if err := compactSummaries(host.Port(), redRoot, redStart, map[string]bool{}); err == nil {
		t.Fatal("an unreadable recent summary was compacted")
	}
}

func TestCompactSummariesMergesIntoAnExistingArchive(t *testing.T) {
	host := redCompactionHost()
	day := time.Date(2026, time.November, 20, 0, 0, 0, 0, time.UTC)
	host.Files.Contents[redHistoryDay] = redGzip(t, redMarshal(t, redSummary("old", day.Add(time.Hour))))
	loose := redRoot + "/new.summary.json"
	redPlace(host, loose, redMarshal(t, redSummary("new", day.Add(2*time.Hour))), day.Add(2*time.Hour))

	if err := compactSummaries(host.Port(), redRoot, redStart, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	if _, exists := host.Files.Contents[loose]; exists {
		t.Fatal("the compacted summary was not removed")
	}
	rows, err := readGzipSummaries(host.Port(), redHistoryDay, time.Time{})
	if err != nil || len(rows) != 2 {
		t.Fatalf("archive rows = %d with %v, want the old and the new run", len(rows), err)
	}
}

func TestCompactSummariesReportsAnArchiveReadFailure(t *testing.T) {
	host := redCompactionHost()
	day := time.Date(2026, time.November, 20, 0, 0, 0, 0, time.UTC)
	host.Files.Contents[redHistoryDay] = redGzip(t, redMarshal(t, redSummary("old", day)))
	redPlace(host, redRoot+"/new.summary.json", redMarshal(t, redSummary("new", day)), day)
	port := host.Port()
	port.Files.OpenFile = func(string, int, fs.FileMode) (system.File, error) { return nil, errRed }
	if err := compactSummaries(port, redRoot, redStart, map[string]bool{}); !errors.Is(err, errRed) {
		t.Fatalf("archive read error = %v, want the injected failure", err)
	}
}

func TestCompactSummariesReportsALooseSummaryReadFailureAfterTheDayIsKnown(t *testing.T) {
	host := redCompactionHost()
	day := time.Date(2026, time.November, 20, 0, 0, 0, 0, time.UTC)
	loose := redRoot + "/new.summary.json"
	redPlace(host, loose, redMarshal(t, redSummary("new", day)), day)
	port := host.Port()
	redReadFile(&port, loose, 2, nil, errRed)
	if err := compactSummaries(port, redRoot, redStart, map[string]bool{}); !errors.Is(err, errRed) {
		t.Fatalf("second read error = %v, want the injected failure", err)
	}
}

func TestCompactSummariesRejectsALooseSummaryThatChangedToMalformedJSON(t *testing.T) {
	host := redCompactionHost()
	day := time.Date(2026, time.November, 20, 0, 0, 0, 0, time.UTC)
	loose := redRoot + "/new.summary.json"
	redPlace(host, loose, redMarshal(t, redSummary("new", day)), day)
	port := host.Port()
	redReadFile(&port, loose, 2, []byte("{"), nil)
	if err := compactSummaries(port, redRoot, redStart, map[string]bool{}); err == nil {
		t.Fatal("a summary that turned malformed between reads was compacted")
	}
}

func TestCompactSummariesReportsAFailedDestinationWrite(t *testing.T) {
	host := redCompactionHost()
	day := time.Date(2026, time.November, 20, 0, 0, 0, 0, time.UTC)
	redPlace(host, redRoot+"/new.summary.json", redMarshal(t, redSummary("new", day)), day)
	port := host.Port()
	port.Files.CreateTemp = func(string, string) (system.File, error) { return nil, errRed }
	if err := compactSummaries(port, redRoot, redStart, map[string]bool{}); !errors.Is(err, errRed) {
		t.Fatalf("destination error = %v, want the injected failure", err)
	}
}

func TestCompactSummariesReportsAFailedRemovalOfACompactedSource(t *testing.T) {
	host := redCompactionHost()
	day := time.Date(2026, time.November, 20, 0, 0, 0, 0, time.UTC)
	loose := redRoot + "/new.summary.json"
	redPlace(host, loose, redMarshal(t, redSummary("new", day)), day)
	port := host.Port()
	port.Files.Remove = func(name string) error {
		if name == loose {
			return errRed
		}

		return nil
	}
	if err := compactSummaries(port, redRoot, redStart, map[string]bool{}); !errors.Is(err, errRed) {
		t.Fatalf("source removal error = %v, want the injected failure", err)
	}
}

func TestCompactSummariesKeysAnonymousSummariesByTheirContentHash(t *testing.T) {
	host := redCompactionHost()
	day := time.Date(2026, time.November, 20, 0, 0, 0, 0, time.UTC)
	anonymous := redSummary("", day)
	redPlace(host, redRoot+"/anonymous.summary.json", redMarshal(t, anonymous), day)
	if err := compactSummaries(host.Port(), redRoot, redStart, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	rows, err := readGzipSummaries(host.Port(), redHistoryDay, time.Time{})
	if err != nil || len(rows) != 1 {
		t.Fatalf("archive rows = %d with %v, want the one anonymous run", len(rows), err)
	}
}

func TestCompactRawReportsAListingFailure(t *testing.T) {
	host := redCompactionHost()
	port := host.Port()
	redReadDirFails(&port, redRoot, errRed)
	if err := compactRaw(port, redRoot, redStart, map[string]bool{}, map[string]bool{}); !errors.Is(err, errRed) {
		t.Fatalf("listing error = %v, want the injected failure", err)
	}
}

func TestCompactRawReportsAnEntryInfoFailure(t *testing.T) {
	host := redCompactionHost()
	redPlace(host, redRoot+"/run.jsonl", []byte("{}\n"), redStart.Add(-48*time.Hour))
	port := host.Port()
	redEntryInfo(&port, redRoot+"/run.jsonl", func(fs.DirEntry) (fs.FileInfo, error) { return nil, errRed })
	if err := compactRaw(port, redRoot, redStart, map[string]bool{}, map[string]bool{}); !errors.Is(err, errRed) {
		t.Fatalf("entry info error = %v, want the injected failure", err)
	}
}

func TestCompactRawReportsAPriorDayCompressionFailure(t *testing.T) {
	host := redCompactionHost()
	redPlace(host, redRoot+"/run.jsonl", []byte("{}\n"), redStart.Add(-48*time.Hour))
	port := host.Port()
	redReadFile(&port, redRoot+"/run.jsonl", 0, nil, errRed)
	if err := compactRaw(port, redRoot, redStart, map[string]bool{}, map[string]bool{}); !errors.Is(err, errRed) {
		t.Fatalf("compression error = %v, want the injected failure", err)
	}
}

func TestPruneDirectoryReportsAListingFailure(t *testing.T) {
	host := redCompactionHost()
	port := host.Port()
	redReadDirFails(&port, redRoot+"/receipts", errRed)
	if err := pruneDirectory(port, redRoot+"/receipts", redStart, domain.HistoryRetention, 1); !errors.Is(err, errRed) {
		t.Fatalf("listing error = %v, want the injected failure", err)
	}
}

func TestPruneDirectorySkipsEntriesThatAreNotRegularFiles(t *testing.T) {
	host := redCompactionHost()
	receipts := redRoot + "/receipts"
	host.Files.Dirs[receipts+"/nested"] = true
	if err := pruneDirectory(host.Port(), receipts, redStart, domain.HistoryRetention, 0); err != nil {
		t.Fatal(err)
	}
	if !host.Files.Dirs[receipts+"/nested"] {
		t.Fatal("a nested directory was pruned as a file")
	}
}

func TestPruneDirectoryReportsAnEntryInfoFailure(t *testing.T) {
	host := redCompactionHost()
	receipts := redRoot + "/receipts"
	redPlace(host, receipts+"/receipt.json", []byte("{}"), redStart)
	port := host.Port()
	redEntryInfo(&port, receipts+"/receipt.json", func(fs.DirEntry) (fs.FileInfo, error) { return nil, errRed })
	if err := pruneDirectory(port, receipts, redStart, domain.HistoryRetention, 1<<20); !errors.Is(err, errRed) {
		t.Fatalf("entry info error = %v, want the injected failure", err)
	}
}

func TestPruneDirectoryRemovesOldTemporariesAndKeepsFreshOnes(t *testing.T) {
	host := redCompactionHost()
	receipts := redRoot + "/receipts"
	old := redStart.Add(-2 * time.Hour)
	redPlace(host, receipts+"/.old.tmp", []byte("x"), old)
	redPlace(host, receipts+"/.fresh.tmp", []byte("x"), redStart.Add(-time.Minute))
	if err := pruneDirectory(host.Port(), receipts, redStart, domain.HistoryRetention, 1<<20); err != nil {
		t.Fatal(err)
	}
	if _, stale := host.Files.Contents[receipts+"/.old.tmp"]; stale {
		t.Fatal("an expired temporary survived pruning")
	}
	if _, fresh := host.Files.Contents[receipts+"/.fresh.tmp"]; !fresh {
		t.Fatal("a fresh temporary was pruned")
	}
}

func TestPruneDirectoryReportsAFailedTemporaryRemoval(t *testing.T) {
	host := redCompactionHost()
	receipts := redRoot + "/receipts"
	redPlace(host, receipts+"/.old.tmp", []byte("x"), redStart.Add(-2*time.Hour))
	port := host.Port()
	port.Files.Remove = func(string) error { return errRed }
	if err := pruneDirectory(port, receipts, redStart, domain.HistoryRetention, 1<<20); !errors.Is(err, errRed) {
		t.Fatalf("temporary removal error = %v, want the injected failure", err)
	}
}

func TestPruneDirectoryReportsAFailedExpiredRemoval(t *testing.T) {
	host := redCompactionHost()
	receipts := redRoot + "/receipts"
	redPlace(host, receipts+"/old.json", []byte("x"), redStart.Add(-(domain.HistoryRetention + time.Hour)))
	port := host.Port()
	port.Files.Remove = func(string) error { return errRed }
	if err := pruneDirectory(port, receipts, redStart, domain.HistoryRetention, 1<<20); !errors.Is(err, errRed) {
		t.Fatalf("expired removal error = %v, want the injected failure", err)
	}
}

func TestPruneDirectoryRemovesOldestFilesUntilUnderTheCap(t *testing.T) {
	host := redCompactionHost()
	receipts := redRoot + "/receipts"
	redPlace(host, receipts+"/a.json", []byte("aa"), redStart.Add(-3*time.Hour))
	redPlace(host, receipts+"/b.json", []byte("bb"), redStart.Add(-2*time.Hour))
	if err := pruneDirectory(host.Port(), receipts, redStart, domain.HistoryRetention, 2); err != nil {
		t.Fatal(err)
	}
	if _, oldest := host.Files.Contents[receipts+"/a.json"]; oldest {
		t.Fatal("the oldest file survived the cap")
	}
	if _, newer := host.Files.Contents[receipts+"/b.json"]; !newer {
		t.Fatal("the newer file was pruned while the directory was under the cap")
	}
}

func TestPruneDirectoryReportsAFailedCapRemoval(t *testing.T) {
	host := redCompactionHost()
	receipts := redRoot + "/receipts"
	redPlace(host, receipts+"/a.json", []byte("aa"), redStart.Add(-3*time.Hour))
	redPlace(host, receipts+"/b.json", []byte("bb"), redStart.Add(-2*time.Hour))
	port := host.Port()
	port.Files.Remove = func(string) error { return errRed }
	if err := pruneDirectory(port, receipts, redStart, domain.HistoryRetention, 2); !errors.Is(err, errRed) {
		t.Fatalf("cap removal error = %v, want the injected failure", err)
	}
}

func TestPruneTemporaryFilesReportsAListingFailure(t *testing.T) {
	host := redCompactionHost()
	port := host.Port()
	redReadDirFails(&port, redRoot+"/owner-metadata", errRed)
	if err := pruneTemporaryFiles(port, redRoot+"/owner-metadata", redStart); !errors.Is(err, errRed) {
		t.Fatalf("listing error = %v, want the injected failure", err)
	}
}

func TestPruneTemporaryFilesIgnoresOrdinaryFilesAndDirectories(t *testing.T) {
	host := redCompactionHost()
	metadata := redRoot + "/owner-metadata"
	redPlace(host, metadata+"/record.json", []byte("{}"), redStart.Add(-2*time.Hour))
	host.Files.Dirs[metadata+"/.hidden-dir"] = true
	if err := pruneTemporaryFiles(host.Port(), metadata, redStart); err != nil {
		t.Fatal(err)
	}
	if _, kept := host.Files.Contents[metadata+"/record.json"]; !kept {
		t.Fatal("an ordinary file was pruned as a temporary")
	}
}

func TestPruneTemporaryFilesReportsAnEntryInfoFailure(t *testing.T) {
	host := redCompactionHost()
	metadata := redRoot + "/owner-metadata"
	redPlace(host, metadata+"/.owner-metadata-1.tmp", []byte("x"), redStart)
	port := host.Port()
	redEntryInfo(&port, metadata+"/.owner-metadata-1.tmp", func(fs.DirEntry) (fs.FileInfo, error) { return nil, errRed })
	if err := pruneTemporaryFiles(port, metadata, redStart); !errors.Is(err, errRed) {
		t.Fatalf("entry info error = %v, want the injected failure", err)
	}
}

func TestPruneTemporaryFilesRemovesOnlyExpiredTemporaries(t *testing.T) {
	host := redCompactionHost()
	metadata := redRoot + "/owner-metadata"
	redPlace(host, metadata+"/.owner-metadata-old.tmp", []byte("x"), redStart.Add(-2*time.Hour))
	redPlace(host, metadata+"/.owner-metadata-new.tmp", []byte("x"), redStart)
	if err := pruneTemporaryFiles(host.Port(), metadata, redStart); err != nil {
		t.Fatal(err)
	}
	if _, stale := host.Files.Contents[metadata+"/.owner-metadata-old.tmp"]; stale {
		t.Fatal("an expired temporary survived")
	}
	if _, fresh := host.Files.Contents[metadata+"/.owner-metadata-new.tmp"]; !fresh {
		t.Fatal("a fresh temporary was removed")
	}
}

func TestPruneTemporaryFilesReportsAFailedRemoval(t *testing.T) {
	host := redCompactionHost()
	metadata := redRoot + "/owner-metadata"
	redPlace(host, metadata+"/.owner-metadata-old.tmp", []byte("x"), redStart.Add(-2*time.Hour))
	port := host.Port()
	port.Files.Remove = func(string) error { return errRed }
	if err := pruneTemporaryFiles(port, metadata, redStart); !errors.Is(err, errRed) {
		t.Fatalf("removal error = %v, want the injected failure", err)
	}
}

func TestPruneHistoryReportsAFailedDirectoryPrune(t *testing.T) {
	host := redCompactionHost()
	port := host.Port()
	redReadDirFails(&port, redRoot+"/history", errRed)
	if err := pruneHistory(port, redRoot+"/history", redStart); !errors.Is(err, errRed) {
		t.Fatalf("history prune error = %v, want the injected failure", err)
	}
}

func TestPruneHistoryReportsASecondListingFailure(t *testing.T) {
	host := redCompactionHost()
	port := host.Port()
	redListing(&port, redRoot+"/history", 2, func([]fs.DirEntry) ([]fs.DirEntry, error) { return nil, errRed })
	if err := pruneHistory(port, redRoot+"/history", redStart); !errors.Is(err, errRed) {
		t.Fatalf("second listing error = %v, want the injected failure", err)
	}
}

func TestPruneHistoryReportsAnArchiveInfoFailureAfterTheFirstListing(t *testing.T) {
	host := redCompactionHost()
	archive := redRoot + "/history/2026-11-20.jsonl.gz"
	host.Files.Contents[archive] = redGzip(t, redMarshal(t, redSummary("old", redStart)))
	port := host.Port()
	redListing(&port, redRoot+"/history", 2, func(entries []fs.DirEntry) ([]fs.DirEntry, error) {
		for index, entry := range entries {
			entries[index] = redEntry{DirEntry: entry, info: func(fs.DirEntry) (fs.FileInfo, error) { return nil, errRed }}
		}

		return entries, nil
	})
	if err := pruneHistory(port, redRoot+"/history", redStart); !errors.Is(err, errRed) {
		t.Fatalf("archive info error = %v, want the injected failure", err)
	}
}

func TestPruneHistoryKeepsNonArchiveEntries(t *testing.T) {
	host := redCompactionHost()
	history := redRoot + "/history"
	host.Files.Contents[history+"/notes.txt"] = []byte("not an archive")
	host.Files.Dirs[history+"/nested"] = true
	if err := pruneHistory(host.Port(), history, redStart); err != nil {
		t.Fatal(err)
	}
	if _, kept := host.Files.Contents[history+"/notes.txt"]; !kept {
		t.Fatal("a non-archive history entry was pruned")
	}
}

func TestPruneHistoryRemovesAnAlreadyAggregatedOversizeArchive(t *testing.T) {
	host := redCompactionHost()
	history := redRoot + "/history"
	archive := history + "/2026-11-20.jsonl.gz"
	aggregated := domain.Summary{SchemaVersion: 5, RunID: "aggregate", AggregateCount: 3}
	host.Files.Contents[archive] = redGzip(t, redMarshal(t, aggregated))
	port := host.Port()
	redEntrySize(&port, archive, domain.HistoryMaxBytes+1)
	if err := pruneHistory(port, history, redStart); err != nil {
		t.Fatal(err)
	}
	if _, survived := host.Files.Contents[archive]; survived {
		t.Fatal("an aggregated archive over the cap survived pruning")
	}
}

func TestPruneHistoryReportsAnUnreadableOversizeArchive(t *testing.T) {
	host := redCompactionHost()
	history := redRoot + "/history"
	archive := history + "/2026-11-20.jsonl.gz"
	host.Files.Contents[archive] = []byte("not gzip")
	port := host.Port()
	redEntrySize(&port, archive, domain.HistoryMaxBytes+1)
	if err := pruneHistory(port, history, redStart); err == nil {
		t.Fatal("an unreadable oversize archive was pruned")
	}
}

func TestPruneHistoryAggregatesAnOversizeArchiveBeforeItsRemoval(t *testing.T) {
	host := redCompactionHost()
	history := redRoot + "/history"
	archive := history + "/2026-11-20.jsonl.gz"
	host.Files.Contents[archive] = redGzip(t, redMarshal(t, redSummary("raw", redStart)))
	port := host.Port()
	redEntrySize(&port, archive, domain.HistoryMaxBytes+1)
	if err := pruneHistory(port, history, redStart); err != nil {
		t.Fatal(err)
	}
	if _, survived := host.Files.Contents[archive]; survived {
		t.Fatal("the oversize archive survived aggregation and pruning")
	}
}

func TestPruneHistoryReportsAFailedAggregateWrite(t *testing.T) {
	host := redCompactionHost()
	history := redRoot + "/history"
	archive := history + "/2026-11-20.jsonl.gz"
	host.Files.Contents[archive] = redGzip(t, redMarshal(t, redSummary("raw", redStart)))
	port := host.Port()
	redEntrySize(&port, archive, domain.HistoryMaxBytes+1)
	port.Files.CreateTemp = func(string, string) (system.File, error) { return nil, errRed }
	if err := pruneHistory(port, history, redStart); !errors.Is(err, errRed) {
		t.Fatalf("aggregate write error = %v, want the injected failure", err)
	}
}

func TestCompactLockedReportsEachStageFailure(t *testing.T) {
	stages := []struct {
		name  string
		setup func(host *systemtest.Host, port *system.System)
	}{
		{name: "summaries", setup: func(_ *systemtest.Host, port *system.System) { redReadDirFails(port, redRoot, errRed) }},
		{name: "raw", setup: func(host *systemtest.Host, port *system.System) {
			redPlace(host, redRoot+"/run.jsonl", []byte("{}\n"), redStart.Add(-48*time.Hour))
			redReadFile(port, redRoot+"/run.jsonl", 0, nil, errRed)
		}},
		{name: "raw directory", setup: func(_ *systemtest.Host, port *system.System) { redReadDirFails(port, redRoot+"/raw", errRed) }},
		{name: "owner metadata", setup: func(_ *systemtest.Host, port *system.System) {
			redReadDirFails(port, redRoot+"/owner-metadata", errRed)
		}},
		{name: "receipts", setup: func(_ *systemtest.Host, port *system.System) { redReadDirFails(port, redRoot+"/receipts", errRed) }},
	}
	for _, stage := range stages {
		t.Run(stage.name, func(t *testing.T) {
			host := redCompactionHost()
			port := host.Port()
			stage.setup(host, &port)
			if err := compactLocked(port, redRoot, redStart, map[string]bool{}, map[string]bool{}); !errors.Is(err, errRed) {
				t.Fatalf("%s stage error = %v, want the injected failure", stage.name, err)
			}
		})
	}
}

func TestCopyGzipReportsAnOpenFailure(t *testing.T) {
	port := redHost().Port()
	port.Files.OpenFile = func(string, int, fs.FileMode) (system.File, error) { return nil, errRed }
	if err := copyGzip(port, io.Discard, redRoot+"/run.jsonl.gz"); !errors.Is(err, errRed) {
		t.Fatalf("open error = %v, want the injected failure", err)
	}
}

func TestCopyGzipRejectsAStreamThatIsNotGzip(t *testing.T) {
	host := redCompactionHost()
	host.Files.Contents[redRoot+"/run.jsonl.gz"] = []byte("plain text")
	if err := copyGzip(host.Port(), io.Discard, redRoot+"/run.jsonl.gz"); err == nil {
		t.Fatal("a stream that is not gzip was copied")
	}
}

func TestCopyGzipRejectsATruncatedStream(t *testing.T) {
	host := redCompactionHost()
	whole := redGzip(t, []byte(strings.Repeat("x", 4096)))
	host.Files.Contents[redRoot+"/run.jsonl.gz"] = whole[:len(whole)/2]
	if err := copyGzip(host.Port(), io.Discard, redRoot+"/run.jsonl.gz"); err == nil {
		t.Fatal("a truncated stream was copied")
	}
}

func TestCopyGzipRejectsOutputBeyondTheRawCap(t *testing.T) {
	host := redCompactionHost()
	var stream bytes.Buffer
	writer := gzip.NewWriter(&stream)
	zeros := make([]byte, 1<<20)
	for written := int64(0); written <= domain.RawMaximumBytes; written += int64(len(zeros)) {
		if _, err := writer.Write(zeros); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	host.Files.Contents[redRoot+"/run.jsonl.gz"] = stream.Bytes()
	if err := copyGzip(host.Port(), io.Discard, redRoot+"/run.jsonl.gz"); err == nil {
		t.Fatal("output beyond the raw cap was copied")
	}
}

func TestCopyGzipWritesTheWholeStream(t *testing.T) {
	host := redCompactionHost()
	host.Files.Contents[redRoot+"/run.jsonl.gz"] = redGzip(t, []byte("line"))
	var destination bytes.Buffer
	if err := copyGzip(host.Port(), &destination, redRoot+"/run.jsonl.gz"); err != nil {
		t.Fatal(err)
	}
	if destination.String() != "line\n" {
		t.Fatalf("copied %q, want the decompressed line", destination.String())
	}
}

func TestEvaluatePromotionReportsAHistoryFailure(t *testing.T) {
	host := redCompactionHost()
	port := host.Port()
	redReadDirFails(&port, redRoot, errRed)
	if _, err := evaluatePromotion(port, redRoot, domain.PromotionCriteria{}, redStart); !errors.Is(err, errRed) {
		t.Fatalf("promotion history error = %v, want the injected failure", err)
	}
}

func TestPruneHistorySortsMultipleOversizeArchives(t *testing.T) {
	host := redCompactionHost()
	history := redRoot + "/history"
	oldest := history + "/2026-11-20.jsonl.gz"
	newest := history + "/2026-11-21.jsonl.gz"
	host.Files.Contents[oldest] = redGzip(t, redMarshal(t, redSummary("old", redStart)))
	host.Files.Contents[newest] = redGzip(t, redMarshal(t, redSummary("new", redStart)))
	port := host.Port()
	redEntrySize(&port, oldest, domain.HistoryMaxBytes+1)
	redEntrySize(&port, newest, domain.HistoryMaxBytes+1)
	if err := pruneHistory(port, history, redStart); err != nil {
		t.Fatal(err)
	}
	if _, survived := host.Files.Contents[oldest]; survived {
		t.Fatal("the oldest oversize archive survived pruning")
	}
	if _, survived := host.Files.Contents[newest]; survived {
		t.Fatal("the newest oversize archive survived pruning")
	}
}

func TestPruneHistoryReportsAFailedRemovalOfAnAggregatedArchive(t *testing.T) {
	host := redCompactionHost()
	history := redRoot + "/history"
	archive := history + "/2026-11-20.jsonl.gz"
	aggregated := domain.Summary{SchemaVersion: 5, RunID: "aggregate", AggregateCount: 3}
	host.Files.Contents[archive] = redGzip(t, redMarshal(t, aggregated))
	port := host.Port()
	redEntrySize(&port, archive, domain.HistoryMaxBytes+1)
	port.Files.Remove = func(string) error { return errRed }
	if err := pruneHistory(port, history, redStart); !errors.Is(err, errRed) {
		t.Fatalf("aggregate removal error = %v, want the injected failure", err)
	}
}
