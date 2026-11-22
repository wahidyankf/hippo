package cli //nolint:testpackage // The run argument check is the unexported Cobra argument validator.

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/status"
)

func TestRunRefusesArgumentsBeforeTheGuardSeparatorAndRunsNothing(t *testing.T) {
	// A word before -- is not a flag run knows, so the invocation is refused as
	// a usage mistake before any guarded command could start.
	t.Parallel()

	stderr := &bytes.Buffer{}
	application := Application{Stdout: &bytes.Buffer{}, Stderr: stderr}
	code, _ := application.Run(context.Background(), []string{"run", "stray", "--", "echo", "ok"})
	if code != status.CallerError || !strings.Contains(stderr.String(), "unknown run arguments: [stray]") {
		t.Fatalf("code=%d stderr=%q, want %d naming the stray argument", code, stderr, status.CallerError)
	}
}
