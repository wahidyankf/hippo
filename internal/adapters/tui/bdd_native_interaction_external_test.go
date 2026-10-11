package tui_test

import (
	"os"
	"testing"

	"github.com/wahidyankf/hippo/tests/contract"
)

func TestTUIBDDNativeInteractionCanonical(t *testing.T) {
	if os.Getenv("HIPPO_BDD_SUITE") != contract.Integration {
		t.Skip("native interaction rows require explicit Integration")
	}
	driver := runNativeCanonicalSelection(t, "@tui-resources-01,@tui-resources-02,@tui-views-02,@tui-views-03,@tui-views-04,@tui-views-05,@tui-views-06,@tui-views-07,@tui-keys-03,@tui-keys-04,@tui-keys-05,@tui-keys-06", map[string]int{
		"Missing host readings are distinct from accounting zero":          1,
		"Reservation resources are named separately from host utilization": 1,
		"Too-small view recovers without losing live selection":            1,
		"Untrusted display text cannot escape its cell budget":             1,
		"Synthetic unsafe display text cannot escape its cell budget":      3,
		"Large lists scroll while retaining the selected identity":         1,
		"A disappearing selection moves to the nearest surviving row":      1,
		"A disappeared detail remains navigable":                           1,
		"Empty panels have no false selection":                             1,
		"Manual refresh is reachable from every view":                      5,
		"Help explains every product function using modifier-free keys":    1,
		"Unrelated keys do not capture host navigation":                    8,
		"No-color output retains every state distinction":                  1,
	}, 26, 78)
	if len(driver.native.completed) != 26 || driver.native.cleanupError != nil {
		t.Fatalf("native interaction sessions completed=%d cleanup=%v, want twenty-six clean sessions", len(driver.native.completed), driver.native.cleanupError)
	}
	for _, receipt := range driver.native.completed {
		if !receipt.Joined() || receipt.ExitCode != 0 || len(receipt.Stderr) != 0 {
			t.Fatalf("native interaction did not naturally quit, reap and drain: %+v", receipt)
		}
		t.Logf("native interaction pid=%d natural-exit=%d joined=%t stdout-bytes=%d", receipt.Identity.PID, receipt.ExitCode, receipt.Joined(), len(receipt.Stdout))
	}
}
