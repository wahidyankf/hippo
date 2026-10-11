package tui_test

import (
	"testing"
	"time"
)

func TestPublicDefaultRefreshDispatchBound(t *testing.T) {
	origin := time.Unix(100, 0).UTC()
	family := &publicBDDInteractive{
		probeBefore: origin, probeAfter: origin.Add(2 * time.Millisecond),
		completedLower: origin.Add(time.Second + time.Millisecond),
		completedUpper: origin.Add(time.Second + 10*time.Millisecond),
	}
	cases := []struct {
		name          string
		start         time.Duration
		maximumActive int
		accepted      bool
	}{
		{name: "25ms dispatch within existing bound", start: 6*time.Second + 35*time.Millisecond, maximumActive: 1, accepted: true},
		{name: "definitely early", start: 6*time.Second - 10*time.Millisecond, maximumActive: 1},
		{name: "beyond existing 500ms", start: 6*time.Second + 511*time.Millisecond, maximumActive: 1},
		{name: "missing start", maximumActive: 1},
		{name: "overlapping observations", start: 6*time.Second + 35*time.Millisecond, maximumActive: 2},
		{name: "nonzero origin bracket overlaps lower bound", start: 6 * time.Second, maximumActive: 1, accepted: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if actual := publicRefreshStartWithinCompletionBracket(family, test.start.Nanoseconds(), test.maximumActive); actual != test.accepted {
				t.Fatalf("observed start acceptance=%t, want %t", actual, test.accepted)
			}
		})
	}
}
