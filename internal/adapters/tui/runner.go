package tui

import (
	"context"
	"errors"
	"io"
	"slices"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/status"
)

// Options supplies the invocation's streams, environment and presentation policy.
type Options struct {
	Interval    time.Duration
	Styling     bool
	Input       io.Reader
	Output      io.Writer
	Environment []string
}

type runnerProgram struct {
	run  func() (tea.Model, error)
	send func(tea.Msg)
	kill func()
}

type runnerTimer struct {
	events <-chan time.Time
	reset  func(time.Duration) bool
	stop   func() bool
}

type runnerDependencies struct {
	now        func() time.Time
	newTimer   func(time.Duration) runnerTimer
	newProgram func(context.Context, *model, Options) runnerProgram
	restore    func() error
}

// Run owns the interactive observation session and returns after its work joins.
func Run(ctx context.Context, observe func(context.Context) (application.ObservationSnapshot, int, error), options Options) (status.Interruption, error) {
	terminal, err := captureRunnerTerminal(options.Input, options.Output)
	if err != nil {
		return status.Interruption{}, status.Fail(status.CodeSupervisionFailed, "cannot initialize TUI terminal")
	}
	sessionContext, cancel := context.WithCancel(ctx)
	defer cancel()
	output := newRunnerOutput(&options, cancel)
	options.Environment = slices.Clone(options.Environment)
	dependencies := runnerDependencies{
		now: time.Now,
		newTimer: func(duration time.Duration) runnerTimer {
			timer := time.NewTimer(duration)
			return runnerTimer{events: timer.C, reset: timer.Reset, stop: timer.Stop}
		},
		newProgram: func(programContext context.Context, state *model, options Options) runnerProgram {
			program := tea.NewProgram(state, tea.WithContext(programContext), tea.WithInput(options.Input), tea.WithOutput(options.Output), tea.WithEnvironment(options.Environment), tea.WithoutSignalHandler())
			return runnerProgram{run: program.Run, send: program.Send, kill: program.Kill}
		},
		restore: terminal.restore,
	}
	interruption, runError := runSession(sessionContext, observe, options, dependencies)
	if output.failed.Load() {
		return interruption, status.Fail(status.CodeSupervisionFailed, "TUI terminal output failed")
	}
	return interruption, runError
}

func runSession(ctx context.Context, observe func(context.Context) (application.ObservationSnapshot, int, error), options Options, dependencies runnerDependencies) (status.Interruption, error) {
	ownedContext, cancel := context.WithCancel(ctx)
	defer cancel()
	worker := newObservationWorker(ownedContext, observe, dependencies.now)
	scheduler := newRunnerScheduler(ownedContext, dependencies.now, dependencies.newTimer)
	state := newStyledModel(options.Interval, options.Styling, modelEffects{
		request: func(request observationRequest) {
			scheduler.replace(refreshSchedule{generation: request.generation})
			worker.request(request)
		},
		schedule: scheduler.replace,
		cancel:   cancel,
	})
	// Bubble Tea v2.0.10 skips its input-reader join on context cancellation.
	// The sole event pump translates owned cancellation to graceful Quit instead,
	// reaching its bounded input-loop join before cancelreader closes its files.
	program := dependencies.newProgram(context.WithoutCancel(ctx), state, options)
	pumpDone := make(chan struct{})
	go pumpRunnerEvents(ownedContext, program.send, dependencies.now(), worker.results, scheduler.events, pumpDone)
	_, runError := program.run()
	// Kill also releases Send when Program.Run failed before its event loop.
	program.kill()
	cancel()
	<-worker.done
	<-scheduler.done
	<-pumpDone
	if dependencies.restore != nil {
		if err := dependencies.restore(); err != nil {
			return state.interruption, status.Fail(status.CodeSupervisionFailed, "cannot restore TUI terminal")
		}
	}
	if runError != nil && (ctx.Err() == nil || !errors.Is(runError, ctx.Err())) {
		return state.interruption, status.Fail(status.CodeSupervisionFailed, "TUI terminal session failed")
	}
	if ctx.Err() != nil {
		return state.interruption, errors.Join(ctx.Err(), context.Cause(ctx))
	}
	return state.interruption, nil
}

// newRunnerOutput observes write failures while preparing the program output.
func newRunnerOutput(options *Options, cancel context.CancelFunc) *runnerOutput {
	output := &runnerOutput{writer: options.Output, cancel: cancel}
	if descriptor, ok := options.Output.(runnerDescriptor); ok {
		output.descriptor = descriptor.Fd()
	}
	options.Output = output
	// Bubble Tea queries size and resize only for complete terminal files.
	// Preserve that contract without inventing it for an ordinary writer.
	if file, ok := output.writer.(interface {
		io.ReadCloser
		runnerDescriptor
	}); ok {
		options.Output = &runnerFileOutput{ReadCloser: file, runnerOutput: output}
	}
	return output
}

type runnerFileOutput struct {
	io.ReadCloser
	*runnerOutput
}

type runnerOutput struct {
	writer     io.Writer
	cancel     context.CancelFunc
	descriptor uintptr
	failed     atomic.Bool
}

func (writer *runnerOutput) Fd() uintptr { return writer.descriptor }

func (writer *runnerOutput) Write(content []byte) (int, error) {
	written, err := writer.writer.Write(content)
	if err == nil && written != len(content) {
		err = io.ErrShortWrite
	}
	if err != nil {
		writer.failed.Store(true)
		writer.cancel()
	}
	return written, err
}
