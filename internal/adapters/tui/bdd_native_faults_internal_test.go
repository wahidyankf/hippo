package tui

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/status"
)

// BDDNativeFaultOperation retains real file operations, their results and order.
type BDDNativeFaultOperation struct {
	Kind  string `json:"Kind"`
	FD    int    `json:"FD"`
	Bytes int    `json:"Bytes"`
	Error string `json:"Error"`
	Errno int    `json:"Errno"`
}

// BDDNativeFaultReport adds genuine fault causes to the existing physical report.
type BDDNativeFaultReport struct {
	BDDNativeLifecycleReport

	Fault                string                     `json:"Fault"`
	Operations           []BDDNativeFaultOperation  `json:"Operations"`
	CloneCloseReturns    []BDDNativeFaultOperation  `json:"CloneCloseReturns"`
	CloneIdentity        []json.RawMessage          `json:"CloneIdentity"`
	CloneModes           []BDDNativeTerminalMode    `json:"CloneModes"`
	RestoredBeforeFault  []BDDNativeTerminalMode    `json:"RestoredBeforeFault"`
	FirstDiagnosticModes []BDDNativeTerminalMode    `json:"FirstDiagnosticModes"`
	RunError             string                     `json:"RunError"`
	RunInterruption      BDDNativeFaultInterruption `json:"RunInterruption"`
	RunContextError      string                     `json:"RunContextError"`
	RunContextCause      string                     `json:"RunContextCause"`
}

// BDDNativeFaultInterruption retains the actual production interruption fields.
type BDDNativeFaultInterruption struct {
	Signal int    `json:"Signal"`
	Name   string `json:"Name"`
}

type bddNativeFaultFile struct {
	*os.File

	descriptorMu sync.Mutex
	ready        func() error
	record       func(string, int, int, error)
}

func (file *bddNativeFaultFile) Fd() uintptr {
	file.descriptorMu.Lock()
	defer file.descriptorMu.Unlock()
	return file.File.Fd()
}

func (file *bddNativeFaultFile) Close() error {
	file.descriptorMu.Lock()
	defer file.descriptorMu.Unlock()
	return file.File.Close()
}

func (file *bddNativeFaultFile) Read(value []byte) (int, error) {
	_ = file.ready()
	n, err := file.File.Read(value)
	file.record("read", int(file.Fd()), n, err)
	return n, err
}

func (file *bddNativeFaultFile) Write(value []byte) (int, error) {
	_ = file.ready()
	n, err := file.File.Write(value)
	file.record("write", int(file.Fd()), n, err)
	return n, err
}

type bddNativeFaultDiagnostic struct {
	file   *os.File
	before func()
}

func (writer bddNativeFaultDiagnostic) Write(value []byte) (int, error) {
	writer.before()
	return writer.file.Write(value)
}

func bddNativeFaultOpenDevice(path string, flag int) (*os.File, error) {
	relative, err := filepath.Rel("/dev", path)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot("/dev")
	if err != nil {
		return nil, err
	}
	info, err := root.Lstat(relative)
	if err != nil {
		return nil, errors.Join(err, root.Close())
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeCharDevice == 0 {
		return nil, errors.Join(errors.New("native fault slave is not a nonsymlink character device"), root.Close())
	}
	file, openError := root.OpenFile(relative, flag|syscall.O_NOCTTY, 0)
	rootError := root.Close()
	if openError != nil {
		return nil, errors.Join(openError, rootError)
	}
	if rootError != nil {
		return nil, errors.Join(rootError, file.Close())
	}
	return file, nil
}

func bddNativeFaultReopen(anchor *os.File, path string, flag int) (*os.File, json.RawMessage, error) {
	var original, reopened syscall.Stat_t
	if err := syscall.Fstat(int(anchor.Fd()), &original); err != nil {
		return nil, nil, err
	}
	file, err := bddNativeFaultOpenDevice(path, flag)
	if err != nil {
		return nil, nil, err
	}
	fail := func(cause error) (*os.File, json.RawMessage, error) {
		return nil, nil, errors.Join(cause, file.Close())
	}
	if err := syscall.Fstat(int(file.Fd()), &reopened); err != nil {
		return fail(err)
	}
	if original.Dev != reopened.Dev || original.Ino != reopened.Ino || original.Rdev != reopened.Rdev || original.Uid != reopened.Uid || original.Mode != reopened.Mode || original.Uid == 0 || int64(original.Uid) != int64(os.Getuid()) || os.Getuid() != os.Geteuid() {
		return fail(errors.New("native fault reopen differs from its retained owned slave"))
	}
	before, err := bddNativeLifecycleModes(anchor, file)
	if err != nil {
		return fail(err)
	}
	if len(before) != 2 || !reflect.DeepEqual(before[0].Mode, before[1].Mode) || before[0].Foreground != os.Getpid() || before[1].Foreground != os.Getpid() {
		return fail(errors.New("native fault reopen differs from owned terminal modes or foreground"))
	}
	identity, err := json.Marshal(reopened)
	if err != nil {
		return fail(err)
	}
	return file, identity, nil
}

func bddNativeFaultAwait(parent context.Context, predicate func() (bool, error)) error {
	ctx, cancel := context.WithTimeout(parent, 500*time.Millisecond)
	defer cancel()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		matched, err := predicate()
		if err != nil || matched {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// RunBDDNativeFaultHelper exercises production Run and CLI with owned OS faults.
// The lifecycle sources remain untouched; this helper reuses their physical and
// goroutine observations, and supplies no replacement program or restore result.
func RunBDDNativeFaultHelper(parent context.Context, input, output, diagnostic *os.File, ready func() error, directory string) (int, error) {
	root, err := openBDDNativeObserverRoot(directory)
	if err != nil {
		return 0, err
	}
	fault := os.Getenv("HIPPO_TEST_NATIVE_TUI_FAULT")
	switch fault {
	case "startup", "input read", "output write", "restoration", "restoration-SIGTERM", "JSON":
	default:
		return 0, errors.Join(errors.New("unknown native terminal fault input"), root.Close())
	}
	report := BDDNativeFaultReport{Fault: fault}
	report.Before, err = bddNativeLifecycleModes(input, output)
	if err != nil {
		return 0, errors.Join(err, root.Close())
	}
	inputFlag, outputFlag := os.O_RDWR, os.O_RDWR
	if fault == "input read" {
		inputFlag = os.O_WRONLY
	}
	if fault == "startup" || fault == "JSON" {
		outputFlag = os.O_RDONLY
	}
	path := os.Getenv("HIPPO_TEST_NATIVE_TUI_SLAVE")
	inputClone, inputIdentity, err := bddNativeFaultReopen(input, path, inputFlag)
	if err != nil {
		return 0, errors.Join(err, root.Close())
	}
	outputClone, outputIdentity, err := bddNativeFaultReopen(output, path, outputFlag)
	if err != nil {
		return 0, errors.Join(err, inputClone.Close(), root.Close())
	}
	report.CloneIdentity = []json.RawMessage{inputIdentity, outputIdentity}
	report.CloneModes, err = bddNativeLifecycleModes(inputClone, outputClone)
	if err != nil {
		return 0, errors.Join(err, inputClone.Close(), outputClone.Close(), root.Close())
	}
	var readonly *os.File
	if fault == "output write" {
		var identity json.RawMessage
		readonly, identity, err = bddNativeFaultReopen(output, path, os.O_RDONLY)
		if err != nil {
			return 0, errors.Join(err, inputClone.Close(), outputClone.Close(), root.Close())
		}
		report.CloneIdentity = append(report.CloneIdentity, identity)
	}
	var fixtureError error
	recorder := bddNativeFaultRecorder{report: &report, failure: &fixtureError}
	record := recorder.record
	var once sync.Once
	var readyError error
	reportReady := func() error {
		once.Do(func() { readyError = ready() })
		return readyError
	}
	inputTrace := &bddNativeFaultFile{File: inputClone, ready: reportReady, record: record}
	outputTrace := &bddNativeFaultFile{File: outputClone, ready: reportReady, record: record}
	ctx, stop := cli.SignalContext(parent)
	defer stop()
	inputClosed := false
	observation := bddNativeFaultObservation{
		root: root, report: &report, fault: fault, input: input, output: output,
		inputTrace: inputTrace, outputClone: outputClone, readonly: readonly,
		ready: reportReady, record: record, inputClosed: &inputClosed,
	}
	observe := observation.observe
	boundary := cli.Application{
		Stdin: inputClone, Stdout: outputClone, Environment: os.Environ(),
		Stderr: bddNativeFaultDiagnostic{file: diagnostic, before: func() {
			if !report.RunnerReturned {
				fixtureError = errors.Join(fixtureError, errors.New("native fault diagnostic preceded production Run return"))
				return
			}
			if report.FirstDiagnosticModes == nil {
				report.FirstDiagnosticModes, err = bddNativeLifecycleModes(input, output)
				record("first-cli-diagnostic", int(diagnostic.Fd()), 0, err)
				fixtureError = errors.Join(fixtureError, err)
			}
		}},
		TerminalCapabilities: func() (bool, bool, error) { return TerminalCapabilities(inputClone, outputClone) },
		RunTUI: func(ctx context.Context, _ application.ObservationRequest, interval time.Duration, styling bool) (status.Interruption, error) {
			interruption, runError := Run(ctx, observe, Options{
				Interval: interval, Styling: styling, Environment: os.Environ(),
				Input: inputTrace, Output: outputTrace,
			})
			report.RunnerReturned = true
			report.RunInterruption = BDDNativeFaultInterruption{Signal: interruption.Signal, Name: interruption.Name}
			if contextError := ctx.Err(); contextError != nil {
				report.RunContextError = contextError.Error()
			}
			if cause := context.Cause(ctx); cause != nil {
				report.RunContextCause = cause.Error()
			}
			if runError != nil {
				report.RunError = runError.Error()
			}
			record("production-Run-returned", -1, 0, nil)
			var modeError, stackError error
			report.After, modeError = bddNativeLifecycleModes(input, output)
			report.GoroutinesAfter, stackError = bddNativeLifecycleStacks()
			fixtureError = errors.Join(fixtureError, modeError, stackError)
			return interruption, runError
		},
	}
	arguments := bddNativeFaultArguments(fault)
	report.ExitCode, _ = boundary.Run(ctx, arguments)
	file, reportError := root.OpenFile("fault-report.json", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)

	if !inputClosed {
		fixtureError = errors.Join(fixtureError, closeBDDNativeFaultClone(&report, "owned-run-input-clone-final-close", inputClone))
	}
	fixtureError = errors.Join(fixtureError, closeBDDNativeFaultClone(&report, "owned-run-output-clone-final-close", outputClone))
	if readonly != nil {
		fixtureError = errors.Join(fixtureError, closeBDDNativeFaultClone(&report, "owned-readonly-clone-final-close", readonly))
	}
	if reportError == nil {
		reportError = errors.Join(json.NewEncoder(file).Encode(report), file.Close())
	}
	return report.ExitCode, errors.Join(fixtureError, readyError, reportError, root.Close())
}

func bddNativeFaultArguments(fault string) []string {
	arguments := []string{"tui", "--interval", "1h"}
	if fault == "JSON" {
		arguments = append([]string{"--output", "json"}, arguments...)
	}
	return arguments
}

func closeBDDNativeFaultClone(report *BDDNativeFaultReport, kind string, clone *os.File) error {
	fd := int(clone.Fd())
	closeError := clone.Close()
	row := BDDNativeFaultOperation{Kind: kind, FD: fd}
	if closeError != nil {
		row.Error = closeError.Error()
		if errno, ok := errors.AsType[syscall.Errno](closeError); ok {
			row.Errno = int(errno)
		}
	}
	report.CloneCloseReturns = append(report.CloneCloseReturns, row)
	return closeError
}

// The fault observation owns no context; each actual worker request supplies it.
type bddNativeFaultObservation struct {
	root                                 *os.Root
	report                               *BDDNativeFaultReport
	fault                                string
	input, output, outputClone, readonly *os.File
	inputTrace                           *bddNativeFaultFile
	ready                                func() error
	record                               func(string, int, int, error)
	inputClosed                          *bool
}

func (state *bddNativeFaultObservation) observe(ctx context.Context) (application.ObservationSnapshot, int, error) {
	state.report.Requests++
	state.report.Active++
	defer func() { state.report.Active-- }()
	if state.report.Requests == 1 {
		if err := state.ready(); err != nil {
			return application.ObservationSnapshot{}, 0, err
		}
		state.report.Completed++
		snapshot := BDDNativeObserverSnapshot()
		now := time.Now()
		snapshot.SampledAt = &now
		return snapshot, 0, nil
	}
	var observationError error
	state.report.GoroutinesBefore, observationError = bddNativeLifecycleStacks()
	if observationError == nil {
		observationError = writeBDDNativeLifecycleReport(state.root, "lifecycle-progress.json", state.report.BDDNativeLifecycleReport)
	}
	if observationError != nil {
		return application.ObservationSnapshot{}, 0, observationError
	}
	if state.fault == "output write" {
		observationError = state.applyOutputFault(ctx)
		if observationError != nil {
			return application.ObservationSnapshot{}, 0, observationError
		}
	}
	<-ctx.Done()
	state.report.Cancelled++
	if state.fault == "restoration" || state.fault == "restoration-SIGTERM" {
		observationError = state.applyRestorationFault(ctx)
	}
	return application.ObservationSnapshot{}, 0, errors.Join(ctx.Err(), observationError)
}

func (state *bddNativeFaultObservation) applyOutputFault(ctx context.Context) error {
	var observationError error
	observationError = bddNativeFaultAwait(ctx, func() (bool, error) {
		file, err := state.root.Open("apply-output-fault")
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return true, file.Close()
	})
	if observationError == nil {
		observationError = syscall.Dup2(int(state.readonly.Fd()), int(state.outputClone.Fd()))
		state.record("owned-output-fd-readonly-swap", int(state.outputClone.Fd()), 0, observationError)
	}
	if observationError == nil {
		observationError = writeBDDNativeLifecycleReport(state.root, "fault-applied.json", state.report.BDDNativeLifecycleReport)
	}
	return observationError
}

func (state *bddNativeFaultObservation) applyRestorationFault(ctx context.Context) error {
	var observationError error
	observationError = bddNativeFaultAwait(context.WithoutCancel(ctx), func() (bool, error) {
		modes, err := bddNativeLifecycleModes(state.input, state.output)
		if err != nil {
			return false, err
		}
		if !reflect.DeepEqual(modes, state.report.Before) {
			return false, nil
		}
		state.report.RestoredBeforeFault = modes
		return true, nil
	})
	if observationError == nil {
		fd := int(state.inputTrace.Fd())
		observationError = state.inputTrace.Close()
		*state.inputClosed = observationError == nil
		state.record("owned-run-input-clone-close", fd, 0, observationError)
		_, queryError := runnerGetTermios(fd)
		state.record("closed-run-input-termios", fd, 0, queryError)
		if !errors.Is(queryError, syscall.EBADF) {
			observationError = errors.Join(observationError, errors.New("closed native Run clone did not produce EBADF"))
		}
	}
	return observationError
}

type bddNativeFaultRecorder struct {
	mu      sync.Mutex
	report  *BDDNativeFaultReport
	failure *error
}

func (recorder *bddNativeFaultRecorder) record(kind string, descriptor, byteCount int, operationError error) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.report.Operations) >= 64 {
		*recorder.failure = errors.Join(*recorder.failure, errors.New("native terminal fault operation observation exceeded its bound"))
		return
	}
	row := BDDNativeFaultOperation{Kind: kind, FD: descriptor, Bytes: byteCount}
	if operationError != nil {
		row.Error = operationError.Error()
		if errno, ok := errors.AsType[syscall.Errno](operationError); ok {
			row.Errno = int(errno)
		}
	}
	recorder.report.Operations = append(recorder.report.Operations, row)
}
