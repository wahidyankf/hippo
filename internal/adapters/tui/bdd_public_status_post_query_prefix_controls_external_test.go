package tui_test

import (
	"slices"
	"testing"

	"github.com/wahidyankf/hippo/tests/support"
)

// Constructed ledgers exercise the predicate only; they prove no native value,
// process exit, public observation, terminal restoration, or TUI acceptance.
func TestPublicBDDPostQueryObservedPrefixControls(t *testing.T) {
	for _, test := range []struct {
		name     string
		events   int
		finalACK bool
	}{
		{"one-query-ack", 2, false},
		{"one-query-ack-next-query-started", 3, false},
		{"three-query-acks-recorded-failure-shape", 6, false},
		{"three-query-acks-next-query-started", 7, false},
		{"seven-query-acks-first-reading-complete", 14, false},
		{"first-reading-complete-second-reading-started", 15, false},
		{"second-reading-one-query-ack", 16, false},
		{"second-reading-final-query-started", 27, false},
		{"all-fourteen-query-acks-full-final-ack", 28, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			trace := publicBDDPostQueryControlTrace(test.events)
			if !publicBDDPostQueryObservedPrefix(trace, 101) {
				t.Fatalf("authentic constructed prefix refused: %+v", trace)
			}
			if got := publicBDDPostQueryFinalACK(trace, 101); got != test.finalACK {
				t.Fatalf("full-final-ACK claim=%t, want %t for %d events", got, test.finalACK, test.events)
			}
		})
	}
	t.Run("equal-nonnegative-elapsed-times-are-monotonic", func(t *testing.T) {
		trace := publicBDDPostQueryControlTrace(6)
		for index := range trace.Events {
			trace.Events[index].ElapsedNanoseconds = 0
		}
		if !publicBDDPostQueryObservedPrefix(trace, 101) || publicBDDPostQueryFinalACK(trace, 101) {
			t.Fatal("equal elapsed times must preserve prefix validity and refuse the full-final-ACK claim")
		}
	})
}

func TestPublicBDDPostQueryObservedPrefixRefusals(t *testing.T) {
	for _, test := range []struct {
		name   string
		events int
		mutate func(*support.PublicTUIHostProbeTrace)
	}{
		{"empty", 0, nil},
		{"start-without-any-output-ack", 1, nil},
		{"excess-events-third-reading-started", 29, nil},
		{"closed", 6, func(trace *support.PublicTUIHostProbeTrace) { trace.Closed = true }},
		{"wrong-parent", 6, func(trace *support.PublicTUIHostProbeTrace) { trace.Events[1].ParentPID = 102 }},
		{"nonpositive-parent", 6, func(trace *support.PublicTUIHostProbeTrace) { trace.Events[0].ParentPID = 0 }},
		{"wrong-argv", 6, func(trace *support.PublicTUIHostProbeTrace) { trace.Events[0].Arguments[0] = "-B" }},
		{"unexpected-argv-for-argumentless-query", 6, func(trace *support.PublicTUIHostProbeTrace) {
			trace.Events[4].Arguments = []string{"-Q"}
		}},
		{"wrong-query-order", 6, func(trace *support.PublicTUIHostProbeTrace) {
			trace.Events[2].Operation, trace.Events[3].Operation = "vm_stat", "vm_stat"
			trace.Events[2].Arguments, trace.Events[3].Arguments = nil, nil
		}},
		{"wrong-first-phase", 6, func(trace *support.PublicTUIHostProbeTrace) {
			trace.Events[0].Phase = "output-ack"
		}},
		{"missing-output-ack", 6, func(trace *support.PublicTUIHostProbeTrace) { trace.Events[1].Phase = "start" }},
		{"protocol-error-is-not-ack", 6, func(trace *support.PublicTUIHostProbeTrace) {
			trace.Events[1].Phase = "protocol-error"
		}},
		{"wrong-sequence", 6, func(trace *support.PublicTUIHostProbeTrace) { trace.Events[2].Sequence = 4 }},
		{"wrong-event-reading", 6, func(trace *support.PublicTUIHostProbeTrace) { trace.Events[2].Reading = 2 }},
		{"nonpositive-helper-pid", 6, func(trace *support.PublicTUIHostProbeTrace) { trace.Events[0].PID = 0 }},
		{"start-ack-pid-mismatch", 6, func(trace *support.PublicTUIHostProbeTrace) { trace.Events[1].PID++ }},
		{"duplicate-helper-pid-with-matching-ack", 6, func(trace *support.PublicTUIHostProbeTrace) {
			trace.Events[2].PID, trace.Events[3].PID = trace.Events[0].PID, trace.Events[0].PID
		}},
		{"negative-first-elapsed-time", 6, func(trace *support.PublicTUIHostProbeTrace) {
			trace.Events[0].ElapsedNanoseconds = -1
		}},
		{"reversed-elapsed-times", 6, func(trace *support.PublicTUIHostProbeTrace) {
			trace.Events[3].ElapsedNanoseconds = trace.Events[2].ElapsedNanoseconds - 1
		}},
		{"wrong-reading-count", 6, func(trace *support.PublicTUIHostProbeTrace) { trace.Readings = 2 }},
		{"intermediate-ack-does-not-clear-active-reading", 6, func(trace *support.PublicTUIHostProbeTrace) {
			trace.Active = 0
		}},
		{"completed-reading-must-clear-active", 14, func(trace *support.PublicTUIHostProbeTrace) {
			trace.Active = 1
		}},
		{"second-reading-start-must-increment-readings", 15, func(trace *support.PublicTUIHostProbeTrace) {
			trace.Readings = 1
		}},
		{"second-reading-start-must-be-active", 15, func(trace *support.PublicTUIHostProbeTrace) {
			trace.Active = 0
		}},
		{"overlap-refused", 6, func(trace *support.PublicTUIHostProbeTrace) { trace.MaximumActive = 2 }},
		{"missing-maximum-active", 6, func(trace *support.PublicTUIHostProbeTrace) { trace.MaximumActive = 0 }},
		{"full-ledger-final-ack-pid-mismatch", 28, func(trace *support.PublicTUIHostProbeTrace) {
			trace.Events[27].PID++
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			trace := publicBDDPostQueryControlTrace(test.events)
			if test.mutate != nil {
				test.mutate(&trace)
			}
			if publicBDDPostQueryObservedPrefix(trace, 101) {
				t.Fatalf("malformed or unacknowledged ledger accepted: %+v", trace)
			}
			if publicBDDPostQueryFinalACK(trace, 101) {
				t.Fatal("malformed or partial ledger asserted a full final ACK")
			}
		})
	}
	t.Run("nonpositive-expected-parent", func(t *testing.T) {
		trace := publicBDDPostQueryControlTrace(6)
		if publicBDDPostQueryObservedPrefix(trace, 0) || publicBDDPostQueryFinalACK(trace, 0) {
			t.Fatal("a nonpositive expected parent must never qualify")
		}
	})
}

// The explicit command oracle is independent of hostprobe.Commands so changing
// the diagnostic's whitelist source cannot silently rewrite these controls.
func publicBDDPostQueryControlTrace(eventCount int) support.PublicTUIHostProbeTrace {
	queries := [][]string{
		{"ps", "-A", "-o", "%cpu="},
		{"memory_pressure", "-Q"},
		{"vm_stat"},
		{"sysctl", "-n", "vm.swapusage"},
		{"sysctl", "-n", "kern.memorystatus_vm_pressure_level"},
		{"sysctl", "-n", "vm.compressor_available"},
		{"sysctl", "-n", "vm.compressor_bytes_used"},
	}
	trace := support.PublicTUIHostProbeTrace{}
	for index := range eventCount {
		query := queries[(index/2)%7]
		phase := "start"
		if index%2 == 1 {
			phase = "output-ack"
		}
		trace.Events = append(trace.Events, support.PublicTUIHostProbeEvent{
			Sequence: index + 1, Reading: index/14 + 1,
			Operation: query[0], Phase: phase, Arguments: slices.Clone(query[1:]),
			PID: 201 + index/2, ParentPID: 101, ElapsedNanoseconds: int64(index),
		})
		trace.Readings = index/14 + 1
		trace.Active = 1
		trace.MaximumActive = 1
		if (index+1)%14 == 0 {
			trace.Active = 0
		}
	}
	return trace
}
