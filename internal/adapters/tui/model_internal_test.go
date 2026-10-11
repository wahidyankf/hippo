package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
)

type modelTrace struct {
	requests  []observationRequest
	schedules []refreshSchedule
	cancels   int
}

func newModelContract() (*model, *modelTrace) {
	trace := &modelTrace{}
	state := newModel(5*time.Second, modelEffects{
		request:  func(request observationRequest) { trace.requests = append(trace.requests, request) },
		schedule: func(schedule refreshSchedule) { trace.schedules = append(trace.schedules, schedule) },
		cancel:   func() { trace.cancels++ },
	})
	return state, trace
}

func modelKey(value rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: value, Text: string(value)} }

func modelContractTime(seconds int64) time.Time { return time.Unix(seconds, 0) }

func modelContractRows(ids ...string) []application.ObservationRow {
	rows := make([]application.ObservationRow, len(ids))
	for index, runID := range ids {
		rows[index] = application.ObservationRow{Entry: coordination.ReservationEntry{RunID: runID}}
	}
	return rows
}

func modelContractSnapshot(seconds int64, ids ...string) application.ObservationSnapshot {
	measured := modelContractTime(seconds)
	return application.ObservationSnapshot{SampledAt: &measured, Running: modelContractRows(ids...)}
}

func modelComplete(state *model, generation uint64, seconds int64, snapshot application.ObservationSnapshot, err error) {
	state.Update(observationResult{generation: generation, completedAt: modelContractTime(seconds), snapshot: snapshot, err: err})
}

func assertModelRequestCount(t *testing.T, trace *modelTrace, want int) {
	t.Helper()
	if len(trace.requests) != want {
		t.Errorf("observation requests=%d want=%d: %+v", len(trace.requests), want, trace.requests)
	}
}

func TestModelRefreshCoalescesAndSchedulesAfterCompletion(t *testing.T) {
	state, trace := newModelContract()
	state.Update(startMsg{now: modelContractTime(10)})
	for range 3 {
		state.Update(modelKey('r'))
	}
	assertModelRequestCount(t, trace, 1)
	if !state.busy || !state.pending {
		t.Error("busy refresh did not retain one pending request")
	}
	modelComplete(state, 1, 20, modelContractSnapshot(18), nil)
	assertModelRequestCount(t, trace, 2)
	if !state.busy || state.pending || len(trace.schedules) != 0 {
		t.Error("completion did not consume exactly one immediate follow-up")
	}
	modelComplete(state, 2, 30, modelContractSnapshot(28), nil)
	if state.busy || len(trace.schedules) != 1 {
		t.Errorf("idle completion schedules=%+v busy=%t", trace.schedules, state.busy)
	} else if schedule := trace.schedules[0]; !schedule.deadline.Equal(modelContractTime(35)) || schedule.generation != 2 {
		t.Errorf("refresh deadline must use completion plus interval: %+v", schedule)
	}
	state.Update(clockMsg{now: modelContractTime(34), generation: 2, refreshDue: true})
	assertModelRequestCount(t, trace, 2)
	state.Update(clockMsg{now: modelContractTime(35), generation: 2, refreshDue: true})
	assertModelRequestCount(t, trace, 3)
}

func TestModelNavigationAndResizeDuringOutstandingObservation(t *testing.T) {
	state, trace := newModelContract()
	state.Update(startMsg{now: modelContractTime(10)})
	snapshot := modelContractSnapshot(10, "running-owner")
	snapshot.Queue = modelContractRows("queued-owner")
	modelComplete(state, 1, 11, snapshot, nil)
	state.Update(modelKey('r'))
	// Generation two has no completion. Input must still change state without
	// observing, waiting for, or starting a second simultaneous collection.
	state.Update(modelKey('n'))
	state.Update(tea.WindowSizeMsg{Width: 40, Height: 16})
	if !state.busy || state.section != queueSection || state.width != 40 || state.height != 16 {
		t.Errorf("input unavailable before completion: busy=%t section=%d size=%dx%d", state.busy, state.section, state.width, state.height)
	}
	assertModelRequestCount(t, trace, 2)
}

func TestModelLastGoodStaleFailureAndRecovery(t *testing.T) {
	state, trace := newModelContract()
	state.Update(startMsg{now: modelContractTime(10)})
	unsafe := errors.New("private fixture /secret/path\x1b[31m")
	modelComplete(state, 1, 11, application.ObservationSnapshot{}, unsafe)
	if state.hasSnapshot || state.stale || state.errorSummary == "" || strings.Contains(state.errorSummary, "secret") || strings.Contains(state.errorSummary, "\x1b") {
		t.Errorf("initial failure fabricated data or exposed raw error: known=%t stale=%t summary=%q", state.hasSnapshot, state.stale, state.errorSummary)
	}
	state.Update(modelKey('r'))
	good := modelContractSnapshot(12, "old-owner")
	good.Running[0].Entry.Allocated = coordination.ReservationVector{CPU: 2, MemoryBytes: 1024}
	modelComplete(state, 2, 13, good, nil)
	state.Update(modelKey('r'))
	modelComplete(state, 3, 14, application.ObservationSnapshot{}, unsafe)
	if !state.hasSnapshot || !state.stale || state.errorSummary == "" || len(state.snapshot.Running) != 1 {
		t.Errorf("failure did not retain last-good stale snapshot: known=%t stale=%t summary=%q rows=%d", state.hasSnapshot, state.stale, state.errorSummary, len(state.snapshot.Running))
	} else if row := state.snapshot.Running[0]; row.Entry.RunID != "old-owner" || row.Entry.Allocated != good.Running[0].Entry.Allocated {
		t.Error("failed refresh changed retained owner allocation")
	}
	state.Update(modelKey('r'))
	modelComplete(state, 4, 16, modelContractSnapshot(15, "new-owner"), nil)
	if !state.hasSnapshot || state.stale || state.errorSummary != "" || len(state.snapshot.Running) != 1 {
		t.Errorf("successful recovery kept failure markers: known=%t stale=%t summary=%q", state.hasSnapshot, state.stale, state.errorSummary)
	} else if state.snapshot.Running[0].Entry.RunID != "new-owner" {
		t.Error("recovery did not replace retained data")
	}
	assertModelRequestCount(t, trace, 4)
}

func TestModelAgeTicksNeverObserveOrInventSampleTime(t *testing.T) {
	state, trace := newModelContract()
	state.Update(startMsg{now: modelContractTime(99)})
	modelComplete(state, 1, 110, modelContractSnapshot(100), nil)
	if age, known := state.sampleAge(); !known || age != 10*time.Second {
		t.Errorf("sample age must use second measurement: %s known=%t", age, known)
	}
	state.Update(clockMsg{now: modelContractTime(111), generation: 1})
	if age, known := state.sampleAge(); !known || age != 11*time.Second {
		t.Errorf("lightweight tick failed to advance age: %s known=%t", age, known)
	}
	assertModelRequestCount(t, trace, 1)
	state.Update(modelKey('r'))
	modelComplete(state, 2, 112, application.ObservationSnapshot{}, nil)
	if age, known := state.sampleAge(); known || age != 0 {
		t.Errorf("missing sample time was replaced by completion time: %s known=%t", age, known)
	}
}

func TestModelSelectionTracksFullIDAndNearestSurvivor(t *testing.T) {
	state, _ := newModelContract()
	first, middle, last := strings.Repeat("a", 32), strings.Repeat("a", 31)+"b", strings.Repeat("c", 32)
	state.Update(startMsg{now: modelContractTime(10)})
	modelComplete(state, 1, 11, modelContractSnapshot(10, first, middle, last), nil)
	state.Update(modelKey('j'))
	if state.selected[runningSection].runID != middle {
		t.Error("navigation did not select the complete middle ID")
	}
	state.Update(modelKey('r'))
	modelComplete(state, 2, 12, modelContractSnapshot(11, last, first, middle), nil)
	if selected := state.selected[runningSection]; selected.runID != middle || selected.index != 2 {
		t.Errorf("reorder confused ID shorthand with identity: %+v", selected)
	}
	state.Update(modelKey('r'))
	modelComplete(state, 3, 13, modelContractSnapshot(12, last, first), nil)
	if selected := state.selected[runningSection]; selected.runID != first || selected.index != 1 {
		t.Errorf("removed selection did not choose nearest prior index: %+v", selected)
	}
	state.Update(modelKey('r'))
	modelComplete(state, 4, 14, modelContractSnapshot(13), nil)
	if state.selected[runningSection].runID != "" {
		t.Error("empty panel retained a false selected job")
	}
}

func TestModelRejectsStaleGenerationsAndLateResultsAfterQuit(t *testing.T) {
	state, trace := newModelContract()
	state.Update(startMsg{now: modelContractTime(10)})
	state.Update(modelKey('r'))
	modelComplete(state, 1, 11, modelContractSnapshot(10, "first-owner"), nil)
	modelComplete(state, 1, 12, modelContractSnapshot(11, "stale-result"), nil)
	state.Update(clockMsg{now: modelContractTime(100), generation: 1, refreshDue: true})
	assertModelRequestCount(t, trace, 2)
	state.Update(modelKey('q'))
	modelComplete(state, 2, 101, modelContractSnapshot(100, "late-result"), nil)
	state.Update(modelKey('q'))
	if !state.quitting || trace.cancels != 1 {
		t.Errorf("quit did not cancel owned effects exactly once: quitting=%t cancels=%d", state.quitting, trace.cancels)
	}
	if len(state.snapshot.Running) != 1 || state.snapshot.Running[0].Entry.RunID != "first-owner" {
		t.Error("stale generation or result after quit changed the retained display")
	}
	assertModelRequestCount(t, trace, 2)
}
