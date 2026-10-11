package tui_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/wahidyankf/hippo/tests/contract"
	"github.com/wahidyankf/hippo/tests/support"
)

// TestBDDPublicTUISessionHolderHelper serves only the owned holder re-exec.
func TestBDDPublicTUISessionHolderHelper(t *testing.T) {
	if os.Getenv(support.PublicTUISessionHolderEnvironment) != "1" {
		t.Skip("runs only as a marked owned public session holder")
	}
	if os.Getenv("HIPPO_BDD_SUITE") != contract.Unit || os.Getenv("HIPPO_TEST_ISOLATED_ROOT") == "" {
		fmt.Fprintln(os.Stderr, "owned public holder requires inherited unit isolation")
		os.Exit(1)
	}
	deadline, bounded := t.Deadline()
	if !bounded {
		fmt.Fprintln(os.Stderr, "owned public holder requires the finite test deadline")
		os.Exit(1)
	}
	ctx, cancel := context.WithDeadline(t.Context(), deadline)
	err := support.RunPublicTUISessionHolder(ctx)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "owned public holder: %v\n", err)
		os.Exit(1)
	}
	// Stdout carries framed protocol reports; testing must not append PASS.
	os.Exit(0)
}
