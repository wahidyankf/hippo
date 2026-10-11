package tui_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/wahidyankf/hippo/tests/support"
)

// TestBDDPublicTUIHostProbeHelper runs only an owned external collector probe.
func TestBDDPublicTUIHostProbeHelper(t *testing.T) {
	if os.Getenv(support.PublicTUIHostProbeHelperEnvironment) != "1" {
		t.Skip("runs only as a marked external collector probe")
	}
	if err := support.RunPublicTUIHostProbeHelper(t.Context()); err != nil {
		fmt.Fprintf(os.Stderr, "owned host probe: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
