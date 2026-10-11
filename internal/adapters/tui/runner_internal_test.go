package tui

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/status"
)

type runnerFixture struct {
	base          time.Time
	clock         atomic.Pointer[time.Time]
	events        chan time.Time
	arms          chan time.Time
	started       chan struct{}
	programs      atomic.Int32
	timers        atomic.Int32
	stops         atomic.Int32
	kills         atomic.Int32
	observations  atomic.Int32
	active        atomic.Int32
	joined        atomic.Int32
	ageWithoutRun bool
}

func newRunnerFixture() *runnerFixture {
	fixture := &runnerFixture{base: time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC), events: make(chan time.Time, 1), arms: make(chan time.Time, 1), started: make(chan struct{}, 1)}
	fixture.clock.Store(&fixture.base)
	return fixture
}

func (fixture *runnerFixture) dependencies(action string, startupError error) runnerDependencies {
	return runnerDependencies{
		now: func() time.Time { return *fixture.clock.Load() },
		newTimer: func(time.Duration) runnerTimer {
			fixture.timers.Add(1)
			return runnerTimer{events: fixture.events, reset: fixture.resetTimer, stop: func() bool { fixture.stops.Add(1); return true }}
		},
		newProgram: func(ctx context.Context, state *model, _ Options) runnerProgram {
			fixture.programs.Add(1)
			messages := make(chan tea.Msg)
			programContext, cancel := context.WithCancel(ctx)
			return runnerProgram{
				run: func() (tea.Model, error) {
					if startupError != nil {
						return state, startupError
					}
					return fixture.runProgram(programContext, state, messages, action)
				},
				send: func(message tea.Msg) {
					select {
					case messages <- message:
					case <-programContext.Done():
					}
				},
				kill: func() { fixture.kills.Add(1); cancel() },
			}
		},
	}
}

func (fixture *runnerFixture) runProgram(ctx context.Context, state *model, messages <-chan tea.Msg, action string) (*model, error) {
	completed := 0
	var armedDeadline time.Time
	var pendingAdvance time.Duration
	for {
		select {
		case <-ctx.Done():
			return state, ctx.Err()
		case armedDeadline = <-fixture.arms:
		case <-fixture.started:
			if action == "quit while busy" {
				_, _ = state.Update(tea.KeyPressMsg{Code: 'q'})
				return state, nil
			}
		case message := <-messages:
			if _, quitting := message.(tea.QuitMsg); quitting {
				return state, nil
			}
			_, _ = state.Update(message)
			switch message := message.(type) {
			case observationResult:
				completed++
				if completed == 2 {
					_, _ = state.Update(tea.KeyPressMsg{Code: 'q'})
					return state, nil
				}
				pendingAdvance = time.Second
			case clockMsg:
				if message.now.Equal(fixture.base.Add(time.Second)) {
					age, known := state.sampleAge()
					fixture.ageWithoutRun = known && age == time.Second && fixture.observations.Load() == 1
					pendingAdvance = 5 * time.Second
				}
			}
		}
		// A physical tick follows Reset. Advancing before the scheduler starts
		// would move its first age deadline past the synthetic one-second tick.
		if pendingAdvance != 0 && armedDeadline.After(*fixture.clock.Load()) && !fixture.base.Add(pendingAdvance).Before(armedDeadline) {
			fixture.advance(pendingAdvance)
			pendingAdvance = 0
			armedDeadline = time.Time{}
		}
	}
}

func (fixture *runnerFixture) resetTimer(duration time.Duration) bool {
	deadline := fixture.clock.Load().Add(duration)
	select {
	case <-fixture.arms:
	default:
	}
	fixture.arms <- deadline
	return true
}

func (fixture *runnerFixture) advance(elapsed time.Duration) {
	now := fixture.base.Add(elapsed)
	fixture.clock.Store(&now)
	fixture.events <- now
}

func TestRunnerQuitCancelsAndJoinsBusyObservation(t *testing.T) {
	fixture := newRunnerFixture()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	observe := func(ctx context.Context) (application.ObservationSnapshot, int, error) {
		fixture.observations.Add(1)
		fixture.active.Add(1)
		fixture.started <- struct{}{}
		<-ctx.Done()
		fixture.active.Add(-1)
		fixture.joined.Add(1)
		return application.ObservationSnapshot{}, 1, ctx.Err()
	}
	interruption, err := runSession(ctx, observe, Options{Interval: 5 * time.Second}, fixture.dependencies("quit while busy", nil))
	if err != nil || interruption.Signal != 0 || fixture.observations.Load() != 1 || fixture.joined.Load() != 1 || fixture.active.Load() != 0 {
		t.Errorf("busy quit: error=%v interruption=%+v observations=%d joined=%d active=%d", err, interruption, fixture.observations.Load(), fixture.joined.Load(), fixture.active.Load())
	}
	if fixture.programs.Load() != 1 || fixture.timers.Load() != 1 || fixture.kills.Load() != 1 || fixture.stops.Load() < 1 {
		t.Errorf("owned lifetime cleanup: programs=%d timers=%d kills=%d stops=%d", fixture.programs.Load(), fixture.timers.Load(), fixture.kills.Load(), fixture.stops.Load())
	}
}

func TestRunnerPhysicalTimerAgesWithoutCollectingAndThenRefreshes(t *testing.T) {
	fixture := newRunnerFixture()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	observe := func(context.Context) (application.ObservationSnapshot, int, error) {
		fixture.observations.Add(1)
		return application.ObservationSnapshot{SampledAt: &fixture.base}, 0, nil
	}
	_, err := runSession(ctx, observe, Options{Interval: 5 * time.Second}, fixture.dependencies("timer", nil))
	if err != nil || fixture.observations.Load() != 2 || !fixture.ageWithoutRun || fixture.timers.Load() != 1 {
		t.Errorf("single timer age/refresh: error=%v observations=%d ageWithoutRun=%v physicalTimers=%d", err, fixture.observations.Load(), fixture.ageWithoutRun, fixture.timers.Load())
	}
}

func TestRunnerEarlyProgramFailureKillsPumpAndJoinsOwnedWork(t *testing.T) {
	fixture := newRunnerFixture()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	startupError := errors.New("private initialization detail")
	_, err := runSession(ctx, func(context.Context) (application.ObservationSnapshot, int, error) {
		fixture.observations.Add(1)
		return application.ObservationSnapshot{}, 0, nil
	}, Options{Interval: time.Second}, fixture.dependencies("startup", startupError))
	failure, classified := errors.AsType[status.Failure](err)
	if !classified || failure.Code != status.CodeSupervisionFailed || fixture.programs.Load() != 1 || fixture.kills.Load() != 1 || fixture.timers.Load() != 1 || fixture.stops.Load() < 1 {
		t.Errorf("early failure cleanup: error=%v programs=%d kills=%d timers=%d stops=%d", err, fixture.programs.Load(), fixture.kills.Load(), fixture.timers.Load(), fixture.stops.Load())
	}
	if err != nil && strings.Contains(err.Error(), "private initialization detail") {
		t.Error("unclassified initialization error escaped through runner diagnostic")
	}
}

func TestRunnerRestorationFailureOutranksParentInterruption(t *testing.T) {
	fixture := newRunnerFixture()
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(status.Interruption{Signal: 15, Name: "terminated"})
	dependencies := fixture.dependencies("startup", context.Canceled)
	attempts := 0
	dependencies.restore = func() error { attempts++; return errors.New("private restoration fault") }
	_, err := runSession(ctx, func(context.Context) (application.ObservationSnapshot, int, error) {
		return application.ObservationSnapshot{}, 0, nil
	}, Options{Interval: time.Second}, dependencies)
	failure, classified := errors.AsType[status.Failure](err)
	if attempts != 1 || !classified || failure.Code != status.CodeSupervisionFailed || strings.Contains(failure.Message, "private") {
		t.Errorf("restoration priority: attempts=%d error=%v", attempts, err)
	}
}

func TestRunnerTerminalFailureOutranksConcurrentParentInterruption(t *testing.T) {
	fixture := newRunnerFixture()
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(status.Interruption{Signal: 2, Name: "interrupt"})
	_, err := runSession(ctx, func(context.Context) (application.ObservationSnapshot, int, error) {
		return application.ObservationSnapshot{}, 0, nil
	}, Options{Interval: time.Second}, fixture.dependencies("startup", errors.New("private input fault")))
	failure, classified := errors.AsType[status.Failure](err)
	if !classified || failure.Code != status.CodeSupervisionFailed || strings.Contains(failure.Message, "private") {
		t.Errorf("concurrent terminal failure was displaced by signal: %v", err)
	}
}
