package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/status"
	"github.com/wahidyankf/hippo/tests/contract"
)

type bddRunnerState struct {
	fixture                   *runnerFixture
	view, fault, interruption string
	cancel                    context.CancelCauseFunc
	state                     *model
	err                       error
	exit                      int
	diagnostics               bytes.Buffer
	mutex                     sync.Mutex
	events                    []string
	restores                  atomic.Int32
	finished                  bool
}

func (driver *BDDTestDriver) runnerBindings() []contract.StepBinding {
	return []contract.StepBinding{
		bddStep(`^TUI is displaying (dashboard|job detail|capacity detail|help|minimum-size notice) in an isolated terminal$`, driver.prepareRunnerView),
		bddStep(`^the user presses q$`, driver.quitRunner),
		bddStep(`^TUI exits 0 after cancelling and joining its observation work$`, driver.requireRunnerQuit),
		bddStep(`^the original terminal modes and foreground ownership are restored$`, driver.requireRunnerRestoration),
		bddStep(`^no guarded job is cancelled or retired$`, driver.requireRunnerReadOnly),
		bddStep(`^TUI is observing in an isolated terminal$`, func(ctx context.Context) error { return driver.prepareRunnerView(ctx, "dashboard") }),
		bddStep(`^TUI receives (raw Ctrl-C|SIGINT|SIGTERM)$`, driver.interruptRunner),
		bddStep(`^TUI exits (\d+) without a HIPPO failure diagnostic$`, driver.requireRunnerInterruption),
		bddStep(`^observation is cancelled and joined before the runner returns$`, driver.requireRunnerJoined),
		bddStep(`^TUI encounters a (startup|input read|output write|restoration) failure in an isolated terminal$`, driver.prepareRunnerFault),
		bddStep(`^the terminal session finishes$`, driver.finishRunner),
		bddStep(`^it exits 125 with one privacy-safe HIPPO diagnostic$`, driver.requireRunnerFailure),
		bddStep(`^terminal restoration is attempted before the diagnostic is written$`, driver.requireRunnerRestoreBeforeDiagnostic),
		bddStep(`^its observation work and timers do not survive the runner$`, driver.requireRunnerJoined),
		bddStep(`^an interrupted TUI whose terminal restoration fails$`, driver.prepareInterruptedRestore),
		bddStep(`^it exits 125 for the restoration failure instead of the signal status$`, driver.requireRunnerFailure),
		bddStep(`^restoration is attempted before the diagnostic is written$`, driver.requireRunnerRestoreBeforeDiagnostic),
	}
}

func (state *bddRunnerState) record(event string) {
	state.mutex.Lock()
	defer state.mutex.Unlock()
	state.events = append(state.events, event)
}

type bddRunnerDiagnostic struct{ state *bddRunnerState }

func (writer bddRunnerDiagnostic) Write(data []byte) (int, error) {
	writer.state.record("diagnostic")
	return writer.state.diagnostics.Write(data)
}

func (driver *BDDTestDriver) prepareRunnerView(ctx context.Context, view string) error {
	if driver.adapter.Name != contract.Unit {
		return fmt.Errorf("%s terminal lifecycle requires actual owned PTY; native C/D dispositions remain unset", driver.adapter.Name)
	}
	if driver.guardFactory == nil {
		return errors.New("runner has no independent application guarded-job factory")
	}
	probe, finish, err := driver.guardFactory(ctx)
	if err != nil {
		return err
	}
	driver.guardProbe, driver.guardFinish = probe, finish
	driver.runner = &bddRunnerState{fixture: newRunnerFixture(), view: view}
	return nil
}

func (driver *BDDTestDriver) prepareRunnerFault(ctx context.Context, fault string) error {
	if err := driver.prepareRunnerView(ctx, "dashboard"); err != nil {
		return err
	}
	driver.runner.fault = fault
	return nil
}

func (driver *BDDTestDriver) prepareInterruptedRestore(ctx context.Context) error {
	if err := driver.prepareRunnerFault(ctx, "restoration"); err != nil {
		return err
	}
	driver.runner.interruption = "SIGTERM"
	return nil
}

func (driver *BDDTestDriver) quitRunner(ctx context.Context) error { return driver.executeRunner(ctx) }

func (driver *BDDTestDriver) interruptRunner(ctx context.Context, value string) error {
	driver.runner.interruption = value
	return driver.executeRunner(ctx)
}

func (driver *BDDTestDriver) finishRunner(ctx context.Context) error {
	return driver.executeRunner(ctx)
}

func (driver *BDDTestDriver) executeRunner(caller context.Context) error {
	state := driver.runner
	if state == nil {
		return errors.New("runner was not prepared")
	}
	if state.finished {
		return errors.New("runner scenario executed twice")
	}
	ctx, cancel := context.WithTimeout(caller, 5*time.Second)
	defer cancel()
	owned, cancelCause := context.WithCancelCause(ctx)
	defer cancelCause(nil)
	state.cancel = cancelCause
	fixture := state.fixture
	deps := fixture.dependencies("quit while busy", nil)
	deps.restore = func() error {
		state.record("restore")
		state.restores.Add(1)
		if state.fault == "restoration" {
			return errors.New("/private/restore-fault")
		}
		return nil
	}
	deps.newProgram = func(programContext context.Context, modelState *model, _ Options) runnerProgram {
		return state.newRunnerProgram(programContext, modelState, cancelCause)
	}
	observe := func(ctx context.Context) (application.ObservationSnapshot, int, error) {
		call := fixture.observations.Add(1)
		fixture.active.Add(1)
		defer func() { fixture.active.Add(-1); fixture.joined.Add(1); state.record("join") }()
		if call == 1 {
			return bddSnapshot(), 0, nil
		}
		fixture.started <- struct{}{}
		<-ctx.Done()
		state.record("cancel")
		return application.ObservationSnapshot{}, 1, ctx.Err()
	}
	boundary := cli.Application{Stdout: &bytes.Buffer{}, Stderr: bddRunnerDiagnostic{state}, Environment: []string{"TERM=xterm", "NO_COLOR=1"}, TerminalCapabilities: func() (bool, bool, error) { return true, true, nil }, RunTUI: func(ctx context.Context, _ application.ObservationRequest, _ time.Duration, _ bool) (status.Interruption, error) {
		return runSession(ctx, observe, Options{Interval: 5 * time.Second}, deps)
	}}
	state.exit, state.err = boundary.Run(owned, []string{"tui"})
	state.record("return")
	state.finished = true
	if ctx.Err() != nil {
		return fmt.Errorf("runner scenario exceeded bound: %w", ctx.Err())
	}
	return nil
}

func (state *bddRunnerState) newRunnerProgram(programContext context.Context, modelState *model, cancelCause context.CancelCauseFunc) runnerProgram {
	fixture := state.fixture
	fixture.programs.Add(1)
	state.state = modelState
	messages := make(chan tea.Msg)
	programContext, stop := context.WithCancel(programContext)
	return runnerProgram{
		send: func(msg tea.Msg) {
			select {
			case messages <- msg:
			case <-programContext.Done():
			}
		},
		kill: func() { fixture.kills.Add(1); stop() },
		run: func() (tea.Model, error) {
			err := state.runProgram(programContext, modelState, messages, cancelCause)
			return modelState, err
		},
	}
}

func (state *bddRunnerState) runProgram(ctx context.Context, modelState *model, messages <-chan tea.Msg, cancelCause context.CancelCauseFunc) error {
	if state.fault == "startup" {
		return errors.New("/private/startup-fault")
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-state.fixture.started:
			if state.fixture.observations.Load() < 2 {
				continue
			}
			done, err := state.interruptBusyProgram(modelState, cancelCause)
			if done || err != nil {
				return err
			}
		case msg := <-messages:
			done, err := state.updateProgram(modelState, msg)
			if done || err != nil {
				return err
			}
		}
	}
}

func (state *bddRunnerState) interruptBusyProgram(modelState *model, cancelCause context.CancelCauseFunc) (bool, error) {
	state.record("busy")
	if state.fault == "input read" || state.fault == "output write" {
		return false, errors.New("/private/transport-fault")
	}
	switch state.interruption {
	case "raw Ctrl-C":
		modelState.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	case "":
		modelState.Update(modelKey('q'))
		return true, nil
	default:
		signal, name := 2, "interrupt"
		if state.interruption == "SIGTERM" {
			signal, name = 15, "terminated"
		}
		cancelCause(status.Interruption{Signal: signal, Name: name})
	}
	return false, nil
}

func (state *bddRunnerState) updateProgram(modelState *model, msg tea.Msg) (bool, error) {
	modelState.Update(msg)
	if _, ok := msg.(tea.QuitMsg); ok {
		return true, nil
	}
	if _, ok := msg.(observationResult); !ok {
		return false, nil
	}
	modelState.Update(tea.WindowSizeMsg{Width: 40, Height: 16})
	switch state.view {
	case "dashboard":
	case "job detail":
		modelState.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	case "capacity detail":
		modelState.Update(modelKey('c'))
	case "help":
		modelState.Update(modelKey('?'))
	case "minimum-size notice":
		modelState.Update(tea.WindowSizeMsg{Width: 39, Height: 15})
	default:
		return false, fmt.Errorf("unknown runner surface %q", state.view)
	}
	modelState.Update(modelKey('r'))
	return false, nil
}

func (driver *BDDTestDriver) requireRunnerJoined() error {
	state := driver.runner
	if state == nil || !state.finished {
		return errors.New("runner did not complete")
	}
	fixture := state.fixture
	if fixture.active.Load() != 0 || fixture.joined.Load() != fixture.observations.Load() || fixture.stops.Load() < 1 || fixture.kills.Load() != 1 {
		return fmt.Errorf("runner retained owned work: active=%d joined=%d calls=%d stops=%d kills=%d", fixture.active.Load(), fixture.joined.Load(), fixture.observations.Load(), fixture.stops.Load(), fixture.kills.Load())
	}
	return nil
}

func (driver *BDDTestDriver) requireRunnerQuit() error {
	if err := driver.requireRunnerJoined(); err != nil {
		return err
	}
	state := driver.runner
	if state.exit != 0 || state.err != nil || state.diagnostics.Len() != 0 {
		return fmt.Errorf("quit outcome %d/%w/%q", state.exit, state.err, state.diagnostics.String())
	}
	return nil
}

func (driver *BDDTestDriver) requireRunnerInterruption(code int) error {
	if err := driver.requireRunnerJoined(); err != nil {
		return err
	}
	state := driver.runner
	if state.exit != code || state.diagnostics.Len() != 0 {
		return fmt.Errorf("signal outcome %d want%d diagnostics=%q", state.exit, code, state.diagnostics.String())
	}
	return nil
}

func (driver *BDDTestDriver) requireRunnerFailure() error {
	if err := driver.requireRunnerJoined(); err != nil {
		return err
	}
	state := driver.runner
	diagnostic := state.diagnostics.String()
	if state.exit != 125 || strings.Count(diagnostic, "hippo.supervision.failed") != 1 || strings.Contains(diagnostic, "/private/") {
		return fmt.Errorf("classified runner failure %d/%q", state.exit, diagnostic)
	}
	return nil
}

func (driver *BDDTestDriver) requireRunnerRestoreBeforeDiagnostic() error {
	state := driver.runner
	state.mutex.Lock()
	defer state.mutex.Unlock()
	restore, diagnostic := -1, -1
	for i, event := range state.events {
		if event == "restore" {
			restore = i
		}
		if event == "diagnostic" && diagnostic < 0 {
			diagnostic = i
		}
	}
	if restore < 0 || diagnostic < 0 || restore >= diagnostic {
		return fmt.Errorf("restore did not precede real CLI diagnostic: %v", state.events)
	}
	return nil
}

func (driver *BDDTestDriver) requireRunnerRestoration() error {
	if driver.adapter.Name != contract.Unit {
		return errors.New("physical terminal restoration requires native proof")
	}
	if driver.runner.restores.Load() != 1 {
		return errors.New("actual runner did not invoke its terminal restoration effect once")
	}
	return nil
}

func (driver *BDDTestDriver) requireRunnerReadOnly() error {
	if driver.guardProbe == nil || driver.guardFinish == nil {
		return errors.New("runner has no independently owned guarded application")
	}
	if err := driver.guardProbe(); err != nil {
		return err
	}
	return driver.guardFinish()
}

func (driver *BDDTestDriver) quitBusyRunner(ctx context.Context) error {
	// Keep the original model generation to discriminate late-result updates,
	// and independently exercise real worker/scheduler cancellation and joins.
	state := driver.state
	before := state.snapshot
	state.Update(modelKey('q'))
	modelComplete(state, state.generation, 120, bddSnapshot(), nil)
	if state.hasSnapshot || len(state.snapshot.Running) != len(before.Running) {
		return errors.New("late result updated quit model")
	}
	if err := driver.prepareRunnerView(ctx, "dashboard"); err != nil {
		return err
	}
	return driver.executeRunner(ctx)
}
func (driver *BDDTestDriver) requireBusyRunnerJoined() error { return driver.requireRunnerQuit() }
