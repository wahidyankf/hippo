package tui_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/tui"
	"github.com/wahidyankf/hippo/tests/contract"
	"github.com/wahidyankf/hippo/tests/support"
)

// This focused selection retains the original quit and signal scenarios and
// their composed handlers. It does not replace the full Integration suite.
func TestTUIBDDNativeLifecycleCanonical(t *testing.T) {
	if os.Getenv("HIPPO_BDD_SUITE") != contract.Integration {
		t.Skip("native lifecycle rows require explicit Integration")
	}
	const (
		quitName   = "Quit works from every TUI surface"
		signalName = "TUI interruption preserves the signal status and terminal state"
	)
	paths := []string{
		nativeCanonicalScenarioPath(t, "terminal.feature", quitName),
		nativeCanonicalScenarioPath(t, "terminal.feature", signalName),
	}
	driver := runNativeCanonicalSelection(t, "", map[string]int{
		quitName:   5,
		signalName: 3,
	}, 8, 43, paths...)
	if len(driver.native.lifecycle.completed) != 8 {
		t.Fatalf("native lifecycle retained %d receipts, want eight", len(driver.native.lifecycle.completed))
	}
	for _, receipt := range driver.native.lifecycle.completed {
		if !receipt.Joined || !receipt.IndependentJobCompleted {
			t.Fatalf("native lifecycle receipt lacks process drain or independent job completion: %+v", receipt)
		}
		data, err := json.Marshal(receipt)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("actual-native-lifecycle %s", data)
	}
}

type nativeLifecycleReceipt struct {
	Identity                support.NativeTUIIdentity    `json:"Identity"`
	Report                  tui.BDDNativeLifecycleReport `json:"Report"`
	Joined                  bool                         `json:"Joined"`
	IndependentJobCompleted bool                         `json:"IndependentJobCompleted"`
}

type nativeBDDLifecycle struct {
	fixture    *nativeBDDViewports
	guardedJob func(context.Context) (func() error, func() error, error)
	probe      func() error
	finish     func() error
	directory  string
	result     *tui.BDDNativeLifecycleReport
	completed  []nativeLifecycleReceipt
}

func (state *nativeBDDLifecycle) handler(pattern string) any {
	handlers := map[string]any{
		`^TUI is displaying (dashboard|job detail|capacity detail|help|minimum-size notice) in an isolated terminal$`: state.prepare,
		`^TUI is observing in an isolated terminal$`:                                                                  func(ctx context.Context) error { return state.prepare(ctx, "dashboard") },
		`^the user presses q$`:                                                state.quit,
		`^TUI receives (raw Ctrl-C|SIGINT|SIGTERM)$`:                          state.interrupt,
		`^TUI exits 0 after cancelling and joining its observation work$`:     state.requireQuit,
		`^TUI exits (\d+) without a HIPPO failure diagnostic$`:                state.requireInterruption,
		`^observation is cancelled and joined before the runner returns$`:     state.requireJoined,
		`^the original terminal modes and foreground ownership are restored$`: state.requireRestoration,
		`^no guarded job is cancelled or retired$`:                            state.requireReadOnly,
	}
	return handlers[pattern]
}

func (state *nativeBDDLifecycle) prepare(ctx context.Context, surface string) error {
	state.fixture.closeSessionContext(ctx)
	state.close()
	if state.fixture.cleanupError != nil {
		return state.fixture.cleanupError
	}
	state.result = nil
	probe, finish, err := state.guardedJob(ctx)
	if err != nil {
		return err
	}
	state.probe, state.finish = probe, finish
	state.directory, err = os.MkdirTemp(os.TempDir(), "hippo-native-input-")
	if err != nil {
		return err
	}
	width, height := 100, 30
	if surface == "minimum-size notice" {
		width, height = 39, 15
	}
	environment := append(os.Environ(), "HIPPO_TEST_NATIVE_TUI_LIFECYCLE_DIR="+state.directory)
	if err := state.fixture.startContext(ctx, width, height, environment); err != nil {
		return err
	}
	if err := state.fixture.renderContext(ctx); err != nil {
		return err
	}
	key, title := "", "HIPPO | Running and Queue"
	switch surface {
	case "job detail":
		key, title = "\r", "HIPPO | Job detail"
	case "capacity detail":
		key, title = "c", "HIPPO | Capacity detail"
	case "help":
		key, title = "?", "HIPPO | Help"
	case "minimum-size notice":
		title = "Terminal too small: need 40x16"
	}
	if key != "" {
		if err := state.send(ctx, []byte(key)); err != nil {
			return err
		}
	}
	if err := state.awaitSurface(ctx, title); err != nil {
		return err
	}
	if err := state.send(ctx, []byte("r")); err != nil {
		return err
	}
	return state.awaitBlocked(ctx)
}

func (state *nativeBDDLifecycle) send(parent context.Context, key []byte) error {
	ctx, cancel := context.WithTimeout(parent, 500*time.Millisecond)
	defer cancel()
	return state.fixture.transport.SendKey(ctx, key)
}

func (state *nativeBDDLifecycle) awaitSurface(parent context.Context, title string) error {
	ctx, cancel := context.WithTimeout(parent, 500*time.Millisecond)
	defer cancel()
	_, err := awaitBDDNativeTransportFrame(ctx, bddNativeTransportCapture{transport: state.fixture.transport, screen: state.fixture.screen}, func(frame tui.BDDNativeTransportFrame) bool {
		return frame.Alternate && strings.Contains(strings.Join(frame.Rows, "\n"), title)
	})
	return err
}

func (state *nativeBDDLifecycle) readReport(filename string) (tui.BDDNativeLifecycleReport, error) {
	var report tui.BDDNativeLifecycleReport
	data, err := os.ReadFile(filepath.Join(state.directory, filename))
	if err != nil {
		return report, err
	}
	if len(data) > 1024*1024 {
		return report, errors.New("native lifecycle report exceeds its bound")
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return report, err
	}
	return report, nil
}

func (state *nativeBDDLifecycle) awaitBlocked(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		report, err := state.readReport("lifecycle-progress.json")
		if err == nil {
			if report.Requests != 2 || report.Completed != 1 || report.Active != 1 || report.Cancelled != 0 || len(report.GoroutinesBefore) != 4 {
				return fmt.Errorf("production lifecycle did not start the expected real work: %+v", report)
			}
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("production lifecycle did not reach blocked observation: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (state *nativeBDDLifecycle) quit() error { return state.stopSession("q") }

func (state *nativeBDDLifecycle) interrupt(value string) error { return state.stopSession(value) }

func (state *nativeBDDLifecycle) stopSession(action string) error {
	if state.fixture.transport == nil {
		return errors.New("native lifecycle has no prepared terminal")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var inputError error
	switch action {
	case "q":
		inputError = state.fixture.transport.SendKey(ctx, []byte("q"))
	case "raw Ctrl-C":
		inputError = state.fixture.transport.SendKey(ctx, []byte{3})
	case "SIGINT":
		inputError = state.fixture.transport.Signal(state.fixture.transport.Result().Identity.PID, syscall.SIGINT)
	case "SIGTERM":
		inputError = state.fixture.transport.Signal(state.fixture.transport.Result().Identity.PID, syscall.SIGTERM)
	default:
		return fmt.Errorf("unknown native lifecycle input %q", action)
	}
	if inputError != nil {
		return inputError
	}
	result, err := state.fixture.transport.WaitAndDrain(ctx)
	expected := 0
	switch action {
	case "raw Ctrl-C", "SIGINT":
		expected = 130
	case "SIGTERM":
		expected = 143
	}
	if err != nil && !nativeLifecycleExpectedExit(err, result.Identity.PID, expected) {
		return err
	}
	if !result.Joined() {
		return fmt.Errorf("native lifecycle did not reap and drain: %+v", result)
	}
	report, err := state.readReport("lifecycle-report.json")
	if err != nil {
		return err
	}
	if report.ExitCode != result.ExitCode {
		return fmt.Errorf("actual CLI and process statuses differ: %d/%d", report.ExitCode, result.ExitCode)
	}
	state.result = &report
	state.completed = append(state.completed, nativeLifecycleReceipt{Identity: result.Identity, Report: report, Joined: result.Joined()})
	state.fixture.completed = append(state.fixture.completed, result)
	closeError := state.fixture.transport.Close(ctx)
	if closeError != nil && !nativeLifecycleExpectedExit(closeError, result.Identity.PID, expected) {
		state.fixture.cleanupError = errors.Join(state.fixture.cleanupError, closeError)
	}
	state.fixture.transport = nil
	if state.fixture.cancel != nil {
		state.fixture.cancel()
		state.fixture.cancel = nil
	}
	return state.fixture.cleanupError
}

// The transport retains the actual process ExitError. An intended signal code
// is an outcome, while every other joined transport/decoder error stays fatal.
func nativeLifecycleExpectedExit(err error, pid, expected int) bool {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !nativeLifecycleExpectedExit(child, pid, expected) {
				return false
			}
		}
		return true
	}
	if errors.Unwrap(err) != nil {
		return false
	}
	exit, ok := errors.AsType[*exec.ExitError](err)
	return ok && expected != 0 && exit.ProcessState != nil && exit.Pid() == pid && exit.ExitCode() == expected
}

func (state *nativeBDDLifecycle) requireQuit() error { return state.requireInterruption(0) }

func (state *nativeBDDLifecycle) requireInterruption(code int) error {
	if err := state.requireJoined(); err != nil {
		return err
	}
	completed := state.fixture.completed
	if len(completed) == 0 || state.result.ExitCode != code || len(completed[len(completed)-1].Stderr) != 0 {
		return fmt.Errorf("native lifecycle did not preserve exit %d without diagnostics", code)
	}
	return nil
}

func (state *nativeBDDLifecycle) requireJoined() error {
	report := state.result
	if report == nil || !report.RunnerReturned || report.Requests != 2 || report.Completed != 1 || report.Cancelled != 1 || report.Active != 0 || len(report.GoroutinesBefore) != 4 || len(report.GoroutinesAfter) != 0 {
		return fmt.Errorf("production Run returned without actual cancellation and work disappearance: %+v", report)
	}
	return nil
}

func (state *nativeBDDLifecycle) requireRestoration() error {
	if state.result == nil || len(state.result.Before) != 2 || !reflect.DeepEqual(state.result.Before, state.result.After) {
		return fmt.Errorf("production Run did not restore its physical descriptor modes and foreground: %+v", state.result)
	}
	pid := state.fixture.completed[len(state.fixture.completed)-1].Identity.PID
	for _, mode := range state.result.After {
		if mode.Foreground != pid {
			return errors.New("native lifecycle foreground is not the original owned child")
		}
	}
	return nil
}

func (state *nativeBDDLifecycle) requireReadOnly() error {
	if state.probe == nil || state.finish == nil {
		return errors.New("native lifecycle has no independent guarded job")
	}
	if err := state.probe(); err != nil {
		return err
	}
	err := state.finish()
	if err == nil {
		state.completed[len(state.completed)-1].IndependentJobCompleted = true
	}
	state.finish, state.probe = nil, nil
	return err
}

func (state *nativeBDDLifecycle) close() {
	if state.finish != nil {
		if err := state.finish(); err != nil {
			state.fixture.cleanupError = errors.Join(state.fixture.cleanupError, err)
		} else {
			state.finish, state.probe = nil, nil
		}
	}
	if state.directory != "" {
		if err := os.RemoveAll(state.directory); err != nil {
			state.fixture.cleanupError = errors.Join(state.fixture.cleanupError, err)
		} else {
			state.directory = ""
		}
	}
}
