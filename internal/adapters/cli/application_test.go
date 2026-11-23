package cli //nolint:testpackage // The build identity defaults are read from the package variables.

import (
	"bytes"
	"context"
	"fmt"
	"testing"
)

func TestAnApplicationWithoutAnIdentityReportsTheBuildIdentity(t *testing.T) {
	// An injected application that names no version or commit reports the
	// build's own identity, not an empty one.
	t.Parallel()

	stdout := &bytes.Buffer{}
	code, err := Application{Stdout: stdout, Stderr: &bytes.Buffer{}}.Run(context.Background(), []string{"version"})
	if want := fmt.Sprintf("%s (%s)\n", Version, Commit); err != nil || code != 0 || stdout.String() != want {
		t.Fatalf("code=%d output=%q error=%v, want %q", code, stdout, err, want)
	}
}
