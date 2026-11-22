package evidence //nolint:testpackage // Host-seam tests drive the unexported run writer that takes the system port.

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/policy"
)

const redRunSummary = redRoot + "/run.summary.json"

func TestNewRunWriterReportsAWriterFailure(t *testing.T) {
	port := redHost().Port()
	port.Files.MkdirAll = func(string, fs.FileMode) error { return errRed }
	if _, err := newRunWriter(port, redRoot, "run", redChunkLimits(2)); !errors.Is(err, errRed) {
		t.Fatalf("writer error = %v, want the injected failure", err)
	}
}

func TestRunWriterAppendRejectsAClosedWriter(t *testing.T) {
	var nilWriter *RunWriter
	if err := nilWriter.Append(policy.Sample{}); err == nil {
		t.Fatal("a nil run writer appended a sample")
	}
	if err := (&RunWriter{}).Append(policy.Sample{}); err == nil {
		t.Fatal("a run writer without a stream appended a sample")
	}
}

func TestRunWriterAppendReportsASampleBeyondTheChunk(t *testing.T) {
	writer, err := newRunWriter(redHost().Port(), redRoot, "run", Limits{ChunkBytes: 16, Chunks: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err = writer.Append(policy.Sample{}); err == nil {
		t.Fatal("a sample larger than the chunk was appended")
	}
}

func TestRunWriterFinalizeRejectsAClosedWriter(t *testing.T) {
	var nilWriter *RunWriter
	if _, err := nilWriter.Finalize(policy.TaskEphemeral, OutcomePassed, 0); err == nil {
		t.Fatal("a nil run writer finalized")
	}
	if _, err := (&RunWriter{}).Finalize(policy.TaskEphemeral, OutcomePassed, 0); err == nil {
		t.Fatal("a run writer without a stream finalized")
	}
}

func TestRunWriterFinalizeReportsAFailedStreamClose(t *testing.T) {
	writer, err := newRunWriter(redHost().Port(), redRoot, "run", redChunkLimits(2))
	if err != nil {
		t.Fatal(err)
	}
	writer.output.file = &redFile{File: writer.output.file, faults: redFileFaults{closeErr: errRed}}
	if _, err = writer.Finalize(policy.TaskEphemeral, OutcomePassed, 0); !errors.Is(err, errRed) {
		t.Fatalf("stream close error = %v, want the injected failure", err)
	}
}

func TestRunWriterFinalizeReportsASummaryCreationFailure(t *testing.T) {
	writer, err := newRunWriter(redHost().Port(), redRoot, "run", redChunkLimits(2))
	if err != nil {
		t.Fatal(err)
	}
	writer.host.Files.CreateTemp = func(string, string) (system.File, error) { return nil, errRed }
	if _, err = writer.Finalize(policy.TaskEphemeral, OutcomePassed, 0); !errors.Is(err, errRed) {
		t.Fatalf("summary creation error = %v, want the injected failure", err)
	}
}

func TestRunWriterFinalizeReportsAChmodFailureAndRemovesItsTemporary(t *testing.T) {
	host := redHost()
	writer, err := newRunWriter(host.Port(), redRoot, "run", redChunkLimits(2))
	if err != nil {
		t.Fatal(err)
	}
	redWrapOpenFiles(&writer.host, redFileFaults{chmod: errRed})
	if _, err = writer.Finalize(policy.TaskEphemeral, OutcomePassed, 0); !errors.Is(err, errRed) {
		t.Fatalf("chmod error = %v, want the injected failure", err)
	}
	for name := range host.Files.Contents {
		if strings.HasPrefix(name, redRoot+"/.summary-") {
			t.Fatalf("temporary summary %s survived the failed write", name)
		}
	}
	if _, published := host.Files.Contents[redRunSummary]; published {
		t.Fatal("a failed summary was published")
	}
}

func TestRunWriterFinalizeReportsASyncFailure(t *testing.T) {
	writer, err := newRunWriter(redHost().Port(), redRoot, "run", redChunkLimits(2))
	if err != nil {
		t.Fatal(err)
	}
	redWrapOpenFiles(&writer.host, redFileFaults{sync: errRed})
	if _, err = writer.Finalize(policy.TaskEphemeral, OutcomePassed, 0); !errors.Is(err, errRed) {
		t.Fatalf("sync error = %v, want the injected failure", err)
	}
}

func TestRunWriterFinalizeReportsALinkFailure(t *testing.T) {
	writer, err := newRunWriter(redHost().Port(), redRoot, "run", redChunkLimits(2))
	if err != nil {
		t.Fatal(err)
	}
	writer.host.Files.Link = func(string, string) error { return errRed }
	if _, err = writer.Finalize(policy.TaskEphemeral, OutcomePassed, 0); !errors.Is(err, errRed) {
		t.Fatalf("link error = %v, want the injected failure", err)
	}
}

func TestRunWriterFinalizePublishesTheSummaryOnce(t *testing.T) {
	host := redHost()
	writer, err := newRunWriter(host.Port(), redRoot, "run", redChunkLimits(2))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = writer.Finalize(policy.TaskEphemeral, OutcomePassed, 0); err != nil {
		t.Fatal(err)
	}
	if _, published := host.Files.Contents[redRunSummary]; !published {
		t.Fatal("the summary was not published")
	}
}
