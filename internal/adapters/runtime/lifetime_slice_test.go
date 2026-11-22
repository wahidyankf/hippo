package runtime //nolint:testpackage // Slice (b-rest) doubles run inside the production package, as run_test.go does.

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
)

func lifetimeSliceSystem(random []byte) (*systemtest.Host, system.System) {
	host := systemtest.NewHost(time.Unix(1700000000, 0).UTC())
	host.Random.Bytes = random
	host.Runner.Self = "/test/hippo"

	return host, host.Port()
}

func lifetimeSliceConfig() RunConfig {
	return RunConfig{
		Command: "tool", WorkingDirectory: "/work", Arguments: []string{"--serve"},
		ChildStdin: strings.NewReader(""), ChildStdout: io.Discard, ChildStderr: io.Discard,
	}
}

func TestTokenReadsTheDouble(t *testing.T) {
	_, port := lifetimeSliceSystem([]byte("0123456789abcdef"))
	got, err := token(port)
	if err != nil {
		t.Fatalf("token from the double: %v", err)
	}
	if !sessionTokenPattern.MatchString(got) {
		t.Fatalf("token = %q, want 32 hex characters", got)
	}
	if want := "30313233343536373839616263646566"; got != want {
		t.Fatalf("token = %q, want the hex of the double bytes %q", got, want)
	}
}

func TestTokenReportsDoubleFailure(t *testing.T) {
	_, port := lifetimeSliceSystem(nil)
	if _, err := token(port); err == nil {
		t.Fatal("token without double bytes = nil, want error")
	}
}

func launcherSliceEnvironment(capability string) []string {
	return []string{
		"PATH=/bin", lifetimeLauncherEnvironment + "=1", lifetimeCapabilityEnvironment + "=" + capability,
	}
}

func launcherSliceArguments() []string {
	return []string{"hippo", lifetimeLauncherArgument, "/work", "/bin/tool", "--serve"}
}

func TestLifetimeLauncherAuthorized(t *testing.T) {
	capability := "0123456789abcdef0123456789abcdef"
	pipes := func(uintptr) bool { return true }
	if !lifetimeLauncherAuthorized(launcherSliceArguments(), launcherSliceEnvironment(capability), pipes) {
		t.Fatal("authorized launcher = false, want true")
	}
	for name, test := range map[string]struct {
		arguments, environment []string
		isPipe                 func(uintptr) bool
	}{
		"report pipe is not a pipe": {
			launcherSliceArguments(), launcherSliceEnvironment(capability),
			func(descriptor uintptr) bool { return descriptor != 3 },
		},
		"capability pipe is not a pipe": {
			launcherSliceArguments(), launcherSliceEnvironment(capability),
			func(descriptor uintptr) bool { return descriptor != 4 },
		},
		"launcher flag is missing": {
			[]string{"hippo", "/work", "/bin/tool"}, launcherSliceEnvironment(capability), pipes,
		},
		"launcher variable is missing": {
			launcherSliceArguments(), []string{"PATH=/bin"}, pipes,
		},
		"capability is malformed": {
			launcherSliceArguments(), launcherSliceEnvironment("not-a-token"), pipes,
		},
	} {
		if lifetimeLauncherAuthorized(test.arguments, test.environment, test.isPipe) {
			t.Errorf("%s: authorized = true, want false", name)
		}
	}
}

func TestStartSupervisedLifetimeReachesTheRunnerDouble(t *testing.T) {
	host, port := lifetimeSliceSystem([]byte("0123456789abcdef"))
	_, err := startSupervisedLifetimeWithLauncher(
		port, context.Background(), lifetimeSliceConfig(), "/bin/tool", []string{"K=V"},
		"/test/hippo", time.Second,
	)
	if err == nil {
		t.Fatal("launch through the double = nil, want the double start failure")
	}
	if host.Files.Pipes != 2 {
		t.Fatalf("pipes opened = %d, want the report and capability pipes", host.Files.Pipes)
	}
}

func TestStartSupervisedLifetimeReportsPipeFailure(t *testing.T) {
	host, port := lifetimeSliceSystem([]byte("0123456789abcdef"))
	host.Files.PipeErr = errors.New("systemtest: no pipes")
	_, err := startSupervisedLifetimeWithLauncher(
		port, context.Background(), lifetimeSliceConfig(), "/bin/tool", []string{"K=V"},
		"/test/hippo", time.Second,
	)
	if err == nil || !strings.Contains(err.Error(), "no pipes") {
		t.Fatalf("launch without pipes = %v, want the pipe failure", err)
	}
}

func TestStartSupervisedLifetimeReportsTokenFailure(t *testing.T) {
	host, port := lifetimeSliceSystem(nil)
	_, err := startSupervisedLifetimeWithLauncher(
		port, context.Background(), lifetimeSliceConfig(), "/bin/tool", []string{"K=V"},
		"/test/hippo", time.Second,
	)
	if err == nil {
		t.Fatal("launch without double bytes = nil, want the token failure")
	}
	if host.Files.Pipes != 2 {
		t.Fatalf("pipes opened = %d, want two before the token failure", host.Files.Pipes)
	}
}

func TestStartSupervisedLifetimeHonoursCancellation(t *testing.T) {
	host, port := lifetimeSliceSystem([]byte("0123456789abcdef"))
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := startSupervisedLifetimeWithLauncher(
		port, cancelled, lifetimeSliceConfig(), "/bin/tool", []string{"K=V"},
		"/test/hippo", time.Second,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled launch = %v, want context canceled", err)
	}
	if host.Files.Pipes != 0 {
		t.Fatalf("pipes opened = %d, want none after cancellation", host.Files.Pipes)
	}
}

func TestAwaitLifetimeHandshakeDecodesTheReport(t *testing.T) {
	handshake, err := awaitLifetimeHandshake(
		context.Background(), strings.NewReader(`{"processGroup":4242}`), time.Second,
	)
	if err != nil || handshake.ProcessGroup != 4242 {
		t.Fatalf("handshake = %+v, %v, want group 4242", handshake, err)
	}
}

func TestAwaitLifetimeHandshakeHonoursCancellation(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() { _ = writer.Close() }()
	defer func() { _ = reader.Close() }()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	before := time.Now()
	_, err := awaitLifetimeHandshake(cancelled, reader, time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled handshake = %v, want context canceled", err)
	}
	t.Logf("cancelled handshake waited %s", time.Since(before))
}

func TestStartSupervisedLifetimeResolvesTheLauncherThroughTheDouble(t *testing.T) {
	host, port := lifetimeSliceSystem(nil)
	if _, err := startSupervisedLifetime(
		port, context.Background(), lifetimeSliceConfig(), "/bin/tool", []string{"K=V"},
	); err == nil {
		t.Fatal("start without double bytes = nil, want the token failure")
	}
	if host.Runner.Self != "/test/hippo" {
		t.Fatalf("runner self = %q, want the double path", host.Runner.Self)
	}
}
