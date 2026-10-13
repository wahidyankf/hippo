//go:build linux

package support

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// These bidirectional array bounds make every Linux amd64/arm64 compile prove
// the concrete object's UAPI layout, not merely compare caller-supplied sizes.
// The fixed union uses consecutive eight-byte words; its exit flag is byte 32.
const (
	linuxInfoSize         = unsafe.Sizeof(linuxSyscallInfo{})
	linuxInfoOperation    = unsafe.Offsetof(linuxSyscallInfo{}.Operation)
	linuxInfoArchitecture = unsafe.Offsetof(linuxSyscallInfo{}.Architecture)
	linuxInfoInstruction  = unsafe.Offsetof(linuxSyscallInfo{}.Instruction)
	linuxInfoStack        = unsafe.Offsetof(linuxSyscallInfo{}.Stack)
	linuxInfoUnion        = unsafe.Offsetof(linuxSyscallInfo{}.Data)
	linuxInfoArguments    = linuxInfoUnion + unsafe.Sizeof(uint64(0))
	linuxInfoEntryEnd     = linuxInfoArguments + 6*unsafe.Sizeof(uint64(0))
)

var (
	_ [88 - linuxInfoSize]byte
	_ [linuxInfoSize - 88]byte
	_ [0 - linuxInfoOperation]byte
	_ [linuxInfoOperation - 0]byte
	_ [4 - linuxInfoArchitecture]byte
	_ [linuxInfoArchitecture - 4]byte
	_ [8 - linuxInfoInstruction]byte
	_ [linuxInfoInstruction - 8]byte
	_ [16 - linuxInfoStack]byte
	_ [linuxInfoStack - 16]byte
	_ [24 - linuxInfoUnion]byte
	_ [linuxInfoUnion - 24]byte
	_ [32 - linuxInfoArguments]byte
	_ [linuxInfoArguments - 32]byte
	_ [80 - linuxInfoEntryEnd]byte
	_ [linuxInfoEntryEnd - 80]byte
)

var errLinuxWitnessUnexpectedExecEntry = errors.New("linux witness refused unexpected child exec entry")

type linuxWitnessThread struct {
	observation     linuxWitnessObservation
	call            *linuxWitnessEvent
	paired, initial bool
	cloneAllowed    bool
}

func linuxWitnessObserved(event *linuxWitnessEvent) linuxStopResult {
	if event == nil {
		return linuxStopResult{}
	}
	return linuxStopResult{event: *event, observed: true}
}

type linuxCollectorObserver struct {
	command    *exec.Cmd
	reducer    *linuxCollectorReducer
	threads    map[int]*linuxWitnessThread
	stopped    map[int]int
	leader     int
	detached   int
	lastResume linuxWitnessResumeMetadata
}

// RunPublicLinuxCollectorWitness observes real procfs syscalls of the exact selected
// old status child. It neither replaces the executable nor changes its host reads.
// TRACEEXIT detach hands natural terminal reaping back to that same exec.Cmd.Wait.
func RunPublicLinuxCollectorWitness(ctx context.Context, command *exec.Cmd, disk string) (PublicTUIHostProbeTrace, error) {
	if ctx == nil || command == nil || command.Process != nil || command.SysProcAttr != nil || command.WaitDelay != 3*time.Second {
		return PublicTUIHostProbeTrace{}, errors.New("linux collector witness requires an unstarted owned bounded command")
	}
	if err := ctx.Err(); err != nil {
		return PublicTUIHostProbeTrace{}, err
	}
	deadline, finite := ctx.Deadline()
	if !finite || time.Until(deadline) > 8*time.Second || command.Cancel == nil {
		return PublicTUIHostProbeTrace{}, errors.New("linux collector witness requires the original eight-second command context")
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return PublicTUIHostProbeTrace{}, errors.New("linux collector witness architecture is unsupported")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	command.SysProcAttr = &syscall.SysProcAttr{Ptrace: true}
	if err := command.Start(); err != nil {
		return PublicTUIHostProbeTrace{}, err
	}
	leader := command.Process.Pid
	observer := &linuxCollectorObserver{command: command, leader: leader, reducer: newLinuxCollectorReducer(leader, disk), threads: map[int]*linuxWitnessThread{leader: {initial: true}}, stopped: map[int]int{}}
	observeError := observer.observe(ctx)
	if observeError != nil {
		return observer.reducer.trace, errors.Join(observeError, observer.retire(ctx))
	}
	waitError := command.Wait()
	trace, traceError := observer.reducer.finish()
	return trace, errors.Join(waitError, traceError)
}

func (observer *linuxCollectorObserver) observe(ctx context.Context) error {
	initial := true
	for len(observer.threads) != 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		events, err := observer.drain(ctx, initial)
		if err != nil {
			var failure *linuxWitnessOperationError
			if errors.As(err, &failure) && failure != nil {
				contextError := ctx.Err()
				metadata := observer.resumeFailureMetadata(failure.tid, contextError)
				return linuxWitnessOperationFailureError(err, metadata)
			}
			return err
		}
		if err := observer.reducer.batch(events); err != nil {
			return err
		}
		if len(observer.stopped) == 0 {
			if err := linuxWitnessPause(ctx); err != nil {
				return err
			}
			continue
		}
		initial = false
		for _, tid := range observer.sortedStopped() {
			if err := syscall.PtraceSyscall(tid, observer.stopped[tid]); err != nil {
				metadata := observer.resumeFailureMetadata(tid, ctx.Err())
				return linuxWitnessResumeError(tid, err, metadata)
			}
			observer.lastResume = linuxWitnessSuccessfulResume(tid, observer.stopped[tid], observer.heldStopMetadata(tid))
			delete(observer.stopped, tid)
		}
	}
	return nil
}

func (observer *linuxCollectorObserver) heldStopMetadata(tid int) linuxWitnessStopMetadata {
	if thread := observer.threads[tid]; thread != nil {
		return thread.observation.held
	}
	return linuxWitnessStopMetadata{}
}

func (observer *linuxCollectorObserver) resumeFailureMetadata(tid int, contextError error) linuxWitnessFailureMetadata {
	metadata := linuxWitnessFailureMetadata{
		Leader: observer.leader, TID: tid, Signal: observer.stopped[tid], Last: observer.lastResume, Stop: observer.heldStopMetadata(tid),
		RegisteredHeld: observer.linuxWitnessHeld(tid), Owned: len(observer.threads), Held: len(observer.stopped), Detached: observer.detached,
		Context: linuxWitnessContextObservation(contextError),
	}
	return observer.reducer.failureMetadata(metadata)
}

func linuxWitnessPause(ctx context.Context) error {
	timer := time.NewTimer(time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (observer *linuxCollectorObserver) sortedStopped() []int {
	result := make([]int, 0, len(observer.stopped))
	for tid := range observer.stopped {
		result = append(result, tid)
	}
	slices.Sort(result)
	return result
}

// drain never waits on an unrelated child or resumes a held stop before batch.
func (observer *linuxCollectorObserver) drain(ctx context.Context, initial bool) ([]linuxWitnessEvent, error) {
	return linuxWitnessDrainFrontier(ctx, observer, initial)
}

func (observer *linuxCollectorObserver) linuxWitnessTIDs() []int {
	result := make([]int, 0, len(observer.threads))
	for tid := range observer.threads {
		result = append(result, tid)
	}
	slices.Sort(result)
	return result
}

func (observer *linuxCollectorObserver) linuxWitnessHeld(tid int) bool {
	_, held := observer.stopped[tid]
	return held
}

func (observer *linuxCollectorObserver) linuxWitnessHarvest(tid int, initial bool) (linuxWitnessHarvest, error) {
	var status syscall.WaitStatus
	pid, err := syscall.Wait4(tid, &status, syscall.WNOHANG|syscall.WALL, nil)
	if errors.Is(err, syscall.EINTR) {
		return linuxWitnessHarvest{uncertain: true}, nil
	}
	if err != nil {
		return linuxWitnessHarvest{}, fmt.Errorf("wait owned linux thread %d: %w", tid, err)
	}
	if pid == 0 {
		return linuxWitnessHarvest{}, nil
	}
	if !status.Stopped() {
		if tid != observer.leader && status.Exited() && status.ExitStatus() == 0 {
			// A clone thread that already exited zero before its first
			// traced stop never entered tracing: retire it as detached
			// instead of failing the whole collection. The leader keeps
			// fail-closed semantics so a missing natural receipt stays loud.
			delete(observer.threads, tid)
			observer.detached++
			return linuxWitnessHarvest{}, nil
		}
		return linuxWitnessHarvest{}, fmt.Errorf("owned linux thread %d terminated before detach handoff: status=%d", tid, status)
	}
	result, err := observer.stop(tid, status, initial)
	return linuxWitnessHarvest{result: result, acquired: true}, err
}

func (observer *linuxCollectorObserver) stop(tid int, status syscall.WaitStatus, initial bool) (linuxStopResult, error) {
	thread := observer.threads[tid]
	if thread == nil {
		return linuxStopResult{}, fmt.Errorf("linux witness lacks owned state for thread %d", tid)
	}
	signal := status.StopSignal()
	event := linuxWitnessTrapEvent(int(signal), int(syscall.SIGTRAP), status.TrapCause())
	thread.observation.stop(uint32(status), int(signal), event, 0)
	if initial && tid == observer.leader {
		if signal != syscall.SIGTRAP || event != 0 {
			return linuxStopResult{}, errors.New("linux witness child lacks initial exec stop")
		}
		options := syscall.PTRACE_O_TRACESYSGOOD | syscall.PTRACE_O_TRACECLONE | syscall.PTRACE_O_TRACEEXIT | syscall.PTRACE_O_TRACEEXEC | syscall.PTRACE_O_TRACEFORK | syscall.PTRACE_O_TRACEVFORK | linuxTraceExitKill
		if err := syscall.PtraceSetOptions(tid, options); err != nil {
			return linuxStopResult{}, linuxWitnessTagOperation(linuxWitnessInitialSetOptions, tid, err)
		}
		thread.observation.held.Kind = 1
		observer.stopped[tid] = 0
		return linuxStopResult{}, nil
	}
	if event == syscall.PTRACE_EVENT_EXIT {
		thread.observation.held.Kind = 2
		if err := syscall.PtraceDetach(tid); err != nil {
			return linuxStopResult{}, fmt.Errorf("detach exiting owned thread %d: %w", tid, err)
		}
		delete(observer.threads, tid)
		observer.detached++
		return linuxStopResult{}, nil
	}
	if event == syscall.PTRACE_EVENT_CLONE {
		thread.observation.held.Kind = 3
		if !thread.cloneAllowed {
			return linuxStopResult{}, errors.New("linux witness observed an unqualified clone")
		}
		child, err := syscall.PtraceGetEventMsg(tid)
		if err != nil {
			return linuxStopResult{}, linuxWitnessTagOperation(linuxWitnessCloneEventMessage, tid, err)
		}
		if child == 0 || child > math.MaxInt32 {
			return linuxStopResult{}, errors.New("linux witness clone id exceeds bounds")
		}
		if err := observer.reducer.addThread(tid, int(child)); err != nil {
			return linuxStopResult{}, err
		}
		observer.threads[int(child)] = &linuxWitnessThread{initial: true}
		observer.stopped[tid] = 0
		return linuxStopResult{}, nil
	}
	if event != 0 {
		return linuxStopResult{}, errors.New("linux witness refuses unexpected exec or fork event")
	}
	if signal == syscall.Signal(int(syscall.SIGTRAP)|0x80) {
		info, err := linuxWitnessSyscallInfo(tid)
		if err != nil {
			return linuxStopResult{}, err
		}
		thread.observation.held.Kind = 4
		thread.observation.syscall(info.Operation, info.Data[0])
		observer.stopped[tid] = 0
		return observer.syscall(tid, info)
	}
	if signal == syscall.SIGSTOP && thread.initial {
		thread.observation.held.Kind = 5
		observer.stopped[tid] = 0
		return linuxStopResult{}, nil
	}
	if signal == syscall.SIGTRAP || signal == syscall.SIGSTOP {
		return linuxStopResult{}, errors.New("linux witness received unexplained tracing stop")
	}
	thread.observation.held.Kind = 6
	observer.stopped[tid] = int(signal)
	return linuxStopResult{}, nil
}

func linuxWitnessSyscallInfo(tid int) (linuxSyscallInfo, error) {
	var info linuxSyscallInfo
	// Safety: the kernel writes at most the supplied size into this live, fixed
	// UAPI-shaped object; no subject read buffer is dereferenced or retained.
	// #nosec G103 -- GET_SYSCALL_INFO requires the live 88-byte typed ABI buffer; direct argument conversion and
	// KeepAlive retain it through the syscall; errno, native architecture and entry/exit lengths are validated.
	length, _, errno := syscall.RawSyscall6(syscall.SYS_PTRACE, linuxGetSyscallInfo, uintptr(tid), unsafe.Sizeof(info), uintptr(unsafe.Pointer(&info)), 0, 0)
	runtime.KeepAlive(&info)
	if errno != 0 {
		return info, linuxWitnessTagOperation(linuxWitnessSyscallInfoKernel, tid, errno)
	}
	expectedArchitecture := uint32(linuxAuditAMD64)
	if runtime.GOARCH == "arm64" {
		expectedArchitecture = linuxAuditARM64
	}
	if err := info.validate(length, unsafe.Sizeof(info), expectedArchitecture); err != nil {
		return info, linuxWitnessTagOperation(linuxWitnessSyscallInfoValidation, tid, err)
	}
	return info, nil
}

func (observer *linuxCollectorObserver) syscall(tid int, info linuxSyscallInfo) (linuxStopResult, error) {
	thread := observer.threads[tid]
	if thread == nil {
		return linuxStopResult{}, fmt.Errorf("linux witness lacks owned state for thread %d", tid)
	}
	if info.Operation == 1 {
		if thread.paired {
			return linuxStopResult{}, errors.New("linux witness duplicated syscall entry")
		}
		call, clone, err := linuxWitnessEntry(tid, info)
		if err != nil {
			return linuxStopResult{}, err
		}
		thread.call, thread.paired, thread.cloneAllowed, thread.initial = call, true, clone, false
		return linuxWitnessObserved(call), nil
	}
	if !thread.paired {
		if thread.initial {
			thread.initial = false
			return linuxStopResult{}, nil
		}
		return linuxStopResult{}, errors.New("linux witness received unpaired kernel syscall exit")
	}
	thread.paired = false
	if thread.call == nil {
		return linuxStopResult{}, nil
	}
	result, err := linuxWitnessSigned(info.Data[0])
	if err != nil {
		return linuxStopResult{}, err
	}
	return linuxStopResult{observed: true, event: linuxWitnessEvent{TID: tid, Operation: thread.call.Operation, Result: result}}, nil
}

func linuxWitnessEntry(tid int, info linuxSyscallInfo) (*linuxWitnessEvent, bool, error) {
	number, args := info.Data[0], info.Data[1:7]
	// Missing frozen-stdlib numbers are anchored by this module's pinned x/sys
	// v0.48.0 unix/zsysnum_linux_{amd64,arm64}.go; no register/parity inference.
	open, execveat := uint64(2), uint64(322)
	if info.Architecture == linuxAuditARM64 {
		open, execveat = math.MaxUint64, 281
	}
	openat, read, closeNumber, statfs, clone, execve := uint64(syscall.SYS_OPENAT), uint64(syscall.SYS_READ), uint64(syscall.SYS_CLOSE), uint64(syscall.SYS_STATFS), uint64(syscall.SYS_CLONE), uint64(syscall.SYS_EXECVE)
	if number == execve || number == execveat {
		return nil, false, errLinuxWitnessUnexpectedExecEntry
	}
	if info.Architecture == linuxAuditAMD64 && (number == 57 || number == 58) {
		return nil, false, errors.New("linux witness refuses unexpected child fork")
	}
	if number == clone {
		if args[0]&syscall.CLONE_THREAD == 0 || args[0]&syscall.CLONE_FILES == 0 {
			return nil, false, errors.New("linux witness refuses clone outside the shared thread group")
		}
		return nil, true, nil
	}
	if number == 435 {
		flags, err := linuxWitnessCloneFlags(tid, args[0], args[1])
		if err != nil {
			return nil, false, err
		}
		if flags&syscall.CLONE_THREAD == 0 || flags&syscall.CLONE_FILES == 0 {
			return nil, false, errors.New("linux witness refuses clone3 outside the shared thread group")
		}
		return nil, true, nil
	}
	call := &linuxWitnessEvent{TID: tid, Entry: true}
	switch number {
	case open, openat:
		pointer := args[0]
		if number == openat {
			pointer = args[1]
		}
		path, err := linuxWitnessPathname(tid, pointer)
		if err != nil {
			return nil, false, err
		}
		call.Operation, call.Path = linuxWitnessOpen, path
		if !linuxWitnessPath(path) {
			// Unrelated config/runtime filenames never enter retained witness metadata.
			call.Path = ""
		}
	case read, closeNumber:
		descriptor := args[0]
		if descriptor > 1<<20 {
			return nil, false, errors.New("linux syscall descriptor exceeds bounds")
		}
		call.FD, call.Operation = int(descriptor), linuxWitnessRead
		if number == closeNumber {
			call.Operation = linuxWitnessClose
		} else {
			call.ReadIdentity = linuxWitnessReadIdentity{Number: number, Buffer: args[1], Count: args[2], Observed: true}
		}
	case statfs:
		path, err := linuxWitnessPathname(tid, args[0])
		if err != nil {
			return nil, false, err
		}
		call.Operation, call.Path = linuxWitnessStatFS, path
	default:
		return nil, false, nil
	}
	return call, false, nil
}

func linuxWitnessCloneFlags(tid int, pointer, size uint64) (uint64, error) {
	if size < 8 || pointer == 0 {
		return 0, errors.New("linux witness clone ABI is truncated")
	}
	var data [8]byte
	count, err := syscall.PtracePeekData(tid, uintptr(pointer), data[:])
	if err != nil {
		return 0, linuxWitnessTagOperation(linuxWitnessCloneFlagsKernel, tid, err)
	}
	if count != len(data) {
		return 0, errors.New("linux witness clone flags are truncated")
	}
	var flags uint64
	for index, value := range data {
		flags |= uint64(value) << (index * 8)
	}
	return flags, nil
}

func linuxWitnessPathname(tid int, pointer uint64) (string, error) {
	if pointer == 0 {
		return "", errors.New("linux witness syscall path is null")
	}
	var result strings.Builder
	for offset := uint64(0); offset < 4096; offset += 8 {
		var data [8]byte
		count, err := syscall.PtracePeekData(tid, uintptr(pointer+offset), data[:])
		if err != nil {
			return "", linuxWitnessTagOperation(linuxWitnessPathnameKernel, tid, err)
		}
		if count != len(data) {
			return "", errors.New("linux witness syscall pathname is truncated")
		}
		for _, value := range data {
			if value == 0 {
				return result.String(), nil
			}
			if err := result.WriteByte(value); err != nil {
				return "", err
			}
		}
	}
	return "", errors.New("linux witness syscall pathname exceeds bounds")
}

func (observer *linuxCollectorObserver) retire(ctx context.Context) error {
	killError := observer.command.Process.Kill()
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), observer.command.WaitDelay)
	defer cancel()
	var retireError error
	for len(observer.threads) != 0 && cleanup.Err() == nil {
		for tid := range observer.threads {
			if _, stopped := observer.stopped[tid]; stopped {
				retireError = errors.Join(retireError, syscall.PtraceCont(tid, int(syscall.SIGKILL)))
				delete(observer.stopped, tid)
				continue
			}
			var status syscall.WaitStatus
			pid, err := syscall.Wait4(tid, &status, syscall.WNOHANG|syscall.WALL, nil)
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			if err != nil {
				retireError = errors.Join(retireError, err)
				delete(observer.threads, tid)
				continue
			}
			if pid == 0 {
				continue
			}
			if !status.Stopped() {
				// This is failed trace evidence, never a natural Cmd.Wait receipt.
				retireError = errors.Join(retireError, fmt.Errorf("owned thread %d terminal consumed during failed trace cleanup", tid))
				delete(observer.threads, tid)
				continue
			}
			if status.TrapCause() == syscall.PTRACE_EVENT_EXIT {
				if err := syscall.PtraceDetach(tid); err != nil {
					retireError = errors.Join(retireError, err)
					continue
				}
				delete(observer.threads, tid)
				continue
			}
			retireError = errors.Join(retireError, syscall.PtraceCont(tid, int(syscall.SIGKILL)))
		}
		if err := linuxWitnessPause(cleanup); err != nil {
			retireError = errors.Join(retireError, err)
		}
	}
	if len(observer.threads) != 0 {
		return errors.Join(killError, retireError, fmt.Errorf("linux witness cleanup unproven for %d owned threads; natural Wait unavailable", len(observer.threads)))
	}
	return errors.Join(killError, retireError, observer.command.Wait())
}
