package coordination_test

import (
	"reflect"
	"testing"

	"github.com/wahidyankf/hippo/internal/domain/coordination"
)

func TestResourceTierSetsAreClosedOrderedAndSeparateRequiredFromOptional(t *testing.T) {
	cases := []struct {
		name string
		got  []coordination.ResourceTier
		want []coordination.ResourceTier
	}{
		{
			name: "required tiers are the three every schema-3 file must declare",
			got:  coordination.RequiredResourceTiers(),
			want: []coordination.ResourceTier{coordination.ResourceTierLight, coordination.ResourceTierStandard, coordination.ResourceTierHeavy},
		},
		{
			name: "optional tiers are declared only where the configuration asks for them",
			got:  coordination.OptionalResourceTiers(),
			want: []coordination.ResourceTier{coordination.ResourceTierVeryHeavy},
		},
		{
			name: "known tiers are the required tiers followed by the optional ones",
			got:  coordination.KnownResourceTiers(),
			want: []coordination.ResourceTier{
				coordination.ResourceTierLight, coordination.ResourceTierStandard,
				coordination.ResourceTierHeavy, coordination.ResourceTierVeryHeavy,
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if !reflect.DeepEqual(test.got, test.want) {
				t.Fatalf("tiers = %v, want %v", test.got, test.want)
			}
		})
	}
}

func TestParseResourceTierAcceptsOnlyKnownNamesAndRefusesTypos(t *testing.T) {
	for _, name := range []string{"light", "standard", "heavy", "very-heavy"} {
		tier, err := coordination.ParseResourceTier(name)
		if err != nil || tier.String() != name {
			t.Fatalf("ParseResourceTier(%q) = %q, %v; want the same name", name, tier, err)
		}
	}
	for _, name := range []string{"", "very_heavy", "veryheavy", "Very-Heavy", "extra", "heavy "} {
		if tier, err := coordination.ParseResourceTier(name); err == nil {
			t.Fatalf("ParseResourceTier(%q) = %q; want a refusal", name, tier)
		}
	}
}

func TestDefaultResourceTiersStaysThreeWithoutCompiledVeryHeavyBounds(t *testing.T) {
	tiers := coordination.DefaultResourceTiers()
	if len(tiers) != 3 {
		t.Fatalf("DefaultResourceTiers has %d tiers, want the three required tiers", len(tiers))
	}
	if _, declared := tiers[coordination.ResourceTierVeryHeavy.String()]; declared {
		t.Fatal("DefaultResourceTiers carries compiled very-heavy bounds; they must come only from the configuration")
	}
}
