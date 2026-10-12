package support

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type linuxStopResult struct {
	event    linuxWitnessEvent
	observed bool
}

type linuxWitnessHarvest struct {
	result              linuxStopResult
	acquired, uncertain bool
}

// This consumer owns the complete known-TID scan; the native adapter alone
// owns Wait4, stop classification, cohort changes and held-stop retirement.
type linuxWitnessFrontierSource interface {
	linuxWitnessTIDs() []int
	linuxWitnessHeld(tid int) bool
	linuxWitnessHarvest(tid int, initial bool) (linuxWitnessHarvest, error)
}

func linuxWitnessDrainFrontier(ctx context.Context, source linuxWitnessFrontierSource, initial bool) ([]linuxWitnessEvent, error) {
	var events []linuxWitnessEvent
	// Linux v6.8 kernel/signal.c ptrace_stop publishes TASK_TRACED/exit_code
	// before notifying the parent; the stop persists until our resume. A full
	// scan after the latest harvested completion must find any earlier pending
	// ENTRY. This proves the published-stop frontier, not atomic quiescence.
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cohort := source.linuxWitnessTIDs()
		progress := false
		for _, tid := range cohort {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if source.linuxWitnessHeld(tid) {
				continue
			}
			harvest, err := source.linuxWitnessHarvest(tid, initial)
			if err != nil {
				return nil, err
			}
			progress = progress || harvest.acquired || harvest.uncertain
			if harvest.result.observed {
				events = append(events, harvest.result.event)
			}
		}
		if !progress && slices.Equal(cohort, source.linuxWitnessTIDs()) {
			return events, nil
		}
	}
}

// Linux v6.8 UAPI include/uapi/linux/ptrace.h defines the typed info request
// (lines 69-95) and EXITKILL (line 162), absent from frozen syscall APIs.
const (
	linuxGetSyscallInfo = 0x420e
	linuxTraceExitKill  = 0x100000
	linuxAuditAMD64     = 0xc000003e
	linuxAuditARM64     = 0xc00000b7
	linuxWitnessOpen    = "open"
	linuxWitnessRead    = "read"
	linuxWitnessClose   = "close"
	linuxWitnessStatFS  = "statfs"
	linuxWitnessMeminfo = "/proc/meminfo"
)

// linuxSyscallInfo matches the fixed-width UAPI. kernel/ptrace.c returns 80
// bytes for ENTRY and 33 for EXIT, through actual fields, not padded unions.
type linuxSyscallInfo struct {
	Operation          uint8
	Padding            [3]byte
	Architecture       uint32
	Instruction, Stack uint64
	Data               [8]uint64
}

func (info linuxSyscallInfo) validate(length, size uintptr, architecture uint32) error {
	minimum := uintptr(80)
	if info.Operation == 2 {
		minimum = 33
	}
	if length < minimum || info.Operation != 1 && info.Operation != 2 || size != 88 || architecture != linuxAuditAMD64 && architecture != linuxAuditARM64 || info.Architecture != architecture {
		return errors.New("linux syscall witness returned truncated or unsupported ABI evidence")
	}
	if info.Operation == 2 && info.Data[1]&0xff > 1 {
		return errors.New("linux syscall witness returned an invalid error flag")
	}
	return nil
}

func linuxWitnessSigned(value uint64) (int64, error) {
	if value <= math.MaxInt64 {
		return int64(value), nil
	}
	magnitude := ^value
	if magnitude > math.MaxInt64 {
		return 0, errors.New("linux syscall signed result exceeds representable bounds")
	}
	return -1 - int64(magnitude), nil
}

// linuxWitnessEvent is metadata from an owned kernel syscall stop, never read content.
type linuxWitnessEvent struct {
	TID, FD         int
	Operation, Path string
	Entry           bool
	Result          int64
	ReadIdentity    linuxWitnessReadIdentity
}

// ReadIdentity is the observed syscall invocation, never the buffer's contents.
type linuxWitnessReadIdentity struct {
	Number, Buffer, Count uint64
	Observed              bool
}

var errLinuxWitnessInterruptedEvidence = errors.New("linux collector witness cannot prove interrupted syscall continuity")

var errLinuxWitnessEvidenceOrder = errors.New("linux collector evidence path order changed")

var errLinuxWitnessCollectorOverlap = errors.New("linux collector readings overlap")

type linuxCollectorReducer struct {
	trace        PublicTUIHostProbeTrace
	threads      map[int]bool
	pending      map[int]linuxWitnessEvent
	files        map[int]*linuxWitnessFile
	disk, cgroup string
	steps        []string
	step         int
	retry        *linuxWitnessReadRetry
}

type linuxWitnessReadRetry struct {
	file       *linuxWitnessFile
	identity   linuxWitnessReadIdentity
	descriptor int
	activeTID  int
}

type linuxWitnessFile struct {
	path        string
	bytes       int64
	eof, failed bool
}

func newLinuxCollectorReducer(leader int, disk string) *linuxCollectorReducer {
	return &linuxCollectorReducer{threads: map[int]bool{leader: true}, pending: map[int]linuxWitnessEvent{}, files: map[int]*linuxWitnessFile{}, disk: disk}
}

func (reducer *linuxCollectorReducer) addThread(parent, child int) error {
	if !reducer.threads[parent] || child <= 0 || reducer.threads[child] || len(reducer.threads) >= 64 {
		return errors.New("linux witness refused foreign, duplicate or excessive clone")
	}
	reducer.threads[child] = true
	return nil
}

// batch counts all pending starts before any completion can hide an overlap.
func (reducer *linuxCollectorReducer) batch(events []linuxWitnessEvent) error {
	for _, event := range events {
		if event.Entry && event.Operation == linuxWitnessOpen && event.Path == linuxWitnessMeminfo {
			if err := reducer.begin(event.TID); err != nil {
				return err
			}
		}
	}
	for _, event := range events {
		if err := reducer.accept(event); err != nil {
			return err
		}
	}
	return nil
}

func (reducer *linuxCollectorReducer) begin(tid int) error {
	if !reducer.threads[tid] {
		return errors.New("linux witness received foreign thread")
	}
	reducer.trace.Readings++
	reducer.trace.MaximumActive = max(reducer.trace.MaximumActive, reducer.trace.Active+1)
	if reducer.trace.Active != 0 {
		return errLinuxWitnessCollectorOverlap
	}
	if reducer.trace.Readings > 2 {
		return errors.New("linux collector produced excess readings")
	}
	reducer.trace.Active = 1
	reducer.step, reducer.cgroup = 0, ""
	reducer.steps = []string{linuxWitnessMeminfo, "/proc/stat", "/proc/self/cgroup", "memory.max", "memory.high", "memory.current", "memory.stat", "cpu.max", "memory.pressure", "memory.events", "memory.swap.max", "memory.swap.current", linuxWitnessStatFS, "/proc/vmstat"}
	return nil
}

func (reducer *linuxCollectorReducer) accept(event linuxWitnessEvent) error {
	if !reducer.threads[event.TID] {
		return errors.New("linux witness received foreign thread")
	}
	if event.Entry {
		if _, found := reducer.pending[event.TID]; found {
			return errors.New("linux witness duplicated a syscall entry")
		}
		if err := reducer.validateEntry(event); err != nil {
			return err
		}
		reducer.pending[event.TID] = event
		return nil
	}
	entry, found := reducer.pending[event.TID]
	if !found || entry.Operation != event.Operation {
		return errors.New("linux witness received unpaired syscall exit")
	}
	delete(reducer.pending, event.TID)
	_, watchedFD := reducer.files[entry.FD]
	watched := entry.Operation == linuxWitnessOpen && linuxWitnessPath(entry.Path) || entry.Operation == linuxWitnessStatFS || watchedFD && (entry.Operation == linuxWitnessRead || entry.Operation == linuxWitnessClose)
	if watched && linuxWitnessInterrupted(event.Result) && (event.Result != -4 || entry.Operation != linuxWitnessRead) {
		// Internal kernel restart results are not terminal optional-file failures.
		// Without proved restart control continuity their disposition stays UNKNOWN.
		return errLinuxWitnessInterruptedEvidence
	}
	switch entry.Operation {
	case linuxWitnessOpen:
		return reducer.openResult(entry, event.Result)
	case linuxWitnessRead:
		return reducer.readResult(entry, event.Result)
	case linuxWitnessClose:
		return reducer.closeResult(entry.TID, entry.FD, event.Result)
	case linuxWitnessStatFS:
		if event.Result != 0 {
			return errors.New("linux collector disk statfs failed")
		}
		return reducer.complete(entry.TID, linuxWitnessStatFS, 0, false)
	default:
		return errors.New("linux witness received unknown operation")
	}
}

// Linux v6.8 include/linux/errno.h reserves the restart results below. Go's
// os.Open/internal/poll.Read retry EINTR, but open pointers can be reallocated.
func linuxWitnessInterrupted(result int64) bool {
	return result == -4 || result == -512 || result == -513 || result == -514 || result == -516
}

func linuxWitnessPath(path string) bool {
	return slices.Contains([]string{linuxWitnessMeminfo, "/proc/stat", "/proc/self/cgroup", "/proc/pressure/memory", "/proc/vmstat"}, path) || strings.HasPrefix(path, "/sys/fs/cgroup/")
}

func (reducer *linuxCollectorReducer) validateEntry(event linuxWitnessEvent) error {
	if err := reducer.validateRetryEntry(event); err != nil {
		return err
	}
	if event.Operation == linuxWitnessOpen && linuxWitnessPath(event.Path) {
		return reducer.requirePath(event.Path)
	}
	if event.Operation == linuxWitnessStatFS {
		if event.Path != reducer.disk {
			return errors.New("linux witness disk path changed")
		}
		return reducer.requirePath(linuxWitnessStatFS)
	}
	if event.Operation != linuxWitnessOpen && event.Operation != linuxWitnessRead && event.Operation != linuxWitnessClose {
		return errors.New("linux witness received unknown operation")
	}
	return nil
}

func (reducer *linuxCollectorReducer) validateRetryEntry(event linuxWitnessEvent) error {
	retry := reducer.retry
	if retry == nil {
		return nil
	}
	if event.Operation == linuxWitnessClose && event.FD == retry.descriptor {
		return errLinuxWitnessInterruptedEvidence
	}
	if event.Operation != linuxWitnessRead {
		return nil
	}
	file, watched := reducer.files[event.FD]
	if !watched {
		return nil
	}
	if event.FD != retry.descriptor || file != retry.file || event.ReadIdentity != retry.identity || retry.activeTID != 0 {
		return errLinuxWitnessInterruptedEvidence
	}
	retry.activeTID = event.TID
	return nil
}

func (reducer *linuxCollectorReducer) requirePath(path string) error {
	if reducer.trace.Active != 1 || reducer.step >= len(reducer.steps) {
		return errors.New("linux collector evidence outside a reading")
	}
	expected := reducer.steps[reducer.step]
	if strings.HasPrefix(expected, "/") || expected == linuxWitnessStatFS {
		if path != expected {
			return fmt.Errorf("%w: expected %s", errLinuxWitnessEvidenceOrder, expected)
		}
		return nil
	}
	directory := filepath.Dir(path)
	if filepath.Base(path) != expected || filepath.Clean(path) != path || directory != "/sys/fs/cgroup" && !strings.HasPrefix(directory, "/sys/fs/cgroup/") {
		return errors.New("linux collector cgroup evidence order or root changed")
	}
	if reducer.cgroup == "" {
		reducer.cgroup = directory
	}
	if reducer.cgroup != directory {
		return errors.New("linux collector changed cgroup root within a reading")
	}
	return nil
}

func (reducer *linuxCollectorReducer) openResult(entry linuxWitnessEvent, result int64) error {
	if !linuxWitnessPath(entry.Path) {
		if _, found := reducer.files[int(result)]; result >= 0 && found {
			return errors.New("linux witness descriptor reused before close")
		}
		return nil
	}
	if result < 0 {
		if reducer.step < 2 {
			return errors.New("linux collector mandatory open failed")
		}
		return reducer.complete(entry.TID, entry.Path, 0, true)
	}
	if result > 1<<20 {
		return errors.New("linux witness descriptor exceeds bounds")
	}
	if _, found := reducer.files[int(result)]; found {
		return errors.New("linux witness descriptor reused before close")
	}
	reducer.files[int(result)] = &linuxWitnessFile{path: entry.Path}
	return reducer.record(entry.TID, entry.Path, linuxWitnessOpen)
}

func (reducer *linuxCollectorReducer) readResult(entry linuxWitnessEvent, result int64) error {
	file, found := reducer.files[entry.FD]
	if !found {
		return nil
	}
	if file.eof || file.failed {
		return errors.New("linux collector read after EOF or failure")
	}
	if result == -4 {
		if !entry.ReadIdentity.Observed || entry.ReadIdentity.Buffer == 0 || entry.ReadIdentity.Count == 0 {
			return errLinuxWitnessInterruptedEvidence
		}
		for _, pending := range reducer.pending {
			if pending.Operation == linuxWitnessRead && pending.FD == entry.FD {
				return errLinuxWitnessInterruptedEvidence
			}
		}
		if reducer.retry == nil {
			reducer.retry = &linuxWitnessReadRetry{file: file, identity: entry.ReadIdentity, descriptor: entry.FD}
		} else {
			reducer.retry.activeTID = 0
		}
		// Pinned Go internal/poll.Read holds readLock while ignoringEINTRIO
		// retries exactly the same fd and slice. Migration does not change it.
		return reducer.record(entry.TID, file.path, "read-interrupted")
	}
	if reducer.retry != nil {
		if reducer.retry.activeTID != entry.TID || reducer.retry.descriptor != entry.FD || reducer.retry.identity != entry.ReadIdentity || reducer.retry.file != file {
			return errLinuxWitnessInterruptedEvidence
		}
		reducer.retry = nil
	}
	switch {
	case result < 0:
		file.failed = true
		if reducer.step < 2 {
			return errors.New("linux collector mandatory read failed")
		}
	case result == 0:
		file.eof = true
	default:
		file.bytes += result
	}
	return reducer.record(entry.TID, file.path, "read-result="+strconv.FormatInt(result, 10))
}

func (reducer *linuxCollectorReducer) closeResult(tid, descriptor int, result int64) error {
	file, found := reducer.files[descriptor]
	if !found {
		return nil
	}
	if result != 0 || !file.eof && !file.failed {
		return errors.New("linux collector closed incomplete evidence")
	}
	if reducer.step < 2 && (file.bytes == 0 || file.failed) {
		return errors.New("linux collector mandatory evidence has no bytes")
	}
	delete(reducer.files, descriptor)
	return reducer.complete(tid, file.path, file.bytes, file.failed)
}

func (reducer *linuxCollectorReducer) complete(tid int, path string, bytes int64, failed bool) error {
	if err := reducer.requirePath(path); err != nil {
		return err
	}
	if filepath.Base(path) == "memory.pressure" && (bytes == 0 || failed) {
		reducer.steps = slices.Insert(reducer.steps, reducer.step+1, "/proc/pressure/memory")
	}
	if err := reducer.record(tid, path, "complete"); err != nil {
		return err
	}
	reducer.step++
	if reducer.step == len(reducer.steps) {
		reducer.trace.Active = 0
	}
	return nil
}

func (reducer *linuxCollectorReducer) record(tid int, path, phase string) error {
	if len(reducer.trace.Events) >= 256 {
		return errors.New("linux witness event retention exceeded bounds")
	}
	reducer.trace.Events = append(reducer.trace.Events, PublicTUIHostProbeEvent{Sequence: len(reducer.trace.Events) + 1, Reading: reducer.trace.Readings, Operation: path, Phase: phase, PID: tid})
	return nil
}

func (reducer *linuxCollectorReducer) finish() (PublicTUIHostProbeTrace, error) {
	if reducer.retry != nil {
		return reducer.trace, errLinuxWitnessInterruptedEvidence
	}
	if reducer.trace.Readings != 2 || reducer.trace.Active != 0 || reducer.trace.MaximumActive != 1 || len(reducer.files) != 0 || len(reducer.pending) != 0 {
		return reducer.trace, errors.New("linux collector witness lacks two complete serialized readings")
	}
	reducer.trace.Closed = true
	return reducer.trace, nil
}
