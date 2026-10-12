package support

import (
	"context"
	"errors"
	"math"
	"slices"
	"testing"
	"time"
)

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
