package tui

import (
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/policy"
)

func capacityDetailFrames(t *testing.T, state *model) string {
	t.Helper()
	state.Update(modelKey('c'))
	if state.view != capacityView {
		t.Fatal("capacity key did not open capacity detail")
	}
	frames := []string{}
	for range 256 {
		frames = append(frames, assertViewsBounds(t, state))
		before := state.body.YOffset()
		state.Update(modelKey('j'))
		if state.body.YOffset() == before {
			return strings.Join(frames, "\n")
		}
	}
	t.Fatal("capacity detail did not reach its bounded scroll end")
	return ""
}

func TestCapacityDetailCollectedSwapReadingsRemainDistinct(t *testing.T) {
	state, _ := newViewsContract(t, 40, 16)
	state.snapshot.Status.SwapTotalBytes = new(int64(9 << 30))
	state.snapshot.Status.SwapUsedBytes = new(int64(2 << 30))
	state.snapshot.Status.SwapFreeBytes = new(int64(7 << 30))
	state.snapshot.Status.SwapIns = new(int64(123))
	state.snapshot.Status.SwapOuts = new(int64(456))
	state.snapshot.Status.SwapState = "active"
	content := capacityDetailFrames(t, state)
	requireViewsText(t, content,
		"Swap total: 9.0GiB", "Swap used: 2.0GiB", "Swap free: 7.0GiB",
		"Swap-ins: 123", "Swap-outs: 456", "Swap state: active")
}

func TestCapacityDetailUnavailableSwapDiffersFromCollectedZero(t *testing.T) {
	for _, test := range []struct {
		name  string
		value *int64
		bytes string
		count string
	}{
		{name: "missing", bytes: "unavailable", count: "unavailable"},
		{name: "collected zero", value: new(int64(0)), bytes: "0B", count: "0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			state, _ := newViewsContract(t, 40, 16)
			state.snapshot.Status.SwapTotalBytes = test.value
			state.snapshot.Status.SwapUsedBytes = test.value
			state.snapshot.Status.SwapFreeBytes = test.value
			state.snapshot.Status.SwapIns = test.value
			state.snapshot.Status.SwapOuts = test.value
			content := capacityDetailFrames(t, state)
			requireViewsText(t, content,
				"Swap total: "+test.bytes, "Swap used: "+test.bytes, "Swap free: "+test.bytes,
				"Swap-ins: "+test.count, "Swap-outs: "+test.count, "Swap state: unavailable",
				"Allocated: 2CPU 1.0GiB")
		})
	}
}

func TestCapacityDetailPressureAndOwnerPromotionExplainGlobalLimits(t *testing.T) {
	state, _ := newViewsContract(t, 40, 16)
	state.snapshot.Status.Resource.State = policy.StateWarning
	state.snapshot.Status.Resource.Reason = "memory-reserve-low"
	state.snapshot.Status.Promotion.BaseOwners = 2
	state.snapshot.Status.Promotion.EffectiveOwners = 4
	state.snapshot.Status.Promotion.MaximumOwners = 7
	state.snapshot.Status.Promotion.Reason = "healthy-evidence"
	// Row filters may hide all jobs; owner limits and reservations stay global.
	state.snapshot.Running, state.snapshot.Queue = nil, nil
	content := capacityDetailFrames(t, state)
	requireViewsText(t, content,
		"Pressure: warning", "Pressure reason: memory-reserve-low",
		"Base owners: 2", "Effective owners: 4", "Maximum owners: 7",
		"Promotion reason: healthy-evidence", "Allocated: 2CPU 1.0GiB")
}

func TestCapacityDetailReasonsStaySafeAndReachableInCellBounds(t *testing.T) {
	state, _ := newViewsContract(t, 40, 16)
	state.snapshot.Status.Resource.Reason = "pressure-start " + strings.Repeat("\u754ce\u0301 ", 30) + " pressure-end\x1b]52;c;hidden-pressure\x07"
	state.snapshot.Status.Promotion.Reason = "promotion-start " + strings.Repeat("\u754ce\u0301 ", 30) + " promotion-end\x1b[31m\r\t"
	content := capacityDetailFrames(t, state)
	requireViewsText(t, content, "pressure-start", "pressure-end", "promotion-start", "promotion-end", "\u754ce\u0301")
	if strings.ContainsAny(content, "\x1b\x07\r\t") || strings.Contains(content, "hidden-pressure") {
		t.Error("capacity reason exposed terminal controls or hidden escape payload")
	}
	state.Update(modelKey('b'))
	if state.view != dashboardView {
		t.Error("capacity scroll prevented return to dashboard")
	}
}
