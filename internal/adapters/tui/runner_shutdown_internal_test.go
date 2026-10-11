package tui

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/status"
)

func TestRunnerParentCancellationQuitsBeforeFrameworkInputCloses(t *testing.T) {
	for _, alreadyCanceled := range []bool{false, true} {
		name := "during blocked observation"
		if alreadyCanceled {
			name = "before initialization"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			cause := status.Interruption{Signal: 15, Name: "terminated"}
			if alreadyCanceled {
				cancel(cause)
			}
			fixture := newRunnerFixture()
			var active, joined atomic.Int32
			graceful := false
			dependencies := fixture.dependencies("shutdown", nil)
			dependencies.newProgram = func(programContext context.Context, state *model, _ Options) runnerProgram {
				return newShutdownRegressionProgram(programContext, state, &graceful)
			}
			observe := func(observeContext context.Context) (application.ObservationSnapshot, int, error) {
				active.Add(1)
				defer func() { active.Add(-1); joined.Add(1) }()
				cancel(cause)
				<-observeContext.Done()
				return application.ObservationSnapshot{}, 1, observeContext.Err()
			}
			_, err := runSession(ctx, observe, Options{Interval: time.Second}, dependencies)
			if !graceful || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) || active.Load() != 0 {
				t.Errorf("input shutdown: graceful=%v active=%d joined=%d cause=%v", graceful, active.Load(), joined.Load(), err)
			}
			if !alreadyCanceled && joined.Load() != 1 {
				t.Errorf("blocked observation did not join: %d", joined.Load())
			}
		})
	}
}

func newShutdownRegressionProgram(ctx context.Context, state *model, graceful *bool) runnerProgram {
	messages := make(chan tea.Msg)
	finished := make(chan struct{})
	return runnerProgram{
		run: func() (tea.Model, error) {
			return runShutdownRegressionProgram(ctx, state, messages, finished, graceful)
		},
		send: func(message tea.Msg) {
			select {
			case messages <- message:
			case <-finished:
			}
		},
		kill: func() {},
	}
}

func runShutdownRegressionProgram(ctx context.Context, state *model, messages <-chan tea.Msg, finished chan<- struct{}, graceful *bool) (*model, error) {
	defer close(finished)
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			return state, ctx.Err()
		case <-deadline.C:
			return state, errors.New("framework did not receive quit")
		case message := <-messages:
			if _, quitting := message.(tea.QuitMsg); quitting {
				*graceful = ctx.Err() == nil
				return state, nil
			}
			_, _ = state.Update(message)
		}
	}
}
