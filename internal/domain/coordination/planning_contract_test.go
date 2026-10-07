package coordination_test

import (
	"errors"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/policy"
)

func planningInputs() (policy.Sample, policy.Resolution, coordination.ReservationPolicy) {
	return policy.Sample{AvailableParallelism: 9, EffectiveMemoryLimitBytes: 16 * policy.GiB},
		policy.Resolution{ResolvedProfile: "balanced", Lineage: policy.LineageBalanced, MemoryReserve: 2 * policy.GiB},
		coordination.ReservationPolicy{OwnerShares: map[policy.ProfileName]int{"balanced": 2}, Tiers: coordination.DefaultResourceTiers()}
}

func TestTierPlanningPinsExplicitDimensionsAndClampsBurstCapacity(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "safe burst", true: "explicit fixed dimensions"}[explicit], func(t *testing.T) {
			sample, resolution, settings := planningInputs()
			settings.MaxCPU, settings.MaxMemoryBytes = 3, 5*policy.GiB
			cpu, memory := 0, int64(0)
			minimum := coordination.ReservationVector{CPU: 2, MemoryBytes: 3 * policy.GiB}
			maximum := coordination.ReservationVector{CPU: 3, MemoryBytes: 5 * policy.GiB}
			if explicit {
				cpu, memory = 3, 4*policy.GiB
				minimum = coordination.ReservationVector{CPU: cpu, MemoryBytes: memory}
				maximum = minimum
			}
			plan, deadline, err := coordination.PlanTierReservation(sample, resolution, settings, "standard", cpu, memory)
			want := coordination.ReservationPlan{
				Capacity:  coordination.ReservationVector{CPU: 3, MemoryBytes: 5 * policy.GiB},
				Requested: minimum, Allocated: minimum, Minimum: minimum, Maximum: maximum, Tier: "standard",
			}
			if err != nil || plan != want || deadline != 90*time.Minute {
				t.Fatalf("plan=%+v deadline=%s error=%v, want %+v and 90m", plan, deadline, err, want)
			}
		})
	}
}

func TestTierPlanningRejectsImpossibleRequestsBeforeQueueing(t *testing.T) {
	cases := []struct {
		name   string
		cpu    int
		memory int64
	}{
		{name: "negative CPU", cpu: -1},
		{name: "negative memory", memory: -1},
		{name: "CPU below tier", cpu: 1},
		{name: "CPU above tier", cpu: 5},
		{name: "memory below tier", memory: 2 * policy.GiB},
		{name: "memory above tier", memory: 7 * policy.GiB},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			sample, resolution, settings := planningInputs()
			plan, deadline, err := coordination.PlanTierReservation(sample, resolution, settings, "standard", test.cpu, test.memory)
			if !errors.Is(err, coordination.ErrReservationReplan) || plan != (coordination.ReservationPlan{}) || deadline != 0 {
				t.Fatalf("impossible request returned plan=%+v deadline=%s error=%v", plan, deadline, err)
			}
		})
	}
}

func TestTierPlanningRejectsUnsafeConfigurationAndHostFloors(t *testing.T) {
	cases := []struct {
		name   string
		change func(*policy.Sample, *policy.Resolution, *coordination.ReservationPolicy)
	}{
		{"unknown tier", func(_ *policy.Sample, _ *policy.Resolution, settings *coordination.ReservationPolicy) {
			settings.Tiers = nil
		}},
		{"no safe memory", func(_ *policy.Sample, resolution *policy.Resolution, _ *coordination.ReservationPolicy) {
			resolution.MemoryReserve = 16 * policy.GiB
		}},
		{"CPU cannot fit minimum", func(_ *policy.Sample, _ *policy.Resolution, settings *coordination.ReservationPolicy) {
			settings.MaxCPU = 1
		}},
		{"memory cannot fit minimum", func(_ *policy.Sample, _ *policy.Resolution, settings *coordination.ReservationPolicy) {
			settings.MaxMemoryBytes = 2 * policy.GiB
		}},
		{"unbounded queue", func(_ *policy.Sample, _ *policy.Resolution, settings *coordination.ReservationPolicy) {
			tier := settings.Tiers["standard"]
			tier.QueueDeadline = 0
			settings.Tiers["standard"] = tier
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			sample, resolution, settings := planningInputs()
			test.change(&sample, &resolution, &settings)
			plan, deadline, err := coordination.PlanTierReservation(sample, resolution, settings, "standard", 0, 0)
			if !errors.Is(err, coordination.ErrReservationReplan) || plan != (coordination.ReservationPlan{}) || deadline != 0 {
				t.Fatalf("unsafe tier returned plan=%+v deadline=%s error=%v", plan, deadline, err)
			}
		})
	}
}

func TestFairSharePlanningRejectsMemoryBelowImmutableFloor(t *testing.T) {
	sample, resolution, settings := planningInputs()
	settings.MaxMemoryBytes = coordination.MinimumReservationMemoryBytes - 1
	plan, err := coordination.PlanReservation(sample, resolution, settings, 0, 0)
	if !errors.Is(err, coordination.ErrReservationReplan) || plan != (coordination.ReservationPlan{}) {
		t.Fatalf("unsafe memory floor returned plan=%+v error=%v", plan, err)
	}
}
