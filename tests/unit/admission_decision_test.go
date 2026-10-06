package unit_test

import (
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/policy"
)

// admissionHealthySamples is n consecutive samples of a healthy host, one second apart.
func admissionHealthySamples(count int) []policy.Sample {
	samples := make([]policy.Sample, count)
	for index := range samples {
		samples[index] = policySample(time.Unix(int64(index), 0))
	}

	return samples
}

// admissionWithDisk is a healthy window whose newest sample has the free disk given.
func admissionWithDisk(free int64) []policy.Sample {
	samples := admissionHealthySamples(3)
	samples[len(samples)-1].DiskFreeBytes = &free

	return samples
}

// admissionWarningPressure is a healthy window whose newest sample reports the
// memory pressure warning level, which is a warning on any platform.
func admissionWarningPressure() []policy.Sample {
	samples := admissionHealthySamples(3)
	samples[len(samples)-1].MemoryPressureLevel = new(2)

	return samples
}

// admissionCriticalPressure is a healthy window whose newest sample reports the
// critical memory pressure level.
func admissionCriticalPressure() []policy.Sample {
	samples := admissionHealthySamples(3)
	samples[len(samples)-1].MemoryPressureLevel = new(4)

	return samples
}

// admissionBusyCPU is a normal window whose newest sample leaves no CPU headroom.
func admissionBusyCPU() []policy.Sample {
	samples := admissionHealthySamples(3)
	samples[len(samples)-1].CPUUtilizationPercent = new(100.0)

	return samples
}

// admissionResolution is what resolving a profile of the lineage yields when the
// host fits it: decision run, no reason.
func admissionResolution(lineage policy.Lineage) policy.Resolution {
	return policy.Resolution{Decision: policy.DecisionRun, Lineage: lineage, Policy: policy.DefaultPolicy()}
}

// admissionInput builds a complete input, since the type's literal must name every field.
func admissionInput(
	resolution policy.Resolution,
	class policy.TaskClass,
	samples []policy.Sample,
	resourcePolicy policy.Policy,
	window policy.EvidenceWindow,
) policy.AdmissionInput {
	return policy.AdmissionInput{
		Resolution: resolution, TaskClass: class, Samples: samples, Policy: resourcePolicy, Window: window,
	}
}

func TestAnUnsetAdmissionPathAndWindowAreTheZeroValues(t *testing.T) {
	var path policy.AdmissionPath
	var window policy.EvidenceWindow
	if path != policy.AdmissionUnset || window != policy.WindowUnset {
		t.Errorf("zero values are path %d and window %d, want AdmissionUnset and WindowUnset", path, window)
	}
}

func TestEveryAdmissionPathIsDecidedUnderBothWindows(t *testing.T) {
	stableWarning := warningAdmissionSamples()
	rows := []struct {
		name       string
		resolution policy.Resolution
		class      policy.TaskClass
		samples    []policy.Sample
		window     policy.EvidenceWindow
		want       policy.AdmissionPath
	}{
		// Status's two samples: a normal state runs, anything else waits, a blocked disk cleans up.
		{
			"snapshot normal", admissionResolution(policy.LineageBalanced), policy.TaskEphemeral,
			admissionHealthySamples(2), policy.WindowSnapshot, policy.AdmissionNormal,
		},
		{
			"snapshot of one normal sample", admissionResolution(policy.LineageBalanced), policy.TaskEphemeral,
			admissionHealthySamples(1), policy.WindowSnapshot, policy.AdmissionNormal,
		},
		{
			"snapshot warning", admissionResolution(policy.LineageBalanced), policy.TaskEphemeral,
			admissionWarningPressure(), policy.WindowSnapshot, policy.AdmissionWait,
		},
		{
			"snapshot critical", admissionResolution(policy.LineageBalanced), policy.TaskEphemeral,
			admissionCriticalPressure(), policy.WindowSnapshot, policy.AdmissionWait,
		},
		{
			"snapshot of no samples", admissionResolution(policy.LineageBalanced), policy.TaskEphemeral,
			nil, policy.WindowSnapshot, policy.AdmissionWait,
		},
		{
			"snapshot storage blocked", admissionResolution(policy.LineageBalanced), policy.TaskEphemeral,
			admissionWithDisk(29 * policy.GiB), policy.WindowSnapshot, policy.AdmissionCleanup,
		},
		{
			// Two samples cannot fill the warning window, and status keeps v0.8.4's wait for any warning.
			"snapshot of a stable warning waits", admissionResolution(policy.LineageBalanced), policy.TaskEphemeral,
			stableWarning, policy.WindowSnapshot, policy.AdmissionWait,
		},
		// Run's samples: consecutive safe samples run, a stable warning is degraded, the rest wait.
		{
			"sampling normal", admissionResolution(policy.LineageBalanced), policy.TaskEphemeral,
			admissionHealthySamples(3), policy.WindowSampling, policy.AdmissionNormal,
		},
		{
			"sampling short of the consecutive samples", admissionResolution(policy.LineageBalanced), policy.TaskEphemeral,
			admissionHealthySamples(2), policy.WindowSampling, policy.AdmissionWait,
		},
		{
			"sampling busy CPU", admissionResolution(policy.LineageBalanced), policy.TaskEphemeral,
			admissionBusyCPU(), policy.WindowSampling, policy.AdmissionWait,
		},
		{
			"sampling warning", admissionResolution(policy.LineageBalanced), policy.TaskEphemeral,
			admissionWarningPressure(), policy.WindowSampling, policy.AdmissionWait,
		},
		{
			"sampling critical", admissionResolution(policy.LineageBalanced), policy.TaskEphemeral,
			admissionCriticalPressure(), policy.WindowSampling, policy.AdmissionWait,
		},
		{
			"sampling storage blocked", admissionResolution(policy.LineageBalanced), policy.TaskEphemeral,
			admissionWithDisk(29 * policy.GiB), policy.WindowSampling, policy.AdmissionCleanup,
		},
		{
			"sampling stable warning of the balanced lineage", admissionResolution(policy.LineageBalanced), policy.TaskEphemeral,
			stableWarning, policy.WindowSampling, policy.AdmissionDegraded,
		},
		{
			"sampling stable warning of the constrained lineage", admissionResolution(policy.LineageConstrained),
			policy.TaskEphemeral, stableWarning, policy.WindowSampling, policy.AdmissionWait,
		},
		{
			"sampling stable warning of the minimal lineage", admissionResolution(policy.LineageMinimal),
			policy.TaskEphemeral, stableWarning, policy.WindowSampling, policy.AdmissionWait,
		},
		{
			"sampling stable warning of no lineage", admissionResolution(policy.LineageUnset),
			policy.TaskEphemeral, stableWarning, policy.WindowSampling, policy.AdmissionWait,
		},
		{
			"sampling stable warning of a service", admissionResolution(policy.LineageBalanced), policy.TaskService,
			stableWarning, policy.WindowSampling, policy.AdmissionWait,
		},
		{
			"sampling stable warning of a transaction", admissionResolution(policy.LineageBalanced),
			policy.TaskTransactional, stableWarning, policy.WindowSampling, policy.AdmissionWait,
		},
		{
			"sampling stable warning of a release", admissionResolution(policy.LineageBalanced), policy.TaskRelease,
			stableWarning, policy.WindowSampling, policy.AdmissionWait,
		},
		{
			"sampling a short stable warning", admissionResolution(policy.LineageBalanced), policy.TaskEphemeral,
			stableWarning[:len(stableWarning)-1], policy.WindowSampling, policy.AdmissionWait,
		},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			path, err := policy.DecideAdmission(admissionInput(row.resolution, row.class, row.samples, policy.DefaultPolicy(), row.window))
			if err != nil || path != row.want {
				t.Errorf("path = %d (%v), want %d", path, err, row.want)
			}
		})
	}
}

func TestAResolutionThatAlreadyStopsDecidesItsOwnPathUnderBothWindows(t *testing.T) {
	for _, window := range []policy.EvidenceWindow{policy.WindowSnapshot, policy.WindowSampling} {
		for _, row := range []struct {
			decision policy.Decision
			want     policy.AdmissionPath
		}{
			{policy.DecisionReplan, policy.AdmissionReplan},
			{policy.DecisionCleanup, policy.AdmissionCleanup},
		} {
			// Healthy samples would admit; the resolution's own decision outranks them.
			resolution := admissionResolution(policy.LineageBalanced)
			resolution.Decision = row.decision
			input := admissionInput(resolution, policy.TaskEphemeral, admissionHealthySamples(3), policy.DefaultPolicy(), window)
			if path, err := policy.DecideAdmission(input); err != nil || path != row.want {
				t.Errorf("window %d, resolution %q: path = %d (%v), want %d", window, row.decision, path, err, row.want)
			}
		}
	}
}

func TestAReplanResolutionOutranksABlockedDisk(t *testing.T) {
	resolution := admissionResolution(policy.LineageBalanced)
	resolution.Decision = policy.DecisionReplan
	input := admissionInput(
		resolution, policy.TaskEphemeral, admissionWithDisk(29*policy.GiB), policy.DefaultPolicy(), policy.WindowSnapshot,
	)
	if path, err := policy.DecideAdmission(input); err != nil || path != policy.AdmissionReplan {
		t.Errorf("a strict misfit on a blocked disk decides %d (%v), want the replan the resolution chose", path, err)
	}
}

func TestAWaitingResolutionStillMeetsItsSamples(t *testing.T) {
	// No resolution is produced at wait, but the decision is not the resolution's to make, so the samples decide.
	resolution := admissionResolution(policy.LineageBalanced)
	resolution.Decision = policy.DecisionWait
	input := admissionInput(resolution, policy.TaskEphemeral, admissionHealthySamples(3), policy.DefaultPolicy(), policy.WindowSampling)
	if path, err := policy.DecideAdmission(input); err != nil || path != policy.AdmissionNormal {
		t.Errorf("a resolution at wait over healthy samples decides %d (%v), want %d", path, err, policy.AdmissionNormal)
	}
}

func TestAnUnsetOrUnknownWindowIsRefused(t *testing.T) {
	for _, window := range []policy.EvidenceWindow{policy.WindowUnset, policy.EvidenceWindow(200)} {
		input := admissionInput(
			admissionResolution(policy.LineageBalanced), policy.TaskEphemeral, admissionHealthySamples(3), policy.DefaultPolicy(), window,
		)
		if path, err := policy.DecideAdmission(input); err == nil || path != policy.AdmissionUnset {
			t.Errorf("window %d decided %d (%v), want a refusal that names no path", window, path, err)
		}
	}
}

func TestTheInputsPolicyAloneDecidesAdmission(t *testing.T) {
	// A host with 12 GiB available is normal at the default 9 GiB admission floor and a warning at 16 GiB.
	strict := policy.DefaultPolicy()
	strict.AdmissionMemoryBytes = 16 * policy.GiB
	lenient := policy.DefaultPolicy()
	lenient.AdmissionMemoryBytes = policy.GiB
	healthy := admissionHealthySamples(3)

	for _, row := range []struct {
		name     string
		window   policy.EvidenceWindow
		carried  policy.Policy
		input    policy.Policy
		wantPath policy.AdmissionPath
	}{
		{"snapshot: the resolution's strict policy does not apply", policy.WindowSnapshot, strict, lenient, policy.AdmissionNormal},
		{"snapshot: the resolution's lenient policy does not apply", policy.WindowSnapshot, lenient, strict, policy.AdmissionWait},
		{"sampling: the resolution's strict policy does not apply", policy.WindowSampling, strict, lenient, policy.AdmissionNormal},
		{"sampling: the resolution's lenient policy does not apply", policy.WindowSampling, lenient, strict, policy.AdmissionWait},
		{"sampling: a zero resolution policy does not apply", policy.WindowSampling, policy.Policy{}, lenient, policy.AdmissionNormal},
	} {
		t.Run(row.name, func(t *testing.T) {
			resolution := admissionResolution(policy.LineageBalanced)
			resolution.Policy = row.carried
			input := admissionInput(resolution, policy.TaskEphemeral, healthy, row.input, row.window)
			if path, err := policy.DecideAdmission(input); err != nil || path != row.wantPath {
				t.Errorf("path = %d (%v), want %d", path, err, row.wantPath)
			}
		})
	}
}

func TestTheInputsPolicyAloneDecidesTheStableWarning(t *testing.T) {
	stableWarning := warningAdmissionSamples()
	noWarningFloor := policy.DefaultPolicy()
	noWarningFloor.WarningAdmissionMemoryBytes = 0
	resolution := admissionResolution(policy.LineageBalanced)
	resolution.Policy = noWarningFloor

	// The resolution's policy forbids the warning window, the input's allows it: the input's decides.
	input := admissionInput(resolution, policy.TaskEphemeral, stableWarning, policy.DefaultPolicy(), policy.WindowSampling)
	if path, err := policy.DecideAdmission(input); err != nil || path != policy.AdmissionDegraded {
		t.Errorf("path = %d (%v), want %d: the input's policy allows a stable warning", path, err, policy.AdmissionDegraded)
	}
	if !policy.SparesStableWarning(policy.TaskEphemeral, resolution, stableWarning, policy.DefaultPolicy()) {
		t.Error("the stable warning is not spared under the policy passed, whatever the resolution carries")
	}

	// And the other way round.
	resolution.Policy = policy.DefaultPolicy()
	input = admissionInput(resolution, policy.TaskEphemeral, stableWarning, noWarningFloor, policy.WindowSampling)
	if path, err := policy.DecideAdmission(input); err != nil || path != policy.AdmissionWait {
		t.Errorf("path = %d (%v), want %d: the input's policy has no warning floor", path, err, policy.AdmissionWait)
	}
	if policy.SparesStableWarning(policy.TaskEphemeral, resolution, stableWarning, noWarningFloor) {
		t.Error("the stable warning is spared under a policy with no warning floor")
	}
}

func TestOnlyAnEphemeralChildOfTheBalancedLineageIsSparedAStableWarning(t *testing.T) {
	stableWarning := warningAdmissionSamples()
	rows := []struct {
		name    string
		lineage policy.Lineage
		class   policy.TaskClass
		samples []policy.Sample
		want    bool
	}{
		{"ephemeral balanced", policy.LineageBalanced, policy.TaskEphemeral, stableWarning, true},
		{"ephemeral constrained", policy.LineageConstrained, policy.TaskEphemeral, stableWarning, false},
		{"ephemeral minimal", policy.LineageMinimal, policy.TaskEphemeral, stableWarning, false},
		{"ephemeral without lineage", policy.LineageUnset, policy.TaskEphemeral, stableWarning, false},
		{"service balanced", policy.LineageBalanced, policy.TaskService, stableWarning, false},
		{"transactional balanced", policy.LineageBalanced, policy.TaskTransactional, stableWarning, false},
		{"release balanced", policy.LineageBalanced, policy.TaskRelease, stableWarning, false},
		{
			"ephemeral balanced, window one sample short", policy.LineageBalanced, policy.TaskEphemeral,
			stableWarning[:len(stableWarning)-1], false,
		},
		{"ephemeral balanced, healthy host", policy.LineageBalanced, policy.TaskEphemeral, admissionHealthySamples(3), false},
	}

	for _, row := range rows {
		got := policy.SparesStableWarning(row.class, admissionResolution(row.lineage), row.samples, policy.DefaultPolicy())
		if got != row.want {
			t.Errorf("%s: spared = %t, want %t", row.name, got, row.want)
		}
	}
}

func TestDegradedAdmissionIsExactlyAStableWarningThatIsSpared(t *testing.T) {
	// The sampling window's degraded path and the supervision loop's exemption share one rule: whenever a
	// stable warning is spared and the host is not already normal, the decision is degraded, and never otherwise.
	stableWarning := warningAdmissionSamples()
	for _, lineage := range []policy.Lineage{policy.LineageBalanced, policy.LineageConstrained, policy.LineageMinimal, policy.LineageUnset} {
		for _, class := range []policy.TaskClass{policy.TaskEphemeral, policy.TaskService, policy.TaskTransactional, policy.TaskRelease} {
			for _, samples := range [][]policy.Sample{stableWarning, stableWarning[1:], admissionHealthySamples(3), admissionWarningPressure()} {
				resolution := admissionResolution(lineage)
				path, err := policy.DecideAdmission(admissionInput(resolution, class, samples, policy.DefaultPolicy(), policy.WindowSampling))
				spared := policy.SparesStableWarning(class, resolution, samples, policy.DefaultPolicy())
				if err != nil || (path == policy.AdmissionDegraded) != spared {
					t.Errorf("lineage %d class %q over %d samples: path %d (%v) but spared = %t", lineage, class, len(samples), path, err, spared)
				}
			}
		}
	}
}
