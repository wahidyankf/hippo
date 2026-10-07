package application_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"

	"github.com/wahidyankf/hippo/internal/application"
)

func TestReleaseContractMissingHealthDependencyRefusesBeforeOpeningEvidence(t *testing.T) {
	repository := &releaseTestRepository{}
	services := application.ReleaseServices{Clock: &releaseTestClock{}, Evidence: repository}
	config := releaseTestConfig(&releaseTestCollector{})
	config.Health = nil
	config.HealthURL = "http://localhost/health"
	err := services.RunMonitor(context.Background(), config)
	if err == nil || !strings.Contains(err.Error(), "health probe is required") || len(repository.calls) != 0 {
		t.Fatalf("missing dependency error=%v, evidence calls=%v", err, repository.calls)
	}
}

type releaseContractRepository struct {
	releaseTestRepository

	root      string
	config    application.MonitorConfig
	data      []byte
	readError error
	cleanupAt time.Time
}

func (repository *releaseContractRepository) OpenReleaseOutput(config application.MonitorConfig) (application.ReleaseOutput, string, error) {
	repository.config = config
	output, _, err := repository.releaseTestRepository.OpenReleaseOutput(config)
	return output, repository.root, err
}

func (repository *releaseContractRepository) ReadReleaseSummary(string) ([]byte, error) {
	return repository.data, repository.readError
}

func (repository *releaseContractRepository) CleanupRelease(root string, now time.Time, active ...string) error {
	repository.cleanupAt = now
	return repository.releaseTestRepository.CleanupRelease(root, now, active...)
}

func TestReleaseContractMonitorRefusesInvalidInputsBeforeEvidence(t *testing.T) {
	cases := []struct {
		name    string
		modify  func(*application.ReleaseServices, *application.MonitorConfig)
		message string
		input   bool
	}{
		{"missing raw", func(_ *application.ReleaseServices, c *application.MonitorConfig) { c.OutputPath = "" }, "output, summary", true},
		{"missing summary", func(_ *application.ReleaseServices, c *application.MonitorConfig) { c.SummaryPath = "" }, "output, summary", true},
		{"missing deployment", func(_ *application.ReleaseServices, c *application.MonitorConfig) { c.DeploymentRoot = "" }, "deployment root", true},
		{"duplicate raw", func(_ *application.ReleaseServices, c *application.MonitorConfig) { c.RawOutput = io.Discard }, "exactly one", true},
		{"duplicate summary", func(_ *application.ReleaseServices, c *application.MonitorConfig) { c.SummaryOutput = io.Discard }, "exactly one", true},
		{"collector", func(_ *application.ReleaseServices, c *application.MonitorConfig) { c.Collector = nil }, "host collector", false},
		{"interval", func(_ *application.ReleaseServices, c *application.MonitorConfig) { c.Interval = -time.Second }, "nonnegative", false},
		{"clock", func(s *application.ReleaseServices, _ *application.MonitorConfig) { s.Clock = nil }, "release clock", false},
		{"evidence", func(s *application.ReleaseServices, _ *application.MonitorConfig) { s.Evidence = nil }, "release evidence", false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			repository := &releaseTestRepository{}
			services := application.ReleaseServices{Clock: &releaseTestClock{}, Evidence: repository}
			config := releaseTestConfig(&releaseTestCollector{})
			test.modify(&services, &config)
			err := services.RunMonitor(context.Background(), config)
			if err == nil || !strings.Contains(err.Error(), test.message) || errors.Is(err, application.ErrMonitorInput) != test.input || len(repository.calls) != 0 {
				t.Fatalf("error=%v evidence calls=%v", err, repository.calls)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repository := &releaseTestRepository{}
	err := (application.ReleaseServices{Clock: &releaseTestClock{}, Evidence: repository}).RunMonitor(ctx, releaseTestConfig(&releaseTestCollector{}))
	if !errors.Is(err, context.Canceled) || len(repository.calls) != 0 {
		t.Fatalf("cancel error=%v calls=%v", err, repository.calls)
	}
}

func TestReleaseContractPartialHealthDependenciesAllRefuseBeforeStorage(t *testing.T) {
	for _, missing := range []string{"rss", "local", "routed", "load"} {
		t.Run(missing, func(t *testing.T) {
			config := releaseTestConfig(&releaseTestCollector{})
			config.HealthURL, config.RoutedOrigin = "http://localhost/health", "https://example.invalid"
			switch missing {
			case "rss":
				config.ServiceRSS = nil
			case "local":
				config.Health = nil
			case "routed":
				config.RoutedHealth = nil
			case "load":
				config.LoadAverage = nil
			}
			repository := &releaseTestRepository{}
			err := (application.ReleaseServices{Clock: &releaseTestClock{}, Evidence: repository}).RunMonitor(context.Background(), config)
			if err == nil || !strings.Contains(err.Error(), "health probe is required") || len(repository.calls) != 0 {
				t.Fatalf("dependency error=%v calls=%v", err, repository.calls)
			}
		})
	}
	// Endpoint syntax remains a caller error even when the runtime capability is absent.
	config := releaseTestConfig(&releaseTestCollector{})
	config.Health = nil
	err := (application.ReleaseServices{}).RunMonitor(context.Background(), config)
	if !errors.Is(err, application.ErrMonitorInput) {
		t.Fatalf("endpoint precedence error=%v", err)
	}
}

func TestReleaseContractSamplingFailureRetainsPartialOutputWithoutSummary(t *testing.T) {
	for _, failAt := range []int{1, 2} {
		t.Run(strconv.Itoa(failAt), func(t *testing.T) {
			fixture := newObservationFixture()
			clock := &releaseTestClock{ticks: make(chan time.Time, 1)}
			clock.ticks <- fixture.now
			collector := &observationContractCollector{fixture: fixture, failAt: failAt, failure: io.ErrUnexpectedEOF}
			repository := &releaseTestRepository{}
			err := (application.ReleaseServices{Clock: clock, Evidence: repository}).RunMonitor(context.Background(), releaseTestConfig(collector))
			want := []string{"open", "close"}
			if failAt == 2 {
				want = []string{"open", "append", "close"}
			}
			if !errors.Is(err, io.ErrUnexpectedEOF) || !reflect.DeepEqual(repository.calls, want) || clock.stopped != (failAt == 2) {
				t.Fatalf("error=%v calls=%v stopped=%t", err, repository.calls, clock.stopped)
			}
		})
	}
}

func TestReleaseContractCancellationDuringProbeFinalizesOnlyRecordedSamples(t *testing.T) {
	fixture := newObservationFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clock := &releaseTestClock{ticks: make(chan time.Time, 1)}
	clock.ticks <- fixture.now
	collector := &observationContractCollector{fixture: fixture, failAt: 2, failure: context.Canceled, cancel: cancel}
	repository := &releaseContractRepository{root: ""}
	err := (application.ReleaseServices{Clock: clock, Evidence: repository}).RunMonitor(ctx, releaseTestConfig(collector))
	want := []string{"open", "append", "close", "summary"}
	if err != nil || !reflect.DeepEqual(repository.calls, want) || repository.summary.SampleCount != 1 || !clock.stopped {
		t.Fatalf("error=%v calls=%v summary=%+v stopped=%t", err, repository.calls, repository.summary, clock.stopped)
	}
}

func TestReleaseContractClockAndStreamDestinationsReachEvidenceUnchanged(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clock := &releaseTestClock{ticks: make(chan time.Time, 1)}
	clock.ticks <- time.Unix(101, 0)
	repository := &releaseContractRepository{root: "owned-root"}
	collector := &releaseTestCollector{cancel: cancel}
	config := releaseTestConfig(collector)
	config.OutputPath, config.SummaryPath = "", ""
	raw, summary := &bytes.Buffer{}, &bytes.Buffer{}
	config.RawOutput, config.SummaryOutput = raw, summary
	now := time.Unix(500, 0)
	config.Now = func() time.Time { return now }
	err := (application.ReleaseServices{Clock: clock, Evidence: repository}).RunMonitor(ctx, config)
	if err != nil || repository.config.Interval != time.Second || repository.config.RawOutput != raw || repository.config.SummaryOutput != summary || !repository.cleanupAt.Equal(now) {
		t.Fatalf("error=%v destinations=%+v cleanup=%v", err, repository.config, repository.cleanupAt)
	}
}

func TestReleaseContractCheckPreservesDependencyProbeAndCancellationErrors(t *testing.T) {
	if err := application.Check(context.Background(), nil, "disk", nil); err == nil {
		t.Fatal("missing collector accepted")
	}
	fixture := newObservationFixture()
	collector := &releaseCheckCollector{sample: fixture.sample, failure: io.ErrUnexpectedEOF}
	if err := application.Check(context.Background(), collector, "disk", nil); !errors.Is(err, io.ErrUnexpectedEOF) || collector.readings != 1 {
		t.Fatalf("probe error=%v readings=%d", err, collector.readings)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	collector = &releaseCheckCollector{sample: fixture.sample}
	if err := application.Check(ctx, collector, "disk", nil); !errors.Is(err, context.Canceled) || collector.readings != 0 {
		t.Fatalf("cancel error=%v readings=%d", err, collector.readings)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	if err := application.Check(ctx, collector, "disk", func(time.Duration) { cancel() }); !errors.Is(err, context.Canceled) || collector.readings != 1 {
		t.Fatalf("pause cancellation=%v readings=%d", err, collector.readings)
	}
}

func TestReleaseContractCheckRequiresConsecutiveReadinessAfterBusySample(t *testing.T) {
	fixture := newObservationFixture()
	busy := fixture.sample
	cpu := 100.0
	busy.CPUUtilizationPercent = &cpu
	fixture.sampleSequence = []policy.Sample{fixture.sample, fixture.sample, busy, fixture.sample, fixture.sample, fixture.sample}
	waits := 0
	err := application.Check(context.Background(), fixture, "disk", func(time.Duration) { waits++ })
	if err != nil || fixture.samples != 6 || waits != 5 {
		t.Fatalf("consecutive verdict=%v samples=%d waits=%d", err, fixture.samples, waits)
	}
}

func TestReleaseContractAssessmentDistinguishesUnusableAndRejectedEvidence(t *testing.T) {
	cases := []struct {
		name, data         string
		code               status.Code
		assessed, accepted bool
	}{
		{"accepted", `{"schemaVersion":3,"sampleCount":1,"availableParallelism":12,"availableNonCompressedEstimateMinBytes":13958643712,"memoryPressureLevelMax":1,"compressorAvailableAll":true,"cpuUtilizationP95Percent":10,"healthFailures":0}`, "", true, true},
		{"rejected", `{"schemaVersion":3,"sampleCount":1,"availableParallelism":12,"availableNonCompressedEstimateMinBytes":13958643712,"memoryPressureLevelMax":1,"compressorAvailableAll":true,"cpuUtilizationP95Percent":10,"healthFailures":1}`, status.CodeLimitReleaseEnvelopeExceeded, true, false},
		{"malformed", `{`, status.CodeEvidenceUnreadable, false, false},
		{"invalid schema", `{"schemaVersion":99}`, status.CodeEvidenceUnreadable, false, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newObservationFixture()
			result, code, err := releaseEntryServices(fixture).AssessRelease(application.ReleaseAssessRequest{SummaryPath: "-", Input: strings.NewReader(test.data)})
			if test.code != "" {
				requireContractFailure(t, err, test.code)
			} else if err != nil {
				t.Fatal(err)
			}
			if code != 0 || result.Assessed != test.assessed || result.Accepted != test.accepted || len(fixture.calls) != 1 {
				t.Fatalf("result=%+v code=%d calls=%v", result, code, fixture.calls)
			}
			if test.assessed && result.SchemaVersion != 3 {
				t.Fatalf("assessed schema=%d", result.SchemaVersion)
			}
		})
	}
	if _, err := (application.ReleaseServices{}).AssessFile("summary"); err == nil {
		t.Fatal("missing evidence accepted")
	}
	data := cases[0].data
	summary, err := (application.ReleaseServices{Evidence: &releaseContractRepository{data: []byte(data)}}).AssessFile("summary")
	if err != nil || summary.SchemaVersion != 3 {
		t.Fatalf("file assessment=%+v error=%v", summary, err)
	}
}

func TestReleaseContractEntryAssessmentValidationPrecedesRead(t *testing.T) {
	fixture := newObservationFixture()
	result, _, err := releaseEntryServices(fixture).AssessRelease(application.ReleaseAssessRequest{})
	requireContractFailure(t, err, status.CodeArgsInvalid)
	if result.Assessed || len(fixture.calls) != 0 {
		t.Fatalf("missing path result=%+v calls=%v", result, fixture.calls)
	}
	fixture.failureAt, fixture.failure = "load", io.ErrClosedPipe
	result, _, err = releaseEntryServices(fixture).AssessRelease(application.ReleaseAssessRequest{SummaryPath: "summary"})
	requireContractFailure(t, err, status.CodeConfigUnreadable)
	if result.Assessed || !reflect.DeepEqual(fixture.calls, []string{"load"}) {
		t.Fatalf("unreadable config result=%+v calls=%v", result, fixture.calls)
	}
}

func TestReleaseContractEntryCheckRefusalsAndSamplingFailures(t *testing.T) {
	cases := []struct {
		name    string
		phase   string
		profile policy.ProfileName
		change  func(*observationFixture)
		failure status.Code
		reason  policy.Reason
		code    int
	}{
		{name: "configuration", phase: "load", failure: status.CodeConfigUnreadable},
		{name: "probe", phase: "collect", code: 1},
		{name: "unknown profile", profile: "missing", reason: policy.ReasonReplanRequired},
		{name: "strict capacity", change: func(f *observationFixture) { small := policy.GiB; f.sample.AvailableMemoryBytes = &small }, reason: policy.ReasonReplanRequired},
		{name: "hard disk floor", change: func(f *observationFixture) { low := int64(1); f.sample.DiskFreeBytes = &low }, reason: policy.ReasonStorageBlocked},
		{name: "memory pressure", change: func(f *observationFixture) {
			warning := f.sample
			pressure := 4
			warning.MemoryPressureLevel = &pressure
			f.sampleSequence = []policy.Sample{f.sample, warning}
		}, failure: status.CodeLimitCapacityDeferred},
		{name: "disk reserve", change: func(f *observationFixture) {
			low := f.sample
			disk := policy.HardDiskFloorBytes + 1
			low.DiskFreeBytes = &disk
			f.sampleSequence = []policy.Sample{f.sample, low}
		}, failure: status.CodeLimitStorageBlocked},
		{name: "busy CPU", change: func(f *observationFixture) {
			busy := f.sample
			cpu := 100.0
			busy.CPUUtilizationPercent = &cpu
			f.sampleSequence = []policy.Sample{f.sample, busy}
		}, failure: status.CodeLimitCapacityDeferred},
		{name: "sampling wait", phase: "wait", code: 1},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newObservationFixture()
			fixture.sample.PhysicalMemoryBytes = 64 * policy.GiB
			fixture.sample.EffectiveMemoryLimitBytes = 64 * policy.GiB
			fixture.failureAt, fixture.failure = test.phase, io.ErrClosedPipe
			if test.change != nil {
				test.change(fixture)
			}
			code, err := releaseEntryServices(fixture).CheckRelease(context.Background(), application.ReleaseCheckRequest{RequestedProfile: test.profile})
			if code != test.code || err == nil {
				t.Fatalf("code=%d error=%v", code, err)
			}
			if test.failure != "" {
				requireContractFailure(t, err, test.failure)
			}
			if test.reason != policy.ReasonNone {
				stop, ok := errors.AsType[*policy.Stop](err)
				if !ok || stop.Reason != test.reason {
					t.Fatalf("reason error=%v", err)
				}
			}
			if test.code == 1 && !errors.Is(err, io.ErrClosedPipe) {
				t.Fatalf("sampling error lost: %v", err)
			}
		})
	}
}

func TestReleaseContractEntryMonitorValidatesBeforeConfiguration(t *testing.T) {
	cases := []application.ReleaseMonitorRequest{
		{DurationMs: -1},
		{DurationMs: math.MaxInt64},
		{MonitorConfig: application.MonitorConfig{ServicePorts: []int{0}}},
		{MonitorConfig: application.MonitorConfig{ServicePorts: []int{65536}}},
		{MonitorConfig: application.MonitorConfig{OutputPath: "-", SummaryPath: "-"}},
	}
	for index, request := range cases {
		t.Run(strconv.Itoa(index), func(t *testing.T) {
			fixture := newObservationFixture()
			code, err := releaseEntryServices(fixture).MonitorRelease(context.Background(), request, func(context.Context, application.MonitorConfig) error {
				t.Error("invalid invocation reached monitor")
				return nil
			})
			requireContractFailure(t, err, status.CodeArgsInvalid)
			if code != 0 || len(fixture.calls) != 0 {
				t.Fatalf("code=%d calls=%v", code, fixture.calls)
			}
		})
	}
	fixture := newObservationFixture()
	fixture.failureAt, fixture.failure = "load", io.ErrClosedPipe
	_, err := releaseEntryServices(fixture).MonitorRelease(context.Background(), application.ReleaseMonitorRequest{}, nil)
	requireContractFailure(t, err, status.CodeConfigUnreadable)
}

func TestReleaseContractEntryMonitorMapsStreamsDeadlineAndInvocationErrors(t *testing.T) {
	for _, raw := range []bool{false, true} {
		t.Run(map[bool]string{false: "summary stream", true: "raw stream"}[raw], func(t *testing.T) {
			fixture := newObservationFixture()
			output := &bytes.Buffer{}
			request := application.ReleaseMonitorRequest{MonitorConfig: application.MonitorConfig{OutputPath: "raw", SummaryPath: "summary", ServicePorts: []int{1, 65535}}, DurationMs: 1000, Stdout: output}
			if raw {
				request.OutputPath = "-"
			} else {
				request.SummaryPath = "-"
			}
			var invokedContextDone <-chan struct{}
			code, err := releaseEntryServices(fixture).MonitorRelease(context.Background(), request, func(ctx context.Context, config application.MonitorConfig) error {
				invokedContextDone = ctx.Done()
				if _, ok := ctx.Deadline(); !ok {
					t.Error("bounded monitor lacks deadline")
				}
				if raw {
					if config.OutputPath != "" || config.RawOutput != output {
						t.Error("raw stream not mapped")
					}
				} else if config.SummaryPath != "" || config.SummaryOutput != output {
					t.Error("summary stream not mapped")
				}
				return io.ErrClosedPipe
			})
			if code != 1 || !errors.Is(err, io.ErrClosedPipe) {
				t.Fatalf("code=%d error=%v", code, err)
			}
			select {
			case <-invokedContextDone:
			default:
				t.Fatal("bounded monitor context was not cancelled on return")
			}
		})
	}
	fixture := newObservationFixture()
	services := releaseEntryServices(fixture)
	services.Evidence = &releaseTestRepository{}
	code, err := services.MonitorRelease(context.Background(), application.ReleaseMonitorRequest{}, nil)
	requireContractFailure(t, err, status.CodeArgsInvalid)
	if code != 0 {
		t.Fatalf("input refusal code=%d", code)
	}
}

type releaseContractCancelCollector struct {
	fixture *observationFixture
	cancel  context.CancelFunc
}

func (collector *releaseContractCancelCollector) Collect(ctx context.Context, previous policy.CPUState, path string) (policy.Reading, error) {
	reading, err := collector.fixture.Collect(ctx, previous, path)
	collector.cancel()
	return reading, err
}

func TestReleaseContractDefaultClockWaitsForConsecutiveSamplesAndCancelsPromptly(t *testing.T) {
	fixture := newObservationFixture()
	// The optional pause hook is absent: the public compatibility API owns its timer.
	if err := application.Check(context.Background(), fixture, "disk", nil); err != nil || fixture.samples != 3 {
		t.Fatalf("default clock error=%v samples=%d", err, fixture.samples)
	}
	fixture = newObservationFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	collector := &releaseContractCancelCollector{fixture: fixture, cancel: cancel}
	if err := application.Check(ctx, collector, "disk", nil); !errors.Is(err, context.Canceled) || fixture.samples != 1 {
		t.Fatalf("default clock cancellation error=%v samples=%d", err, fixture.samples)
	}
}

func TestReleaseContractPendingTickCannotAppendAfterCallerStop(t *testing.T) {
	fixture := newObservationFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	collector := &releaseContractCancelCollector{fixture: fixture, cancel: cancel}
	clock := &releaseTestClock{ticks: make(chan time.Time, 1)}
	clock.ticks <- fixture.now
	repository := &releaseTestRepository{}
	err := (application.ReleaseServices{Clock: clock, Evidence: repository}).RunMonitor(ctx, releaseTestConfig(collector))
	if err != nil || repository.summary.SampleCount != 1 || fixture.samples != 1 || !clock.stopped {
		t.Fatalf("cancel error=%v summary count=%d collected=%d stopped=%t", err, repository.summary.SampleCount, fixture.samples, clock.stopped)
	}
}
