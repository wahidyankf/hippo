package integration_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wahidyankf/resource-guard/internal/evidence"
	"github.com/wahidyankf/resource-guard/internal/policy"
	releaseguard "github.com/wahidyankf/resource-guard/internal/release"
)

func TestReleaseMonitorWritesAndAssessesPrivateEvidence(t *testing.T) {
	root := t.TempDir()
	outputPath, summaryPath := filepath.Join(root, "samples.jsonl"), filepath.Join(root, "summary.json")
	base := time.Now()
	samples := []policy.Sample{
		integrationSample(base),
		integrationSample(base.Add(time.Millisecond)),
		integrationSample(base.Add(2 * time.Millisecond)),
	}

	for index := range samples {
		swapIn, swapOut, swapFree := int64(index), int64(index), 2*policy.GiB
		samples[index].SwapIns, samples[index].SwapOuts, samples[index].SwapFreeBytes = &swapIn, &swapOut, &swapFree
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	collector := &integrationCollector{samples: samples, cancel: cancel, cancelAfter: len(samples)}

	err := releaseguard.RunMonitor(ctx, releaseguard.MonitorConfig{
		OutputPath:     outputPath,
		SummaryPath:    summaryPath,
		DeploymentRoot: root,
		Collector:      collector,
		Interval:       time.Millisecond,
		ServiceRSS:     func(context.Context) int64 { return 4096 },
		Health:         func(context.Context) (int, float64) { return 200, 2.5 },
		RoutedHealth:   func(context.Context) (int, float64) { return 200, 75 },
		LoadAverage:    func(context.Context) float64 { return 1.5 },
	})
	if err != nil {
		t.Fatal(err)
	}

	if info, statError := os.Stat(outputPath); statError != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("invalid output mode: info=%v error=%v", info, statError)
	}

	summary, err := releaseguard.AssessFile(summaryPath)
	if err != nil {
		t.Fatal(err)
	}

	if summary.SchemaVersion != 5 ||
		summary.SampleCount < 2 ||
		summary.ServiceRSSPeakBytes != 4096 ||
		summary.HealthLatencyP95Ms != 2.5 ||
		summary.HealthFailures != 0 ||
		summary.RoutedJourneyFailures != 0 ||
		summary.RoutedJourneyLatencyP95Ms != 75 ||
		summary.RoutedJourneyLatencyMaxMs != 75 {
		t.Fatalf("unexpected summary %+v", summary)
	}
}

func TestReleaseMonitorRotatesRawEvidenceWithoutTruncatingSummary(t *testing.T) {
	root := t.TempDir()
	outputPath := filepath.Join(root, "samples.jsonl")
	summaryPath := filepath.Join(root, "summary.json")
	samples := make([]policy.Sample, 30)
	for index := range samples {
		samples[index] = integrationSample(time.Unix(int64(index), 0))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	collector := &integrationCollector{samples: samples, cancel: cancel, cancelAfter: len(samples)}
	err := releaseguard.RunMonitor(ctx, releaseguard.MonitorConfig{
		OutputPath:     outputPath,
		SummaryPath:    summaryPath,
		DeploymentRoot: root,
		Collector:      collector,
		Interval:       time.Microsecond,
		EvidenceLimits: evidence.Limits{ChunkBytes: 2048, Chunks: 5},
		ServiceRSS:     func(context.Context) int64 { return 4096 },
		Health:         func(context.Context) (int, float64) { return 200, 2.5 },
		RoutedHealth:   func(context.Context) (int, float64) { return 200, 75 },
		LoadAverage:    func(context.Context) float64 { return 1.5 },
	})
	if err != nil {
		t.Fatal(err)
	}

	chunks, err := filepath.Glob(filepath.Join(root, "samples*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 5 {
		t.Fatalf("retained %d release evidence chunks, want 5", len(chunks))
	}

	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatal(err)
	}
	var summary policy.ReleaseSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatal(err)
	}
	if summary.SampleCount != len(samples) {
		t.Fatalf("summary counted %d samples, want %d", summary.SampleCount, len(samples))
	}
}

func TestReleaseAssessmentRejectsInvalidAndUnhealthyEvidence(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "legacy.json")
	legacyData := []byte(`{"schemaVersion":3,"sampleCount":1,"availableParallelism":12,"availableNonCompressedEstimateMinBytes":13958643712,"memoryPressureLevelMax":1,"compressorAvailableAll":true,"cpuUtilizationP95Percent":10,"healthFailures":0}`)
	if err := os.WriteFile(legacy, legacyData, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := releaseguard.AssessFile(legacy); err != nil {
		t.Fatalf("legacy summary rejected: %v", err)
	}

	legacyRouted := filepath.Join(root, "legacy-routed.json")
	legacyRoutedData := []byte(`{"schemaVersion":4,"sampleCount":1,"availableParallelism":12,"availableNonCompressedEstimateMinBytes":13958643712,"memoryPressureLevelMax":1,"compressorAvailableAll":true,"cpuUtilizationP95Percent":10,"healthFailures":0,"routedJourneyLatencyP95Ms":50,"routedJourneyLatencyMaxMs":100}`)
	if err := os.WriteFile(legacyRouted, legacyRoutedData, 0o600); err != nil {
		t.Fatal(err)
	}

	if summary, err := releaseguard.AssessFile(legacyRouted); err != nil || summary.SchemaVersion != 4 {
		t.Fatalf("legacy routed summary rejected: summary=%+v error=%v", summary, err)
	}

	invalid := filepath.Join(root, "invalid.json")
	if err := os.WriteFile(invalid, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := releaseguard.AssessFile(invalid); err == nil {
		t.Fatal("invalid summary accepted")
	}

	unhealthy := filepath.Join(root, "unhealthy.json")
	data := []byte(`{"schemaVersion":2,"sampleCount":1,"availableParallelism":12,"availableNonCompressedEstimateMinBytes":13958643712,"memoryPressureLevelMax":1,"compressorAvailableAll":true,"cpuUtilizationP95Percent":10,"healthFailures":1}`)
	if err := os.WriteFile(unhealthy, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := releaseguard.AssessFile(unhealthy); err == nil {
		t.Fatal("unhealthy summary accepted")
	}
}
