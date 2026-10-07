package runtime

import (
	"context"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
)

// ReleaseClock supplies real time to the release application workflow.
type ReleaseClock struct {
	NowFunc func() time.Time
	Pause   func(time.Duration)
}

// Now returns the current timestamp.
func (clock ReleaseClock) Now() time.Time { return ObservationClock{NowFunc: clock.NowFunc}.Now() }

// Ticker starts the sampling clock requested by the application.
func (ReleaseClock) Ticker(interval time.Duration) application.ReleaseTicker {
	ticker := time.NewTicker(interval)
	return application.ReleaseTicker{Ticks: ticker.C, Stop: ticker.Stop}
}

// Wait waits through the host clock or the retained test pause hook.
func (clock ReleaseClock) Wait(ctx context.Context, duration time.Duration) error {
	return ObservationClock{Pause: clock.Pause}.Wait(ctx, duration)
}
