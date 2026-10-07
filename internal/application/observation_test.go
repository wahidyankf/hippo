package application_test

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/status"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/domain/evidence"
	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/internal/policy"
)

type observationFixture struct {
	calls          []string
	sample         policy.Sample
	ticks          chan time.Time
	now            time.Time
	waits          int
	cancel         context.CancelFunc
	cancelAt       int
	sampleSequence []policy.Sample
	samples        int
	stopped        bool
	failureAt      string
	failure        error
}

func (fixture *observationFixture) Load(string, map[string]string) (application.Configuration, error) {
	fixture.calls = append(fixture.calls, "load")
	if fixture.failureAt == "load" {
		return application.Configuration{}, fixture.failure
	}
	return application.Configuration{Catalog: policy.BuiltinCatalog(), Coordination: coordination.Configuration{SchemaVersion: 3, Mode: "reservation", BaseActiveOwners: 1, MaxActiveOwners: 2}, Hash: "config-hash"}, nil
}

func (*observationFixture) Identity(map[string]string, string, string, []string) (identity.Value, string, error) {
	return identity.Value{}, "", nil
}

func (fixture *observationFixture) StateRoot(map[string]string) string {
	fixture.calls = append(fixture.calls, "root")
	return "shared"
}
func (*observationFixture) AbsolutePath(path string) (string, error) { return path, nil }
func (fixture *observationFixture) Collect(context.Context, policy.CPUState, string) (policy.Reading, error) {
	fixture.calls = append(fixture.calls, "collect")
	if fixture.failureAt == "collect" {
		return policy.Reading{}, fixture.failure
	}
	value := fixture.sample
	if len(fixture.sampleSequence) > 0 {
		value = fixture.sampleSequence[min(fixture.samples, len(fixture.sampleSequence)-1)]
	}
	fixture.samples++
	return policy.Reading{Sample: value, CPUState: policy.CPUState{1}}, nil
}

func (fixture *observationFixture) Snapshot(context.Context, string, string) (coordination.ReservationTotals, error) {
	fixture.calls = append(fixture.calls, "snapshot")
	if fixture.failureAt == "snapshot" {
		return coordination.ReservationTotals{}, fixture.failure
	}
	return coordination.ReservationTotals{SchemaVersion: 5, ActiveOwners: 2, WaitingOwners: 1, Owners: []coordination.ReservationEntry{{RunID: "included", Source: "hippo"}, {RunID: "excluded", Source: "rhino"}}}, nil
}

func (fixture *observationFixture) History(string, evidence.Query) ([]evidence.Summary, error) {
	fixture.calls = append(fixture.calls, "history")
	if fixture.failureAt == "history" {
		return nil, fixture.failure
	}
	return []evidence.Summary{}, nil
}
func (fixture *observationFixture) Now() time.Time { return fixture.now }
func (fixture *observationFixture) Wait(ctx context.Context, _ time.Duration) error {
	fixture.calls = append(fixture.calls, "wait")
	if fixture.failureAt == "wait" {
		return fixture.failure
	}
	fixture.waits++
	if fixture.cancel != nil && fixture.waits == fixture.cancelAt {
		fixture.cancel()
	}
	return ctx.Err()
}

func (fixture *observationFixture) Ticker(time.Duration) application.ReleaseTicker {
	return application.ReleaseTicker{Ticks: fixture.ticks, Stop: func() { fixture.stopped = true }}
}

func newObservationFixture() *observationFixture {
	available, disk, cpu, pressure := 20*policy.GiB, 100*policy.GiB, 20.0, 1
	now := time.Unix(100, 0)
	return &observationFixture{now: now, ticks: make(chan time.Time, 2), sample: policy.Sample{MeasuredAt: now.Format(time.RFC3339Nano), AvailableMemoryBytes: &available, AvailableParallelism: 8, DiskFreeBytes: &disk, CPUUtilizationPercent: &cpu, MemoryPressureLevel: &pressure, Platform: "darwin", Capabilities: []string{"memory-pressure"}}}
}

func observationServices(fixture *observationFixture) application.ObservationServices {
	return application.ObservationServices{Configuration: fixture, Collector: fixture, Runtime: fixture, Evidence: fixture, Clock: fixture}
}

func TestObservationOwnsTwoSamplesPromotionAndAtomicSnapshot(t *testing.T) {
	fixture := newObservationFixture()
	view, code, err := observationServices(fixture).Observe(context.Background(), application.StatusRequest{DiskPath: "disk", Source: "hippo"})
	if err != nil || code != 0 {
		t.Fatalf("code=%d error=%v", code, err)
	}
	want := []string{"load", "collect", "wait", "collect", "root", "history", "snapshot"}
	if !reflect.DeepEqual(fixture.calls, want) {
		t.Fatalf("observation calls=%v, want %v", fixture.calls, want)
	}
	if view.SchemaVersion != 5 || view.ConfigHash != "config-hash" || view.Coordination.ActiveOwners != 2 || len(view.Coordination.Owners) != 1 || view.Coordination.Owners[0].RunID != "included" {
		t.Fatalf("snapshot changed global totals or rowfilter: %+v", view)
	}
}

func TestObservationWatchOwnsRepeatedObservationAndChangeProjection(t *testing.T) {
	fixture := newObservationFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fixture.cancel = cancel
	fixture.cancelAt = 4
	projected, emitted := 0, 0
	code, err := observationServices(fixture).Watch(ctx, application.StatusRequest{Source: "hippo"}, time.Second, func(view application.StatusView) ([]byte, error) { projected++; return []byte(view.ConfigHash), nil }, func([]byte) error { emitted++; return nil })
	if err != nil || code != 0 {
		t.Fatalf("code=%d error=%v", code, err)
	}
	if projected != 2 || emitted != 1 || fixture.waits != 4 {
		t.Fatalf("projected=%d emitted=%d waits=%d, want2/1/4", projected, emitted, fixture.waits)
	}
}

func TestObservationHistoryOwnsQueryValidationAndEmptyAnswer(t *testing.T) {
	fixture := newObservationFixture()
	result, code, err := observationServices(fixture).History(application.HistoryFlags{Since: "30d", Source: "hippo", ClassFlag: "ephemeral", Tier: "standard", OutcomeFlag: "passed", Tags: []string{"role=test"}})
	if err != nil || code != 1 {
		t.Fatalf("empty history code=%d error=%v, want1", code, err)
	}
	if result.Since != "30d" || result.Rows == nil || !reflect.DeepEqual(fixture.calls, []string{"root", "history"}) {
		t.Fatalf("history result=%+v calls=%v", result, fixture.calls)
	}
}

func TestObservationMonitorOwnsTicksAndSuppressesUnchangedTransitions(t *testing.T) {
	fixture := newObservationFixture()
	warning := fixture.sample
	pressure := 2
	warning.MemoryPressureLevel = &pressure
	fixture.sampleSequence = []policy.Sample{fixture.sample, fixture.sample, warning}
	fixture.ticks <- fixture.now
	fixture.ticks <- fixture.now
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	states := []policy.State{}
	code, err := observationServices(fixture).Monitor(ctx, application.MonitorRequest{Interval: time.Second}, func(value application.MonitorTransition) error {
		states = append(states, value.State)
		if len(states) == 2 {
			cancel()
		}
		return nil
	})
	if err != nil || code != 0 {
		t.Fatalf("code=%d error=%v", code, err)
	}
	if !reflect.DeepEqual(states, []policy.State{policy.StateNormal, policy.StateWarning}) || fixture.samples != 3 || !fixture.stopped {
		t.Fatalf("states=%v samples=%d stopped=%t", states, fixture.samples, fixture.stopped)
	}
}

func TestObservationFailuresStopBeforeLaterCapabilities(t *testing.T) {
	cases := []struct {
		name  string
		code  int
		calls []string
	}{
		{"load", 0, []string{"load"}},
		{"collect", 1, []string{"load", "collect"}},
		{"wait", 1, []string{"load", "collect", "wait"}},
		{"history", 1, []string{"load", "collect", "wait", "collect", "root", "history"}},
		{"snapshot", 1, []string{"load", "collect", "wait", "collect", "root", "history", "snapshot"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newObservationFixture()
			fixture.failureAt = test.name
			fixture.failure = io.ErrUnexpectedEOF
			_, code, err := observationServices(fixture).Observe(context.Background(), application.StatusRequest{})
			if code != test.code || err == nil {
				t.Fatalf("code=%d error=%v", code, err)
			}
			if !reflect.DeepEqual(fixture.calls, test.calls) {
				t.Fatalf("capabilities=%v, want%v", fixture.calls, test.calls)
			}
		})
	}
}

func TestObservationRefusesInvalidFiltersBeforeAnyEffects(t *testing.T) {
	fixture := newObservationFixture()
	_, _, err := observationServices(fixture).Observe(context.Background(), application.StatusRequest{Tags: []string{"invalid"}})
	failure, classified := errors.AsType[status.Failure](err)
	if !classified || failure.Code != status.CodeArgsInvalid || len(fixture.calls) != 0 {
		t.Fatalf("failure=%v calls=%v", err, fixture.calls)
	}
	for _, request := range []application.HistoryFlags{{Since: "0h"}, {Since: "31d"}, {Since: "30d", ClassFlag: "future"}, {Since: "30d", Tier: "unknown"}, {Since: "30d", OutcomeFlag: "future"}, {Since: "30d", Tags: []string{"invalid"}}} {
		_, _, err := observationServices(fixture).History(request)
		if err == nil || len(fixture.calls) != 0 {
			t.Fatalf("history request=%+v error=%v calls=%v", request, err, fixture.calls)
		}
	}
}

func TestObservationSnapshotProtocolFailureKeepsReason(t *testing.T) {
	fixture := newObservationFixture()
	fixture.failureAt = "snapshot"
	fixture.failure = coordination.ErrCoordinationProtocolMismatch
	_, code, err := observationServices(fixture).Observe(context.Background(), application.StatusRequest{})
	stop, stopped := errors.AsType[*policy.Stop](err)
	if code != 0 || !stopped || stop.Reason != policy.ReasonProtocolMismatch {
		t.Fatalf("code=%d error=%v reason=%v", code, err, stop)
	}
}
