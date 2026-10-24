package coordination

import (
	"fmt"
	"slices"
)

// ResourceTier names one launch-time resource tier. A schema-3 configuration
// must declare every required tier; an optional tier exists only where the
// configuration declares it, and its bounds come from that configuration alone.
type ResourceTier string

const (
	// ResourceTierLight is the smallest required tier.
	ResourceTierLight ResourceTier = "light"
	// ResourceTierStandard is the middle required tier.
	ResourceTierStandard ResourceTier = "standard"
	// ResourceTierHeavy is the largest required tier.
	ResourceTierHeavy ResourceTier = "heavy"
	// ResourceTierVeryHeavy is the optional last-resort tier.
	ResourceTierVeryHeavy ResourceTier = "very-heavy"
)

// RequiredResourceTiers returns the tiers every schema-3 configuration declares.
func RequiredResourceTiers() []ResourceTier {
	return []ResourceTier{ResourceTierLight, ResourceTierStandard, ResourceTierHeavy}
}

// OptionalResourceTiers returns the tiers a schema-3 configuration may declare.
func OptionalResourceTiers() []ResourceTier {
	return []ResourceTier{ResourceTierVeryHeavy}
}

// KnownResourceTiers returns every tier name, the required tiers first.
func KnownResourceTiers() []ResourceTier {
	return slices.Concat(RequiredResourceTiers(), OptionalResourceTiers())
}

// ParseResourceTier reads the text of one tier and refuses any name that is not
// a member of KnownResourceTiers, the empty name included. Whether a known name
// is declared is a question for the loaded configuration, not for this parser.
func ParseResourceTier(text string) (ResourceTier, error) {
	tier := ResourceTier(text)
	if !slices.Contains(KnownResourceTiers(), tier) {
		return "", fmt.Errorf("unknown resource tier %q", text)
	}

	return tier, nil
}

// String returns the tier's name as configuration and the command line spell it.
func (tier ResourceTier) String() string {
	return string(tier)
}
