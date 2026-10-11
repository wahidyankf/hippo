package tui

import (
	"context"
	"sync"
	"time"
)

type runnerScheduler struct {
	mutex   sync.Mutex
	desired refreshSchedule
	wake    chan struct{}
	events  chan clockMsg
	done    chan struct{}
}

func newRunnerScheduler(ctx context.Context, now func() time.Time, newTimer func(time.Duration) runnerTimer) *runnerScheduler {
	scheduler := &runnerScheduler{wake: make(chan struct{}, 1), events: make(chan clockMsg, 1), done: make(chan struct{})}
	timer := newTimer(time.Hour)
	timer.stop()
	go scheduler.run(ctx, now, timer)
	return scheduler
}

func (scheduler *runnerScheduler) replace(desired refreshSchedule) {
	scheduler.mutex.Lock()
	scheduler.desired = desired
	scheduler.mutex.Unlock()
	select {
	case scheduler.wake <- struct{}{}:
	default:
	}
}

func (scheduler *runnerScheduler) current() refreshSchedule {
	scheduler.mutex.Lock()
	defer scheduler.mutex.Unlock()
	return scheduler.desired
}

func (scheduler *runnerScheduler) run(ctx context.Context, now func() time.Time, timer runnerTimer) {
	defer close(scheduler.done)
	defer timer.stop()
	nextAge := now().Add(time.Second)
	for {
		desired := scheduler.current()
		var ticks <-chan time.Time
		if desired.generation != 0 {
			deadline := nextAge
			if !desired.deadline.IsZero() && desired.deadline.Before(deadline) {
				deadline = desired.deadline
			}
			if !timer.stop() {
				select {
				case <-timer.events:
				default:
				}
			}
			timer.reset(max(0, deadline.Sub(now())))
			ticks = timer.events
		}
		select {
		case <-ctx.Done():
			return
		case <-scheduler.wake:
		case sampled := <-ticks:
			desired = scheduler.current()
			ageDue := !sampled.Before(nextAge)
			refreshDue := !desired.deadline.IsZero() && !sampled.Before(desired.deadline)
			if !ageDue && !refreshDue {
				continue
			}
			if ageDue {
				nextAge = sampled.Add(time.Second)
			}
			if refreshDue {
				scheduler.clearRefresh(desired)
			}
			select {
			case scheduler.events <- clockMsg{now: sampled, generation: desired.generation, refreshDue: refreshDue}:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (scheduler *runnerScheduler) clearRefresh(expected refreshSchedule) {
	scheduler.mutex.Lock()
	defer scheduler.mutex.Unlock()
	if scheduler.desired == expected {
		scheduler.desired.deadline = time.Time{}
	}
}
