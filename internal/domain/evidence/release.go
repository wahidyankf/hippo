package evidence

import (
	"errors"
	"math"

	"github.com/wahidyankf/hippo/internal/policy"
)

// MaximumProbeLatencyMs bounds the release health histograms.
const MaximumProbeLatencyMs = 3000.0

// ReleaseSample records one resource and health observation.
type ReleaseSample struct {
	policy.Sample

	OneMinuteLoad          float64 `json:"oneMinuteLoad"`
	ServiceRSSBytes        int64   `json:"serviceRssBytes"`
	HealthStatus           int     `json:"healthStatus"`
	HealthLatencyMs        float64 `json:"healthLatencyMs"`
	RoutedJourneyStatus    int     `json:"routedJourneyStatus"`
	RoutedJourneyLatencyMs float64 `json:"routedJourneyLatencyMs"`
}

// ReleaseAccumulator aggregates the complete release session without retaining raw samples.
type ReleaseAccumulator struct {
	summary       policy.ReleaseSummary
	first         *policy.Sample
	last          *policy.Sample
	cpu           *Histogram
	healthLatency *Histogram
	routedLatency *Histogram
}

// NewReleaseAccumulator starts an empty schema-5 summary.
func NewReleaseAccumulator() *ReleaseAccumulator {
	return &ReleaseAccumulator{
		summary: policy.ReleaseSummary{
			SchemaVersion:                          5,
			CompressorAvailableAll:                 true,
			MemoryPressureLevelMax:                 0,
			AvailableNonCompressedEstimateMinBytes: math.MaxInt64,
			DiskFreeMinBytes:                       math.MaxInt64,
			SwapFreeMinBytes:                       math.MaxInt64,
		},
		cpu:           NewHistogram(100, 0.01),
		healthLatency: NewHistogram(MaximumProbeLatencyMs, 0.1),
		routedLatency: NewHistogram(MaximumProbeLatencyMs, 0.1),
	}
}

// Add incorporates one successfully written sample.
func (accumulator *ReleaseAccumulator) Add(sample ReleaseSample) {
	if accumulator.first == nil {
		first := sample.Sample
		accumulator.first = &first
		accumulator.summary.Platform = sample.Platform
		accumulator.summary.Capabilities = append([]string(nil), sample.Capabilities...)
		accumulator.summary.AvailableParallelism = sample.AvailableParallelism
	}

	last := sample.Sample
	accumulator.last = &last
	accumulator.summary.SampleCount++

	available := sample.AvailableMemoryBytes
	if available == nil {
		available = sample.AvailableNonCompressedEstimateBytes
	}

	if available != nil {
		accumulator.summary.AvailableNonCompressedEstimateMinBytes = min(accumulator.summary.AvailableNonCompressedEstimateMinBytes, *available)
	}
	if sample.MemoryPressureLevel != nil {
		accumulator.summary.MemoryPressureLevelMax = max(accumulator.summary.MemoryPressureLevelMax, *sample.MemoryPressureLevel)
	}
	if sample.CompressorAvailable == nil || !*sample.CompressorAvailable {
		accumulator.summary.CompressorAvailableAll = false
	}
	if sample.CompressorPayloadBytes != nil {
		accumulator.summary.CompressorPayloadPeakBytes = max(accumulator.summary.CompressorPayloadPeakBytes, *sample.CompressorPayloadBytes)
	}
	if sample.CPUUtilizationPercent != nil {
		accumulator.cpu.Add(*sample.CPUUtilizationPercent)
	}
	if sample.DiskFreeBytes != nil {
		accumulator.summary.DiskFreeMinBytes = min(accumulator.summary.DiskFreeMinBytes, *sample.DiskFreeBytes)
	}
	if sample.SwapFreeBytes != nil {
		accumulator.summary.SwapFreeMinBytes = min(accumulator.summary.SwapFreeMinBytes, *sample.SwapFreeBytes)
	}

	accumulator.summary.ServiceRSSPeakBytes = max(accumulator.summary.ServiceRSSPeakBytes, sample.ServiceRSSBytes)
	accumulator.summary.RoutedJourneyLatencyMaxMs = max(accumulator.summary.RoutedJourneyLatencyMaxMs, sample.RoutedJourneyLatencyMs)
	accumulator.healthLatency.Add(min(sample.HealthLatencyMs, MaximumProbeLatencyMs))
	accumulator.routedLatency.Add(min(sample.RoutedJourneyLatencyMs, MaximumProbeLatencyMs))

	if sample.HealthStatus != 200 {
		accumulator.summary.HealthFailures++
	}
	if sample.RoutedJourneyStatus != 200 {
		accumulator.summary.RoutedJourneyFailures++
	}
}

// Result returns the summary of all incorporated samples.
func (accumulator *ReleaseAccumulator) Result() policy.ReleaseSummary {
	summary := accumulator.summary
	if accumulator.first == nil || accumulator.last == nil {
		return summary
	}

	summary.PhysicalMemoryBytes = accumulator.first.PhysicalMemoryBytes
	if accumulator.first.SwapIns != nil && accumulator.last.SwapIns != nil {
		summary.SwapInsDelta = max(0, *accumulator.last.SwapIns-*accumulator.first.SwapIns)
	}
	if accumulator.first.SwapOuts != nil && accumulator.last.SwapOuts != nil {
		summary.SwapOutsDelta = max(0, *accumulator.last.SwapOuts-*accumulator.first.SwapOuts)
	}

	if value, ok := accumulator.cpu.Quantile(0.95); ok {
		summary.CPUUtilizationP95Percent = value
	}
	if value, ok := accumulator.healthLatency.Quantile(0.95); ok {
		summary.HealthLatencyP95Ms = value
	}
	if value, ok := accumulator.routedLatency.Quantile(0.95); ok {
		summary.RoutedJourneyLatencyP95Ms = value
	}

	return summary
}

// ErrReleaseHeadroomExhausted marks a well-formed summary outside the release envelope.
var ErrReleaseHeadroomExhausted = errors.New("release overlap exhausted resource or routed responsiveness headroom")

// ValidateReleaseSummary checks the supported evidence shape and release envelope.
func ValidateReleaseSummary(summary policy.ReleaseSummary) error {
	if summary.SchemaVersion < 2 || summary.SchemaVersion > 5 || summary.SampleCount <= 0 || summary.AvailableParallelism <= 0 {
		return errors.New("resource evidence summary is invalid")
	}
	if !policy.ReleaseHeadroomAvailable(summary) {
		return ErrReleaseHeadroomExhausted
	}
	return nil
}
