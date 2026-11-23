package cli //nolint:testpackage // Output refusal and encoding failures are injected through the application's own writers and collector.

import (
	"bytes"
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	guard "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/tests/support/runtimewiring"
)

// refusingWriter accepts the first accepting writes and refuses every write after them, as a closed pipe does.
type refusingWriter struct {
	accepting int
	writes    int
	output    bytes.Buffer
}

func (writer *refusingWriter) Write(data []byte) (int, error) {
	writer.writes++
	if writer.writes > writer.accepting {
		return 0, errors.New("stdout refused the write")
	}

	return writer.output.Write(data)
}

func TestStatusTextReportsAWriterThatRefusesItsHeader(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	stderr := &bytes.Buffer{}
	application := wiredApplication(Application{
		Stdout: &refusingWriter{}, Stderr: stderr, Environment: []string{"HIPPO_ROOT=" + t.TempDir()},
		Collector: stableCollector{sample: stableDevelopmentSample(now)}, Now: func() time.Time { return now },
		Sleep: func(time.Duration) {},
	})
	code, _ := application.Run(context.Background(), []string{"status"})
	if code == 0 || !strings.Contains(stderr.String(), "stdout refused the write") {
		t.Fatalf("code=%d stderr=%q, want the refused header reported", code, stderr)
	}
}

func TestStatusTextReportsAWriterThatRefusesAnOwnerRow(t *testing.T) {
	// The header is written, so the refusal lands on the owner row that follows
	// it: the second write of the status text.
	t.Parallel()

	root := t.TempDir()
	owner, err := runtimewiring.AcquireSession(context.Background(), root, "", policy.TaskEphemeral, time.Second)
	if err != nil || owner == nil {
		t.Fatalf("acquire exclusive owner: session=%v error=%v", owner != nil, err)
	}
	defer func() { _ = guard.ReleaseSession(root, owner) }()

	now := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	stderr := &bytes.Buffer{}
	application := wiredApplication(Application{
		Stdout: &refusingWriter{accepting: 1}, Stderr: stderr, Environment: []string{"HIPPO_ROOT=" + root},
		Collector: stableCollector{sample: stableDevelopmentSample(now)}, Now: func() time.Time { return now },
		Sleep: func(time.Duration) {},
	})
	code, _ := application.Run(context.Background(), []string{"status"})
	if code == 0 || !strings.Contains(stderr.String(), "stdout refused the write") {
		t.Fatalf("code=%d stderr=%q, want the refused owner row reported", code, stderr)
	}
}

func TestStatusJSONReportsASampleThatCannotBeEncoded(t *testing.T) {
	// A sample whose CPU reading is not a number has no JSON form, so the
	// snapshot cannot be written and the failure is reported, not printed short.
	t.Parallel()

	now := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	notANumber := math.NaN()
	sample := stableDevelopmentSample(now)
	sample.CPUUtilizationPercent = &notANumber
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	application := wiredApplication(Application{
		Stdout: stdout, Stderr: stderr, Environment: []string{"HIPPO_ROOT=" + t.TempDir()},
		Collector: stableCollector{sample: sample}, Now: func() time.Time { return now },
		Sleep: func(time.Duration) {},
	})
	code, _ := application.Run(context.Background(), []string{"status", "--json"})
	if code == 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "encode status JSON") {
		t.Fatalf("code=%d stdout=%q stderr=%q, want an encoding failure and no partial output", code, stdout, stderr)
	}
}
