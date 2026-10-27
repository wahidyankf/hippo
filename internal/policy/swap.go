package policy

import "math"

// SwapOverAllowanceAdvisory names the advisory an assessment reports while swap
// in use exceeds the allowance. It never changes the state.
const SwapOverAllowanceAdvisory = "swap-over-allowance"

// SwapAllowanceFor derives the swap allowance a sample may count as memory: the
// smaller of swap total and the effective memory limit rounded up to whole GiB,
// with physical memory standing in for an unset limit. It is zero wherever swap
// is unavailable, unreadable, or not reported by Linux, so counting never
// invents headroom.
func SwapAllowanceFor(sample Sample) int64 {
	if sample.Platform != "linux" || sample.SwapTotalBytes == nil || sample.SwapUsedBytes == nil {
		return 0
	}
	if sample.SwapState != "active" && sample.SwapState != "idle" {
		return 0
	}
	memoryLimit := sample.EffectiveMemoryLimitBytes
	if memoryLimit <= 0 {
		memoryLimit = sample.PhysicalMemoryBytes
	}

	return max(0, min(ceilGiB(memoryLimit), *sample.SwapTotalBytes))
}

// ceilGiB rounds a byte count up to whole GiB, saturating where the rounded value
// would overflow.
func ceilGiB(bytes int64) int64 {
	if bytes <= 0 {
		return 0
	}
	if bytes > math.MaxInt64-GiB {
		return math.MaxInt64
	}

	return (bytes + GiB - 1) / GiB * GiB
}

// WithSwapAllowance records the swap allowance on sample when counting is on, and
// leaves no allowance field at all when it is off, so a counting-off sample
// serializes exactly as it did before counting existed. It returns a copy.
func WithSwapAllowance(sample Sample, enabled bool) Sample {
	if !enabled {
		sample.SwapAllowanceBytes = nil

		return sample
	}
	allowance := SwapAllowanceFor(sample)
	sample.SwapAllowanceBytes = &allowance

	return sample
}

// CountedSwapAllowance is the allowance a sample counts, zero when counting is off.
func CountedSwapAllowance(sample Sample) int64 {
	if sample.SwapAllowanceBytes == nil {
		return 0
	}

	return *sample.SwapAllowanceBytes
}

// SwapHeadroomBytes is the allowance less the swap in use, never negative: the
// memory a counting sample adds beyond RAM. It is nil when counting is off, and
// zero when swap in use cannot be read.
func SwapHeadroomBytes(sample Sample) *int64 {
	if sample.SwapAllowanceBytes == nil {
		return nil
	}
	headroom := int64(0)
	if used, readable := swapInUse(sample); readable {
		headroom = max(0, *sample.SwapAllowanceBytes-used)
	}

	return &headroom
}

// CountedAvailableMemory is the available memory the policy decides on: the raw
// reading plus the swap headroom. It falls back to the non-compressed estimate as
// the raw reading does, and it is nil when there is no reading at all.
func CountedAvailableMemory(sample Sample) *int64 {
	return availableMemory(sample)
}

// rawAvailableMemory is the reading a host reports before any swap is counted.
func rawAvailableMemory(sample Sample) *int64 {
	if sample.AvailableMemoryBytes != nil {
		return sample.AvailableMemoryBytes
	}

	return sample.AvailableNonCompressedEstimateBytes
}

func swapInUse(sample Sample) (int64, bool) {
	if sample.SwapUsedBytes == nil {
		return 0, false
	}

	return *sample.SwapUsedBytes, true
}

// swapRateWaived reports whether the per-machine swap gate excuses the
// byte-based swap reasons for one sample: the policy names a positive multiple
// and readable swap in use fits within that multiple of physical memory. It is
// false wherever swap in use cannot be read, so an unreadable swap never
// waives the rate rules by default.
func swapRateWaived(sample Sample, policy Policy) bool {
	if policy.SwapUsedMaxMultiple <= 0 || sample.PhysicalMemoryBytes <= 0 {
		return false
	}
	used, readable := swapInUse(sample)
	if !readable {
		return false
	}

	return float64(used) <= policy.SwapUsedMaxMultiple*float64(sample.PhysicalMemoryBytes)
}

// countsAllowance reports whether the sample counts a non-zero allowance. Only
// then do the swap rules below differ from a host that does not count swap.
func countsAllowance(sample Sample) bool {
	return CountedSwapAllowance(sample) > 0
}

// swapWithinAllowance reports a counted sample whose swap in use is no more than
// its allowance.
func swapWithinAllowance(sample Sample) bool {
	used, readable := swapInUse(sample)

	return readable && countsAllowance(sample) && used <= CountedSwapAllowance(sample)
}

// swapOverAllowance reports a counted sample whose swap in use exceeds its allowance.
func swapOverAllowance(sample Sample) bool {
	used, readable := swapInUse(sample)

	return readable && countsAllowance(sample) && used > CountedSwapAllowance(sample)
}

// psiReadable reports whether the host exposes memory pressure stall information.
func psiReadable(sample Sample) bool {
	return sample.MemoryPSISomeAvg10 != nil || sample.MemoryPSIFullAvg10 != nil
}

// saturatingAdd adds a non-negative headroom without overflowing.
func saturatingAdd(value, headroom int64) int64 {
	if headroom > 0 && value > math.MaxInt64-headroom {
		return math.MaxInt64
	}

	return value + headroom
}
