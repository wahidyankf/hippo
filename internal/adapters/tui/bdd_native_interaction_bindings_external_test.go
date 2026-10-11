package tui_test

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/wahidyankf/hippo/internal/adapters/tui"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/policy"
)

// Only observer inputs and physical keys cross this original scenario boundary.
// Native bindings observe the genuine renderer's decoded PTY cells.
type nativeBDDInteraction struct {
	fixture  *nativeBDDViewports
	kind     string
	view     string
	selected string
	key      string
	frames   []tui.BDDNativeTransportFrame

	// Per-instance effects let the sampler regression drive these actual callers;
	// native bindings leave them nil and use the original transport lifecycle.
	samplingKey         func(context.Context, []byte) error
	samplingObservation bddNativeTransportObservation
	samplingQuit        func() error
}

func (fixture *nativeBDDViewports) interactionHandler(pattern string) any {
	if fixture.interaction == nil {
		fixture.interaction = &nativeBDDInteraction{fixture: fixture}
	}
	state := fixture.interaction
	handlers := map[string]any{
		`^known zero reservation demand and unavailable optional CPU memory swap and disk readings$`: state.prepareMissingResources,
		`^capacity detail is displayed$`:                                                                                                                     state.openCapacity,
		`^reservation demand says zero and the missing host readings say unavailable$`:                                                                       state.requireMissingResources,
		`^a job allocated two CPUs and one GiB with host CPU utilization at twenty percent$`:                                                                 state.prepareResources,
		`^job and capacity details are inspected$`:                                                                                                           state.inspectResources,
		`^reservation allocation and machine utilization have distinct labels without claiming process CPU or RSS$`:                                          state.requireResources,
		`^a selected live job at 40 columns and 16 rows$`:                                                                                                    state.prepareResizeSelection,
		`^the terminal shrinks to 39 by 15 cells and returns to 40 by 16 cells$`:                                                                             state.shrinkAndRecover,
		`^the fitted minimum-size message gives way to the compact view with the same selected run ID$`:                                                      state.requireRecovery,
		`^a TUI row and detail containing (an overlong unbroken value|wide Unicode characters|combining Unicode characters|ANSI escapes and control bytes)$`: state.prepareUnsafe,
		`^they are rendered at 40 columns and 16 rows$`:                                                                                                      state.renderUnsafe,
		`^controls are not displayed and each row and detail fits its terminal-cell and row budget$`:                                                         state.requireUnsafe,
		`^one thousand running and queued jobs with a selected live run ID$`:                                                                                 state.prepareLarge,
		`^the operator navigates and repeatedly resizes between compact and wide views$`:                                                                     state.navigateLarge,
		`^the selected run ID stays visible and each list shows its bounded position and count$`:                                                             state.requireLarge,
		`^three visible jobs with the middle run ID selected$`:                                                                                               state.prepareMiddle,
		`^a refresh removes that selected job$`:                                                                                                              state.removeSelected,
		`^the nearest surviving row is selected by its prior position and remains visible$`:                                                                  state.requireNearest,
		`^job detail is open for a live run ID$`:                                                                                                             state.prepareDetail,
		`^a refresh removes that run ID$`:                                                                                                                    state.removeDetail,
		`^detail reports job no longer active and back returns to a usable section$`:                                                                         state.requireMissingDetail,
		`^a TUI whose active job panel is empty$`:                                                                                                            state.prepareEmpty,
		`^the operator requests job detail$`:                                                                                                                 state.requestDetail,
		`^the explicit empty panel remains usable with no selected job$`:                                                                                     state.requireEmpty,
		`^the TUI (dashboard|job detail|capacity detail|help|minimum-size) is open$`:                                                                         state.prepareView,
		`^one refresh is requested without leaving that view$`:                                                                                               state.requireManualRefresh,
		`^a populated TUI$`:         state.prepareHelp,
		`^keyboard help is opened$`: state.openHelp,
		`^every section selection detail capacity back refresh help and quit action has a documented usable key$`: state.requireHelp,

		`^a populated TUI with a selected run ID$`:                                    state.prepareIgnored,
		`^no job action or TUI quit occurs and selection is unchanged$`:               state.requireIgnored,
		`^color is disabled and the TUI has selected waiting stale and error states$`: state.prepareNoColor,
		`^the dashboard and details are rendered$`:                                    state.renderNoColor,
		`^text markers distinguish those states without ANSI styling$`:                state.requireNoColor,
	}
	return handlers[pattern]
}

func (fixture *nativeBDDViewports) pressInteractionOrDetailKeyboard(key string) error {
	if fixture.interaction != nil && fixture.interaction.kind != "" {
		return fixture.interaction.pressKey(key)
	}
	return fixture.pressDetailKeyboard(key)
}

func (state *nativeBDDInteraction) reset() {
	state.kind, state.view, state.selected, state.key = "", "", "", ""
	state.frames = nil
}

func (state *nativeBDDInteraction) prepare(kind string, snapshot application.ObservationSnapshot) error {
	if state.fixture.refresh == nil {
		return errors.New("native interaction observer was not composed")
	}
	if err := state.fixture.refresh.prepareViewport(snapshot, false, nativeObserverTime(100), 40, 16); err != nil {
		return err
	}
	state.kind, state.view = kind, "HIPPO | Running and Queue"
	state.selected = fmt.Sprintf("%032x", 1)
	if len(snapshot.Running) == 0 {
		state.selected = ""
		return state.await(state.view, "LIVE", "[Running]", "Running 0/0", "No running jobs")
	}
	return state.await(state.view, "LIVE", "[Running]", fmt.Sprintf("Running 1/%d", len(snapshot.Running)))
}

func (state *nativeBDDInteraction) await(title string, tokens ...string) error {
	observation := state.samplingObservation
	if observation == nil {
		if state.fixture.transport == nil || state.fixture.screen == nil {
			return errors.New("native interaction transport was not prepared")
		}
		observation = bddNativeTransportCapture{transport: state.fixture.transport, screen: state.fixture.screen}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	frame, err := awaitBDDNativeTransportFrame(ctx, observation, func(frame tui.BDDNativeTransportFrame) bool {
		if !frame.Alternate || frame.Width != state.fixture.width || frame.Height != state.fixture.height || frame.OverflowWrites != 0 {
			return false
		}
		content := strings.Join(frame.Rows, "\n")
		if !strings.Contains(content, title) {
			return false
		}
		for _, token := range tokens {
			if !strings.Contains(content, token) {
				return false
			}
		}
		return true
	})
	if err != nil {
		return fmt.Errorf("native interaction %s omitted %q: %w; rows=%q", title, tokens, err, frame.Rows)
	}
	if err := state.bounds(frame); err != nil {
		return err
	}
	state.frames = append(state.frames, frame)
	return nil
}

func (state *nativeBDDInteraction) bounds(frame tui.BDDNativeTransportFrame) error {
	if frame.Width != state.fixture.width || frame.Height != state.fixture.height || !frame.Alternate || len(frame.Rows) > frame.Height || frame.OverflowWrites != 0 {
		return fmt.Errorf("native interaction frame exceeds %dx%d: %+v", state.fixture.width, state.fixture.height, frame)
	}
	for _, row := range frame.Rows {
		if lipgloss.Width(row) > frame.Width {
			return fmt.Errorf("native interaction row exceeds %d cells: %q", frame.Width, row)
		}
	}
	return nil
}

func (state *nativeBDDInteraction) resize(width, height int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := state.fixture.transport.Resize(ctx, width, height); err != nil {
		return err
	}
	state.fixture.width, state.fixture.height = width, height
	return nil
}

func (state *nativeBDDInteraction) keyBytes(value string) error {
	if state.kind == "" || (state.fixture.transport == nil && state.samplingKey == nil) {
		return errors.New("native interaction key has no prepared session")
	}
	if value == "" {
		return errors.New("native interaction key bytes are empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	sendKey := state.samplingKey
	if sendKey == nil {
		sendKey = state.fixture.transport.SendKey
	}
	// Keep one original action deadline while respecting SendKey's byte limit.
	for len(value) != 0 {
		count := min(64, len(value))
		if err := sendKey(ctx, []byte(value[:count])); err != nil {
			return err
		}
		value = value[count:]
	}
	return nil
}

func (state *nativeBDDInteraction) open(key, title string, tokens ...string) error {
	if err := state.keyBytes(key); err != nil {
		return err
	}
	if err := state.await(title, tokens...); err != nil {
		return err
	}
	state.view = title
	return nil
}

func (state *nativeBDDInteraction) identity(id string) error {
	if err := state.open("\r", "HIPPO | Job detail", "Run ID: "+id); err != nil {
		return err
	}
	return state.open("b", "HIPPO | Running and Queue")
}

func (state *nativeBDDInteraction) texts() string {
	var rows []string
	for _, frame := range state.frames {
		rows = append(rows, frame.Rows...)
	}
	return strings.Join(strings.Fields(strings.Join(rows, "\n")), " ")
}

func (state *nativeBDDInteraction) requireText(tokens ...string) error {
	content := state.texts()
	for _, token := range tokens {
		if !strings.Contains(content, token) {
			return fmt.Errorf("native interaction observed frames omit %q", token)
		}
	}
	return nil
}

func (state *nativeBDDInteraction) prepareMissingResources() error {
	snapshot := tui.BDDNativeObserverSnapshot()
	snapshot.Status.Sample = policy.Sample{}
	snapshot.Status.Coordination.Allocated = coordination.ReservationVector{}
	snapshot.Status.Coordination.Waiting = coordination.ReservationVector{}
	return state.prepare("missing-resources", snapshot)
}

func (state *nativeBDDInteraction) prepareResources() error {
	return state.prepare("resources", tui.BDDNativeObserverSnapshot())
}

func (state *nativeBDDInteraction) openCapacity() error {
	return state.open("c", "HIPPO | Capacity detail", "Host CPU utilization:")
}

func (state *nativeBDDInteraction) capacityEvidence() error {
	for _, stop := range []struct {
		keys   string
		tokens []string
	}{
		{strings.Repeat("j", 8), []string{"Disk free:", "Reservation accounting"}},
		{strings.Repeat("j", 10), []string{"Waiting request:"}},
		{strings.Repeat("j", 200), []string{"process RSS or CPU usage."}},
	} {
		if err := state.keyBytes(stop.keys); err != nil {
			return err
		}
		if err := state.await("HIPPO | Capacity detail", stop.tokens...); err != nil {
			return err
		}
	}
	return nil
}

func (state *nativeBDDInteraction) requireMissingResources() error {
	if err := state.capacityEvidence(); err != nil {
		return err
	}
	if err := state.requireText("Host CPU utilization: unavailable", "Available RAM: unavailable", "Swap used: unavailable", "Disk free: unavailable", "Waiting request: 0CPU", "Allocated: 0CPU"); err != nil {
		return err
	}
	return state.fixture.quit()
}

func (state *nativeBDDInteraction) inspectResources() error {
	if err := state.open("\r", "HIPPO | Job detail", state.selected, "Allocated: 2CPU", "1.0GiB"); err != nil {
		return err
	}
	if err := state.open("b", "HIPPO | Running and Queue"); err != nil {
		return err
	}
	if err := state.openCapacity(); err != nil {
		return err
	}
	return state.capacityEvidence()
}

func (state *nativeBDDInteraction) requireResources() error {
	if err := state.requireText("Allocated: 2CPU", "1.0GiB", "Host CPU utilization: 20%", "Reservation accounting", "not process RSS or CPU usage."); err != nil {
		return err
	}
	return state.fixture.quit()
}

func (state *nativeBDDInteraction) prepareMiddle() error {
	if err := state.prepare("middle", tui.BDDNativeObserverSnapshot()); err != nil {
		return err
	}
	if err := state.resize(40, 16); err != nil {
		return err
	}
	if err := state.await(state.view, "Running 1/3"); err != nil {
		return err
	}
	if err := state.keyBytes("j"); err != nil {
		return err
	}
	state.selected = fmt.Sprintf("%032x", 2)
	return state.await(state.view, "Running 2/3", "> ACTIVE unavailable run-1 ")
}

func (state *nativeBDDInteraction) prepareResizeSelection() error {
	if err := state.prepareMiddle(); err != nil {
		return err
	}
	state.kind = "resize"
	return state.identity(state.selected)
}

func (state *nativeBDDInteraction) shrinkAndRecover() error {
	if err := state.resize(39, 15); err != nil {
		return err
	}
	if err := state.await("Terminal too small: need 40x16"); err != nil {
		return err
	}
	if err := state.resize(40, 16); err != nil {
		return err
	}
	return state.await("HIPPO | Running and Queue", "[Running]", "Running 2/3", "> ACTIVE unavailable run-1 ")
}

func (state *nativeBDDInteraction) requireRecovery() error {
	if err := state.identity(state.selected); err != nil {
		return err
	}
	return state.fixture.quit()
}

func (state *nativeBDDInteraction) prepareUnsafe(value string) error {
	texts := map[string]string{
		"an overlong unbroken value":     strings.Repeat("unbroken", 160),
		"wide Unicode characters":        strings.Repeat("\u754c", 80),
		"combining Unicode characters":   strings.Repeat("e\u0301", 80),
		"ANSI escapes and control bytes": "safe\x1b[31mred\x1b]52;c;private-payload\x07\r\t",
	}
	text, ok := texts[value]
	if !ok {
		return fmt.Errorf("unknown original unsafe text %q", value)
	}
	snapshot := tui.BDDNativeObserverSnapshot()
	snapshot.Running[0].Entry.Source = text
	snapshot.Running[0].Entry.Tags = map[string]string{"text": text}
	if err := state.prepare("unsafe", snapshot); err != nil {
		return err
	}
	return state.resize(40, 16)
}

func (state *nativeBDDInteraction) renderUnsafe() error {
	if err := state.await("HIPPO | Running and Queue", "Running 1/3"); err != nil {
		return err
	}
	if err := state.open("\r", "HIPPO | Job detail", state.selected); err != nil {
		return err
	}
	for range 200 {
		// Even a saturated scroll must consume the previous physical j before
		// its observation. Help/back restores the exact saved detail offset;
		// the intervening Help frame cannot satisfy the following detail wait.
		if err := state.open("?", "HIPPO | Help", "Help: keyboard actions"); err != nil {
			return err
		}
		if err := state.open("b", "HIPPO | Job detail"); err != nil {
			return err
		}
		if err := state.keyBytes("j"); err != nil {
			return err
		}
	}
	// Also join consumption of the final original j before the last observation.
	if err := state.open("?", "HIPPO | Help", "Help: keyboard actions"); err != nil {
		return err
	}
	return state.open("b", "HIPPO | Job detail")
}

func (state *nativeBDDInteraction) requireUnsafe() error {
	if len(state.frames) < 202 {
		return errors.New("native unsafe dashboard/detail observations are incomplete")
	}
	for _, frame := range state.frames {
		if err := state.bounds(frame); err != nil {
			return err
		}
		content := strings.Join(frame.Rows, "\n")
		if strings.ContainsAny(content, "\x1b\x07\r\t") || strings.Contains(content, "private-payload") {
			return errors.New("unsafe terminal payload escaped native cells")
		}
	}
	return state.fixture.quit()
}

func (state *nativeBDDInteraction) prepareLarge() error {
	snapshot := tui.BDDNativeObserverSnapshot()
	snapshot.Running, snapshot.Queue = nil, nil
	for index := range 1000 {
		snapshot.Running = append(snapshot.Running, application.ObservationRow{Entry: coordination.ReservationEntry{RunID: fmt.Sprintf("%032x", index+1), Source: fmt.Sprintf("run-%04d", index), Class: policy.TaskEphemeral}})
		snapshot.Queue = append(snapshot.Queue, application.ObservationRow{Entry: coordination.ReservationEntry{RunID: fmt.Sprintf("%032x", index+1001), Source: fmt.Sprintf("queue-%04d", index), Position: index + 1, Class: policy.TaskEphemeral}})
	}
	if err := state.prepare("large", snapshot); err != nil {
		return err
	}
	// Given setup acknowledges each physical batch before sending the next.
	for selectedIndex := 0; selectedIndex < 777; {
		count := min(64, 777-selectedIndex)
		if err := state.keyBytes(strings.Repeat("j", count)); err != nil {
			return err
		}
		selectedIndex += count
		if err := state.await(state.view, fmt.Sprintf("Running %d/1000", selectedIndex+1), fmt.Sprintf("> ACTIVE unavailable run-%04d ", selectedIndex)); err != nil {
			return err
		}
	}
	state.selected = fmt.Sprintf("%032x", 778)
	return nil
}

func (state *nativeBDDInteraction) navigateLarge() error {
	for _, size := range [][2]int{{40, 16}, {100, 30}, {60, 20}, {80, 24}} {
		if err := state.resize(size[0], size[1]); err != nil {
			return err
		}
		if err := state.await("HIPPO | Running and Queue", "Running 778/1000", "> ACTIVE unavailable run-0777 "); err != nil {
			return err
		}
		if err := state.identity(state.selected); err != nil {
			return err
		}
	}
	if err := state.keyBytes("nj"); err != nil {
		return err
	}
	return state.await("HIPPO | Running and Queue", "Queue 2/1000", "> WAIT #2 unavailable queue-0001 ")
}

func (state *nativeBDDInteraction) requireLarge() error {
	if err := state.identity(fmt.Sprintf("%032x", 1002)); err != nil {
		return err
	}
	return state.fixture.quit()
}

func (state *nativeBDDInteraction) refreshSnapshot(snapshot application.ObservationSnapshot) error {
	state.fixture.refresh.input.Snapshot = snapshot
	if err := state.fixture.refresh.apply(); err != nil {
		return err
	}
	if err := state.keyBytes("r"); err != nil {
		return err
	}
	return state.fixture.refresh.awaitEvents(func(events []tui.BDDNativeObserverEvent) bool {
		return countNativeEvents(events, "complete") == 2
	})
}

func (state *nativeBDDInteraction) removeSelected() error {
	snapshot := state.fixture.refresh.input.Snapshot
	snapshot.Running = append(snapshot.Running[:1:1], snapshot.Running[2:]...)
	return state.refreshSnapshot(snapshot)
}

func (state *nativeBDDInteraction) requireNearest() error {
	if err := state.await("HIPPO | Running and Queue", "Running 2/2", "> ACTIVE unavailable run-2 "); err != nil {
		return err
	}
	if err := state.identity(fmt.Sprintf("%032x", 3)); err != nil {
		return err
	}
	return state.fixture.quit()
}

func (state *nativeBDDInteraction) prepareDetail() error {
	if err := state.prepare("detail", tui.BDDNativeObserverSnapshot()); err != nil {
		return err
	}
	return state.open("\r", "HIPPO | Job detail", state.selected)
}

func (state *nativeBDDInteraction) removeDetail() error {
	snapshot := state.fixture.refresh.input.Snapshot
	snapshot.Running = nil
	return state.refreshSnapshot(snapshot)
}

func (state *nativeBDDInteraction) requireMissingDetail() error {
	if err := state.await("HIPPO | Job detail", "job no longer active", state.selected); err != nil {
		return err
	}
	if err := state.open("b", "HIPPO | Running and Queue", "No running jobs", "Running 0/0"); err != nil {
		return err
	}
	return state.fixture.quit()
}

func (state *nativeBDDInteraction) prepareEmpty() error {
	snapshot := tui.BDDNativeObserverSnapshot()
	snapshot.Running = nil
	return state.prepare("empty", snapshot)
}

func (state *nativeBDDInteraction) requestDetail() error {
	// The queue round trip is an ordered input barrier that cannot disguise an
	// unexpected detail transition caused by Enter on the empty Running panel.
	if err := state.keyBytes("\r"); err != nil {
		return err
	}
	if err := state.open("n", "HIPPO | Running and Queue", "[Queue]", "> WAIT #3 unavailable queued-job #bbb"); err != nil {
		return err
	}
	return state.open("p", "HIPPO | Running and Queue", "[Running]", "No running jobs", "Running 0/0")
}

func (state *nativeBDDInteraction) requireEmpty() error {
	if err := state.await("HIPPO | Running and Queue", "No running jobs", "Running 0/0"); err != nil {
		return err
	}
	for _, row := range state.fixture.screen.Snapshot().Rows {
		if strings.HasPrefix(row, "> ACTIVE") {
			return errors.New("empty native Running panel fabricated a selection")
		}
	}
	return state.fixture.quit()
}

func (state *nativeBDDInteraction) prepareView(view string) error {
	if err := state.prepare("manual-refresh", tui.BDDNativeObserverSnapshot()); err != nil {
		return err
	}
	switch view {
	case "dashboard":
		return nil
	case "job detail":
		return state.open("\r", "HIPPO | Job detail", state.selected)
	case "capacity detail":
		return state.openCapacity()
	case "help":
		return state.open("?", "HIPPO | Help", "Help: keyboard actions")
	case "minimum-size":
		if err := state.resize(39, 15); err != nil {
			return err
		}
		state.view = "Terminal too small: need 40x16"
		return state.await(state.view)
	default:
		return fmt.Errorf("unknown original manual refresh view %q", view)
	}
}

func (state *nativeBDDInteraction) pressKey(key string) error {
	state.key = key
	if state.kind == "manual-refresh" && key == "r" {
		return state.keyBytes("r")
	}
	if state.kind != "ignored" {
		return fmt.Errorf("native interaction %s does not accept original key %q", state.kind, key)
	}
	encoded := map[string]string{"x": "x", "Ctrl-Space": "\x00", "Ctrl-h": "\x08", "Ctrl-j": "\x0a", "Ctrl-k": "\x0b", "Ctrl-l": "\x0c", "Ctrl-backslash": "\x1c", "Escape": "\x1b[27u"}
	value, ok := encoded[key]
	if !ok {
		return fmt.Errorf("unsupported original ignored key %q", key)
	}
	return state.keyBytes(value)
}

func (state *nativeBDDInteraction) requireManualRefresh() error {
	if state.kind != "manual-refresh" || state.key != "r" {
		return errors.New("manual native refresh has no actual r input")
	}
	if err := state.fixture.refresh.awaitEvents(func(events []tui.BDDNativeObserverEvent) bool {
		return countNativeEvents(events, "complete") == 2
	}); err != nil {
		return err
	}
	if err := state.fixture.refresh.requireCounts(2, 2); err != nil {
		return err
	}
	if err := state.await(state.view); err != nil {
		return err
	}
	return state.fixture.quit()
}

func (state *nativeBDDInteraction) prepareHelp() error {
	return state.prepare("help", tui.BDDNativeObserverSnapshot())
}

func (state *nativeBDDInteraction) openHelp() error {
	if err := state.resize(40, 16); err != nil {
		return err
	}
	return state.open("?", "HIPPO | Help", "Help: keyboard actions")
}

func (state *nativeBDDInteraction) requireHelp() error {
	if err := state.keyBytes(strings.Repeat("j", 8)); err != nil {
		return err
	}
	if err := state.await("HIPPO | Help", "Enter: open selected job detail", "c:"); err != nil {
		return err
	}
	if err := state.keyBytes(strings.Repeat("j", 200)); err != nil {
		return err
	}
	if err := state.await("HIPPO | Help", "Host prefixes"); err != nil {
		return err
	}
	if err := state.requireText("Tab or n", "Shift-Tab or p", "Down or j", "Up or k", "Enter", "c:", "b:", "r refresh", "? help", "q quit"); err != nil {
		return err
	}
	if state.samplingQuit != nil {
		return state.samplingQuit()
	}
	return state.fixture.quit()
}

func (state *nativeBDDInteraction) prepareIgnored() error {
	return state.prepare("ignored", tui.BDDNativeObserverSnapshot())
}

func (state *nativeBDDInteraction) requireIgnored() error {
	if state.kind != "ignored" || state.key == "" {
		return errors.New("native ignored-key row has no actual tested key")
	}
	if err := state.open("n", "HIPPO | Running and Queue", "[Queue]", "> WAIT #3 unavailable queued-job #bbb"); err != nil {
		return err
	}
	if err := state.open("p", "HIPPO | Running and Queue", "[Running]", "Running 1/3", "> ACTIVE unavailable run-0 "); err != nil {
		return err
	}
	if err := state.fixture.refresh.requireCounts(1, 1); err != nil {
		return err
	}
	if err := state.identity(state.selected); err != nil {
		return err
	}
	return state.fixture.quit()
}

func (state *nativeBDDInteraction) prepareNoColor() error {
	if err := state.prepare("no-color", tui.BDDNativeObserverSnapshot()); err != nil {
		return err
	}
	if err := state.resize(80, 24); err != nil {
		return err
	}
	return state.fixture.refresh.failNext()
}

func (state *nativeBDDInteraction) renderNoColor() error {
	if err := state.await("HIPPO | Running and Queue", "ACTIVE", "WAIT", "STALE", "ERROR", ">"); err != nil {
		return err
	}
	return state.open("\r", "HIPPO | Job detail", state.selected, "STALE", "ERROR")
}

func (state *nativeBDDInteraction) requireNoColor() error {
	if err := state.requireText("ACTIVE", "WAIT", "STALE", "ERROR", ">"); err != nil {
		return err
	}
	if err := state.fixture.quit(); err != nil {
		return err
	}
	if len(state.fixture.completed) == 0 {
		return errors.New("native no-color output has no joined capture")
	}
	// The decoder deliberately strips SGR. Inspect the final actual capture.
	result := state.fixture.completed[len(state.fixture.completed)-1]
	if regexp.MustCompile("\x1b\\[[0-9;:]*m").Match(result.Stdout) {
		return errors.New("native no-color output retained ANSI styling")
	}
	return nil
}
