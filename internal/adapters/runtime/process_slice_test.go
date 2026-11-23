package runtime //nolint:testpackage // Slice (b) doubles run inside the production package, as run_test.go does.

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
	"github.com/wahidyankf/hippo/internal/policy"
)

func processSliceSystem() (*systemtest.Host, system.System) {
	host := systemtest.NewHost(time.Unix(1700000000, 0).UTC())

	return host, host.Port()
}

// retireAfter schedules the lifetime exit once the stop had time to signal the
// group. The buffered channel keeps the timer callback from blocking, and the
// channel receive inside terminateAndWait orders every recorded signal before
// the test reads them, so no polling races the double.
func retireAfter(exited chan error, delay time.Duration, err error) {
	time.AfterFunc(delay, func() { exited <- err })
}

func TestSignalGroupRefusesInvalidGroup(t *testing.T) {
	_, port := processSliceSystem()
	if err := signalGroup(port, 0, syscall.SIGTERM); err == nil {
		t.Fatal("signal group zero = nil, want error")
	}
}

func TestSignalGroupTreatsMissingGroupAsStopped(t *testing.T) {
	host, port := processSliceSystem()
	if err := signalGroup(port, 4242, syscall.SIGTERM); err != nil {
		t.Fatalf("signal missing group: %v", err)
	}
	if len(host.Processes.Sent) != 1 || host.Processes.Sent[0].Signal != syscall.SIGTERM {
		t.Fatalf("sent signals = %+v, want one SIGTERM", host.Processes.Sent)
	}
	if host.Processes.Sent[0].Pid != -4242 {
		t.Fatalf("signalled pid = %d, want the group -4242", host.Processes.Sent[0].Pid)
	}
}

func TestTerminateAndWaitReturnsAnAlreadyExitedChild(t *testing.T) {
	host, port := processSliceSystem()
	exited := make(chan error, 1)
	exited <- errors.New("child failed")
	waitError, signalError := terminateAndWait(
		port, &supervisedLifetime{processGroup: 4242, exited: exited}, time.Minute, time.Minute,
	)
	if waitError == nil || waitError.Error() != "child failed" {
		t.Fatalf("wait error = %v, want the child failure", waitError)
	}
	if signalError != nil {
		t.Fatalf("signal error = %v, want nil for an exited child", signalError)
	}
	if len(host.Processes.Sent) != 0 {
		t.Fatalf("sent signals = %+v, want none", host.Processes.Sent)
	}
}

func TestTerminateAndWaitConfirmsAfterTermination(t *testing.T) {
	host, port := processSliceSystem()
	// The double answers every group signal, so the test drives the lifetime
	// exit once the termination signal is observed.
	host.Processes.Alive[-4242] = true
	exited := make(chan error, 1)
	retireAfter(exited, 200*time.Millisecond, nil)
	waitError, signalError := terminateAndWait(
		port, &supervisedLifetime{processGroup: 4242, exited: exited}, time.Minute, time.Minute,
	)
	if waitError != nil || signalError != nil {
		t.Fatalf("stop = %v, %v, want clean retirement", waitError, signalError)
	}
	if len(host.Processes.Sent) != 1 {
		t.Fatalf("sent signals = %+v, want only SIGTERM", host.Processes.Sent)
	}
}

func TestTerminateAndWaitEscalatesToKill(t *testing.T) {
	host, port := processSliceSystem()
	host.Processes.Alive[-4242] = true
	waitError, signalError := terminateAndWait(
		port, &supervisedLifetime{processGroup: 4242, exited: make(chan error)},
		10*time.Millisecond, 50*time.Millisecond,
	)
	if !errors.Is(signalError, errChildRetirementUnconfirmed) {
		t.Fatalf("signal error = %v, want unconfirmed retirement", signalError)
	}
	if waitError != nil {
		t.Fatalf("wait error = %v, want nil with an unconfirmed retirement", waitError)
	}
	if len(host.Processes.Sent) != 2 || host.Processes.Sent[1].Signal != syscall.SIGKILL {
		t.Fatalf("sent signals = %+v, want SIGTERM then SIGKILL", host.Processes.Sent)
	}
}

func TestStopConfiguredLifetimeHonoursTheHook(t *testing.T) {
	_, port := processSliceSystem()
	called := 0
	config := RunConfig{
		Policy: policy.DefaultPolicy(),
		stopLifetime: func(_ *supervisedLifetime, _ time.Duration) (error, error) {
			called++

			//nolint:nilnil // The hook models a stop that ran with nothing to report.
			return nil, nil
		},
	}
	if _, err := stopConfiguredLifetime(
		port, config, &supervisedLifetime{processGroup: 4242, exited: make(chan error)},
	); err != nil {
		t.Fatalf("stop configured lifetime: %v", err)
	}
	if called != 1 {
		t.Fatalf("hook calls = %d, want one", called)
	}
}

func TestCheckCommandIsRunnable(t *testing.T) {
	host, port := processSliceSystem()
	host.Runner.Executables["tool"] = "/bin/tool"
	host.Files.Contents["/bin/tool"] = []byte("x")
	host.Files.Modes["/bin/tool"] = 0o755
	if failure := checkCommandIsRunnable(port, "tool"); failure != nil {
		t.Fatalf("runnable command failed: %v", failure)
	}
	if failure := checkCommandIsRunnable(port, "missing"); failure == nil {
		t.Fatal("missing command = nil, want a failure")
	}
	host.Runner.Executables["gone"] = "/bin/gone"
	if failure := checkCommandIsRunnable(port, "gone"); failure == nil {
		t.Fatal("unresolvable command = nil, want a failure")
	}
	host.Files.Dirs["/bin/dir"] = true
	host.Runner.Executables["dir"] = "/bin/dir"
	if failure := checkCommandIsRunnable(port, "dir"); failure == nil {
		t.Fatal("directory command = nil, want a failure")
	}
	host.Files.Contents["/bin/plain"] = []byte("x")
	host.Files.Modes["/bin/plain"] = 0o644
	host.Runner.Executables["plain"] = "/bin/plain"
	if failure := checkCommandIsRunnable(port, "plain"); failure == nil {
		t.Fatal("non-executable command = nil, want a failure")
	}
}

func TestExecutableGuardPathUsesTheRunner(t *testing.T) {
	host, port := processSliceSystem()
	host.Runner.Self = "/test/hippo"
	if path := executableGuardPath(port); path != "/test/hippo" {
		t.Fatalf("guard path = %q, want the runner path", path)
	}
}

func TestLaunchConfiguredLifetimeRunsTheStarterHook(t *testing.T) {
	host, port := processSliceSystem()
	host.Runner.Executables["tool"] = "/bin/tool"
	host.Runner.Self = "/test/hippo"
	var seenExecutable, seenSession string
	config := RunConfig{
		Command: "tool",
		startLifetime: func(
			_ context.Context, _ RunConfig, executable string, environment []string, _ ...*os.File,
		) (*supervisedLifetime, error) {
			seenExecutable = executable
			seenSession = environmentValue(environment, "HIPPO_SESSION")

			return &supervisedLifetime{}, nil
		},
	}
	session := &Session{Token: "session-token"}
	if _, err := launchConfiguredLifetime(port, context.Background(), config, session, nil); err != nil {
		t.Fatalf("launch configured lifetime: %v", err)
	}
	if seenExecutable != "/bin/tool" {
		t.Fatalf("starter executable = %q, want the resolved tool", seenExecutable)
	}
	if seenSession != "session-token" {
		t.Fatalf("starter session = %q, want the session token", seenSession)
	}
}

func TestLaunchConfiguredLifetimeRefusesAnUnknownCommand(t *testing.T) {
	_, port := processSliceSystem()
	config := RunConfig{Command: "missing"}
	if _, err := launchConfiguredLifetime(port, context.Background(), config, &Session{}, nil); err == nil {
		t.Fatal("launch unknown command = nil, want error")
	}
}

func TestSignalGroupReturnsAnUnclassifiedKillError(t *testing.T) {
	_, port := processSliceSystem()
	port.Processes.Kill = func(pid int, signal syscall.Signal) error {
		if pid != -4242 || signal != syscall.SIGTERM {
			t.Errorf("signalled pid = %d signal = %v, want the group -4242 SIGTERM", pid, signal)
		}

		return syscall.EIO
	}
	if err := signalGroup(port, 4242, syscall.SIGTERM); !errors.Is(err, syscall.EIO) {
		t.Fatalf("signal unclassified error = %v, want the EIO", err)
	}
}

func TestWaitStatusCodeFallsBackToOne(t *testing.T) {
	if code := waitStatusCode(errors.New("boom")); code != 1 {
		t.Fatalf("wait status code = %d, want one for an unclassified error", code)
	}
}

func TestCheckCommandIsRunnableRefusesAnUnclassifiedLookupError(t *testing.T) {
	_, port := processSliceSystem()
	port.Runner.LookPath = func(string) (string, error) { return "", errors.New("lookup exploded") }
	if failure := checkCommandIsRunnable(port, "tool"); failure == nil {
		t.Fatal("unclassified lookup error = nil, want a failure")
	}
}

func TestCheckCommandIsRunnableRefusesAnUnclassifiedStatError(t *testing.T) {
	host, port := processSliceSystem()
	host.Runner.Executables["tool"] = "/bin/tool"
	port.Files.Stat = func(string) (fs.FileInfo, error) { return nil, errors.New("stat exploded") }
	if failure := checkCommandIsRunnable(port, "tool"); failure == nil {
		t.Fatal("unclassified stat error = nil, want a failure")
	}
}

func TestExecutableGuardPathFallsBackToEmptyOnRunnerError(t *testing.T) {
	_, port := processSliceSystem()
	port.Runner.Executable = func() (string, error) { return "", errors.New("runner exploded") }
	if path := executableGuardPath(port); path != "" {
		t.Fatalf("guard path = %q, want empty when the runner fails", path)
	}
}

func TestWaitForContext(t *testing.T) {
	_, port := processSliceSystem()
	paused := 0
	pause := func(_ time.Duration) { paused++ }
	if err := waitForContext(context.Background(), port.Clock, time.Second, pause); err != nil {
		t.Fatalf("wait with pause: %v", err)
	}
	if paused != 1 {
		t.Fatalf("pause calls = %d, want one", paused)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitForContext(cancelled, port.Clock, time.Minute, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait cancelled = %v, want context canceled", err)
	}
	if err := waitForContext(context.Background(), port.Clock, time.Millisecond, nil); err != nil {
		t.Fatalf("wait elapsed: %v", err)
	}
}
