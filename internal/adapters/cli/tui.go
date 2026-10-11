package cli

import (
	"context"
	"errors"
	"time"

	"github.com/spf13/cobra"
	app "github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/status"
)

type tuiOptions struct {
	configOptions

	diskPath string
	source   string
	tags     []string
	interval time.Duration
}

// WithTUIDefaults supplies refreshed composition callbacks while preserving caller overrides.
// Run applies these defaults after composition, so mutating outer inputs cannot retain stale captures.
func (application Application) WithTUIDefaults(
	capabilities func() (inputTTY, outputTTY bool, err error),
	runner func(context.Context, app.ObservationRequest, time.Duration, bool) (status.Interruption, error),
) Application {
	application.defaultTerminalCapabilities = capabilities
	application.defaultRunTUI = runner
	return application
}

func (application Application) tuiCommand(execution *commandExecution) *cobra.Command {
	options := tuiOptions{}
	command := &cobra.Command{
		Use:   "tui",
		Short: "Inspect shared work and machine capacity in a keyboard-driven terminal",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return executeHandler(command, execution, func() (int, error) {
				return application.tui(command.Context(), options, execution)
			})
		},
	}
	command.Flags().DurationVar(&options.interval, "interval", 5*time.Second, "refresh delay after collection; minimum 1s")
	command.Flags().StringVar(&options.diskPath, "disk-path", ".", "path whose free space is measured")
	command.Flags().StringVar(&options.source, "source", "", "filter owner and waiter rows by source")
	command.Flags().StringArrayVar(&options.tags, "tag", nil, "filter owner and waiter rows by key=value; repeatable")
	addConfigFlags(command, &options.configOptions)
	return command
}

func (application Application) tui(ctx context.Context, options tuiOptions, execution *commandExecution) (int, error) {
	if options.interval < time.Second {
		return 0, status.Failure{Code: status.CodeArgsInvalid, Field: "--interval", Message: "--interval must be at least 1s"}
	}
	request, err := app.PrepareObservation(app.StatusRequest{ConfigPath: options.configPath, DiskPath: options.diskPath, RequestedProfile: options.requestedProfile(), Source: options.source, Tags: options.tags})
	if err != nil {
		return 0, err
	}
	if environmentMap(application.Environment)["TERM"] == "dumb" {
		return 0, unusableTUITerminal()
	}
	if application.TerminalCapabilities == nil {
		return 0, status.Fail(status.CodeSupervisionFailed, "TUI terminal capabilities are unavailable")
	}
	inputTTY, outputTTY, capabilityError := application.TerminalCapabilities()
	if capabilityError != nil {
		return 0, status.Fail(status.CodeSupervisionFailed, "cannot inspect TUI terminal capabilities")
	}
	if !inputTTY || !outputTTY {
		return 0, unusableTUITerminal()
	}
	if application.RunTUI == nil {
		return 0, status.Fail(status.CodeSupervisionFailed, "TUI terminal runner is unavailable")
	}
	interruption, runError := application.RunTUI(ctx, request, options.interval, execution.colour)
	execution.interruption = interruption
	if runError != nil {
		if _, classified := errors.AsType[status.Failure](runError); classified {
			return 1, runError
		}
		if (ctx.Err() != nil || interruption.Signal != 0) && errors.Is(runError, context.Canceled) {
			return 1, runError
		}
		return 1, status.Fail(status.CodeSupervisionFailed, "TUI terminal session failed")
	}
	return 0, nil
}

func unusableTUITerminal() status.Failure {
	return status.Fail(status.CodeArgsInvalid, "TUI requires terminal input and output with TERM other than dumb; use hippo status or hippo watch")
}
