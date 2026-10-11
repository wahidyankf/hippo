package tui

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/status"
)

// BDDNativeTerminalMode retains physical descriptor readings, not a restore
// callback counter. The second reading occurs immediately after production Run.
type BDDNativeTerminalMode struct {
	Descriptor int             `json:"Descriptor"`
	Mode       json.RawMessage `json:"Mode"`
	Foreground int             `json:"Foreground"`
}

// BDDNativeLifecycleReport records actual observer calls and goroutine stacks
// inside the single-program child. Process reaping is a separate parent receipt.
type BDDNativeLifecycleReport struct {
	ExitCode         int                     `json:"ExitCode"`
	Requests         int                     `json:"Requests"`
	Completed        int                     `json:"Completed"`
	Cancelled        int                     `json:"Cancelled"`
	Active           int                     `json:"Active"`
	Before           []BDDNativeTerminalMode `json:"Before"`
	After            []BDDNativeTerminalMode `json:"After"`
	GoroutinesBefore []string                `json:"GoroutinesBefore"`
	GoroutinesAfter  []string                `json:"GoroutinesAfter"`
	RunnerReturned   bool                    `json:"RunnerReturned"`
}

var bddNativeLifecycleGoroutines = []string{
	"charm.land/bubbletea/v2.(*Program).readLoop(",
	"github.com/wahidyankf/hippo/internal/adapters/tui.(*observationWorker).run(",
	"github.com/wahidyankf/hippo/internal/adapters/tui.(*runnerScheduler).run(",
	"github.com/wahidyankf/hippo/internal/adapters/tui.pumpRunnerEvents(",
}

func bddNativeLifecycleStacks() ([]string, error) {
	buffer := make([]byte, 2*1024*1024)
	stackBytes := runtime.Stack(buffer, true)
	if stackBytes == len(buffer) {
		return nil, errors.New("native lifecycle goroutine observation exceeded its bound")
	}
	var active []string
	for _, name := range bddNativeLifecycleGoroutines {
		if strings.Contains(string(buffer[:stackBytes]), name) {
			active = append(active, name)
		}
	}
	return active, nil
}

func bddNativeLifecycleModes(input, output *os.File) ([]BDDNativeTerminalMode, error) {
	terminal, err := captureRunnerTerminal(input, output)
	if err != nil {
		return nil, err
	}
	var modes []BDDNativeTerminalMode
	for _, state := range terminal.states {
		mode, err := json.Marshal(state.mode)
		if err != nil {
			return nil, err
		}
		modes = append(modes, BDDNativeTerminalMode{Descriptor: state.descriptorNumber, Mode: mode, Foreground: state.foreground})
	}
	return modes, nil
}

func writeBDDNativeLifecycleReport(root *os.Root, filename string, report BDDNativeLifecycleReport) error {
	file, err := root.OpenFile("next-lifecycle.json", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	writeError := json.NewEncoder(file).Encode(report)
	if err := errors.Join(writeError, file.Close()); err != nil {
		return err
	}
	return root.Rename("next-lifecycle.json", filename)
}

// RunBDDNativeLifecycleHelper invokes the production exported Run through the
// actual CLI, with real signal translation and a controlled observer port.
// A second real request blocks until its own context is cancelled, exercising
// worker join while real scheduler, pump and Tea input goroutines are active.
func RunBDDNativeLifecycleHelper(parent context.Context, input, output, diagnostic *os.File, ready func() error, directory string) (int, error) {
	root, err := openBDDNativeObserverRoot(directory)
	if err != nil {
		return 0, err
	}
	var report BDDNativeLifecycleReport
	report.Before, err = bddNativeLifecycleModes(input, output)
	if err != nil {
		return 0, errors.Join(err, root.Close())
	}
	ctx, stop := cli.SignalContext(parent)
	defer stop()
	var observationError error
	observe := func(ctx context.Context) (application.ObservationSnapshot, int, error) {
		report.Requests++
		report.Active++
		defer func() { report.Active-- }()
		if report.Requests == 1 {
			if err := ready(); err != nil {
				observationError = err
				return application.ObservationSnapshot{}, 0, err
			}
			report.Completed++
			snapshot := BDDNativeObserverSnapshot()
			now := time.Now()
			snapshot.SampledAt = &now
			return snapshot, 0, nil
		}
		report.GoroutinesBefore, observationError = bddNativeLifecycleStacks()
		if observationError == nil {
			observationError = writeBDDNativeLifecycleReport(root, "lifecycle-progress.json", report)
		}
		if observationError != nil {
			return application.ObservationSnapshot{}, 0, observationError
		}
		<-ctx.Done()
		report.Cancelled++
		return application.ObservationSnapshot{}, 0, ctx.Err()
	}
	boundary := cli.Application{
		Stdin: input, Stdout: output, Stderr: diagnostic, Environment: os.Environ(),
		TerminalCapabilities: func() (bool, bool, error) { return TerminalCapabilities(input, output) },
		RunTUI: func(ctx context.Context, _ application.ObservationRequest, interval time.Duration, styling bool) (status.Interruption, error) {
			interruption, runError := Run(ctx, observe, Options{Interval: interval, Styling: styling, Input: input, Output: output, Environment: os.Environ()})
			report.RunnerReturned = true
			var modeError, stackError error
			report.After, modeError = bddNativeLifecycleModes(input, output)
			report.GoroutinesAfter, stackError = bddNativeLifecycleStacks()
			observationError = errors.Join(observationError, modeError, stackError)
			return interruption, runError
		},
	}
	report.ExitCode, _ = boundary.Run(ctx, []string{"tui", "--interval", "1h"})
	reportError := writeBDDNativeLifecycleReport(root, "lifecycle-report.json", report)
	return report.ExitCode, errors.Join(observationError, reportError, root.Close())
}
