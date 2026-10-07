package evidence

import (
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/policy"
)

// RunSummary captures bounded aggregate evidence for one guarded task.
type RunSummary struct {
	SchemaVersion                          int                  `json:"schemaVersion"`
	RunID                                  string               `json:"runId,omitempty"`
	StartedAt                              string               `json:"startedAt,omitempty"`
	FinishedAt                             string               `json:"finishedAt,omitempty"`
	Source                                 string               `json:"source,omitempty"`
	Tags                                   map[string]string    `json:"tags,omitempty"`
	ResourceTier                           string               `json:"resourceTier,omitempty"`
	SampleCount                            int                  `json:"sampleCount"`
	TaskClass                              policy.TaskClass     `json:"taskClass"`
	Outcome                                Outcome              `json:"outcome"`
	AvailableParallelism                   int                  `json:"availableParallelism"`
	AvailableNonCompressedEstimateMinBytes *int64               `json:"availableNonCompressedEstimateMinBytes"`
	MemoryPressureLevelMax                 *int                 `json:"memoryPressureLevelMax"`
	CompressorAvailableAll                 bool                 `json:"compressorAvailableAll"`
	CompressorPayloadPeakBytes             *int64               `json:"compressorPayloadPeakBytes"`
	CPUUtilizationP95Percent               float64              `json:"cpuUtilizationP95Percent"`
	DiskFreeMinBytes                       *int64               `json:"diskFreeMinBytes"`
	SwapInsDelta                           int64                `json:"swapInsDelta"`
	SwapOutsDelta                          int64                `json:"swapOutsDelta"`
	SwapFreeMinBytes                       *int64               `json:"swapFreeMinBytes"`
	HealthFailures                         int                  `json:"healthFailures"`
	Platform                               string               `json:"platform,omitempty"`
	Capabilities                           []string             `json:"capabilities,omitempty"`
	RequestedProfile                       policy.ProfileName   `json:"requestedProfile,omitempty"`
	ResolvedProfile                        policy.ProfileName   `json:"resolvedProfile,omitempty"`
	FallbackChain                          []policy.ProfileName `json:"fallbackChain,omitempty"`
	Concurrency                            int                  `json:"concurrency,omitempty"`
	ConfigHash                             string               `json:"configHash,omitempty"`
	RequestedCPU                           int                  `json:"requestedCpu,omitempty"`
	RequestedMemoryBytes                   int64                `json:"requestedMemoryBytes,omitempty"`
	AllocatedCPU                           int                  `json:"allocatedCpu,omitempty"`
	AllocatedMemoryBytes                   int64                `json:"allocatedMemoryBytes,omitempty"`
	ReservationWaitMilliseconds            int64                `json:"reservationWaitMilliseconds,omitempty"`
	PeakOwnerCount                         int                  `json:"peakOwnerCount,omitempty"`
	BudgetOutcome                          BudgetOutcome        `json:"budgetOutcome,omitempty"`
}

// RunAccumulator keeps fixed-memory aggregates for one workload lifetime.
type RunAccumulator struct {
	summary     RunSummary
	first, last *policy.Sample
	cpu         *Histogram
	resolution  policy.Resolution
	configHash  string
}

// NewRunAccumulator creates schema-5 aggregate state for one run.
func NewRunAccumulator(identifier string) *RunAccumulator {
	return &RunAccumulator{summary: RunSummary{SchemaVersion: 5, RunID: identifier, CompressorAvailableAll: true}, cpu: NewHistogram(100, 0.01)}
}

// RunID returns the evidence identity independently of concrete writer state.
func (writer *RunAccumulator) RunID() string { return writer.summary.RunID }

// SetIdentity attaches validated privacy-safe labels to the completed summary.
func (writer *RunAccumulator) SetIdentity(metadata coordination.ReservationMetadata) {
	if writer == nil {
		return
	}
	writer.summary.Source = metadata.Source
	writer.summary.Tags = metadata.Tags
	writer.summary.ResourceTier = metadata.Tier
}

// SetReservationContext attaches only aggregate resource allocation metadata.
func (writer *RunAccumulator) SetReservationContext(session *coordination.LeaseMetadata, peakOwners int, outcome BudgetOutcome) {
	if writer == nil || session == nil {
		return
	}
	writer.summary.RequestedCPU = session.Requested.CPU
	writer.summary.RequestedMemoryBytes = session.Requested.MemoryBytes
	writer.summary.AllocatedCPU = session.Allocation.CPU
	writer.summary.AllocatedMemoryBytes = session.Allocation.MemoryBytes
	writer.summary.ReservationWaitMilliseconds = session.WaitDuration.Milliseconds()
	writer.ObserveReservationOwners(peakOwners)
	writer.summary.BudgetOutcome = outcome
}

// ObserveReservationOwners retains the highest shared-root owner count sampled during this child lifetime.
func (writer *RunAccumulator) ObserveReservationOwners(activeOwners int) {
	if writer == nil {
		return
	}

	writer.summary.PeakOwnerCount = max(writer.summary.PeakOwnerCount, activeOwners)
}

// SetContext attaches resolved, non-sensitive policy metadata to the summary.
func (writer *RunAccumulator) SetContext(resolution policy.Resolution, configHash string) {
	writer.resolution = resolution
	writer.configHash = configHash
}

func copyInt64(value *int64) *int64 {
	result := *value

	return &result
}

func copyInt(value *int) *int {
	result := *value

	return &result
}

func minimum(current, candidate *int64) *int64 {
	if candidate == nil {
		return current
	}
	if current == nil || *candidate < *current {
		return copyInt64(candidate)
	}

	return current
}

func maximumInt64(current, candidate *int64) *int64 {
	if candidate == nil {
		return current
	}
	if current == nil || *candidate > *current {
		return copyInt64(candidate)
	}

	return current
}

func maximumInt(current, candidate *int) *int {
	if candidate == nil {
		return current
	}
	if current == nil || *candidate > *current {
		return copyInt(candidate)
	}

	return current
}

// Append records one sample and updates fixed-memory lifetime aggregates.
func (writer *RunAccumulator) Append(sample policy.Sample) {
	if writer.first == nil {
		first := sample
		writer.first = &first
		writer.summary.AvailableParallelism = sample.AvailableParallelism
		writer.summary.Platform = sample.Platform
		writer.summary.Capabilities = append([]string(nil), sample.Capabilities...)
		writer.summary.StartedAt = sample.MeasuredAt
	}

	last := sample
	writer.last = &last
	writer.summary.SampleCount++
	writer.summary.FinishedAt = sample.MeasuredAt

	available := sample.AvailableMemoryBytes
	if available == nil {
		available = sample.AvailableNonCompressedEstimateBytes
	}

	writer.summary.AvailableNonCompressedEstimateMinBytes = minimum(writer.summary.AvailableNonCompressedEstimateMinBytes, available)
	writer.summary.MemoryPressureLevelMax = maximumInt(writer.summary.MemoryPressureLevelMax, sample.MemoryPressureLevel)
	writer.summary.CompressorPayloadPeakBytes = maximumInt64(writer.summary.CompressorPayloadPeakBytes, sample.CompressorPayloadBytes)
	writer.summary.DiskFreeMinBytes = minimum(writer.summary.DiskFreeMinBytes, sample.DiskFreeBytes)
	writer.summary.SwapFreeMinBytes = minimum(writer.summary.SwapFreeMinBytes, sample.SwapFreeBytes)

	if sample.CompressorAvailable == nil || !*sample.CompressorAvailable {
		writer.summary.CompressorAvailableAll = false
	}
	if sample.CPUUtilizationPercent != nil {
		writer.cpu.Add(*sample.CPUUtilizationPercent)
	}
}

func delta(first, last *int64) int64 {
	if first == nil || last == nil || *last <= *first {
		return 0
	}

	return *last - *first
}

// Complete applies the final verdict to bounded lifetime aggregates.
func (writer *RunAccumulator) Complete(taskClass policy.TaskClass, outcome Outcome, healthFailures int) RunSummary {
	writer.summary.TaskClass = taskClass
	writer.summary.Outcome = outcome
	writer.summary.HealthFailures = healthFailures
	writer.summary.RequestedProfile = writer.resolution.RequestedProfile
	writer.summary.ResolvedProfile = writer.resolution.ResolvedProfile
	writer.summary.FallbackChain = append([]policy.ProfileName(nil), writer.resolution.FallbackChain...)
	writer.summary.Concurrency = writer.resolution.Concurrency
	writer.summary.ConfigHash = writer.configHash

	if writer.first != nil && writer.last != nil {
		writer.summary.SwapInsDelta = delta(writer.first.SwapIns, writer.last.SwapIns)
		writer.summary.SwapOutsDelta = delta(writer.first.SwapOuts, writer.last.SwapOuts)
	}
	if value, ok := writer.cpu.Quantile(0.95); ok {
		writer.summary.CPUUtilizationP95Percent = value
	}

	return writer.summary
}
