package tui_test

import (
	"testing"
	"time"

	"github.com/wahidyankf/hippo/tests/support"
)

// Constructed receipts exercise the exact predicates consumed by public BDD
// bindings. They do not establish a native observation or lifecycle acceptance.
func TestPublicRefreshPlatformEvidenceLinuxCompletion(t *testing.T) {
	trace := publicRefreshLinuxControlTrace()
	family := publicRefreshControlBracket()
	lower, ok := publicRefreshCompletionLower("linux", trace, 101, family)
	if !ok || !lower.Equal(family.probeBefore.Add(8*time.Millisecond)) {
		t.Fatalf("authentic Linux final stat output lower bound refused: lower=%s accepted=%t", lower, ok)
	}
}

func TestPublicRefreshPlatformEvidenceLinuxCancellation(t *testing.T) {
	before, after := publicRefreshLinuxCancelControlTrace()
	if !publicRefreshCancellation("linux", before, after, 1, 101) {
		t.Fatal("authentic held meminfo writer ERR/no NVAL cancellation refused")
	}
}

func publicRefreshControlBracket() *publicBDDInteractive {
	origin := time.Unix(100, 0).UTC()
	return &publicBDDInteractive{probeBefore: origin, probeAfter: origin.Add(time.Millisecond), completedUpper: origin.Add(time.Second)}
}

func publicRefreshLinuxControlTrace() support.PublicTUIHostProbeTrace {
	trace := support.PublicTUIHostProbeTrace{Readings: 2, MaximumActive: 1}
	for index := range 8 {
		operation := "meminfo"
		if index%4 >= 2 {
			operation = "stat"
		}
		phase := "start"
		if index%2 == 1 {
			phase = "output"
		}
		trace.Events = append(trace.Events, support.PublicTUIHostProbeEvent{Sequence: index + 1, Reading: index/4 + 1, Operation: operation, Phase: phase, ElapsedNanoseconds: int64(index+1) * int64(time.Millisecond)})
	}
	return trace
}

func publicRefreshLinuxCancelControlTrace() (support.PublicTUIHostProbeTrace, support.PublicTUIHostProbeTrace) {
	start := support.PublicTUIHostProbeEvent{Sequence: 1, Reading: 1, Operation: "meminfo", Phase: "start", ElapsedNanoseconds: 1}
	before := support.PublicTUIHostProbeTrace{Events: []support.PublicTUIHostProbeEvent{start}, Readings: 1, Active: 1, MaximumActive: 1}
	canceled := support.PublicTUIHostProbeEvent{Sequence: 2, Reading: 1, Operation: "meminfo", Phase: "canceled-reader-absent", ElapsedNanoseconds: 2, Descriptor: 42, PollEvents: 0x0008}
	after := support.PublicTUIHostProbeTrace{Events: []support.PublicTUIHostProbeEvent{start, canceled}, Readings: 1, MaximumActive: 1, Closed: true}
	return before, after
}

func TestPublicRefreshPlatformEvidenceDarwinCompletion(t *testing.T) {
	trace := publicBDDPostQueryControlTrace(28)
	for index := range trace.Events {
		trace.Events[index].RaceOptions = "atexit_sleep_ms=0"
	}
	family := publicRefreshControlBracket()
	lower, ok := publicRefreshCompletionLower("darwin", trace, 101, family)
	if !ok || !lower.Equal(family.probeBefore.Add(27*time.Nanosecond)) {
		t.Fatalf("authentic Darwin final ACK lower bound refused: lower=%s accepted=%t", lower, ok)
	}
}

func TestPublicRefreshPlatformEvidenceCompletionRefusals(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*support.PublicTUIHostProbeTrace)
	}{
		{"closed", func(trace *support.PublicTUIHostProbeTrace) { trace.Closed = true }},
		{"reading-count", func(trace *support.PublicTUIHostProbeTrace) { trace.Readings = 3 }},
		{"active", func(trace *support.PublicTUIHostProbeTrace) { trace.Active = 1 }},
		{"overlap", func(trace *support.PublicTUIHostProbeTrace) { trace.MaximumActive = 2 }},
		{"missing", func(trace *support.PublicTUIHostProbeTrace) { trace.Events = trace.Events[:7] }},
		{"extra", func(trace *support.PublicTUIHostProbeTrace) { trace.Events = append(trace.Events, trace.Events[7]) }},
		{"reordered", func(trace *support.PublicTUIHostProbeTrace) {
			trace.Events[2], trace.Events[4] = trace.Events[4], trace.Events[2]
		}},
		{"sequence", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[2].Sequence = 4 }},
		{"stale-reading", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[7].Reading = 1 }},
		{"write-error", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[7].Phase = "write-error" }},
		{"ACK", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[7].Phase = "output-ack" }},
		{"EOF", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[7].Phase = "canceled-eof" }},
		{"output-before-start", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[6].Phase = "output" }},
		{"wrong-final-query", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[7].Operation = "meminfo" }},
		{"invented-helper", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[7].PID = 12 }},
		{"invented-parent", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[7].ParentPID = 101 }},
		{"invented-argv", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[7].Arguments = []string{"-Q"} }},
		{"invented-reporting", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[7].RaceOptions = "atexit_sleep_ms=0" }},
		{"invented-descriptor", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[7].Descriptor = 42 }},
		{"invented-poll", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[7].PollEvents = 0x0008 }},
		{"negative-time", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[0].ElapsedNanoseconds = -1 }},
		{"reversed-time", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[7].ElapsedNanoseconds = 1 }},
		{"zero-final-time", func(trace *support.PublicTUIHostProbeTrace) {
			for index := range trace.Events {
				trace.Events[index].ElapsedNanoseconds = 0
			}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			trace := publicRefreshLinuxControlTrace()
			test.mutate(&trace)
			if _, ok := publicRefreshCompletionLower("linux", trace, 101, publicRefreshControlBracket()); ok {
				t.Fatal("malformed Linux completion accepted")
			}
		})
	}
}

func TestPublicRefreshPlatformEvidenceCompletionIdentityAndBracket(t *testing.T) {
	cases := []struct {
		name, platform string
		pid            int
		family         *publicBDDInteractive
	}{
		{name: "unsupported", platform: "freebsd", pid: 101, family: publicRefreshControlBracket()},
		{name: "Linux-evidence-on-Darwin", platform: "darwin", pid: 101, family: publicRefreshControlBracket()},
		{name: "missing-candidate", platform: "linux", family: publicRefreshControlBracket()},
		{name: "missing-bracket", platform: "linux", pid: 101},
		{name: "zero-origin", platform: "linux", pid: 101, family: &publicBDDInteractive{}},
		{name: "reversed-constructor", platform: "linux", pid: 101, family: &publicBDDInteractive{probeBefore: time.Unix(100, 0), probeAfter: time.Unix(99, 0), completedUpper: time.Unix(101, 0)}},
		{name: "upper-before-constructor", platform: "linux", pid: 101, family: &publicBDDInteractive{probeBefore: time.Unix(100, 0), probeAfter: time.Unix(100, 2), completedUpper: time.Unix(100, 1)}},
		{name: "lower-after-live", platform: "linux", pid: 101, family: &publicBDDInteractive{probeBefore: time.Unix(100, 0), probeAfter: time.Unix(100, 1), completedUpper: time.Unix(100, 2)}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, ok := publicRefreshCompletionLower(test.platform, publicRefreshLinuxControlTrace(), test.pid, test.family); ok {
				t.Fatal("invalid completion identity/bracket accepted")
			}
		})
	}
	t.Run("final-output-only-lower-bound", func(t *testing.T) {
		trace := publicRefreshLinuxControlTrace()
		for index := range trace.Events {
			trace.Events[index].ElapsedNanoseconds = 9
		}
		family := publicRefreshControlBracket()
		lower, ok := publicRefreshCompletionLower("linux", trace, 101, family)
		if !ok || !lower.Equal(family.probeBefore.Add(9*time.Nanosecond)) {
			t.Fatal("equal ordered timestamps must retain final output lower bound")
		}
	})
}

func TestPublicRefreshPlatformEvidenceDarwinCompletionRefusals(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(*support.PublicTUIHostProbeTrace)
		pid      int
		platform string
	}{
		{"stale-candidate", nil, 102, "darwin"},
		{"Darwin-evidence-on-Linux", nil, 101, "linux"},
		{"wrong-parent", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[27].ParentPID = 102 }, 101, "darwin"},
		{"wrong-helper", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[27].PID++ }, 101, "darwin"},
		{"reused-helper", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[26].PID = 201; trace.Events[27].PID = 201 }, 101, "darwin"},
		{"wrong-query", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[26].Arguments[1] = "vm.swapusage" }, 101, "darwin"},
		{"Linux-output", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[27].Phase = "output" }, 101, "darwin"},
		{"reporting-mismatch", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[27].RaceOptions = "foreign" }, 101, "darwin"},
		{"invented-FD", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[27].Descriptor = 42 }, 101, "darwin"},
		{"invented-poll", func(trace *support.PublicTUIHostProbeTrace) { trace.Events[27].PollEvents = 0x0008 }, 101, "darwin"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			trace := publicBDDPostQueryControlTrace(28)
			if test.mutate != nil {
				test.mutate(&trace)
			}
			if _, ok := publicRefreshCompletionLower(test.platform, trace, test.pid, publicRefreshControlBracket()); ok {
				t.Fatal("malformed or wrong-platform Darwin receipt accepted")
			}
		})
	}
}

func publicRefreshDarwinCancelControlTrace() (support.PublicTUIHostProbeTrace, support.PublicTUIHostProbeTrace) {
	before := publicBDDPostQueryControlTrace(1)
	before.Events[0].RaceOptions = "atexit_sleep_ms=0"
	canceled := before.Events[0]
	canceled.Sequence = 2
	canceled.Phase = "canceled-eof"
	canceled.ElapsedNanoseconds = 1
	after := support.PublicTUIHostProbeTrace{Events: []support.PublicTUIHostProbeEvent{before.Events[0], canceled}, Readings: 1, MaximumActive: 1, Closed: true}
	return before, after
}

func TestPublicRefreshPlatformEvidenceDarwinCancellation(t *testing.T) {
	before, after := publicRefreshDarwinCancelControlTrace()
	if !publicRefreshCancellation("darwin", before, after, 1, 101) {
		t.Fatal("authentic Darwin held ps EOF refused")
	}
}

func TestPublicRefreshPlatformEvidenceCancellationRefusals(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*support.PublicTUIHostProbeTrace, *support.PublicTUIHostProbeTrace)
	}{
		{"before-closed", func(before, _ *support.PublicTUIHostProbeTrace) { before.Closed = true }},
		{"before-not-held", func(before, _ *support.PublicTUIHostProbeTrace) { before.Active = 0 }},
		{"before-overlap", func(before, _ *support.PublicTUIHostProbeTrace) { before.MaximumActive = 2 }},
		{"before-wrong-reading", func(before, _ *support.PublicTUIHostProbeTrace) { before.Readings = 2 }},
		{"missing-start", func(before, _ *support.PublicTUIHostProbeTrace) { before.Events = nil }},
		{"extra-start", func(before, _ *support.PublicTUIHostProbeTrace) {
			before.Events = append(before.Events, before.Events[0])
		}},
		{"after-open", func(_, after *support.PublicTUIHostProbeTrace) { after.Closed = false }},
		{"after-active", func(_, after *support.PublicTUIHostProbeTrace) { after.Active = 1 }},
		{"after-overlap", func(_, after *support.PublicTUIHostProbeTrace) { after.MaximumActive = 2 }},
		{"after-new-reading", func(_, after *support.PublicTUIHostProbeTrace) { after.Readings = 2 }},
		{"missing-cancel", func(_, after *support.PublicTUIHostProbeTrace) { after.Events = after.Events[:1] }},
		{"extra-event", func(_, after *support.PublicTUIHostProbeTrace) { after.Events = append(after.Events, after.Events[1]) }},
		{"changed-prefix", func(_, after *support.PublicTUIHostProbeTrace) { after.Events[0].ElapsedNanoseconds++ }},
		{"wrong-sequence", func(_, after *support.PublicTUIHostProbeTrace) { after.Events[1].Sequence = 3 }},
		{"stale-reading", func(_, after *support.PublicTUIHostProbeTrace) { after.Events[1].Reading = 2 }},
		{"unrelated-operation", func(_, after *support.PublicTUIHostProbeTrace) { after.Events[1].Operation = "stat" }},
		{"negative-start-time", func(before, after *support.PublicTUIHostProbeTrace) {
			before.Events[0].ElapsedNanoseconds = -1
			after.Events[0].ElapsedNanoseconds = -1
		}},
		{"reversed-cancel-time", func(_, after *support.PublicTUIHostProbeTrace) { after.Events[1].ElapsedNanoseconds = -1 }},
		{"output-is-not-cancel", func(_, after *support.PublicTUIHostProbeTrace) { after.Events[1].Phase = "output" }},
		{"wrong-start-phase", func(before, after *support.PublicTUIHostProbeTrace) {
			before.Events[0].Phase = "output"
			after.Events[0].Phase = "output"
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			before, after := publicRefreshLinuxCancelControlTrace()
			test.mutate(&before, &after)
			if publicRefreshCancellation("linux", before, after, 1, 101) {
				t.Fatal("malformed held/canceled trace accepted")
			}
		})
	}
}

func TestPublicRefreshPlatformEvidenceLinuxCancellationRefusals(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(*support.PublicTUIHostProbeTrace, *support.PublicTUIHostProbeTrace)
		platform  string
		gate, pid int
	}{
		{name: "unsupported", platform: "freebsd", gate: 1, pid: 101},
		{name: "wrong-platform", platform: "darwin", gate: 1, pid: 101},
		{name: "wrong-gate", platform: "linux", gate: 2, pid: 101},
		{name: "missing-candidate", platform: "linux", gate: 1},
		{name: "negative-FD", platform: "linux", gate: 1, pid: 101, mutate: func(_, after *support.PublicTUIHostProbeTrace) { after.Events[1].Descriptor = -1 }},
		{name: "missing-ERR", platform: "linux", gate: 1, pid: 101, mutate: func(_, after *support.PublicTUIHostProbeTrace) { after.Events[1].PollEvents = 0 }},
		{name: "ERR-plus-NVAL", platform: "linux", gate: 1, pid: 101, mutate: func(_, after *support.PublicTUIHostProbeTrace) { after.Events[1].PollEvents = 0x0028 }},
		{name: "fake-parent", platform: "linux", gate: 1, pid: 101, mutate: func(_, after *support.PublicTUIHostProbeTrace) { after.Events[1].ParentPID = 101 }},
		{name: "fake-helper", platform: "linux", gate: 1, pid: 101, mutate: func(_, after *support.PublicTUIHostProbeTrace) { after.Events[1].PID = 201 }},
		{name: "fake-args", platform: "linux", gate: 1, pid: 101, mutate: func(_, after *support.PublicTUIHostProbeTrace) { after.Events[1].Arguments = []string{"-A"} }},
		{name: "fake-reporting", platform: "linux", gate: 1, pid: 101, mutate: func(_, after *support.PublicTUIHostProbeTrace) { after.Events[1].RaceOptions = "foreign" }},
		{name: "fake-start-FD", platform: "linux", gate: 1, pid: 101, mutate: func(before, after *support.PublicTUIHostProbeTrace) {
			before.Events[0].Descriptor = 42
			after.Events[0].Descriptor = 42
		}},
		{name: "fake-start-poll", platform: "linux", gate: 1, pid: 101, mutate: func(before, after *support.PublicTUIHostProbeTrace) {
			before.Events[0].PollEvents = 0x0008
			after.Events[0].PollEvents = 0x0008
		}},
		{name: "Darwin-EOF", platform: "linux", gate: 1, pid: 101, mutate: func(_, after *support.PublicTUIHostProbeTrace) { after.Events[1].Phase = "canceled-eof" }},
		{name: "ACK", platform: "linux", gate: 1, pid: 101, mutate: func(_, after *support.PublicTUIHostProbeTrace) { after.Events[1].Phase = "output-ack" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			before, after := publicRefreshLinuxCancelControlTrace()
			if test.mutate != nil {
				test.mutate(&before, &after)
			}
			if publicRefreshCancellation(test.platform, before, after, test.gate, test.pid) {
				t.Fatal("unproven Linux cancellation accepted")
			}
		})
	}
	t.Run("zero-valid-original-FD", func(t *testing.T) {
		before, after := publicRefreshLinuxCancelControlTrace()
		after.Events[1].Descriptor = 0
		if !publicRefreshCancellation("linux", before, after, 1, 101) {
			t.Fatal("nonnegative original descriptor zero is valid")
		}
	})
}

func TestPublicRefreshPlatformEvidenceDarwinCancellationRefusals(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(*support.PublicTUIHostProbeTrace, *support.PublicTUIHostProbeTrace)
		pid      int
		platform string
	}{
		{"stale-candidate", nil, 102, "darwin"},
		{"wrong-platform", nil, 101, "linux"},
		{"wrong-helper", func(_, after *support.PublicTUIHostProbeTrace) { after.Events[1].PID++ }, 101, "darwin"},
		{"wrong-parent", func(_, after *support.PublicTUIHostProbeTrace) { after.Events[1].ParentPID++ }, 101, "darwin"},
		{"wrong-argv", func(_, after *support.PublicTUIHostProbeTrace) { after.Events[1].Arguments = []string{"-B"} }, 101, "darwin"},
		{"wrong-query", func(before, after *support.PublicTUIHostProbeTrace) {
			before.Events[0].Operation = "sysctl"
			after.Events[0].Operation = "sysctl"
			after.Events[1].Operation = "sysctl"
		}, 101, "darwin"},
		{"missing-helper", func(before, after *support.PublicTUIHostProbeTrace) {
			before.Events[0].PID = 0
			after.Events[0].PID = 0
			after.Events[1].PID = 0
		}, 101, "darwin"},
		{"wrong-start-args", func(before, after *support.PublicTUIHostProbeTrace) {
			before.Events[0].Arguments = []string{"-B"}
			after.Events[0].Arguments = []string{"-B"}
			after.Events[1].Arguments = []string{"-B"}
		}, 101, "darwin"},
		{"reporting-mismatch", func(_, after *support.PublicTUIHostProbeTrace) { after.Events[1].RaceOptions = "foreign" }, 101, "darwin"},
		{"fake-FD", func(_, after *support.PublicTUIHostProbeTrace) { after.Events[1].Descriptor = 42 }, 101, "darwin"},
		{"fake-poll", func(_, after *support.PublicTUIHostProbeTrace) { after.Events[1].PollEvents = 0x0008 }, 101, "darwin"},
		{"Linux-cancel", func(_, after *support.PublicTUIHostProbeTrace) { after.Events[1].Phase = "canceled-reader-absent" }, 101, "darwin"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			before, after := publicRefreshDarwinCancelControlTrace()
			if test.mutate != nil {
				test.mutate(&before, &after)
			}
			if publicRefreshCancellation(test.platform, before, after, 1, test.pid) {
				t.Fatal("unproven Darwin cancellation accepted")
			}
		})
	}
}
