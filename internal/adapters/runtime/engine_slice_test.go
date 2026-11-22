package runtime //nolint:testpackage // Slice (b-rest) doubles run inside the production package, as run_test.go does.

import (
	"context"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
	"github.com/wahidyankf/hippo/internal/application"
)

func engineSliceSystem() (*systemtest.Host, system.System) {
	host := systemtest.NewHost(time.Unix(1700000000, 0).UTC())

	return host, host.Port()
}

func TestDefaultRunEnvironmentTakesStreamsFromThePort(t *testing.T) {
	host, port := engineSliceSystem()
	host.Environment.Temp = "/fake-tmp"
	host.Environment.Values["CI"] = "true"
	got := defaultRunEnvironment(port, application.RunConfig{})
	if got.ChildStdin != port.Environment.Stdin {
		t.Error("child stdin is not the port stdin")
	}
	if got.ChildStdout != port.Environment.Stdout {
		t.Error("child stdout is not the port stdout")
	}
	if got.ChildStderr != port.Environment.Stderr || got.Stderr != port.Environment.Stderr {
		t.Error("stderr is not the port stderr")
	}
	if len(got.Environment) != 1 || got.Environment[0] != "CI=true" {
		t.Errorf("environment = %q, want the port values", got.Environment)
	}
	if want := "/fake-tmp/hippo-port-leases"; got.PortLeaseRoot != want {
		t.Errorf("port lease root = %q, want %q", got.PortLeaseRoot, want)
	}
}

func TestDefaultRunEnvironmentKeepsCallerValues(t *testing.T) {
	_, port := engineSliceSystem()
	input := &callerReader{}
	output := &callerWriter{}
	config := defaultRunEnvironment(port, application.RunConfig{
		Environment: []string{"K=V"}, PortLeaseRoot: "/caller-root", ChildStdin: input,
		ChildStdout: output, ChildStderr: output, Stderr: output,
	})
	if len(config.Environment) != 1 || config.Environment[0] != "K=V" {
		t.Errorf("environment = %q, want the caller value", config.Environment)
	}
	if config.PortLeaseRoot != "/caller-root" || config.ChildStdin != input {
		t.Error("caller lease root or stdin was replaced")
	}
	if config.ChildStdout != output || config.ChildStderr != output || config.Stderr != output {
		t.Error("caller writers were replaced")
	}
}

type callerReader struct{}

func (*callerReader) Read([]byte) (int, error) { return 0, nil }

type callerWriter struct{}

func (*callerWriter) Write(data []byte) (int, error) { return len(data), nil }

func TestRunClockNowReadsTheDouble(t *testing.T) {
	host, port := engineSliceSystem()
	clock := RunClock{System: &port}
	if got := clock.Now(); !got.Equal(host.Clock.Current) {
		t.Fatalf("run clock now = %v, want %v", got, host.Clock.Current)
	}
}

func TestRunClockZeroValueKeepsProductionTime(t *testing.T) {
	before := time.Now()
	got := RunClock{}.Now()
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Fatalf("zero run clock now = %v, want within [%v, %v]", got, before, after)
	}
}

func TestRunClockWaitPreservesThePauseSeam(t *testing.T) {
	_, port := engineSliceSystem()
	clock := RunClock{System: &port}
	paused := 0
	if err := clock.Wait(context.Background(), time.Second, func(time.Duration) { paused++ }); err != nil {
		t.Fatalf("run clock wait with pause: %v", err)
	}
	if paused != 1 {
		t.Fatalf("pause calls = %d, want one", paused)
	}
	if err := clock.Wait(context.Background(), time.Millisecond, nil); err != nil {
		t.Fatalf("run clock wait elapsed: %v", err)
	}
}

func TestRunClockTickerBindsTheDouble(t *testing.T) {
	host, port := engineSliceSystem()
	clock := RunClock{System: &port}
	ticker := clock.Ticker(time.Second)
	select {
	case sample := <-ticker.Ticks:
		if !sample.Equal(host.Clock.Current) {
			t.Fatalf("ticker sample = %v, want %v", sample, host.Clock.Current)
		}
	default:
		t.Fatal("ticker has no buffered sample")
	}
	ticker.Stop()
	if host.Clock.Stops != 1 {
		t.Fatalf("ticker stops = %d, want one", host.Clock.Stops)
	}
}
