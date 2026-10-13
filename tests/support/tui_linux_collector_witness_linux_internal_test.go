//go:build linux

package support

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/host"
)

const linuxNativeChildMode = "HIPPO_TEST_LINUX_WITNESS_CHILD"

func TestLinuxCollectorWitnessAdapterRejectsMissingThreadState(t *testing.T) {
	for _, test := range []struct {
		name      string
		present   bool
		operation uint8
	}{
		{"absent clone stop", false, 0},
		{"nil clone stop", true, 0},
		{"absent syscall entry", false, 1},
		{"nil syscall entry", true, 1},
		{"absent syscall exit", false, 2},
		{"nil syscall exit", true, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			live := &linuxWitnessThread{paired: true, initial: true, cloneAllowed: true}
			before := *live
			command := &exec.Cmd{}
			reducer := newLinuxCollectorReducer(witnessLeader, "/owned/disk")
			observer := &linuxCollectorObserver{command: command, reducer: reducer, leader: witnessLeader, threads: map[int]*linuxWitnessThread{witnessLeader: live}, stopped: map[int]int{witnessLeader: int(syscall.SIGUSR1)}}
			if test.present {
				observer.threads[22] = nil
			}
			count := len(observer.threads)
			var result linuxStopResult
			var err error
			if test.operation == 0 {
				// A modeled clone stop reaches the missing-state boundary before
				// any ptrace call. This is an adapter control, not kernel evidence.
				status := syscall.WaitStatus(syscall.PTRACE_EVENT_CLONE<<16 | int(syscall.SIGTRAP)<<8 | 0x7f)
				result, err = observer.stop(22, status, false)
			} else {
				result, err = observer.syscall(22, linuxSyscallInfo{Operation: test.operation})
			}
			if err == nil || result.observed || result.event != (linuxWitnessEvent{}) {
				t.Fatal("missing thread state became accepted witness evidence", result, err)
			}
			_, present := observer.threads[22]
			if present != test.present || observer.threads[22] != nil || len(observer.threads) != count || observer.threads[witnessLeader] != live || *live != before {
				t.Fatal("missing-state refusal changed thread ownership or a live syscall pair")
			}
			if len(observer.stopped) != 1 || observer.stopped[witnessLeader] != int(syscall.SIGUSR1) || observer.detached != 0 || observer.command != command || command.Process != nil || observer.reducer != reducer {
				t.Fatal("missing-state refusal changed held stops, detach handoff or command ownership")
			}
			linuxAssertMissingThreadReducerUnchanged(t, reducer)
		})
	}
}

func linuxAssertMissingThreadReducerUnchanged(t *testing.T, reducer *linuxCollectorReducer) {
	t.Helper()
	if len(reducer.trace.Events) != 0 || reducer.trace.Readings != 0 || reducer.trace.Active != 0 || reducer.trace.MaximumActive != 0 || reducer.trace.Closed || reducer.step != 0 || reducer.retry != nil || len(reducer.pending) != 0 || len(reducer.files) != 0 || len(reducer.threads) != 1 || !reducer.threads[witnessLeader] {
		t.Fatal("missing-state refusal advanced or closed collector evidence")
	}
}

type linuxNativeReceipt struct {
	PID      int    `json:"pid"`
	OpenTID  int    `json:"openTID"`
	ReadTID  int    `json:"readTID"`
	Readings int    `json:"readings"`
	Mode     string `json:"mode"`
	Root     string `json:"root"`
}

// This entry is invoked only in the same test binary's owned child. The
// ordinary support-package gate executes every parent control below on Linux.
func TestLinuxCollectorWitnessNativeChild(t *testing.T) {
	mode := os.Getenv(linuxNativeChildMode)
	if mode == "" {
		return
	}
	root := linuxNativeOwnedChildRoot(t)

	switch mode {
	case "entry":
		if os.Getenv("HIPPO_BIN") != "" {
			t.Fatal("trusted helper entry unexpectedly selected a product candidate")
		}
		linuxNativeWriteReceipt(t, linuxNativeReceipt{PID: os.Getpid(), Mode: mode, Root: root})
	case "plain", "migrated", "exec":
		linuxNativeCollectChild(t, mode)
	case "overlap":
		linuxNativeOverlapChild(t)
	case "late-overlap":
		linuxNativeLateOverlapChild(t)
	case "wrong-order":
		collector := host.SystemCollector{ReadFile: func(path string) ([]byte, error) {
			data, err := os.ReadFile(path)
			if err == nil && path == "/proc/meminfo" {
				// A genuine extra read falsifies the reducer's expected CPU step.
				_, err = os.ReadFile("/proc/vmstat")
			}
			return data, err
		}}
		if _, err := collector.Collect(t.Context(), nil, "/"); err != nil {
			t.Fatal(err)
		}
	case "foreign-live":
		linuxNativeWriteReceipt(t, linuxNativeReceipt{PID: os.Getpid()})
		if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
			t.Fatal(err)
		}
	case "foreign-exited":
		linuxNativeWriteReceipt(t, linuxNativeReceipt{PID: os.Getpid()})
	case "exec-target":
		// The parent sets this child's working directory to its owned TempDir.
		// Root confinement and a fixed leaf prohibit a caller-selected write path.
		root, err := os.OpenRoot(".")
		if err != nil {
			t.Fatal(err)
		}
		if err := root.WriteFile("exec-ran", []byte("unexpected exec ran\n"), 0o600); err != nil {
			t.Fatalf("write exec marker: %v; close root: %v", err, root.Close())
		}
		if err := root.Close(); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown native witness child mode %q", mode)
	}
}

type linuxNativeTask struct {
	operation func()
	done      chan struct{}
}

type linuxNativeWorker struct {
	tasks chan linuxNativeTask
	done  chan struct{}
	tid   int
}

func linuxNativeStartWorker() *linuxNativeWorker {
	worker := &linuxNativeWorker{tasks: make(chan linuxNativeTask), done: make(chan struct{})}
	ready := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(worker.done)
		worker.tid = syscall.Gettid()
		close(ready)
		for task := range worker.tasks {
			task.operation()
			close(task.done)
		}
	}()
	<-ready
	return worker
}

func (worker *linuxNativeWorker) run(operation func()) {
	done := make(chan struct{})
	worker.tasks <- linuxNativeTask{operation: operation, done: done}
	<-done
}

func (worker *linuxNativeWorker) stop() {
	close(worker.tasks)
	<-worker.done
}

func linuxNativeMigratingRead(openWorker, readWorker *linuxNativeWorker, path string) ([]byte, error) {
	var file *os.File
	var openError error
	openWorker.run(func() { file, openError = os.Open(path) })
	if openError != nil {
		return nil, openError
	}
	var data []byte
	var readError, closeError error
	readWorker.run(func() { data, readError = io.ReadAll(file) })
	openWorker.run(func() { closeError = file.Close() })
	return data, errors.Join(readError, closeError)
}

func linuxNativeCollectChild(t *testing.T, mode string) {
	t.Helper()
	collector := host.SystemCollector{}
	receipt := linuxNativeReceipt{PID: os.Getpid()}
	if mode == "migrated" {
		openWorker, readWorker := linuxNativeStartWorker(), linuxNativeStartWorker()
		defer openWorker.stop()
		defer readWorker.stop()
		receipt.OpenTID, receipt.ReadTID = openWorker.tid, readWorker.tid
		collector.ReadFile = func(path string) ([]byte, error) {
			return linuxNativeMigratingRead(openWorker, readWorker, path)
		}
	}
	var previous host.CPUState
	for range 2 {
		reading, err := collector.Collect(t.Context(), previous, "/")
		if err != nil {
			t.Fatal(err)
		}
		if reading.Sample.Platform != "linux" || reading.Sample.PhysicalMemoryBytes <= 0 || reading.Sample.DiskTotalBytes == nil || *reading.Sample.DiskTotalBytes <= 0 {
			t.Fatalf("production collector did not parse genuine host evidence: %+v", reading.Sample)
		}
		previous = reading.CPUState
		receipt.Readings++
	}
	linuxNativeWriteReceipt(t, receipt)
	if mode == "exec" {
		linuxNativeValidateSelfExec(t)
		// This kernel-owned link denotes the running test executable itself,
		// without resolving any executable or argument from external input.
		root := linuxNativeOwnedChildRoot(t)
		environment := []string{linuxNativeChildMode + "=exec-target", "GOMAXPROCS=2", isolatedRootVariable + "=" + root, "HIPPO_ROOT=" + root}
		if err := syscall.Exec("/proc/self/exe", []string{"/proc/self/exe", "-test.run=^TestLinuxCollectorWitnessNativeChild$"}, environment); err != nil {
			t.Fatal(err)
		}
	}
}

func linuxNativeOwnedChildRoot(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv(isolatedRootVariable) != root || os.Getenv("HIPPO_ROOT") != root {
		t.Fatal("native child lacks its exact owned isolation root")
	}
	if err := validateOwnedCandidateDirectory(root); err != nil {
		t.Fatalf("native child root ownership: %v", err)
	}
	return root
}

func linuxNativeValidateSelfExec(t *testing.T) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	current, err := os.Stat(executable)
	if err != nil {
		t.Fatal(err)
	}
	kernelTarget, err := os.Stat("/proc/self/exe")
	if err != nil {
		t.Fatal(err)
	}
	if !current.Mode().IsRegular() || !os.SameFile(current, kernelTarget) {
		t.Fatal("kernel self-exec target is not the exact running test executable")
	}
}

func linuxNativeOverlapChild(t *testing.T) {
	t.Helper()
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	results := make(chan error, 2)
	collector := host.SystemCollector{ReadFile: func(path string) ([]byte, error) {
		data, err := os.ReadFile(path)
		if path == "/proc/meminfo" && err == nil {
			ready <- struct{}{}
			<-release
		}
		return data, err
	}}
	for range 2 {
		go func() {
			_, err := collector.Collect(t.Context(), nil, "/")
			results <- err
		}()
	}
	// Both genuine memory reads precede either CPU read. This is an actual
	// collector overlap, even when ptrace changes the runnable-thread schedule.
	<-ready
	<-ready
	close(release)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}

func linuxNativeLateOverlapChild(t *testing.T) {
	t.Helper()
	terminalReady := make(chan struct{})
	releaseTerminal := make(chan struct{})
	firstResult := make(chan error, 1)
	first := host.SystemCollector{ReadFile: func(path string) ([]byte, error) {
		if path != "/proc/vmstat" {
			return os.ReadFile(path)
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		data, readError := io.ReadAll(file)
		if readError != nil {
			return nil, errors.Join(readError, file.Close())
		}
		// Actual vmstat EOF precedes this notification. The terminal FD stays
		// open until the second production collector's memory read returns.
		close(terminalReady)
		<-releaseTerminal
		return data, file.Close()
	}}
	go func() {
		_, err := first.Collect(t.Context(), nil, "/")
		firstResult <- err
	}()
	select {
	case <-terminalReady:
	case err := <-firstResult:
		t.Fatalf("first production collection never reached genuine terminal vmstat EOF: %v", err)
	}
	second := host.SystemCollector{ReadFile: func(path string) ([]byte, error) {
		data, err := os.ReadFile(path)
		if path == "/proc/meminfo" {
			close(releaseTerminal)
		}
		return data, err
	}}
	_, secondError := second.Collect(t.Context(), nil, "/")
	if err := errors.Join(secondError, <-firstResult); err != nil {
		t.Fatal(err)
	}
}

func linuxNativeWriteReceipt(t *testing.T, receipt linuxNativeReceipt) {
	t.Helper()
	if err := json.NewEncoder(os.Stdout).Encode(receipt); err != nil {
		t.Fatal(err)
	}
}

func linuxNativeCommand(ctx context.Context, t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, executable, "-test.run=^TestLinuxCollectorWitnessNativeChild$")
	root := linuxNativePrivateRoot(t, nil)
	// This exact private root belongs to this parent test. The established
	// re-exec marker keeps trusted modes through TestMain without another build
	// or environment scrub. Ordinary support-suite setup remains unchanged.
	command.Dir = root
	command.Env = []string{linuxNativeChildMode + "=" + mode, "GOMAXPROCS=2", isolatedRootVariable + "=" + root, "HIPPO_ROOT=" + root}
	command.WaitDelay = 3 * time.Second
	return command
}

func TestLinuxCollectorWitnessNativeOwnedHelperEntry(t *testing.T) {
	for _, marked := range []bool{true, false} {
		t.Run(strconv.FormatBool(marked), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
			defer cancel()
			command := linuxNativeCommand(ctx, t, "entry")
			if !marked {
				linuxNativeRemoveIsolationMarker(command)
			}
			var output, diagnostics bytes.Buffer
			command.Stdout, command.Stderr = &output, &diagnostics
			err := command.Run()
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || ctx.Err() != nil {
				t.Fatalf("helper entry exceeded its actual finite budget: %v; context=%v", err, ctx.Err())
			}
			if !marked {
				var exitError *exec.ExitError
				// No PATH, Go-toolchain, or candidate input is supplied. Removing
				// only the owned marker must fail ordinary TestMain setup before
				// any helper receipt or candidate build can execute.
				if !errors.As(err, &exitError) || exitError.ExitCode() != 1 || output.Len() != 0 || diagnostics.Len() == 0 {
					t.Fatalf("unmarked helper bypassed setup or emitted a receipt: %v; stdout=%s; stderr=%s", err, output.String(), diagnostics.String())
				}
				linuxNativeAssertAbsent(t, command)
				return
			}
			if err != nil || diagnostics.Len() != 0 || command.ProcessState == nil || !command.ProcessState.Success() {
				t.Fatalf("owned marked helper did not enter naturally: %v; stderr=%s", err, diagnostics.String())
			}
			var receipt linuxNativeReceipt
			if err := json.NewDecoder(&output).Decode(&receipt); err != nil {
				t.Fatal(err)
			}
			if receipt.PID != command.Process.Pid || receipt.PID <= 0 || receipt.Mode != "entry" || receipt.Root != command.Dir {
				t.Fatalf("helper receipt lacks selected mode/exact PID/owned root: %+v", receipt)
			}
			linuxNativeAssertAbsent(t, command)
		})
	}
}

func linuxNativeRemoveIsolationMarker(command *exec.Cmd) {
	var environment []string
	for _, entry := range command.Env {
		if !strings.HasPrefix(entry, isolatedRootVariable+"=") {
			environment = append(environment, entry)
		}
	}
	command.Env = environment
}

func TestLinuxCollectorWitnessNativeOwnedSelfExecEntry(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	command := linuxNativeCommand(ctx, t, "exec")
	var output, diagnostics bytes.Buffer
	command.Stdout, command.Stderr = &output, &diagnostics
	if err := command.Run(); err != nil || diagnostics.Len() != 0 || ctx.Err() != nil {
		t.Fatalf("owned self-exec target did not enter naturally: %v; context=%v; stderr=%s", err, ctx.Err(), diagnostics.String())
	}
	if command.ProcessState == nil || command.ProcessState.Pid() != command.Process.Pid || !command.ProcessState.Success() {
		t.Fatalf("same-PID self-exec lacks its natural Wait receipt: %v", command.ProcessState)
	}
	var receipt linuxNativeReceipt
	if err := json.NewDecoder(&output).Decode(&receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.PID != command.Process.Pid || receipt.PID <= 0 || receipt.Readings != 2 {
		t.Fatalf("self-exec entry did not follow the exact production collector child: %+v", receipt)
	}
	root, err := os.OpenRoot(command.Dir)
	if err != nil {
		t.Fatal(err)
	}
	data, readError := root.ReadFile("exec-ran")
	if err := errors.Join(readError, root.Close()); err != nil {
		t.Fatal(err)
	}
	if string(data) != "unexpected exec ran\n" {
		t.Fatalf("owned exec-target entry did not write its exact fixed-leaf marker: %q", data)
	}
	linuxNativeAssertAbsent(t, command)
}

func linuxNativeAssertAbsent(t *testing.T, command *exec.Cmd) {
	t.Helper()
	if command.Process == nil || command.Process.Pid <= 0 {
		t.Fatal("native control lacks the positive exact subject PID")
	}
	pid := command.Process.Pid
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("owned PID %d remains or absence is unknown: %v", pid, err)
	}
	if _, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid), "task")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned PID %d task cohort remains or absence is unknown: %v", pid, err)
	}
}

func TestLinuxCollectorWitnessNativeProductionReadings(t *testing.T) {
	for _, mode := range []string{"plain", "migrated"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
			defer cancel()
			command := linuxNativeCommand(ctx, t, mode)
			var output, diagnostics bytes.Buffer
			command.Stdout, command.Stderr = &output, &diagnostics
			trace, err := RunPublicLinuxCollectorWitness(ctx, command, "/")
			if err != nil {
				t.Fatalf("genuine production collector witness failed: %v; diagnostics=%s", err, diagnostics.String())
			}
			if command.ProcessState == nil || command.ProcessState.Pid() != command.Process.Pid || !command.ProcessState.Success() || command.ProcessState.ExitCode() != 0 {
				t.Fatalf("same exec.Cmd lacks natural exit-zero receipt: %+v", command.ProcessState)
			}
			if diagnostics.Len() != 0 || trace.Readings != 2 || trace.Active != 0 || trace.MaximumActive != 1 || !trace.Closed {
				t.Fatalf("genuine two-reading oracle failed: %+v; diagnostics=%s", trace, diagnostics.String())
			}
			var receipt linuxNativeReceipt
			if err := json.NewDecoder(&output).Decode(&receipt); err != nil {
				t.Fatal(err)
			}
			if receipt.PID != command.Process.Pid || receipt.Readings != 2 {
				t.Fatalf("receipt is not the exact two-reading production subject: %+v", receipt)
			}
			if mode == "migrated" {
				linuxNativeAssertMigration(t, trace, receipt)
			}
			linuxNativeAssertAbsent(t, command)
		})
	}
}

func linuxNativeAssertMigration(t *testing.T, trace PublicTUIHostProbeTrace, receipt linuxNativeReceipt) {
	t.Helper()
	if receipt.OpenTID <= 0 || receipt.ReadTID <= 0 || receipt.OpenTID == receipt.ReadTID {
		t.Fatalf("workers did not execute on distinct real TIDs: %+v", receipt)
	}
	for reading := 1; reading <= 2; reading++ {
		for _, path := range []string{"/proc/meminfo", "/proc/stat"} {
			opened, transferred, completed := false, false, false
			for _, event := range trace.Events {
				if event.Reading != reading || event.Operation != path {
					continue
				}
				opened = opened || event.Phase == "open" && event.PID == receipt.OpenTID
				transferred = transferred || strings.HasPrefix(event.Phase, "read-result=") && event.Phase != "read-result=0" && event.PID == receipt.ReadTID
				completed = completed || event.Phase == "complete" && event.PID == receipt.OpenTID
			}
			if !opened || !transferred || !completed {
				t.Fatalf("reading %d %s did not migrate its genuine FD across TIDs: %+v", reading, path, trace)
			}
		}
	}
}

func TestLinuxCollectorWitnessNativeRejectsCollectorOverlap(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	command := linuxNativeCommand(ctx, t, "overlap")
	trace, err := RunPublicLinuxCollectorWitness(ctx, command, "/")
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		t.Fatalf("deadline or cancellation cannot prove a collector-overlap refusal: %+v %v", trace, err)
	}
	if !errors.Is(err, errLinuxWitnessCollectorOverlap) || trace.Closed || trace.Readings != 2 || trace.MaximumActive != 2 {
		t.Fatalf("genuine concurrent production readings were not rejected: %+v %v", trace, err)
	}
	linuxNativeAssertAbsent(t, command)
}

func TestLinuxCollectorWitnessNativeRejectsLateVMStatOverlap(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	command := linuxNativeCommand(ctx, t, "late-overlap")
	trace, err := RunPublicLinuxCollectorWitness(ctx, command, "/")
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		t.Fatalf("deadline or cancellation cannot prove a late-vmstat overlap refusal: %+v %v", trace, err)
	}
	if !errors.Is(err, errLinuxWitnessCollectorOverlap) || trace.Closed || trace.Readings != 2 || trace.MaximumActive != 2 || trace.Active != 1 {
		t.Fatalf("real second collection escaped the still-active terminal vmstat interval: %+v %v", trace, err)
	}
	opened, eof := false, false
	for _, event := range trace.Events {
		if event.Reading != 1 || event.Operation != "/proc/vmstat" {
			continue
		}
		opened = opened || event.Phase == "open" && event.PID > 0
		eof = eof || event.Phase == "read-result=0" && event.PID > 0
		if event.Phase == "complete" {
			t.Fatal("late overlap control completed terminal vmstat before the second memory attempt")
		}
	}
	if !opened || !eof {
		t.Fatalf("late overlap was not anchored after genuine terminal vmstat EOF: %+v", trace)
	}
	linuxNativeAssertAbsent(t, command)
}

func TestLinuxCollectorWitnessNativeCancellationWhileKernelStopped(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	command := linuxNativeCommand(ctx, t, "plain")
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	command.SysProcAttr = &syscall.SysProcAttr{Ptrace: true}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	leader := command.Process.Pid
	observer := &linuxCollectorObserver{command: command, leader: leader, reducer: newLinuxCollectorReducer(leader, "/"), threads: map[int]*linuxWitnessThread{leader: {initial: true}}, stopped: map[int]int{}}
	for len(observer.stopped) == 0 && ctx.Err() == nil {
		if _, err := observer.drain(ctx, true); err != nil {
			t.Fatalf("initial kernel stop failed: %v; cleanup=%v", err, observer.retire(ctx))
		}
		if err := linuxWitnessPause(ctx); err != nil {
			t.Fatalf("initial kernel stop timed out: %v; cleanup=%v", err, observer.retire(ctx))
		}
	}
	if _, stopped := observer.stopped[leader]; !stopped {
		t.Fatalf("cancellation control lacks a real stopped PID: cleanup=%v", observer.retire(ctx))
	}
	cancel()
	observeError := observer.observe(ctx)
	cleanupError := observer.retire(ctx)
	if !errors.Is(observeError, context.Canceled) || observer.reducer.trace.Closed {
		t.Fatalf("stopped cancellation became successful trace evidence: %v; cleanup=%v", observeError, cleanupError)
	}
	// A kill/terminal race may leave cleanup errors. They remain errors, and no
	// natural Wait receipt is synthesized by this cancellation control.
	t.Logf("actual stopped-cancellation cleanup result: %v; ProcessState=%v", cleanupError, command.ProcessState)
	if len(observer.threads) != 0 {
		t.Fatalf("stopped cancellation retained unknown owned TIDs: %v", cleanupError)
	}
	linuxNativeAssertAbsent(t, command)
}

func TestLinuxCollectorWitnessNativeRejectsUnexpectedExec(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	command := linuxNativeCommand(ctx, t, "exec")
	marker := filepath.Join(command.Dir, "exec-ran")
	trace, err := RunPublicLinuxCollectorWitness(ctx, command, "/")
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		t.Fatalf("deadline or cancellation cannot prove an unexpected-exec refusal: %+v %v", trace, err)
	}
	if !errors.Is(err, errLinuxWitnessUnexpectedExecEntry) || trace.Closed || trace.Readings != 2 || trace.Active != 0 {
		t.Fatalf("unexpected exec was not refused after genuine complete readings: %+v %v", trace, err)
	}
	if _, statError := os.Stat(marker); !errors.Is(statError, os.ErrNotExist) {
		t.Fatalf("unexpected executable ran or its absence is unknown: %v", statError)
	}
	linuxNativeAssertAbsent(t, command)
}

func linuxNativeStartForeign(ctx context.Context, t *testing.T, mode string) (*exec.Cmd, *os.File) {
	t.Helper()
	command := linuxNativeCommand(ctx, t, mode)
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stdin = readEnd
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := readEnd.Close(); err != nil {
			t.Error(err)
		}
		if err := writeEnd.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Error(err)
		}
		if command.ProcessState == nil {
			if err := command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				t.Error(err)
			}
			if err := command.Wait(); err != nil {
				t.Error(err)
			}
		}
	})
	var receipt linuxNativeReceipt
	if err := json.NewDecoder(output).Decode(&receipt); err != nil || receipt.PID != command.Process.Pid || receipt.PID <= 0 {
		t.Fatalf("foreign control did not report its exact positive PID: %+v %v", receipt, err)
	}
	return command, writeEnd
}

func TestLinuxCollectorWitnessNativeFaultPreservesForeignChildren(t *testing.T) {
	foreignContext, foreignCancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer foreignCancel()
	exited, _ := linuxNativeStartForeign(foreignContext, t, "foreign-exited")
	live, release := linuxNativeStartForeign(foreignContext, t, "foreign-live")
	// Establish a real terminal status waiting for its own exec.Cmd.Wait. A
	// wildcard tracer wait would consume it and falsify the receipt below.
	statusPath := filepath.Join("/proc", strconv.Itoa(exited.Process.Pid), "status")
	for {
		data, err := os.ReadFile(statusPath)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "State:\tZ") {
			break
		}
		if err := linuxWitnessPause(foreignContext); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	command := linuxNativeCommand(ctx, t, "wrong-order")
	trace, err := RunPublicLinuxCollectorWitness(ctx, command, "/")
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		t.Fatalf("deadline or cancellation cannot prove a reducer-order refusal: %+v %v", trace, err)
	}
	if !errors.Is(err, errLinuxWitnessEvidenceOrder) || trace.Closed || trace.Readings != 1 || trace.Active != 1 {
		t.Fatalf("real malformed collection order did not fault the reducer: %+v %v", trace, err)
	}
	linuxNativeAssertAbsent(t, command)
	if err := syscall.Kill(live.Process.Pid, 0); err != nil {
		t.Fatalf("tracer cleanup signaled foreign live PID or lost its identity: %v", err)
	}
	if err := exited.Wait(); err != nil || exited.ProcessState == nil || !exited.ProcessState.Success() {
		t.Fatalf("tracer consumed foreign terminal status: %v; ProcessState=%v", err, exited.ProcessState)
	}
	if _, err := release.WriteString("release\n"); err != nil {
		t.Fatal(err)
	}
	if err := release.Close(); err != nil {
		t.Fatal(err)
	}
	if err := live.Wait(); err != nil || live.ProcessState == nil || !live.ProcessState.Success() {
		t.Fatalf("foreign live child failed its own natural Wait: %v; ProcessState=%v", err, live.ProcessState)
	}
}

func linuxHarvestableExitedChild(ctx context.Context, t *testing.T) (*exec.Cmd, int) {
	t.Helper()
	command := linuxNativeCommand(ctx, t, "foreign-exited")
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	var receipt linuxNativeReceipt
	if err := json.NewDecoder(output).Decode(&receipt); err != nil || receipt.PID != command.Process.Pid || receipt.PID <= 0 {
		t.Fatalf("exited control did not report its exact positive PID: %+v %v", receipt, err)
	}
	// Establish a real terminal status before harvest: the child has exited
	// zero and only its zombie report remains, exactly the short-lived clone
	// report the CI collector met.
	pid := command.Process.Pid
	statusPath := filepath.Join("/proc", strconv.Itoa(pid), "status")
	for {
		data, err := os.ReadFile(statusPath)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "State:\tZ") {
			break
		}
		if err := linuxWitnessPause(ctx); err != nil {
			t.Fatal(err)
		}
	}
	// Diagnose the harvest input state without consuming it: kill-zero tells
	// reaped apart from present, and the stat ppid tells foreign apart from
	// owned. These lines are evidence for the retirement repair.
	t.Logf("pre-harvest kill-zero=%v", syscall.Kill(pid, 0))
	if parentage, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat")); err != nil {
		t.Logf("pre-harvest stat=%v", err)
	} else {
		t.Logf("pre-harvest self=%d ppid=%s", os.Getpid(), fieldOrUnknown(parentage, 3))
	}
	return command, pid
}

func fieldOrUnknown(stat []byte, index int) string {
	fields := strings.Fields(string(stat))
	if index < 0 || index >= len(fields) {
		return "unknown"
	}
	return fields[index]
}

func linuxCertainHarvest(ctx context.Context, t *testing.T, observer *linuxCollectorObserver, pid int) (linuxWitnessHarvest, error) {
	t.Helper()
	// An interrupted wait reports nothing; only a certain harvest decides
	// the retirement. The zombie cannot resume or vanish.
	for {
		harvest, err := observer.linuxWitnessHarvest(pid, false)
		if err != nil || !harvest.uncertain {
			return harvest, err
		}
		if ctx.Err() != nil {
			t.Fatalf("interrupted harvest never settled: %v", ctx.Err())
		}
	}
}

func TestLinuxCollectorWitnessHarvestRetiresCleanCloneExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	command, pid := linuxHarvestableExitedChild(ctx, t)
	observer := &linuxCollectorObserver{command: command, leader: -1, reducer: newLinuxCollectorReducer(-1, "/owned/disk"), threads: map[int]*linuxWitnessThread{pid: {}}, stopped: map[int]int{}}
	harvest, err := linuxCertainHarvest(ctx, t, observer, pid)
	// The natural receipt result is evidence: success means the zombie
	// lingered past harvest, ECHILD means something reaped it first.
	t.Logf("post-harvest natural wait=%v", command.Wait())
	if err != nil {
		t.Fatalf("clean clone exit failed harvest: %v", err)
	}
	if harvest != (linuxWitnessHarvest{}) {
		t.Fatalf("clean clone exit produced harvest evidence: %+v", harvest)
	}
	if len(observer.threads) != 0 || observer.detached != 1 {
		t.Fatal("clean clone exit did not retire as detached")
	}
}

func TestLinuxCollectorWitnessHarvestKeepsLeaderExitFailClosed(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	command, pid := linuxHarvestableExitedChild(ctx, t)
	observer := &linuxCollectorObserver{command: command, leader: pid, reducer: newLinuxCollectorReducer(pid, "/owned/disk"), threads: map[int]*linuxWitnessThread{pid: {}}, stopped: map[int]int{}}
	_, err := linuxCertainHarvest(ctx, t, observer, pid)
	t.Logf("post-harvest natural wait=%v", command.Wait())
	if err == nil {
		t.Fatal("leader exit became accepted witness evidence")
	}
}
