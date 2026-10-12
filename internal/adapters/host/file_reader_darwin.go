//go:build darwin

package host

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func readFileContext(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(path)
	if cancelled := ctx.Err(); cancelled != nil {
		return nil, cancelled
	}
	if err != nil {
		return nil, &os.PathError{Op: "abs", Path: path, Err: err}
	}
	metadata, err := os.Stat(absolute)
	if cancelled := ctx.Err(); cancelled != nil {
		return nil, cancelled
	}
	if err != nil {
		return nil, err
	}
	if metadata.Mode()&os.ModeNamedPipe == 0 {
		return readUnixFileContext(ctx, absolute)
	}
	return readDarwinFIFO(ctx, absolute)
}

type fifoCommandOperations struct {
	start  func() error
	wait   func() error
	output func() []byte
}

func newDarwinFIFOCommand(ctx context.Context, path string, output *bytes.Buffer) *exec.Cmd {
	command := exec.CommandContext(ctx, "/bin/cat", "--", path)
	command.Stdout = output
	command.Stderr = io.Discard
	return command
}

func readDarwinFIFO(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	command := newDarwinFIFOCommand(ctx, path, &output)
	return runDarwinFIFOCommand(ctx, path, fifoCommandOperations{start: command.Start, wait: command.Wait, output: output.Bytes})
}

func runDarwinFIFOCommand(ctx context.Context, path string, operations fifoCommandOperations) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := operations.start(); err != nil {
		if cancelled := ctx.Err(); cancelled != nil {
			return nil, cancelled
		}
		return nil, &os.PathError{Op: "start", Path: path, Err: err}
	}
	// Every successful Start owns exactly one Wait. Command.Wait joins its child,
	// context watcher and private copying work and closes command-owned pipes.
	waitError := operations.wait()
	if cancelled := ctx.Err(); cancelled != nil {
		if waitError != nil && !expectedFIFOContextKill(waitError) {
			return nil, errors.Join(cancelled, &os.PathError{Op: "wait", Path: path, Err: waitError})
		}
		return nil, cancelled
	}
	if waitError != nil {
		return nil, &os.PathError{Op: "wait", Path: path, Err: waitError}
	}
	return operations.output(), nil
}

func expectedFIFOContextKill(err error) bool {
	exited, ok := errors.AsType[*exec.ExitError](err)
	if !ok || exited == nil || exited.ProcessState == nil {
		return false
	}
	state, ok := exited.Sys().(syscall.WaitStatus)
	return ok && state.Signaled() && state.Signal() == syscall.SIGKILL
}
