package support

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
	"github.com/wahidyankf/hippo/tests/contract"
)

// tuiCLIState records effects at the real command boundary; the probes never
// supply configuration, host evidence, or terminal output for the command.
type tuiCLIState struct {
	boundary            cli.Application
	output, diagnostics bytes.Buffer
	providers           tuiCLIProviders
	capabilities, runs  int
	compiledRoot        string
}

type tuiCLIProviders struct {
	configuration, host, coordination int
}

func (providers *tuiCLIProviders) Load(string, map[string]string) (application.Configuration, error) {
	providers.configuration++
	return application.Configuration{}, errors.New("unexpected TUI configuration load")
}

func (providers *tuiCLIProviders) Identity(map[string]string, string, string, []string) (identity.Value, string, error) {
	providers.configuration++
	return identity.Value{}, "", errors.New("unexpected TUI identity lookup")
}

func (providers *tuiCLIProviders) StateRoot(map[string]string) string {
	providers.configuration++
	return ""
}

func (providers *tuiCLIProviders) AbsolutePath(string) (string, error) {
	providers.configuration++
	return "", errors.New("unexpected TUI path lookup")
}

func (providers *tuiCLIProviders) IdentityPresent(map[string]string, string) bool {
	providers.configuration++
	return false
}

func (providers *tuiCLIProviders) DisplayPath(string, string) string {
	providers.configuration++
	return ""
}

func (providers *tuiCLIProviders) Collect(context.Context, policy.CPUState, string) (policy.Reading, error) {
	providers.host++
	return policy.Reading{}, errors.New("unexpected TUI host collection")
}

func (providers *tuiCLIProviders) Snapshot(context.Context, string, string) (coordination.ReservationTotals, error) {
	providers.coordination++
	return coordination.ReservationTotals{}, errors.New("unexpected TUI coordination snapshot")
}

func (driver *Driver) tuiCLIHelpBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^TUI input and output are not terminals and observation providers record calls$`, driver.prepareTUIHelp),
		step(`^TUI help is requested$`, driver.requestTUIHelp),
		step(`^help lists config disk-path profile source tag and interval options$`, driver.requireTUIHelpOptions),
		step(`^the interval default is five seconds and the disk-path default is the current directory$`, driver.requireTUIHelpDefaults),
		step(`^no configuration host coordination or terminal session is opened$`, driver.requireTUIHelpNoEffects),
		step(`^help exits 0$`, driver.requireTUIHelpSuccess),
	}
}

func (driver *Driver) prepareTUIHelp() error {
	state := &tuiCLIState{}
	state.boundary = cli.Application{
		Stdin: strings.NewReader(""), Stdout: &state.output, Stderr: &state.diagnostics,
		Environment: []string{"TERM=xterm"}, Configuration: &state.providers, Collector: &state.providers,
		Observer: application.ObservationServices{Configuration: &state.providers, Collector: &state.providers, Runtime: &state.providers},
		TerminalCapabilities: func() (bool, bool, error) {
			state.capabilities++
			return false, false, nil
		},
		RunTUI: func(context.Context, application.ObservationRequest, time.Duration, bool) (status.Interruption, error) {
			state.runs++
			return status.Interruption{}, errors.New("unexpected TUI terminal entry")
		},
	}
	driver.tuiCLI = state
	return nil
}

func (driver *Driver) requestTUIHelp(ctx context.Context) error {
	state := driver.tuiCLI
	if state == nil {
		return errors.New("TUI help has no prepared command boundary")
	}
	if driver.mode == contract.E2E {
		if driver.binary == "" {
			return errors.New("TUI help has no selected compiled candidate")
		}
		rootsBefore := len(driver.temporaryPaths)
		// The existing bounded command fixture supplies closed input, captured
		// streams and an isolated cwd/root. Missing configuration discriminates
		// help precedence from an accidental observation in the child.
		attempt, err := driver.attemptUsageContext(ctx, []string{tuiCommandName, configFlag, "missing.json", "--help"})
		if err != nil {
			return err
		}
		if len(driver.temporaryPaths) != rootsBefore+1 {
			return errors.New("compiled TUI help did not record its isolated root")
		}
		state.compiledRoot = driver.temporaryPaths[rootsBefore]
		driver.exitCode, driver.output, driver.errorOutput = attempt.exitCode, attempt.stdout, attempt.stderr
		return nil
	}
	code, err := state.boundary.Run(ctx, []string{tuiCommandName, "--help"})
	driver.exitCode, driver.output, driver.errorOutput = code, state.output.String(), state.diagnostics.String()
	return err
}

func (driver *Driver) requireTUIHelpOptions() error {
	if !strings.Contains(driver.output, "hippo tui [flags]") {
		return fmt.Errorf("executing TUI help produced no command usage: stdout=%q", driver.output)
	}
	for _, option := range []string{configFlag, diskPathFlag, "--profile", sourceFlagName, tagFlagName, intervalFlagName} {
		if !strings.Contains(driver.output, option) {
			return fmt.Errorf("executing TUI help omits %s: stdout=%q", option, driver.output)
		}
	}
	return nil
}

func (driver *Driver) requireTUIHelpDefaults() error {
	if !strings.Contains(driver.output, "(default 5s)") || !strings.Contains(driver.output, `(default ".")`) {
		return fmt.Errorf("TUI help defaults are absent or incorrect: stdout=%q", driver.output)
	}
	return nil
}

func (driver *Driver) requireTUIHelpNoEffects() error {
	state := driver.tuiCLI
	if state == nil {
		return errors.New("TUI help has no effect recorder")
	}
	if driver.mode == contract.E2E {
		// Parent probes cannot count private child calls. At this boundary the
		// evidence is successful help despite missing configuration, plain
		// captured streams and an untouched isolated state root.
		if state.compiledRoot == "" {
			return errors.New("compiled TUI help has no observed state root")
		}
		entries, err := os.ReadDir(state.compiledRoot)
		if err != nil {
			return err
		}
		if len(entries) != 0 || driver.exitCode != 0 || driver.errorOutput != "" || strings.Contains(driver.output, "\x1b") {
			return fmt.Errorf("compiled help changed state or entered a terminal: entries=%d exit=%d stdout=%q stderr=%q", len(entries), driver.exitCode, driver.output, driver.errorOutput)
		}
		return nil
	}
	if state.providers != (tuiCLIProviders{}) || state.capabilities != 0 || state.runs != 0 {
		return fmt.Errorf("TUI help opened a provider or terminal: providers=%+v capabilities=%d sessions=%d", state.providers, state.capabilities, state.runs)
	}
	return nil
}

func (driver *Driver) requireTUIHelpSuccess() error {
	if driver.exitCode != 0 || driver.errorOutput != "" {
		return fmt.Errorf("TUI help exited %d with stderr=%q", driver.exitCode, driver.errorOutput)
	}
	return nil
}
