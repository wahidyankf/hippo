package tui_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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

// This focused selection retains the six original fault, precedence and JSON
// rows. The full composed registry is verified before selecting these rows.
func TestTUIBDDNativeFaultCanonical(t *testing.T) {
	if os.Getenv("HIPPO_BDD_SUITE") != contract.Integration {
		t.Skip("native terminal fault rows require explicit Integration")
	}
	const (
		faultName      = "Terminal failures restore ownership before reporting failure"
		precedenceName = "A classified terminal failure takes precedence over an interruption"
		jsonName       = "Global JSON output formats a TUI refusal after terminal restoration"
	)
	paths := []string{
		nativeCanonicalScenarioPath(t, "terminal.feature", faultName),
		nativeCanonicalScenarioPath(t, "terminal.feature", precedenceName),
		nativeCanonicalScenarioPath(t, "public-cli.feature", jsonName),
	}
	driver := runNativeCanonicalSelection(t, "", map[string]int{
		faultName:      4,
		precedenceName: 1,
		jsonName:       1,
	}, 6, 29, paths...)
	if len(driver.faults.completed) != 6 {
		t.Fatalf("native faults retained %d receipts, want six", len(driver.faults.completed))
	}
	for _, receipt := range driver.faults.completed {
		data, err := json.Marshal(receipt.jsonReceipt())
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("actual-native-fault %s", data)
	}
}

type nativeFaultReceipt struct {
	Identity             support.NativeTUIIdentity
	Report               tui.BDDNativeFaultReport
	Joined               bool
	Stdout, Stderr       []byte
	Result               support.NativeTUIResult
	TransportCloseReturn string
}

type nativeFaultJSONResult struct {
	Identity  support.NativeTUIIdentity `json:"Identity"`
	Stdout    []byte                    `json:"Stdout"`
	Stderr    []byte                    `json:"Stderr"`
	ExitCode  int                       `json:"ExitCode"`
	Reaped    bool                      `json:"Reaped"`
	OutputEOF bool                      `json:"OutputEOF"`
	ErrorEOF  bool                      `json:"ErrorEOF"`
	ReadyEOF  bool                      `json:"ReadyEOF"`
}

type nativeFaultJSONReceipt struct {
	Identity             support.NativeTUIIdentity `json:"Identity"`
	Report               tui.BDDNativeFaultReport  `json:"Report"`
	Joined               bool                      `json:"Joined"`
	Stdout               []byte                    `json:"Stdout"`
	Stderr               []byte                    `json:"Stderr"`
	Result               nativeFaultJSONResult     `json:"Result"`
	TransportCloseReturn string                    `json:"TransportCloseReturn"`
}

func (receipt nativeFaultReceipt) jsonReceipt() nativeFaultJSONReceipt {
	result := receipt.Result
	return nativeFaultJSONReceipt{
		Identity: receipt.Identity, Report: receipt.Report, Joined: receipt.Joined,
		Stdout: receipt.Stdout, Stderr: receipt.Stderr, TransportCloseReturn: receipt.TransportCloseReturn,
		Result: nativeFaultJSONResult{
			Identity: result.Identity, Stdout: result.Stdout, Stderr: result.Stderr,
			ExitCode: result.ExitCode, Reaped: result.Reaped,
			OutputEOF: result.OutputEOF, ErrorEOF: result.ErrorEOF, ReadyEOF: result.ReadyEOF,
		},
	}
}

type nativeBDDFaults struct {
	fixture          *nativeBDDViewports
	transport        *support.NativeTUITransport
	directory, fault string
	result           *tui.BDDNativeFaultReport
	completed        []nativeFaultReceipt
}

func (state *nativeBDDFaults) handler(pattern string) any {
	handlers := map[string]any{
		`^TUI encounters a (startup|input read|output write|restoration) failure in an isolated terminal$`: state.prepare,
		`^the terminal session finishes$`:                                         state.finish,
		`^it exits 125 with one privacy-safe HIPPO diagnostic$`:                   state.requireFailure,
		`^terminal restoration is attempted before the diagnostic is written$`:    state.requireRestoration,
		`^its observation work and timers do not survive the runner$`:             state.requireJoined,
		`^an interrupted TUI whose terminal restoration fails$`:                   func(ctx context.Context) error { return state.prepare(ctx, "restoration-SIGTERM") },
		`^it exits 125 for the restoration failure instead of the signal status$`: state.requireFailure,
		`^restoration is attempted before the diagnostic is written$`:             state.requireRestoration,
		`^a TUI terminal whose session can be restored$`:                          func(ctx context.Context) error { return state.prepare(ctx, "JSON") },
		`^TUI rendering fails with global JSON output requested$`:                 state.finish,
		`^the terminal is restored before the failure is written$`:                state.requireRestoration,
		`^the only machine-readable failure body names hippo tui with exit 125$`:  state.requireJSON,
		`^TUI frames are not serialized as JSON status records$`:                  state.requireNoJSONFrames,
	}
	return handlers[pattern]
}

func (state *nativeBDDFaults) prepare(parent context.Context, fault string) error {
	state.fixture.closeSessionContext(parent)
	state.close()
	if state.fixture.cleanupError != nil {
		return state.fixture.cleanupError
	}
	state.fault, state.result = fault, nil
	var err error
	state.directory, err = os.MkdirTemp(os.TempDir(), "hippo-native-input-")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	state.fixture.cancel, state.fixture.width, state.fixture.height = cancel, 100, 30
	state.fixture.screen = tui.NewBDDNativeTransportScreen(100, 30)
	environment := append(os.Environ(), "HIPPO_TEST_NATIVE_TUI_FAULT_DIR="+state.directory, "HIPPO_TEST_NATIVE_TUI_FAULT="+fault)
	state.fixture.transport, err = support.StartNativeTUI(ctx, support.NativeTUIStart{
		Mode: contract.Integration, Executable: os.Args[0], Helper: support.NativeTUITeaHelper,
		Environment: environment, Width: 100, Height: 30,
		Feed: state.fixture.screen.Feed, ResizeScreen: state.fixture.screen.Resize, Finish: state.fixture.screen.Finish,
	})
	if err != nil {
		cancel()
		return err
	}
	state.transport = state.fixture.transport
	// Cold failures authenticate the raw-mode readiness record during the final
	// drain, which permits the genuine child to have already exited naturally.
	if fault == "startup" || fault == "JSON" || fault == "input read" {
		return nil
	}
	if err := state.fixture.transport.AwaitReady(ctx); err != nil {
		return err
	}
	if err := state.fixture.renderContext(ctx); err != nil {
		return err
	}
	lifecycle := nativeBDDLifecycle{fixture: state.fixture, directory: state.directory}
	if err := lifecycle.send(ctx, []byte("r")); err != nil {
		return err
	}
	return lifecycle.awaitBlocked(ctx)
}

func (state *nativeBDDFaults) finish(parent context.Context) error {
	if state.fixture.transport == nil {
		return errors.New("native fault terminal was not prepared")
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	transport := state.fixture.transport
	switch state.fault {
	case "input read":
		// A successful live readiness check permits a real input byte to wake the
		// OS-backed reader. A naturally exited child is qualified by WaitAndDrain.
		if err := transport.AwaitReady(ctx); err == nil {
			if err := transport.SendKey(ctx, []byte("x")); err != nil {
				return err
			}
		}
	case "output write":
		cleanup, inputError := state.finishOutput(ctx, transport)
		if cleanup != nil {
			defer cleanup()
		}
		if inputError != nil {
			return inputError
		}
	case "restoration":
		if err := transport.SendKey(ctx, []byte("q")); err != nil {
			return err
		}
	case "restoration-SIGTERM":
		if err := transport.Signal(transport.Result().Identity.PID, syscall.SIGTERM); err != nil {
			return err
		}
	}
	result, drainError := transport.WaitAndDrain(ctx)
	if drainError != nil && !nativeLifecycleExpectedExit(drainError, result.Identity.PID, 125) {
		return drainError
	}
	if !result.Joined() {
		return fmt.Errorf("native fault did not naturally reap and drain: %+v", result)
	}
	data, err := os.ReadFile(filepath.Join(state.directory, "fault-report.json"))
	if err != nil {
		return err
	}
	if len(data) > 1024*1024 {
		return errors.New("native fault report exceeds its bound")
	}
	var report tui.BDDNativeFaultReport
	if err := json.Unmarshal(data, &report); err != nil {
		return err
	}
	state.result = &report
	state.completed = append(state.completed, nativeFaultReceipt{Identity: result.Identity, Report: report, Joined: result.Joined(), Stdout: result.Stdout, Stderr: result.Stderr, Result: result})
	state.fixture.completed = append(state.fixture.completed, result)
	closeError := transport.Close(ctx)
	if closeError != nil {
		state.completed[len(state.completed)-1].TransportCloseReturn = closeError.Error()
	}
	if closeError != nil && !nativeLifecycleExpectedExit(closeError, result.Identity.PID, 125) {
		state.fixture.cleanupError = errors.Join(state.fixture.cleanupError, closeError)
	}
	state.fixture.transport = nil
	if state.fixture.cancel != nil {
		state.fixture.cancel()
		state.fixture.cancel = nil
	}
	if report.ExitCode != result.ExitCode || report.Fault != state.fault {
		return fmt.Errorf("actual native fault report/process differ: %+v/%+v", report, result)
	}
	return state.fixture.cleanupError
}

func (state *nativeBDDFaults) requireJoined() error {
	report := state.result
	if report == nil || !report.RunnerReturned || report.Active != 0 || len(report.GoroutinesAfter) != 0 {
		return fmt.Errorf("native fault work survived production Run: %+v", report)
	}
	if state.fault == "output write" || strings.HasPrefix(state.fault, "restoration") {
		if report.Requests != 2 || report.Completed != 1 || report.Cancelled != 1 || len(report.GoroutinesBefore) != 4 {
			return fmt.Errorf("native fault did not cancel actual blocked work: %+v", report)
		}
	}
	return nil
}

func (state *nativeBDDFaults) requireFailure() error {
	if err := state.requireJoined(); err != nil {
		return err
	}
	receipt := state.completed[len(state.completed)-1]
	diagnostic := string(receipt.Stderr)
	if receipt.Report.ExitCode != 125 || strings.Count(diagnostic, "hippo.supervision.failed") != 1 || strings.Contains(diagnostic, "/private/") || strings.Contains(diagnostic, "/dev/") || strings.Contains(diagnostic, state.directory) {
		return fmt.Errorf("native fault did not produce one privacy-safe exit125 diagnostic: %d/%q", receipt.Report.ExitCode, diagnostic)
	}
	if len(strings.Split(strings.TrimSpace(diagnostic), "\n")) != 1 {
		return fmt.Errorf("native terminal failure produced multiple diagnostic lines: %q", diagnostic)
	}
	return state.requireCause()
}

func (state *nativeBDDFaults) requireCause() error {
	if state.result == nil {
		return errors.New("native fault has no actual report")
	}
	wanted := state.wantedOperation()
	failed, swap, closed, returned, diagnostic := -1, -1, -1, -1, -1
	positiveWrite := false
	for index, operation := range state.result.Operations {
		if operation.Kind == "write" && operation.Bytes > 0 && operation.Error == "" {
			positiveWrite = true
		}
		if operation.Kind == wanted && operation.Errno == int(syscall.EBADF) && operation.Bytes == 0 && operation.Error != "" && failed < 0 {
			failed = index
		}
		switch operation.Kind {
		case "owned-output-fd-readonly-swap":
			if operation.Error == "" {
				swap = index
			}
		case "owned-run-input-clone-close":
			if operation.Error == "" {
				closed = index
			}
		case "production-Run-returned":
			returned = index
		case "first-cli-diagnostic":
			diagnostic = index
		}
	}
	if failed < 0 || returned <= failed || diagnostic <= returned {
		return fmt.Errorf("native fault lacks actual EBADF/Run/diagnostic order: %+v", state.result.Operations)
	}
	if state.fault == "output write" && (!positiveWrite || swap < 0 || swap >= failed) {
		return errors.New("late output failure lacks positive-frame write before owned fd swap and actual EBADF")
	}
	return state.requireRestoreCause(closed, failed)
}

func (state *nativeBDDFaults) wantedOperation() string {
	wanted := "write"
	if state.fault == "input read" {
		wanted = "read"
	}
	if strings.HasPrefix(state.fault, "restoration") {
		wanted = "closed-run-input-termios"
	}
	return wanted
}

func (state *nativeBDDFaults) requireRestoreCause(closed, failed int) error {
	if strings.HasPrefix(state.fault, "restoration") && (closed < 0 || closed >= failed || !reflect.DeepEqual(state.result.Before, state.result.RestoredBeforeFault) || !strings.Contains(state.result.RunError, "cannot restore TUI terminal")) {
		return fmt.Errorf("restoration fault lacks actual physical restoration, closed clone and production classification: %+v", state.result)
	}
	return nil
}

func (state *nativeBDDFaults) requireRestoration() error {
	if err := state.requireJoined(); err != nil {
		return err
	}
	if err := state.requireCause(); err != nil {
		return err
	}
	report := state.result
	if len(report.Before) != 2 || len(report.CloneIdentity) < 2 || len(report.CloneModes) != 2 || !reflect.DeepEqual(report.Before, report.After) || !reflect.DeepEqual(report.Before, report.FirstDiagnosticModes) {
		return fmt.Errorf("native fault physical restoration did not precede real diagnostic: %+v", report)
	}
	pid := state.completed[len(state.completed)-1].Identity.PID
	for index, mode := range report.Before {
		if mode.Foreground != pid || report.CloneModes[index].Foreground != pid || !reflect.DeepEqual(mode.Mode, report.CloneModes[index].Mode) {
			return errors.New("native fault clone/anchor mode or owned foreground differs")
		}
	}
	return nil
}

func (state *nativeBDDFaults) requireJSON() error {
	if err := state.requireCause(); err != nil {
		return err
	}
	receipt := state.completed[len(state.completed)-1]
	lines := strings.Split(strings.TrimSpace(string(receipt.Stderr)), "\n")
	if len(lines) != 2 {
		return fmt.Errorf("want one diagnostic/one machine failure body: %q", receipt.Stderr)
	}
	var body struct {
		SchemaVersion int    `json:"schemaVersion"`
		Command       string `json:"command"`
		ExitCode      int    `json:"exitCode"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &body); err != nil {
		return err
	}
	if receipt.Report.ExitCode != 125 || body.SchemaVersion != 1 || body.Command != "hippo tui" || body.ExitCode != 125 {
		return fmt.Errorf("wrong native TUI JSON failure: %+v exit=%d", body, receipt.Report.ExitCode)
	}
	return nil
}

func (state *nativeBDDFaults) requireNoJSONFrames() error {
	receipt := state.completed[len(state.completed)-1]
	if len(receipt.Stdout) != 0 || strings.Contains(string(receipt.Stderr), `"schemaVersion":5`) {
		return errors.New("native TUI failure emitted status frames")
	}
	return nil
}

func (state *nativeBDDFaults) close() {
	if state.directory == "" {
		return
	}
	if state.transport != nil && !state.transport.Result().Joined() {
		state.fixture.cleanupError = errors.Join(state.fixture.cleanupError, errors.New("native fault directory retained while transport remains owned"))
		return
	}
	if err := os.RemoveAll(state.directory); err != nil {
		state.fixture.cleanupError = errors.Join(state.fixture.cleanupError, err)
	} else {
		state.directory = ""
		state.transport = nil
	}
}

func (state *nativeBDDFaults) finishOutput(ctx context.Context, transport *support.NativeTUITransport) (func(), error) {
	if err := os.WriteFile(filepath.Join(state.directory, "apply-output-fault"), nil, 0o600); err != nil {
		return nil, err
	}
	wait, stop := context.WithTimeout(ctx, 500*time.Millisecond)
	ticker := time.NewTicker(time.Millisecond)
	cleanup := func() { ticker.Stop(); stop() }
	for {
		_, err := os.Stat(filepath.Join(state.directory, "fault-applied.json"))
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return cleanup, err
		}
		select {
		case <-wait.Done():
			return cleanup, wait.Err()
		case <-ticker.C:
		}
	}
	if err := transport.SendKey(ctx, []byte("c")); err != nil {
		return cleanup, err
	}
	return cleanup, nil
}
