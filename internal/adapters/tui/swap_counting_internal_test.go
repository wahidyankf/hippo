package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/policy"
)

// TestSwapCountingLinesAppearOnlyWhileCountingIsOn proves a counting-off view
// adds no line, and a counting-on view names the allowance, headroom, and counted
// memory, plus the advisory only while swap use exceeds the allowance.
func TestSwapCountingLinesAppearOnlyWhileCountingIsOn(t *testing.T) {
	if lines := swapCountingLines(application.StatusView{}); lines != nil {
		t.Fatalf("counting off added lines %q", lines)
	}

	counted := application.StatusView{}
	counted.SwapAllowanceBytes = new(4 * policy.GiB)
	counted.Resource.SwapHeadroomBytes = new(3 * policy.GiB)
	counted.Resource.CountedAvailableMemoryBytes = new(6 * policy.GiB)
	lines := swapCountingLines(counted)
	if len(lines) != 3 {
		t.Fatalf("counting on added %d lines, want 3: %q", len(lines), lines)
	}
	for _, want := range []string{"Swap allowance: ", "Swap headroom: ", "Counted available RAM: "} {
		if !slices.ContainsFunc(lines, func(line string) bool { return strings.HasPrefix(line, want) }) {
			t.Fatalf("lines %q lack %q", lines, want)
		}
	}

	counted.Resource.Advisories = []string{policy.SwapOverAllowanceAdvisory}
	if lines := swapCountingLines(counted); len(lines) != 4 {
		t.Fatalf("over the allowance added %d lines, want the advisory as a fourth: %q", len(lines), lines)
	}
}
