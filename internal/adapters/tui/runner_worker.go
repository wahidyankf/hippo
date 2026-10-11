package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/status"
)

type observationWorker struct {
	cancelled <-chan struct{}
	requests  chan observationRequest
	results   chan observationResult
	done      chan struct{}
}

func newObservationWorker(ctx context.Context, observe func(context.Context) (application.ObservationSnapshot, int, error), now func() time.Time) *observationWorker {
	worker := &observationWorker{cancelled: ctx.Done(), requests: make(chan observationRequest, 1), results: make(chan observationResult, 1), done: make(chan struct{})}
	go worker.run(ctx, observe, now)
	return worker
}

func (worker *observationWorker) request(request observationRequest) {
	select {
	case <-worker.cancelled:
	case worker.requests <- request:
	default:
	}
}

func (worker *observationWorker) run(ctx context.Context, observe func(context.Context) (application.ObservationSnapshot, int, error), now func() time.Time) {
	defer close(worker.done)
	for {
		select {
		case <-ctx.Done():
			return
		case request := <-worker.requests:
			if ctx.Err() != nil {
				return
			}
			snapshot, code, err := observe(ctx)
			if err == nil && code != 0 {
				err = status.Fail(status.CodeSupervisionFailed, "Observation unavailable")
			}
			result := observationResult{snapshot: snapshot, err: err, completedAt: now(), generation: request.generation}
			select {
			case worker.results <- result:
			case <-ctx.Done():
				return
			}
		}
	}
}

func pumpRunnerEvents(ctx context.Context, send func(tea.Msg), started time.Time, results <-chan observationResult, clocks <-chan clockMsg, done chan<- struct{}) {
	defer close(done)
	send(startMsg{now: started})
	for {
		select {
		case <-ctx.Done():
			send(tea.QuitMsg{})
			return
		case result := <-results:
			send(result)
		case clock := <-clocks:
			send(clock)
		}
	}
}
