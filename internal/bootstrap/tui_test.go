package bootstrap_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/bootstrap"
	"github.com/wahidyankf/hippo/internal/status"
)

func TestBootstrapPreservesCustomTUIOverridesBeforeAndAfterComposition(t *testing.T) {
	for _, when := range []string{"before", "after"} {
		t.Run(when, func(t *testing.T) {
			var output, diagnostics bytes.Buffer
			inspections, runs := 0, 0
			capabilities := func() (bool, bool, error) { inspections++; return true, true, nil }
			runner := func(_ context.Context, _ application.ObservationRequest, interval time.Duration, _ bool) (status.Interruption, error) {
				runs++
				if interval != 5*time.Second {
					t.Errorf("interval=%v", interval)
				}
				return status.Interruption{}, nil
			}
			boundary := cli.Application{Stdout: &output, Stderr: &diagnostics, Environment: []string{"TERM=xterm"}}
			if when == "before" {
				boundary.TerminalCapabilities = capabilities
				boundary.RunTUI = runner
			}
			boundary = bootstrap.WithDefaults(boundary)
			if when == "after" {
				boundary.TerminalCapabilities = capabilities
				boundary.RunTUI = runner
			}
			boundary.Environment = []string{"TERM=xterm-256color"}
			code, err := boundary.Run(t.Context(), []string{"tui"})
			if code != 0 || err != nil || inspections != 1 || runs != 1 || diagnostics.Len() != 0 {
				t.Fatalf("custom TUI replaced at %s composition: code=%d error=%v inspections=%d runs=%d diagnostics=%q", when, code, err, inspections, runs, diagnostics.String())
			}
			if boundary.Observer.Annotations == nil {
				t.Fatal("production observation has no display annotation capability")
			}
		})
	}
}

func TestBootstrapRefreshesDefaultTUITerminalStreamsBeforeRun(t *testing.T) {
	for _, test := range []struct {
		name, stream string
		closedAfter  bool
	}{
		{"input closed before composition", "input", false},
		{"input closed after composition", "input", true},
		{"output closed before composition", "output", false},
		{"output closed after composition", "output", true},
	} {
		stream, closedAfter := test.stream, test.closedAfter
		t.Run(test.name, func(t *testing.T) {
			closed, err := os.CreateTemp(t.TempDir(), "closed-terminal-")
			if err != nil {
				t.Fatal(err)
			}
			if err := closed.Close(); err != nil {
				t.Fatal(err)
			}
			var output, diagnostics bytes.Buffer
			boundary := cli.Application{Stdin: strings.NewReader(""), Stdout: &output, Stderr: &diagnostics, Environment: []string{"TERM=xterm"}}
			if !closedAfter {
				setBootstrapTUIStream(&boundary, stream, closed)
			}
			runs := 0
			boundary.RunTUI = func(context.Context, application.ObservationRequest, time.Duration, bool) (status.Interruption, error) {
				runs++
				return status.Interruption{}, nil
			}
			boundary = bootstrap.WithDefaults(boundary)
			switch {
			case closedAfter:
				setBootstrapTUIStream(&boundary, stream, closed)
			case stream == "input":
				boundary.Stdin = strings.NewReader("")
			default:
				boundary.Stdout = &output
			}
			wanted, wantedCode := 2, status.CodeArgsInvalid
			if closedAfter {
				wanted, wantedCode = 125, status.CodeSupervisionFailed
			}
			code, runError := boundary.Run(t.Context(), []string{"tui", "--config", "missing.json"})
			if code != wanted || runError == nil || !strings.Contains(diagnostics.String(), "["+string(wantedCode)+"]") || runs != 0 {
				t.Fatalf("mutable %s: code=%d error=%v runner=%d diagnostics=%q, want %d %s", stream, code, runError, runs, diagnostics.String(), wanted, wantedCode)
			}
		})
	}
}

func setBootstrapTUIStream(boundary *cli.Application, stream string, file *os.File) {
	if stream == "input" {
		boundary.Stdin = file
	} else {
		boundary.Stdout = file
	}
}
