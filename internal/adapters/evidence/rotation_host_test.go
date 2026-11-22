package evidence //nolint:testpackage // Host-seam tests drive the unexported writer and lock functions that take the system port.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
)

const (
	redChunk       = redRoot + "/run.jsonl"
	redChunkMarker = redRoot + "/run.active.json"
	redFirstRotate = redRoot + "/run.1.jsonl"
	redLock        = redRoot + "/.writers.lock"
	// redRotating is a sample that crosses a 32-byte chunk after one small sample.
	redRotating = "bbbbbbbbbbbbbbbbbbbbbbbb"
)

// redChunkLimits gives chunks small enough that a test crosses the rotation boundary with two samples.
func redChunkLimits(chunks int) Limits { return Limits{ChunkBytes: 32, Chunks: chunks} }

// redNewWriter opens a chunk in the host with the given number of retained chunks.
func redNewWriter(t *testing.T, host *systemtest.Host, chunks int) *Writer {
	t.Helper()
	writer, err := newWriter(host.Port(), redChunk, redChunkLimits(chunks))
	if err != nil {
		t.Fatal(err)
	}

	return writer
}

func TestWithWriterLockReportsAnOpenFailure(t *testing.T) {
	port := redHost().Port()
	port.Files.OpenFile = func(string, int, fs.FileMode) (system.File, error) { return nil, errRed }
	ran := false
	if err := withWriterLock(port, redRoot, func() error { ran = true; return nil }); !errors.Is(err, errRed) || ran {
		t.Fatalf("lock open error = %v with operation run %t, want the failure and no operation", err, ran)
	}
}

func TestWithWriterLockReportsAFlockFailure(t *testing.T) {
	port := redHost().Port()
	port.Files.Flock = func(system.File, int) error { return errRed }
	ran := false
	if err := withWriterLock(port, redRoot, func() error { ran = true; return nil }); !errors.Is(err, errRed) || ran {
		t.Fatalf("flock error = %v with operation run %t, want the failure and no operation", err, ran)
	}
}

func TestWithWriterLockRunsTheOperationAndReturnsItsError(t *testing.T) {
	host := redHost()
	ran := false
	if err := withWriterLock(host.Port(), redRoot, func() error { ran = true; return errRed }); !errors.Is(err, errRed) || !ran {
		t.Fatalf("operation error = %v with operation run %t", err, ran)
	}
}

func TestNormalizeLimitsRejectsNonPositiveBounds(t *testing.T) {
	for _, limits := range []Limits{{ChunkBytes: -1, Chunks: 1}, {ChunkBytes: 1, Chunks: -1}} {
		if _, err := normalizeLimits(limits); err == nil {
			t.Fatalf("limits %+v were accepted", limits)
		}
	}
	defaults, err := normalizeLimits(Limits{})
	if err != nil || defaults.ChunkBytes != DefaultLimits().ChunkBytes || defaults.Chunks != DefaultLimits().Chunks {
		t.Fatalf("zero limits normalized to %+v with %v, want the defaults", defaults, err)
	}
}

func TestNewWriterRefusesInvalidLimitsBeforeCreatingAChunk(t *testing.T) {
	host := redHost()
	if _, err := newWriter(host.Port(), redChunk, Limits{ChunkBytes: -1, Chunks: 1}); err == nil {
		t.Fatal("invalid limits opened a writer")
	}
	if _, created := host.Files.Contents[redChunk]; created {
		t.Fatal("a chunk was created for invalid limits")
	}
}

func TestNewWriterReportsADirectoryFailure(t *testing.T) {
	port := redHost().Port()
	port.Files.MkdirAll = func(string, fs.FileMode) error { return errRed }
	if _, err := newWriter(port, redChunk, redChunkLimits(2)); !errors.Is(err, errRed) {
		t.Fatalf("directory error = %v, want the injected failure", err)
	}
}

func TestNewWriterReportsALockListingFailure(t *testing.T) {
	host := redHost()
	port := host.Port()
	redReadDirFails(&port, redRoot, errRed)
	if _, err := newWriter(port, redChunk, redChunkLimits(2)); !errors.Is(err, errRed) {
		t.Fatalf("listing error = %v, want the injected failure", err)
	}
}

func TestNewWriterReportsAnUnreadableLiveMarker(t *testing.T) {
	host := redHost()
	other := redRoot + "/other.active.json"
	host.Files.Contents[other] = []byte(`{"schemaVersion":1,"pid":1}`)
	port := host.Port()
	redReadFile(&port, other, 0, nil, errRed)
	if _, err := newWriter(port, redChunk, redChunkLimits(2)); !errors.Is(err, errRed) {
		t.Fatalf("marker read error = %v, want the injected failure", err)
	}
}

func TestNewWriterRefusesTheTwentyFirstLiveSession(t *testing.T) {
	host := redHost()
	host.Processes.Alive[7] = true
	for index := range maximumLiveWriters {
		host.Files.Contents[fmt.Sprintf("%s/live-%d.active.json", redRoot, index)] = []byte(`{"schemaVersion":1,"pid":7}`)
	}
	if _, err := newWriter(host.Port(), redChunk, redChunkLimits(2)); err == nil {
		t.Fatal("a session beyond the live-writer limit opened")
	}
}

func TestNewWriterReportsAnExclusiveCreateFailure(t *testing.T) {
	port := redHost().Port()
	open := port.Files.OpenFile
	port.Files.OpenFile = func(name string, flag int, perm fs.FileMode) (system.File, error) {
		if name == redChunk {
			return nil, errRed
		}

		return open(name, flag, perm)
	}
	if _, err := newWriter(port, redChunk, redChunkLimits(2)); !errors.Is(err, errRed) {
		t.Fatalf("create error = %v, want the injected failure", err)
	}
}

func TestNewWriterRemovesItsChunkWhenTheMarkerCannotBeWritten(t *testing.T) {
	host := redHost()
	port := host.Port()
	write := port.Files.WriteFile
	port.Files.WriteFile = func(name string, data []byte, perm fs.FileMode) error {
		if name == redChunkMarker {
			return errRed
		}

		return write(name, data, perm)
	}
	if _, err := newWriter(port, redChunk, redChunkLimits(2)); !errors.Is(err, errRed) {
		t.Fatalf("marker error = %v, want the injected failure", err)
	}
	if _, created := host.Files.Contents[redChunk]; created {
		t.Fatal("the chunk survived a failed marker write")
	}
}

func TestNewWriterRecordsTheHostProcessInItsMarker(t *testing.T) {
	host := redHost()
	host.Processes.Pid = 4321
	redNewWriter(t, host, 2)
	if marker := string(host.Files.Contents[redChunkMarker]); !strings.Contains(marker, `"pid":4321`) {
		t.Fatalf("marker = %q, want the host process identifier", marker)
	}
}

func TestAbandonClosesTheChunkAndRemovesItsMarker(t *testing.T) {
	host := redHost()
	writer := redNewWriter(t, host, 2)
	writer.abandon()
	if writer.file != nil {
		t.Fatal("the abandoned writer still held its chunk")
	}
	if _, marked := host.Files.Contents[redChunkMarker]; marked {
		t.Fatal("the abandoned writer left its marker")
	}
}

func TestAppendJSONRotatesAcrossTheChunkBoundary(t *testing.T) {
	host := redHost()
	writer := redNewWriter(t, host, 2)
	if err := writer.AppendJSON("aaaa"); err != nil {
		t.Fatal(err)
	}
	if err := writer.AppendJSON(redRotating); err != nil {
		t.Fatal(err)
	}
	if _, rotated := host.Files.Contents[redFirstRotate]; !rotated {
		t.Fatal("the full chunk was not rotated aside")
	}
}

func TestAppendJSONRejectsAClosedWriter(t *testing.T) {
	var nilWriter *Writer
	if err := nilWriter.AppendJSON("a"); err == nil {
		t.Fatal("a nil writer accepted a sample")
	}
	if err := (&Writer{}).AppendJSON("a"); err == nil {
		t.Fatal("a writer without a chunk accepted a sample")
	}
}

func TestAppendJSONRejectsAValueThatCannotBeEncoded(t *testing.T) {
	writer := redNewWriter(t, redHost(), 2)
	if err := writer.AppendJSON(make(chan int)); err == nil {
		t.Fatal("an unencodable value was appended")
	}
}

func TestAppendJSONRejectsASampleBeyondTheChunk(t *testing.T) {
	writer := redNewWriter(t, redHost(), 2)
	if err := writer.AppendJSON(strings.Repeat("c", 40)); err == nil {
		t.Fatal("a sample larger than the chunk was appended")
	}
}

func TestAppendJSONAbandonsTheWriterWhenRotationFails(t *testing.T) {
	writer := redNewWriter(t, redHost(), 2)
	writer.host.Files.OpenFile = func(string, int, fs.FileMode) (system.File, error) { return nil, errRed }
	if err := writer.AppendJSON("aaaa"); err != nil {
		t.Fatal(err)
	}
	if err := writer.AppendJSON(redRotating); !errors.Is(err, errRed) {
		t.Fatalf("rotation error = %v, want the injected failure", err)
	}
	if writer.file != nil {
		t.Fatal("a writer whose rotation failed kept its chunk")
	}
}

func TestRotateReportsAFailedCloseOfTheCurrentChunk(t *testing.T) {
	writer := redNewWriter(t, redHost(), 2)
	writer.file = &redFile{File: writer.file, faults: redFileFaults{closeErr: errRed}}
	if err := writer.AppendJSON("aaaa"); err != nil {
		t.Fatal(err)
	}
	if err := writer.AppendJSON(redRotating); !errors.Is(err, errRed) {
		t.Fatalf("close error = %v, want the injected failure", err)
	}
}

func TestRotateReportsAFailedRemovalOfTheOldestChunk(t *testing.T) {
	host := redHost()
	writer := redNewWriter(t, host, 2)
	writer.host.Files.Remove = func(name string) error {
		if name == redFirstRotate {
			return errRed
		}

		return nil
	}
	if err := writer.AppendJSON("aaaa"); err != nil {
		t.Fatal(err)
	}
	if err := writer.AppendJSON(redRotating); !errors.Is(err, errRed) {
		t.Fatalf("oldest removal error = %v, want the injected failure", err)
	}
}

func TestRotateReportsAFailedRenameBetweenGenerations(t *testing.T) {
	host := redHost()
	writer := redNewWriter(t, host, 3)
	writer.host.Files.Rename = func(oldpath, _ string) error {
		if oldpath == redFirstRotate {
			return errRed
		}

		return nil
	}
	if err := writer.AppendJSON("aaaa"); err != nil {
		t.Fatal(err)
	}
	if err := writer.AppendJSON(redRotating); !errors.Is(err, errRed) {
		t.Fatalf("generation rename error = %v, want the injected failure", err)
	}
}

func TestRotateReportsAFailedRenameOfTheCurrentChunk(t *testing.T) {
	host := redHost()
	writer := redNewWriter(t, host, 2)
	writer.host.Files.Rename = func(oldpath, _ string) error {
		if oldpath == redChunk {
			return errRed
		}

		return nil
	}
	if err := writer.AppendJSON("aaaa"); err != nil {
		t.Fatal(err)
	}
	if err := writer.AppendJSON(redRotating); !errors.Is(err, errRed) {
		t.Fatalf("current rename error = %v, want the injected failure", err)
	}
}

func TestRotateReplacesTheOnlyChunkWhenOneIsKept(t *testing.T) {
	host := redHost()
	writer := redNewWriter(t, host, 1)
	writer.host.Files.Remove = func(name string) error {
		if name == redChunk {
			return errRed
		}

		return nil
	}
	if err := writer.AppendJSON("aaaa"); err != nil {
		t.Fatal(err)
	}
	if err := writer.AppendJSON(redRotating); !errors.Is(err, errRed) {
		t.Fatalf("single-chunk removal error = %v, want the injected failure", err)
	}
}

func TestRotateReportsAFailedOpenOfTheNewChunk(t *testing.T) {
	host := redHost()
	writer := redNewWriter(t, host, 2)
	open := writer.host.Files.OpenFile
	writer.host.Files.OpenFile = func(name string, flag int, perm fs.FileMode) (system.File, error) {
		if name == redChunk && flag&os.O_EXCL != 0 {
			return nil, errRed
		}

		return open(name, flag, perm)
	}
	if err := writer.AppendJSON("aaaa"); err != nil {
		t.Fatal(err)
	}
	if err := writer.AppendJSON(redRotating); !errors.Is(err, errRed) {
		t.Fatalf("new chunk error = %v, want the injected failure", err)
	}
}

func TestCloseRejectsAClosedWriter(t *testing.T) {
	var nilWriter *Writer
	if err := nilWriter.Close(); err == nil {
		t.Fatal("a nil writer closed without error")
	}
	if err := (&Writer{}).Close(); err == nil {
		t.Fatal("a writer without a chunk closed without error")
	}
}

func TestCloseTolerateAMarkerThatIsAlreadyGone(t *testing.T) {
	host := redHost()
	writer := redNewWriter(t, host, 2)
	delete(host.Files.Contents, redChunkMarker)
	if err := writer.Close(); err != nil {
		t.Fatalf("close with the marker gone returned %v", err)
	}
}
