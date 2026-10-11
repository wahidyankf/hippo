package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/internal/policy"
)

func viewsSnapshot(t *testing.T) application.ObservationSnapshot {
	t.Helper()
	snapshot := modelContractSnapshot(100, strings.Repeat("a", 32))
	label, err := identity.ParseDisplayLabel("check-docs")
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Running[0].Label = application.DisplayAnnotation{Label: label, Availability: application.DisplayAvailable}
	snapshot.Running[0].Entry.Source = "hippo-unit"
	snapshot.Running[0].Entry.Class = policy.TaskService
	snapshot.Running[0].Entry.Allocated = coordination.ReservationVector{CPU: 2, MemoryBytes: 1 << 30}
	snapshot.Running[0].AllocatedKnown = true
	snapshot.Queue = modelContractRows(strings.Repeat("b", 32))
	snapshot.Queue[0].Entry.Source = "queued-unit"
	snapshot.Queue[0].Entry.Position = 3
	snapshot.Queue[0].Entry.Requested = coordination.ReservationVector{CPU: 1, MemoryBytes: 1 << 29}
	snapshot.Queue[0].RequestedKnown = true
	snapshot.CapacityKnown, snapshot.UsageKnown, snapshot.QueueRegistered = true, true, true
	snapshot.Status.Sample = policy.Sample{
		AvailableParallelism: 8, CPUUtilizationPercent: new(25.0), PhysicalMemoryBytes: 8 << 30,
		EffectiveMemoryLimitBytes: 8 << 30, AvailableMemoryBytes: new(int64(4 << 30)), DiskFreeBytes: new(int64(12 << 30)),
	}
	snapshot.Status.Resource.State = policy.StateNormal
	snapshot.Status.Coordination = coordination.ReservationTotals{
		Mode: "reservation", ActiveOwners: 1, WaitingOwners: 1,
		Capacity:  coordination.ReservationVector{CPU: 4, MemoryBytes: 4 << 30},
		Allocated: coordination.ReservationVector{CPU: 2, MemoryBytes: 1 << 30},
		Waiting:   coordination.ReservationVector{CPU: 1, MemoryBytes: 1 << 29},
	}
	return snapshot
}

func newViewsContract(t *testing.T, width, height int) (*model, *modelTrace) {
	t.Helper()
	_, trace := newModelContract()
	state := newStyledModel(5*time.Second, false, modelEffects{
		request:  func(request observationRequest) { trace.requests = append(trace.requests, request) },
		schedule: func(schedule refreshSchedule) { trace.schedules = append(trace.schedules, schedule) },
		cancel:   func() { trace.cancels++ },
	})
	state.Update(startMsg{now: modelContractTime(99)})
	modelComplete(state, 1, 110, viewsSnapshot(t), nil)
	state.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return state, trace
}

func assertViewsBounds(t *testing.T, state *model) string {
	t.Helper()
	frame := state.View()
	if !frame.AltScreen {
		t.Error("every frame must retain terminal ownership metadata")
	}
	content := frame.Content
	lines := []string{}
	if content != "" {
		lines = strings.Split(content, "\n")
	}
	if len(lines) > state.height {
		t.Errorf("frame has %d rows, budget=%d", len(lines), state.height)
	}
	for index, line := range lines {
		if cells := lipgloss.Width(line); cells > state.width {
			t.Errorf("line %d width=%d exceeds %d cells: %q", index, cells, state.width, line)
		}
	}
	return content
}

func requireViewsText(t *testing.T, content string, words ...string) {
	t.Helper()
	for _, word := range words {
		if !strings.Contains(content, word) {
			t.Errorf("frame missing %q: %q", word, content)
		}
	}
}

func viewsOpen(state *model, view viewKind) {
	switch view {
	case dashboardView:
	case jobView:
		state.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	case capacityView:
		state.Update(modelKey('c'))
	case helpView:
		state.Update(modelKey('?'))
	}
}

func TestViewsEveryViewportFitsAndKeepsGlobalSummary(t *testing.T) {
	for _, size := range [][2]int{{0, 0}, {0, 16}, {1, 1}, {12, 3}, {39, 15}, {40, 16}, {60, 20}, {80, 24}, {100, 30}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			state, _ := newViewsContract(t, size[0], size[1])
			content := assertViewsBounds(t, state)
			switch {
			case size[0] == 0 || size[1] == 0:
				if content != "" {
					t.Error("zero cell budget emitted content")
				}
			case size[0] == 1 && size[1] == 1:
				requireViewsText(t, content, "q")
			case size[0] < 40 || size[1] < 16:
				requireViewsText(t, content, fmt.Sprintf("%dx%d", size[0], size[1]), "q", "?")
			default:
				requireViewsText(t, content, "CPU", "RAM", "Disk", "Running", "Queue", "check-docs")
				if size[0] >= 80 && size[1] >= 24 {
					requireViewsText(t, content, "queued-unit", "WAIT")
				} else if strings.Contains(content, "queued-unit") {
					t.Error("compact running panel rendered inactive queue rows")
				}
			}
		})
	}
}

func TestViewsMinimumSizeControlsAndRecoveryPreserveSelection(t *testing.T) {
	for _, size := range [][2]int{{0, 0}, {1, 1}, {39, 15}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			state, trace := newViewsContract(t, size[0], size[1])
			selected := state.selected[runningSection].runID
			state.Update(modelKey('?'))
			if state.view != helpView {
				t.Error("help is inaccessible at minimum/zero bounds")
			}
			state.Update(modelKey('r'))
			assertModelRequestCount(t, trace, 2)
			state.Update(modelKey('b'))
			state.Update(tea.WindowSizeMsg{Width: 40, Height: 16})
			if state.view != dashboardView || state.selected[runningSection].runID != selected {
				t.Error("minimum-size recovery lost prior view or complete selection")
			}
			requireViewsText(t, assertViewsBounds(t, state), "check-docs", "CPU")
			state.Update(modelKey('q'))
			if !state.quitting || trace.cancels != 1 {
				t.Error("quit failed after size recovery")
			}
		})
	}
}

func TestViewsDashboardKeyboardAliasesAndDetailDestination(t *testing.T) {
	cases := []struct {
		name     string
		key      tea.KeyPressMsg
		wantView viewKind
		queue    bool
	}{
		{"tab", tea.KeyPressMsg{Code: tea.KeyTab}, dashboardView, true},
		{"next", modelKey('n'), dashboardView, true},
		{"shift-tab", tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, dashboardView, true},
		{"previous", modelKey('p'), dashboardView, true},
		{"enter", tea.KeyPressMsg{Code: tea.KeyEnter}, jobView, false},
		{"capacity", modelKey('c'), capacityView, false},
		{"help", modelKey('?'), helpView, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			state, _ := newViewsContract(t, 60, 20)
			state.Update(test.key)
			if state.view != test.wantView || (state.section == queueSection) != test.queue {
				t.Errorf("key reached wrong destination: view=%d section=%d", state.view, state.section)
			}
			if test.wantView == jobView {
				requireViewsText(t, assertViewsBounds(t, state), strings.Repeat("a", 32), "Allocated", "Source")
			}
			if test.wantView == capacityView {
				requireViewsText(t, assertViewsBounds(t, state), "Capacity", "Reservation", "Host")
			}
		})
	}
}

func TestViewsRowSelectionKeysAndEmptyDetailRemainUsable(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{modelKey('j'), {Code: tea.KeyDown}} {
		state, _ := newViewsContract(t, 40, 16)
		state.Update(modelKey('r'))
		modelComplete(state, 2, 112, modelContractSnapshot(111, "first", "second", "third"), nil)
		state.Update(key)
		if state.selected[runningSection].runID != "second" {
			t.Error("next selection key skipped intended row")
		}
		for _, up := range []tea.KeyPressMsg{modelKey('k'), {Code: tea.KeyUp}} {
			state.Update(up)
			if state.selected[runningSection].runID != "first" {
				t.Error("previous selection key failed")
			}
			state.Update(key)
		}
	}
	state, _ := newViewsContract(t, 40, 16)
	state.Update(modelKey('r'))
	modelComplete(state, 2, 112, modelContractSnapshot(111), nil)
	state.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if state.view != dashboardView || state.selected[runningSection].runID != "" {
		t.Error("empty panel fabricated a job detail destination")
	}
	requireViewsText(t, assertViewsBounds(t, state), "No running jobs")
}

func TestViewsHelpExplainsEveryModifierFreeActionAndReturns(t *testing.T) {
	state, _ := newViewsContract(t, 100, 50)
	state.Update(modelKey('n'))
	selection := state.selected[queueSection]
	state.Update(modelKey('?'))
	content := assertViewsBounds(t, state)
	requireViewsText(t, content, "Help", "n", "p", "j", "k", "Enter", "c", "b", "r", "?", "q", "section", "select", "detail", "capacity", "back", "refresh", "quit")
	state.Update(modelKey('b'))
	if state.view != dashboardView || state.section != queueSection || state.selected[queueSection] != selection {
		t.Error("help back lost the previous section or selection")
	}
}

func TestViewsDetailCapacityHelpScrollAndBackHaveNoTrap(t *testing.T) {
	for _, view := range []viewKind{jobView, capacityView, helpView} {
		t.Run(fmt.Sprint(view), func(t *testing.T) {
			state, _ := newViewsContract(t, 40, 16)
			state.snapshot.Running[0].Entry.Tags = map[string]string{"long": strings.Repeat("unbroken", 160)}
			viewsOpen(state, view)
			for _, key := range []tea.KeyPressMsg{modelKey('j'), {Code: tea.KeyDown}} {
				before := state.body.YOffset()
				state.Update(key)
				if state.body.YOffset() != before+1 {
					t.Error("detail/help next key did not scroll one row")
				}
				assertViewsBounds(t, state)
			}
			for _, key := range []tea.KeyPressMsg{modelKey('k'), {Code: tea.KeyUp}} {
				before := state.body.YOffset()
				state.Update(key)
				if state.body.YOffset() != before-1 {
					t.Error("detail/help previous key did not scroll one row")
				}
			}
			state.Update(modelKey('b'))
			if state.view != dashboardView {
				t.Error("back trapped the operator outside dashboard")
			}
		})
	}
}

func TestViewsHelpBackRestoresDetailViewportAndMissingJobIsSafe(t *testing.T) {
	state, _ := newViewsContract(t, 40, 16)
	state.snapshot.Running[0].Entry.Tags = map[string]string{"long": strings.Repeat("longword", 160)}
	viewsOpen(state, jobView)
	state.Update(modelKey('j'))
	offset := state.body.YOffset()
	state.Update(modelKey('?'))
	state.Update(modelKey('b'))
	if state.view != jobView || state.body.YOffset() != offset {
		t.Error("help back did not restore prior detail and scroll")
	}
	state.Update(modelKey('r'))
	modelComplete(state, 2, 112, modelContractSnapshot(111), nil)
	requireViewsText(t, assertViewsBounds(t, state), "job no longer active")
	state.Update(modelKey('b'))
	if state.view != dashboardView || state.selected[runningSection].runID != "" {
		t.Error("disappeared job detail could not return to empty dashboard")
	}
}

func TestViewsRefreshAndQuitReachEveryViewIncludingRawControlC(t *testing.T) {
	for _, view := range []viewKind{dashboardView, jobView, capacityView, helpView} {
		for _, interrupt := range []bool{false, true} {
			state, trace := newViewsContract(t, 40, 16)
			viewsOpen(state, view)
			state.Update(modelKey('r'))
			if state.view != view {
				t.Error("refresh left the active view")
			}
			assertModelRequestCount(t, trace, 2)
			quit := modelKey('q')
			if interrupt {
				quit = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
			}
			state.Update(quit)
			if !state.quitting || trace.cancels != 1 || (state.interruption.Signal == 2) != interrupt {
				t.Errorf("view=%d rawCtrlC=%t lost owned stop outcome: quitting=%t cancels=%d interruption=%+v", view, interrupt, state.quitting, trace.cancels, state.interruption)
			}
		}
	}
}

func TestViewsIgnoreHostPrefixesAndConflictingViewportDefaults(t *testing.T) {
	keys := []tea.KeyPressMsg{
		{Code: 'h', Mod: tea.ModCtrl},
		{Code: 'j', Mod: tea.ModCtrl},
		{Code: 'k', Mod: tea.ModCtrl},
		{Code: 'l', Mod: tea.ModCtrl},
		{Code: '\\', Mod: tea.ModCtrl},
		{Code: tea.KeySpace, Mod: tea.ModCtrl},
		{Code: 'b', Mod: tea.ModSuper},
		{Code: tea.KeyEscape},
		{Code: tea.KeyEscape},
		modelKey('u'), modelKey('d'), modelKey('f'), modelKey(' '),
	}
	for _, view := range []viewKind{dashboardView, jobView, capacityView, helpView} {
		state, trace := newViewsContract(t, 40, 16)
		viewsOpen(state, view)
		previous, selected, offset := state.view, state.selected, state.body.YOffset()
		for _, key := range keys {
			state.Update(key)
		}
		if state.view != previous || state.selected != selected || state.body.YOffset() != offset || state.quitting {
			t.Error("host prefix or unrelated key captured terminal navigation")
		}
		assertModelRequestCount(t, trace, 1)
		for _, binding := range []bool{state.body.KeyMap.PageUp.Enabled(), state.body.KeyMap.PageDown.Enabled(), state.body.KeyMap.HalfPageUp.Enabled(), state.body.KeyMap.HalfPageDown.Enabled(), state.body.KeyMap.Left.Enabled(), state.body.KeyMap.Right.Enabled()} {
			if binding {
				t.Error("conflicting default viewport action still enabled")
			}
		}
	}
}

func TestViewsLargeListsKeepCompleteSelectionVisibleAndCounted(t *testing.T) {
	state, _ := newViewsContract(t, 40, 16)
	snapshot := viewsSnapshot(t)
	snapshot.Running, snapshot.Queue = nil, nil
	for index := range 1000 {
		runID := fmt.Sprintf("%032x", index+1)
		snapshot.Running = append(snapshot.Running, application.ObservationRow{Entry: coordination.ReservationEntry{RunID: runID, Source: fmt.Sprintf("run-%04d", index)}})
		snapshot.Queue = append(snapshot.Queue, application.ObservationRow{Entry: coordination.ReservationEntry{RunID: fmt.Sprintf("%032x", index+1001), Source: fmt.Sprintf("queue-%04d", index), Position: index + 1}})
	}
	state.Update(modelKey('r'))
	modelComplete(state, 2, 112, snapshot, nil)
	for range 777 {
		state.Update(modelKey('j'))
	}
	selected := state.selected[runningSection].runID
	for _, size := range [][2]int{{40, 16}, {100, 30}, {60, 20}, {39, 15}, {80, 24}} {
		state.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		content := assertViewsBounds(t, state)
		if state.selected[runningSection].runID != selected {
			t.Error("resize changed the complete selected identity")
		}
		if size[0] >= 40 && size[1] >= 16 {
			requireViewsText(t, content, "run-0777", "1000", ">")
		}
	}
	state.Update(modelKey('n'))
	state.Update(modelKey('j'))
	requireViewsText(t, assertViewsBounds(t, state), "queue-0001", "1000")
}

func TestViewsSanitizeControlsAndFitUnicodeAndLongDetails(t *testing.T) {
	state, _ := newViewsContract(t, 40, 16)
	row := &state.snapshot.Running[0]
	row.Entry.Source = "safe\x1b[31mred\x1b[0m\x1b]52;c;private-payload\x07\r\n\t" + strings.Repeat("\u754c", 80) + "e\u0301"
	row.Entry.Tags = map[string]string{"unicode": strings.Repeat("\u754ce\u0301", 80), "long": strings.Repeat("unbroken", 160)}
	for _, view := range []viewKind{dashboardView, jobView} {
		viewsOpen(state, view)
		content := assertViewsBounds(t, state)
		if strings.ContainsAny(content, "\x1b\x07\r\t") || strings.Contains(content, "private-payload") {
			t.Error("untrusted ANSI/control payload escaped into frame")
		}
		if view == dashboardView && !strings.Contains(content, "...") {
			t.Error("truncated row has no ASCII truncation marker")
		}
	}
	for range 200 {
		state.Update(modelKey('j'))
		assertViewsBounds(t, state)
	}
}

func TestViewsNoColorPreservesUnavailableWaitingStaleAndErrorText(t *testing.T) {
	state, _ := newViewsContract(t, 80, 24)
	state.Update(modelKey('r'))
	modelComplete(state, 2, 112, application.ObservationSnapshot{}, errors.New("private error /fixture\x1b[31m"))
	content := assertViewsBounds(t, state)
	requireViewsText(t, content, "ACTIVE", "WAIT", "STALE", "ERROR", "12s", "Observation unavailable")
	if strings.Contains(content, "\x1b") || strings.Contains(content, "/fixture") {
		t.Error("no-color frame styled state or exposed raw observation error")
	}
	state.Update(modelKey('r'))
	snapshot := viewsSnapshot(t)
	snapshot.Status.CPUUtilizationPercent = nil
	snapshot.QueueRegistered, snapshot.CapacityKnown, snapshot.UsageKnown = false, false, false
	snapshot.Status.Coordination.Mode = "exclusive"
	modelComplete(state, 3, 113, snapshot, nil)
	requireViewsText(t, assertViewsBounds(t, state), "unavailable", "unregistered")
}

func TestViewsUnobservedDashboardRemainsUnavailable(t *testing.T) {
	for _, size := range [][2]int{{40, 16}, {120, 32}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			state, _ := newModelContract()
			state.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			state.Update(startMsg{now: modelContractTime(100)})
			assertUnknown := func(phase string) {
				t.Helper()
				content := assertViewsBounds(t, state)
				requireViewsText(t, content, "Running unavailable", "Queue unavailable", "No observation")
				for _, falseEmpty := range []string{"Running 0/0", "Queue 0/0", "No running jobs", "No queue jobs", "Running 0 | Queue 0"} {
					if strings.Contains(content, falseEmpty) {
						t.Errorf("%s claims observed empty accounting before any successful sample: %q", phase, content)
					}
				}
			}
			assertUnknown("loading")
			modelComplete(state, 1, 101, application.ObservationSnapshot{}, errors.New("untrusted initial collector failure"))
			assertUnknown("failed first observation")
			requireViewsText(t, assertViewsBounds(t, state), "ERROR Observation unavailable")
			state.Update(modelKey('r'))
			modelComplete(state, 2, 102, modelContractSnapshot(102), nil)
			content := assertViewsBounds(t, state)
			requireViewsText(t, content, "Running 0/0", "No running jobs", "LIVE")
			if strings.Contains(content, "Running unavailable") || strings.Contains(content, "ERROR") {
				t.Error("genuinely observed empty snapshot did not replace the unavailable state")
			}
		})
	}
}
