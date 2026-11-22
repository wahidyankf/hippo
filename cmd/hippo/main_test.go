package main

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
)

func TestRunReturnsTheCommandStatusAndStopsSignals(t *testing.T) {
	host := systemtest.NewHost(time.Time{})
	host.Environment.Arguments = []string{"hippo", "--version"}

	code := run(host.Port())

	if code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}
	if got := host.Environment.Output.String(); !strings.Contains(got, cli.Version) {
		t.Fatalf("stdout = %q, want it to contain %q", got, cli.Version)
	}
	if host.Signals.Stopped != 1 {
		t.Fatalf("signal registrations stopped = %d, want 1", host.Signals.Stopped)
	}
}

func TestMainExitsWithTheStatusOfRun(t *testing.T) {
	host := systemtest.NewHost(time.Time{})
	host.Environment.Arguments = []string{"hippo", "--version"}
	original := process
	t.Cleanup(func() { process = original })
	process = host.Port()

	main()

	if want := []int{0}; !reflect.DeepEqual(host.Environment.Exits, want) {
		t.Fatalf("exit codes = %v, want %v", host.Environment.Exits, want)
	}
}
