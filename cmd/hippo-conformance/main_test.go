package main

import (
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
)

// conformanceUsage is the line ConformanceMain writes to its error stream when it is given the wrong arguments.
const conformanceUsage = "usage: hippo-conformance <manifest.json>\n"

func TestRunReportsUsageAndReturnsTwoForBadArguments(t *testing.T) {
	host := systemtest.NewHost(time.Time{})
	host.Environment.Arguments = []string{"hippo-conformance"}

	code := run(host.Port())

	if code != 2 {
		t.Fatalf("run() = %d, want 2", code)
	}
	if got := host.Environment.Errors.String(); got != conformanceUsage {
		t.Fatalf("stderr = %q, want %q", got, conformanceUsage)
	}
	if got := host.Environment.Output.String(); got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
	if host.Signals.Stopped != 1 {
		t.Fatalf("signal registrations stopped = %d, want 1", host.Signals.Stopped)
	}
}

func TestMainExitsWithTheStatusOfRun(t *testing.T) {
	host := systemtest.NewHost(time.Time{})
	host.Environment.Arguments = []string{"hippo-conformance"}
	original := process
	t.Cleanup(func() { process = original })
	process = host.Port()

	main()

	if len(host.Environment.Exits) != 1 || host.Environment.Exits[0] != 2 {
		t.Fatalf("exit codes = %v, want [2]", host.Environment.Exits)
	}
	if got := host.Environment.Errors.String(); got != conformanceUsage {
		t.Fatalf("stderr = %q, want %q", got, conformanceUsage)
	}
}
