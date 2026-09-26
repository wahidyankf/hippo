package conformance

import (
	"context"
	"fmt"
	"io"

	"github.com/wahidyankf/hippo/internal/status"
)

// Main runs hippo-conformance with its arguments and returns its exit status:
// 0 when the four consumers conform, 1 when they do not, 2 for an unusable
// invocation, and 128+N when signal N stopped the run first. A stopped run
// reached no verdict, so it must not read as a failed one; whatever the
// harness found while stopping and reconciling is still printed.
func Main(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	if len(arguments) != 1 {
		_, _ = fmt.Fprintln(stderr, "usage: hippo-conformance <manifest.json>")

		return 2
	}
	err := Run(ctx, arguments[0], stdout)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
	}
	if interruption, interrupted := status.Interrupted(ctx); interrupted && err != nil {
		return interruption.Status()
	}
	if err != nil {
		return 1
	}

	return 0
}
