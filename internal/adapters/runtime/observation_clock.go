package runtime

import (
	"context"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
)

// ObservationClock supplies time and the existing test timing hooks to observers.
type ObservationClock struct {
	NowFunc func() time.Time
	Pause   func(time.Duration)
}

// Now returns the configured timestamp source or the host clock.
func (clock ObservationClock) Now() time.Time {
	if clock.NowFunc != nil {
		return clock.NowFunc()
	}
	return time.Now()
}

// Wait waits until the interval elapses or the caller cancels.
func (clock ObservationClock) Wait(ctx context.Context, duration time.Duration) error {
	if clock.Pause != nil {
		clock.Pause(duration)
		return ctx.Err()
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Ticker starts the sampling interval chosen by the application.
func (ObservationClock) Ticker(interval time.Duration) application.ReleaseTicker {
	return ReleaseClock{}.Ticker(interval)
}
