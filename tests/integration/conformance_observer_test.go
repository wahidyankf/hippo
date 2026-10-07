package integration_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type conformancePIDObservation struct {
	pid          int
	runnerExited bool
	runnerError  error
}

func readConformancePID(path string) int {
	data, err := os.ReadFile(path)
	var pid int
	if err == nil {
		_, _ = fmt.Sscan(string(data), &pid)
	}
	return pid
}

// observeConformancePID keeps the fixture's original PID deadline while giving
// an owned runner's completed output a synchronization boundary.
func observeConformancePID(path string, runnerDone <-chan error, output *bytes.Buffer, deadline time.Time) (conformancePIDObservation, error) {
	return observeConformancePIDWithReader(path, runnerDone, output, deadline, func() int { return readConformancePID(path) })
}

func observeConformancePIDWithReader(path string, runnerDone <-chan error, output *bytes.Buffer, deadline time.Time, readPID func() int) (conformancePIDObservation, error) {
	for {
		if pid := readPID(); pid > 0 {
			return conformancePIDObservation{pid: pid}, nil
		}
		select {
		case runnerError := <-runnerDone:
			observation := conformancePIDObservation{pid: readPID(), runnerExited: true, runnerError: runnerError}
			if observation.pid > 0 {
				return observation, nil
			}
			return observation, completedConformancePIDError(path, runnerError, output)
		default:
		}
		if time.Now().After(deadline) {
			return conformancePIDObservation{}, errors.New("conformance descendant did not report its PID before the existing deadline; runner still running")
		}
		time.Sleep(time.Millisecond)
	}
}

type conformancePIDObservationError struct {
	message string
	cause   error
}

func (failure *conformancePIDObservationError) Error() string { return failure.message }
func (failure *conformancePIDObservationError) Unwrap() error { return failure.cause }

func completedConformancePIDError(path string, runnerError error, output *bytes.Buffer) error {
	completedOutput := ""
	if output != nil {
		completedOutput = output.String()
	}
	diagnostic := fmt.Sprintf("conformance runner exited before descendant reported PID: %v; output=%q", runnerError, completedOutput)
	root := filepath.Dir(path)
	diagnostic = strings.ReplaceAll(diagnostic, root, "[fixture]")
	if canonical, err := filepath.EvalSymlinks(root); err == nil {
		diagnostic = strings.ReplaceAll(diagnostic, canonical, "[fixture]")
	}
	const diagnosticLimit = 4096
	if len(diagnostic) > diagnosticLimit {
		diagnostic = diagnostic[:diagnosticLimit] + " [truncated]"
	}
	return &conformancePIDObservationError{message: diagnostic, cause: runnerError}
}

func TestConformancePIDObserverReportsOwnedRunnerExit(t *testing.T) {
	root := t.TempDir()
	done := make(chan error, 1)
	runnerError := errors.New("owned runner exit 17")
	done <- runnerError
	output := bytes.NewBufferString("manifest refused before gate launch")
	observation, err := observeConformancePID(filepath.Join(root, "pid"), done, output, time.Now())
	pid := observation.pid
	if pid != 0 || !errors.Is(err, runnerError) || !strings.Contains(err.Error(), "runner exited before descendant reported PID") || !strings.Contains(err.Error(), "manifest refused before gate launch") {
		t.Fatalf("early runner failure lost its cause/output: pid=%d error=%v", pid, err)
	}
}

func TestConformancePIDObserverDistinguishesLiveDeadline(t *testing.T) {
	root := t.TempDir()
	done := make(chan error, 1)
	output := bytes.NewBufferString("output still being written by the live runner")
	observation, err := observeConformancePID(filepath.Join(root, "pid"), done, output, time.Now())
	pid := observation.pid
	if pid != 0 || err == nil || !strings.Contains(err.Error(), "runner still running") || strings.Contains(err.Error(), output.String()) {
		t.Fatalf("live deadline read unfinished output or lacked state: pid=%d error=%v", pid, err)
	}
}

func TestConformancePIDObserverKeepsCompletedPID(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "pid")
	if err := os.WriteFile(path, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	done <- errors.New("completed runner")
	observation, err := observeConformancePID(path, done, &bytes.Buffer{}, time.Now())
	pid := observation.pid
	if pid != 12345 || err != nil || len(done) != 1 {
		t.Fatalf("reported PID/completion lost: pid=%d error=%v done=%d", pid, err, len(done))
	}
}

func TestConformancePIDObserverBoundsAndRedactsCompletedOutput(t *testing.T) {
	root := t.TempDir()
	done := make(chan error, 1)
	done <- fmt.Errorf("owned runner path %s", root)
	output := bytes.NewBufferString("fixture=" + root + "; " + strings.Repeat("detail ", 1000))
	_, err := observeConformancePID(filepath.Join(root, "pid"), done, output, time.Now())
	if err == nil || strings.Contains(err.Error(), root) || !strings.Contains(err.Error(), "[fixture]") || len(err.Error()) > 4500 {
		t.Fatalf("completed diagnostic is unbounded or exposes private fixture: error=%v", err)
	}
}

func TestConformancePIDObserverReportsSuccessfulRunnerWithoutPID(t *testing.T) {
	root := t.TempDir()
	done := make(chan error, 1)
	done <- nil
	_, err := observeConformancePID(filepath.Join(root, "pid"), done, bytes.NewBufferString("runner completed"), time.Now())
	if err == nil || !strings.Contains(err.Error(), "runner exited before descendant reported PID") || !strings.Contains(err.Error(), "runner completed") {
		t.Fatalf("successful runner without required descendant PID was accepted: %v", err)
	}
}

func TestConformancePIDObserverRereadsPublicationBeforeRunnerCompletion(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "pid")
	done := make(chan error, 1)
	runnerError := errors.New("owned runner exit 9")
	reads := 0
	readPID := func() int {
		reads++
		if reads == 1 {
			if err := os.WriteFile(path, []byte("12345"), 0o600); err != nil {
				t.Fatal(err)
			}
			done <- runnerError
			return 0
		}
		return readConformancePID(path)
	}
	observation, err := observeConformancePIDWithReader(path, done, &bytes.Buffer{}, time.Now(), readPID)
	if err != nil || observation.pid != 12345 || !observation.runnerExited || !errors.Is(observation.runnerError, runnerError) || reads != 2 || len(done) != 0 {
		t.Fatalf("PID publication raced completed runner: observation=%+v error=%v reads=%d remaining-completion=%d", observation, err, reads, len(done))
	}
}

// These callbacks are bound only to the runner this fixture started. Its own
// cancellation path retires command groups; the fixture never signals arbitrary
// PIDs or process groups while diagnosing a missing descendant marker.
type conformanceRunnerControl struct {
	terminate func() error
	kill      func() error
	done      <-chan error
}

func stopConformanceRunner(control conformanceRunnerControl, terminationGrace, reapGrace time.Duration) error {
	terminateError := control.terminate()
	timer := time.NewTimer(terminationGrace)
	defer timer.Stop()
	select {
	case <-control.done:
		return terminateError
	case <-timer.C:
	}
	killError := control.kill()
	reapTimer := time.NewTimer(reapGrace)
	defer reapTimer.Stop()
	select {
	case <-control.done:
		return errors.Join(terminateError, killError)
	case <-reapTimer.C:
		return errors.Join(terminateError, killError, errors.New("owned conformance runner cleanup remained unconfirmed"))
	}
}

func TestConformanceRunnerCleanupGracefulCompletion(t *testing.T) {
	done := make(chan error, 1)
	terminated, killed := 0, 0
	control := conformanceRunnerControl{terminate: func() error { terminated++; done <- nil; return nil }, kill: func() error { killed++; return nil }, done: done}
	err := stopConformanceRunner(control, time.Hour, time.Hour)
	if err != nil || terminated != 1 || killed != 0 || len(done) != 0 {
		t.Fatalf("graceful cleanup error=%v terminate=%d kill=%d pending=%d", err, terminated, killed, len(done))
	}
}

func TestConformanceRunnerCleanupPreservesSignalFailure(t *testing.T) {
	done := make(chan error, 1)
	terminated, killed := 0, 0
	cause := errors.New("owned runner signal failed")
	control := conformanceRunnerControl{terminate: func() error { terminated++; done <- nil; return cause }, kill: func() error { killed++; return nil }, done: done}
	err := stopConformanceRunner(control, time.Hour, time.Hour)
	if !errors.Is(err, cause) || terminated != 1 || killed != 0 || len(done) != 0 {
		t.Fatalf("signal failure lost: error=%v terminate=%d kill=%d pending=%d", err, terminated, killed, len(done))
	}
}

func TestConformanceRunnerCleanupForceStopsAfterGrace(t *testing.T) {
	done := make(chan error, 1)
	terminated, killed := 0, 0
	cause := errors.New("owned runner kill failed")
	control := conformanceRunnerControl{terminate: func() error { terminated++; return nil }, kill: func() error { killed++; done <- nil; return cause }, done: done}
	err := stopConformanceRunner(control, 0, time.Hour)
	if !errors.Is(err, cause) || terminated != 1 || killed != 1 || len(done) != 0 {
		t.Fatalf("forced cleanup error=%v terminate=%d kill=%d pending=%d", err, terminated, killed, len(done))
	}
}

func TestConformanceRunnerCleanupRefusesUnconfirmedCompletion(t *testing.T) {
	done := make(chan error, 1)
	terminated, killed := 0, 0
	control := conformanceRunnerControl{terminate: func() error { terminated++; return nil }, kill: func() error { killed++; return nil }, done: done}
	err := stopConformanceRunner(control, 0, 0)
	if err == nil || !strings.Contains(err.Error(), "owned conformance runner cleanup remained unconfirmed") || terminated != 1 || killed != 1 {
		t.Fatalf("unconfirmed cleanup error=%v terminate=%d kill=%d", err, terminated, killed)
	}
}
