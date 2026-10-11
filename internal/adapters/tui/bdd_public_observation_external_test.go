package tui_test

import (
	"os"
	"testing"

	"github.com/wahidyankf/hippo/tests/contract"
)

// This entry executes the original registered scenario and its original steps.
// Its frozen original RED refused the missing public fixture at the initial Given.
func TestTUIBDDPublicObservationCanonical(t *testing.T) {
	if os.Getenv("HIPPO_BDD_SUITE") != contract.E2E {
		t.Fatal("public observation entry requires explicit HIPPO_BDD_SUITE=e2e")
	}
	driver := runPublicCanonicalSelection(t, "@tui-snapshot-01", map[string]int{
		"TUI observation preserves running identities and FIFO demand": 1,
	}, 1, 3)
	if driver.public == nil || len(driver.public.completed) != 1 {
		t.Fatal("original public observation lacks exactly one completed actual native receipt")
	}
	for _, receipt := range driver.public.completed {
		if receipt.StatusPID <= 0 || !receipt.Candidate.Joined() || !receipt.Candidate.NaturalExit() || receipt.Candidate.ExitCode != 0 || !receipt.Collector.Closed || receipt.Collector.Active != 0 || receipt.Collector.Readings != 2 {
			t.Fatalf("original observation retained incomplete actual public status/native/probe lifetime: %+v", receipt)
		}
		t.Logf("original public observation actual statusPID=%d status=%q diagnostics=%q fixture=%+v collector=%+v candidate=%+v", receipt.StatusPID, receipt.StatusOutput, receipt.StatusDiagnostics, receipt.Expected, receipt.Collector, receipt.Candidate)
	}
}
