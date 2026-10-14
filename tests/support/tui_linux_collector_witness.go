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
	"syscall"
)

type linuxWitnessOperation uint8

const (
	linuxWitnessInitialSetOptions linuxWitnessOperation = iota + 1
	linuxWitnessCloneEventMessage
	linuxWitnessSyscallInfoKernel
	linuxWitnessSyscallInfoValidation
	linuxWitnessCloneFlagsKernel
	linuxWitnessPathnameKernel
)

func (operation linuxWitnessOperation) String() string {
	switch operation {
	case linuxWitnessInitialSetOptions:
		return "initial-ptrace-set-options"
	case linuxWitnessCloneEventMessage:
		return "clone-get-event-message"
	case linuxWitnessSyscallInfoKernel:
		return "get-syscall-info-kernel"
	case linuxWitnessSyscallInfoValidation:
		return "get-syscall-info-validation"
	case linuxWitnessCloneFlagsKernel:
		return "peek-clone-flags-kernel"
	case linuxWitnessPathnameKernel:
		return "peek-pathname-kernel"
	default:
		return "unknown-operation"
	}
}

type linuxWitnessOperationError struct {
	operation linuxWitnessOperation
	tid       int
	original  error
}

func (failure *linuxWitnessOperationError) Error() string {
	return fmt.Sprintf("linux witness operation=%s tid=%d: %v", failure.operation, failure.tid, failure.original)
}

func (failure *linuxWitnessOperationError) Unwrap() error { return failure.original }

func linuxWitnessTagOperation(operation linuxWitnessOperation, tid int, original error) error {
	if original == nil {
		return nil
	}
	return &linuxWitnessOperationError{operation: operation, tid: tid, original: original}
}

func linuxWitnessOperationFailureError(original error, metadata linuxWitnessFailureMetadata) error {
	var failure *linuxWitnessOperationError
	if !errors.As(original, &failure) || failure == nil {
		return original
	}
	metadata.TID = failure.tid
	return fmt.Errorf("%w; witness=%+v", original, metadata)
}

// Resume diagnostics contain only copied scalar observations, never subject data.
type linuxWitnessStopMetadata struct {
	Status      uint32
	Signal      int
	Event       int
	Kind        uint8
	Phase       uint8
	NumberKnown bool
	Number      uint64
}

type linuxWitnessResumeMetadata struct {
	Known  bool
	TID    int
	Signal int
	Stop   linuxWitnessStopMetadata
}

type linuxWitnessFailureMetadata struct {
	Leader         int
	TID            int
	Signal         int
	Stop           linuxWitnessStopMetadata
	Last           linuxWitnessResumeMetadata
	RegisteredHeld bool
	Owned          int
	Held           int
	Detached       int
	Readings       int
	Active         int
	MaximumActive  int
	Closed         bool
	Step           int
	Steps          int
	Events         int
	Pending        int
	Files          int
	PreludePending int
	PreludeFiles   int
	Retry          bool
	RetryTID       int
	Context        linuxWitnessContextMetadata
}

type linuxWitnessContextMetadata uint8

func (state linuxWitnessContextMetadata) String() string {
	switch state {
	case 0:
		return "nil"
	case 1:
		return "canceled"
	case 2:
		return "deadline-exceeded"
	default:
		return "unexpected-context-error"
	}
}

func linuxWitnessContextObservation(err error) linuxWitnessContextMetadata {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, context.Canceled):
		return 1
	case errors.Is(err, context.DeadlineExceeded):
		return 2
	default:
		return 3
	}
}

type linuxWitnessObservation struct {
	held        linuxWitnessStopMetadata
	entryKnown  bool
	entryNumber uint64
}

func (observation *linuxWitnessObservation) stop(status uint32, signal, event int, kind uint8) {
	observation.held = linuxWitnessStopMetadata{Status: status, Signal: signal, Event: event, Kind: kind}
}

func (observation *linuxWitnessObservation) syscall(phase uint8, value uint64) {
	if phase == 1 {
		observation.entryKnown, observation.entryNumber = true, value
	}

	observation.held.Phase = phase
	observation.held.NumberKnown = observation.entryKnown
	if observation.entryKnown {
		observation.held.Number = observation.entryNumber
	}
	// EXIT's value is a result. Only a previously observed ENTRY supplies its number.
	if phase == 2 {
		observation.entryKnown, observation.entryNumber = false, 0
	}
}

func linuxWitnessSuccessfulResume(tid, signal int, stop linuxWitnessStopMetadata) linuxWitnessResumeMetadata {
	return linuxWitnessResumeMetadata{Known: true, TID: tid, Signal: signal, Stop: stop}
}

func (reducer *linuxCollectorReducer) failureMetadata(metadata linuxWitnessFailureMetadata) linuxWitnessFailureMetadata {
	metadata.Readings, metadata.Active = reducer.trace.Readings, reducer.trace.Active
	metadata.MaximumActive, metadata.Closed = reducer.trace.MaximumActive, reducer.trace.Closed
	metadata.Step, metadata.Steps = reducer.step, len(reducer.steps)
	metadata.Events, metadata.Pending, metadata.Files = len(reducer.trace.Events), len(reducer.pending), len(reducer.files)
	metadata.PreludePending, metadata.PreludeFiles = len(reducer.prelude.pending), len(reducer.prelude.files)
	metadata.Retry, metadata.RetryTID = false, 0
	if reducer.retry != nil {
		metadata.Retry, metadata.RetryTID = true, reducer.retry.activeTID
	}
	return metadata
}

func linuxWitnessResumeError(tid int, original error, metadata linuxWitnessFailureMetadata) error {
	return fmt.Errorf("resume owned linux thread %d: %w; witness=%+v", tid, original, metadata)
}

type linuxStopResult struct {
	event    linuxWitnessEvent
	observed bool
}

func linuxWitnessTrapEvent(signal, trapSignal, cause int) int {
	// Go's Linux WaitStatus.TrapCause is an event only for plain SIGTRAP;
	// other stops return -1, including TRACESYSGOOD syscall stops.
	if signal == trapSignal {
		return cause
	}
	return 0
}

type linuxWitnessHarvest struct {
	result              linuxStopResult
	acquired, uncertain bool
}

// linuxWitnessTerminalRetire reports whether a terminated thread retires as
// detached without failure evidence. Only a non-leader thread that already
// exited zero before its first traced stop qualifies: it never entered
// tracing, so there is nothing to detach. Every other terminal state,
// including the leader's, stays fail-closed to protect the natural receipt.
// The harvester can only observe statuses its wait flags match; plain
// children never reach this decision, so cover it directly.
func linuxWitnessTerminalRetire(leader bool, status syscall.WaitStatus) bool {
	return !leader && status.Exited() && status.ExitStatus() == 0
}

// linuxWitnessVanishedDecision reports whether a thread that vanished
// between its stop report and a follow-up kernel query retires as detached.
// Only a reaped clean non-leader exit qualifies; every wait failure, every
// unreaped thread and every other terminal state stays fail-closed. Any wait
// error, including an interrupted wait, counts as unobserved.
func linuxWitnessVanishedDecision(waitErr error, wpid int, status syscall.WaitStatus, leader bool) bool {
	if waitErr != nil || wpid == 0 {
		return false
	}
	return linuxWitnessTerminalRetire(leader, status)
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
	linuxGetSyscallInfo    = 0x420e
	linuxTraceExitKill     = 0x100000
	linuxAuditAMD64        = 0xc000003e
	linuxAuditARM64        = 0xc00000b7
	linuxWitnessOpen       = "open"
	linuxWitnessRead       = "read"
	linuxWitnessClose      = "close"
	linuxWitnessStatFS     = "statfs"
	linuxWitnessMeminfo    = "/proc/meminfo"
	linuxWitnessProcCgroup = "/proc/self/cgroup"
	linuxWitnessCPUPeriod  = "cpu.cfs_period_us"
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
	prelude      linuxWitnessPrelude
}

// Go initializes CPU cgroup discovery before applying GOMAXPROCS. These
// bounded startup descriptors are occupancy, never collector evidence.
type linuxWitnessPrelude struct {
	files                   map[int]string
	pending                 map[int]bool
	procAttempt, procClosed bool
	procFailed              bool
	directory, family       string
	cpuAttempts             int
}

func (prelude *linuxWitnessPrelude) entry(event linuxWitnessEvent, readings int) (bool, error) {
	if event.Operation == linuxWitnessRead || event.Operation == linuxWitnessClose {
		_, found := prelude.files[event.FD]
		return found, nil
	}
	if event.Operation != linuxWitnessOpen || readings != 0 {
		return false, nil
	}
	if event.Path == linuxWitnessProcCgroup {
		if prelude.procAttempt {
			return false, errors.New("linux runtime prelude repeated cgroup discovery")
		}
		prelude.procAttempt = true
		return true, nil
	}
	if !linuxWitnessCPUPreludePath(event.Path) {
		return false, nil
	}
	return true, prelude.cpuOpen(event.Path)
}

func linuxWitnessCPUPreludePath(path string) bool {
	directory := filepath.Dir(path)
	return filepath.Clean(path) == path && (directory == "/sys/fs/cgroup" || strings.HasPrefix(directory, "/sys/fs/cgroup/")) &&
		slices.Contains([]string{"cpu.max", "cpu.cfs_quota_us", linuxWitnessCPUPeriod}, filepath.Base(path))
}

func (prelude *linuxWitnessPrelude) cpuOpen(path string) error {
	if !prelude.procClosed || prelude.procFailed {
		return errors.New("linux runtime CPU prelude lacks completed cgroup discovery")
	}
	directory, name := filepath.Dir(path), filepath.Base(path)
	if prelude.directory != "" && prelude.directory != directory {
		return errors.New("linux runtime CPU prelude changed directory")
	}
	if prelude.cpuAttempts == 0 {
		if name == linuxWitnessCPUPeriod {
			return errors.New("linux runtime CPU prelude reversed quota and period")
		}
		prelude.family, prelude.directory = name, directory
	} else {
		quotaOpen := prelude.hasFile(filepath.Join(directory, "cpu.cfs_quota_us"))
		if prelude.cpuAttempts != 1 || prelude.family != "cpu.cfs_quota_us" || name != linuxWitnessCPUPeriod || !quotaOpen {
			return errors.New("linux runtime CPU prelude repeated or mixed limit attempts")
		}
	}
	prelude.cpuAttempts++
	return nil
}

func (prelude *linuxWitnessPrelude) begin() error {
	if len(prelude.pending) != 0 || prelude.hasFile(linuxWitnessProcCgroup) {
		return errors.New("linux runtime prelude remains pending at collector begin")
	}
	return nil
}

func (prelude *linuxWitnessPrelude) hasFile(path string) bool {
	for _, openPath := range prelude.files {
		if openPath == path {
			return true
		}
	}
	return false
}

func (prelude *linuxWitnessPrelude) result(entry linuxWitnessEvent, result int64) error {
	if linuxWitnessInterrupted(result) {
		return errLinuxWitnessInterruptedEvidence
	}
	switch entry.Operation {
	case linuxWitnessOpen:
		if result >= 0 {
			prelude.files[int(result)] = entry.Path
		} else if entry.Path == linuxWitnessProcCgroup {
			prelude.procFailed = true
		}
	case linuxWitnessRead:
		if result < 0 && prelude.files[entry.FD] == linuxWitnessProcCgroup {
			prelude.procFailed = true
		}
	case linuxWitnessClose:
		if result != 0 {
			return errors.New("linux runtime prelude close is unproven")
		}
		if prelude.files[entry.FD] == linuxWitnessProcCgroup {
			prelude.procClosed = true
		}
		delete(prelude.files, entry.FD)
	}
	return nil
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
	return &linuxCollectorReducer{threads: map[int]bool{leader: true}, pending: map[int]linuxWitnessEvent{}, files: map[int]*linuxWitnessFile{}, disk: disk, prelude: linuxWitnessPrelude{files: map[int]string{}, pending: map[int]bool{}}}
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
	if reducer.trace.Readings == 1 {
		if err := reducer.prelude.begin(); err != nil {
			return err
		}
	}
	reducer.trace.Active = 1
	reducer.step, reducer.cgroup = 0, ""
	reducer.steps = []string{linuxWitnessMeminfo, "/proc/stat", linuxWitnessProcCgroup, "memory.max", "memory.high", "memory.current", "memory.stat", "cpu.max", "memory.pressure", "memory.events", "memory.swap.max", "memory.swap.current", linuxWitnessStatFS, "/proc/vmstat"}
	return nil
}

func (reducer *linuxCollectorReducer) accept(event linuxWitnessEvent) error {
	if !reducer.threads[event.TID] {
		return errors.New("linux witness received foreign thread")
	}
	if event.Entry {
		return reducer.acceptEntry(event)
	}
	entry, found := reducer.pending[event.TID]
	if !found || entry.Operation != event.Operation {
		return errors.New("linux witness received unpaired syscall exit")
	}
	delete(reducer.pending, event.TID)
	if entry.Operation == linuxWitnessOpen && event.Result >= 0 {
		if err := reducer.validateOpenDescriptor(event.Result); err != nil {
			return err
		}
	}
	if reducer.prelude.pending[event.TID] {
		delete(reducer.prelude.pending, event.TID)
		return reducer.prelude.result(entry, event.Result)
	}
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

func (reducer *linuxCollectorReducer) acceptEntry(event linuxWitnessEvent) error {
	if _, found := reducer.pending[event.TID]; found {
		return errors.New("linux witness duplicated a syscall entry")
	}
	prelude, err := reducer.prelude.entry(event, reducer.trace.Readings)
	if err != nil {
		return err
	}
	if err := reducer.validateAcceptedEntry(event, prelude); err != nil {
		return err
	}
	if prelude {
		reducer.prelude.pending[event.TID] = true
	}
	reducer.pending[event.TID] = event
	return nil
}

func (reducer *linuxCollectorReducer) validateAcceptedEntry(event linuxWitnessEvent, prelude bool) error {
	if prelude {
		return reducer.validateRetryEntry(event)
	}
	return reducer.validateEntry(event)
}

func (reducer *linuxCollectorReducer) validateOpenDescriptor(result int64) error {
	if result > 1<<20 {
		return errors.New("linux witness descriptor exceeds bounds")
	}
	_, collector := reducer.files[int(result)]
	_, background := reducer.prelude.files[int(result)]
	if collector || background {
		return errors.New("linux witness descriptor reused before close")
	}
	return nil
}

// Linux v6.8 include/linux/errno.h reserves the restart results below. Go's
// os.Open/internal/poll.Read retry EINTR, but open pointers can be reallocated.
func linuxWitnessInterrupted(result int64) bool {
	return result == -4 || result == -512 || result == -513 || result == -514 || result == -516
}

func linuxWitnessPath(path string) bool {
	return slices.Contains([]string{linuxWitnessMeminfo, "/proc/stat", linuxWitnessProcCgroup, "/proc/pressure/memory", "/proc/vmstat"}, path) || strings.HasPrefix(path, "/sys/fs/cgroup/")
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
