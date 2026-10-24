package application_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
)

// stopAfterTickContext is a context whose caller stop lands after the loop under
// test has taken its one pending tick. A real stop that arrives together with a tick
// leaves Go to pick either ready case at random, so a test that cancels a real context
// while a tick is pending runs the "stop wins over the tick" branch on some runs and
// the other branch on others. Here the stop is reported only once the tick channel is
// empty, so the loop always takes the tick first and meets the stop second, and the
// branch that must let the stop win runs on every run.
type stopAfterTickContext struct {
	ticks   <-chan time.Time
	pending chan struct{}
	stopped chan struct{}
}

func newStopAfterTickContext(ticks <-chan time.Time) *stopAfterTickContext {
	stopped := make(chan struct{})
	close(stopped)

	return &stopAfterTickContext{ticks: ticks, pending: make(chan struct{}), stopped: stopped}
}

func (*stopAfterTickContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (*stopAfterTickContext) Value(any) any               { return nil }

func (ctx *stopAfterTickContext) Done() <-chan struct{} {
	if len(ctx.ticks) == 0 {
		return ctx.stopped
	}

	return ctx.pending
}

func (ctx *stopAfterTickContext) Err() error {
	if len(ctx.ticks) == 0 {
		return context.Canceled
	}

	return nil
}

func TestObservationContractMonitorStopLandingWithATickWinsOverTheTick(t *testing.T) {
	fixture := newObservationFixture()
	fixture.ticks <- fixture.now
	ctx := newStopAfterTickContext(fixture.ticks)
	code, err := observationServices(fixture).Monitor(ctx, application.MonitorRequest{Interval: time.Second}, func(application.MonitorTransition) error { return nil })
	if code != 0 || err != nil || fixture.samples != 1 || !fixture.stopped {
		t.Fatalf("code=%d error=%v samples=%d stopped=%t, want a clean stop that never collects for the tick", code, err, fixture.samples, fixture.stopped)
	}
}

func TestReleaseMonitorStopLandingWithATickWinsOverTheTick(t *testing.T) {
	clock := &releaseTestClock{ticks: make(chan time.Time, 1)}
	clock.ticks <- time.Unix(101, 0)
	ctx := newStopAfterTickContext(clock.ticks)
	collector := &releaseTestCollector{cancel: func() {}}
	repository := &releaseTestRepository{}
	services := application.ReleaseServices{Clock: clock, Evidence: repository}
	if err := services.RunMonitor(ctx, releaseTestConfig(collector)); err != nil {
		t.Fatal(err)
	}
	want := []string{"open", "append", "close", "summary", "cleanup"}
	if !reflect.DeepEqual(repository.calls, want) {
		t.Fatalf("release lifecycle=%v, want %v", repository.calls, want)
	}
	if collector.samples != 1 || !clock.stopped {
		t.Fatalf("samples=%d ticker stopped=%t, want one sample and a stopped ticker", collector.samples, clock.stopped)
	}
}
