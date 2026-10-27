package policy

import (
	"context"
	"math"
	"slices"
	"sort"
	"time"
)

const (
	// GiB is one binary gibibyte.
	GiB = int64(1024 * 1024 * 1024)
	// MiB is one binary mebibyte.
	MiB                  = int64(1024 * 1024)
	swapUnavailableState = "unavailable"
	platformDarwin       = "darwin"
	// ReleaseRoutedLatencyP95BudgetMs is the hard routed-user-surface p95 ceiling during release overlap.
	ReleaseRoutedLatencyP95BudgetMs = 500.0
	// ReleaseRoutedLatencyMaxBudgetMs is the hard single routed-user-surface sample ceiling during release overlap.
	ReleaseRoutedLatencyMaxBudgetMs = 2_000.0
)

// State is a resource-pressure classification emitted in evidence JSON.
type State string

// StateNormal indicates safe resource headroom.
const StateNormal State = "normal"

// StateWarning indicates pressure that may recover within a grace period.
const StateWarning State = "warning"

// StateCritical indicates pressure that requires immediate refusal or shedding.
const StateCritical State = "critical"

// Sample is one timestamped host resource observation.
type Sample struct {
	SchemaVersion                       int      `json:"schemaVersion"`
	MeasuredAt                          string   `json:"measuredAt"`
	Platform                            string   `json:"platform,omitempty"`
	Capabilities                        []string `json:"capabilities,omitempty"`
	EffectiveMemoryLimitBytes           int64    `json:"effectiveMemoryLimitBytes,omitempty"`
	AvailableMemoryBytes                *int64   `json:"availableMemoryBytes,omitempty"`
	AvailableNonCompressedEstimateBytes *int64   `json:"availableNonCompressedEstimateBytes"`
	MemoryPressureLevel                 *int     `json:"memoryPressureLevel"`
	CompressorAvailable                 *bool    `json:"compressorAvailable"`
	CompressorPayloadBytes              *int64   `json:"compressorPayloadBytes"`
	PhysicalMemoryBytes                 int64    `json:"physicalMemoryBytes"`
	AvailableParallelism                int      `json:"availableParallelism"`
	CPUUtilizationPercent               *float64 `json:"cpuUtilizationPercent"`
	DiskFreeBytes                       *int64   `json:"diskFreeBytes"`
	DiskTotalBytes                      *int64   `json:"diskTotalBytes,omitempty"`
	PageSizeBytes                       *int64   `json:"pageSizeBytes"`
	CompressorStoredPages               *int64   `json:"compressorStoredPages"`
	CompressorOccupiedPages             *int64   `json:"compressorOccupiedPages"`
	SwapIns                             *int64   `json:"swapIns"`
	SwapOuts                            *int64   `json:"swapOuts"`
	SwapTotalBytes                      *int64   `json:"swapTotalBytes"`
	SwapUsedBytes                       *int64   `json:"swapUsedBytes"`
	SwapFreeBytes                       *int64   `json:"swapFreeBytes"`
	SwapAllowanceBytes                  *int64   `json:"swapAllowanceBytes,omitempty"`
	SwapState                           string   `json:"swapState,omitempty"`
	MemoryPSISomeAvg10                  *float64 `json:"memoryPsiSomeAvg10,omitempty"`
	MemoryPSIFullAvg10                  *float64 `json:"memoryPsiFullAvg10,omitempty"`
	OOMEvents                           *int64   `json:"oomEvents,omitempty"`
	OOMKillEvents                       *int64   `json:"oomKillEvents,omitempty"`
}

// CPUState contains cumulative CPU counters used to calculate utilization.
type CPUState []uint64

// Reading combines one sample with the counters needed by the next collection.
type Reading struct {
	CPUState CPUState
	Sample   Sample
}

// Collector produces host readings for admission and monitoring.
type Collector interface {
	Collect(ctx context.Context, previous CPUState, diskPath string) (Reading, error)
}

// Policy defines thresholds, timing, and resource reservations.
type Policy struct {
	AdmissionMemoryBytes        int64
	WarningAdmissionMemoryBytes int64
	CriticalMemoryBytes         int64
	// WarningMemoryPercent is the optional RAM-proportional warning threshold,
	// in percent of the effective memory limit with physical memory standing in
	// for an unset limit. Zero or negative keeps the legacy fixed admission
	// thresholds, so a policy that never names it behaves as before.
	WarningMemoryPercent float64
	// CriticalMemoryPercent is the optional RAM-proportional critical threshold,
	// shaped like the warning one. Zero or negative keeps the legacy fixed
	// critical threshold.
	CriticalMemoryPercent float64
	// WarningMemoryFloorBytes floors the proportional warning threshold.
	// CriticalMemoryFloorBytes floors the proportional critical threshold.
	// Either is read only while its percent is positive.
	WarningMemoryFloorBytes  int64
	CriticalMemoryFloorBytes int64
	// SwapUsedMaxMultiple is the optional per-machine swap gate: while readable
	// swap in use fits within this multiple of physical memory, the byte-based
	// swap reasons are waived. Zero or negative keeps the legacy swap rules.
	SwapUsedMaxMultiple            float64
	DiskWarningBytes               int64
	DiskCriticalBytes              int64
	TrendWindow                    time.Duration
	SwapOutWarningBytes            int64
	SwapOutCriticalBytes           int64
	CompressorWarningPayloadBytes  int64
	CompressorWarningGrowthBytes   int64
	CompressorCriticalPayloadBytes int64
	CompressorCriticalGrowthBytes  int64
	ReservedCPUUnits               int
	MaxCPUUtilizationPercent       float64
	ConsecutiveCPUSamples          int
	AdmissionWindow                time.Duration
	EphemeralWarningGrace          time.Duration
	ServiceWarningGrace            time.Duration
	TerminationGrace               time.Duration
	LeaseWait                      time.Duration
	SampleInterval                 time.Duration
}

// DefaultPolicy returns an independent guarded-development policy value.
func DefaultPolicy() Policy {
	return Policy{
		AdmissionMemoryBytes:        9 * GiB,
		WarningAdmissionMemoryBytes: 8 * GiB,
		CriticalMemoryBytes:         4 * GiB,

		DiskWarningBytes:  30 * GiB,
		DiskCriticalBytes: 20 * GiB,

		TrendWindow:          15 * time.Second,
		SwapOutWarningBytes:  128 * MiB,
		SwapOutCriticalBytes: 512 * MiB,

		CompressorWarningPayloadBytes:  12 * GiB,
		CompressorWarningGrowthBytes:   1 * GiB,
		CompressorCriticalPayloadBytes: 16 * GiB,
		CompressorCriticalGrowthBytes:  2 * GiB,

		ReservedCPUUnits:         2,
		MaxCPUUtilizationPercent: 85,
		ConsecutiveCPUSamples:    3,

		AdmissionWindow:       16 * time.Second,
		EphemeralWarningGrace: 10 * time.Second,
		ServiceWarningGrace:   30 * time.Second,
		TerminationGrace:      10 * time.Second,

		LeaseWait:      5 * time.Minute,
		SampleInterval: time.Second,
	}
}

// MemoryPolicy is the optional RAM-proportional admission override a
// coordination file may carry. Every field is zero when the file names no
// block, which keeps the legacy fixed thresholds.
type MemoryPolicy struct {
	WarningPercent     float64
	CriticalPercent    float64
	WarningFloorBytes  int64
	CriticalFloorBytes int64
}

// ApplyCoordinationThresholds returns base with the coordination admission
// knobs applied: the RAM-proportional memory thresholds and the per-machine
// swap gate. Zero values leave the legacy fixed thresholds in force, so a
// coordination without either knob resolves the policy it always did.
func ApplyCoordinationThresholds(base Policy, memory MemoryPolicy, swapUsedMaxMultiple float64) Policy {
	base.WarningMemoryPercent = memory.WarningPercent
	base.CriticalMemoryPercent = memory.CriticalPercent
	base.WarningMemoryFloorBytes = memory.WarningFloorBytes
	base.CriticalMemoryFloorBytes = memory.CriticalFloorBytes
	base.SwapUsedMaxMultiple = swapUsedMaxMultiple

	return base
}

// proportionalThreshold is percent of a memory limit in whole bytes, rounding
// up, saturating where the rounded value would overflow. It is zero without a
// positive limit and percent.
func proportionalThreshold(limit int64, percent float64) int64 {
	if limit <= 0 || percent <= 0 {
		return 0
	}
	scaled := float64(limit) * percent / 100
	if scaled >= float64(math.MaxInt64) {
		return math.MaxInt64
	}

	return int64(math.Ceil(scaled))
}

// memoryThresholds resolves the available-memory floors a sample is decided
// against: the RAM-proportional thresholds where the policy names a percent,
// the legacy fixed thresholds otherwise. The caller names the legacy warning
// floor it already reads, since the normal boundary and the stable-warning
// degraded boundary keep their own.
func memoryThresholds(sample Sample, policy Policy, legacyWarning int64) (int64, int64) {
	warning, critical := legacyWarning, policy.CriticalMemoryBytes
	if policy.WarningMemoryPercent > 0 {
		warning = max(
			proportionalThreshold(sampleMemoryLimit(sample), policy.WarningMemoryPercent),
			policy.WarningMemoryFloorBytes,
		)
	}
	if policy.CriticalMemoryPercent > 0 {
		critical = max(
			proportionalThreshold(sampleMemoryLimit(sample), policy.CriticalMemoryPercent),
			policy.CriticalMemoryFloorBytes,
		)
	}

	return warning, critical
}

// Assessment classifies current and trend-based resource evidence.
type Assessment struct {
	CompressorGrowthWindowBytes float64 `json:"compressorGrowthWindowBytes"`
	SwapOutWindowBytes          float64 `json:"swapOutWindowBytes"`
	Reason                      string  `json:"reason"`
	State                       State   `json:"state"`
	StorageBlocked              bool    `json:"storageBlocked"`
	SwapState                   string  `json:"swapState,omitempty"`
	// SwapHeadroomBytes and CountedAvailableMemoryBytes are present only while
	// swap counting is on. Advisories never change State.
	SwapHeadroomBytes           *int64   `json:"swapHeadroomBytes,omitempty"`
	CountedAvailableMemoryBytes *int64   `json:"countedAvailableMemoryBytes,omitempty"`
	Advisories                  []string `json:"advisories,omitempty"`
}

func ptrFinite[T ~int | ~int64](value *T) bool {
	return value != nil
}

// EssentialReadingsValid reports whether required admission evidence is usable.
func EssentialReadingsValid(sample Sample) bool {
	_, timeError := time.Parse(time.RFC3339Nano, sample.MeasuredAt)
	return ptrFinite(availableMemory(sample)) && timeError == nil && sample.DiskFreeBytes != nil && sample.AvailableParallelism > 0
}

// availableMemory is the memory every threshold reads: the raw reading plus the
// swap headroom a counting sample adds. Without counting it is the raw reading.
func availableMemory(sample Sample) *int64 {
	available := rawAvailableMemory(sample)
	headroom := SwapHeadroomBytes(sample)
	if available == nil || headroom == nil {
		return available
	}
	counted := saturatingAdd(*available, *headroom)

	return &counted
}

// psiCriticalBand is the memory pressure that is critical whatever the swap.
func psiCriticalBand(sample Sample) bool {
	return sample.MemoryPSISomeAvg10 != nil && *sample.MemoryPSISomeAvg10 >= 25 ||
		sample.MemoryPSIFullAvg10 != nil && *sample.MemoryPSIFullAvg10 >= 5
}

// psiWarningBand is the some-average warning band. Within the swap allowance
// swapping is expected, so the band does not count there.
func psiWarningBand(sample Sample) bool {
	return sample.MemoryPSISomeAvg10 != nil && *sample.MemoryPSISomeAvg10 >= 10 && !swapWithinAllowance(sample)
}

// recordSwapCounting fills the counted-memory fields when the sample carries an
// allowance, and reports swap use beyond it as an advisory.
func recordSwapCounting(result *Assessment, sample Sample) {
	if sample.SwapAllowanceBytes != nil {
		result.SwapHeadroomBytes = SwapHeadroomBytes(sample)
		result.CountedAvailableMemoryBytes = availableMemory(sample)
	}
	if swapOverAllowance(sample) {
		result.Advisories = []string{SwapOverAllowanceAdvisory}
	}
}

// memoryReason names the evidence behind a memory state: PSI when a PSI band
// applies, otherwise the state itself.
func memoryReason(sample Sample, state State) string {
	if psiCriticalBand(sample) {
		return "memory-psi"
	}
	if psiWarningBand(sample) {
		return "memory-psi"
	}
	return "memory-" + string(state)
}

func hasCapability(sample Sample, expected string) bool {
	return slices.Contains(sample.Capabilities, expected)
}

// MemoryState classifies memory evidence as normal, warning, or critical.
func MemoryState(sample Sample, policy Policy) State {
	if !EssentialReadingsValid(sample) {
		return StateCritical
	}

	available := availableMemory(sample)
	warning, critical := memoryThresholds(sample, policy, policy.AdmissionMemoryBytes)
	criticalPressure := sample.MemoryPressureLevel != nil && *sample.MemoryPressureLevel == 4
	compressorFailed := sample.CompressorAvailable != nil &&
		!*sample.CompressorAvailable &&
		(sample.Platform == "" || hasCapability(sample, "compressor"))
	if criticalPressure || compressorFailed || psiCriticalBand(sample) || *available < critical {
		return StateCritical
	}

	warningPressure := sample.MemoryPressureLevel != nil && *sample.MemoryPressureLevel == 2

	if warningPressure || psiWarningBand(sample) || *available < warning {
		return StateWarning
	}

	return StateNormal
}

// CPUAdmissionReady reports whether utilization preserves reserved execution units.
func CPUAdmissionReady(sample Sample, policy Policy) bool {
	if sample.CPUUtilizationPercent == nil ||
		math.IsNaN(*sample.CPUUtilizationPercent) ||
		math.IsInf(*sample.CPUUtilizationPercent, 0) ||
		*sample.CPUUtilizationPercent < 0 ||
		*sample.CPUUtilizationPercent > 100 ||
		sample.AvailableParallelism <= 0 {
		return false
	}

	ceiling := policy.MaxCPUUtilizationPercent
	if ceiling <= 0 {
		reserved := min(policy.ReservedCPUUnits, sample.AvailableParallelism)
		ceiling = 100 * (1 - float64(reserved)/float64(sample.AvailableParallelism))
	}

	return *sample.CPUUtilizationPercent <= ceiling
}

func scaledWindowDelta(samples []Sample, value func(Sample) *int64, multiplier int64, policy Policy) float64 {
	current := samples[len(samples)-1]
	currentValue := value(current)
	currentTime, err := time.Parse(time.RFC3339Nano, current.MeasuredAt)
	if err != nil || currentValue == nil {
		return 0
	}

	for _, candidate := range samples[:len(samples)-1] {
		candidateTime, parseError := time.Parse(time.RFC3339Nano, candidate.MeasuredAt)
		candidateValue := value(candidate)
		deltaTime := currentTime.Sub(candidateTime)

		if parseError != nil || candidateValue == nil || deltaTime <= 0 || deltaTime > policy.TrendWindow {
			continue
		}

		delta := *currentValue - *candidateValue
		if delta <= 0 {
			return 0
		}

		return float64(delta*multiplier) * float64(policy.TrendWindow) / float64(deltaTime)
	}

	return 0
}

// ResourceAssessment classifies the newest sample and bounded pressure trends.
func ResourceAssessment(samples []Sample, policy Policy) Assessment { //nolint:cyclop // Ordered independent resource signals must remain visibly auditable.
	if len(samples) == 0 {
		return Assessment{Reason: "missing-sample", State: StateCritical}
	}

	current := samples[len(samples)-1]
	pageSize := int64(0)
	if current.PageSizeBytes != nil {
		pageSize = *current.PageSizeBytes
	}

	result := Assessment{
		CompressorGrowthWindowBytes: scaledWindowDelta(samples, func(s Sample) *int64 { return s.CompressorPayloadBytes }, 1, policy),
		SwapOutWindowBytes:          scaledWindowDelta(samples, func(s Sample) *int64 { return s.SwapOuts }, pageSize, policy),
		Reason:                      string(StateNormal), State: StateNormal, SwapState: current.SwapState,
	}

	recordSwapCounting(&result, current)

	if current.DiskFreeBytes == nil {
		result.Reason, result.State, result.StorageBlocked = "disk-unavailable", StateCritical, true

		return result
	}

	result.StorageBlocked = *current.DiskFreeBytes < policy.DiskWarningBytes
	type candidate struct {
		reason string
		state  State
	}
	candidates := []candidate{}

	if len(samples) > 1 {
		previous := samples[len(samples)-2]
		oomIncreased := current.OOMEvents != nil && previous.OOMEvents != nil && *current.OOMEvents > *previous.OOMEvents
		oomKillIncreased := current.OOMKillEvents != nil && previous.OOMKillEvents != nil && *current.OOMKillEvents > *previous.OOMKillEvents
		if oomIncreased || oomKillIncreased {
			candidates = append(candidates, candidate{reason: "memory-oom", state: StateCritical})
		}
	}

	if *current.DiskFreeBytes < policy.DiskCriticalBytes {
		candidates = append(candidates, candidate{reason: "disk-critical", state: StateCritical})
	} else if result.StorageBlocked {
		candidates = append(candidates, candidate{reason: "disk-warning", state: StateWarning})
	}

	if state := MemoryState(current, policy); state != StateNormal {
		candidates = append(candidates, candidate{reason: memoryReason(current, state), state: state})
	}

	// While swap counts and PSI is readable, PSI is the memory signal, so the
	// swap-out rate is not assessed. The per-machine swap gate waives the same
	// byte-based swap reasons while readable swap in use fits within its
	// multiple of physical memory; every other signal stays in force.
	swapRateAssessed := (!countsAllowance(current) || !psiReadable(current)) && !swapRateWaived(current, policy)
	if swapRateAssessed && current.SwapState != swapUnavailableState && result.SwapOutWindowBytes >= float64(policy.SwapOutCriticalBytes) {
		result.SwapState = "stressed"
		candidates = append(candidates, candidate{reason: "swap-critical", state: StateCritical})
	} else if swapRateAssessed && current.SwapState != swapUnavailableState && result.SwapOutWindowBytes >= float64(policy.SwapOutWarningBytes) {
		result.SwapState = "stressed"
		candidates = append(candidates, candidate{reason: "swap-warning", state: StateWarning})
	}

	if current.CompressorPayloadBytes != nil &&
		*current.CompressorPayloadBytes >= policy.CompressorCriticalPayloadBytes &&
		result.CompressorGrowthWindowBytes >= float64(policy.CompressorCriticalGrowthBytes) {
		candidates = append(candidates, candidate{reason: "compressor-critical", state: StateCritical})
	} else if current.CompressorPayloadBytes != nil &&
		*current.CompressorPayloadBytes >= policy.CompressorWarningPayloadBytes &&
		result.CompressorGrowthWindowBytes >= float64(policy.CompressorWarningGrowthBytes) {
		candidates = append(candidates, candidate{reason: "compressor-warning", state: StateWarning})
	}

	severity := map[State]int{StateNormal: 0, StateWarning: 1, StateCritical: 2}
	sort.SliceStable(candidates, func(i, j int) bool { return severity[candidates[i].state] > severity[candidates[j].state] })

	if len(candidates) > 0 {
		result.Reason, result.State = candidates[0].reason, candidates[0].state
	}

	return result
}

// AdmissionReady requires a normal assessment and consecutive safe CPU samples.
func AdmissionReady(samples []Sample, policy Policy) bool {
	if ResourceAssessment(samples, policy).State != StateNormal || len(samples) < policy.ConsecutiveCPUSamples {
		return false
	}

	for _, sample := range samples[len(samples)-policy.ConsecutiveCPUSamples:] {
		if !CPUAdmissionReady(sample, policy) {
			return false
		}
	}

	return true
}

func warningWindowReady(samples []Sample, policy Policy) bool {
	if len(samples) < policy.ConsecutiveCPUSamples || policy.TrendWindow <= 0 {
		return false
	}
	warning, _ := memoryThresholds(samples[len(samples)-1], policy, policy.WarningAdmissionMemoryBytes)
	if warning <= 0 {
		return false
	}

	firstTime, firstError := time.Parse(time.RFC3339Nano, samples[0].MeasuredAt)
	lastTime, lastError := time.Parse(time.RFC3339Nano, samples[len(samples)-1].MeasuredAt)

	return firstError == nil && lastError == nil && lastTime.Sub(firstTime) >= policy.TrendWindow
}

// WarningAdmissionReady permits only a full, stable Darwin warning window with conservative headroom.
func WarningAdmissionReady(samples []Sample, policy Policy) bool { //nolint:cyclop // Every independent safety signal remains visible.
	if !warningWindowReady(samples, policy) {
		return false
	}

	current := samples[len(samples)-1]
	if current.Platform != platformDarwin || current.MemoryPressureLevel == nil || *current.MemoryPressureLevel != 2 {
		return false
	}

	assessment := ResourceAssessment(samples, policy)
	if assessment.State == StateCritical || assessment.StorageBlocked ||
		assessment.SwapOutWindowBytes >= float64(policy.SwapOutWarningBytes) ||
		assessment.CompressorGrowthWindowBytes >= float64(policy.CompressorWarningGrowthBytes) {
		return false
	}

	for index, sample := range samples {
		available := availableMemory(sample)
		warning, _ := memoryThresholds(sample, policy, policy.WarningAdmissionMemoryBytes)
		if sample.Platform != platformDarwin || !EssentialReadingsValid(sample) || available == nil || *available < warning ||
			sample.MemoryPressureLevel == nil || *sample.MemoryPressureLevel == 4 ||
			sample.CompressorAvailable == nil || !*sample.CompressorAvailable ||
			sample.DiskFreeBytes == nil || *sample.DiskFreeBytes < policy.DiskWarningBytes {
			return false
		}

		if index > 0 {
			previous := samples[index-1]
			oomIncreased := sample.OOMEvents != nil && previous.OOMEvents != nil && *sample.OOMEvents > *previous.OOMEvents
			oomKillIncreased := sample.OOMKillEvents != nil && previous.OOMKillEvents != nil && *sample.OOMKillEvents > *previous.OOMKillEvents
			if oomIncreased || oomKillIncreased {
				return false
			}
		}
	}

	for _, sample := range samples[len(samples)-policy.ConsecutiveCPUSamples:] {
		if !CPUAdmissionReady(sample, policy) {
			return false
		}
	}

	return true
}

// ReleaseSummary contains aggregate release-monitor evidence.
type ReleaseSummary struct {
	SchemaVersion                          int      `json:"schemaVersion"`
	Platform                               string   `json:"platform,omitempty"`
	Capabilities                           []string `json:"capabilities,omitempty"`
	SampleCount                            int      `json:"sampleCount"`
	AvailableParallelism                   int      `json:"availableParallelism"`
	AvailableNonCompressedEstimateMinBytes int64    `json:"availableNonCompressedEstimateMinBytes"`
	MemoryPressureLevelMax                 int      `json:"memoryPressureLevelMax"`
	CompressorAvailableAll                 bool     `json:"compressorAvailableAll"`
	CompressorPayloadPeakBytes             int64    `json:"compressorPayloadPeakBytes"`
	PhysicalMemoryBytes                    int64    `json:"physicalMemoryBytes,omitempty"`
	CPUUtilizationP95Percent               float64  `json:"cpuUtilizationP95Percent"`
	ServiceRSSPeakBytes                    int64    `json:"serviceRssPeakBytes,omitempty"`
	DiskFreeMinBytes                       int64    `json:"diskFreeMinBytes"`
	SwapInsDelta                           int64    `json:"swapInsDelta"`
	SwapOutsDelta                          int64    `json:"swapOutsDelta"`
	SwapFreeMinBytes                       int64    `json:"swapFreeMinBytes"`
	HealthLatencyP95Ms                     float64  `json:"healthLatencyP95Ms,omitempty"`
	HealthFailures                         int      `json:"healthFailures"`
	RoutedJourneyLatencyP95Ms              float64  `json:"routedJourneyLatencyP95Ms,omitempty"`
	RoutedJourneyLatencyMaxMs              float64  `json:"routedJourneyLatencyMaxMs,omitempty"`
	RoutedJourneyFailures                  int      `json:"routedJourneyFailures,omitempty"`
}

// ReleaseHeadroomAvailable validates aggregate capacity for release overlap.
func ReleaseHeadroomAvailable(summary ReleaseSummary) bool {
	swapPressure := (summary.SwapInsDelta > 0 || summary.SwapOutsDelta > 0) && summary.AvailableNonCompressedEstimateMinBytes < 4*GiB

	if summary.AvailableParallelism <= 0 {
		return false
	}

	ceiling := 100 * (1 - 2/float64(summary.AvailableParallelism))
	compressorHealthy := summary.Platform != platformDarwin && summary.Platform != "" || summary.CompressorAvailableAll
	routedHealthy := summary.SchemaVersion < 4 ||
		summary.RoutedJourneyFailures == 0 &&
			summary.RoutedJourneyLatencyP95Ms > 0 &&
			summary.RoutedJourneyLatencyP95Ms <= ReleaseRoutedLatencyP95BudgetMs &&
			summary.RoutedJourneyLatencyMaxMs > 0 &&
			summary.RoutedJourneyLatencyMaxMs <= ReleaseRoutedLatencyMaxBudgetMs

	return summary.SampleCount > 0 &&
		summary.AvailableNonCompressedEstimateMinBytes >= 2*GiB &&
		summary.MemoryPressureLevelMax == 1 &&
		compressorHealthy &&
		!swapPressure &&
		summary.CPUUtilizationP95Percent <= ceiling &&
		summary.HealthFailures == 0 &&
		routedHealthy
}

// ReleaseMemoryAvailable reports whether one sample preserves release memory headroom.
func ReleaseMemoryAvailable(sample Sample) bool {
	available := availableMemory(sample)
	compressorHealthy := sample.Platform != platformDarwin || sample.CompressorAvailable != nil && *sample.CompressorAvailable

	return available != nil &&
		*available >= 9*GiB &&
		sample.MemoryPressureLevel != nil &&
		*sample.MemoryPressureLevel == 1 &&
		compressorHealthy
}
