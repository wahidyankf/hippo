package evidence_test

import (
	"errors"
	"testing"

	"github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
)

func TestReleaseSummaryValidationPreservesLegacyAndRejectsMissingEvidence(t *testing.T) {
	healthy := policy.ReleaseSummary{SchemaVersion: 3, SampleCount: 1, AvailableParallelism: 12, AvailableNonCompressedEstimateMinBytes: 13 * policy.GiB, MemoryPressureLevelMax: 1, CompressorAvailableAll: true, CPUUtilizationP95Percent: 10}
	cases := []struct {
		name      string
		mutate    func(*policy.ReleaseSummary)
		invalid   bool
		exhausted bool
	}{
		{name: "legacy accepted", mutate: func(*policy.ReleaseSummary) {}},
		{name: "old schema rejected", mutate: func(s *policy.ReleaseSummary) { s.SchemaVersion = 1 }, invalid: true},
		{name: "future schema rejected", mutate: func(s *policy.ReleaseSummary) { s.SchemaVersion = 6 }, invalid: true},
		{name: "no samples rejected", mutate: func(s *policy.ReleaseSummary) { s.SampleCount = 0 }, invalid: true},
		{name: "no parallelism rejected", mutate: func(s *policy.ReleaseSummary) { s.AvailableParallelism = 0 }, invalid: true},
		{name: "failed health rejected", mutate: func(s *policy.ReleaseSummary) { s.HealthFailures = 1 }, invalid: true, exhausted: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			summary := healthy
			test.mutate(&summary)
			err := evidence.ValidateReleaseSummary(summary)
			if (err != nil) != test.invalid {
				t.Fatalf("invalid=%t, error=%v", test.invalid, err)
			}
			if errors.Is(err, evidence.ErrReleaseHeadroomExhausted) != test.exhausted {
				t.Fatalf("headroom exhausted=%t, error=%v", test.exhausted, err)
			}
		})
	}
}

func TestReleaseAccumulatorPreservesCompleteSessionAndMissingObservations(t *testing.T) {
	accumulator := evidence.NewReleaseAccumulator()
	if empty := accumulator.Result(); empty.SampleCount != 0 || empty.SchemaVersion != 5 {
		t.Fatalf("empty summary=%+v", empty)
	}
	preferred, fallback := 12*policy.GiB, 10*policy.GiB
	pressure, compressor, payload, cpu, disk, swap, ins, outs := 1, true, int64(512), 10.0, int64(100), 2*policy.GiB, int64(1), int64(4)
	first := evidence.ReleaseSample{Sample: policy.Sample{Platform: "darwin", Capabilities: []string{"cpu"}, AvailableParallelism: 4, PhysicalMemoryBytes: 32 * policy.GiB, AvailableMemoryBytes: &preferred, AvailableNonCompressedEstimateBytes: &fallback, MemoryPressureLevel: &pressure, CompressorAvailable: &compressor, CompressorPayloadBytes: &payload, CPUUtilizationPercent: &cpu, DiskFreeBytes: &disk, SwapFreeBytes: &swap, SwapIns: &ins, SwapOuts: &outs}, ServiceRSSBytes: 100, HealthStatus: 200, HealthLatencyMs: 2.5, RoutedJourneyStatus: 200, RoutedJourneyLatencyMs: 50}
	accumulator.Add(first)
	first.Capabilities[0] = "changed"
	nextMemory, nextPressure, nextPayload, nextCPU, nextDisk, nextSwap, nextIns, nextOuts := 11*policy.GiB, 2, int64(1024), 90.0, int64(90), policy.GiB, int64(3), int64(2)
	second := evidence.ReleaseSample{Sample: policy.Sample{AvailableNonCompressedEstimateBytes: &nextMemory, MemoryPressureLevel: &nextPressure, CompressorPayloadBytes: &nextPayload, CPUUtilizationPercent: &nextCPU, DiskFreeBytes: &nextDisk, SwapFreeBytes: &nextSwap, SwapIns: &nextIns, SwapOuts: &nextOuts}, ServiceRSSBytes: 200, HealthStatus: 0, HealthLatencyMs: 5, RoutedJourneyStatus: 0, RoutedJourneyLatencyMs: 70}
	accumulator.Add(second)
	summary := accumulator.Result()
	if summary.SampleCount != 2 || summary.AvailableNonCompressedEstimateMinBytes != 11*policy.GiB || summary.MemoryPressureLevelMax != 2 || summary.CompressorAvailableAll || summary.CompressorPayloadPeakBytes != 1024 || summary.DiskFreeMinBytes != 90 || summary.SwapFreeMinBytes != policy.GiB || summary.ServiceRSSPeakBytes != 200 {
		t.Fatalf("resource aggregates=%+v", summary)
	}
	if summary.PhysicalMemoryBytes != 32*policy.GiB || summary.SwapInsDelta != 2 || summary.SwapOutsDelta != 0 || summary.CPUUtilizationP95Percent != 90 || summary.HealthLatencyP95Ms != 5 || summary.RoutedJourneyLatencyP95Ms != 70 || summary.RoutedJourneyLatencyMaxMs != 70 || summary.HealthFailures != 1 || summary.RoutedJourneyFailures != 1 {
		t.Fatalf("session aggregates=%+v", summary)
	}
	if len(summary.Capabilities) != 1 || summary.Capabilities[0] != "cpu" {
		t.Fatalf("first capabilities changed=%v", summary.Capabilities)
	}
}
