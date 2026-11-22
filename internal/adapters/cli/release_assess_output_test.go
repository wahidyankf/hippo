package cli //nolint:testpackage // The verdict writer is injected through the application's own Stdout.

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestReleaseAssessReportsAWriterThatRefusesItsVerdict(t *testing.T) {
	// A summary that parses is assessed, so a verdict is due; a writer that
	// refuses it must fail the command rather than report a silent success.
	t.Parallel()

	stderr := &bytes.Buffer{}
	application := wiredApplication(Application{
		Stdout: &refusingWriter{}, Stderr: stderr, Environment: []string{"HIPPO_ROOT=" + t.TempDir()},
		Stdin: strings.NewReader(`{"schemaVersion":3,"sampleCount":1,"availableParallelism":8}`),
	})
	code, _ := application.Run(context.Background(), []string{"release", "assess", "--summary", "-"})
	if code == 0 || !strings.Contains(stderr.String(), "stdout refused the write") {
		t.Fatalf("code=%d stderr=%q, want the refused verdict reported", code, stderr)
	}
}
