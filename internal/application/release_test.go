package application_test

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
)

type releaseTestClock struct {
	ticks   chan time.Time
	stopped bool
}

func (*releaseTestClock) Wait(ctx context.Context, _ time.Duration) error { return ctx.Err() }
func (clock *releaseTestClock) Now() time.Time                            { return time.Unix(100, 0) }

func (clock *releaseTestClock) Ticker(time.Duration) application.ReleaseTicker {
	return application.ReleaseTicker{Ticks: clock.ticks, Stop: clock.Stop}
}
func (clock *releaseTestClock) Ticks() <-chan time.Time { return clock.ticks }

func (clock *releaseTestClock) Stop() { clock.stopped = true }

type releaseTestCollector struct {
	samples int
	cancel  context.CancelFunc
}

func (collector *releaseTestCollector) Collect(context.Context, policy.CPUState, string) (policy.Reading, error) {
	collector.samples++
	if collector.samples == 2 {
		collector.cancel()
	}
	return policy.Reading{Sample: policy.Sample{AvailableParallelism: 4, MeasuredAt: time.Unix(int64(collector.samples), 0).Format(time.RFC3339Nano)}}, nil
}

type releaseTestRepository struct {
	calls   []string
	fail    string
	summary policy.ReleaseSummary
}

func (repository *releaseTestRepository) step(name string) error {
	repository.calls = append(repository.calls, name)
	if repository.fail == name {
		return io.ErrClosedPipe
	}
	return nil
}

func (repository *releaseTestRepository) OpenReleaseOutput(application.MonitorConfig) (application.ReleaseOutput, string, error) {
	if err := repository.step("open"); err != nil {
		return application.ReleaseOutput{}, "", err
	}
	return application.ReleaseOutput{Append: repository.Append, Close: repository.Close}, "owned-root", nil
}

func (repository *releaseTestRepository) ReadReleaseSummary(string) ([]byte, error) {
	return nil, io.ErrClosedPipe
}

func (repository *releaseTestRepository) Append(evidence.ReleaseSample) error {
	return repository.step("append")
}
func (repository *releaseTestRepository) Close() error { return repository.step("close") }
func (repository *releaseTestRepository) WriteReleaseSummary(_ string, _ io.Writer, summary policy.ReleaseSummary) error {
	repository.summary = summary
	return repository.step("summary")
}

func (repository *releaseTestRepository) CleanupRelease(string, time.Time, ...string) error {
	return repository.step("cleanup")
}

func releaseTestConfig(collector policy.Collector) application.MonitorConfig {
	return application.MonitorConfig{OutputPath: "samples.jsonl", SummaryPath: "summary.json", DeploymentRoot: "deployment", Collector: collector, ServiceRSS: func(context.Context) int64 { return 0 }, Health: func(context.Context) (int, float64) { return 200, 0 }, RoutedHealth: func(context.Context) (int, float64) { return 200, 0 }, LoadAverage: func(context.Context) float64 { return 0 }}
}

func TestReleaseMonitorUsesInjectedTicksAndFinalizesAfterClosing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	clock := &releaseTestClock{ticks: make(chan time.Time, 1)}
	clock.ticks <- time.Unix(101, 0)
	collector := &releaseTestCollector{cancel: cancel}
	repository := &releaseTestRepository{}
	services := application.ReleaseServices{Clock: clock, Evidence: repository}
	if err := services.RunMonitor(ctx, releaseTestConfig(collector)); err != nil {
		t.Fatal(err)
	}
	want := []string{"open", "append", "append", "close", "summary", "cleanup"}
	if !reflect.DeepEqual(repository.calls, want) {
		t.Fatalf("release lifecycle=%v, want %v", repository.calls, want)
	}
	if repository.summary.SampleCount != 2 || !clock.stopped {
		t.Fatalf("sample count=%d, ticker stopped=%t", repository.summary.SampleCount, clock.stopped)
	}
}

func TestReleaseMonitorStopsFinalizationAtFailedPrimitive(t *testing.T) {
	cases := []struct {
		name string
		want []string
	}{
		{"open", []string{"open"}},
		{"append", []string{"open", "append", "close"}},
		{"close", []string{"open", "append", "append", "close", "close"}},
		{"summary", []string{"open", "append", "append", "close", "summary"}},
		{"cleanup", []string{"open", "append", "append", "close", "summary", "cleanup"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			clock := &releaseTestClock{ticks: make(chan time.Time, 1)}
			clock.ticks <- time.Unix(101, 0)
			collector := &releaseTestCollector{cancel: cancel}
			repository := &releaseTestRepository{fail: test.name}
			services := application.ReleaseServices{Clock: clock, Evidence: repository}
			err := services.RunMonitor(ctx, releaseTestConfig(collector))
			if !errors.Is(err, io.ErrClosedPipe) {
				t.Fatalf("error=%v", err)
			}
			if !reflect.DeepEqual(repository.calls, test.want) {
				t.Fatalf("release lifecycle=%v, want %v", repository.calls, test.want)
			}
		})
	}
}

type releaseTestHealth struct{ calls []string }

func (probe *releaseTestHealth) ServiceRSS(context.Context, []int) int64 {
	probe.calls = append(probe.calls, "rss")
	return 0
}

func (probe *releaseTestHealth) Local(context.Context, string) (int, float64) {
	probe.calls = append(probe.calls, "local")
	return 200, 2.5
}

func (probe *releaseTestHealth) Routed(context.Context, string) (int, float64) {
	probe.calls = append(probe.calls, "routed")
	return 200, 75
}

func (probe *releaseTestHealth) LoadAverage(context.Context) float64 {
	probe.calls = append(probe.calls, "load")
	return 0
}

func TestReleaseMonitorUsesHealthCapabilitiesAndKeepsUnavailableMetrics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clock := &releaseTestClock{ticks: make(chan time.Time, 1)}
	clock.ticks <- time.Unix(101, 0)
	collector := &releaseTestCollector{cancel: cancel}
	repository := &releaseTestRepository{}
	probe := &releaseTestHealth{}
	services := application.ReleaseServices{Clock: clock, Evidence: repository, Health: probe}
	config := releaseTestConfig(collector)
	config.ServiceRSS = nil
	config.Health = nil
	config.RoutedHealth = nil
	config.LoadAverage = nil
	config.HealthURL = "http://localhost/health"
	config.RoutedOrigin = "https://example.invalid"
	if err := services.RunMonitor(ctx, config); err != nil {
		t.Fatal(err)
	}
	want := []string{"local", "routed", "load", "rss", "local", "routed", "load", "rss"}
	if !reflect.DeepEqual(probe.calls, want) {
		t.Fatalf("health observations=%v, want %v", probe.calls, want)
	}
	summary := repository.summary
	if summary.ServiceRSSPeakBytes != 0 || summary.HealthLatencyP95Ms != 2.5 || summary.RoutedJourneyLatencyP95Ms != 75 || summary.CompressorAvailableAll {
		t.Fatalf("optional resource evidence changed: %+v", summary)
	}
}

func TestReleaseMonitorValidatesEndpointsBeforeCancelledContext(t *testing.T) {
	for _, test := range []struct{ name, health, origin string }{
		{"missing health", "", "https://example.invalid"},
		{"credentialed health", "http://user@localhost/", "https://example.invalid"},
		{"routed path", "http://localhost/health", "https://example.invalid/path"},
		{"routed query", "http://localhost/health", "https://example.invalid?query=1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			repository := &releaseTestRepository{}
			services := application.ReleaseServices{Clock: &releaseTestClock{}, Evidence: repository, Health: &releaseTestHealth{}}
			config := releaseTestConfig(&releaseTestCollector{})
			config.Health = nil
			config.RoutedHealth = nil
			config.HealthURL = test.health
			config.RoutedOrigin = test.origin
			err := services.RunMonitor(ctx, config)
			if !errors.Is(err, application.ErrMonitorInput) {
				t.Fatalf("expected invocation failure, error=%v", err)
			}
			if len(repository.calls) != 0 {
				t.Fatalf("storage invoked for invalid input: %v", repository.calls)
			}
		})
	}
}

type releaseCheckCollector struct {
	sample   policy.Sample
	readings int
	counter  uint64
	previous []policy.CPUState
	failure  error
}

func (collector *releaseCheckCollector) Collect(_ context.Context, previous policy.CPUState, _ string) (policy.Reading, error) {
	collector.previous = append(collector.previous, append(policy.CPUState(nil), previous...))
	collector.readings++
	collector.counter++
	return policy.Reading{CPUState: policy.CPUState{collector.counter}, Sample: collector.sample}, collector.failure
}

func TestReleaseCheckUsesBoundedConsecutiveObservationsAndTypedVerdicts(t *testing.T) {
	memory, disk := 20*policy.GiB, 40*policy.GiB
	pressure := 1
	compressor := true
	cpu := 10.0
	healthy := policy.Sample{MeasuredAt: time.Unix(0, 0).Format(time.RFC3339Nano), AvailableParallelism: 4, AvailableNonCompressedEstimateBytes: &memory, MemoryPressureLevel: &pressure, CompressorAvailable: &compressor, CPUUtilizationPercent: &cpu, DiskFreeBytes: &disk}
	cases := []struct {
		name            string
		mutate          func(*policy.Sample)
		want            error
		readings, waits int
	}{
		{"ready", func(*policy.Sample) {}, nil, 3, 2},
		{"CPU never settles", func(sample *policy.Sample) { busy := 100.0; sample.CPUUtilizationPercent = &busy }, application.ErrCPUHeadroom, 31, 30},
		{"memory defers", func(sample *policy.Sample) { critical := 4; sample.MemoryPressureLevel = &critical }, application.ErrMemoryHeadroom, 1, 0},
		{"low disk blocks", func(sample *policy.Sample) { low := 20 * policy.GiB; sample.DiskFreeBytes = &low }, application.ErrDiskReserve, 1, 0},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			sample := healthy
			test.mutate(&sample)
			collector := &releaseCheckCollector{sample: sample}
			waits := 0
			err := application.Check(context.Background(), collector, "disk", func(duration time.Duration) {
				if duration != 500*time.Millisecond {
					t.Errorf("pause=%v", duration)
				}
				waits++
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("verdict=%v, want %v", err, test.want)
			}
			if collector.readings != test.readings || waits != test.waits {
				t.Fatalf("readings=%d waits=%d, want %d/%d", collector.readings, waits, test.readings, test.waits)
			}
			for index, previous := range collector.previous {
				if index == 0 && len(previous) != 0 || index > 0 && (len(previous) != 1 || previous[0] != uint64(index)) {
					t.Fatalf("CPU state at observation %d=%v", index, previous)
				}
			}
		})
	}
}

func TestReleaseAssessmentUsesEvidencePortAndPreservesReaderFailure(t *testing.T) {
	services := application.ReleaseServices{Evidence: &releaseTestRepository{}}
	if _, err := services.AssessFile("summary"); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("assessment error=%v", err)
	}
	if _, err := application.Assess(&releaseFailingReader{}); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("reader error=%v", err)
	}
}

type releaseFailingReader struct{}

func (*releaseFailingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
