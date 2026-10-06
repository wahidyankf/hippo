package policy

import (
	"errors"
	"fmt"
	"math"
)

const (
	// HardDiskFloorBytes is the immutable cleanup boundary.
	HardDiskFloorBytes = 256 * MiB
	swapUnavailable    = "unavailable"
)

// ProfileName names one resource profile: a built-in one, or a configured one
// that extends another. The set is open, since configuration adds names, so it
// is a defined string; what a profile may do is decided by its Lineage, never
// by comparing its name. Its JSON is the plain string.
type ProfileName string

const (
	profileBalanced    ProfileName = "balanced"
	profileConstrained ProfileName = "constrained"
	profileMinimal     ProfileName = "minimal"
)

// Lineage is the built-in profile a profile derives from, which decides every
// rule that once keyed on a profile's name: whether it may use degraded
// admission, whether the last-resort floor applies, and how many owners share
// capacity when nothing configures it. A built-in profile carries its own
// lineage, and a configured profile inherits its parent's through any depth of
// extends; no configuration key can set one. The zero value, LineageUnset, is
// the lineage of nothing, and Resolve refuses a profile that carries it.
type Lineage uint8

const (
	// LineageUnset is the zero value: no lineage.
	LineageUnset Lineage = iota
	// LineageBalanced is the lineage of the built-in balanced profile.
	LineageBalanced
	// LineageConstrained is the lineage of the built-in constrained profile.
	LineageConstrained
	// LineageMinimal is the lineage of the built-in minimal profile.
	LineageMinimal
)

// DegradedAdmission reports whether ephemeral work of the lineage may start at
// concurrency one under a stable macOS warning, and is spared while that
// warning stays stable. Only the balanced lineage may.
func (lineage Lineage) DegradedAdmission() bool {
	switch lineage {
	case LineageBalanced:
		return true
	case LineageConstrained, LineageMinimal, LineageUnset:
		return false
	}

	return false
}

// LastResortFloor reports whether a profile of the lineage that ends its
// fallback chain, with no fallback behind it, is still admitted when it does not
// fit, at relaxed memory and disk thresholds, unless the work is strict. A
// profile that names a fallback falls back to it instead. Only the minimal
// lineage has the floor.
func (lineage Lineage) LastResortFloor() bool {
	switch lineage {
	case LineageMinimal:
		return true
	case LineageBalanced, LineageConstrained, LineageUnset:
		return false
	}

	return false
}

// DefaultOwnerShares is how many owners share the host's capacity for one
// automatic reservation of a profile of the lineage when no configuration says.
func (lineage Lineage) DefaultOwnerShares() int {
	switch lineage {
	case LineageBalanced:
		return 4
	case LineageConstrained:
		return 2
	case LineageMinimal:
		return 1
	case LineageUnset:
		// No lineage, no rule: the share v0.8.4 gave a profile it had no name for.
	}

	return 1
}

// TaskClass identifies the guarded workload category used for admission.
type TaskClass string

const (
	// TaskEphemeral identifies restartable bounded development work.
	TaskEphemeral TaskClass = "ephemeral"
	// TaskService identifies long-lived development services.
	TaskService TaskClass = "service"
	// TaskTransactional identifies non-restartable development work.
	TaskTransactional TaskClass = "transactional"
	// TaskRelease identifies strict release work.
	TaskRelease TaskClass = "release"
)

// Decision is the action selected by profile resolution.
type Decision string

const (
	// DecisionRun admits work using the resolved profile.
	DecisionRun Decision = "run"
	// DecisionWait defers work until transient pressure clears.
	DecisionWait Decision = "wait"
	// DecisionCleanup requires storage cleanup before retrying.
	DecisionCleanup Decision = "cleanup"
	// DecisionReplan requires a different requested capacity envelope.
	DecisionReplan Decision = "replan"
)

// Profile defines one adaptive admission envelope.
type Profile struct {
	Name                        ProfileName `json:"name"`
	Fallback                    ProfileName `json:"fallback,omitempty"`
	Strict                      bool        `json:"strict"`
	MemoryReservePercent        float64     `json:"memoryReservePercent"`
	MemoryReserveMinBytes       int64       `json:"memoryReserveMinBytes"`
	MemoryReserveMaxBytes       int64       `json:"memoryReserveMaxBytes"`
	NoSwapMemoryReservePercent  float64     `json:"noSwapMemoryReservePercent"`
	NoSwapMemoryReserveMinBytes int64       `json:"noSwapMemoryReserveMinBytes"`
	NoSwapMemoryReserveMaxBytes int64       `json:"noSwapMemoryReserveMaxBytes"`
	DiskReservePercent          float64     `json:"diskReservePercent"`
	DiskReserveMinBytes         int64       `json:"diskReserveMinBytes"`
	DiskReserveMaxBytes         int64       `json:"diskReserveMaxBytes"`
	MaxConcurrency              int         `json:"maxConcurrency"`
	MaxCPUUtilizationPercent    float64     `json:"maxCpuUtilizationPercent"`
	// Lineage is the built-in profile this one derives from, which decides its
	// degraded admission, its floor, and its default owner shares. Only a
	// built-in profile sets it; a configured profile inherits it through
	// extends and can never set it, since it has no configuration key.
	Lineage Lineage `json:"-"`
}

// Catalog owns the named profile graph.
type Catalog struct {
	DefaultProfile ProfileName
	Profiles       map[ProfileName]Profile
}

// Resolution is the selected profile and its concrete host thresholds.
type Resolution struct {
	RequestedProfile ProfileName   `json:"requestedProfile"`
	ResolvedProfile  ProfileName   `json:"resolvedProfile"`
	FallbackChain    []ProfileName `json:"fallbackChain"`
	Strict           bool          `json:"strict"`
	Concurrency      int           `json:"concurrency"`
	MemoryReserve    int64         `json:"memoryReserveBytes"`
	DiskReserve      int64         `json:"diskReserveBytes"`
	Decision         Decision      `json:"decision"`
	// Reason is why the resolution stops work, and ReasonNone when it does not.
	// Status JSON publishes it as exitCode, the integer v0.8.4 published for it.
	Reason    Reason `json:"exitCode"`
	Retryable bool   `json:"retryable"`
	// DegradedAdmission is the lineage's answer to whether the resolved profile
	// may use degraded admission under a stable macOS warning, as status
	// publishes it. Nothing decides from it: the guard asks Lineage.
	DegradedAdmission bool `json:"degradedAdmission"`
	// Lineage is the resolved profile's lineage, so the rules that key on it
	// need not look the profile up again.
	Lineage Lineage `json:"-"`
	Policy  Policy  `json:"-"`
}

// BuiltinCatalog returns deterministic capacity-relative defaults.
func BuiltinCatalog() Catalog {
	return Catalog{DefaultProfile: profileBalanced, Profiles: map[ProfileName]Profile{
		profileBalanced: {
			Name:                        profileBalanced,
			Fallback:                    profileConstrained,
			MemoryReservePercent:        15,
			MemoryReserveMinBytes:       GiB,
			MemoryReserveMaxBytes:       4 * GiB,
			NoSwapMemoryReservePercent:  20,
			NoSwapMemoryReserveMinBytes: GiB,
			NoSwapMemoryReserveMaxBytes: 4 * GiB,
			DiskReservePercent:          10,
			DiskReserveMinBytes:         2 * GiB,
			DiskReserveMaxBytes:         20 * GiB,
			MaxCPUUtilizationPercent:    85,
			Lineage:                     LineageBalanced,
		},
		profileConstrained: {
			Name:                        profileConstrained,
			Fallback:                    profileMinimal,
			MemoryReservePercent:        10,
			MemoryReserveMinBytes:       512 * MiB,
			MemoryReserveMaxBytes:       2 * GiB,
			NoSwapMemoryReservePercent:  15,
			NoSwapMemoryReserveMinBytes: 512 * MiB,
			NoSwapMemoryReserveMaxBytes: 2 * GiB,
			DiskReservePercent:          5,
			DiskReserveMinBytes:         GiB,
			DiskReserveMaxBytes:         8 * GiB,
			MaxConcurrency:              2,
			MaxCPUUtilizationPercent:    92,
			Lineage:                     LineageConstrained,
		},
		profileMinimal: {
			Name:                        profileMinimal,
			MemoryReservePercent:        5,
			MemoryReserveMinBytes:       128 * MiB,
			MemoryReserveMaxBytes:       512 * MiB,
			NoSwapMemoryReservePercent:  10,
			NoSwapMemoryReserveMinBytes: 256 * MiB,
			NoSwapMemoryReserveMaxBytes: 768 * MiB,
			DiskReservePercent:          2,
			DiskReserveMinBytes:         HardDiskFloorBytes,
			DiskReserveMaxBytes:         GiB,
			MaxConcurrency:              1,
			MaxCPUUtilizationPercent:    98,
			Lineage:                     LineageMinimal,
		},
	}}
}

func clampPercent(capacity int64, percentage float64, minimum, maximum int64) int64 {
	value := int64(math.Ceil(float64(capacity) * percentage / 100))
	return min(maximum, max(minimum, value))
}

func sampleMemoryLimit(sample Sample) int64 {
	if sample.EffectiveMemoryLimitBytes > 0 {
		return sample.EffectiveMemoryLimitBytes
	}

	return sample.PhysicalMemoryBytes
}

func profilePolicy(profile Profile, sample Sample) (Policy, int64, int64, int) {
	memoryCapacity := sampleMemoryLimit(sample)
	memoryReserve := clampPercent(memoryCapacity, profile.MemoryReservePercent, profile.MemoryReserveMinBytes, profile.MemoryReserveMaxBytes)
	if sample.SwapState == swapUnavailable {
		memoryReserve = clampPercent(
			memoryCapacity,
			profile.NoSwapMemoryReservePercent,
			profile.NoSwapMemoryReserveMinBytes,
			profile.NoSwapMemoryReserveMaxBytes,
		)
	}

	diskCapacity := int64(0)
	if sample.DiskTotalBytes != nil {
		diskCapacity = *sample.DiskTotalBytes
	} else if sample.DiskFreeBytes != nil {
		diskCapacity = *sample.DiskFreeBytes
	}

	diskReserve := clampPercent(diskCapacity, profile.DiskReservePercent, profile.DiskReserveMinBytes, profile.DiskReserveMaxBytes)
	parallelism := max(1, sample.AvailableParallelism)
	concurrency := max(1, parallelism-1)
	if profile.MaxConcurrency > 0 {
		concurrency = min(concurrency, profile.MaxConcurrency)
	}

	policy := DefaultPolicy()
	policy.AdmissionMemoryBytes = memoryReserve
	policy.WarningAdmissionMemoryBytes = clampPercent(memoryCapacity, 25, 4*GiB, 8*GiB)
	policy.CriticalMemoryBytes = max(64*MiB, memoryReserve/2)

	policy.DiskWarningBytes = diskReserve
	policy.DiskCriticalBytes = HardDiskFloorBytes

	policy.MaxCPUUtilizationPercent = profile.MaxCPUUtilizationPercent

	policy.SwapOutWarningBytes = clampPercent(memoryCapacity, .4, 64*MiB, 128*MiB)
	policy.SwapOutCriticalBytes = clampPercent(memoryCapacity, 1.6, 256*MiB, 512*MiB)

	policy.CompressorWarningPayloadBytes = clampPercent(memoryCapacity, 37.5, 0, math.MaxInt64)
	policy.CompressorWarningGrowthBytes = clampPercent(memoryCapacity, 3.125, 0, math.MaxInt64)
	policy.CompressorCriticalPayloadBytes = clampPercent(memoryCapacity, 50, 0, math.MaxInt64)
	policy.CompressorCriticalGrowthBytes = clampPercent(memoryCapacity, 6.25, 0, math.MaxInt64)

	return policy, memoryReserve, diskReserve, concurrency
}

func profileFits(sample Sample, policy Policy) bool {
	available := availableMemory(sample)
	cpuFits := sample.CPUUtilizationPercent == nil || CPUAdmissionReady(sample, policy)

	return available != nil &&
		*available >= policy.AdmissionMemoryBytes &&
		sample.DiskFreeBytes != nil &&
		*sample.DiskFreeBytes >= policy.DiskWarningBytes &&
		cpuFits
}

// Resolve chooses a concrete profile for one sample and task class.
func (catalog Catalog) Resolve(requested ProfileName, taskClass TaskClass, sample Sample) (Resolution, error) {
	if requested == "" {
		requested = catalog.DefaultProfile
	}
	if requested == "" {
		requested = profileBalanced
	}

	strictClass := taskClass == TaskTransactional || taskClass == TaskRelease
	seen := map[ProfileName]bool{}
	chain := []ProfileName{}
	current := requested

	for current != "" {
		if seen[current] {
			return Resolution{}, fmt.Errorf("profile fallback cycle at %q", current)
		}

		seen[current] = true
		profile, exists := catalog.Profiles[current]
		if !exists {
			return Resolution{}, fmt.Errorf("unknown resource profile %q", current)
		}

		if profile.Lineage == LineageUnset {
			return Resolution{}, fmt.Errorf("resource profile %q has no lineage", current)
		}

		chain = append(chain, current)
		policy, memoryReserve, diskReserve, concurrency := profilePolicy(profile, sample)
		resolution := Resolution{
			RequestedProfile:  requested,
			ResolvedProfile:   current,
			FallbackChain:     append([]ProfileName(nil), chain...),
			Strict:            strictClass || profile.Strict,
			Concurrency:       concurrency,
			MemoryReserve:     memoryReserve,
			DiskReserve:       diskReserve,
			Decision:          DecisionRun,
			DegradedAdmission: profile.Lineage.DegradedAdmission(),
			Lineage:           profile.Lineage,
			Policy:            policy,
		}

		if sample.DiskFreeBytes == nil || *sample.DiskFreeBytes < HardDiskFloorBytes {
			resolution.Decision, resolution.Reason = DecisionCleanup, ReasonStorageBlocked

			return resolution, nil
		}

		fits := profileFits(sample, policy)
		if fits || profile.Lineage.LastResortFloor() && profile.Fallback == "" && !resolution.Strict {
			if !fits {
				resolution.Policy.AdmissionMemoryBytes = resolution.Policy.CriticalMemoryBytes
				resolution.Policy.DiskWarningBytes = HardDiskFloorBytes
			}

			return resolution, nil
		}

		if resolution.Strict {
			resolution.Decision, resolution.Reason = DecisionReplan, ReasonReplanRequired

			return resolution, nil
		}

		current = profile.Fallback
	}

	return Resolution{}, errors.New("resource profile has no usable fallback")
}
