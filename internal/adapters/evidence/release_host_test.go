package evidence //nolint:testpackage // Host-seam tests drive the unexported release functions that take the system port.

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"math"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/application"
	domain "github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
)

// redShortWriter accepts one byte less than it is given and reports no error.
type redShortWriter struct{}

func (redShortWriter) Write(data []byte) (int, error) { return max(len(data)-1, 0), nil }

// redMonitor is a monitor configuration whose output and summary live under the evidence root.
func redMonitor() application.MonitorConfig {
	return application.MonitorConfig{
		OutputPath:  redRoot + "/release/out.jsonl",
		SummaryPath: redRoot + "/release/summary.json",
		Now:         func() time.Time { return redStart },
	}
}

func TestOpenReleaseOutputStreamRejectsAnUnencodableSample(t *testing.T) {
	output, root, err := openReleaseOutput(redHost().Port(), application.MonitorConfig{RawOutput: &bytes.Buffer{}})
	if err != nil || root != "" {
		t.Fatalf("stream output opened with root %q and %v", root, err)
	}
	if err = output.Append(domain.ReleaseSample{OneMinuteLoad: math.NaN()}); err == nil {
		t.Fatal("a sample that cannot be encoded was appended to the stream")
	}
}

func TestOpenReleaseOutputStreamReportsAShortWrite(t *testing.T) {
	output, _, err := openReleaseOutput(redHost().Port(), application.MonitorConfig{RawOutput: redShortWriter{}})
	if err != nil {
		t.Fatal(err)
	}
	if err = output.Append(domain.ReleaseSample{}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write error = %v, want io.ErrShortWrite", err)
	}
}

func TestOpenReleaseOutputReportsACleanupFailure(t *testing.T) {
	port := redHost().Port()
	port.Files.MkdirAll = func(string, fs.FileMode) error { return errRed }
	if _, _, err := openReleaseOutput(port, redMonitor()); !errors.Is(err, errRed) {
		t.Fatalf("cleanup error = %v, want the injected failure", err)
	}
}

func TestOpenReleaseOutputOpensAFileOwnedByTheHost(t *testing.T) {
	host := redHost()
	output, root, err := openReleaseOutput(host.Port(), redMonitor())
	if err != nil || root != redRoot+"/release" {
		t.Fatalf("file output opened with root %q and %v", root, err)
	}
	if err = output.Append(domain.ReleaseSample{}); err != nil {
		t.Fatal(err)
	}
	if err = output.Close(); err != nil {
		t.Fatal(err)
	}
	if _, written := host.Files.Contents[redMonitor().OutputPath]; !written {
		t.Fatal("the release stream was not written into the host")
	}
}

func TestWriteReleaseSummaryRequiresSamples(t *testing.T) {
	err := writeReleaseSummary(redHost().Port(), redRoot+"/summary.json", nil, policy.ReleaseSummary{})
	if err == nil {
		t.Fatal("a summary without samples was written")
	}
}

func TestWriteReleaseSummaryRejectsAnUnencodableDestinationSummary(t *testing.T) {
	summary := policy.ReleaseSummary{SampleCount: 1, CPUUtilizationP95Percent: math.NaN()}
	if err := writeReleaseSummary(redHost().Port(), redRoot+"/summary.json", &bytes.Buffer{}, summary); err == nil {
		t.Fatal("an unencodable summary was written to the destination")
	}
}

func TestWriteReleaseSummaryReportsAShortDestinationWrite(t *testing.T) {
	summary := policy.ReleaseSummary{SampleCount: 1}
	if err := writeReleaseSummary(redHost().Port(), redRoot+"/summary.json", redShortWriter{}, summary); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write error = %v, want io.ErrShortWrite", err)
	}
}

func TestWriteReleaseSummaryRejectsAnUnencodableFileSummary(t *testing.T) {
	summary := policy.ReleaseSummary{SampleCount: 1, CPUUtilizationP95Percent: math.NaN()}
	if err := writeReleaseSummary(redHost().Port(), redRoot+"/summary.json", nil, summary); err == nil {
		t.Fatal("an unencodable summary was written to a file")
	}
}

func TestWriteReleaseSummaryReportsAnExistingOrRefusedFile(t *testing.T) {
	port := redHost().Port()
	port.Files.OpenFile = func(string, int, fs.FileMode) (system.File, error) { return nil, errRed }
	summary := policy.ReleaseSummary{SampleCount: 1}
	if err := writeReleaseSummary(port, redRoot+"/summary.json", nil, summary); !errors.Is(err, errRed) {
		t.Fatalf("file open error = %v, want the injected failure", err)
	}
}

func TestWriteReleaseSummaryStoresTheSummaryOnce(t *testing.T) {
	host := redHost()
	summary := policy.ReleaseSummary{SampleCount: 1}
	path := redRoot + "/summary.json"
	if err := writeReleaseSummary(host.Port(), path, nil, summary); err != nil {
		t.Fatal(err)
	}
	if _, stored := host.Files.Contents[path]; !stored {
		t.Fatal("the summary was not stored")
	}
	if err := writeReleaseSummary(host.Port(), path, nil, summary); err == nil {
		t.Fatal("a second summary overwrote the first")
	}
}

func TestReadReleaseSummaryReportsAReadFailure(t *testing.T) {
	port := redHost().Port()
	port.Files.ReadFile = func(string) ([]byte, error) { return nil, errRed }
	if _, err := readReleaseSummary(port, redRoot+"/summary.json"); !errors.Is(err, errRed) {
		t.Fatalf("read error = %v, want the injected failure", err)
	}
}

func TestReadReleaseSummaryReadsTheStoredBytes(t *testing.T) {
	host := redHost()
	host.Files.Contents[redRoot+"/summary.json"] = []byte("{}")
	data, err := readReleaseSummary(host.Port(), redRoot+"/summary.json")
	if err != nil || string(data) != "{}" {
		t.Fatalf("summary bytes = %q with %v", data, err)
	}
}

func TestCleanupReleaseRejectsAnEmptyRoot(t *testing.T) {
	if err := cleanupRelease(redHost().Port(), "", redStart); err == nil {
		t.Fatal("an empty release root was cleaned")
	}
}
