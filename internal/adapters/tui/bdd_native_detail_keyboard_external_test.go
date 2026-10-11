package tui_test

import (
	"os"
	"testing"

	"github.com/wahidyankf/hippo/tests/contract"
)

func TestTUIBDDNativeDetailKeyboardCanonical(t *testing.T) {
	if os.Getenv("HIPPO_BDD_SUITE") != contract.Integration {
		t.Skip("native detail/help keyboard rows require explicit Integration")
	}
	driver := runNativeCanonicalSelection(t, "@tui-keys-02", map[string]int{
		"Detail and help scroll and return without a focus trap": 15,
	}, 15, 45)
	if len(driver.native.completed) != 15 || driver.native.cleanupError != nil {
		t.Fatalf("native detail/help keyboard sessions completed=%d cleanup=%v, want fifteen clean sessions", len(driver.native.completed), driver.native.cleanupError)
	}
	for _, receipt := range driver.native.completed {
		if !receipt.Joined() || receipt.ExitCode != 0 || len(receipt.Stderr) != 0 {
			t.Fatalf("native detail/help keyboard did not naturally quit, reap and drain: %+v", receipt)
		}
		t.Logf("native detail/help keyboard pid=%d natural-exit=%d joined=%t stdout-bytes=%d", receipt.Identity.PID, receipt.ExitCode, receipt.Joined(), len(receipt.Stdout))
	}
}
