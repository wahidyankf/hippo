package conformance //nolint:testpackage // Deterministic lifecycle seams are intentionally exercised inside the production package.

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestExecuteDoesNotStartWithPreCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var starts atomic.Int32
	runtime := commandRuntime{
		start: func(*exec.Cmd) error {
			starts.Add(1)

			return nil
		},
		wait: func(*exec.Cmd) error { return nil },
		signal: func(*os.Process, syscall.Signal) error {
			return nil
		},
	}
	err := executeWithRuntime(ctx, ".", Command{Arguments: []string{"true"}}, nil, &bytes.Buffer{}, runtime)
	if err == nil || starts.Load() != 0 {
		t.Fatalf("pre-cancelled execution started %d commands with error %v", starts.Load(), err)
	}
}

func TestExecuteBoundsPostKillReapFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	waitRelease := make(chan struct{})
	runtime := commandRuntime{
		start: func(*exec.Cmd) error {
			cancel()

			return nil
		},
		wait: func(*exec.Cmd) error {
			<-waitRelease

			return nil
		},
		signal: func(*os.Process, syscall.Signal) error {
			return nil
		},
	}
	done := make(chan error, 1)
	go func() {
		done <- executeWithRuntime(ctx, ".", Command{Arguments: []string{"true"}}, nil, &bytes.Buffer{}, runtime)
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("unreaped force-stop returned no error")
		}
	// The bound under test is the reap window itself, so this deadline tracks
	// the constant rather than a hand-picked millisecond budget.
	case <-time.After(commandStopGrace + commandReapGrace + time.Second):
		close(waitRelease)
		<-done
		t.Fatal("post-KILL reap observation was unbounded")
	}
	close(waitRelease)
}

func TestExecuteReportsForceStopSignalFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	waitRelease := make(chan struct{})
	signalCount := 0
	runtime := commandRuntime{
		start: func(*exec.Cmd) error {
			cancel()

			return nil
		},
		wait: func(*exec.Cmd) error {
			<-waitRelease

			return nil
		},
		signal: func(*os.Process, syscall.Signal) error {
			signalCount++
			if signalCount == 2 {
				close(waitRelease)

				return errors.New("synthetic signal failure")
			}

			return nil
		},
	}
	err := executeWithRuntime(ctx, ".", Command{Arguments: []string{"true"}}, nil, &bytes.Buffer{}, runtime)
	if err == nil || !strings.Contains(err.Error(), "force-stop signal failed") {
		t.Fatalf("force-stop signal failure was not reported: %v", err)
	}
}

func TestExecuteObservesLiveGroupAfterKillFailureAndLeaderReap(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	waitRelease := make(chan struct{})
	signalCount := 0
	runtime := commandRuntime{
		start: func(*exec.Cmd) error {
			cancel()

			return nil
		},
		wait: func(*exec.Cmd) error {
			<-waitRelease

			return nil
		},
		signal: func(*os.Process, syscall.Signal) error {
			signalCount++
			if signalCount == 2 {
				close(waitRelease)

				return errors.New("synthetic kill failure")
			}

			return nil
		},
		alive: func(*os.Process) (bool, error) { return true, nil },
	}
	start := time.Now()
	err := executeWithRuntime(ctx, ".", Command{Arguments: []string{"true"}}, nil, &bytes.Buffer{}, runtime)
	if err == nil || !strings.Contains(err.Error(), "force-stop reap timed out") {
		t.Fatalf("live group after kill failure was not reported privately: %v", err)
	}
	if elapsed := time.Since(start); elapsed < commandStopGrace+commandReapGrace {
		t.Fatalf("kill failure returned before bounded group observation: %s", elapsed)
	}
}

func TestExecuteReturnsAfterTermReap(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	waitRelease := make(chan struct{})
	signalCount := 0
	runtime := commandRuntime{
		start: func(*exec.Cmd) error {
			cancel()

			return nil
		},
		wait: func(*exec.Cmd) error {
			<-waitRelease

			return nil
		},
		signal: func(*os.Process, syscall.Signal) error {
			signalCount++
			close(waitRelease)

			return nil
		},
	}
	err := executeWithRuntime(ctx, ".", Command{Arguments: []string{"true"}}, nil, &bytes.Buffer{}, runtime)
	if err == nil || !strings.Contains(err.Error(), "cancelled") || signalCount != 1 {
		t.Fatalf("TERM completion returned %v after %d signals", err, signalCount)
	}
}

func TestSignalProcessGroupAcceptsAnExitedButUnreapedGroup(t *testing.T) {
	// Darwin answers EPERM for every signal aimed at a process group whose
	// members have exited but have not been reaped yet, because no member is
	// left that the caller may signal. Retiring a group that is already gone is
	// not a failure of the conformance run that owned it.
	command := exec.Command("/bin/sh", "-c", "sleep 60")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	// Darwin starts answering EPERM as soon as the group is zombie-only, while
	// Linux keeps answering success for the same state; this short bound settles
	// the first case without spending the second's time waiting for an answer
	// that never changes.
	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		if syscall.Kill(-command.Process.Pid, 0) != nil {
			break
		}
		time.Sleep(time.Millisecond)
	}

	signalError := signalProcessGroup(command.Process, syscall.SIGTERM)
	_ = command.Wait()

	if signalError != nil {
		t.Fatalf("signalling an exited but unreaped group reported %v", signalError)
	}
}

func TestCommandGroupRetirementToleratesDelayedReaping(t *testing.T) {
	// After SIGKILL the only remaining delay is the operating system reaping the
	// group, which a loaded host stretches well past the grace a cooperating
	// child is given. Retirement must wait for that reaping instead of calling a
	// killed group a timeout.
	retired := time.Now().Add(300 * time.Millisecond)
	runtime := commandRuntime{
		start:  func(*exec.Cmd) error { return nil },
		wait:   func(*exec.Cmd) error { return nil },
		signal: func(*os.Process, syscall.Signal) error { return nil },
		alive:  func(*os.Process) (bool, error) { return time.Now().Before(retired), nil },
	}

	if err := retireCompletedCommandGroup(runtime, nil); err != nil {
		t.Fatalf("retiring a group reaped after the stop grace reported %v", err)
	}
}
