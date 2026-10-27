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
	// CountSwap is coordination.countSwap: whether swap counts toward memory.
	CountSwap bool
	// MemoryPolicy is coordination.memoryPolicy: the optional RAM-proportional
	// admission thresholds. Zero throughout means the block was absent, and the
	// legacy fixed thresholds apply.
	MemoryPolicy policy.MemoryPolicy
	// SwapUsedMaxMultiple is coordination.swapUsedMaxMultiple: the optional
	// per-machine multiple of physical memory within which swap-out pressure is
	// waived. Zero or negative means the setting was absent, and the legacy
	// swap rules apply.
	SwapUsedMaxMultiple float64
}
