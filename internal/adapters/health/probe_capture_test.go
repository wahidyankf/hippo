package health_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	releaseguard "github.com/wahidyankf/hippo/internal/adapters/health"
	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
)

// The tests in this file and probe_http_test.go script the probe's external commands through the runner double:
// systemtest.Runner.Stdout and Failures answer a CaptureOutput call by command name, and systemtest.Runner.Captures
// records every call. They fail until the probe reads its command output through system.ProcessRunner.CaptureOutput
// instead of running the commands itself.

// argumentsOf returns the argument lists of the recorded captures of one command, in call order.
func argumentsOf(captures []systemtest.Capture, name string) [][]string {
	var calls [][]string
	for _, capture := range captures {
		if capture.Name == name {
			calls = append(calls, capture.Arguments)
		}
	}

	return calls
}

func TestServiceRSSSumsEachListenerPidOnceThroughTheRunnerDouble(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Runner.Stdout["lsof"] = []byte("101\n202\n")
	host.Runner.Stdout["ps"] = []byte("1024\n4096\n")
	probe := releaseguard.Probe{Host: host.Port()}

	if got, want := probe.ServiceRSS(context.Background(), []int{8080, 8081}), int64((1024+4096)*1024); got != want {
		t.Fatalf("ServiceRSS() = %d, want %d from the lsof and ps doubles", got, want)
	}
	lsof := argumentsOf(host.Runner.Captures, "lsof")
	if len(lsof) != 2 ||
		!slices.Equal(lsof[0], []string{"-nP", "-iTCP:8080", "-sTCP:LISTEN", "-t"}) ||
		!slices.Equal(lsof[1], []string{"-nP", "-iTCP:8081", "-sTCP:LISTEN", "-t"}) {
		t.Fatalf("lsof calls %v, want one listener query per port", lsof)
	}
	ps := argumentsOf(host.Runner.Captures, "ps")
	if len(ps) != 1 || len(ps[0]) != 4 || !slices.Equal(ps[0][:3], []string{"-o", "rss=", "-p"}) {
		t.Fatalf("ps calls %v, want one rss query", ps)
	}
	pids := strings.Split(ps[0][3], ",")
	slices.Sort(pids)
	if !slices.Equal(pids, []string{"101", "202"}) {
		t.Fatalf("ps queried pids %v, want each listener pid once", pids)
	}
}

func TestServiceRSSReadsNoPidsWhenLsofPrintsNothing(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Runner.Stdout["lsof"] = []byte("")
	probe := releaseguard.Probe{Host: host.Port()}

	if got := probe.ServiceRSS(context.Background(), []int{9}); got != 0 {
		t.Fatalf("ServiceRSS() = %d, want 0 with no listener", got)
	}
	if lsof := argumentsOf(host.Runner.Captures, "lsof"); len(lsof) != 1 ||
		!slices.Equal(lsof[0], []string{"-nP", "-iTCP:9", "-sTCP:LISTEN", "-t"}) {
		t.Fatalf("lsof calls %v, want one listener query for port 9", lsof)
	}
	if ps := argumentsOf(host.Runner.Captures, "ps"); len(ps) != 0 {
		t.Fatalf("ps ran with no listener: %v", ps)
	}
}

func TestServiceRSSIsZeroWhenLsofFails(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Runner.Failures["lsof"] = errors.New("lsof: exit status 1")
	probe := releaseguard.Probe{Host: host.Port()}

	if got := probe.ServiceRSS(context.Background(), []int{9}); got != 0 {
		t.Fatalf("ServiceRSS() = %d, want 0 when lsof fails", got)
	}
	if lsof := argumentsOf(host.Runner.Captures, "lsof"); len(lsof) != 1 {
		t.Fatalf("lsof calls %v, want one query", lsof)
	}
}

func TestServiceRSSIsZeroWhenPsFails(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Runner.Stdout["lsof"] = []byte("101\n")
	host.Runner.Failures["ps"] = errors.New("ps: exit status 1")
	probe := releaseguard.Probe{Host: host.Port()}

	if got := probe.ServiceRSS(context.Background(), []int{8080}); got != 0 {
		t.Fatalf("ServiceRSS() = %d, want 0 when ps fails", got)
	}
	if ps := argumentsOf(host.Runner.Captures, "ps"); len(ps) != 1 || !slices.Equal(ps[0], []string{"-o", "rss=", "-p", "101"}) {
		t.Fatalf("ps calls %v, want one rss query for pid 101", ps)
	}
}

func TestServiceRSSSkipsPsFieldsThatAreNotNumbers(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Runner.Stdout["lsof"] = []byte("101\n")
	host.Runner.Stdout["ps"] = []byte("x\n2\n")
	probe := releaseguard.Probe{Host: host.Port()}

	if got, want := probe.ServiceRSS(context.Background(), []int{8080}), int64(2*1024); got != want {
		t.Fatalf("ServiceRSS() = %d, want %d: a field that is not a number counts as zero", got, want)
	}
}

func TestLoadAverageFallsBackToSysctlWhenProcLoadavgCannotBeRead(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Runner.Stdout["sysctl"] = []byte("{ 0.31 0.40 0.45 }\n")
	probe := releaseguard.Probe{Host: host.Port()}

	if got := probe.LoadAverage(context.Background()); got != 0.31 {
		t.Fatalf("LoadAverage() = %v, want 0.31 from the sysctl double", got)
	}
	if sysctl := argumentsOf(host.Runner.Captures, "sysctl"); len(sysctl) != 1 ||
		!slices.Equal(sysctl[0], []string{"-n", "vm.loadavg"}) {
		t.Fatalf("sysctl calls %v, want one vm.loadavg query", sysctl)
	}
}

func TestLoadAverageFallsBackToSysctlWhenProcLoadavgIsNotANumber(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Files.Contents["/proc/loadavg"] = []byte("n/a 0.40 0.45\n")
	host.Runner.Stdout["sysctl"] = []byte("{ 1.5 1.1 0.9 }\n")
	probe := releaseguard.Probe{Host: host.Port()}

	if got := probe.LoadAverage(context.Background()); got != 1.5 {
		t.Fatalf("LoadAverage() = %v, want 1.5 from the sysctl double", got)
	}
}

func TestLoadAverageFallsBackToSysctlWhenProcLoadavgIsEmpty(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Files.Contents["/proc/loadavg"] = []byte("  \n")
	host.Runner.Stdout["sysctl"] = []byte("{ 0.9 }\n")
	probe := releaseguard.Probe{Host: host.Port()}

	if got := probe.LoadAverage(context.Background()); got != 0.9 {
		t.Fatalf("LoadAverage() = %v, want 0.9 from the sysctl double", got)
	}
}

func TestLoadAverageIsZeroWhenSysctlFails(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Runner.Failures["sysctl"] = errors.New("sysctl: exit status 1")
	probe := releaseguard.Probe{Host: host.Port()}

	if got := probe.LoadAverage(context.Background()); got != 0 {
		t.Fatalf("LoadAverage() = %v, want 0 when sysctl fails", got)
	}
	if sysctl := argumentsOf(host.Runner.Captures, "sysctl"); len(sysctl) != 1 {
		t.Fatalf("sysctl calls %v, want one query", sysctl)
	}
}

func TestLoadAverageIsZeroWhenSysctlPrintsNoNumber(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Runner.Stdout["sysctl"] = []byte("{ busy }\n")
	probe := releaseguard.Probe{Host: host.Port()}

	if got := probe.LoadAverage(context.Background()); got != 0 {
		t.Fatalf("LoadAverage() = %v, want 0 when sysctl prints no number", got)
	}
	if sysctl := argumentsOf(host.Runner.Captures, "sysctl"); len(sysctl) != 1 {
		t.Fatalf("sysctl calls %v, want one query", sysctl)
	}
}
