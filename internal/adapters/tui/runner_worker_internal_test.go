package tui

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
)

func TestRunnerWorkerCancellationUnblocksFullCompletionMailbox(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var calls atomic.Int32
	secondCompleted := make(chan struct{})
	worker := newObservationWorker(ctx, func(context.Context) (application.ObservationSnapshot, int, error) {
		if calls.Add(1) == 2 {
			close(secondCompleted)
		}
		return application.ObservationSnapshot{}, 0, nil
	}, time.Now)
	worker.request(observationRequest{generation: 1})
	var first observationResult
	select {
	case first = <-worker.results:
	case <-ctx.Done():
		t.Fatal("first observation did not publish")
	}
	// A stalled event pump still owns the first unread completion. The next
	// completion has no mailbox space and must yield to cancellation.
	worker.results <- first
	worker.request(observationRequest{generation: 2})
	select {
	case <-secondCompleted:
	case <-ctx.Done():
		t.Fatal("second observation did not complete")
	}
	cancel()
	select {
	case <-worker.done:
	case <-time.After(time.Second):
		t.Fatal("full completion mailbox prevented worker join")
	}
	if calls.Load() != 2 || len(worker.results) != 1 || cap(worker.requests) != 1 || cap(worker.results) != 1 {
		t.Errorf("bounded publication: calls=%d queued=%d capacities=%d/%d", calls.Load(), len(worker.results), cap(worker.requests), cap(worker.results))
	}
}

func TestRunnerSchedulerUpdatesGenerationWhileObservationIsBusy(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	base := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	ticks := make(chan time.Time, 1)
	physicalTimers := 0
	scheduler := newRunnerScheduler(ctx, func() time.Time { return base }, func(time.Duration) runnerTimer {
		physicalTimers++
		return runnerTimer{events: ticks, reset: func(time.Duration) bool { return true }, stop: func() bool { return true }}
	})
	for generation := uint64(1); generation <= 2; generation++ {
		// A new request disarms completion's refresh deadline but age still
		// follows the current generation while that observation is blocked.
		scheduler.replace(refreshSchedule{generation: generation})
		ticks <- base.Add(time.Duration(generation) * time.Second)
		select {
		case message := <-scheduler.events:
			if message.generation != generation || message.refreshDue {
				t.Errorf("busy clock message=%+v generation=%d", message, generation)
			}
		case <-ctx.Done():
			t.Fatal("age clock stopped during busy generation")
		}
	}
	cancel()
	<-scheduler.done
	if physicalTimers != 1 || cap(scheduler.wake) != 1 || cap(scheduler.events) != 1 {
		t.Errorf("scheduler lifetime allocations: timers=%d capacities=%d/%d", physicalTimers, cap(scheduler.wake), cap(scheduler.events))
	}
}
