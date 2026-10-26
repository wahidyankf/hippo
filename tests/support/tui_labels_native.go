package support

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	runtimeadapter "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/bootstrap"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/tests/contract"
)

type annotationNativeRun struct {
	root, scratch, configPath, ready, release, token, operation string
	reservation, json                                           bool
	childExit, exit                                             int
	output, diagnostics                                         bytes.Buffer
	readyData, retainedAnnotation                               []byte
	mutationVerified                                            bool
	runError                                                    error
	totals                                                      coordination.ReservationTotals
	lockWriter                                                  *annotationLockWriter
}

// AnnotationNativeCase selects a genuine filesystem/child contract fixture.
// Binary is supplied only by the compiled package's ordinary TestMain.
type AnnotationNativeCase struct {
	Operation            string
	Reservation, JSON    bool
	ChildExit            int
	Binary               string
	FailDiagnosticWriter bool
}

// CheckAnnotationNativeRun executes the actual CLI/runtime or compiled child
// and holds it to status, allocation, release, diagnostic and privacy assertions.
// It shares the native fixture used by the canonical annotation step handlers.
func CheckAnnotationNativeRun(ctx context.Context, adapter contract.Adapter, test AnnotationNativeCase) error {
	if adapter.Name != contract.Integration && adapter.Name != contract.E2E {
		return errors.New("annotation native contract requires Integration or E2E")
	}
	driver := NewDriver(adapter)
	defer driver.closeAnnotationNativeFixture()
	if adapter.Name == contract.E2E {
		if test.Binary == "" || test.Binary != os.Getenv("HIPPO_BIN") {
			return errors.New("compiled annotation contract lacks the TestMain-selected candidate")
		}
		driver.binary = test.Binary
	}
	state, err := driver.prepareAnnotationNative(test.Operation, test.Reservation, test.ChildExit, test.JSON)
	if err != nil {
		return err
	}
	if test.FailDiagnosticWriter {
		if adapter.Name != contract.Integration {
			return errors.New("failed invocation writer is an embedded Integration boundary")
		}
		state.lockWriter.failure = errors.New("/private/downstream-writer raw-error\x1b[31m")
	}
	if err = driver.executeAnnotationNative(ctx, state); err != nil {
		return err
	}
	if err = state.requireOutcome(); err != nil {
		return err
	}
	if test.FailDiagnosticWriter {
		if state.lockWriter.calls != 1 || state.diagnostics.Len() != 0 {
			return fmt.Errorf("native failed writer changed/report retried: calls=%d stderr=%q", state.lockWriter.calls, state.diagnostics.String())
		}
		return nil
	}
	if test.Operation == "healthy" || test.Operation == annotationOperationMissing {
		if state.diagnostics.Len() != 0 {
			return fmt.Errorf("healthy/missing annotation invented diagnostic %q", state.diagnostics.String())
		}
		return nil
	}
	if strings.Contains(state.output.String()+state.diagnostics.String(), state.root) || strings.Contains(state.output.String()+state.diagnostics.String(), state.scratch) {
		return errors.New("native diagnostic leaked a private path")
	}
	return requireAnnotationDiagnostics(state.output.String(), state.diagnostics.String(), test.Operation)
}

// annotationLockWriter attempts the actual coordination lock whenever a
// diagnostic is written. A contention error is retained as an assertion,
// rather than deadlocking a broken implementation inside the transaction.
type annotationLockWriter struct {
	root           string
	output         *bytes.Buffer
	calls, blocked int
	failure        error
}

func (writer *annotationLockWriter) Write(data []byte) (int, error) {
	writer.calls++
	file, err := os.OpenFile(filepath.Join(writer.root, "coordination.lock"), os.O_RDWR, 0)
	if err != nil {
		writer.blocked++
		return 0, err
	}
	lockError := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if lockError != nil {
		writer.blocked++
	} else {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	}
	err = errors.Join(lockError, file.Close())
	if err != nil {
		return 0, err
	}
	if writer.failure != nil {
		return 0, writer.failure
	}
	return writer.output.Write(data)
}

func (driver *Driver) prepareAnnotationNative(operation string, reservation bool, childExit int, jsonMode bool) (*annotationNativeRun, error) {
	if driver.mode == contract.Unit {
		return nil, errors.New("native annotation fixture cannot be used as Unit")
	}
	scratch, err := driver.temporaryRoot()
	if err != nil {
		return nil, err
	}
	state := &annotationNativeRun{scratch: scratch, root: filepath.Join(scratch, "private-state"), configPath: filepath.Join(scratch, "config.json"), ready: filepath.Join(scratch, "ready"), release: filepath.Join(scratch, "release"), operation: operation, reservation: reservation, childExit: childExit, json: jsonMode}
	if err = os.Mkdir(state.root, 0o700); err != nil {
		return nil, err
	}
	document := boundarySchemaOne
	if reservation {
		document = boundarySchemaTwo
	}
	if err = os.WriteFile(state.configPath, []byte(document), 0o600); err != nil {
		return nil, err
	}
	if operation == annotationOperationWrite || operation == annotationOperationBoth {
		if err = os.WriteFile(filepath.Join(state.root, "run-display"), []byte("private annotation blocker"), 0o600); err != nil {
			return nil, err
		}
	}
	state.lockWriter = &annotationLockWriter{root: state.root, output: &state.diagnostics}
	return state, nil
}

const annotationChildScript = `printf '%s:%s:%s' "$HIPPO_SESSION" "$HIPPO_CONCURRENCY" "${HIPPO_RESERVED_MEMORY_BYTES:-none}" > "$ANNOTATION_READY.tmp"; /bin/mv "$ANNOTATION_READY.tmp" "$ANNOTATION_READY"; while [ ! -e "$ANNOTATION_RELEASE" ]; do /bin/sleep 0.01; done; printf 'annotation-child-payload\n'; exit "$ANNOTATION_CHILD_EXIT"`

func (state *annotationNativeRun) faultAfterLaunch(ctx context.Context) error {
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	if err := state.awaitAnnotationLaunch(ctx, deadline.C, ticker.C); err != nil {
		return err
	}
	if err := state.applyAnnotationFault(); err != nil {
		return err
	}
	state.mutationVerified = true
	return os.WriteFile(state.release, []byte("release real child"), 0o600)
}

func (state *annotationNativeRun) awaitAnnotationLaunch(ctx context.Context, deadline, ticker <-chan time.Time) error {
	for {
		data, err := os.ReadFile(state.ready)
		if err == nil && len(strings.Split(string(data), ":")) == 3 {
			state.readyData = data
			state.token = strings.Split(string(data), ":")[0]
			if len(state.token) != 32 {
				return errors.New("launched child did not expose a complete owned token")
			}
			break
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return errors.New("actual guarded child never reached annotation fault rendezvous")
		case <-ticker:
		}
	}
	return nil
}

func (state *annotationNativeRun) applyAnnotationFault() error {
	directory := filepath.Join(state.root, "run-display")
	sidecar := filepath.Join(directory, state.token+".json")
	switch state.operation {
	case annotationOperationWrite:
		info, err := os.Stat(directory)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("publication fault no longer has its actual regular-file blocker")
		}
		if err = os.Remove(directory); err != nil {
			return err
		}
		if err = os.Mkdir(directory, 0o700); err != nil {
			return err
		}
	case annotationOperationRemoval, annotationOperationMissing, "healthy":
		if err := state.verifyOwnedAnnotation(sidecar); err != nil {
			return err
		}
		switch state.operation {
		case annotationOperationRemoval:
			if err := os.Rename(directory, filepath.Join(state.scratch, "retained-display")); err != nil {
				return err
			}
			if err := os.WriteFile(directory, []byte("private removal blocker"), 0o600); err != nil {
				return err
			}
		case annotationOperationMissing:
			if err := os.Remove(sidecar); err != nil {
				return err
			}
		}
	case annotationOperationBoth:
		info, err := os.Stat(directory)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("two-operation fixture lost its actual regular-file blocker")
		}
	default:
		return fmt.Errorf("unknown native annotation fixture %q", state.operation)
	}
	return nil
}

func (driver *Driver) executeAnnotationNative(parent context.Context, state *annotationNativeRun) error {
	ctx, cancel := context.WithTimeout(parent, 40*time.Second)
	defer cancel()
	environment := []string{hippoRootEnvironment + "=" + state.root, "ANNOTATION_READY=" + state.ready, "ANNOTATION_RELEASE=" + state.release, "ANNOTATION_CHILD_EXIT=" + strconv.Itoa(state.childExit), isolatedPath}
	arguments := []string{runCommandName, configFlag, state.configPath, annotationCwdFlag, state.scratch, diskPathFlag, state.scratch, "--profile", profileConstrained, "--label", annotationOwnerLabel, annotationColorFlag, "always"}
	if state.json {
		arguments = append([]string{outputFlag, outputJSONValue}, arguments...)
	}
	if state.reservation {
		arguments = append(arguments, reserveCPUFlag, "2", reserveMemoryFlag, "1024")
	}
	arguments = append(arguments, "--", shellPath, "-c", annotationChildScript, "private-argument")
	faultDone := make(chan error, 1)
	go func() {
		err := state.faultAfterLaunch(ctx)
		if err != nil {
			cancel()
		}
		faultDone <- err
	}()
	if driver.mode == contract.E2E {
		if err := driver.runCompiledAnnotation(ctx, state, arguments, environment); err != nil {
			cancel()
			<-faultDone
			return err
		}
	} else {
		driver.runEmbeddedAnnotation(ctx, state, arguments, environment)
	}

	if err := <-faultDone; err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	mode := exclusiveMode
	if state.reservation {
		mode = reservationMode
	}
	totals, err := runtimeadapter.NewEngine().Snapshot(parent, state.root, mode)
	state.totals = totals
	return err
}

func (state *annotationNativeRun) requireOutcome() error {
	if !state.mutationVerified || state.exit != state.childExit || state.output.String() != "annotation-child-payload\n" || state.totals.ActiveOwners != 0 || state.totals.WaitingOwners != 0 {
		return fmt.Errorf("native annotation fault changed launched run/release: verified=%t exit=%d want=%d stdout=%q totals=%+v", state.mutationVerified, state.exit, state.childExit, state.output.String(), state.totals)
	}
	want := state.token + ":2:none"
	if state.reservation {
		want = state.token + ":2:1073741824"
	}
	if string(state.readyData) != want {
		return fmt.Errorf("actual child allocation=%q want=%q", state.readyData, want)
	}
	if state.lockWriter.blocked != 0 {
		return fmt.Errorf("annotation diagnostics held the actual coordination lock: %d", state.lockWriter.blocked)
	}
	if state.operation == annotationOperationRemoval {
		after, err := os.ReadFile(filepath.Join(state.scratch, "retained-display", state.token+".json"))
		if err != nil || !bytes.Equal(after, state.retainedAnnotation) {
			return errors.New("owned failed cleanup lost its retained annotation bytes")
		}
	}
	for _, path := range []string{filepath.Join(state.root, "sessions", state.token+".json"), filepath.Join(state.root, "owner-metadata", state.token+".json")} {
		if err := requireAnnotationNativeOwnerAbsent(path); err != nil {
			return err
		}
	}
	return nil
}

// Compile-time interface checks keep the memory boundary on ordinary ports.
var (
	_ application.RunCoordination = (*annotationMemoryRun)(nil)
	_ application.WorkloadRuntime = (*annotationMemoryRun)(nil)
	_ application.RunEvidence     = (*annotationMemoryRun)(nil)
	_ application.PortRuntime     = (*annotationMemoryRun)(nil)
	_ application.RunClock        = (*annotationMemoryRun)(nil)
)

const (
	annotationCwdFlag   = "--cwd"
	annotationColorFlag = "--color"
)

// closeAnnotationNativeFixture ends only the temporary paths and host servers
// this fresh native driver owns. Teardown remains independent of caller cancellation.
func (driver *Driver) closeAnnotationNativeFixture() {
	for _, stop := range slices.Backward(driver.stops) {
		stop()
	}
	driver.stops = nil
	for _, path := range driver.temporaryPaths {
		_ = os.RemoveAll(path)
	}
	driver.temporaryPaths = nil
}

func (state *annotationNativeRun) verifyOwnedAnnotation(sidecar string) error {
	data, err := os.ReadFile(sidecar)
	if err != nil {
		return fmt.Errorf("launched fresh owner has no annotation before fault: %w", err)
	}
	var annotation struct {
		RunID string `json:"runId"`
		Label string `json:"label"`
	}
	if err = json.Unmarshal(data, &annotation); err != nil || annotation.RunID != state.token || annotation.Label != annotationOwnerLabel {
		return errors.New("fault target is not this invocation's freshly published owned annotation")
	}
	state.retainedAnnotation = data
	return nil
}

func (driver *Driver) runCompiledAnnotation(ctx context.Context, state *annotationNativeRun, arguments, environment []string) error {
	if driver.binary == "" {
		return errors.New("compiled annotation run has no TestMain-selected stamped candidate")
	}
	prepared, err := driver.quietHostEnvironment(environment)
	if err != nil {
		return err
	}
	command, err := driver.NativeAnnotationCommand(ctx, arguments...)
	if err != nil {
		return err
	}
	command.Dir, command.Env, command.Stdin = state.scratch, prepared, strings.NewReader("")
	command.Stdout, command.Stderr = &state.output, &state.diagnostics
	state.runError = command.Run()
	state.exit = 0
	if state.runError != nil {
		var exitError *exec.ExitError
		if !errors.As(state.runError, &exitError) {
			return state.runError
		}
		state.exit = exitError.ExitCode()
	}
	return nil
}

func (driver *Driver) runEmbeddedAnnotation(ctx context.Context, state *annotationNativeRun, arguments, environment []string) {
	boundary := bootstrap.WithDefaults(cli.Application{Stdin: strings.NewReader(""), Stdout: &state.output, Stderr: state.lockWriter, Environment: environment, Collector: &sequenceCollector{samples: []policy.Sample{healthySample(time.Now())}}, Sleep: func(time.Duration) {}})
	state.exit, state.runError = boundary.Run(ctx, arguments)
}

func requireAnnotationNativeOwnerAbsent(path string) error {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reported annotation fault before its real owner retired: %s: %w", path, err)
	}
	return fmt.Errorf("reported annotation fault before its real owner retired: %s: %v", path, nil)
}

// NativeAnnotationCommand creates a command for this driver's selected public candidate.
// The caller retains responsibility for streams, environment, execution and joining.
func (driver *Driver) NativeAnnotationCommand(ctx context.Context, arguments ...string) (*exec.Cmd, error) {
	if driver.mode != contract.E2E {
		return nil, errors.New("compiled annotation command requires the E2E boundary")
	}
	if driver.binary == "" || driver.binary != os.Getenv("HIPPO_BIN") {
		return nil, errors.New("compiled annotation run has no unchanged TestMain-selected stamped candidate")
	}
	return exec.CommandContext(ctx, driver.binary, arguments...), nil
}
