package application_test

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
)

type observationContractHistory struct {
	rows    []evidence.Summary
	query   evidence.Query
	root    string
	failure error
}

func (history *observationContractHistory) History(root string, query evidence.Query) ([]evidence.Summary, error) {
	history.root, history.query = root, query
	return history.rows, history.failure
}

type observationContractCollector struct {
	fixture *observationFixture
	failAt  int
	failure error
	cancel  context.CancelFunc
}

func (collector *observationContractCollector) Collect(ctx context.Context, previous policy.CPUState, path string) (policy.Reading, error) {
	if collector.fixture.samples+1 == collector.failAt {
		if collector.cancel != nil {
			collector.cancel()
		}
		return policy.Reading{}, collector.failure
	}
	return collector.fixture.Collect(ctx, previous, path)
}

func requireContractFailure(t *testing.T, err error, code status.Code) {
	t.Helper()
	failure, ok := errors.AsType[status.Failure](err)
	if !ok || failure.Code != code {
		t.Fatalf("failure=%v, want code=%v", err, code)
	}
}

func TestObservationContractHistoryKeepsRowsAndPassesCompleteQuery(t *testing.T) {
	fixture := newObservationFixture()
	rows := []evidence.Summary{{RunID: "retained", Source: "hippo"}}
	history := &observationContractHistory{rows: rows}
	services := observationServices(fixture)
	services.Evidence = history
	request := application.HistoryFlags{Since: "2d", Source: "hippo", Tags: []string{"role=build"}, ClassFlag: "ephemeral", Tier: "light", OutcomeFlag: "passed"}
	result, code, err := services.History(request)
	if err != nil || code != 0 || !reflect.DeepEqual(result.Rows, rows) || result.Since != "2d" {
		t.Fatalf("result=%+v code=%d error=%v", result, code, err)
	}
	want := evidence.Query{Since: 48 * time.Hour, Now: fixture.now, Source: "hippo", Tags: map[string]string{"role": "build"}, Class: policy.TaskEphemeral, Tier: "light", Outcome: evidence.OutcomePassed}
	if history.root != "shared" || !reflect.DeepEqual(history.query, want) {
		t.Fatalf("root=%q query=%+v, want %+v", history.root, history.query, want)
	}
	history.failure = io.ErrUnexpectedEOF
	result, code, err = services.History(request)
	requireContractFailure(t, err, status.CodeEvidenceUnreadable)
	if code != 0 || len(result.Rows) != 0 {
		t.Fatalf("unreadable history returned rows=%v code=%d", result.Rows, code)
	}
}

func TestObservationContractInvalidSourcePrecedesStorageAndOtherFilters(t *testing.T) {
	fixture := newObservationFixture()
	services := observationServices(fixture)
	_, _, err := services.Observe(context.Background(), application.StatusRequest{Source: "bad source"})
	requireContractFailure(t, err, status.CodeArgsInvalid)
	_, _, err = services.History(application.HistoryFlags{Since: "bad", Source: "bad source", ClassFlag: "future"})
	requireContractFailure(t, err, status.CodeArgsInvalid)
	if !strings.Contains(err.Error(), "--source") || len(fixture.calls) != 0 {
		t.Fatalf("error precedence=%v calls=%v", err, fixture.calls)
	}
	if _, err := application.ParseRollingDuration("invalidd"); err == nil {
		t.Fatal("invalid whole-day duration accepted")
	}
}

func TestObservationContractRowFiltersRetainGlobalCapacity(t *testing.T) {
	included := coordination.ReservationEntry{RunID: "matching", Source: "hippo", Tags: map[string]string{"role": "build", "extra": "yes"}}
	excluded := coordination.ReservationEntry{RunID: "wrong-tag", Source: "hippo", Tags: map[string]string{"role": "test"}}
	totals := coordination.ReservationTotals{ActiveOwners: 7, WaitingOwners: 9, Owners: []coordination.ReservationEntry{included, excluded}, Waiters: []coordination.ReservationEntry{excluded, included}}
	if actual := application.FilterCoordinationRows(totals, "", nil); !reflect.DeepEqual(actual, totals) {
		t.Fatalf("unfiltered totals changed: %+v", actual)
	}
	actual := application.FilterCoordinationRows(totals, "hippo", map[string]string{"role": "build"})
	if actual.ActiveOwners != 7 || actual.WaitingOwners != 9 || !reflect.DeepEqual(actual.Owners, []coordination.ReservationEntry{included}) || !reflect.DeepEqual(actual.Waiters, []coordination.ReservationEntry{included}) {
		t.Fatalf("filtered capacity=%+v", actual)
	}
	if len(totals.Owners) != 2 || len(totals.Waiters) != 2 {
		t.Fatal("filter mutated original rows")
	}
}

func TestObservationContractAssessmentDecisionPreservesOrClassifiesPaths(t *testing.T) {
	original := policy.Resolution{ResolvedProfile: "balanced", Decision: policy.DecisionRun, Reason: policy.ReasonNone}
	cases := []struct {
		name      string
		path      policy.AdmissionPath
		decision  policy.Decision
		reason    policy.Reason
		retryable bool
	}{
		{"normal", policy.AdmissionNormal, policy.DecisionRun, policy.ReasonNone, false},
		{"replan", policy.AdmissionReplan, policy.DecisionRun, policy.ReasonNone, false},
		{"warning", policy.AdmissionWait, policy.DecisionWait, policy.ReasonCapacityDeferred, true},
		{"degraded", policy.AdmissionDegraded, policy.DecisionWait, policy.ReasonCapacityDeferred, true},
		{"disk", policy.AdmissionCleanup, policy.DecisionCleanup, policy.ReasonStorageBlocked, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			actual, err := application.WithAssessmentDecision(original, test.path)
			if err != nil || actual.Decision != test.decision || actual.Reason != test.reason || actual.Retryable != test.retryable || actual.ResolvedProfile != original.ResolvedProfile {
				t.Fatalf("decision=%+v error=%v", actual, err)
			}
		})
	}
	actual, err := application.WithAssessmentDecision(original, policy.AdmissionUnset)
	if err == nil || !reflect.DeepEqual(actual, original) {
		t.Fatalf("unset path changed resolution=%+v error=%v", actual, err)
	}
}

func TestObservationContractPromotionUsesHealthyEvidenceButLivePressureWins(t *testing.T) {
	memory := 20 * policy.GiB
	history := &observationContractHistory{rows: []evidence.Summary{{Source: "hippo", PeakOwnerCount: 2, Outcome: evidence.Recorded(evidence.OutcomePassed), AvailableNonCompressedEstimateMinBytes: &memory}}}
	services := application.ObservationServices{Evidence: history}
	config := coordination.Configuration{SchemaVersion: 3, BaseActiveOwners: 1, MaxActiveOwners: 2}
	config.Promotion.CompletedRuns = 1
	config.Promotion.MinimumSources = 1
	config.Promotion.MinimumAvailableMemoryBytes = 10 * policy.GiB
	config.Promotion.MaximumCPUP95Percent = 50
	now := time.Unix(100, 0)
	actual, err := services.EvaluatePromotion("shared", config, policy.Assessment{State: policy.StateNormal}, now)
	if err != nil || !actual.Eligible || actual.EffectiveOwners != 2 || history.query.Since != evidence.HistoryRetention || !history.query.Now.Equal(now) {
		t.Fatalf("healthy promotion=%+v query=%+v error=%v", actual, history.query, err)
	}
	actual, err = services.EvaluatePromotion("shared", config, policy.Assessment{State: policy.StateWarning}, now)
	if err != nil || !actual.Eligible || actual.EffectiveOwners != 1 || actual.Reason != "live-pressure-warning" {
		t.Fatalf("pressure promotion=%+v error=%v", actual, err)
	}
	config.SchemaVersion = 2
	services.Evidence = nil
	actual, err = services.EvaluatePromotion("shared", config, policy.Assessment{State: policy.StateNormal}, now)
	if err != nil || actual.EffectiveOwners != 2 || actual.Reason != "not-configured" {
		t.Fatalf("legacy promotion=%+v error=%v", actual, err)
	}
}

func TestObservationContractSecondProbeFailureAndUnknownProfileStopBeforeSnapshot(t *testing.T) {
	fixture := newObservationFixture()
	services := observationServices(fixture)
	services.Collector = &observationContractCollector{fixture: fixture, failAt: 2, failure: io.ErrUnexpectedEOF}
	_, code, err := services.Observe(context.Background(), application.StatusRequest{})
	if code != 1 || !errors.Is(err, io.ErrUnexpectedEOF) || !reflect.DeepEqual(fixture.calls, []string{"load", "collect", "wait"}) {
		t.Fatalf("probe error=%v code=%d calls=%v", err, code, fixture.calls)
	}
	fixture = newObservationFixture()
	_, code, err = observationServices(fixture).Observe(context.Background(), application.StatusRequest{RequestedProfile: "missing"})
	stop, ok := errors.AsType[*policy.Stop](err)
	if code != 0 || !ok || stop.Reason != policy.ReasonReplanRequired || !reflect.DeepEqual(fixture.calls, []string{"load", "collect", "wait", "collect"}) {
		t.Fatalf("profile error=%v code=%d calls=%v", err, code, fixture.calls)
	}
}

func TestObservationContractWatchStopsAtProjectionOutputAndWaitFailures(t *testing.T) {
	cases := []struct {
		name                      string
		projectError, outputError bool
		phase                     string
	}{
		{name: "projection", projectError: true}, {name: "output", outputError: true}, {name: "interval wait", phase: "wait"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newObservationFixture()
			// The second wait is between watch frames; the first belongs to status.
			services := observationServices(fixture)
			if test.phase != "" {
				services.Clock = &observationContractClock{fixture: fixture, failWait: 2}
			}
			emitted := 0
			code, err := services.Watch(context.Background(), application.StatusRequest{}, time.Second, func(application.StatusView) ([]byte, error) {
				if test.projectError {
					return nil, io.ErrClosedPipe
				}
				return []byte("frame"), nil
			}, func([]byte) error {
				emitted++
				if test.outputError {
					return io.ErrClosedPipe
				}
				return nil
			})
			if code != 1 || !errors.Is(err, io.ErrClosedPipe) {
				t.Fatalf("code=%d error=%v", code, err)
			}
			wantEmitted := 1
			if test.projectError {
				wantEmitted = 0
			}
			if emitted != wantEmitted || fixture.samples != 2 {
				t.Fatalf("emitted=%d samples=%d", emitted, fixture.samples)
			}
		})
	}
}

type observationContractClock struct {
	fixture  *observationFixture
	failWait int
}

func (clock *observationContractClock) Wait(ctx context.Context, duration time.Duration) error {
	if clock.fixture.waits+1 == clock.failWait {
		return io.ErrClosedPipe
	}
	return clock.fixture.Wait(ctx, duration)
}
func (clock *observationContractClock) Now() time.Time { return clock.fixture.Now() }
func (clock *observationContractClock) Ticker(interval time.Duration) application.ReleaseTicker {
	return clock.fixture.Ticker(interval)
}

func TestObservationContractWatchValidationAndCallerCancellation(t *testing.T) {
	fixture := newObservationFixture()
	services := observationServices(fixture)
	project := func(application.StatusView) ([]byte, error) { t.Error("projection should not run"); return nil, nil }
	emit := func([]byte) error { t.Error("output should not run"); return nil }
	_, err := services.Watch(context.Background(), application.StatusRequest{}, 0, project, emit)
	requireContractFailure(t, err, status.CodeArgsInvalid)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = services.Watch(ctx, application.StatusRequest{Source: "bad source"}, time.Second, project, emit)
	requireContractFailure(t, err, status.CodeArgsInvalid)
	code, err := services.Watch(ctx, application.StatusRequest{}, time.Second, project, emit)
	if code != 0 || err != nil {
		t.Fatalf("caller stop code=%d error=%v", code, err)
	}
}

func TestObservationContractMonitorRefusalsAndInitialFailures(t *testing.T) {
	cases := []struct {
		name     string
		interval time.Duration
		phase    string
		profile  policy.ProfileName
		typed    status.Code
	}{
		{name: "interval", typed: status.CodeArgsInvalid},
		{name: "configuration", interval: time.Second, phase: "load", typed: status.CodeConfigUnreadable},
		{name: "collector", interval: time.Second, phase: "collect"},
		{name: "profile", interval: time.Second, profile: "missing"},
		{name: "output", interval: time.Second},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newObservationFixture()
			fixture.failureAt, fixture.failure = test.phase, io.ErrClosedPipe
			code, err := observationServices(fixture).Monitor(context.Background(), application.MonitorRequest{Interval: test.interval, RequestedProfile: test.profile}, func(application.MonitorTransition) error { return io.ErrClosedPipe })
			if test.typed != "" {
				requireContractFailure(t, err, test.typed)
				if code != 0 {
					t.Fatalf("typed refusal code=%d", code)
				}
			} else if code != 1 || err == nil {
				t.Fatalf("code=%d error=%v", code, err)
			}
			if fixture.stopped {
				t.Fatal("ticker started before initial observation completed")
			}
		})
	}
}

func TestObservationContractMonitorStopsTickerOnProbeFailureOrCancellation(t *testing.T) {
	for _, cancelProbe := range []bool{false, true} {
		t.Run(map[bool]string{false: "failure", true: "caller cancellation"}[cancelProbe], func(t *testing.T) {
			fixture := newObservationFixture()
			fixture.ticks <- fixture.now
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			collector := &observationContractCollector{fixture: fixture, failAt: 2, failure: io.ErrClosedPipe}
			if cancelProbe {
				collector.cancel = cancel
			}
			services := observationServices(fixture)
			services.Collector = collector
			emitted := 0
			code, err := services.Monitor(ctx, application.MonitorRequest{Interval: time.Second}, func(application.MonitorTransition) error { emitted++; return nil })
			if cancelProbe {
				if code != 0 || err != nil {
					t.Fatalf("cancel code=%d error=%v", code, err)
				}
			} else if code != 1 || !errors.Is(err, io.ErrClosedPipe) {
				t.Fatalf("failure code=%d error=%v", code, err)
			}
			if emitted != 1 || !fixture.stopped {
				t.Fatalf("emitted=%d stopped=%t", emitted, fixture.stopped)
			}
		})
	}
}

func TestObservationContractMonitorLongStableWindowHasOneTransition(t *testing.T) {
	fixture := newObservationFixture()
	fixture.ticks = make(chan time.Time, 20)
	for range 20 {
		fixture.ticks <- fixture.now
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	services := observationServices(fixture)
	services.Collector = &observationContractCollector{fixture: fixture, failAt: 21, failure: context.Canceled, cancel: cancel}
	emitted := 0
	code, err := services.Monitor(ctx, application.MonitorRequest{Interval: time.Second}, func(application.MonitorTransition) error { emitted++; return nil })
	if code != 0 || err != nil || emitted != 1 || fixture.samples != 20 || !fixture.stopped {
		t.Fatalf("code=%d error=%v emitted=%d samples=%d stopped=%t", code, err, emitted, fixture.samples, fixture.stopped)
	}
}

func TestObservationContractMonitorCancelledInitialProbeDoesNotStartTicker(t *testing.T) {
	fixture := newObservationFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	services := observationServices(fixture)
	services.Collector = &observationContractCollector{fixture: fixture, failAt: 1, failure: context.Canceled, cancel: cancel}
	code, err := services.Monitor(ctx, application.MonitorRequest{Interval: time.Second}, func(application.MonitorTransition) error { t.Error("cancelled probe emitted a transition"); return nil })
	if code != 0 || err != nil || fixture.stopped {
		t.Fatalf("initial cancellation code=%d error=%v ticker started=%t", code, err, fixture.stopped)
	}
}

func TestObservationContractMonitorMissingEvidenceReportsCriticalTransition(t *testing.T) {
	fixture := newObservationFixture()
	fixture.sample = policy.Sample{MeasuredAt: fixture.now.Format(time.RFC3339Nano)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	emissions := 0
	code, err := observationServices(fixture).Monitor(ctx, application.MonitorRequest{Interval: time.Second}, func(transition application.MonitorTransition) error {
		emissions++
		if transition.State != policy.StateCritical || transition.Reason != "disk-unavailable" || transition.SchemaVersion != 1 || transition.MeasuredAt != fixture.sample.MeasuredAt {
			t.Errorf("missing evidence transition=%+v", transition)
		}
		cancel()
		return nil
	})
	if code != 0 || err != nil || emissions != 1 || fixture.samples != 1 || !fixture.stopped {
		t.Fatalf("code=%d error=%v emissions=%d samples=%d stopped=%t", code, err, emissions, fixture.samples, fixture.stopped)
	}
}

func TestObservationContractMonitorPendingTickCannotCollectAfterCallerStop(t *testing.T) {
	fixture := newObservationFixture()
	fixture.ticks <- fixture.now
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	code, err := observationServices(fixture).Monitor(ctx, application.MonitorRequest{Interval: time.Second}, func(application.MonitorTransition) error { cancel(); return nil })
	if code != 0 || err != nil || fixture.samples != 1 || !fixture.stopped {
		t.Fatalf("code=%d error=%v samples=%d stopped=%t", code, err, fixture.samples, fixture.stopped)
	}
}
