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
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/adapters/tui"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/status"
	"github.com/wahidyankf/hippo/tests/contract"
)

const (
	tuiSurfaceHelper = "HIPPO_TUI_ARGUMENT_SURFACE_HELPER"
	tuiSurfaceRoot   = "HIPPO_TUI_ARGUMENT_SURFACE_ROOT"
)

// CheckTUIUnsupportedSurface executes the same actual CLI/public boundary and
// assertions as the canonical binding. It exposes no product testing switch.
func CheckTUIUnsupportedSurface(ctx context.Context, adapter contract.Adapter, binary, surface string) error {
	if adapter.Name != contract.Integration && adapter.Name != contract.E2E {
		return errors.New("native surface contract requires explicit Integration or E2E")
	}
	driver := NewDriver(adapter)
	defer driver.closeAnnotationNativeFixture()
	if adapter.Name == contract.E2E {
		if binary == "" || binary != os.Getenv("HIPPO_BIN") {
			return errors.New("native surface contract lacks unchanged selected candidate")
		}
		driver.binary = binary
	}
	if err := driver.prepareTUISurface(surface); err != nil {
		return err
	}
	if err := driver.requestTUIPlain(ctx); err != nil {
		return err
	}
	if err := driver.requireTUITerminalRefusal(); err != nil {
		return err
	}
	return driver.requireTUIArgumentNoObservation()
}

type (
	tuiSurfaceReport struct {
		Exit          int    `json:"Exit"`
		Output        string `json:"Output"`
		Diagnostic    string `json:"Diagnostic"`
		InputTTY      bool   `json:"InputTTY"`
		OutputTTY     bool   `json:"OutputTTY"`
		Configuration int    `json:"Configuration"`
		Host          int    `json:"Host"`
		Coordination  int    `json:"Coordination"`
		Capabilities  int    `json:"Capabilities"`
		Sessions      int    `json:"Sessions"`
	}

	tuiSurfaceOutput struct {
		file   *os.File
		record bytes.Buffer
	}
)

func (writer *tuiSurfaceOutput) Fd() uintptr { return writer.file.Fd() }
func (writer *tuiSurfaceOutput) Write(value []byte) (int, error) {
	n, err := writer.file.Write(value)
	writer.record.Write(value[:n])
	return n, err
}

// RunTUIArgumentSurfaceHelper runs the embedded CLI on a genuine owned PTY.
// Its test export lives in the external TUI suite; it never recursively runs Go.
func RunTUIArgumentSurfaceHelper(ctx context.Context) error {
	surface := os.Getenv(tuiSurfaceHelper)
	root, err := tuiSurfaceOwnedRoot(os.Getenv(tuiSurfaceRoot))
	if err != nil {
		return err
	}
	input, output := os.Stdin, os.Stdout
	switch surface {
	case tuiSurfaceInput:
		file, err := os.Open(os.DevNull)
		if err != nil {
			return err
		}
		return errors.Join(runTUIArgumentSurface(ctx, file, output, surface, root), file.Close())
	case tuiSurfaceOutputName:
		file, err := os.Create(filepath.Join(root, "invocation-stdout"))
		if err != nil {
			return err
		}
		return errors.Join(runTUIArgumentSurface(ctx, input, file, surface, root), file.Close())
	case tuiSurfaceDumb:
		return runTUIArgumentSurface(ctx, input, output, surface, root)
	default:
		return fmt.Errorf("unknown native TUI surface %q", surface)
	}
}

func tuiSurfaceOwnedRoot(root string) (string, error) {
	if root == "" || !filepath.IsAbs(root) {
		return "", errors.New("native TUI surface helper has no exact owned fixture")
	}
	temporaryBase, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		return "", fmt.Errorf("resolve native TUI temporary base: %w", err)
	}
	relative, err := filepath.Rel(temporaryBase, root)
	if err != nil || !filepath.IsLocal(relative) || filepath.Dir(relative) != "." || !strings.HasPrefix(relative, "hippo-lease-") {
		return "", errors.New("native TUI surface helper root is outside its owned temporary fixture")
	}
	return filepath.Join(temporaryBase, relative), nil
}

func runTUIArgumentSurface(ctx context.Context, input, output *os.File, surface, root string) error {
	writer := &tuiSurfaceOutput{file: output}
	inputTTY, outputTTY, err := tui.TerminalCapabilities(input, writer)
	if err != nil {
		return err
	}
	if surface == tuiSurfaceInput && (inputTTY || !outputTTY) || surface == tuiSurfaceOutputName && (!inputTTY || outputTTY) || surface == tuiSurfaceDumb && (!inputTTY || !outputTTY) {
		return fmt.Errorf("native fixture does not own exact descriptor shape: %s %t/%t", surface, inputTTY, outputTTY)
	}
	var diagnostics bytes.Buffer
	providers := &tuiCLIProviders{}
	capabilities, sessions := 0, 0
	boundary := cli.Application{Stdin: input, Stdout: writer, Stderr: &diagnostics, Environment: os.Environ(), Configuration: providers, Collector: providers, Observer: application.ObservationServices{Configuration: providers, Collector: providers, Runtime: providers}, TerminalCapabilities: func() (bool, bool, error) { capabilities++; return tui.TerminalCapabilities(input, writer) }, RunTUI: func(context.Context, application.ObservationRequest, time.Duration, bool) (status.Interruption, error) {
		sessions++
		return status.Interruption{}, errors.New("unexpected native surface terminal entry")
	}}
	code, _ := boundary.Run(ctx, []string{tuiCommandName, configFlag, filepath.Join(root, tuiMissingConfigName)})
	data, err := json.Marshal(tuiSurfaceReport{Exit: code, Output: writer.record.String(), Diagnostic: diagnostics.String(), InputTTY: inputTTY, OutputTTY: outputTTY, Configuration: providers.configuration, Host: providers.host, Coordination: providers.coordination, Capabilities: capabilities, Sessions: sessions})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "invocation-report.json"), data, 0o600)
}

func tuiSurfaceShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func tuiSurfaceScriptArguments(command string) ([]string, error) {
	switch runtime.GOOS {
	case darwinPlatform, "freebsd", "openbsd", "netbsd":
		return []string{"-q", os.DevNull, "/bin/sh", "-c", command}, nil
	case linuxPlatform:
		return []string{"-q", "-e", "-c", "/bin/sh -c " + tuiSurfaceShellQuote(command), os.DevNull}, nil
	default:
		return nil, fmt.Errorf("no actual owned TUI surface PTY transport for %s", runtime.GOOS)
	}
}

func (driver *Driver) executeTUINativeSurface(caller context.Context, args []string) error {
	state := driver.tuiArguments
	if state == nil || state.root == "" {
		return errors.New("native surface lacks owned root")
	}
	ctx, cancel := context.WithTimeout(caller, usageAttemptBound)
	defer cancel()
	script, err := exec.LookPath("script")
	if err != nil {
		return fmt.Errorf("native surface requires existing script utility: %w", err)
	}
	commandText, err := driver.tuiSurfaceCommand(args)
	if err != nil {
		return err
	}
	arguments, err := tuiSurfaceScriptArguments(commandText)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, script, arguments...)
	command.Dir = state.root
	command.Env = append(os.Environ(), "HIPPO_ROOT="+filepath.Join(state.root, "state"), "HOME="+state.root, isolatedPath, "TERM="+state.term, "NO_COLOR=1", tuiSurfaceHelper+"="+state.surface, tuiSurfaceRoot+"="+state.root)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	command.WaitDelay = time.Second
	reader, writer, err := os.Pipe()
	if err != nil {
		return err
	}
	command.Stdin = reader
	transcript, runError := command.CombinedOutput()
	closeError := errors.Join(reader.Close(), writer.Close())
	if err := tuiSurfaceTransportError(ctx, transcript, runError, closeError); err != nil {
		return err
	}
	if driver.mode == contract.Integration {
		if err := driver.readTUIEmbeddedSurfaceReport(transcript); err != nil {
			return err
		}
	} else if err := driver.readTUIPublicSurfaceReport(transcript); err != nil {
		return err
	}

	driver.output, driver.errorOutput = state.output.String(), state.diagnostics.String()
	return nil
}

func (driver *Driver) tuiSurfaceCommand(args []string) (string, error) {
	state := driver.tuiArguments
	var commandText string
	switch driver.mode {
	case contract.Integration:
		commandText = "exec " + tuiSurfaceShellQuote(os.Args[0]) + " -test.run='^TestTUIBDDArgumentSurfaceHelper$' -test.v"
	case contract.E2E:
		if driver.binary == "" || driver.binary != os.Getenv("HIPPO_BIN") {
			return "", errors.New("native public surface requires unchanged TestMain-selected candidate")
		}
		parts := append([]string{driver.binary}, args...)
		for i := range parts {
			parts[i] = tuiSurfaceShellQuote(parts[i])
		}
		// Prove both original descriptors are terminals, then change exactly one.
		commandText = "test -t 0 && test -t 1 || exit 97; " + strings.Join(parts, " ") + " 2>" + tuiSurfaceShellQuote(filepath.Join(state.root, "invocation-stderr"))
		switch state.surface {
		case tuiSurfaceInput:
			commandText += " </dev/null"
		case tuiSurfaceOutputName:
			commandText += " >" + tuiSurfaceShellQuote(filepath.Join(state.root, "invocation-stdout"))
		case tuiSurfaceDumb:
		default:
			return "", fmt.Errorf("unknown public terminal surface %q", state.surface)
		}
		commandText += "; code=$?; printf '%s\\n' \"$code\" >" + tuiSurfaceShellQuote(filepath.Join(state.root, "invocation-exit"))
	default:
		return "", errors.New("native surface cannot replace pure Unit")
	}
	return commandText, nil
}

func (driver *Driver) readTUIEmbeddedSurfaceReport(transcript []byte) error {
	state := driver.tuiArguments

	data, err := os.ReadFile(filepath.Join(state.root, "invocation-report.json"))
	if err != nil {
		return fmt.Errorf("actual embedded surface helper returned no report: %w: %q", err, transcript)
	}
	var report tuiSurfaceReport
	if err := json.Unmarshal(data, &report); err != nil {
		return err
	}
	driver.exitCode = report.Exit
	state.output.WriteString(report.Output)
	state.diagnostics.WriteString(report.Diagnostic)
	state.providers = tuiCLIProviders{configuration: report.Configuration, host: report.Host, coordination: report.Coordination}
	state.capabilities, state.sessions = report.Capabilities, report.Sessions
	return nil
}

func (driver *Driver) readTUIPublicSurfaceReport(transcript []byte) error {
	state := driver.tuiArguments

	data, err := os.ReadFile(filepath.Join(state.root, "invocation-exit"))
	if err != nil {
		return err
	}
	driver.exitCode, err = strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return err
	}
	diagnostic, err := os.ReadFile(filepath.Join(state.root, "invocation-stderr"))
	if err != nil {
		return err
	}
	state.diagnostics.Write(diagnostic)
	if state.surface == tuiSurfaceOutputName {
		out, err := os.ReadFile(filepath.Join(state.root, "invocation-stdout"))
		if err != nil {
			return err
		}
		state.output.Write(out)
		if len(transcript) != 0 {
			return fmt.Errorf("public surface emitted unexpected PTY output %q", transcript)
		}
	} else {
		state.output.Write(transcript)
	}
	return nil
}

func tuiSurfaceTransportError(ctx context.Context, transcript []byte, runError, closeError error) error {
	if err := ctx.Err(); err != nil {
		return errors.Join(fmt.Errorf("native TUI surface exceeded its bound: %w", err), closeError)
	}
	if runError != nil {
		return errors.Join(fmt.Errorf("native TUI surface transport failed: %w: %q", runError, transcript), closeError)
	}
	return closeError
}
