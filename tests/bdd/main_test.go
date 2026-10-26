package bdd_test

import (
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/wahidyankf/hippo/tests/contract"
	"github.com/wahidyankf/hippo/tests/support"
)

// TestMain keeps inherited HIPPO_ variables and the shared evidence root away
// from the binary under test; see support.RunIsolated. The compiled end-to-end
// entries resolve their candidate through support.RunCompiled instead.
func TestMain(m *testing.M) {
	flag.Parse()
	adapter, err := executingBDDSuite(os.Getenv("HIPPO_BDD_SUITE"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "select executing BDD suite: %v\n", err)
		os.Exit(2)
	}
	if adapter.Name == contract.E2E && !complianceOnlySelection(flag.Lookup("test.run").Value.String()) {
		os.Exit(support.RunCompiled(m))
	}
	os.Exit(support.RunIsolated(m))
}
