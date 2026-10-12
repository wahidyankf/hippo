//go:build darwin

package host

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"reflect"
	"syscall"
	"testing"
)

func TestDarwinFIFOCommandHasFixedPrivateBoundary(t *testing.T) {
	var output bytes.Buffer
	command := newDarwinFIFOCommand(t.Context(), "/absolute/evidence", &output)
	if command.Path != "/bin/cat" || !reflect.DeepEqual(command.Args, []string{"/bin/cat", "--", "/absolute/evidence"}) || command.Stdout != &output || command.Stderr != io.Discard || command.Stdin != nil || len(command.ExtraFiles) != 0 || command.Env != nil || command.Cancel == nil {
		t.Fatalf("reader command lost fixed executable/operand/private-stream ownership")
	}
}

func TestDarwinFIFOCommandAlreadyCancelledStartsNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	operations := fifoCommandOperations{
		start:  func() error { t.Error("cancelled reader started a child"); return nil },
		wait:   func() error { t.Error("cancelled reader waited an unstarted child"); return nil },
		output: func() []byte { t.Error("cancelled reader exposed bytes"); return nil },
	}
	data, err := runDarwinFIFOCommand(ctx, "/absolute/evidence", operations)
	if data != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancelled reader: %q/%v", data, err)
	}
}

func TestDarwinFIFOCommandAccountsStartWaitAndOutput(t *testing.T) {
	for _, row := range []struct {
		name                               string
		startError, waitError              error
		wantStarts, wantWaits, wantOutputs int
	}{
		{"success", nil, nil, 1, 1, 1},
		{"start failure", syscall.EACCES, nil, 1, 0, 0},
		{"true wait failure", nil, syscall.ECHILD, 1, 1, 0},
		{"ordinary native reader failure", nil, &exec.ExitError{}, 1, 1, 0},
	} {
		t.Run(row.name, func(t *testing.T) {
			starts, waits, outputs := 0, 0, 0
			operations := fifoCommandOperations{start: func() error { starts++; return row.startError }, wait: func() error { waits++; return row.waitError }, output: func() []byte { outputs++; return []byte("private bytes") }}
			data, err := runDarwinFIFOCommand(t.Context(), "/absolute/evidence", operations)
			if starts != row.wantStarts || waits != row.wantWaits || outputs != row.wantOutputs {
				t.Fatalf("ownership counts %d/%d/%d", starts, waits, outputs)
			}
			if row.startError != nil {
				if data != nil || !errors.Is(err, row.startError) {
					t.Fatalf("start identity: %q/%v", data, err)
				}
				return
			}
			if row.waitError != nil {
				if data != nil || !errors.Is(err, row.waitError) {
					t.Fatalf("wait identity: %q/%v", data, err)
				}
				return
			}
			if err != nil || string(data) != "private bytes" {
				t.Fatalf("successful private bytes: %q/%v", data, err)
			}
		})
	}
}

func TestDarwinFIFOCommandCancellationStillWaitsAndDiscardsOutput(t *testing.T) {
	for _, when := range []string{"during start", "during EOF wait", "during failed wait"} {
		t.Run(when, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			starts, waits, outputs := 0, 0, 0
			operations := fifoCommandOperations{
				start: func() error {
					starts++
					if when == "during start" {
						cancel()
					}
					return nil
				},
				wait: func() error {
					waits++
					cancel()
					if when == "during failed wait" {
						return syscall.ECHILD
					}
					return nil
				},
				output: func() []byte { outputs++; return []byte("partial private output") },
			}
			data, err := runDarwinFIFOCommand(ctx, "/absolute/evidence", operations)
			if data != nil || !errors.Is(err, context.Canceled) || starts != 1 || waits != 1 || outputs != 0 {
				t.Fatalf("cancellation ownership: %q/%v counts=%d/%d/%d", data, err, starts, waits, outputs)
			}
			if when == "during failed wait" && !errors.Is(err, syscall.ECHILD) {
				t.Fatalf("genuine wait failure hidden: %v", err)
			}
		})
	}
}

func TestDarwinFIFOContextKillRequiresTerminalNativeStatus(t *testing.T) {
	for _, err := range []error{syscall.ECHILD, &exec.ExitError{}, errors.New("private copying failure")} {
		if expectedFIFOContextKill(err) {
			t.Fatalf("unknown wait/copy failure falsely earned terminal kill proof: %v", err)
		}
	}
}
