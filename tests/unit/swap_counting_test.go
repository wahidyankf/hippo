package unit_test

import (
	"encoding/json"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/policy"
)

// swapHost is a Linux sample: memory bytes of RAM and limit, available bytes
// of memory, and swapTotal bytes of swap of which swapUsed bytes are in use.
func swapHost(memory, available, swapTotal, swapUsed int64) policy.Sample {
	sample := policySample(time.Unix(0, 0))
	sample.SchemaVersion = 3
	sample.Platform = "linux"
	sample.Capabilities = []string{"cgroup-v2", "memory-psi"}
	sample.PhysicalMemoryBytes = memory
	sample.EffectiveMemoryLimitBytes = memory
	sample.AvailableMemoryBytes = new(available)
	sample.AvailableNonCompressedEstimateBytes = nil
	sample.CompressorAvailable = nil
	sample.CompressorPayloadBytes = nil
	sample.SwapTotalBytes = new(swapTotal)
	sample.SwapUsedBytes = new(swapUsed)
	sample.SwapFreeBytes = new(swapTotal - swapUsed)
	sample.SwapState = "idle"
	if swapUsed > 0 {
		sample.SwapState = "active"
	}

	return sample
}

// swapOutGrowth is a 15-second window in which swap-outs grow by the 128 MiB
// the policy window normalizes.
func swapOutGrowth(sample policy.Sample) []policy.Sample {
	first, second := sample, sample
	first.MeasuredAt = time.Unix(0, 0).UTC().Format(time.RFC3339Nano)
	second.MeasuredAt = time.Unix(15, 0).UTC().Format(time.RFC3339Nano)
	first.SwapOuts = new(int64(0))
	second.SwapOuts = new(128 * policy.MiB / 16_384)

	return []policy.Sample{first, second}
}

// swapAssessmentPolicy is a policy whose thresholds read the 4 GiB hosts below.
func swapAssessmentPolicy() policy.Policy {
	assessment := policy.DefaultPolicy()
	assessment.AdmissionMemoryBytes = policy.GiB
	assessment.CriticalMemoryBytes = 512 * policy.MiB
	assessment.SwapOutWarningBytes = 64 * policy.MiB
	assessment.SwapOutCriticalBytes = 256 * policy.MiB
	assessment.DiskWarningBytes = 10 * policy.GiB
	assessment.DiskCriticalBytes = 256 * policy.MiB

	return assessment
}

func TestSwapAllowanceIsTheSmallerOfRoundedMemoryAndSwap(t *testing.T) {
	cases := []struct {
		name   string
		sample func() policy.Sample
		want   int64
	}{
		{"memory rounds up to whole GiB", func() policy.Sample { return swapHost(7618*policy.MiB, policy.GiB, 8*policy.GiB, 0) }, 8 * policy.GiB},
		{"swap smaller than memory", func() policy.Sample { return swapHost(4*policy.GiB, policy.GiB, 2*policy.GiB, 0) }, 2 * policy.GiB},
		{"exact memory and larger swap", func() policy.Sample { return swapHost(4*policy.GiB, policy.GiB, 8*policy.GiB, 0) }, 4 * policy.GiB},
		{"no swap", func() policy.Sample { return swapHost(4*policy.GiB, policy.GiB, 0, 0) }, 0},
		{"unreadable swap in use", func() policy.Sample {
			sample := swapHost(4*policy.GiB, policy.GiB, 8*policy.GiB, 0)
			sample.SwapUsedBytes = nil
			return sample
		}, 0},
		{"unavailable swap state", func() policy.Sample {
			sample := swapHost(4*policy.GiB, policy.GiB, 8*policy.GiB, 0)
			sample.SwapState = "unavailable"
			return sample
		}, 0},
		{"unknown swap state", func() policy.Sample {
			sample := swapHost(4*policy.GiB, policy.GiB, 8*policy.GiB, 0)
			sample.SwapState = "unknown"
			return sample
		}, 0},
		{"macOS never counts", func() policy.Sample {
			sample := swapHost(4*policy.GiB, policy.GiB, 8*policy.GiB, 0)
			sample.Platform = "darwin"
			return sample
		}, 0},
		{"physical memory stands in for an unset limit", func() policy.Sample {
			sample := swapHost(4*policy.GiB, policy.GiB, 8*policy.GiB, 0)
			sample.EffectiveMemoryLimitBytes = 0
			sample.PhysicalMemoryBytes = 2 * policy.GiB
			return sample
		}, 2 * policy.GiB},
		{"memory beyond rounding range is capped by swap", func() policy.Sample {
			sample := swapHost(4*policy.GiB, policy.GiB, 8*policy.GiB, 0)
			sample.EffectiveMemoryLimitBytes = math.MaxInt64
			return sample
		}, 8 * policy.GiB},
		{"no memory evidence at all", func() policy.Sample {
			sample := swapHost(4*policy.GiB, policy.GiB, 8*policy.GiB, 0)
			sample.EffectiveMemoryLimitBytes = 0
			sample.PhysicalMemoryBytes = 0
			return sample
		}, 0},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := policy.SwapAllowanceFor(testCase.sample()); got != testCase.want {
				t.Fatalf("allowance = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestSwapCountingIsAbsentUnlessTurnedOn(t *testing.T) {
	sample := swapHost(4*policy.GiB, policy.GiB, 8*policy.GiB, policy.GiB)
	if off := policy.WithSwapAllowance(sample, false); off.SwapAllowanceBytes != nil {
		t.Fatalf("counting off carried allowance %d", *off.SwapAllowanceBytes)
	}
	on := policy.WithSwapAllowance(sample, true)
	if on.SwapAllowanceBytes == nil || *on.SwapAllowanceBytes != 4*policy.GiB {
		t.Fatalf("counting on carried %v, want 4 GiB", on.SwapAllowanceBytes)
	}
	if sample.SwapAllowanceBytes != nil {
		t.Fatal("WithSwapAllowance changed its input instead of returning a copy")
	}
	zero := policy.WithSwapAllowance(swapHost(4*policy.GiB, policy.GiB, 0, 0), true)
	if zero.SwapAllowanceBytes == nil || *zero.SwapAllowanceBytes != 0 {
		t.Fatalf("counting on without swap carried %v, want an explicit zero", zero.SwapAllowanceBytes)
	}
}

func TestSwapHeadroomIsTheAllowanceLessSwapInUse(t *testing.T) {
	counted := func(used int64) policy.Sample {
		return policy.WithSwapAllowance(swapHost(4*policy.GiB, policy.GiB, 8*policy.GiB, used), true)
	}
	if headroom := policy.SwapHeadroomBytes(counted(policy.GiB)); headroom == nil || *headroom != 3*policy.GiB {
		t.Fatalf("headroom = %v, want 3 GiB", headroom)
	}
	if headroom := policy.SwapHeadroomBytes(counted(4 * policy.GiB)); headroom == nil || *headroom != 0 {
		t.Fatalf("headroom at the allowance = %v, want zero", headroom)
	}
	if headroom := policy.SwapHeadroomBytes(counted(6 * policy.GiB)); headroom == nil || *headroom != 0 {
		t.Fatalf("headroom beyond the allowance = %v, want zero, never negative", headroom)
	}
	unreadable := counted(policy.GiB)
	unreadable.SwapUsedBytes = nil
	if headroom := policy.SwapHeadroomBytes(unreadable); headroom == nil || *headroom != 0 {
		t.Fatalf("headroom with unreadable swap = %v, want zero", headroom)
	}
	if headroom := policy.SwapHeadroomBytes(swapHost(4*policy.GiB, policy.GiB, 8*policy.GiB, 0)); headroom != nil {
		t.Fatalf("counting off reported headroom %d", *headroom)
	}
}

func TestCountedAvailableMemoryAddsHeadroomAndSaturates(t *testing.T) {
	counted := policy.WithSwapAllowance(swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, policy.GiB), true)
	if got := policy.CountedAvailableMemory(counted); got == nil || *got != 6*policy.GiB {
		t.Fatalf("counted available = %v, want 6 GiB", got)
	}
	nearMaximum := policy.WithSwapAllowance(swapHost(4*policy.GiB, math.MaxInt64-1, 8*policy.GiB, 0), true)
	if got := policy.CountedAvailableMemory(nearMaximum); got == nil || *got != math.MaxInt64 {
		t.Fatalf("counted available = %v, want saturation at the maximum", got)
	}
	off := swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, policy.GiB)
	if got := policy.CountedAvailableMemory(off); got == nil || *got != 3*policy.GiB {
		t.Fatalf("counting off changed available memory to %v", got)
	}
	noAvailable := policy.Sample{}
	if got := policy.CountedAvailableMemory(noAvailable); got != nil {
		t.Fatalf("no evidence became %d", *got)
	}
}

// TestSwapCountingAssessmentFollowsTheAllowance walks the pressure matrix: each
// row fixes the swap in use, the pressure readings, and whether counting is on.
func TestSwapCountingAssessmentFollowsTheAllowance(t *testing.T) {
	psi := func(some, full float64) func(policy.Sample) policy.Sample {
		return func(sample policy.Sample) policy.Sample {
			sample.MemoryPSISomeAvg10 = new(some)
			sample.MemoryPSIFullAvg10 = new(full)
			return sample
		}
	}
	cases := []struct {
		name       string
		sample     func() []policy.Sample
		counting   bool
		state      policy.State
		reason     string
		advisories []string
	}{
		{
			name: "within the allowance the counted memory keeps the host normal",
			sample: func() []policy.Sample {
				return []policy.Sample{swapHost(4*policy.GiB, 256*policy.MiB, 8*policy.GiB, policy.GiB)}
			},
			counting: true, state: policy.StateNormal, reason: "normal",
		},
		{
			name: "counting off, the raw memory is critical",
			sample: func() []policy.Sample {
				return []policy.Sample{swapHost(4*policy.GiB, 256*policy.MiB, 8*policy.GiB, policy.GiB)}
			},
			counting: false, state: policy.StateCritical, reason: "memory-critical",
		},
		{
			name: "used equal to the allowance is still within it, with no headroom",
			sample: func() []policy.Sample {
				return []policy.Sample{swapHost(4*policy.GiB, 256*policy.MiB, 8*policy.GiB, 4*policy.GiB)}
			},
			counting: true, state: policy.StateCritical, reason: "memory-critical",
		},
		{
			name: "beyond the allowance the raw memory decides and an advisory is reported",
			sample: func() []policy.Sample {
				return []policy.Sample{swapHost(4*policy.GiB, 256*policy.MiB, 8*policy.GiB, 5*policy.GiB)}
			},
			counting: true, state: policy.StateCritical, reason: "memory-critical",
			advisories: []string{"swap-over-allowance"},
		},
		{
			name: "beyond the allowance with ample memory is normal and only advised",
			sample: func() []policy.Sample {
				return []policy.Sample{swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, 5*policy.GiB)}
			},
			counting: true, state: policy.StateNormal, reason: "normal",
			advisories: []string{"swap-over-allowance"},
		},
		{
			name: "PSI warning band is ignored within the allowance",
			sample: func() []policy.Sample {
				return []policy.Sample{psi(10, 0)(swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, policy.GiB))}
			},
			counting: true, state: policy.StateNormal, reason: "normal",
		},
		{
			name: "PSI warning band applies beyond the allowance",
			sample: func() []policy.Sample {
				return []policy.Sample{psi(10, 0)(swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, 5*policy.GiB))}
			},
			counting: true, state: policy.StateWarning, reason: "memory-psi",
			advisories: []string{"swap-over-allowance"},
		},
		{
			name: "PSI warning band applies when counting is off",
			sample: func() []policy.Sample {
				return []policy.Sample{psi(10, 0)(swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, policy.GiB))}
			},
			counting: false, state: policy.StateWarning, reason: "memory-psi",
		},
		{
			name: "PSI critical holds within the allowance",
			sample: func() []policy.Sample {
				return []policy.Sample{psi(25, 0)(swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, policy.GiB))}
			},
			counting: true, state: policy.StateCritical, reason: "memory-psi",
		},
		{
			name: "PSI full critical holds within the allowance",
			sample: func() []policy.Sample {
				return []policy.Sample{psi(0, 5)(swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, policy.GiB))}
			},
			counting: true, state: policy.StateCritical, reason: "memory-psi",
		},
		{
			name: "readable PSI leaves swap-out growth unassessed while counting",
			sample: func() []policy.Sample {
				return swapOutGrowth(psi(0, 0)(swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, policy.GiB)))
			},
			counting: true, state: policy.StateNormal, reason: "normal",
		},
		{
			name: "readable PSI still assesses swap-out growth when counting is off",
			sample: func() []policy.Sample {
				return swapOutGrowth(psi(0, 0)(swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, policy.GiB)))
			},
			counting: false, state: policy.StateWarning, reason: "swap-warning",
		},
		{
			name: "missing PSI keeps the swap-out rate rules while counting",
			sample: func() []policy.Sample {
				return swapOutGrowth(swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, policy.GiB))
			},
			counting: true, state: policy.StateWarning, reason: "swap-warning",
		},
		{
			name:     "no swap means counting changes nothing",
			sample:   func() []policy.Sample { return []policy.Sample{swapHost(4*policy.GiB, 256*policy.MiB, 0, 0)} },
			counting: true, state: policy.StateCritical, reason: "memory-critical",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			samples := testCase.sample()
			for index := range samples {
				samples[index] = policy.WithSwapAllowance(samples[index], testCase.counting)
			}
			assessment := policy.ResourceAssessment(samples, swapAssessmentPolicy())
			if assessment.State != testCase.state || assessment.Reason != testCase.reason {
				t.Fatalf("state %q reason %q, want %q %q", assessment.State, assessment.Reason, testCase.state, testCase.reason)
			}
			if !slices.Equal(assessment.Advisories, testCase.advisories) {
				t.Fatalf("advisories %v, want %v", assessment.Advisories, testCase.advisories)
			}
		})
	}
}

// TestSwapCountingFieldsAreAdditive proves a counting-off assessment and sample
// serialize exactly as before, and a counting-on one adds the swap fields.
func TestSwapCountingFieldsAreAdditive(t *testing.T) {
	off := swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, policy.GiB)
	offAssessment := policy.ResourceAssessment([]policy.Sample{off}, swapAssessmentPolicy())
	offJSON := marshalFields(t, offAssessment)
	for _, key := range []string{"swapHeadroomBytes", "countedAvailableMemoryBytes", "advisories"} {
		if _, present := offJSON[key]; present {
			t.Fatalf("counting off serialized %q", key)
		}
	}
	if _, present := marshalFields(t, off)["swapAllowanceBytes"]; present {
		t.Fatal("counting off serialized swapAllowanceBytes on the sample")
	}

	on := policy.WithSwapAllowance(off, true)
	onAssessment := policy.ResourceAssessment([]policy.Sample{on}, swapAssessmentPolicy())
	onJSON := marshalFields(t, onAssessment)
	if got := onJSON["swapHeadroomBytes"]; got != float64(3*policy.GiB) {
		t.Fatalf("swapHeadroomBytes = %v, want 3 GiB", got)
	}
	if got := onJSON["countedAvailableMemoryBytes"]; got != float64(6*policy.GiB) {
		t.Fatalf("countedAvailableMemoryBytes = %v, want 6 GiB", got)
	}
	if got := marshalFields(t, on)["swapAllowanceBytes"]; got != float64(4*policy.GiB) {
		t.Fatalf("swapAllowanceBytes = %v, want 4 GiB", got)
	}
}

func marshalFields(t *testing.T, value any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]any{}
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}

	return fields
}
