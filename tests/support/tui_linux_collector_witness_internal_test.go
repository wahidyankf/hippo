package support

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestLinuxCollectorWitnessRuntimePreludePreservesCollectorTrace(t *testing.T) {
	readings := append(witnessReading(), witnessReading()...)
	baseline, err := witnessRun(readings)
	if err != nil {
		t.Fatal(err)
	}
	// Model the observed operations required by pinned Go's startup source.
	// This portable protocol control is not a claimed native syscall recording.
	prefix := witnessCall(witnessLeader, linuxWitnessOpen, "/proc/self/cgroup", 0, 30)
	prefix = append(prefix, witnessCall(witnessLeader, linuxWitnessRead, "", 30, 32)...)
	prefix = append(prefix, witnessCall(witnessLeader, linuxWitnessRead, "", 30, 0)...)
	prefix = append(prefix, witnessCall(witnessLeader, linuxWitnessClose, "", 30, 0)...)
	prefix = append(prefix, witnessCall(witnessLeader, linuxWitnessOpen, "/sys/fs/cgroup/cpu.max", 0, 31)...)
	trace, err := witnessRun(append(prefix, readings...))
	if err != nil || !reflect.DeepEqual(trace, baseline) {
		t.Fatalf("runtime startup changed genuine collector obligations: trace=%+v baseline=%+v err=%v", trace, baseline, err)
	}
}

func witnessPreludeDiscovery() []linuxWitnessEvent {
	events := witnessCall(witnessLeader, linuxWitnessOpen, "/proc/self/cgroup", 0, 30)
	events = append(events, witnessCall(witnessLeader, linuxWitnessRead, "", 30, 32)...)
	// The v1 parser can return before EOF; only the actual close ends discovery.
	return append(events, witnessCall(witnessLeader, linuxWitnessClose, "", 30, 0)...)
}

func witnessPreludeCPU() []linuxWitnessEvent {
	return append(witnessPreludeDiscovery(), witnessCall(witnessLeader, linuxWitnessOpen, "/sys/fs/cgroup/cpu.max", 0, 31)...)
}

func witnessPreludeFeed(t *testing.T, events []linuxWitnessEvent) *linuxCollectorReducer {
	t.Helper()
	reducer := newLinuxCollectorReducer(witnessLeader, "/owned/disk")
	if err := reducer.addThread(witnessLeader, 22); err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if err := reducer.batch([]linuxWitnessEvent{event}); err != nil {
			t.Fatalf("valid prelude fixture failed: %+v: %v", event, err)
		}
	}
	return reducer
}

func TestLinuxCollectorWitnessRuntimePreludeFiniteStartupShapes(t *testing.T) {
	failedRead := witnessCall(witnessLeader, linuxWitnessOpen, "/proc/self/cgroup", 0, 30)
	failedRead = append(failedRead, witnessCall(witnessLeader, linuxWitnessRead, "", 30, -5)...)
	failedRead = append(failedRead, witnessCall(witnessLeader, linuxWitnessClose, "", 30, 0)...)
	quota := append(witnessPreludeDiscovery(), witnessCall(witnessLeader, linuxWitnessOpen, "/sys/fs/cgroup/cpu/cpu.cfs_quota_us", 0, 31)...)
	readings := append(witnessReading(), witnessReading()...)
	baseline, err := witnessRun(readings)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		prefix []linuxWitnessEvent
	}{
		{"no-prelude", nil},
		{"v1-early-parser-close", append(slices.Clone(quota), witnessCall(witnessLeader, linuxWitnessOpen, "/sys/fs/cgroup/cpu/cpu.cfs_period_us", 0, 32)...)},
		{"failed-proc-open", witnessCall(witnessLeader, linuxWitnessOpen, "/proc/self/cgroup", 0, -2)},
		{"failed-proc-read", failedRead},
		{"failed-v2-open", append(witnessPreludeDiscovery(), witnessCall(witnessLeader, linuxWitnessOpen, "/sys/fs/cgroup/cpu.max", 0, -2)...)},
		{"failed-v1-quota-open", append(witnessPreludeDiscovery(), witnessCall(witnessLeader, linuxWitnessOpen, "/sys/fs/cgroup/cpu/cpu.cfs_quota_us", 0, -2)...)},
		{"v1-partial-open-period-failure", append(slices.Clone(quota), witnessCall(witnessLeader, linuxWitnessOpen, "/sys/fs/cgroup/cpu/cpu.cfs_period_us", 0, -2)...)},
	} {
		t.Run(test.name, func(t *testing.T) {
			trace, err := witnessRun(append(slices.Clone(test.prefix), readings...))
			if err != nil || !reflect.DeepEqual(trace, baseline) {
				t.Fatalf("finite runtime startup changed collector trace: %v", err)
			}
		})
	}
}

func TestLinuxCollectorWitnessRuntimePreludeRefusesMalformedMetadata(t *testing.T) {
	quota := append(witnessPreludeDiscovery(), witnessCall(witnessLeader, linuxWitnessOpen, "/sys/fs/cgroup/cpu/cpu.cfs_quota_us", 0, 31)...)
	period := append(slices.Clone(quota), witnessCall(witnessLeader, linuxWitnessOpen, "/sys/fs/cgroup/cpu/cpu.cfs_period_us", 0, 32)...)
	failedRead := witnessPreludeDiscovery()
	failedRead[3].Result = -5
	for _, test := range []struct {
		name        string
		prefix, bad []linuxWitnessEvent
	}{
		{"duplicate-proc", witnessPreludeDiscovery(), witnessCall(witnessLeader, linuxWitnessOpen, "/proc/self/cgroup", 0, 30)[:1]},
		{"cpu-without-discovery", nil, witnessCall(witnessLeader, linuxWitnessOpen, "/sys/fs/cgroup/cpu.max", 0, 31)[:1]},
		{"failed-discovery-cannot-open-cpu", witnessCall(witnessLeader, linuxWitnessOpen, "/proc/self/cgroup", 0, -2), witnessCall(witnessLeader, linuxWitnessOpen, "/sys/fs/cgroup/cpu.max", 0, 31)[:1]},
		{"failed-read-cannot-open-cpu", failedRead, witnessCall(witnessLeader, linuxWitnessOpen, "/sys/fs/cgroup/cpu.max", 0, 31)[:1]},
		{"duplicate-v2", witnessPreludeCPU(), witnessCall(witnessLeader, linuxWitnessOpen, "/sys/fs/cgroup/cpu.max", 0, 32)[:1]},
		{"mixed-family", witnessPreludeCPU(), witnessCall(witnessLeader, linuxWitnessOpen, "/sys/fs/cgroup/cpu.cfs_quota_us", 0, 32)[:1]},
		{"v1-period-first", witnessPreludeDiscovery(), witnessCall(witnessLeader, linuxWitnessOpen, "/sys/fs/cgroup/cpu.cfs_period_us", 0, 31)[:1]},
		{"changed-directory", quota, witnessCall(witnessLeader, linuxWitnessOpen, "/sys/fs/cgroup/foreign/cpu.cfs_period_us", 0, 32)[:1]},
		{"excess-v1-attempt", period, witnessCall(witnessLeader, linuxWitnessOpen, "/sys/fs/cgroup/cpu/cpu.cfs_period_us", 0, 33)[:1]},
		{"traversal", witnessPreludeDiscovery(), witnessCall(witnessLeader, linuxWitnessOpen, "/sys/fs/cgroup/../cpu.max", 0, 31)[:1]},
		{"foreign-thread", nil, witnessCall(99, linuxWitnessOpen, "/proc/self/cgroup", 0, 30)[:1]},
		{"unpaired-exit", nil, witnessCall(witnessLeader, linuxWitnessOpen, "/proc/self/cgroup", 0, 30)[1:]},
		{"duplicate-entry", witnessCall(witnessLeader, linuxWitnessOpen, "/proc/self/cgroup", 0, 30)[:1], witnessCall(witnessLeader, linuxWitnessOpen, "/proc/self/cgroup", 0, 30)[:1]},
		{"mismatched-exit", witnessCall(witnessLeader, linuxWitnessOpen, "/proc/self/cgroup", 0, 30)[:1], witnessCall(witnessLeader, linuxWitnessRead, "", 30, 32)[1:]},
		{"oversized-fd", nil, witnessCall(witnessLeader, linuxWitnessOpen, "/proc/self/cgroup", 0, 1<<20+1)},
		{"occupied-prelude-fd", quota, witnessCall(witnessLeader, linuxWitnessOpen, "/sys/fs/cgroup/cpu/cpu.cfs_period_us", 0, 31)},
		{"unrelated-open-reuses-prelude-fd", witnessPreludeCPU(), witnessCall(witnessLeader, linuxWitnessOpen, "", 0, 31)},
		{"failed-background-close", witnessPreludeCPU(), witnessCall(witnessLeader, linuxWitnessClose, "", 31, -5)},
		{"failed-proc-close", witnessCall(witnessLeader, linuxWitnessOpen, "/proc/self/cgroup", 0, 30), witnessCall(witnessLeader, linuxWitnessClose, "", 30, -5)},
	} {
		t.Run(test.name, func(t *testing.T) {
			reducer := witnessPreludeFeed(t, test.prefix)
			for index, event := range test.bad {
				err := reducer.batch([]linuxWitnessEvent{event})
				if (err != nil) != (index == len(test.bad)-1) {
					t.Fatalf("malformed prelude refusal at wrong operation: %d %v", index, err)
				}
			}
			if reducer.trace.Readings != 0 || len(reducer.trace.Events) != 0 || reducer.trace.Closed {
				t.Fatal("malformed startup fabricated collector evidence")
			}
		})
	}
}

func TestLinuxCollectorWitnessRuntimePreludeDoesNotExemptCollectorPaths(t *testing.T) {
	for _, path := range []string{"/proc/stat", "/proc/vmstat", "/proc/pressure/memory", "/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/foreign", "/sys/fs/cgroup//cpu.max"} {
		t.Run(path, func(t *testing.T) {
			reducer := witnessPreludeFeed(t, witnessPreludeDiscovery())
			if err := reducer.batch(witnessCall(witnessLeader, linuxWitnessOpen, path, 0, 31)[:1]); err == nil {
				t.Fatal("non-runtime startup path became prelude evidence")
			}
		})
	}
	reducer := witnessPreludeFeed(t, witnessPreludeDiscovery())
	if err := reducer.batch(witnessCall(witnessLeader, linuxWitnessStatFS, "/owned/disk", 0, 0)[:1]); err == nil {
		t.Fatal("runtime startup exempted disk evidence outside a reading")
	}
	for _, count := range []int{1, 2} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			prefix := witnessPreludeCPU()
			for range count {
				prefix = append(prefix, witnessReading()...)
			}
			reducer := witnessPreludeFeed(t, prefix)
			if err := reducer.batch(witnessCall(witnessLeader, linuxWitnessOpen, "/sys/fs/cgroup/cpu.max", 0, 32)[:1]); err == nil || reducer.trace.Readings != count {
				t.Fatal("new runtime-looking open escaped order after first begin", err)
			}
		})
	}
}

func TestLinuxCollectorWitnessRuntimePreludeFailedCloseRetainsGeneration(t *testing.T) {
	for _, test := range []struct {
		name   string
		prefix []linuxWitnessEvent
		fd     int
		path   string
	}{
		{"proc", witnessCall(witnessLeader, linuxWitnessOpen, "/proc/self/cgroup", 0, 30), 30, "/proc/self/cgroup"},
		{"cpu", witnessPreludeCPU(), 31, "/sys/fs/cgroup/cpu.max"},
	} {
		t.Run(test.name, func(t *testing.T) {
			reducer := witnessPreludeFeed(t, test.prefix)
			closeCall := witnessCall(witnessLeader, linuxWitnessClose, "", test.fd, -5)
			if err := reducer.batch(closeCall[:1]); err != nil {
				t.Fatal(err)
			}
			if err := reducer.batch(closeCall[1:]); err == nil || reducer.prelude.files[test.fd] != test.path || reducer.trace.Readings != 0 {
				t.Fatal("failed close discarded an occupied runtime generation", err)
			}
		})
	}
}

func TestLinuxCollectorWitnessRuntimePreludeCannotMaskMissingOrReorderedReadings(t *testing.T) {
	reducer := witnessPreludeFeed(t, witnessPreludeCPU())
	if trace, err := reducer.finish(); err == nil || trace.Readings != 0 || len(trace.Events) != 0 || trace.Closed {
		t.Fatal("runtime-only startup became a complete collector reading", trace, err)
	}
	reducer = witnessPreludeFeed(t, append(witnessPreludeCPU(), witnessReading()[:8]...))
	if err := reducer.batch(witnessCall(witnessLeader, linuxWitnessOpen, "/proc/vmstat", 0, 9)[:1]); !errors.Is(err, errLinuxWitnessEvidenceOrder) {
		t.Fatal("runtime prefix masked reordered mandatory collector evidence", err)
	}
}

func TestLinuxCollectorWitnessRuntimePreludeKeepsDescriptorGenerationsSeparate(t *testing.T) {
	readings := append(witnessReading(), witnessReading()...)
	baseline, err := witnessRun(readings)
	if err != nil {
		t.Fatal(err)
	}
	prefix := witnessPreludeCPU()
	background := witnessCall(22, linuxWitnessRead, "", 31, 8)
	background = append(background, witnessCall(witnessLeader, linuxWitnessRead, "", 31, 0)...)
	background = append(background, witnessCall(22, linuxWitnessClose, "", 31, 0)...)
	events := append(slices.Clone(prefix), readings[:2]...)
	events = append(events, background...)
	// Only an actual successful background close frees this descriptor for the
	// next collector generation. All collector operations retain their protocol.
	for _, event := range readings[2:] {
		if event.FD == 9 {
			event.FD = 31
		}
		if event.Operation == linuxWitnessOpen && !event.Entry && event.Result == 9 {
			event.Result = 31
		}
		events = append(events, event)
	}
	// The first meminfo open above used FD9, so its read/close keeps that generation.
	for index := 2 + len(prefix) + len(background); index < 8+len(prefix)+len(background); index++ {
		if events[index].FD == 31 {
			events[index].FD = 9
		}
	}
	trace, err := witnessRun(events)
	if err != nil || !reflect.DeepEqual(trace, baseline) {
		t.Fatal("background descriptor activity borrowed collector evidence", err)
	}
	reducer := witnessPreludeFeed(t, append(witnessPreludeCPU(), witnessReading()[:1]...))
	if err := reducer.batch([]linuxWitnessEvent{{TID: witnessLeader, Operation: linuxWitnessOpen, Result: 31}}); err == nil || len(reducer.files) != 0 || reducer.prelude.files[31] == "" {
		t.Fatal("collector adopted an occupied background generation", err)
	}
}

func TestLinuxCollectorWitnessRuntimePreludePreservesCollectorRetryDebt(t *testing.T) {
	reducer := witnessPreludeFeed(t, append(witnessPreludeCPU(), witnessReading()[:2]...))
	read := witnessCall(witnessLeader, linuxWitnessRead, "", 9, -4)
	read[0].ReadIdentity = linuxWitnessReadIdentity{Number: 0, Buffer: 4096, Count: 32, Observed: true}
	for _, event := range read {
		if err := reducer.batch([]linuxWitnessEvent{event}); err != nil {
			t.Fatal(err)
		}
	}
	debt := reducer.retry
	if debt == nil {
		t.Fatal("mandatory read did not establish exact retry debt")
	}
	for _, event := range witnessCall(22, linuxWitnessRead, "", 31, 8) {
		if err := reducer.batch([]linuxWitnessEvent{event}); err != nil {
			t.Fatal(err)
		}
	}
	file := reducer.files[9]
	if file == nil {
		t.Fatal("background read discarded the live mandatory collector generation")
	}
	if reducer.retry != debt || debt.activeTID != 0 || reducer.step != 0 || file.bytes != 0 || file.eof {
		t.Fatal("background read satisfied or changed collector debt")
	}
	if _, err := reducer.finish(); !errors.Is(err, errLinuxWitnessInterruptedEvidence) {
		t.Fatal("background read converted unresolved collector debt into completion", err)
	}
}

func TestLinuxCollectorWitnessRuntimePreludeInterruptionsRetainOccupancy(t *testing.T) {
	for _, operation := range []string{linuxWitnessOpen, linuxWitnessRead, linuxWitnessClose} {
		for _, result := range []int64{-4, -512, -513, -514, -516} {
			t.Run(fmt.Sprintf("%s/%d", operation, result), func(t *testing.T) {
				prefix := witnessPreludeCPU()
				path := ""
				if operation == linuxWitnessOpen {
					prefix, path = witnessPreludeDiscovery(), "/sys/fs/cgroup/cpu.max"
				}
				reducer := witnessPreludeFeed(t, prefix)
				before := len(reducer.prelude.files)
				call := witnessCall(witnessLeader, operation, path, 31, result)
				if err := reducer.batch(call[:1]); err != nil {
					t.Fatal(err)
				}
				if err := reducer.batch(call[1:]); !errors.Is(err, errLinuxWitnessInterruptedEvidence) || reducer.retry != nil || len(reducer.prelude.files) != before || reducer.trace.Readings != 0 {
					t.Fatal("interrupted startup evidence advanced or borrowed collector debt", err)
				}
			})
		}
	}
}

func TestLinuxCollectorWitnessRuntimePreludeRefusesAmbiguousBegin(t *testing.T) {
	procOpen := witnessCall(witnessLeader, linuxWitnessOpen, "/proc/self/cgroup", 0, 30)
	for _, test := range []struct {
		name   string
		prefix []linuxWitnessEvent
	}{
		{"pending-open", procOpen[:1]},
		{"live-proc-fd", procOpen},
		{"pending-proc-read", append(slices.Clone(procOpen), witnessCall(witnessLeader, linuxWitnessRead, "", 30, 32)[:1]...)},
		{"pending-proc-close", append(slices.Clone(procOpen), witnessCall(witnessLeader, linuxWitnessClose, "", 30, 0)[:1]...)},
		{"pending-cpu-open", append(witnessPreludeDiscovery(), witnessCall(witnessLeader, linuxWitnessOpen, "/sys/fs/cgroup/cpu.max", 0, 31)[:1]...)},
		{"pending-cpu-read", append(witnessPreludeCPU(), witnessCall(witnessLeader, linuxWitnessRead, "", 31, 32)[:1]...)},
	} {
		t.Run(test.name, func(t *testing.T) {
			reducer := witnessPreludeFeed(t, test.prefix)
			begin := witnessCall(22, linuxWitnessOpen, linuxWitnessMeminfo, 0, 9)[0]
			if err := reducer.batch([]linuxWitnessEvent{begin}); err == nil || reducer.trace.Readings != 1 || reducer.trace.Closed {
				t.Fatal("actual meminfo begin hid unresolved startup metadata", err)
			}
		})
	}
	reducer := witnessPreludeFeed(t, nil)
	batch := []linuxWitnessEvent{procOpen[0], witnessCall(22, linuxWitnessOpen, linuxWitnessMeminfo, 0, 9)[0]}
	if err := reducer.batch(batch); err == nil || reducer.trace.Readings != 1 {
		t.Fatal("same-batch begin retroactively admitted startup evidence", err)
	}
}

func TestLinuxCollectorWitnessTrapEventRequiresPlainTrapSignal(t *testing.T) {
	// Go's Linux WaitStatus.TrapCause returns -1 for non-SIGTRAP stops.
	// SIGTRAP|0x80 identifies syscall stops under PTRACE_O_TRACESYSGOOD.
	const trapSignal = 5
	for _, test := range []struct {
		name                    string
		signal, cause, expected int
	}{
		{"syscall-stop", trapSignal | 0x80, -1, 0},
		{"cloned-thread-initial-stop", 19, -1, 0},
		{"asynchronous-signal", 23, -1, 0},
		{"initial-exec-trap", trapSignal, 0, 0},
		{"fork-event", trapSignal, 1, 1},
		{"vfork-event", trapSignal, 2, 2},
		{"clone-event", trapSignal, 3, 3},
		{"exec-event", trapSignal, 4, 4},
		{"exit-event", trapSignal, 6, 6},
		{"unknown-event", trapSignal, 99, 99},
		{"invalid-trap-cause", trapSignal, -1, -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			if event := linuxWitnessTrapEvent(test.signal, trapSignal, test.cause); event != test.expected {
				t.Fatalf("signal=%d cause=%d: event=%d, want %d", test.signal, test.cause, event, test.expected)
			}
		})
	}
}

func TestLinuxCollectorWitnessFrontierRescansMutationAndUncertainty(t *testing.T) {
	for _, mode := range []string{"clone", "exit", "uncertain", "second-pass"} {
		t.Run(mode, func(t *testing.T) {
			source := witnessFrontierScenario(mode)
			events, err := linuxWitnessDrainFrontier(t.Context(), source, false)
			if err != nil {
				t.Fatal(err)
			}
			if len(events) == 0 || events[len(events)-1].TID != witnessLeader {
				t.Fatal("frontier lost newly ready known-TID evidence", mode, source.calls, events)
			}
			if mode == "second-pass" && source.counts[witnessLeader] != 3 {
				t.Fatal("second-pass readiness was declared stable prematurely", source.calls)
			}
		})
	}
}

func witnessFrontierScenario(mode string) *witnessFrontierSource {
	source := &witnessFrontierSource{known: []int{witnessLeader, 22}, held: map[int]bool{}, counts: map[int]int{}}
	ready := false
	if mode == "clone" {
		source.known = []int{22}
	}
	if mode == "second-pass" {
		source.known = append(source.known, 23)
	}
	source.harvest = func(tid, count int) (linuxWitnessHarvest, error) {
		if tid == 22 {
			ready = mode != "second-pass"
			if mode == "clone" {
				source.known = append(source.known, witnessLeader)
			}
			if mode == "exit" {
				source.known = []int{witnessLeader}
			}
			return linuxWitnessHarvest{acquired: true}, nil
		}
		if mode == "uncertain" && tid == witnessLeader && count == 1 {
			return linuxWitnessHarvest{uncertain: true}, nil
		}
		if mode == "second-pass" && tid == 23 && count == 2 {
			ready = true
			return linuxWitnessHarvest{acquired: true}, nil
		}
		if tid == witnessLeader && ready {
			return linuxWitnessHarvest{acquired: true, result: linuxStopResult{observed: true, event: linuxWitnessEvent{TID: tid, Entry: true, Operation: linuxWitnessOpen, Path: linuxWitnessMeminfo}}}, nil
		}
		return linuxWitnessHarvest{}, nil
	}
	return source
}

func TestLinuxCollectorWitnessFrontierHonorsOriginalContextAndOwnedQueries(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	source := &witnessFrontierSource{known: []int{witnessLeader}, held: map[int]bool{}, counts: map[int]int{}}
	source.harvest = func(tid, _ int) (linuxWitnessHarvest, error) {
		if tid != witnessLeader {
			t.Fatal("frontier queried a foreign thread")
		}
		cancel()
		return linuxWitnessHarvest{uncertain: true}, nil
	}
	if events, err := linuxWitnessDrainFrontier(ctx, source, false); !errors.Is(err, context.Canceled) || len(events) != 0 || len(source.calls) != 1 {
		t.Fatal("uncertain pass escaped original cancellation", events, source.calls, err)
	}
	expired, retire := context.WithDeadline(t.Context(), time.Time{})
	defer retire()
	source.calls = nil
	if _, err := linuxWitnessDrainFrontier(expired, source, false); !errors.Is(err, context.DeadlineExceeded) || len(source.calls) != 0 {
		t.Fatal("frontier queried children after original deadline", source.calls, err)
	}
}

type witnessFrontierSource struct {
	known   []int
	held    map[int]bool
	counts  map[int]int
	calls   []int
	initial []bool
	harvest func(int, int) (linuxWitnessHarvest, error)
}

func (source *witnessFrontierSource) linuxWitnessTIDs() []int       { return slices.Clone(source.known) }
func (source *witnessFrontierSource) linuxWitnessHeld(tid int) bool { return source.held[tid] }
func (source *witnessFrontierSource) linuxWitnessHarvest(tid int, initial bool) (linuxWitnessHarvest, error) {
	if !slices.Contains(source.known, tid) || source.held[tid] {
		return linuxWitnessHarvest{}, errors.New("frontier queried foreign or already held thread")
	}
	source.counts[tid]++
	source.calls = append(source.calls, tid)
	source.initial = append(source.initial, initial)
	result, err := source.harvest(tid, source.counts[tid])
	if result.acquired && slices.Contains(source.known, tid) {
		source.held[tid] = true
	}
	return result, err
}

func TestLinuxCollectorWitnessFrontierPreservesInitialExecDiscriminator(t *testing.T) {
	for _, initial := range []bool{true, false} {
		source := &witnessFrontierSource{known: []int{witnessLeader}, held: map[int]bool{}, counts: map[int]int{}}
		source.harvest = func(_, _ int) (linuxWitnessHarvest, error) {
			return linuxWitnessHarvest{acquired: true}, nil
		}
		if _, err := linuxWitnessDrainFrontier(t.Context(), source, initial); err != nil || len(source.initial) != 1 || source.initial[0] != initial {
			t.Fatal("frontier changed native initial-exec/ordinary-stop discriminator", initial, source.initial, err)
		}
	}
}

func TestLinuxCollectorWitnessFrontierCapturesEarlierScannedPendingBegin(t *testing.T) {
	reading := witnessReading()
	reading[len(reading)-2].TID, reading[len(reading)-1].TID = 22, 22
	reducer := newLinuxCollectorReducer(witnessLeader, "/owned/disk")
	if err := reducer.addThread(witnessLeader, 22); err != nil {
		t.Fatal(err)
	}
	for _, event := range reading[:len(reading)-1] {
		if err := reducer.batch([]linuxWitnessEvent{event}); err != nil {
			t.Fatal(err)
		}
	}
	ready := false
	source := &witnessFrontierSource{known: []int{witnessLeader, 22}, held: map[int]bool{}, counts: map[int]int{}}
	source.harvest = func(tid, _ int) (linuxWitnessHarvest, error) {
		if tid == witnessLeader && ready {
			return linuxWitnessHarvest{acquired: true, result: linuxStopResult{observed: true, event: linuxWitnessEvent{TID: tid, Entry: true, Operation: linuxWitnessOpen, Path: linuxWitnessMeminfo}}}, nil
		}
		if tid == 22 {
			// A becomes durably stopped after its empty scan, before B's
			// completion is harvested. No child is resumed by this source.
			ready = true
			return linuxWitnessHarvest{acquired: true, result: linuxStopResult{observed: true, event: reading[len(reading)-1]}}, nil
		}
		return linuxWitnessHarvest{}, nil
	}
	events, err := linuxWitnessDrainFrontier(t.Context(), source, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := reducer.batch(events); err == nil || reducer.trace.MaximumActive != 2 {
		t.Fatalf("adapter frontier certified hidden pending overlap: scans=%v trace=%+v err=%v", source.calls, reducer.trace, err)
	}
	if !source.held[witnessLeader] || !source.held[22] {
		t.Fatal("frontier resumed or discarded a harvested stop before batch")
	}
}

func TestLinuxCollectorWitnessReadEINTRRequiresExactLiveInvocation(t *testing.T) {
	identity := linuxWitnessReadIdentity{Number: 63, Buffer: 4096, Count: 32, Observed: true}
	for _, repeated := range []bool{false, true} {
		events := append(witnessReading(), witnessReading()...)
		events[2].ReadIdentity = identity
		events[3].Result = -4
		retry := witnessCall(witnessLeader, linuxWitnessRead, "", 9, 32)
		retry[0].ReadIdentity = identity
		if repeated {
			interrupted := slices.Clone(retry)
			interrupted[1].Result = -4
			events = slices.Insert(events, 4, interrupted...)
			events = slices.Insert(events, 6, retry...)
		} else {
			events = slices.Insert(events, 4, retry...)
		}
		trace, err := witnessRun(events)
		if err != nil || trace.Readings != 2 || trace.Active != 0 || trace.MaximumActive != 1 || !trace.Closed {
			t.Fatalf("observed same-buffer read retry lost serialized reading: %+v %v", trace, err)
		}
	}
	for _, changed := range []linuxWitnessReadIdentity{
		{},
		{Number: 0, Buffer: 4096, Count: 32, Observed: true},
		{Number: 63, Buffer: 8192, Count: 32, Observed: true},
		{Number: 63, Buffer: 4096, Count: 64, Observed: true},
	} {
		events := append(witnessReading(), witnessReading()...)
		events[2].ReadIdentity, events[3].Result = identity, -4
		retry := witnessCall(witnessLeader, linuxWitnessRead, "", 9, 32)
		retry[0].ReadIdentity = changed
		events = slices.Insert(events, 4, retry...)
		if _, err := witnessRun(events); !errors.Is(err, errLinuxWitnessInterruptedEvidence) {
			t.Fatalf("changed or unknown invocation was treated as a proved retry: %v", err)
		}
	}
}

func witnessAcceptEvents(t *testing.T, reducer *linuxCollectorReducer, events []linuxWitnessEvent) {
	t.Helper()
	for _, event := range events {
		if err := reducer.batch([]linuxWitnessEvent{event}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLinuxCollectorWitnessOptionalReadEINTRPreservesExactLiveContinuation(t *testing.T) {
	reducer := newLinuxCollectorReducer(witnessLeader, "/owned/disk")
	if err := reducer.addThread(witnessLeader, 22); err != nil {
		t.Fatal(err)
	}
	path := "/proc/self/cgroup"
	prefix := append(witnessFile(linuxWitnessMeminfo, true), witnessFile("/proc/stat", true)...)
	prefix = append(prefix, witnessCall(witnessLeader, linuxWitnessOpen, path, 0, 9)...)
	prefix = append(prefix, witnessCall(witnessLeader, linuxWitnessRead, "", 9, 8)...)
	witnessAcceptEvents(t, reducer, prefix)
	file, step := reducer.files[9], reducer.step
	if file == nil {
		t.Fatal("optional continuation control requires its live FD")
	}
	identity := linuxWitnessReadIdentity{Number: 63, Buffer: 4096, Count: 32, Observed: true}
	interrupted := witnessCall(witnessLeader, linuxWitnessRead, "", 9, -4)
	interrupted[0].ReadIdentity = identity
	witnessAcceptEvents(t, reducer, interrupted)
	witnessAssertOptionalInterruptedFile(t, reducer, file, identity, step)
	retry := witnessCall(22, linuxWitnessRead, "", 9, 24)
	retry[0].ReadIdentity = identity
	witnessAcceptEvents(t, reducer, retry[:1])
	if reducer.retry == nil || reducer.retry.activeTID != 22 || file.bytes != 8 || file.eof || reducer.step != step {
		t.Fatal("optional continuation entry fabricated transferred bytes, EOF, or completion")
	}
	witnessAcceptEvents(t, reducer, retry[1:])
	if reducer.retry != nil || reducer.files[9] != file || file.bytes != 32 || file.eof || file.failed || reducer.step != step {
		t.Fatal("exact optional continuation did not resolve debt on the same live FD")
	}
	witnessAcceptEvents(t, reducer, witnessCall(22, linuxWitnessRead, "", 9, 0))
	witnessAcceptEvents(t, reducer, witnessCall(witnessLeader, linuxWitnessClose, "", 9, 0))
	if reducer.step != step+1 || reducer.files[9] != nil {
		t.Fatal("optional evidence did not advance only after actual EOF and close")
	}
	reading := witnessReading()
	tail := slices.IndexFunc(reading, func(event linuxWitnessEvent) bool {
		return event.Entry && event.Operation == linuxWitnessOpen && event.Path == "/sys/fs/cgroup/memory.max"
	})
	if tail < 0 {
		t.Fatal("optional continuation control lost its source-declared tail")
	}
	witnessAcceptEvents(t, reducer, reading[tail:])
	witnessAcceptEvents(t, reducer, reading)
	trace, err := reducer.finish()
	if err != nil || trace.Readings != 2 || trace.Active != 0 || trace.MaximumActive != 1 || !trace.Closed {
		t.Fatalf("exact optional userspace continuation lost two complete readings: %+v %v", trace, err)
	}
}

func witnessAssertOptionalInterruptedFile(t *testing.T, reducer *linuxCollectorReducer, file *linuxWitnessFile, identity linuxWitnessReadIdentity, step int) {
	t.Helper()
	if reducer.retry == nil || reducer.retry.file != file || reducer.retry.descriptor != 9 || reducer.retry.identity != identity || reducer.files[9] != file || file.bytes != 8 || file.eof || file.failed || reducer.step != step || reducer.trace.Active != 1 || reducer.trace.Closed {
		t.Fatal("optional EINTR completed evidence, lost bytes, or replaced its live FD generation")
	}
}

func TestLinuxCollectorWitnessPendingMeminfoBeginWinsOverLiveReadRetryDebt(t *testing.T) {
	identity := linuxWitnessReadIdentity{Number: 63, Buffer: 4096, Count: 32, Observed: true}
	reducer, err := witnessInterruptedReadDebt(identity)
	if err != nil {
		t.Fatal(err)
	}
	retry, file := reducer.retry, reducer.files[9]
	if file == nil {
		t.Fatal("pending BEGIN control requires its live interrupted FD")
	}
	continuation := witnessCall(witnessLeader, linuxWitnessRead, "", 9, 32)
	continuation[0].ReadIdentity = identity
	witnessAcceptEvents(t, reducer, continuation[:1])
	begin := linuxWitnessEvent{TID: 22, Operation: linuxWitnessOpen, Path: linuxWitnessMeminfo, Entry: true}
	// Completion appears first in this harvested batch. Actual BEGIN still
	// has priority, including while the earlier reading has live retry debt.
	err = reducer.batch([]linuxWitnessEvent{continuation[1], begin})
	if !errors.Is(err, errLinuxWitnessCollectorOverlap) || reducer.trace.Readings != 2 || reducer.trace.MaximumActive != 2 || reducer.trace.Active != 1 || reducer.trace.Closed || reducer.step != 0 {
		t.Fatalf("pending BEGIN was hidden behind read-debt continuation: %+v %v", reducer.trace, err)
	}
	if reducer.retry != retry || retry.file != file || retry.activeTID != witnessLeader || file.bytes != 0 || file.eof || file.failed {
		t.Fatal("overlap refusal completed or discarded the unresolved earlier read")
	}
	if pending, exists := reducer.pending[witnessLeader]; !exists || pending.FD != 9 || pending.ReadIdentity != identity {
		t.Fatal("overlap refusal consumed the still-pending exact read continuation")
	}
	if trace, err := reducer.finish(); !errors.Is(err, errLinuxWitnessInterruptedEvidence) || trace.Closed {
		t.Fatal("unresolved interrupted reading became a successful finish", trace, err)
	}
}

func TestLinuxCollectorWitnessUnqualifiedInterruptionsRemainUnknown(t *testing.T) {
	for _, result := range []int64{-4, -512, -513, -514, -516} {
		for _, operation := range []string{linuxWitnessOpen, linuxWitnessClose, linuxWitnessStatFS} {
			events := append(witnessReading(), witnessReading()...)
			for index, event := range events {
				if event.Entry && event.Operation == operation && (operation != linuxWitnessOpen || event.Path == "/sys/fs/cgroup/memory.max") {
					events[index+1].Result = result
					break
				}
			}
			if trace, err := witnessRun(events); !errors.Is(err, errLinuxWitnessInterruptedEvidence) || trace.Closed {
				t.Fatalf("unproved interruption became collector success/failure: op=%s result=%d trace=%+v err=%v", operation, result, trace, err)
			}
		}
	}
	for _, result := range []int64{-512, -513, -514, -516} {
		events := append(witnessReading(), witnessReading()...)
		events[3].Result = result
		if _, err := witnessRun(events); !errors.Is(err, errLinuxWitnessInterruptedEvidence) {
			t.Fatalf("unproved kernel restart became mandatory read failure: %d %v", result, err)
		}
	}
}

func TestLinuxCollectorWitnessInterruptionsDoNotAdvanceOptionalOrMandatoryEvidence(t *testing.T) {
	for _, path := range []string{linuxWitnessMeminfo, "/proc/self/cgroup"} {
		for _, operation := range []string{linuxWitnessOpen, linuxWitnessRead} {
			for _, result := range []int64{-512, -513, -514, -516} {
				witnessAssertInterruptedStep(t, path, operation, result)
			}
		}
	}
}

func witnessInterruptedPrefix(t *testing.T, path, operation string) ([]linuxWitnessEvent, int) {
	t.Helper()
	events := witnessFile(linuxWitnessMeminfo, true)
	if path != linuxWitnessMeminfo {
		events = append(events, witnessFile("/proc/stat", true)...)
		events = append(events, witnessFile(path, true)...)
	}
	for position, event := range events {
		if !event.Entry || event.Operation != operation {
			continue
		}
		open := operation == linuxWitnessOpen && event.Path == path
		read := operation == linuxWitnessRead && position >= 2 && events[position-2].Path == path
		if open || read {
			return events, position + 1
		}
	}
	t.Fatal("interruption control did not select a real normalized syscall")
	return nil, 0
}

func witnessAssertInterruptedStep(t *testing.T, path, operation string, result int64) {
	t.Helper()
	events, index := witnessInterruptedPrefix(t, path, operation)
	reducer := newLinuxCollectorReducer(witnessLeader, "/owned/disk")
	if err := reducer.addThread(witnessLeader, 22); err != nil {
		t.Fatal(err)
	}
	for _, event := range events[:index] {
		if err := reducer.batch([]linuxWitnessEvent{event}); err != nil {
			t.Fatal(err)
		}
	}
	before := reducer.step
	interrupted := events[index]
	interrupted.Result = result
	if err := reducer.batch([]linuxWitnessEvent{interrupted}); !errors.Is(err, errLinuxWitnessInterruptedEvidence) || reducer.step != before || reducer.trace.Active != 1 || reducer.trace.Closed {
		t.Fatalf("restart advanced source evidence: %s %s %d step%d->%d err=%v", path, operation, result, before, reducer.step, err)
	}
}

func witnessInterruptedReadDebt(identity linuxWitnessReadIdentity) (*linuxCollectorReducer, error) {
	reducer := newLinuxCollectorReducer(witnessLeader, "/owned/disk")
	if err := reducer.addThread(witnessLeader, 22); err != nil {
		return reducer, err
	}
	events := witnessFile(linuxWitnessMeminfo, true)[:4]
	events[2].ReadIdentity, events[3].Result = identity, -4
	for _, event := range events {
		if err := reducer.batch([]linuxWitnessEvent{event}); err != nil {
			return reducer, err
		}
	}
	return reducer, nil
}

func witnessAssertDebtTransition(t *testing.T, reducer *linuxCollectorReducer, mode string, identity linuxWitnessReadIdentity) {
	t.Helper()
	entry := linuxWitnessEvent{TID: witnessLeader, Operation: linuxWitnessRead, FD: 9, Entry: true, ReadIdentity: identity}
	if mode == "close" {
		entry.Operation = linuxWitnessClose
	}
	if mode == "generation" {
		reducer.files[9] = &linuxWitnessFile{path: linuxWitnessMeminfo}
	}
	if mode == "parallel" {
		if err := reducer.batch([]linuxWitnessEvent{entry}); err != nil {
			t.Fatal(err)
		}
		entry.TID = 22
	}
	if err := reducer.batch([]linuxWitnessEvent{entry}); !errors.Is(err, errLinuxWitnessInterruptedEvidence) {
		t.Fatalf("unproved read-debt transition accepted: %s %v", mode, err)
	}
}

func TestLinuxCollectorWitnessUnresolvedOrParallelReadDebtRefusesProof(t *testing.T) {
	identity := linuxWitnessReadIdentity{Number: 63, Buffer: 4096, Count: 32, Observed: true}
	for _, mode := range []string{"unresolved", "close", "parallel", "generation", "missing-identity"} {
		t.Run(mode, func(t *testing.T) {
			observed := identity
			if mode == "missing-identity" {
				observed = linuxWitnessReadIdentity{}
			}
			reducer, err := witnessInterruptedReadDebt(observed)
			if mode == "missing-identity" {
				if !errors.Is(err, errLinuxWitnessInterruptedEvidence) {
					t.Fatal("missing observed read invocation created a retry debt", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "unresolved" {
				if _, err := reducer.finish(); !errors.Is(err, errLinuxWitnessInterruptedEvidence) {
					t.Fatal("unresolved debt accepted or attributed to collector failure", err)
				}
				return
			}
			witnessAssertDebtTransition(t, reducer, mode, identity)
		})
	}
}

func TestLinuxCollectorWitnessRejectsTruncatedAndForeignABI(t *testing.T) {
	for _, architecture := range []uint32{linuxAuditAMD64, linuxAuditARM64} {
		for _, operation := range []uint8{1, 2} {
			length := uintptr(80)
			if operation == 2 {
				length = 33
			}
			valid := linuxSyscallInfo{Operation: operation, Architecture: architecture}
			if err := valid.validate(length, 88, architecture); err != nil {
				t.Fatal(err)
			}
			if err := valid.validate(length-1, 88, architecture); err == nil {
				t.Fatal("actual syscall ABI truncation accepted")
			}
			if err := valid.validate(length, 87, architecture); err == nil {
				t.Fatal("wrong local ABI layout accepted")
			}
			valid.Architecture = 0
			if err := valid.validate(length, 88, architecture); err == nil {
				t.Fatal("unknown kernel architecture accepted")
			}
		}
	}
	for _, operation := range []uint8{0, 3, 255} {
		if err := (linuxSyscallInfo{Operation: operation, Architecture: linuxAuditAMD64}).validate(88, 88, linuxAuditAMD64); err == nil {
			t.Fatal("unknown syscall operation accepted")
		}
	}
	if err := (linuxSyscallInfo{Operation: 2, Architecture: linuxAuditAMD64, Data: [8]uint64{0, 2}}).validate(33, 88, linuxAuditAMD64); err == nil {
		t.Fatal("invalid kernel error flag accepted")
	}
	if err := (linuxSyscallInfo{Operation: 1, Architecture: linuxAuditAMD64}).validate(80, 88, linuxAuditARM64); err == nil {
		t.Fatal("foreign architecture substituted for native ABI")
	}
}

func TestLinuxCollectorWitnessSignedKernelResults(t *testing.T) {
	for _, test := range []struct {
		bits   uint64
		result int64
	}{
		{0, 0}, {1, 1}, {math.MaxInt64, math.MaxInt64}, {1 << 63, math.MinInt64}, {math.MaxUint64, -1}, {math.MaxUint64 - 12, -13},
	} {
		if actual, err := linuxWitnessSigned(test.bits); err != nil || actual != test.result {
			t.Fatalf("signed kernel result lost errno or descriptor: %d != %d error=%v", actual, test.result, err)
		}
	}
}

const witnessLeader = 21

func witnessCall(tid int, operation, path string, fd int, result int64) []linuxWitnessEvent {
	return []linuxWitnessEvent{
		{TID: tid, Operation: operation, Path: path, FD: fd, Entry: true},
		{TID: tid, Operation: operation, Result: result},
	}
}

func witnessFile(path string, mandatory bool) []linuxWitnessEvent {
	if !mandatory {
		return witnessCall(witnessLeader, "open", path, 0, -2)
	}
	result := witnessCall(witnessLeader, "open", path, 0, 9)
	result = append(result, witnessCall(22, "read", "", 9, 32)...)
	result = append(result, witnessCall(witnessLeader, "read", "", 9, 0)...)
	return append(result, witnessCall(22, "close", "", 9, 0)...)
}

func witnessReading() []linuxWitnessEvent {
	result := witnessFile("/proc/meminfo", true)
	result = append(result, witnessFile("/proc/stat", true)...)
	for _, path := range []string{"/proc/self/cgroup", "/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory.high", "/sys/fs/cgroup/memory.current", "/sys/fs/cgroup/memory.stat", "/sys/fs/cgroup/cpu.max", "/sys/fs/cgroup/memory.pressure", "/proc/pressure/memory", "/sys/fs/cgroup/memory.events", "/sys/fs/cgroup/memory.swap.max", "/sys/fs/cgroup/memory.swap.current"} {
		result = append(result, witnessFile(path, false)...)
	}
	result = append(result, witnessCall(witnessLeader, "statfs", "/owned/disk", 0, 0)...)
	return append(result, witnessFile("/proc/vmstat", false)...)
}

func witnessRun(events []linuxWitnessEvent) (PublicTUIHostProbeTrace, error) {
	reducer := newLinuxCollectorReducer(witnessLeader, "/owned/disk")
	if err := reducer.addThread(witnessLeader, 22); err != nil {
		return reducer.trace, err
	}
	for _, event := range events {
		if err := reducer.batch([]linuxWitnessEvent{event}); err != nil {
			return reducer.trace, err
		}
	}
	return reducer.finish()
}

func TestLinuxCollectorWitnessTwoCompleteIntervalsAcrossThreads(t *testing.T) {
	events := append(witnessReading(), witnessReading()...)
	trace, err := witnessRun(events)
	if err != nil || trace.Readings != 2 || trace.Active != 0 || trace.MaximumActive != 1 || !trace.Closed {
		t.Fatalf("two migrated, ordered kernel reading intervals rejected: %+v %v", trace, err)
	}
}

func TestLinuxCollectorWitnessUsesExactPositiveLeaderCohort(t *testing.T) {
	for _, input := range []struct {
		leader int
		disk   string
	}{{1, "/owned/disk"}, {witnessLeader, "/owned/other"}, {301, "/owned/nested/disk"}} {
		witnessAssertExactCohort(t, input.leader, input.disk)
	}
}

func witnessAssertExactCohort(t *testing.T, leader int, disk string) {
	t.Helper()
	reducer := newLinuxCollectorReducer(leader, disk)
	if err := reducer.addThread(leader, leader+1); err != nil {
		t.Fatal(err)
	}
	if err := reducer.begin(leader + 2); err == nil || reducer.trace.Readings != 0 {
		t.Fatal("foreign leader identity entered the witness")
	}
	events := append(witnessReading(), witnessReading()...)
	for _, event := range events {
		if event.Entry && event.Operation == linuxWitnessStatFS {
			if err := reducer.validateEntry(linuxWitnessEvent{Operation: linuxWitnessStatFS, Path: disk + "/foreign"}); err == nil {
				t.Fatal("foreign disk path entered the witness")
			}
			event.Path = disk
		}
		if event.TID == witnessLeader {
			event.TID = leader
		} else {
			event.TID = leader + 1
		}
		if err := reducer.batch([]linuxWitnessEvent{event}); err != nil {
			t.Fatal(err)
		}
	}
	trace, err := reducer.finish()
	if err != nil || !trace.Closed || trace.Readings != 2 || trace.MaximumActive != 1 {
		t.Fatal("positive leader cohort lost exact readings", trace, err)
	}
	for _, event := range trace.Events {
		if event.PID != leader && event.PID != leader+1 {
			t.Fatal("trace metadata borrowed another leader identity", event)
		}
	}
}

func TestLinuxCollectorWitnessOptionalEvidenceAndPressureFallback(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		var reading []linuxWitnessEvent
		for _, path := range []string{"/proc/meminfo", "/proc/stat", "/proc/self/cgroup", "/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory.high", "/sys/fs/cgroup/memory.current", "/sys/fs/cgroup/memory.stat", "/sys/fs/cgroup/cpu.max", "/sys/fs/cgroup/memory.pressure"} {
			reading = append(reading, witnessFile(path, path != "/sys/fs/cgroup/memory.pressure" || !fallback)...)
		}
		if fallback {
			reading = append(reading, witnessFile("/proc/pressure/memory", true)...)
		}
		for _, path := range []string{"/sys/fs/cgroup/memory.events", "/sys/fs/cgroup/memory.swap.max", "/sys/fs/cgroup/memory.swap.current"} {
			reading = append(reading, witnessFile(path, true)...)
		}
		reading = append(reading, witnessCall(witnessLeader, "statfs", "/owned/disk", 0, 0)...)
		reading = append(reading, witnessFile("/proc/vmstat", true)...)
		if trace, err := witnessRun(append(slices.Clone(reading), reading...)); err != nil || trace.Readings != 2 {
			t.Fatalf("genuine optional-read protocol or pressure fallback rejected: %+v %v", trace, err)
		}
	}
}

func TestLinuxCollectorWitnessRejectsIncompleteOrAlteredKernelEvidence(t *testing.T) {
	for _, test := range []struct {
		name  string
		alter func([]linuxWitnessEvent) []linuxWitnessEvent
	}{
		{"one-reading", func(events []linuxWitnessEvent) []linuxWitnessEvent { return events[:len(events)/2] }},
		{"three-readings", func(events []linuxWitnessEvent) []linuxWitnessEvent { return append(events, witnessReading()...) }},
		{"no-mandatory-bytes", func(events []linuxWitnessEvent) []linuxWitnessEvent { events[3].Result = 0; return events }},
		{"failed-memory-open", func(events []linuxWitnessEvent) []linuxWitnessEvent { events[1].Result = -13; return events }},
		{"missing-eof", func(events []linuxWitnessEvent) []linuxWitnessEvent { events[5].Result = 2; return events }},
		{"wrong-tail-order", func(events []linuxWitnessEvent) []linuxWitnessEvent {
			events[20].Path = "/sys/fs/cgroup/memory.current"
			return events
		}},
		{"wrong-disk", func(events []linuxWitnessEvent) []linuxWitnessEvent {
			for index := range events {
				if events[index].Operation == "statfs" {
					events[index].Path = "/foreign/disk"
				}
			}
			return events
		}},
		{"failed-disk", func(events []linuxWitnessEvent) []linuxWitnessEvent {
			for index := range events {
				if events[index].Operation == "statfs" && !events[index].Entry {
					events[index].Result = -5
				}
			}
			return events
		}},
		{"foreign-thread", func(events []linuxWitnessEvent) []linuxWitnessEvent { events[2].TID = 99; return events }},
		{"duplicate-entry", func(events []linuxWitnessEvent) []linuxWitnessEvent { return slices.Insert(events, 1, events[0]) }},
		{"missing-terminal-attempt", func(events []linuxWitnessEvent) []linuxWitnessEvent { return events[:len(events)-2] }},
		{"unqualified-descriptor", func(events []linuxWitnessEvent) []linuxWitnessEvent { events[2].FD = 77; return events }},
		{"descriptor-reuse", func(events []linuxWitnessEvent) []linuxWitnessEvent {
			return slices.Insert(events, 2, witnessCall(witnessLeader, "open", "/owned/unrelated", 0, 9)...)
		}},
		{"changed-cgroup-root", func(events []linuxWitnessEvent) []linuxWitnessEvent {
			events[20].Path = "/sys/fs/cgroup/foreign/memory.high"
			return events
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			events := test.alter(append(witnessReading(), witnessReading()...))
			if _, err := witnessRun(events); err == nil {
				t.Fatal("altered kernel evidence proved a two-reading witness")
			}
		})
	}
}

func TestLinuxCollectorWitnessRefusesOverlappingAndSimultaneouslyPendingStarts(t *testing.T) {
	for _, simultaneous := range []bool{false, true} {
		reducer := newLinuxCollectorReducer(witnessLeader, "/owned/disk")
		if err := reducer.addThread(witnessLeader, 22); err != nil {
			t.Fatal(err)
		}
		first := witnessReading()
		for _, event := range first[:len(first)-1] {
			if err := reducer.batch([]linuxWitnessEvent{event}); err != nil {
				t.Fatal(err)
			}
		}
		start := linuxWitnessEvent{TID: 22, Operation: "open", Path: "/proc/meminfo", Entry: true}
		batch := []linuxWitnessEvent{start}
		if simultaneous {
			batch = []linuxWitnessEvent{first[len(first)-1], start}
		}
		if err := reducer.batch(batch); err == nil || reducer.trace.MaximumActive != 2 {
			t.Fatalf("pending concurrent start was hidden by completion ordering: %+v %v", reducer.trace, err)
		}
	}
}

func TestLinuxCollectorWitnessRejectsForeignCloneOwnership(t *testing.T) {
	for _, pair := range [][2]int{{99, 22}, {witnessLeader, witnessLeader}, {witnessLeader, 0}} {
		reducer := newLinuxCollectorReducer(witnessLeader, "/owned/disk")
		if err := reducer.addThread(pair[0], pair[1]); err == nil {
			t.Fatal("unobserved or duplicate thread joined the trace cohort")
		}
	}
}

func linuxNativePrivateRoot(t *testing.T, initialize func(string) error) string {
	t.Helper()
	directory := t.TempDir()
	if initialize != nil {
		if err := initialize(directory); err != nil {
			t.Fatal(err)
		}
	}
	root, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	// testing.TempDir does not guarantee 0700 on its returned leaf. Enforce
	// privacy only on this freshly test-owned canonical root, then validate it.
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := validateOwnedCandidateDirectory(root); err != nil {
		t.Fatalf("own native helper root: %v", err)
	}
	return root
}

func TestLinuxCollectorWitnessNativeRootPrivacy(t *testing.T) {
	t.Run("prepare newly owned nonprivate leaf", func(t *testing.T) {
		root := linuxNativePrivateRoot(t, func(directory string) error {
			return os.Chmod(directory, 0o755)
		})
		canonical, err := filepath.EvalSymlinks(root)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Lstat(root)
		if err != nil {
			t.Fatal(err)
		}
		if canonical != root || !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Fatal("owned helper root is not canonical and private", root, info.Mode())
		}
		if err := validateOwnedCandidateDirectory(root); err != nil {
			t.Fatal("owned private root remains invalid", err)
		}
	})
	t.Run("validator refuses nonprivate leaf", func(t *testing.T) {
		root, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := validateOwnedCandidateDirectory(root); err == nil {
			t.Fatal("validator accepted a nonprivate owned root")
		}
	})
}

func TestLinuxCollectorWitnessResumeMetadataPresentation(t *testing.T) {
	metadata := linuxWitnessFailureMetadata{
		Leader: 11, TID: 22, Signal: 10,
		Stop:  linuxWitnessStopMetadata{Status: 1407, Signal: 5, Event: 0, Kind: 4, Phase: 1, NumberKnown: true, Number: 231},
		Last:  linuxWitnessResumeMetadata{Known: true, TID: 33, Signal: 0, Stop: linuxWitnessStopMetadata{Phase: 2, NumberKnown: true, Number: 202}},
		Owned: 3, Held: 2, Detached: 1, Readings: 2, Active: 0, MaximumActive: 1,
		Step: 14, Steps: 14, Events: 90, Pending: 1, Files: 2, PreludePending: 3, PreludeFiles: 4,
		Retry: true, RetryTID: 44, Context: 1,
	}
	for _, original := range []error{syscall.ESRCH, syscall.EPERM} {
		err := linuxWitnessResumeError(22, original, metadata)
		if !errors.Is(err, original) || !errors.Is(errors.Join(err, errors.New("cleanup failed")), original) {
			t.Fatal("resume diagnostic lost original errno identity", err)
		}
		for _, want := range []string{"Leader:11", "TID:22", "Number:231", "Number:202", "Known:true", "Owned:3", "Held:2", "Detached:1", "Readings:2", "Active:0", "MaximumActive:1", "Closed:false", "Step:14", "Steps:14", "Events:90", "Pending:1", "Files:2", "PreludePending:3", "PreludeFiles:4", "Retry:true", "RetryTID:44", "Context:canceled"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("resume diagnostic lacks observed field %q: %v", want, err)
			}
		}
	}
}

func TestLinuxCollectorWitnessResumeMetadataSyscallAssociation(t *testing.T) {
	var observation linuxWitnessObservation
	if observation.held.NumberKnown || observation.entryKnown {
		t.Fatal("initial observation invented a syscall")
	}
	observation.stop(1407, 5, 0, 4)
	observation.syscall(1, 231)
	if !observation.held.NumberKnown || observation.held.Number != 231 || observation.held.Phase != 1 {
		t.Fatal("ignored ENTRY lost its observed syscall number", observation)
	}
	last := linuxWitnessSuccessfulResume(22, 0, observation.held)
	originalLast := last
	observation.stop(196991, 5, 3, 3)
	if observation.held.Status != 196991 || observation.held.Signal != 5 || observation.held.Event != 3 || observation.held.Kind != 3 {
		t.Fatal("non-syscall stop lost exact numeric observations", observation)
	}
	observation.stop(2687, 10, 0, 6)
	if observation.held.Phase != 0 || observation.held.NumberKnown || observation.held.Number != 0 {
		t.Fatal("signal stop invented a current syscall", observation)
	}
	observation.stop(1407, 5, 0, 4)
	observation.syscall(2, math.MaxUint64)
	if !observation.held.NumberKnown || observation.held.Number != 231 || observation.held.Phase != 2 {
		t.Fatal("EXIT result replaced the observed ENTRY number", observation)
	}
	if last != originalLast || !last.Known || last.Stop.Phase != 1 || last.Stop.Number != 231 {
		t.Fatal("later observation rewrote the copied successful resume", last)
	}
	observation.stop(1407, 5, 0, 4)
	observation.syscall(2, 202)
	if observation.held.NumberKnown || observation.held.Number != 0 {
		t.Fatal("unpaired EXIT treated result as a syscall number", observation)
	}
	observation.stop(1407, 5, 0, 4)
	observation.syscall(1, 0)
	if !observation.held.NumberKnown || observation.held.Number != 0 {
		t.Fatal("valid syscall zero became unknown", observation)
	}
}

func TestLinuxCollectorWitnessResumeMetadataSnapshotIsBoundedAndImmutable(t *testing.T) {
	const disclosureSentinel = "subject-path-buffer-payload-must-not-appear"
	reducer := newLinuxCollectorReducer(11, disclosureSentinel)
	file := &linuxWitnessFile{}
	reducer.files[9] = file
	reducer.pending[22] = linuxWitnessEvent{Path: disclosureSentinel, ReadIdentity: linuxWitnessReadIdentity{Buffer: math.MaxUint64}}
	reducer.prelude.files[30] = disclosureSentinel
	reducer.prelude.pending[22] = true
	reducer.trace.Events = []PublicTUIHostProbeEvent{{Operation: disclosureSentinel}}
	reducer.trace.Readings, reducer.trace.Active, reducer.trace.MaximumActive = 2, 1, 2
	reducer.step = 3
	reducer.retry = &linuxWitnessReadRetry{file: file, activeTID: 22, identity: linuxWitnessReadIdentity{Buffer: math.MaxUint64}}
	before := *reducer
	before.files, before.pending = maps.Clone(reducer.files), maps.Clone(reducer.pending)
	before.prelude.files, before.prelude.pending = maps.Clone(reducer.prelude.files), maps.Clone(reducer.prelude.pending)
	before.trace.Events = slices.Clone(reducer.trace.Events)
	beforeRetry, beforeFile := *reducer.retry, *file
	last := linuxWitnessSuccessfulResume(22, 0, linuxWitnessStopMetadata{NumberKnown: true, Number: 231})
	metadata := reducer.failureMetadata(linuxWitnessFailureMetadata{Leader: 11, TID: 33, Last: last})
	if metadata.Readings != 2 || metadata.Active != 1 || metadata.MaximumActive != 2 || metadata.Closed || metadata.Step != 3 || metadata.Steps != len(reducer.steps) || metadata.Events != 1 || metadata.Pending != 1 || metadata.Files != 1 || metadata.PreludePending != 1 || metadata.PreludeFiles != 1 || !metadata.Retry || metadata.RetryTID != 22 {
		t.Fatal("snapshot lost observed reducer state", metadata)
	}
	formatted := linuxWitnessResumeError(33, syscall.ESRCH, metadata).Error()
	if strings.Contains(formatted, disclosureSentinel) || strings.Contains(formatted, "18446744073709551615") || len(formatted) > 2048 || metadata.Last != last {
		t.Fatal("snapshot disclosed subject data, exceeded bound or rewrote last resume", formatted)
	}
	if !reflect.DeepEqual(*reducer, before) || *reducer.retry != beforeRetry || *file != beforeFile {
		t.Fatal("snapshot or formatting mutated collector evidence")
	}
	reducer.retry = nil
	metadata = reducer.failureMetadata(metadata)
	if metadata.Retry || metadata.RetryTID != 0 {
		t.Fatal("snapshot kept stale retry debt", metadata)
	}
}

func TestLinuxCollectorWitnessResumeMetadataMaximumScalarsAndContext(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{"live", nil, "nil"},
		{"canceled", context.Canceled, "canceled"},
		{"expired", context.DeadlineExceeded, "deadline-exceeded"},
		{"unexpected", errors.New("private-context-error-must-not-appear"), "unexpected-context-error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			metadata := linuxWitnessFailureMetadata{Context: linuxWitnessContextObservation(test.err)}
			witnessMetadataMaximumScalars(reflect.ValueOf(&metadata).Elem())
			metadata.Context = linuxWitnessContextObservation(test.err)
			err := linuxWitnessResumeError(math.MaxInt, syscall.ESRCH, metadata)
			if len(err.Error()) > 2048 || !strings.Contains(err.Error(), "Context:"+test.want) || strings.Contains(err.Error(), "private-context-error-must-not-appear") || !errors.Is(err, syscall.ESRCH) {
				t.Fatal("scalar diagnostic exceeded bound, disclosed context text or lost errno", err)
			}
		})
	}
}

func witnessMetadataMaximumScalars(value reflect.Value) {
	for _, field := range value.Fields() {
		switch field.Kind() {
		case reflect.Struct:
			witnessMetadataMaximumScalars(field)
		case reflect.Int:
			field.SetInt(math.MaxInt)
		case reflect.Int32:
			field.SetInt(math.MaxInt32)
		case reflect.Uint32:
			field.SetUint(math.MaxUint32)
		case reflect.Uint8:
			field.SetUint(math.MaxUint8)
		case reflect.Uint64:
			field.SetUint(math.MaxUint64)
		case reflect.Bool:
			field.SetBool(true)
		case reflect.Invalid, reflect.Int8, reflect.Int16, reflect.Int64, reflect.Uint, reflect.Uint16, reflect.Uintptr,
			reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128, reflect.Array, reflect.Chan, reflect.Func,
			reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.String, reflect.UnsafePointer:
			panic("diagnostic added a non-scalar field")
		}
	}
}
