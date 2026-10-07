package application_test

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

type runContractFixture struct {
	now                                                                                                                                        time.Time
	trace                                                                                                                                      []string
	receiptStates, receiptReasons                                                                                                              []string
	stderr                                                                                                                                     bytes.Buffer
	lease                                                                                                                                      application.Lease
	done                                                                                                                                       chan application.ChildCompletion
	ticks                                                                                                                                      chan time.Time
	starts, stops, appends, finalizations, receipts, waits, cleanups, releases, abandoned, portReleases, portAbandoned, closed, observedOwners int
	outcome                                                                                                                                    evidence.Outcome
	completion                                                                                                                                 application.ChildCompletion
	config                                                                                                                                     application.RunConfig
	validateFailure                                                                                                                            *status.Failure
	beginError, releaseError, abandonError, portBeginError, portReleaseError, portAbandonError                                                 error
	openError, finalizeError, cleanupFirstError, cleanupLastError, receiptError, waitError, snapshotError, peakError                           error
	startError, stopError, activationError, selectionError, victimError, presentError                                                          error
	appendErrorAt, collectErrorAt                                                                                                              int
	appendError, collectError                                                                                                                  error
	refused                                                                                                                                    bool
	inherited, startHasWorkload, selected, victimSelected, victimPresent                                                                       bool
	shedCause                                                                                                                                  coordination.ShedCause
	victim                                                                                                                                     coordination.ReservationOwner
	initialDone                                                                                                                                bool
	begin                                                                                                                                      func(context.Context, application.RunConfig) (application.Admission, error)
	waitHook                                                                                                                                   func(context.Context)
	startHook                                                                                                                                  func()
	collectHook                                                                                                                                func(int, context.Context, *policy.Reading)
	peakHook                                                                                                                                   func(context.Context) (int, error)
	selectionHook                                                                                                                              func() (bool, coordination.ShedCause, error)
	victimHook                                                                                                                                 func() (coordination.ReservationOwner, bool, error)
	presentHook                                                                                                                                func(context.Context) (bool, error)
	collectCalls                                                                                                                               int
}

func newRunContractFixture() *runContractFixture {
	return &runContractFixture{now: time.Unix(1, 0).UTC(), lease: application.Lease{Token: "owned", Allocation: coordination.ReservationVector{CPU: 2, MemoryBytes: policy.GiB}}, done: make(chan application.ChildCompletion, 1), ticks: make(chan time.Time, 32), initialDone: true, completion: application.ChildCompletion{ExitCode: 0}, shedCause: coordination.ShedCausePressure}
}

func (fixture *runContractFixture) record(event string) { fixture.trace = append(fixture.trace, event) }

func (fixture *runContractFixture) services() application.RunServices {
	return application.RunServices{Coordination: fixture, Workload: fixture, Ports: fixture, Evidence: fixture, Clock: fixture}
}

func (fixture *runContractFixture) settings() application.RunConfig {
	settings := policy.DefaultPolicy()
	settings.ConsecutiveCPUSamples = 1
	settings.AdmissionWindow = time.Minute
	settings.LeaseWait = time.Second
	return application.RunConfig{Command: "payload", Collector: fixture, Policy: settings, Stderr: &fixture.stderr, Now: fixture.Now, Resolution: policy.Resolution{RequestedProfile: "balanced", ResolvedProfile: "balanced", Concurrency: 2, Lineage: policy.LineageBalanced}, TaskClass: policy.TaskEphemeral}
}

func (fixture *runContractFixture) Normalize(config application.RunConfig) application.RunConfig {
	fixture.record("normalize")
	return config
}

func (fixture *runContractFixture) ValidateCommand(string) *status.Failure {
	fixture.record("validate")
	return fixture.validateFailure
}

func (fixture *runContractFixture) BeginAdmission(ctx context.Context, config application.RunConfig) (application.Admission, error) {
	fixture.record("begin-admission")
	if fixture.begin != nil {
		return fixture.begin(ctx, config)
	}
	fixture.lease.Inherited = fixture.inherited
	return application.Admission{Admitted: &fixture.lease, Close: func() { fixture.closed++; fixture.record("close-admission") }}, fixture.beginError
}

func (fixture *runContractFixture) Release(application.RunConfig, *application.Lease) error {
	fixture.releases++
	fixture.record("release")
	return fixture.releaseError
}

func (fixture *runContractFixture) Abandon(*application.Lease) error {
	fixture.abandoned++
	fixture.record("abandon")
	return fixture.abandonError
}
func (fixture *runContractFixture) DescribeHeavy(string) string { return "running owner" }
func (fixture *runContractFixture) Snapshot(context.Context, string, string) (coordination.ReservationTotals, error) {
	fixture.record("snapshot")
	return coordination.ReservationTotals{ActiveOwners: 1}, fixture.snapshotError
}

func (fixture *runContractFixture) OwnerPeak(ctx context.Context, _ string, _ *application.Lease) (int, error) {
	fixture.record("peak")
	if fixture.peakHook != nil {
		return fixture.peakHook(ctx)
	}
	return 2, fixture.peakError
}

func (fixture *runContractFixture) SheddingSelection(string, *application.Lease) (bool, coordination.ShedCause, error) {
	fixture.record("selection")
	if fixture.selectionHook != nil {
		return fixture.selectionHook()
	}
	return fixture.selected, fixture.shedCause, fixture.selectionError
}

func (fixture *runContractFixture) SelectVictim(_ string, _ coordination.ShedCause, emergency bool) (coordination.ReservationOwner, bool, error) {
	fixture.record("victim")
	if emergency {
		fixture.record("emergency-victim")
	}
	if fixture.victimHook != nil {
		return fixture.victimHook()
	}
	return fixture.victim, fixture.victimSelected, fixture.victimError
}

func (fixture *runContractFixture) VictimPresent(ctx context.Context, _ string, _ coordination.ReservationOwner) (bool, error) {
	fixture.record("present")
	if fixture.presentHook != nil {
		return fixture.presentHook(ctx)
	}
	return fixture.victimPresent, fixture.presentError
}

func (fixture *runContractFixture) Start(_ context.Context, config application.RunConfig, _ *application.Lease, _ *application.PortLease) (*application.Workload, error) {
	fixture.starts++
	fixture.config = config
	fixture.record("start")
	if fixture.startHook != nil {
		fixture.startHook()
	}
	if fixture.startError != nil && !fixture.startHasWorkload {
		return nil, fixture.startError
	}
	if fixture.initialDone {
		fixture.done <- fixture.completion
	}
	return &application.Workload{ID: "child", Done: fixture.done}, fixture.startError
}

func (fixture *runContractFixture) Stop(application.RunConfig, *application.Workload) (application.ChildCompletion, error) {
	fixture.stops++
	fixture.record("stop")
	return application.ChildCompletion{ExitCode: 137, Failed: true}, fixture.stopError
}

func (fixture *runContractFixture) Activate(application.RunConfig, *application.Lease, *application.Workload) error {
	fixture.record("activate")
	return fixture.activationError
}

func (fixture *runContractFixture) BeginPort(application.RunConfig) (application.PortAdmission, error) {
	fixture.record("begin-port")
	return application.PortAdmission{Attempt: func() (application.PortResult, error) {
		fixture.record("port-attempt")
		return application.PortResult{Lease: &application.PortLease{ID: "port"}}, nil
	}}, fixture.portBeginError
}

func (fixture *runContractFixture) ReleasePort(application.RunConfig, *application.PortLease) error {
	fixture.portReleases++
	fixture.record("release-port")
	return fixture.portReleaseError
}

func (fixture *runContractFixture) AbandonPort(*application.PortLease) error {
	fixture.portAbandoned++
	fixture.record("abandon-port")
	return fixture.portAbandonError
}
func (fixture *runContractFixture) Now() time.Time { return fixture.now }
func (fixture *runContractFixture) Wait(ctx context.Context, duration time.Duration, _ func(time.Duration)) error {
	fixture.waits++
	fixture.record("wait")
	fixture.now = fixture.now.Add(duration)
	if fixture.waitHook != nil {
		fixture.waitHook(ctx)
	}
	return fixture.waitError
}

func (fixture *runContractFixture) Ticker(time.Duration) application.RunTicker {
	fixture.record("ticker")
	return application.RunTicker{Ticks: fixture.ticks, Stop: func() { fixture.record("stop-ticker") }}
}

func (fixture *runContractFixture) Cleanup(string, time.Time) error {
	fixture.cleanups++
	fixture.record("cleanup")
	if fixture.cleanups == 1 {
		return fixture.cleanupFirstError
	}
	return fixture.cleanupLastError
}

func (fixture *runContractFixture) Open(application.RunConfig) (application.RunSink, error) {
	fixture.record("open")
	return application.RunSink{RunID: "run", SetIdentity: func(coordination.ReservationMetadata) { fixture.record("identity") }, SetContext: func(policy.Resolution, string) { fixture.record("context") }, SetReservationContext: func(*application.Lease, int, evidence.BudgetOutcome) { fixture.record("reservation-context") }, ObserveReservationOwners: func(int) { fixture.observedOwners++; fixture.record("observe-owners") }, Append: func(policy.Sample) error {
		fixture.appends++
		fixture.record("append")
		if fixture.appends == fixture.appendErrorAt {
			return fixture.appendError
		}
		return nil
	}, Finalize: func(_ policy.TaskClass, outcome evidence.Outcome, _ int) error {
		fixture.finalizations++
		fixture.outcome = outcome
		fixture.record("finalize")
		return fixture.finalizeError
	}}, fixture.openError
}

func (fixture *runContractFixture) Receipt(_, _, state, reason string, _ coordination.ReservationMetadata, _ policy.TaskClass, _ time.Time) error {
	fixture.receipts++
	fixture.receiptStates = append(fixture.receiptStates, state)
	fixture.receiptReasons = append(fixture.receiptReasons, reason)
	fixture.record("receipt")
	return fixture.receiptError
}

func (fixture *runContractFixture) WriteRefused(err error) bool { return err != nil && fixture.refused }

func (fixture *runContractFixture) Collect(ctx context.Context, _ policy.CPUState, _ string) (policy.Reading, error) {
	fixture.collectCalls++
	fixture.record("collect")
	sample := policy.Sample{SchemaVersion: 3, MeasuredAt: fixture.now.Format(time.RFC3339Nano), Platform: "darwin", Capabilities: []string{"compressor", "memory-pressure", "swap"}, AvailableMemoryBytes: new(12 * policy.GiB), MemoryPressureLevel: new(1), CompressorAvailable: new(true), CompressorPayloadBytes: new(7 * policy.GiB), PhysicalMemoryBytes: 32 * policy.GiB, AvailableParallelism: 8, CPUUtilizationPercent: new(20.0), DiskFreeBytes: new(40 * policy.GiB), SwapIns: new(int64(10)), SwapOuts: new(int64(20))}
	reading := policy.Reading{Sample: sample}
	if fixture.collectHook != nil {
		fixture.collectHook(fixture.collectCalls, ctx, &reading)
	}
	if fixture.collectCalls == fixture.collectErrorAt {
		return reading, fixture.collectError
	}
	return reading, nil
}

var errRunContract = errors.New("injected port failure")
