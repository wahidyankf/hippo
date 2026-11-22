package health_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	releaseguard "github.com/wahidyankf/hippo/internal/adapters/health"
	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
)

// probeTarget is the URL the recorded curl arguments must carry. Port 9 is the discard port, normally closed.
const probeTarget = "http://127.0.0.1:9/health/ready"

func TestLocalProbeReadsStatusAndMillisecondsFromTheCurlCapture(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Runner.Stdout["curl"] = []byte("200 0.250\n")
	probe := releaseguard.Probe{Host: host.Port()}

	status, millis := probe.Local(context.Background(), probeTarget)
	if status != 200 || millis != 250 {
		t.Fatalf("Local() = %d, %v; want 200, 250 from the curl double", status, millis)
	}
	curl := argumentsOf(host.Runner.Captures, "curl")
	if len(curl) != 1 || !slices.Equal(curl[0], releaseguard.HTTPProbeArguments(probeTarget, false)) {
		t.Fatalf("curl calls %v, want the local probe arguments once", curl)
	}
}

func TestRoutedProbeReadsStatusAndMillisecondsFromTheCurlCapture(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Runner.Stdout["curl"] = []byte("301 0.500\n")
	probe := releaseguard.Probe{Host: host.Port()}

	status, millis := probe.Routed(context.Background(), probeTarget)
	if status != 301 || millis != 500 {
		t.Fatalf("Routed() = %d, %v; want 301, 500 from the curl double", status, millis)
	}
	curl := argumentsOf(host.Runner.Captures, "curl")
	if len(curl) != 1 || !slices.Equal(curl[0], releaseguard.HTTPProbeArguments(probeTarget, true)) {
		t.Fatalf("curl calls %v, want the routed probe arguments once", curl)
	}
}

func TestLocalProbeReportsUnreachableWhenCurlFails(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Runner.Failures["curl"] = errors.New("curl: exit status 7")
	probe := releaseguard.Probe{Host: host.Port()}

	status, millis := probe.Local(context.Background(), probeTarget)
	if status != 0 || millis != 3000 {
		t.Fatalf("Local() = %d, %v; want 0, 3000 when curl fails", status, millis)
	}
	if curl := argumentsOf(host.Runner.Captures, "curl"); len(curl) != 1 {
		t.Fatalf("curl calls %v, want one probe", curl)
	}
}

func TestLocalProbeReportsUnreachableWhenCurlPrintsOneField(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Runner.Stdout["curl"] = []byte("200\n")
	probe := releaseguard.Probe{Host: host.Port()}

	status, millis := probe.Local(context.Background(), probeTarget)
	if status != 0 || millis != 3000 {
		t.Fatalf("Local() = %d, %v; want 0, 3000 when curl prints one field", status, millis)
	}
	if curl := argumentsOf(host.Runner.Captures, "curl"); len(curl) != 1 {
		t.Fatalf("curl calls %v, want one probe", curl)
	}
}

func TestRoutedProbeReportsUnreachableWhenLatencyIsNotANumber(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Runner.Stdout["curl"] = []byte("200 slow\n")
	probe := releaseguard.Probe{Host: host.Port()}

	status, millis := probe.Routed(context.Background(), probeTarget)
	if status != 0 || millis != 3000 {
		t.Fatalf("Routed() = %d, %v; want 0, 3000 when the latency is not a number", status, millis)
	}
	if curl := argumentsOf(host.Runner.Captures, "curl"); len(curl) != 1 {
		t.Fatalf("curl calls %v, want one probe", curl)
	}
}
