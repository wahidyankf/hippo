package tui_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/tui"
	"github.com/wahidyankf/hippo/internal/application"
)

// Inputs control the actual observer and clock, while assertions read real PTY
// cells and the real worker's journal. No handler calls a private model method.
type nativeBDDRefresh struct {
	fixture     *nativeBDDViewports
	observation func(context.Context, string) (application.ObservationSnapshot, error)
	directory   string
	input       tui.BDDNativeObserverInput
	finalEvents []tui.BDDNativeObserverEvent
	initialID   string
}

func (fixture *nativeBDDViewports) refreshHandler(pattern string) any {
	state := fixture.refresh
	if state == nil {
		return nil
	}
	handlers := map[string]any{
		`^a TUI whose previous observation has just completed$`:                                               state.prepareCompleted,
		`^five seconds elapse after that completion$`:                                                         state.elapseRefresh,
		`^exactly one new observation starts and no observation starts before that deadline$`:                 state.requireDeadline,
		`^one blocked TUI observation$`:                                                                       state.prepareBlocked,
		`^refresh is requested three times before that observation completes$`:                                state.requestThree,
		`^no second observation overlaps it and exactly one immediate follow-up starts after completion$`:     state.requireCoalescing,
		`^one blocked TUI observation and a selected running job$`:                                            state.prepareBusySelection,
		`^the operator switches section and resizes to 40 by 16 cells$`:                                       state.navigateBusy,
		`^the section and compact layout change before the observation is released$`:                          state.requireNavigation,
		`^a TUI without a successful observation$`:                                                            state.prepareBlocked,
		`^its first observation fails$`:                                                                       state.failInitial,
		`^the display shows unavailable data and a safe error summary with refresh still accessible$`:         state.requireInitialFailure,
		`^a successful TUI snapshot with one owner and known allocated resources$`:                            state.prepareOwner,
		`^the next observation fails$`:                                                                        state.failNext,
		`^the same owner and allocation remain visible with STALE sample age and a safe error summary$`:       state.requireStale,
		`^a stale TUI snapshot after an observation failure$`:                                                 state.prepareStale,
		`^a later observation succeeds$`:                                                                      state.recover,
		`^the new snapshot replaces the old data and STALE and ERROR markers disappear$`:                      state.requireRecovery,
		`^a successful observation whose second host reading was measured ten seconds ago$`:                   state.prepareAged,
		`^one lightweight age tick elapses before the next refresh deadline$`:                                 state.elapseAge,
		`^the displayed sample age advances and the host collector is not called again$`:                      state.requireAge,
		`^a successful observation whose second host measurement time is (absent|malformed)$`:                 state.prepareUnknown,
		`^the TUI displays its sample age$`:                                                                   state.renderAge,
		`^sample age is unavailable instead of using refresh completion time$`:                                state.requireUnknown,
		`^the operator quits and the collector receives cancellation$`:                                        state.quitBusy,
		`^the TUI exits after its observation and timing work finish and no late result updates its display$`: state.requireLateJoined,
	}
	return handlers[pattern]
}

func nativeObserverTime(second int) time.Time { return time.Unix(int64(second), 0).UTC() }

func (state *nativeBDDRefresh) prepare(snapshot application.ObservationSnapshot, blocked bool, now time.Time) error {
	return state.prepareViewport(snapshot, blocked, now, 100, 30)
}

func (state *nativeBDDRefresh) prepareViewport(snapshot application.ObservationSnapshot, blocked bool, now time.Time, width, height int) error {
	state.fixture.closeSession()
	if state.fixture.cleanupError != nil {
		return state.fixture.cleanupError
	}
	directory, err := os.MkdirTemp(os.TempDir(), "hippo-native-input-")
	if err != nil {
		return err
	}
	state.directory = directory
	state.finalEvents = nil
	state.input = tui.BDDNativeObserverInput{Sequence: 1, Now: now, Snapshot: snapshot, Blocked: blocked}
	if err := state.writeInput(); err != nil {
		return err
	}
	environment := append(os.Environ(), "HIPPO_TEST_NATIVE_TUI_INPUT_DIR="+directory)
	if err := state.fixture.start(width, height, environment); err != nil {
		return err
	}
	return state.awaitEvents(func(events []tui.BDDNativeObserverEvent) bool { return countNativeEvents(events, "request") == 1 })
}

func (state *nativeBDDRefresh) writeInput() error {
	data, err := json.Marshal(state.input)
	if err != nil {
		return err
	}
	temporary := filepath.Join(state.directory, "next-input.json")
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, filepath.Join(state.directory, "input.json"))
}

func (state *nativeBDDRefresh) apply() error {
	state.input.Sequence++
	if err := state.writeInput(); err != nil {
		return err
	}
	return state.awaitEvents(func(events []tui.BDDNativeObserverEvent) bool {
		for _, event := range events {
			if event.Kind == "input applied" && event.Sequence == state.input.Sequence {
				return true
			}
		}
		return false
	})
}

func (state *nativeBDDRefresh) events(final bool) ([]tui.BDDNativeObserverEvent, error) {
	data, err := os.ReadFile(filepath.Join(state.directory, "events.jsonl"))
	if err != nil {
		return nil, err
	}
	if len(data) > 1024*1024 {
		return nil, errors.New("native observer journal exceeded its limit")
	}
	parts := bytes.Split(data, []byte{'\n'})
	if final && len(parts[len(parts)-1]) != 0 {
		return nil, errors.New("native observer journal has incomplete final event")
	}
	var events []tui.BDDNativeObserverEvent
	for _, line := range parts[:len(parts)-1] {
		var event tui.BDDNativeObserverEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func countNativeEvents(events []tui.BDDNativeObserverEvent, kind string) int {
	count := 0
	for _, event := range events {
		if event.Kind == kind {
			count++
		}
	}
	return count
}

func (state *nativeBDDRefresh) awaitEvents(predicate func([]tui.BDDNativeObserverEvent) bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	var last []tui.BDDNativeObserverEvent
	for {
		events, err := state.events(false)
		if err == nil {
			last = events
			if predicate(events) {
				return nil
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("native observer predicate timed out: %w; events=%+v", ctx.Err(), last)
		case <-ticker.C:
		}
	}
}

func (state *nativeBDDRefresh) frame(predicate func(string) bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := awaitBDDNativeTransportFrame(ctx, bddNativeTransportCapture{transport: state.fixture.transport, screen: state.fixture.screen}, func(frame tui.BDDNativeTransportFrame) bool {
		return frame.Alternate && frame.Width == state.fixture.width && frame.Height == state.fixture.height && frame.OverflowWrites == 0 && predicate(strings.Join(frame.Rows, "\n"))
	})
	return err
}

func (state *nativeBDDRefresh) key(value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	return state.fixture.transport.SendKey(ctx, []byte(value))
}

func (state *nativeBDDRefresh) requireCounts(requests, completed int) error {
	events, err := state.events(false)
	if err != nil {
		return err
	}
	if countNativeEvents(events, "request") != requests || countNativeEvents(events, "complete") != completed {
		return fmt.Errorf("native worker count differs: want requests=%d completed=%d actual=%+v", requests, completed, events)
	}
	active := 0
	for _, event := range events {
		switch event.Kind {
		case "request":
			active++
			if active != 1 {
				return errors.New("native observations overlapped")
			}
		case "complete":
			active--
			if active != 0 {
				return errors.New("native observation completed without an active request")
			}
		}
	}
	return nil
}

func (state *nativeBDDRefresh) prepareCompleted() error {
	if err := state.prepare(tui.BDDNativeObserverSnapshot(), false, nativeObserverTime(100)); err != nil {
		return err
	}
	return state.frame(func(content string) bool {
		return strings.Contains(content, "LIVE") && strings.Contains(content, "Running 1/3")
	})
}

func (state *nativeBDDRefresh) prepareBlocked() error {
	return state.prepare(tui.BDDNativeObserverSnapshot(), true, nativeObserverTime(100))
}

func (state *nativeBDDRefresh) elapseRefresh() error {
	// Observe a genuine pre-deadline clock render before releasing the deadline.
	state.input.Now = nativeObserverTime(105).Add(-time.Millisecond)
	if err := state.apply(); err != nil {
		return err
	}
	if err := state.frame(func(content string) bool { return strings.Contains(content, "age 4s") }); err != nil {
		return err
	}
	if err := state.requireCounts(1, 1); err != nil {
		return err
	}
	state.input.Now = nativeObserverTime(105)
	if err := state.apply(); err != nil {
		return err
	}
	return state.awaitEvents(func(events []tui.BDDNativeObserverEvent) bool { return countNativeEvents(events, "complete") == 2 })
}

func (state *nativeBDDRefresh) requireDeadline() error {
	if err := state.requireCounts(2, 2); err != nil {
		return err
	}
	events, err := state.events(false)
	if err != nil {
		return err
	}
	var firstComplete, secondRequest time.Time
	for _, event := range events {
		if event.Kind == "complete" && event.Request == 1 {
			firstComplete = event.Now
		}
		if event.Kind == "request" && event.Request == 2 {
			secondRequest = event.Now
		}
	}
	if !secondRequest.Equal(firstComplete.Add(5 * time.Second)) {
		return fmt.Errorf("native deadline is not completion+5s: %+v", events)
	}
	return state.fixture.quit()
}

func (state *nativeBDDRefresh) requestThree() error {
	if err := state.key("rrr?"); err != nil {
		return err
	}
	// Help is an ordered input-loop barrier after all three refresh keys.
	return state.frame(func(content string) bool { return strings.Contains(content, "Help: keyboard actions") })
}

func (state *nativeBDDRefresh) requireCoalescing() error {
	if err := state.requireCounts(1, 0); err != nil {
		return err
	}
	state.input.Blocked = false
	if err := state.apply(); err != nil {
		return err
	}
	if err := state.awaitEvents(func(events []tui.BDDNativeObserverEvent) bool { return countNativeEvents(events, "complete") == 2 }); err != nil {
		return err
	}
	if err := state.requireCounts(2, 2); err != nil {
		return err
	}
	return state.fixture.quit()
}

func (state *nativeBDDRefresh) prepareBusySelection() error {
	if err := state.prepareCompleted(); err != nil {
		return err
	}
	state.input.Blocked = true
	if err := state.apply(); err != nil {
		return err
	}
	if err := state.key("r"); err != nil {
		return err
	}
	return state.awaitEvents(func(events []tui.BDDNativeObserverEvent) bool { return countNativeEvents(events, "request") == 2 })
}

func (state *nativeBDDRefresh) navigateBusy() error {
	if err := state.key("n"); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	state.fixture.width, state.fixture.height = 40, 16
	return state.fixture.transport.Resize(ctx, 40, 16)
}

func (state *nativeBDDRefresh) requireNavigation() error {
	if err := state.frame(func(content string) bool {
		return strings.Contains(content, "[Queue]") && strings.Contains(content, "Queue 1/1") && strings.Contains(content, "refreshing")
	}); err != nil {
		return err
	}
	if err := state.requireCounts(2, 1); err != nil {
		return err
	}
	state.input.Blocked = false
	if err := state.apply(); err != nil {
		return err
	}
	if err := state.awaitEvents(func(events []tui.BDDNativeObserverEvent) bool { return countNativeEvents(events, "complete") == 2 }); err != nil {
		return err
	}
	return state.fixture.quit()
}

func (state *nativeBDDRefresh) failInitial() error {
	state.input.Blocked, state.input.Failed = false, true
	if err := state.apply(); err != nil {
		return err
	}
	return state.frame(func(content string) bool {
		return strings.Contains(content, "ERROR") && strings.Contains(content, "No observation")
	})
}

func (state *nativeBDDRefresh) requireInitialFailure() error {
	if err := state.frame(func(content string) bool {
		return strings.Contains(content, "unavailable") && strings.Contains(content, "Observation unavailable") && !strings.Contains(content, "/private/") && !strings.Contains(content, "Running 0")
	}); err != nil {
		return err
	}
	if err := state.key("r"); err != nil {
		return err
	}
	if err := state.awaitEvents(func(events []tui.BDDNativeObserverEvent) bool { return countNativeEvents(events, "complete") == 2 }); err != nil {
		return err
	}
	return state.fixture.quit()
}

func (state *nativeBDDRefresh) prepareOwner() error {
	snapshot := tui.BDDNativeObserverSnapshot()
	snapshot.Running = snapshot.Running[:1]
	snapshot.Status.Coordination.ActiveOwners = 1
	state.initialID = snapshot.Running[0].Entry.RunID
	if err := state.prepare(snapshot, false, nativeObserverTime(100)); err != nil {
		return err
	}
	return state.frame(func(content string) bool {
		return strings.Contains(content, "LIVE") && strings.Contains(content, "Running 1/1")
	})
}

func (state *nativeBDDRefresh) failNext() error {
	state.input.Failed = true
	if err := state.apply(); err != nil {
		return err
	}
	if err := state.key("r"); err != nil {
		return err
	}
	return state.frame(func(content string) bool {
		return strings.Contains(content, "STALE") && strings.Contains(content, "ERROR")
	})
}

func (state *nativeBDDRefresh) requireStale() error {
	if err := state.key("\r"); err != nil {
		return err
	}
	if err := state.frame(func(content string) bool {
		return strings.Contains(content, state.initialID) && strings.Contains(content, "Allocated: 2CPU") && strings.Contains(content, "1.0GiB") && strings.Contains(content, "STALE") && strings.Contains(content, "age") && strings.Contains(content, "Observation unavailable") && !strings.Contains(content, "/private/")
	}); err != nil {
		return err
	}
	return state.fixture.quit()
}

func (state *nativeBDDRefresh) prepareStale() error {
	if err := state.prepareOwner(); err != nil {
		return err
	}
	return state.failNext()
}

func (state *nativeBDDRefresh) recover() error {
	state.input.Failed = false
	state.input.Snapshot.Running[0].Entry.RunID = "replacement-owner"
	if err := state.apply(); err != nil {
		return err
	}
	if err := state.key("r"); err != nil {
		return err
	}
	return state.frame(func(content string) bool {
		return strings.Contains(content, "LIVE") && !strings.Contains(content, "STALE") && !strings.Contains(content, "ERROR")
	})
}

func (state *nativeBDDRefresh) requireRecovery() error {
	if err := state.key("\r"); err != nil {
		return err
	}
	if err := state.frame(func(content string) bool {
		return strings.Contains(content, "replacement-owner") && !strings.Contains(content, "STALE") && !strings.Contains(content, "ERROR")
	}); err != nil {
		return err
	}
	return state.fixture.quit()
}

func (state *nativeBDDRefresh) prepareAged() error {
	if err := state.prepare(tui.BDDNativeObserverSnapshot(), false, nativeObserverTime(110)); err != nil {
		return err
	}
	return state.frame(func(content string) bool { return strings.Contains(content, "age 10s") })
}

func (state *nativeBDDRefresh) elapseAge() error {
	state.input.Now = nativeObserverTime(111)
	return state.apply()
}

func (state *nativeBDDRefresh) requireAge() error {
	if err := state.frame(func(content string) bool { return strings.Contains(content, "age 11s") }); err != nil {
		return err
	}
	if err := state.requireCounts(1, 1); err != nil {
		return err
	}
	return state.fixture.quit()
}

func (state *nativeBDDRefresh) prepareUnknown(value string) error {
	if state.observation == nil {
		return errors.New("native sample time has no real observation factory")
	}
	snapshot, err := state.observation(context.Background(), value)
	if err != nil {
		return err
	}
	return state.prepare(snapshot, false, nativeObserverTime(100))
}

func (state *nativeBDDRefresh) renderAge() error {
	return state.frame(func(content string) bool {
		return strings.Contains(content, "LIVE") && strings.Contains(content, "age unavailable")
	})
}

func (state *nativeBDDRefresh) requireUnknown() error {
	if err := state.renderAge(); err != nil {
		return err
	}
	if err := state.requireCounts(1, 1); err != nil {
		return err
	}
	return state.fixture.quit()
}

func (state *nativeBDDRefresh) quitBusy() error {
	state.input.LateOnCancel = true
	state.input.Snapshot.Running[0].Entry.Source = "late-should-not-render"
	if err := state.apply(); err != nil {
		return err
	}
	return state.fixture.quit()
}

func (state *nativeBDDRefresh) requireJoinedJournal() error {
	events, err := state.events(true)
	if err != nil {
		return err
	}
	if len(events) == 0 || events[len(events)-1].Kind != "control joined" || events[len(events)-1].Failed {
		return fmt.Errorf("native controls did not join cleanly: %+v", events)
	}
	if countNativeEvents(events, "program returned") != 1 {
		return fmt.Errorf("native program termination is missing: %+v", events)
	}
	state.finalEvents = events
	return nil
}

func (state *nativeBDDRefresh) requireLateJoined() error {
	positions := map[string]int{}
	for index, event := range state.finalEvents {
		positions[event.Kind] = index
	}
	cancelled, cancelOK := positions["cancelled"]
	returned, returnOK := positions["program returned"]
	late, lateOK := positions["late returned"]
	joined, joinOK := positions["control joined"]
	if !cancelOK || !returnOK || !lateOK || !joinOK || late <= cancelled || late <= returned || joined <= late {
		return fmt.Errorf("late result or owned joins not demonstrated: %+v", state.finalEvents)
	}
	if len(state.fixture.completed) == 0 {
		return errors.New("busy native quit has no terminal receipt")
	}
	receipt := state.fixture.completed[len(state.fixture.completed)-1]
	if !receipt.Joined() || receipt.ExitCode != 0 || bytes.Contains(receipt.Stdout, []byte("late-should-not-render")) {
		return errors.New("late observation changed terminal output or failed teardown")
	}
	return nil
}
