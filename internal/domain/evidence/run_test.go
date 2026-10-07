package evidence_test

import (
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
)

func TestRunAggregatePreservesLifetimePeaksAndAllocation(t *testing.T) {
	accumulator := evidence.NewRunAccumulator("run")
	accumulator.SetIdentity(coordination.ReservationMetadata{Source: "hippo", Tags: map[string]string{"kind": "test"}, Tier: "light"})
	accumulator.SetContext(policy.Resolution{RequestedProfile: "balanced", ResolvedProfile: "minimal", FallbackChain: []policy.ProfileName{"balanced", "minimal"}, Concurrency: 2}, "hash")
	accumulator.SetReservationContext(&coordination.LeaseMetadata{Requested: coordination.ReservationVector{CPU: 1, MemoryBytes: policy.GiB}, Allocation: coordination.ReservationVector{CPU: 2, MemoryBytes: 2 * policy.GiB}, WaitDuration: 15 * time.Millisecond}, 2, evidence.BudgetOutcomeAdmitted)
	accumulator.ObserveReservationOwners(3)
	accumulator.ObserveReservationOwners(1)
	low, high := int64(10), int64(30)
	mild, severe := 1, 3
	yes, no := true, false
	cpuLow, cpuHigh := 10.0, 30.0
	first := policy.Sample{MeasuredAt: "first", Platform: "linux", Capabilities: []string{"cpu"}, AvailableParallelism: 4, AvailableMemoryBytes: &high, MemoryPressureLevel: &mild, CompressorAvailable: &yes, CompressorPayloadBytes: &low, CPUUtilizationPercent: &cpuLow, DiskFreeBytes: &high, SwapFreeBytes: &high, SwapIns: &low, SwapOuts: &low}
	second := policy.Sample{MeasuredAt: "second", AvailableNonCompressedEstimateBytes: &low, MemoryPressureLevel: &severe, CompressorAvailable: &no, CompressorPayloadBytes: &high, CPUUtilizationPercent: &cpuHigh, DiskFreeBytes: &low, SwapFreeBytes: &low, SwapIns: &high, SwapOuts: &high}
	accumulator.Append(first)
	accumulator.Append(second)
	accumulator.Append(first)
	accumulator.Append(second)
	summary := accumulator.Complete(policy.TaskEphemeral, evidence.OutcomePassed, 2)
	assertLifetimeIdentity(t, accumulator, summary)
	assertLifetimeMetrics(t, summary, low, high, severe, cpuHigh)
	assertLifetimeAllocation(t, summary)
}

func assertLifetimeIdentity(t *testing.T, accumulator *evidence.RunAccumulator, summary evidence.RunSummary) {
	t.Helper()
	if accumulator.RunID() != "run" || summary.SchemaVersion != 5 || summary.SampleCount != 4 || summary.Source != "hippo" || summary.ResourceTier != "light" || summary.Tags["kind"] != "test" || summary.TaskClass != policy.TaskEphemeral || summary.Outcome != evidence.OutcomePassed || summary.StartedAt != "first" || summary.FinishedAt != "second" || summary.Platform != "linux" || summary.AvailableParallelism != 4 || len(summary.Capabilities) != 1 {
		t.Fatalf("identity/lifetime metadata changed: %+v", summary)
	}
}

func assertLifetimeMetrics(t *testing.T, summary evidence.RunSummary, low, high int64, severe int, cpuHigh float64) {
	t.Helper()
	if *summary.AvailableNonCompressedEstimateMinBytes != low || *summary.MemoryPressureLevelMax != severe || *summary.CompressorPayloadPeakBytes != high || *summary.DiskFreeMinBytes != low || *summary.SwapFreeMinBytes != low || summary.CPUUtilizationP95Percent != cpuHigh || summary.CompressorAvailableAll || summary.SwapInsDelta != 20 || summary.SwapOutsDelta != 20 {
		t.Fatalf("lifetime aggregates changed: %+v", summary)
	}
}

func assertLifetimeAllocation(t *testing.T, summary evidence.RunSummary) {
	t.Helper()
	if summary.RequestedCPU != 1 || summary.AllocatedCPU != 2 || summary.RequestedMemoryBytes != policy.GiB || summary.AllocatedMemoryBytes != 2*policy.GiB || summary.ReservationWaitMilliseconds != 15 || summary.PeakOwnerCount != 3 || summary.BudgetOutcome != evidence.BudgetOutcomeAdmitted || summary.ConfigHash != "hash" || summary.Concurrency != 2 || summary.ResolvedProfile != "minimal" || summary.HealthFailures != 2 {
		t.Fatalf("allocation/context changed: %+v", summary)
	}
}

func TestRunAggregateUnavailableAndResetCountersStayUnknownOrZero(t *testing.T) {
	var absent *evidence.RunAccumulator
	absent.SetIdentity(coordination.ReservationMetadata{})
	absent.SetReservationContext(nil, 1, evidence.BudgetOutcomeAdmitted)
	absent.ObserveReservationOwners(1)
	for _, name := range []string{"empty", "unavailable", "reset"} {
		t.Run(name, func(t *testing.T) {
			accumulator := evidence.NewRunAccumulator(name)
			accumulator.SetReservationContext(nil, 2, evidence.BudgetOutcomeAdmitted)
			if name == "unavailable" {
				accumulator.Append(policy.Sample{})
				accumulator.Append(policy.Sample{})
			}
			if name == "reset" {
				first, last := int64(30), int64(10)
				accumulator.Append(policy.Sample{SwapIns: &first, SwapOuts: &first})
				accumulator.Append(policy.Sample{SwapIns: &last, SwapOuts: &last})
			}
			summary := accumulator.Complete(policy.TaskService, evidence.OutcomeTaskFailed, 0)
			if summary.SwapInsDelta != 0 || summary.SwapOutsDelta != 0 || summary.CPUUtilizationP95Percent != 0 || summary.AvailableNonCompressedEstimateMinBytes != nil || summary.MemoryPressureLevelMax != nil || summary.CompressorPayloadPeakBytes != nil || summary.DiskFreeMinBytes != nil || summary.SwapFreeMinBytes != nil {
				t.Fatalf("unavailable/reset evidence became a metric: %+v", summary)
			}
			if name != "empty" && summary.CompressorAvailableAll {
				t.Fatal("unknown compressor was reported available")
			}
		})
	}
}
