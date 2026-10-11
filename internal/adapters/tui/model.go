// Package tui owns interactive terminal presentation for application observations.
package tui

import (
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/status"
)

type section uint8

const (
	runningSection section = iota
	queueSection
)

type viewKind uint8

const (
	dashboardView viewKind = iota
	jobView
	capacityView
	helpView
)

type observationRequest struct{ generation uint64 }

type refreshSchedule struct {
	deadline   time.Time
	generation uint64
}

type modelEffects struct {
	request  func(observationRequest)
	schedule func(refreshSchedule)
	cancel   func()
}

type startMsg struct{ now time.Time }

type clockMsg struct {
	now        time.Time
	generation uint64
	refreshDue bool
}

type observationResult struct {
	snapshot    application.ObservationSnapshot
	err         error
	completedAt time.Time
	generation  uint64
}

type rowSelection struct {
	runID string
	index int
}

type model struct {
	effects        modelEffects
	body           viewport.Model
	interval       time.Duration
	snapshot       application.ObservationSnapshot
	interruption   status.Interruption
	now            time.Time
	refreshAt      time.Time
	selected       [2]rowSelection
	errorSummary   string
	detailID       string
	width          int
	height         int
	generation     uint64
	section        section
	view           viewKind
	previousView   viewKind
	previousOffset int
	hasSnapshot    bool
	busy           bool
	pending        bool
	stale          bool
	quitting       bool
	styling        bool
}

// newModel keeps observation outside the model. Its effect hooks enqueue bounded
// work without collecting, waiting for completion, or owning physical timers.
func newModel(interval time.Duration, effects modelEffects) *model {
	return &model{interval: interval, effects: effects, body: newBodyViewport()}
}

// newStyledModel applies invocation styling without changing observation effects.
func newStyledModel(interval time.Duration, styling bool, effects modelEffects) *model {
	state := newModel(interval, effects)
	state.styling = styling
	return state
}

func (*model) Init() tea.Cmd { return nil }

func (state *model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if state.quitting {
		return state, nil
	}
	switch message := message.(type) {
	case startMsg:
		if state.generation == 0 {
			state.now = message.now
			state.requestObservation()
		}
	case observationResult:
		state.completeObservation(message)
	case clockMsg:
		state.advanceClock(message)
	case tea.WindowSizeMsg:
		state.width, state.height = max(0, message.Width), max(0, message.Height)
		state.syncBody()
	case tea.KeyPressMsg:
		return state, state.handleKey(message)
	}
	return state, nil
}

func (state *model) requestObservation() {
	if state.busy {
		state.pending = true
		return
	}
	state.busy = true
	state.generation++
	state.refreshAt = time.Time{}
	if state.effects.request != nil {
		state.effects.request(observationRequest{generation: state.generation})
	}
}

func (state *model) completeObservation(result observationResult) {
	if !state.busy || result.generation != state.generation {
		return
	}
	state.busy = false
	state.advanceNow(result.completedAt)
	if result.err != nil {
		state.errorSummary = "Observation unavailable"
		state.stale = state.hasSnapshot
		state.syncBody()
	} else {
		state.snapshot = result.snapshot
		state.hasSnapshot, state.stale = true, false
		state.errorSummary = ""
		state.reconcileSelection(runningSection, result.snapshot.Running)
		state.reconcileSelection(queueSection, result.snapshot.Queue)
		state.syncBody()
	}
	if state.pending {
		state.pending = false
		state.requestObservation()
		return
	}
	state.refreshAt = result.completedAt.Add(state.interval)
	if state.effects.schedule != nil {
		state.effects.schedule(refreshSchedule{deadline: state.refreshAt, generation: state.generation})
	}
}

func (state *model) advanceNow(now time.Time) {
	if now.After(state.now) {
		state.now = now
	}
}

func (state *model) advanceClock(message clockMsg) {
	if message.generation != state.generation {
		return
	}
	state.advanceNow(message.now)
	if state.view == jobView {
		state.syncBody()
	}
	if message.refreshDue && !state.busy && !state.refreshAt.IsZero() && !message.now.Before(state.refreshAt) {
		state.requestObservation()
	}
}

func (state *model) rows(active section) []application.ObservationRow {
	if active == queueSection {
		return state.snapshot.Queue
	}
	return state.snapshot.Running
}

func (state *model) moveSelection(delta int) {
	rows := state.rows(state.section)
	if len(rows) == 0 {
		state.selected[state.section] = rowSelection{}
		return
	}
	index := min(max(state.selected[state.section].index+delta, 0), len(rows)-1)
	state.selected[state.section] = rowSelection{runID: rows[index].Entry.RunID, index: index}
}

func (state *model) reconcileSelection(active section, rows []application.ObservationRow) {
	if len(rows) == 0 {
		state.selected[active] = rowSelection{}
		return
	}
	previous := state.selected[active]
	for index, row := range rows {
		if previous.runID != "" && row.Entry.RunID == previous.runID {
			state.selected[active] = rowSelection{runID: previous.runID, index: index}
			return
		}
	}
	index := min(max(previous.index, 0), len(rows)-1)
	state.selected[active] = rowSelection{runID: rows[index].Entry.RunID, index: index}
}

func (state *model) View() tea.View {
	view := tea.NewView(state.render())
	view.AltScreen = true
	return view
}

func (state *model) sampleAge() (time.Duration, bool) {
	if !state.hasSnapshot || state.snapshot.SampledAt == nil || state.snapshot.SampledAt.IsZero() || state.now.IsZero() {
		return 0, false
	}
	return max(0, state.now.Sub(*state.snapshot.SampledAt)), true
}
