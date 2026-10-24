package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/policy"
)

// TestStatusTextNamesTheSwapAllowanceOnlyWhileCountingIsOn proves the text status
// line keeps its shape while counting is off, and gains one token while it is on.
func TestStatusTextNamesTheSwapAllowanceOnlyWhileCountingIsOn(t *testing.T) {
	var off bytes.Buffer
	if _, err := renderStatus(&off, false, application.StatusView{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(off.String(), "swapAllowanceGiB") {
		t.Fatalf("counting off named the allowance: %q", off.String())
	}

	var on bytes.Buffer
	counted := application.StatusView{}
	counted.SwapAllowanceBytes = new(4 * policy.GiB)
	if _, err := renderStatus(&on, false, counted); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(on.String(), " swapAllowanceGiB=4.00\n") {
		t.Fatalf("counting on line = %q, want the allowance token last", on.String())
	}
}
