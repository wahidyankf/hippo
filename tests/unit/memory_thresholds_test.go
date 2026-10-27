package unit_test

import (
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/policy"
)

// thresholdHost is a Linux sample: limit bytes of effective memory limit and
// RAM, and available bytes of memory, with no pressure readings and no swap
// evidence unless the caller adds them. A caller that needs the two to differ
// sets PhysicalMemoryBytes after.
func thresholdHost(limit, available int64) policy.Sample {
	sample := policySample(time.Unix(0, 0))
	sample.Platform = "linux"
	sample.Capabilities = []string{"cgroup-v2", "memory-psi"}
	sample.PhysicalMemoryBytes = limit
	sample.EffectiveMemoryLimitBytes = limit
	sample.AvailableMemoryBytes = new(available)
	sample.AvailableNonCompressedEstimateBytes = nil
	sample.CompressorAvailable = nil
	sample.CompressorPayloadBytes = nil
	sample.SwapState = "idle"

	return sample
}

// thresholdPolicy is a policy with explicit legacy bounds: a 2 GiB warning, a
// 3 GiB degraded warning, and a 1 GiB critical floor.
func thresholdPolicy() policy.Policy {
	assessment := policy.DefaultPolicy()
	assessment.AdmissionMemoryBytes = 2 * policy.GiB
	assessment.WarningAdmissionMemoryBytes = 3 * policy.GiB
	assessment.CriticalMemoryBytes = policy.GiB

	return assessment
}

// TestMemoryThresholdsReplaceLegacyBounds walks the percent matrix: below the
// percent is warning, below the critical percent is critical, floors lift the
// percent, and an absent percent keeps the legacy bound.
func TestMemoryThresholdsReplaceLegacyBounds(t *testing.T) {
	withWarning := func(percent, floorMiB float64) policy.Policy {
		assessment := thresholdPolicy()
		assessment.WarningMemoryPercent = percent
		assessment.WarningMemoryFloorBytes = int64(floorMiB * float64(policy.MiB))

		return assessment
	}
	withCritical := func(percent float64) policy.Policy {
		assessment := thresholdPolicy()
		assessment.CriticalMemoryPercent = percent

		return assessment
	}
	cases := []struct {
		name   string
		policy func() policy.Policy
		sample func() policy.Sample
		state  policy.State
		reason string
	}{
		{
			name:   "legacy without a percent stays normal above the fixed warning",
			policy: thresholdPolicy,
			sample: func() policy.Sample { return thresholdHost(16*policy.GiB, 3*policy.GiB) },
			state:  policy.StateNormal, reason: "normal",
		},
		{
			name:   "warning percent fires below the percent of the limit",
			policy: func() policy.Policy { return withWarning(25, 0) },
			sample: func() policy.Sample { return thresholdHost(16*policy.GiB, 3*policy.GiB) },
			state:  policy.StateWarning, reason: "memory-warning",
		},
		{
			name:   "warning percent stays normal at the percent of the limit",
			policy: func() policy.Policy { return withWarning(25, 0) },
			sample: func() policy.Sample { return thresholdHost(16*policy.GiB, 4*policy.GiB) },
			state:  policy.StateNormal, reason: "normal",
		},
		{
			name:   "warning floor fires below the floor",
			policy: func() policy.Policy { return withWarning(25, 5120) },
			sample: func() policy.Sample { return thresholdHost(16*policy.GiB, 4*policy.GiB) },
			state:  policy.StateWarning, reason: "memory-warning",
		},
		{
			name:   "warning floor stays normal above the floor",
			policy: func() policy.Policy { return withWarning(25, 5120) },
			sample: func() policy.Sample { return thresholdHost(16*policy.GiB, 6*policy.GiB) },
			state:  policy.StateNormal, reason: "normal",
		},
		{
			name:   "floor without a percent keeps the legacy bound",
			policy: func() policy.Policy { return withWarning(0, 5120) },
			sample: func() policy.Sample { return thresholdHost(16*policy.GiB, 3*policy.GiB) },
			state:  policy.StateNormal, reason: "normal",
		},
		{
			name:   "physical memory stands in for an unset limit",
			policy: func() policy.Policy { return withWarning(25, 0) },
			sample: func() policy.Sample {
				sample := thresholdHost(0, 3*policy.GiB)
				sample.PhysicalMemoryBytes = 16 * policy.GiB

				return sample
			},
			state: policy.StateWarning, reason: "memory-warning",
		},
		{
			name:   "critical percent replaces the fixed critical floor",
			policy: func() policy.Policy { return withCritical(5) },
			sample: func() policy.Sample { return thresholdHost(16*policy.GiB, policy.GiB) },
			state:  policy.StateWarning, reason: "memory-warning",
		},
		{
			name:   "critical percent fires below the percent of the limit",
			policy: func() policy.Policy { return withCritical(5) },
			sample: func() policy.Sample { return thresholdHost(16*policy.GiB, policy.GiB/2) },
			state:  policy.StateCritical, reason: "memory-critical",
		},
		{
			name:   "critical percent rounds up to whole bytes",
			policy: func() policy.Policy { return withCritical(5) },
			sample: func() policy.Sample { return thresholdHost(16*policy.GiB, 858993459) },
			state:  policy.StateCritical, reason: "memory-critical",
		},
		{
			name:   "critical percent stays normal at its ceiling",
			policy: func() policy.Policy { return withCritical(5) },
			sample: func() policy.Sample { return thresholdHost(16*policy.GiB, 858993460) },
			state:  policy.StateWarning, reason: "memory-warning",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := policy.MemoryState(testCase.sample(), testCase.policy()); got != testCase.state {
				t.Fatalf("state = %q, want %q", got, testCase.state)
			}
			assessment := policy.ResourceAssessment([]policy.Sample{testCase.sample()}, testCase.policy())
			if assessment.State != testCase.state || assessment.Reason != testCase.reason {
				t.Fatalf("state %q reason %q, want %q %q", assessment.State, assessment.Reason, testCase.state, testCase.reason)
			}
		})
	}
}

// TestWarningAdmissionReadsTheProportionalWarning proves the degraded path
// answers the same percent its normal boundary does: the stable window passes
// with the legacy floor, fails under the percent, and passes again once the
// floor drops below the percent.
func TestWarningAdmissionReadsTheProportionalWarning(t *testing.T) {
	baseline := warningAdmissionSamples()
	if !policy.WarningAdmissionReady(baseline, policy.DefaultPolicy()) {
		t.Fatal("stable Darwin warning was refused without knobs")
	}

	percent := policy.DefaultPolicy()
	percent.WarningMemoryPercent = 50
	if policy.WarningAdmissionReady(baseline, percent) {
		t.Fatal("proportional warning admitted headroom below the percent")
	}

	floorBinds := policy.DefaultPolicy()
	floorBinds.WarningMemoryPercent = 25
	floorBinds.WarningMemoryFloorBytes = 20 * policy.GiB
	if policy.WarningAdmissionReady(baseline, floorBinds) {
		t.Fatal("proportional warning admitted headroom below the floor")
	}

	floorRests := policy.DefaultPolicy()
	floorRests.WarningMemoryPercent = 25
	floorRests.WarningMemoryFloorBytes = 4 * policy.GiB
	if !policy.WarningAdmissionReady(baseline, floorRests) {
		t.Fatal("proportional warning refused headroom at the percent")
	}
}

// TestApplyCoordinationThresholdsIsIdentityWhenAbsent proves a coordination
// without knobs resolves the policy it always did, and a coordination with
// knobs carries them onto the resolved policy.
func TestApplyCoordinationThresholdsIsIdentityWhenAbsent(t *testing.T) {
	base := thresholdPolicy()
	if got := policy.ApplyCoordinationThresholds(base, policy.MemoryPolicy{}, 0); got != base {
		t.Fatal("absent knobs changed the resolved policy")
	}

	applied := policy.ApplyCoordinationThresholds(base, policy.MemoryPolicy{
		WarningPercent:     25,
		CriticalPercent:    5,
		WarningFloorBytes:  5 * policy.GiB,
		CriticalFloorBytes: policy.GiB,
	}, 2)
	if applied.WarningMemoryPercent != 25 || applied.CriticalMemoryPercent != 5 ||
		applied.WarningMemoryFloorBytes != 5*policy.GiB || applied.CriticalMemoryFloorBytes != policy.GiB ||
		applied.SwapUsedMaxMultiple != 2 {
		t.Fatalf("knobs were not carried: %+v", applied)
	}
	if applied.AdmissionMemoryBytes != base.AdmissionMemoryBytes ||
		applied.CriticalMemoryBytes != base.CriticalMemoryBytes {
		t.Fatal("knobs rewrote the legacy bounds they override per sample")
	}
}

// swapGatePolicy is the swap matrix policy with the gate multiple set.
func swapGatePolicy(multiple float64) policy.Policy {
	assessment := swapAssessmentPolicy()
	assessment.SwapUsedMaxMultiple = multiple

	return assessment
}

// swapGateCase is one row of the swap-gate matrix: the samples, the gate
// multiple, and whether counting is on, with the expected outcome.
type swapGateCase struct {
	name       string
	sample     func() []policy.Sample
	multiple   float64
	counting   bool
	state      policy.State
	reason     string
	advisories []string
}

// swapGateCases is the pressure matrix the swap-gate test walks.
func swapGateCases() []swapGateCase {
	pressure := func(level int) func(policy.Sample) policy.Sample {
		return func(sample policy.Sample) policy.Sample {
			sample.MemoryPressureLevel = new(level)
			return sample
		}
	}
	compressorFailed := func(sample policy.Sample) policy.Sample {
		sample.Platform = "darwin"
		sample.Capabilities = []string{"compressor"}
		sample.CompressorAvailable = new(false)
		return sample
	}
	psi := func(some, full float64) func(policy.Sample) policy.Sample {
		return func(sample policy.Sample) policy.Sample {
			sample.MemoryPSISomeAvg10 = new(some)
			sample.MemoryPSIFullAvg10 = new(full)
			return sample
		}
	}
	return []swapGateCase{
		{
			name: "swap-out pressure within the multiple is waived",
			sample: func() []policy.Sample {
				return swapOutGrowth(swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, policy.GiB))
			},
			multiple: 2, state: policy.StateNormal, reason: "normal",
		},
		{
			name: "swap-out pressure at the multiple is waived",
			sample: func() []policy.Sample {
				return swapOutGrowth(swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, 4*policy.GiB))
			},
			multiple: 1, state: policy.StateNormal, reason: "normal",
		},
		{
			name: "swap-out pressure above the multiple still refuses",
			sample: func() []policy.Sample {
				return swapOutGrowth(swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, 5*policy.GiB))
			},
			multiple: 1, state: policy.StateWarning, reason: "swap-warning",
		},
		{
			name:     "unreadable swap never waives the rate rules",
			sample:   func() []policy.Sample { return swapOutGrowth(swapUnreadableHost()) },
			multiple: 2, state: policy.StateWarning, reason: "swap-warning",
		},
		{
			name: "critical PSI stays critical within the multiple",
			sample: func() []policy.Sample {
				return []policy.Sample{psi(25, 0)(swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, policy.GiB))}
			},
			multiple: 2, state: policy.StateCritical, reason: "memory-psi",
		},
		{
			name: "warning PSI stays warning within the multiple",
			sample: func() []policy.Sample {
				return []policy.Sample{psi(10, 0)(swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, policy.GiB))}
			},
			multiple: 2, state: policy.StateWarning, reason: "memory-psi",
		},
		{
			name: "critical pressure level stays critical within the multiple",
			sample: func() []policy.Sample {
				return swapOutGrowth(pressure(4)(swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, policy.GiB)))
			},
			multiple: 2, state: policy.StateCritical, reason: "memory-critical",
		},
		{
			name: "compressor failure stays critical within the multiple",
			sample: func() []policy.Sample {
				return swapOutGrowth(compressorFailed(swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, policy.GiB)))
			},
			multiple: 2, state: policy.StateCritical, reason: "memory-critical",
		},
		{
			name: "over-allowance keeps its advisory while the rate is waived",
			sample: func() []policy.Sample {
				return swapOutGrowth(swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, 5*policy.GiB))
			},
			multiple: 2, counting: true, state: policy.StateNormal, reason: "normal",
			advisories: []string{"swap-over-allowance"},
		},
		{
			name:     "zero physical memory never waives the rate rules",
			sample:   func() []policy.Sample { return swapOutGrowth(swapPhantomHost()) },
			multiple: 2, state: policy.StateWarning, reason: "swap-warning",
		},
		{
			name: "a nonpositive multiple keeps the legacy rules",
			sample: func() []policy.Sample {
				return swapOutGrowth(swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, policy.GiB))
			},
			multiple: -1, state: policy.StateWarning, reason: "swap-warning",
		},
	}
}

// TestSwapGateWaivesOnlyTheByteBasedSwapReasons walks the swap-gate matrix:
// the gate excuses swap-out pressure within its multiple, holds above it, and
// never touches PSI, pressure levels, compressor failure, or advisories.
func TestSwapGateWaivesOnlyTheByteBasedSwapReasons(t *testing.T) {
	for _, testCase := range swapGateCases() {
		t.Run(testCase.name, func(t *testing.T) {
			samples := testCase.sample()
			for index := range samples {
				samples[index] = policy.WithSwapAllowance(samples[index], testCase.counting)
			}
			assessment := policy.ResourceAssessment(samples, swapGatePolicy(testCase.multiple))
			if assessment.State != testCase.state || assessment.Reason != testCase.reason {
				t.Fatalf("state %q reason %q, want %q %q", assessment.State, assessment.Reason, testCase.state, testCase.reason)
			}
			if len(testCase.advisories) > 0 && len(assessment.Advisories) == 0 {
				t.Fatalf("advisories %v, want %v", assessment.Advisories, testCase.advisories)
			}
			legacy := policy.ResourceAssessment(samples, swapAssessmentPolicy())
			if testCase.multiple > 0 && testCase.name == "swap-out pressure within the multiple is waived" &&
				(legacy.State != policy.StateWarning || legacy.Reason != "swap-warning") {
				t.Fatalf("legacy state %q reason %q, want the waiver to change something", legacy.State, legacy.Reason)
			}
		})
	}
}

// swapUnreadableHost is a counted host whose swap in use cannot be read.
func swapUnreadableHost() policy.Sample {
	sample := swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, policy.GiB)
	sample.SwapUsedBytes = nil

	return sample
}

// swapPhantomHost is a host that reports swap but no physical memory.
func swapPhantomHost() policy.Sample {
	sample := swapHost(4*policy.GiB, 3*policy.GiB, 8*policy.GiB, policy.GiB)
	sample.PhysicalMemoryBytes = 0
	sample.EffectiveMemoryLimitBytes = 0

	return sample
}
