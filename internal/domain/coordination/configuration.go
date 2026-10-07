package coordination

import "github.com/wahidyankf/hippo/internal/policy"

// Promotion is the completed-evidence gate for temporarily opening owner three.
type Promotion struct {
	CompletedRuns               int
	MinimumSources              int
	MinimumAvailableMemoryBytes int64
	MaximumCPUP95Percent        float64
}

// Configuration is the validated, privacy-safe shared-root coordination policy.
type Configuration struct {
	SchemaVersion                 int
	Mode                          string
	MaxCPU                        int
	MaxMemoryBytes                int64
	BaseActiveOwners              int
	MaxActiveOwners               int
	OwnerShares                   map[policy.ProfileName]int
	Promotion                     Promotion
	EmergencyAvailableMemoryBytes int64
	Tiers                         map[string]ResourceTierPolicy
}
