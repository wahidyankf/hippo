package cli //nolint:testpackage // The flag validator is a private parser seam, tested before configuration is read.

import (
	"errors"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/status"
)

func TestRunFlagAcceptsEveryKnownResourceTierAndRefusesTypos(t *testing.T) {
	for _, tier := range []string{"", "light", "standard", "heavy", "very-heavy"} {
		if err := runArgumentMistake(runOptions{resourceTier: tier}); err != nil {
			t.Fatalf("--resource-tier %q refused before configuration: %v", tier, err)
		}
	}
	for _, tier := range []string{"very_heavy", "veryheavy", "huge"} {
		err := runArgumentMistake(runOptions{resourceTier: tier})
		if err == nil {
			t.Fatalf("--resource-tier %q accepted", tier)
		}
		if !strings.Contains(err.Error(), "resource tier must be light, standard, heavy, or very-heavy") {
			t.Fatalf("--resource-tier %q error = %v, want the closed set named", tier, err)
		}
		var failure status.Failure
		if !errors.As(err, &failure) || failure.Code != status.CodeArgsInvalid {
			t.Fatalf("--resource-tier %q error = %v, want %s", tier, err, status.CodeArgsInvalid)
		}
	}
}
