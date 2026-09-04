package unit_test

import (
	"math"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
)

func policySample(at time.Time) policy.Sample {
	return policy.Sample{
		SchemaVersion:                       2,
		MeasuredAt:                          at.UTC().Format(time.RFC3339Nano),
		AvailableNonCompressedEstimateBytes: new(12 * policy.GiB),
		MemoryPressureLevel:                 new(1),
		CompressorAvailable:                 new(true),
		CompressorPayloadBytes:              new(7 * policy.GiB),
		AvailableParallelism:                12,
		CPUUtilizationPercent:               new(20.0),
		DiskFreeBytes:                       new(40 * policy.GiB),
		PageSizeBytes:                       new(int64(16_384)),
		SwapOuts:                            new(int64(0)),
	}
}

func TestEssentialReadingsAndMemoryStates(t *testing.T) {
	healthy := policySample(time.Unix(0, 0))
	if !policy.EssentialReadingsValid(healthy) || policy.MemoryState(healthy, policy.DefaultPolicy()) != "normal" {
		t.Fatal("healthy sample rejected")
	}

	invalid := healthy
	invalid.AvailableNonCompressedEstimateBytes = nil
	if policy.EssentialReadingsValid(invalid) || policy.MemoryState(invalid, policy.DefaultPolicy()) != "critical" {
		t.Fatal("invalid sample accepted")
	}

	critical := healthy
	critical.MemoryPressureLevel = new(4)
	if policy.MemoryState(critical, policy.DefaultPolicy()) != "critical" {
		t.Fatal("critical pressure ignored")
	}

	compressor := healthy
	compressor.Platform = "darwin"
	compressor.Capabilities = []string{"compressor"}
	compressor.CompressorAvailable = new(false)
	if policy.MemoryState(compressor, policy.DefaultPolicy()) != "critical" {
		t.Fatal("compressor failure ignored")
	}

	low := healthy
	low.AvailableNonCompressedEstimateBytes = new(3 * policy.GiB)
	if policy.MemoryState(low, policy.DefaultPolicy()) != "critical" {
		t.Fatal("critical memory ignored")
	}

	warning := healthy
	warning.MemoryPressureLevel = new(2)
	if policy.MemoryState(warning, policy.DefaultPolicy()) != "warning" {
		t.Fatal("warning pressure ignored")
	}

	warning = healthy
	warning.AvailableNonCompressedEstimateBytes = new(8 * policy.GiB)
	if policy.MemoryState(warning, policy.DefaultPolicy()) != "warning" {
		t.Fatal("warning memory ignored")
	}
}

func TestCPUAdmissionBoundaries(t *testing.T) {
	sample := policySample(time.Unix(0, 0))
	sample.CPUUtilizationPercent = new(100 * (1 - 2.0/12))
	if !policy.CPUAdmissionReady(sample, policy.DefaultPolicy()) {
		t.Fatal("ceiling should be inclusive")
	}

	for _, value := range []*float64{nil, new(-1.0), new(101.0), new(math.NaN()), new(math.Inf(1))} {
		sample.CPUUtilizationPercent = value
		if policy.CPUAdmissionReady(sample, policy.DefaultPolicy()) {
			t.Fatal("invalid CPU accepted")
		}
	}

	sample.CPUUtilizationPercent = new(0.0)
	sample.AvailableParallelism = 0
	if policy.CPUAdmissionReady(sample, policy.DefaultPolicy()) {
		t.Fatal("zero parallelism accepted")
	}
	sample.AvailableParallelism = 1
	if !policy.CPUAdmissionReady(sample, policy.DefaultPolicy()) {
		t.Fatal("reserved CPU clamp failed")
	}

	legacy := policy.DefaultPolicy()
	legacy.MaxCPUUtilizationPercent = 0
	sample.AvailableParallelism = 12
	sample.CPUUtilizationPercent = new(80.0)
	if !policy.CPUAdmissionReady(sample, legacy) {
		t.Fatal("reserved CPU fallback rejected safe utilization")
	}
}

func TestResourceAssessmentBranches(t *testing.T) {
	resourcePolicy := policy.DefaultPolicy()
	base := time.Unix(0, 0)
	if got := policy.ResourceAssessment(nil, resourcePolicy); got.Reason != "missing-sample" || got.State != "critical" {
		t.Fatalf("unexpected empty assessment %+v", got)
	}

	missingDisk := policySample(base)
	missingDisk.DiskFreeBytes = nil
	if got := policy.ResourceAssessment([]policy.Sample{missingDisk}, resourcePolicy); !got.StorageBlocked || got.Reason != "disk-unavailable" {
		t.Fatalf("unexpected missing disk %+v", got)
	}

	diskCritical := policySample(base)
	diskCritical.DiskFreeBytes = new(19 * policy.GiB)
	diskCritical.MemoryPressureLevel = new(2)
	if got := policy.ResourceAssessment([]policy.Sample{diskCritical}, resourcePolicy); got.Reason != "disk-critical" || got.State != "critical" {
		t.Fatalf("unexpected critical disk %+v", got)
	}

	diskWarning := policySample(base)
	diskWarning.DiskFreeBytes = new(29 * policy.GiB)
	if got := policy.ResourceAssessment([]policy.Sample{diskWarning}, resourcePolicy); got.Reason != "disk-warning" || got.State != "warning" {
		t.Fatalf("unexpected warning disk %+v", got)
	}

	memoryWarning := policySample(base)
	memoryWarning.MemoryPressureLevel = new(2)
	if got := policy.ResourceAssessment([]policy.Sample{memoryWarning}, resourcePolicy); got.Reason != "memory-warning" {
		t.Fatalf("unexpected memory warning %+v", got)
	}

	first, second := policySample(base), policySample(base.Add(15*time.Second))
	second.SwapOuts = new(512 * policy.MiB / 16_384)
	if got := policy.ResourceAssessment([]policy.Sample{first, second}, resourcePolicy); got.Reason != "swap-critical" {
		t.Fatalf("unexpected swap critical %+v", got)
	}

	first, second = policySample(base), policySample(base.Add(15*time.Second))
	first.CompressorPayloadBytes = new(14 * policy.GiB)
	second.CompressorPayloadBytes = new(16 * policy.GiB)
	if got := policy.ResourceAssessment([]policy.Sample{first, second}, resourcePolicy); got.Reason != "compressor-critical" {
		t.Fatalf("unexpected compressor critical %+v", got)
	}

	invalidCurrent := policySample(base)
	invalidCurrent.MeasuredAt = "invalid"
	_ = policy.ResourceAssessment([]policy.Sample{invalidCurrent}, resourcePolicy)
	missingValue := policySample(base)
	missingValue.SwapOuts = nil
	_ = policy.ResourceAssessment([]policy.Sample{missingValue}, resourcePolicy)

	current := policySample(base.Add(2 * time.Second))
	invalidStart := policySample(base)
	invalidStart.MeasuredAt = "invalid"
	nilStart := policySample(base)
	nilStart.SwapOuts = nil
	future := policySample(base.Add(3 * time.Second))
	old := policySample(base.Add(-20 * time.Second))
	_ = policy.ResourceAssessment([]policy.Sample{invalidStart, nilStart, future, old, current}, resourcePolicy)

	decreasing := policySample(base.Add(time.Second))
	decreasing.SwapOuts = new(int64(0))
	increasedStart := policySample(base)
	increasedStart.SwapOuts = new(int64(10))
	_ = policy.ResourceAssessment([]policy.Sample{increasedStart, decreasing}, resourcePolicy)
}

func TestAdmissionPercentileAndReleasePolicies(t *testing.T) {
	base := time.Unix(0, 0)
	samples := []policy.Sample{policySample(base), policySample(base.Add(time.Second)), policySample(base.Add(2 * time.Second))}
	if !policy.AdmissionReady(samples, policy.DefaultPolicy()) {
		t.Fatal("healthy tail rejected")
	}

	samples[2].CPUUtilizationPercent = new(100.0)
	if policy.AdmissionReady(samples, policy.DefaultPolicy()) {
		t.Fatal("busy tail accepted")
	}

	histogram := evidence.NewHistogram(100, 1)
	if evidence.NewHistogram(math.NaN(), 1).Add(1) {
		t.Fatal("histogram with invalid bounds accepted a value")
	}
	if histogram.Add(math.NaN()) || histogram.Add(math.Inf(1)) {
		t.Fatal("invalid histogram values were accepted")
	}
	if histogram.Add(101) {
		t.Fatal("out-of-range histogram value was accepted")
	}
	if _, ok := histogram.Quantile(0); ok {
		t.Fatal("invalid histogram quantile was accepted")
	}
	for _, value := range []float64{3, 1, 2} {
		if !histogram.Add(value) {
			t.Fatalf("valid histogram value %v was rejected", value)
		}
	}
	if got, ok := histogram.Quantile(.5); !ok || got != 2 {
		t.Fatalf("unexpected percentile %v, valid=%t", got, ok)
	}

	healthy := policy.ReleaseSummary{
		SchemaVersion:                          2,
		SampleCount:                            3,
		AvailableParallelism:                   12,
		AvailableNonCompressedEstimateMinBytes: 13 * policy.GiB,
		MemoryPressureLevelMax:                 1,
		CompressorAvailableAll:                 true,
		CPUUtilizationP95Percent:               50,
	}

	if !policy.ReleaseHeadroomAvailable(healthy) {
		t.Fatal("healthy release rejected")
	}
	healthyRouted := healthy
	healthyRouted.SchemaVersion = 4
	healthyRouted.RoutedJourneyLatencyP95Ms = policy.ReleaseRoutedLatencyP95BudgetMs
	healthyRouted.RoutedJourneyLatencyMaxMs = policy.ReleaseRoutedLatencyMaxBudgetMs
	if !policy.ReleaseHeadroomAvailable(healthyRouted) {
		t.Fatal("healthy routed release rejected")
	}

	slowRouted := healthyRouted
	slowRouted.RoutedJourneyLatencyP95Ms++
	if policy.ReleaseHeadroomAvailable(slowRouted) {
		t.Fatal("slow routed p95 accepted")
	}

	spikyRouted := healthyRouted
	spikyRouted.RoutedJourneyLatencyMaxMs++
	if policy.ReleaseHeadroomAvailable(spikyRouted) {
		t.Fatal("slow routed maximum accepted")
	}

	failedRouted := healthyRouted
	failedRouted.RoutedJourneyFailures = 1
	if policy.ReleaseHeadroomAvailable(failedRouted) {
		t.Fatal("failed routed request accepted")
	}

	invalidParallelism := healthy
	invalidParallelism.AvailableParallelism = 0
	if policy.ReleaseHeadroomAvailable(invalidParallelism) {
		t.Fatal("zero parallelism accepted")
	}

	swapPressure := healthy
	swapPressure.AvailableNonCompressedEstimateMinBytes = 3 * policy.GiB
	swapPressure.SwapOutsDelta = 1
	if policy.ReleaseHeadroomAvailable(swapPressure) {
		t.Fatal("swap pressure accepted")
	}

	bad := []policy.ReleaseSummary{healthy, healthy, healthy, healthy, healthy, healthy}
	bad[0].SampleCount = 0
	bad[1].AvailableNonCompressedEstimateMinBytes = policy.GiB
	bad[2].MemoryPressureLevelMax = 2
	bad[3].CompressorAvailableAll = false
	bad[4].CPUUtilizationP95Percent = 100
	bad[5].HealthFailures = 1
	for _, summary := range bad {
		if policy.ReleaseHeadroomAvailable(summary) {
			t.Fatalf("unhealthy release accepted %+v", summary)
		}
	}

	memory := policySample(base)
	if !policy.ReleaseMemoryAvailable(memory) {
		t.Fatal("healthy release memory rejected")
	}
	memory.AvailableNonCompressedEstimateBytes = nil
	if policy.ReleaseMemoryAvailable(memory) {
		t.Fatal("missing release memory accepted")
	}
}

func warningAdmissionSamples() []policy.Sample {
	samples := make([]policy.Sample, 16)

	for index := range samples {
		sample := policySample(time.Unix(int64(index), 0))
		level, available, oom := 2, 8*policy.GiB, int64(0)
		sample.Platform = "darwin"
		sample.EffectiveMemoryLimitBytes = 32 * policy.GiB
		sample.AvailableMemoryBytes = &available
		sample.AvailableNonCompressedEstimateBytes = &available
		sample.OOMEvents = &oom
		sample.OOMKillEvents = &oom
		sample.MemoryPressureLevel = &level
		samples[index] = sample
	}

	return samples
}

func TestWarningAdmissionRequiresStableDarwinHeadroom(t *testing.T) {
	resourcePolicy := policy.DefaultPolicy()
	baseline := warningAdmissionSamples()
	if !policy.WarningAdmissionReady(baseline, resourcePolicy) {
		t.Fatal("stable Darwin warning was not admitted")
	}

	short := append([]policy.Sample(nil), baseline[:len(baseline)-1]...)
	if policy.WarningAdmissionReady(short, resourcePolicy) {
		t.Fatal("short warning window was admitted")
	}

	zeroFloor := resourcePolicy
	zeroFloor.WarningAdmissionMemoryBytes = 0
	if policy.WarningAdmissionReady(baseline, zeroFloor) {
		t.Fatal("missing warning floor was admitted")
	}

	cases := map[string]func([]policy.Sample){
		"low memory":              func(samples []policy.Sample) { samples[4].AvailableMemoryBytes = new(7 * policy.GiB) },
		"critical pressure":       func(samples []policy.Sample) { samples[4].MemoryPressureLevel = new(4) },
		"mixed platform":          func(samples []policy.Sample) { samples[4].Platform = "linux" },
		"compressor unavailable":  func(samples []policy.Sample) { samples[4].CompressorAvailable = new(false) },
		"disk below reserve":      func(samples []policy.Sample) { samples[4].DiskFreeBytes = new(29 * policy.GiB) },
		"busy CPU":                func(samples []policy.Sample) { samples[len(samples)-1].CPUUtilizationPercent = new(100.0) },
		"OOM event":               func(samples []policy.Sample) { samples[4].OOMEvents = new(int64(1)) },
		"OOM kill":                func(samples []policy.Sample) { samples[4].OOMKillEvents = new(int64(1)) },
		"swap growth":             func(samples []policy.Sample) { samples[len(samples)-1].SwapOuts = new(128 * policy.MiB / 16_384) },
		"compressor growth":       func(samples []policy.Sample) { samples[len(samples)-1].CompressorPayloadBytes = new(8 * policy.GiB) },
		"normal current pressure": func(samples []policy.Sample) { samples[len(samples)-1].MemoryPressureLevel = new(1) },
	}

	for name, mutate := range cases {
		samples := append([]policy.Sample(nil), baseline...)
		mutate(samples)
		if policy.WarningAdmissionReady(samples, resourcePolicy) {
			t.Errorf("%s was admitted", name)
		}
	}
}
