package integration_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	runtimeadapter "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/adapters/tui"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/status"
)

func TestTUIRunnerRestoresIsolatedPTYOnEveryExit(t *testing.T) {
	script, err := exec.LookPath("script")
	if err != nil {
		t.Fatal("isolated PTY requires the existing script utility")
	}
	for _, mode := range []string{"q", "raw-ctrl-c", "parent-cancel", "initialization-error", "output-error", "read-error", "sigint", "sigterm"} {
		t.Run(mode, func(t *testing.T) { runTUIRunnerPTY(t, script, mode) })
	}
}

func runTUIRunnerPTY(t *testing.T, script, mode string) {
	t.Helper()
	root := t.TempDir()
	ready := filepath.Join(root, "ready")
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, script, ptyScriptArguments(t, os.Args[0], "-test.run=^TestTUIRunnerPTYHelper$", "-test.v")...)
	command.Env = append(os.Environ(), "HIPPO_TUI_PTY_MODE="+mode, "HIPPO_TUI_PTY_READY="+ready, "TERM=xterm-256color", "NO_COLOR=1")
	command.WaitDelay = time.Second
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	defer func() { _ = writer.Close() }()
	command.Stdin = reader
	inputDone := make(chan struct{})
	go func() {
		defer close(inputDone)
		if mode != "q" && mode != "raw-ctrl-c" && mode != "read-error" {
			return
		}
		ticker := time.NewTicker(2 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := os.Stat(ready); err != nil {
					continue
				}
				input := "q"
				if mode == "raw-ctrl-c" {
					input = "\x03"
				}
				if mode == "read-error" {
					input = "x"
				}
				_, _ = writer.WriteString(input)
				return
			}
		}
	}()
	output, runError := command.CombinedOutput()
	timedOut := ctx.Err() != nil
	cancel()
	<-inputDone
	t.Logf("PTY mode=%s helperError=%v transcript=%q", mode, runError, output)
	if timedOut {
		t.Fatalf("isolated TUI helper exceeded its cleanup bound: %s", output)
	}
	// BSD script does not propagate the helper's exit code. Require the Go
	// helper verdict as well as any process failure on platforms that do.
	if runError != nil || !strings.Contains(string(output), "--- PASS: TestTUIRunnerPTYHelper") {
		t.Errorf("isolated TUI lifecycle assertion failed: %v\n%s", runError, output)
	}
}

type tuiPTYFailingOutput struct{ file *os.File }

func (writer tuiPTYFailingOutput) Write([]byte) (int, error) {
	return 0, errors.New("private output fault")
}

func (writer tuiPTYFailingOutput) Fd() uintptr { return writer.file.Fd() }

type tuiPTYFailingInput struct{ *os.File }

func (tuiPTYFailingInput) Read([]byte) (int, error) { return 0, errors.New("private read fault") }

func tuiPTYMode(t *testing.T) string {
	t.Helper()
	mode, err := readTUIPTYMode(t.Context())
	if err != nil {
		t.Fatalf("read isolated terminal mode: %v", err)
	}
	return mode
}

func readTUIPTYMode(ctx context.Context) (string, error) {
	command := exec.CommandContext(ctx, "stty", "-g")
	command.Stdin = os.Stdin
	output, err := command.Output()
	return strings.TrimSpace(string(output)), err
}

func TestTUIRunnerPTYHelper(t *testing.T) {
	mode := os.Getenv("HIPPO_TUI_PTY_MODE")
	if mode == "" {
		return
	}
	before := tuiPTYMode(t)
	foreground, err := runtimeadapter.ForegroundProcessGroup(os.Stdin)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if mode == "sigint" || mode == "sigterm" {
		var stop func()
		ctx, stop = cli.SignalContext(ctx)
		defer stop()
	}
	observation := &tuiPTYObservation{mode: mode, cancel: cancel}
	options := tuiPTYOptions(t, mode)
	interruption, runError := tui.Run(ctx, observation.collect, options)
	after := tuiPTYMode(t)
	currentForeground, foregroundError := runtimeadapter.ForegroundProcessGroup(os.Stdin)
	t.Logf("mode=%s observations=%d active=%d joined=%d interruption=%+v error=%v before=%q after=%q foreground=%d/%d", mode, observation.observations.Load(), observation.active.Load(), observation.joined.Load(), interruption, runError, before, after, foreground, currentForeground)
	if before != after || foregroundError != nil || foreground != currentForeground {
		t.Errorf("terminal modes/foreground were not restored: before=%q after=%q groups=%d/%d error=%v", before, after, foreground, currentForeground, foregroundError)
	}
	if observation.active.Load() != 0 || observation.joined.Load() != observation.observations.Load() {
		t.Errorf("observation survived runner return: active=%d started=%d joined=%d", observation.active.Load(), observation.observations.Load(), observation.joined.Load())
	}
	assertTUIRunnerPTYExit(ctx, t, mode, before, observation, interruption, runError)
}

type tuiPTYObservation struct {
	mode                         string
	cancel                       context.CancelFunc
	observations, active, joined atomic.Int32
	rawMode                      atomic.Pointer[string]
}

func (observation *tuiPTYObservation) collect(ctx context.Context) (application.ObservationSnapshot, int, error) {
	observation.observations.Add(1)
	observation.active.Add(1)
	defer func() { observation.active.Add(-1); observation.joined.Add(1) }()
	current, modeError := readTUIPTYMode(ctx)
	if modeError != nil {
		observation.cancel()
		return application.ObservationSnapshot{}, 1, modeError
	}
	observation.rawMode.Store(&current)
	if err := writeTUIPTYReady(); err != nil {
		observation.cancel()
		return application.ObservationSnapshot{}, 1, err
	}
	if observation.mode == "parent-cancel" {
		observation.cancel()
	}
	if observation.mode == "sigint" || observation.mode == "sigterm" {
		signal := syscall.SIGINT
		if observation.mode == "sigterm" {
			signal = syscall.SIGTERM
		}
		if err := syscall.Kill(os.Getpid(), signal); err != nil {
			observation.cancel()
			return application.ObservationSnapshot{}, 1, err
		}
	}
	<-ctx.Done()
	return application.ObservationSnapshot{}, 1, ctx.Err()
}

func tuiPTYOptions(t *testing.T, mode string) tui.Options {
	t.Helper()
	options := tui.Options{Interval: 5 * time.Second, Input: os.Stdin, Output: os.Stdout, Environment: []string{"TERM=xterm-256color", "NO_COLOR=1"}}
	if mode == "initialization-error" {
		closed, openError := os.Open(os.Stdin.Name())
		if openError != nil {
			t.Fatal(openError)
		}
		if closeError := closed.Close(); closeError != nil {
			t.Fatal(closeError)
		}
		options.Input = closed
	}
	if mode == "output-error" {
		options.Output = tuiPTYFailingOutput{file: os.Stdout}
	}
	if mode == "read-error" {
		options.Input = tuiPTYFailingInput{File: os.Stdin}
	}
	return options
}

func writeTUIPTYReady() error {
	ready := os.Getenv("HIPPO_TUI_PTY_READY")
	root, err := os.OpenRoot(filepath.Dir(ready))
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	return root.WriteFile(filepath.Base(ready), []byte("ready"), 0o600)
}

func assertTUIRunnerPTYExit(ctx context.Context, t *testing.T, mode, before string, observation *tuiPTYObservation, interruption status.Interruption, runError error) {
	t.Helper()
	switch mode {
	case "q", "raw-ctrl-c", "parent-cancel", "sigint", "sigterm":
		current := observation.rawMode.Load()
		if observation.observations.Load() != 1 || current == nil || *current == before {
			t.Errorf("runner did not enter terminal mode and observe: starts=%d enteredMode=%v", observation.observations.Load(), current)
		}
		if mode == "q" && (runError != nil || interruption.Signal != 0) {
			t.Errorf("q did not finish normally: interruption=%+v error=%v", interruption, runError)
		}
		if mode == "raw-ctrl-c" && (interruption.Signal != 2 || interruption.Name != "interrupt") {
			t.Errorf("raw Ctrl-C did not retain local interruption: %+v", interruption)
		}
		if mode == "parent-cancel" && !errors.Is(runError, context.Canceled) {
			t.Errorf("parent cancellation was lost: %v", runError)
		}
		assertTUIRunnerPTYSignal(ctx, t, mode, runError)
	case "initialization-error", "output-error", "read-error":
		failure, classified := errors.AsType[status.Failure](runError)
		if !classified || failure.Code != status.CodeSupervisionFailed {
			t.Errorf("terminal failure was not classified: %v", runError)
		}
		if runError != nil && strings.Contains(runError.Error(), "private") {
			t.Error("terminal diagnostic exposed the raw IO error")
		}
	}
}

func assertTUIRunnerPTYSignal(ctx context.Context, t *testing.T, mode string, runError error) {
	t.Helper()
	if mode == "sigint" || mode == "sigterm" {
		cause, signaled := status.Interrupted(ctx)
		wanted := 130
		if mode == "sigterm" {
			wanted = 143
		}
		if !signaled || cause.Status() != wanted || !errors.Is(runError, context.Canceled) {
			t.Errorf("process signal cause lost: cause=%+v present=%v wanted=%d error=%v", cause, signaled, wanted, runError)
		}
	}
}
