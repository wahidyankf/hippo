package support

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync/atomic"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	evidencedomain "github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

// annotationMemoryRun doubles outward capabilities. Application Run and the
// ordinary production DisplayAdvisories remain the subjects of the Unit run.
// No native path, process or filesystem is used by this boundary.
type annotationMemoryRun struct {
	advisories                                                      application.DisplayAdvisories
	operation                                                       string
	annotation, owned, inherited, missingRemoval, atomic            bool
	writes, removals, failures, starts, releases, finalized, closed int
	observed, exit                                                  int
	err                                                             error
	stderr                                                          bytes.Buffer
	writer                                                          io.Writer
	config, started                                                 application.RunConfig
	lease                                                           application.Lease
	outcome                                                         evidencedomain.Outcome
	trace                                                           []string
	now                                                             time.Time
	heldDone                                                        chan application.ChildCompletion
	heldStarted                                                     chan struct{}
	heldStops, heldReleases, heldFinalized                          atomic.Int32
}

func newAnnotationMemoryRun(operation string) *annotationMemoryRun {
	fixture := &annotationMemoryRun{operation: operation, observed: -1, now: time.Unix(1, 0).UTC()}
	fixture.writer = &fixture.stderr
	fixture.lease = application.Lease{Token: "memory-owned", Allocation: coordination.ReservationVector{CPU: 2, MemoryBytes: policy.GiB}}
	return fixture
}

func (fixture *annotationMemoryRun) record(event string) {
	fixture.trace = append(fixture.trace, event)
}

func (fixture *annotationMemoryRun) settings(reservation bool) application.RunConfig {
	settings := policy.DefaultPolicy()
	settings.ConsecutiveCPUSamples = 1
	label, _ := identity.ParseDisplayLabel(annotationOwnerLabel)
	return application.RunConfig{
		Command: memoryChildWorkload, Arguments: []string{"private-argument"}, Label: label,
		TaskClass: policy.TaskEphemeral, Collector: fixture, Policy: settings, Stderr: fixture.writer,
		Resolution:         policy.Resolution{RequestedProfile: "balanced", ResolvedProfile: "balanced", Concurrency: 2, Lineage: policy.LineageBalanced},
		ReservationPolicy:  coordination.ReservationPolicy{Enabled: reservation},
		ObserveChildStatus: func(code int) { fixture.observed = code }, Now: fixture.Now,
	}
}

func (fixture *annotationMemoryRun) run(ctx context.Context, reservation bool) {
	services := application.RunServices{Coordination: fixture, Workload: fixture, Ports: fixture, Evidence: fixture, Clock: fixture}
	fixture.exit, fixture.err = services.Run(ctx, fixture.settings(reservation))
}

func (fixture *annotationMemoryRun) mutate(publication bool) error {
	if publication {
		fixture.writes++
		fixture.record(annotationPublishEvent)
		if fixture.operation == annotationOperationWrite || fixture.operation == annotationOperationBoth {
			fixture.failures++
			return errors.New("/private/annotation-write raw-error private-owner-label private-argument\x1b[31m")
		}
		fixture.annotation = true
		return nil
	}
	fixture.removals++
	fixture.record("remove")
	if fixture.missingRemoval {
		fixture.annotation = false
	}
	if !fixture.annotation {
		return nil
	}
	if fixture.operation == annotationOperationRemoval || fixture.operation == annotationOperationBoth {
		fixture.failures++
		return errors.New("/private/annotation-removal raw-error private-owner-label private-argument\x1b[31m")
	}
	fixture.annotation = false
	return nil
}

func (fixture *annotationMemoryRun) BeginAdmission(_ context.Context, config application.RunConfig) (application.Admission, error) {
	fixture.config = config
	fixture.lease.Inherited = fixture.inherited
	fixture.record("begin")
	return application.Admission{Attempt: func(context.Context, time.Duration, time.Time) (application.AdmissionResult, error) {
		fixture.atomic = true
		if !fixture.inherited {
			fixture.advisories.RecordPublication(fixture.mutate(true))
			fixture.owned = true
		}
		fixture.atomic = false
		fixture.advisories.Report(config.Stderr)
		return application.AdmissionResult{Lease: &fixture.lease}, nil
	}, Close: func() { fixture.closed++; fixture.record("close") }}, nil
}

func (fixture *annotationMemoryRun) Release(config application.RunConfig, _ *application.Lease) error {
	fixture.heldReleases.Add(1)
	fixture.releases++
	fixture.record(taskClassRelease)
	fixture.atomic = true
	if !fixture.inherited {
		fixture.owned = false
		fixture.advisories.RecordRemoval(fixture.mutate(false))
	}
	fixture.atomic = false
	fixture.advisories.Report(config.Stderr)
	return nil
}

func (fixture *annotationMemoryRun) Abandon(*application.Lease) error {
	return errors.New("unexpected memory abandon")
}
func (fixture *annotationMemoryRun) DescribeHeavy(string) string { return "memory owner" }
func (fixture *annotationMemoryRun) Snapshot(context.Context, string, string) (coordination.ReservationTotals, error) {
	return coordination.ReservationTotals{ActiveOwners: 1, Allocated: fixture.lease.Allocation}, nil
}

func (fixture *annotationMemoryRun) OwnerPeak(context.Context, string, *application.Lease) (int, error) {
	return 1, nil
}

func (fixture *annotationMemoryRun) SheddingSelection(string, *application.Lease) (bool, coordination.ShedCause, error) {
	return false, coordination.ShedCausePressure, nil
}

func (fixture *annotationMemoryRun) SelectVictim(string, coordination.ShedCause, bool) (coordination.ReservationOwner, bool, error) {
	return coordination.ReservationOwner{}, false, nil
}

func (fixture *annotationMemoryRun) VictimPresent(context.Context, string, coordination.ReservationOwner) (bool, error) {
	return false, nil
}

func (fixture *annotationMemoryRun) Normalize(config application.RunConfig) application.RunConfig {
	return config
}
func (fixture *annotationMemoryRun) ValidateCommand(string) *status.Failure { return nil }
func (fixture *annotationMemoryRun) Start(_ context.Context, config application.RunConfig, _ *application.Lease, _ *application.PortLease) (*application.Workload, error) {
	fixture.starts++
	fixture.started = config
	fixture.record(startPhase)
	done := make(chan application.ChildCompletion, 1)
	if fixture.heldDone != nil {
		return &application.Workload{ID: memoryChildWorkload, Done: fixture.heldDone}, nil
	}
	done <- application.ChildCompletion{ExitCode: 37, Failed: true}
	return &application.Workload{ID: memoryChildWorkload, Done: done}, nil
}

func (fixture *annotationMemoryRun) Stop(application.RunConfig, *application.Workload) (application.ChildCompletion, error) {
	fixture.heldStops.Add(1)
	return application.ChildCompletion{}, errors.New("unexpected memory stop")
}

func (fixture *annotationMemoryRun) Activate(application.RunConfig, *application.Lease, *application.Workload) error {
	fixture.record("activate")
	if fixture.heldStarted != nil {
		close(fixture.heldStarted)
	}
	return nil
}

func (fixture *annotationMemoryRun) BeginPort(application.RunConfig) (application.PortAdmission, error) {
	return application.PortAdmission{}, errors.New("unexpected memory port")
}

func (fixture *annotationMemoryRun) ReleasePort(application.RunConfig, *application.PortLease) error {
	return errors.New("unexpected memory port release")
}

func (fixture *annotationMemoryRun) AbandonPort(*application.PortLease) error {
	return errors.New("unexpected memory port abandon")
}
func (fixture *annotationMemoryRun) Now() time.Time { return fixture.now }
func (fixture *annotationMemoryRun) Wait(_ context.Context, duration time.Duration, _ func(time.Duration)) error {
	fixture.now = fixture.now.Add(duration)
	return nil
}

func (fixture *annotationMemoryRun) Ticker(time.Duration) application.RunTicker {
	return application.RunTicker{Ticks: make(chan time.Time), Stop: func() {}}
}
func (fixture *annotationMemoryRun) Cleanup(string, time.Time) error { return nil }
func (fixture *annotationMemoryRun) Open(application.RunConfig) (application.RunSink, error) {
	return application.RunSink{
		RunID: "memory-run", SetIdentity: func(coordination.ReservationMetadata) {},
		SetContext: func(policy.Resolution, string) {}, SetReservationContext: func(*application.Lease, int, evidencedomain.BudgetOutcome) {},
		SetChildUsage:            func(time.Duration, time.Duration, bool) {},
		ObserveReservationOwners: func(int) {}, Append: func(policy.Sample) error { return nil },
		Finalize: func(_ policy.TaskClass, outcome evidencedomain.Outcome, _ int) error {
			fixture.heldFinalized.Add(1)
			fixture.finalized++
			fixture.outcome = outcome
			fixture.record("finalize")
			return nil
		},
	}, nil
}

func (fixture *annotationMemoryRun) Receipt(string, string, string, string, coordination.ReservationMetadata, policy.TaskClass, time.Time) error {
	return nil
}
func (fixture *annotationMemoryRun) WriteRefused(error) bool { return false }
func (fixture *annotationMemoryRun) Collect(context.Context, policy.CPUState, string) (policy.Reading, error) {
	return policy.Reading{Sample: healthySample(fixture.now)}, nil
}
