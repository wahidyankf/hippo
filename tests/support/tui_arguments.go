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
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/adapters/tui"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/status"
	"github.com/wahidyankf/hippo/tests/contract"
	"golang.org/x/sys/unix"
)

const (
	tuiIdleState                   = "idle"
	e2eBehaviourEntry              = "TestE2EBehaviours"
	tuiRunIDField                  = "runId"
	darwinMemoryPressureTool       = "memory_pressure"
	darwinVMStatTool               = "vm_stat"
	darwinSysctlTool               = "sysctl"
	tuiEmptyVariant                = "empty"
	tuiMemoryChild                 = "memory-child"
	tuiStartPhase                  = "start"
	tuiOwnerRole                   = "owner"
	tuiOtherRole                   = "other"
	tuiUnsupportedSchema           = "unsupported schema"
	tuiUnreadableAnnotation        = "unreadable"
	tuiLabelField                  = "label"
	tuiMismatchedRunID             = "mismatched run ID"
	tuiOversizedAnnotation         = "larger than 512 bytes"
	linuxMeminfoName               = "meminfo"
	tuiCanceledEOFPhase            = "canceled-eof"
	tuiCanceledResetPhase          = "canceled-reset"
	tuiActiveState                 = "active"
	tuiAnnotationFaultVariant      = "annotation-fault"
	publicHolderPostChildPhase     = "post-child"
	publicHolderReadOnlyOutputKind = "readonly-output"
	tuiCommandName                 = "tui"
	tuiMissingConfigName           = "missing.json"
	tuiSurfaceInput                = "non-terminal input"
	tuiSurfaceOutputName           = "non-terminal output"
	tuiSurfaceDumb                 = "TERM set to dumb"
)

type tuiArgumentState struct {
	boundary                          cli.Application
	providers                         tuiCLIProviders
	output, diagnostics               bytes.Buffer
	inputTTY, outputTTY               bool
	term, root                        string
	surface                           string
	capabilityError                   error
	capabilities, sessions            int
	restored, diagnosticBeforeRestore bool
	jsonFault                         bool
	inspectionFailure                 bool
	inspectionEvidence                string
	fileOutput, fileDiagnostics       *os.File
}

type tuiArgumentDiagnosticWriter struct{ state *tuiArgumentState }

// BindingsWithNativeTUIArguments retains the neutral inventory and replaces only
// the five JSON terminal-fault handlers at the controlled Integration boundary.
// The factory is passive, and Unit and public E2E keep their existing handlers.
func (driver *Driver) BindingsWithNativeTUIArguments(factory func(string) any) []contract.StepBinding {
	bindings := driver.Bindings()
	if driver.mode != contract.Integration || factory == nil {
		return bindings
	}
	for index := range bindings {
		switch bindings[index].Pattern {
		case `^a TUI terminal whose session can be restored$`,
			`^TUI rendering fails with global JSON output requested$`,
			`^the terminal is restored before the failure is written$`,
			`^the only machine-readable failure body names hippo tui with exit 125$`,
			`^TUI frames are not serialized as JSON status records$`:
			if handler := factory(bindings[index].Pattern); handler != nil {
				bindings[index].Handler = handler
			}
		}
	}
	return bindings
}

func (writer tuiArgumentDiagnosticWriter) Write(value []byte) (int, error) {
	if writer.state.jsonFault && !writer.state.restored {
		writer.state.diagnosticBeforeRestore = true
	}
	return writer.state.diagnostics.Write(value)
}

func (driver *Driver) tuiArgumentBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^observation providers and terminal entry record calls$`, driver.prepareTUIArguments),
		step(`^TUI is requested with (an interval of 999 milliseconds|an interval of zero|a negative interval|a malformed source filter|a malformed tag filter|both invalid source and tag filters)$`, driver.requestTUIArgumentMistake),
		step(`^it exits 2 naming hippo\.args\.invalid before any provider or terminal entry is called$`, driver.requireTUIArgumentRefusal),
		step(`^TUI is requested with invalid source and tag filters$`, func(ctx context.Context) error {
			return driver.requestTUIArgumentMistake(ctx, "both invalid source and tag filters")
		}),
		step(`^the usage diagnostic identifies the source filter first$`, driver.requireTUISourcePrecedence),
		step(`^no observation or terminal session starts$`, driver.requireTUIArgumentNoEntry),
		step(`^TUI has (non-terminal input|non-terminal output|TERM set to dumb)$`, driver.prepareTUISurface),
		step(`^TUI is requested without help$`, driver.requestTUIPlain),
		step(`^it exits 2 naming hippo\.args\.invalid without entering terminal mode$`, driver.requireTUITerminalRefusal),
		step(`^no configuration host or coordination observation is requested$`, driver.requireTUIArgumentNoObservation),
		step(`^TUI terminal capability inspection fails$`, driver.prepareTUIInspectionFailure),
		step(`^TUI is requested$`, driver.requestTUIPlain),
		step(`^it exits 125 with a privacy-safe diagnostic before terminal entry or observation$`, driver.requireTUIInspectionFailure),
		step(`^a TUI terminal whose session can be restored$`, driver.prepareTUIRestoredFault),
		step(`^TUI rendering fails with global JSON output requested$`, driver.requestTUIJSONFault),
		step(`^the terminal is restored before the failure is written$`, driver.requireTUIRestoredBeforeDiagnostic),
		step(`^the only machine-readable failure body names hippo tui with exit 125$`, driver.requireTUIJSONFailureBody),
		step(`^TUI frames are not serialized as JSON status records$`, driver.requireTUINoJSONFrames),
		step(`^an isolated shared root with no owned run or display annotation$`, driver.prepareTUIArguments),
		step(`^run is requested with an explicit (empty|65 ASCII characters|a forward slash|a backward slash|a control character|dot-dot) display label$`, driver.requestTUIInvalidLabel),
		step(`^it exits 2 naming hippo\.args\.invalid before registering or launching$`, driver.requireTUIInvalidLabel),
		step(`^the shared root has no new run or annotation$`, driver.requireTUIArgumentNoObservation),
	}
}

func (driver *Driver) requestTUIInvalidLabel(ctx context.Context, value string) error {
	labels := map[string]string{tuiEmptyVariant: "", "65 ASCII characters": strings.Repeat("a", 65), "a forward slash": "bad/name", "a backward slash": `bad\name`, "a control character": "bad\x1b[31m", "dot-dot": ".."}
	label, ok := labels[value]
	if !ok {
		return fmt.Errorf("unknown invalid label case %q", value)
	}
	return driver.executeTUIArguments(ctx, []string{runCommandName, configFlag, tuiMissingConfigName, tuiNativeLabelFlag, label, "--", "/usr/bin/printf", "private argument"})
}

func (driver *Driver) requireTUIInvalidLabel() error {
	if err := driver.requireTUIArgumentRefusal(); err != nil {
		return err
	}
	if !strings.Contains(driver.errorOutput, tuiNativeLabelFlag) {
		return fmt.Errorf("invalid label did not name its argument: %q", driver.errorOutput)
	}
	return nil
}

func (driver *Driver) prepareTUIArguments() error {
	state := &tuiArgumentState{inputTTY: true, outputTTY: true, term: "xterm-256color"}
	state.boundary = cli.Application{
		Stdin: strings.NewReader(""), Stdout: &state.output, Stderr: tuiArgumentDiagnosticWriter{state},
		Environment: []string{"TERM=" + state.term}, Configuration: &state.providers, Collector: &state.providers,
		Observer: application.ObservationServices{Configuration: &state.providers, Collector: &state.providers, Runtime: &state.providers},
		TerminalCapabilities: func() (bool, bool, error) {
			state.capabilities++
			return state.inputTTY, state.outputTTY, state.capabilityError
		},
		RunTUI: func(context.Context, application.ObservationRequest, time.Duration, bool) (status.Interruption, error) {
			state.sessions++
			state.restored = true
			return status.Interruption{}, status.Fail(status.CodeSupervisionFailed, "TUI terminal session failed")
		},
	}
	if err := driver.prepareTUIArgumentFiles(state); err != nil {
		return err
	}
	driver.tuiArguments = state
	return nil
}

func (driver *Driver) prepareTUIArgumentFiles(state *tuiArgumentState) error {
	if driver.mode == contract.Unit {
		return nil
	}
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	state.root = root
	if driver.mode == contract.Integration {
		var err error
		state.fileOutput, err = os.Create(filepath.Join(root, "stdout"))
		if err != nil {
			return err
		}
		state.fileDiagnostics, err = os.Create(filepath.Join(root, "stderr"))
		if err != nil {
			_ = state.fileOutput.Close()
			return err
		}
		state.boundary.Stdout, state.boundary.Stderr = state.fileOutput, state.fileDiagnostics
		driver.stops = append(driver.stops, func() { _ = state.fileOutput.Close(); _ = state.fileDiagnostics.Close() })
	}
	return nil
}

func (driver *Driver) executeTUIArguments(ctx context.Context, args []string) error {
	state := driver.tuiArguments
	if state == nil {
		return errors.New("TUI arguments were not prepared")
	}
	if state.surface != "" && driver.mode != contract.Unit {
		return driver.executeTUINativeSurface(ctx, args)
	}
	if driver.mode == contract.E2E {
		if err := driver.executeCompiledTUIArguments(ctx, state, args); err != nil {
			return err
		}
	} else if err := driver.executeEmbeddedTUIArguments(ctx, state, args); err != nil {
		return err
	}

	driver.output, driver.errorOutput = state.output.String(), state.diagnostics.String()
	return nil
}

func (driver *Driver) requestTUIArgumentMistake(ctx context.Context, mistake string) error {
	arguments := map[string][]string{
		"an interval of 999 milliseconds": {intervalFlagName, "999ms"}, "an interval of zero": {intervalFlagName, "0s"},
		"a negative interval": {intervalFlagName, "-1s"}, "a malformed source filter": {sourceFlagName, "/private"},
		"a malformed tag filter": {"--tag", "bad"}, "both invalid source and tag filters": {sourceFlagName, "/private", "--tag", "bad"},
	}
	args, ok := arguments[mistake]
	if !ok {
		return fmt.Errorf("unknown canonical TUI mistake %q", mistake)
	}
	return driver.executeTUIArguments(ctx, append([]string{tuiCommandName, configFlag, tuiMissingConfigName}, args...))
}

func (driver *Driver) requireTUIArgumentNoObservation() error {
	state := driver.tuiArguments
	if state == nil {
		return errors.New("TUI invocation has no effect recorder")
	}
	if driver.mode == contract.E2E {
		_, err := os.Stat(filepath.Join(state.root, "state"))
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("early public refusal changed coordination root: %w", err)
		}
		return nil
	}
	if state.providers != (tuiCLIProviders{}) {
		return fmt.Errorf("early refusal observed providers: %+v", state.providers)
	}
	return nil
}

func (driver *Driver) requireTUIArgumentNoEntry() error {
	if err := driver.requireTUIArgumentNoObservation(); err != nil {
		return err
	}
	state := driver.tuiArguments
	if state.sessions != 0 || state.capabilities != 0 {
		return fmt.Errorf("validation entered capabilities/session: %d/%d", state.capabilities, state.sessions)
	}
	return nil
}

func (driver *Driver) requireTUIArgumentRefusal() error {
	if driver.exitCode != 2 || driver.output != "" || !strings.Contains(driver.errorOutput, "hippo.args.invalid") {
		return fmt.Errorf("TUI validation refusal %d/%q/%q", driver.exitCode, driver.output, driver.errorOutput)
	}
	return driver.requireTUIArgumentNoEntry()
}

func (driver *Driver) requireTUISourcePrecedence() error {
	if err := driver.requireTUIArgumentRefusal(); err != nil {
		return err
	}
	if !strings.Contains(driver.errorOutput, sourceFlagName) || strings.Contains(driver.errorOutput, "--tag") {
		return fmt.Errorf("source precedence changed: %q", driver.errorOutput)
	}
	return nil
}

func (driver *Driver) prepareTUISurface(surface string) error {
	if err := driver.prepareTUIArguments(); err != nil {
		return err
	}
	state := driver.tuiArguments
	state.surface = surface
	switch surface {
	case tuiSurfaceInput:
		state.inputTTY = false
	case tuiSurfaceOutputName:
		state.outputTTY = false
	case tuiSurfaceDumb:
		state.term = "dumb"
	default:
		return fmt.Errorf("unknown terminal surface %q", surface)
	}
	return nil
}

func (driver *Driver) requestTUIPlain(ctx context.Context) error {
	return driver.executeTUIArguments(ctx, []string{tuiCommandName, configFlag, tuiMissingConfigName})
}

func (driver *Driver) requireTUITerminalRefusal() error {
	if err := driver.requireTUIArgumentNoObservation(); err != nil {
		return err
	}
	state := driver.tuiArguments
	if driver.exitCode != 2 || driver.output != "" || !strings.Contains(driver.errorOutput, "hippo.args.invalid") || state.sessions != 0 {
		return fmt.Errorf("terminal refusal changed outcome or entered session: %d/%q/%q sessions=%d", driver.exitCode, driver.output, driver.errorOutput, state.sessions)
	}
	return nil
}

func (driver *Driver) prepareTUIInspectionFailure() error {
	if err := driver.prepareTUIArguments(); err != nil {
		return err
	}
	state := driver.tuiArguments
	if driver.mode == contract.E2E {
		state.inspectionFailure = true
		return nil
	}
	if driver.mode == contract.Integration {
		closed, err := os.Open(os.DevNull)
		if err != nil {
			return err
		}
		if err := closed.Close(); err != nil {
			return err
		}
		state.boundary.Stdin = closed
		state.boundary.TerminalCapabilities = func() (bool, bool, error) {
			state.capabilities++
			return tui.TerminalCapabilities(closed, state.fileOutput)
		}
	} else {
		state.capabilityError = errors.New("/private/unchecked capability failure")
	}
	return nil
}

func (driver *Driver) requireTUIInspectionFailure() error {
	if err := driver.requireTUIArgumentNoObservation(); err != nil {
		return err
	}
	s := driver.tuiArguments
	if driver.exitCode != 125 || s.sessions != 0 || (driver.mode != contract.E2E && s.capabilities != 1) || strings.Contains(driver.errorOutput, "/private/") || !strings.Contains(driver.errorOutput, "hippo.supervision.failed") {
		return fmt.Errorf("inspection failure was not safely classified: %d/%q", driver.exitCode, driver.errorOutput)
	}
	return nil
}

func (driver *Driver) prepareTUIRestoredFault() error {
	if driver.mode != contract.Unit {
		return errors.New("native restoration failure binding awaits actual owned terminal fixture; no model fallback")
	}
	if err := driver.prepareTUIArguments(); err != nil {
		return err
	}
	driver.tuiArguments.jsonFault = true
	return nil
}

func (driver *Driver) requestTUIJSONFault(ctx context.Context) error {
	return driver.executeTUIArguments(ctx, []string{outputFlag, outputJSONValue, tuiCommandName})
}

func (driver *Driver) requireTUIRestoredBeforeDiagnostic() error {
	s := driver.tuiArguments
	if s == nil || !s.restored || s.diagnosticBeforeRestore || s.sessions != 1 {
		return errors.New("CLI diagnostic preceded runner restoration return")
	}
	return nil
}

func (driver *Driver) requireTUIJSONFailureBody() error {
	lines := strings.Split(strings.TrimSpace(driver.errorOutput), "\n")
	if len(lines) != 2 {
		return fmt.Errorf("want one diagnostic/one machine failure body: %q", driver.errorOutput)
	}
	var body struct {
		SchemaVersion int    `json:"schemaVersion"`
		Command       string `json:"command"`
		ExitCode      int    `json:"exitCode"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &body); err != nil {
		return err
	}
	if driver.exitCode != 125 || body.SchemaVersion != 1 || body.Command != "hippo tui" || body.ExitCode != 125 {
		return fmt.Errorf("wrong TUI JSON failure: %+v exit=%d", body, driver.exitCode)
	}
	return nil
}

func (driver *Driver) requireTUINoJSONFrames() error {
	if driver.output != "" || strings.Contains(driver.errorOutput, "\"schemaVersion\":5") {
		return errors.New("TUI failure emitted status frames")
	}
	return nil
}

func (driver *Driver) executeCompiledTUIArguments(ctx context.Context, state *tuiArgumentState, args []string) (result error) {
	if driver.binary == "" {
		return errors.New("public TUI arguments require selected stamped candidate")
	}
	ctx, cancel := context.WithTimeout(ctx, usageAttemptBound)
	defer cancel()
	command := exec.CommandContext(ctx, driver.binary, args...)
	command.Dir, command.Env = state.root, []string{"HIPPO_ROOT=" + filepath.Join(state.root, "state"), "HOME=" + state.root, isolatedPath, "TERM=" + state.term, "NO_COLOR=1"}
	command.Stdin, command.Stdout, command.Stderr = strings.NewReader(""), &state.output, &state.diagnostics
	if state.inspectionFailure {
		input, err := publicTUIInspectionInput(state)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, input.Close()) }()
		command.Stdin = input
	}
	err := command.Run()
	driver.exitCode = 0
	if err != nil {
		var exit *exec.ExitError
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !errors.As(err, &exit) {
			return err
		}
		driver.exitCode = exit.ExitCode()
	}
	return nil
}

// Keep stdin open across exec: Go checkfds reopens a closed standard fd as
// /dev/null. The native boundary returns an owned inspection-error descriptor.
// Linux keeps its hung-up PTY slave; Darwin uses an anonymous socket input.
func publicTUIInspectionInput(state *tuiArgumentState) (*os.File, error) {
	master, slave, err := openNativeTUIPTY()
	if err != nil {
		return nil, err
	}
	before, beforeError := publicTUITermios(int(slave.Fd()))
	if beforeError != nil {
		return nil, errors.Join(beforeError, master.Close(), slave.Close())
	}
	input, err := prepareNativeTUIInspectionSlave(master, slave)
	if err != nil {
		return nil, errors.Join(err, master.Close(), slave.Close())
	}
	if input != slave {
		if err := slave.Close(); err != nil {
			return nil, errors.Join(err, master.Close(), input.Close())
		}
		slave = input
	}
	if err := master.Close(); err != nil {
		return nil, errors.Join(err, slave.Close())
	}
	flags, validError := unix.FcntlInt(slave.Fd(), unix.F_GETFD, 0)
	_, inspectionError := publicTUITermios(int(slave.Fd()))
	state.inspectionEvidence = fmt.Sprintf("owned inspection fd=%d valid=%t flags=%d originalMode=%+v ioctlError=%v", slave.Fd(), validError == nil, flags, before, inspectionError)
	if validError != nil || inspectionError == nil || errors.Is(inspectionError, unix.ENOTTY) || errors.Is(inspectionError, unix.ENODEV) {
		return nil, errors.Join(fmt.Errorf("public inspection fixture lacks a genuine supervision errno: %s", state.inspectionEvidence), validError, slave.Close())
	}
	return slave, nil
}

// PublicTUIInspectionEvidence retains only actual owned-descriptor observations.
func (driver *Driver) PublicTUIInspectionEvidence() string {
	if driver.tuiArguments == nil {
		return ""
	}
	return driver.tuiArguments.inspectionEvidence
}

func (driver *Driver) executeEmbeddedTUIArguments(ctx context.Context, state *tuiArgumentState, args []string) error {
	state.boundary.Environment = []string{"TERM=" + state.term}
	driver.exitCode, _ = state.boundary.Run(ctx, args)
	if state.fileOutput != nil {
		for _, file := range []*os.File{state.fileOutput, state.fileDiagnostics} {
			if err := file.Sync(); err != nil {
				return err
			}
		}
		out, err := os.ReadFile(state.fileOutput.Name())
		if err != nil {
			return err
		}
		diagnostic, err := os.ReadFile(state.fileDiagnostics.Name())
		if err != nil {
			return err
		}
		state.output.Write(out)
		state.diagnostics.Write(diagnostic)
	}
	return nil
}
