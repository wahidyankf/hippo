package application //nolint:testpackage // The declaration check is a private pre-launch seam, tested before identity and sampling.

import (
	"errors"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/status"
)

func TestVeryHeavyRunsOnlyWhereTheConfigurationDeclaresIt(t *testing.T) {
	declared := map[string]coordination.ResourceTierPolicy{
		coordination.ResourceTierLight.String():     {},
		coordination.ResourceTierStandard.String():  {},
		coordination.ResourceTierHeavy.String():     {},
		coordination.ResourceTierVeryHeavy.String(): {},
	}
	if err := tierDeclaredConflict(declared, coordination.ResourceTierVeryHeavy.String()); err != nil {
		t.Fatalf("declared very-heavy refused: %v", err)
	}
	for _, tier := range []string{"", "light", "standard", "heavy"} {
		if err := tierDeclaredConflict(nil, tier); err != nil {
			t.Fatalf("tier %q refused without a declaration: %v", tier, err)
		}
	}
}

func TestUndeclaredVeryHeavyIsRefusedAsUsageNamingTheTiersTheConfigurationHas(t *testing.T) {
	// Schema 1, schema 2, and no configuration all carry no declared tiers.
	err := tierDeclaredConflict(nil, coordination.ResourceTierVeryHeavy.String())
	if err == nil {
		t.Fatal("undeclared very-heavy was accepted")
		return
	}
	var failure status.Failure
	if !errors.As(err, &failure) || failure.Code != status.CodeArgsInvalid {
		t.Fatalf("undeclared very-heavy error = %v, want %s", err, status.CodeArgsInvalid)
	}
	want := "--resource-tier very-heavy: this configuration does not declare the very-heavy tier; choose light, standard, or heavy"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("undeclared very-heavy message = %q, want it to contain %q", err.Error(), want)
	}
}
