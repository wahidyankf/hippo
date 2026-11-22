package host

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
)

type fifoCommandOperations struct {
	start  func() error
	wait   func() error
	output func() []byte
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
