package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

type configOptions struct {
	configPath       string
	requestedProfile string
}

type versionOptions struct {
	jsonOutput bool
}

type statusOptions struct {
	configOptions

	jsonOutput bool
	diskPath   string
}

type monitorOptions struct {
	configOptions

	diskPath string
	interval time.Duration
}

type runOptions struct {
	configOptions

	command      []string
	class        string
	workingDir   string
	diskPath     string
	leasePort    int
	leaseOwner   string
	leaseMinimum int
	leaseMaximum int
}

type releaseCheckOptions struct {
	configOptions

	diskPath string
}

type releaseAssessOptions struct {
	configOptions

	summaryPath string
}

type releaseMonitorOptions struct {
	configOptions

	outputPath     string
	summaryPath    string
	deploymentRoot string
	healthURL      string
	routedOrigin   string
	servicePorts   []int
	durationMs     int64
}

func (application Application) rootCommand(execution *commandExecution) *cobra.Command {
	command := &cobra.Command{
		Use:   "resource-guard",
		Short: "Protect local development work from resource pressure",
		Long:  "Resource Guard admits, supervises, and sheds local development work from host resource evidence.",
	}

	command.AddCommand(
		application.versionCommand(execution),
		application.statusCommand(execution),
		application.monitorCommand(execution),
		application.runCommand(execution),
		application.releaseCommand(execution),
	)

	return command
}

func addConfigFlags(command *cobra.Command, options *configOptions) {
	command.Flags().StringVar(&options.configPath, "config", "", "strict local JSON configuration")
	command.Flags().StringVar(&options.requestedProfile, "profile", "", "requested resource profile")
}

func (application Application) versionCommand(execution *commandExecution) *cobra.Command {
	options := versionOptions{}
	command := &cobra.Command{
		Use:   "version",
		Short: "Print build version information",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return executeHandler(execution, func() (int, error) {
				return application.version(options)
			})
		},
	}
	command.Flags().BoolVar(&options.jsonOutput, "json", false, "emit JSON")

	return command
}

func (application Application) statusCommand(execution *commandExecution) *cobra.Command {
	options := statusOptions{}
	command := &cobra.Command{
		Use:   "status",
		Short: "Inspect current resource evidence",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return executeHandler(execution, func() (int, error) {
				return application.status(options)
			})
		},
	}
	command.Flags().BoolVar(&options.jsonOutput, "json", false, "emit JSON")
	command.Flags().StringVar(&options.diskPath, "disk-path", ".", "path whose free space is measured")
	addConfigFlags(command, &options.configOptions)

	return command
}

func (application Application) monitorCommand(execution *commandExecution) *cobra.Command {
	options := monitorOptions{}
	command := &cobra.Command{
		Use:   "monitor",
		Short: "Monitor resource-state transitions",
		RunE: func(_ *cobra.Command, _ []string) error {
			return executeHandler(execution, func() (int, error) {
				return application.monitor(options)
			})
		},
	}
	command.Flags().StringVar(&options.diskPath, "disk-path", ".", "path whose free space is measured")
	command.Flags().DurationVar(&options.interval, "interval", time.Second, "sample interval")
	addConfigFlags(command, &options.configOptions)

	return command
}

func requireGuardedCommand(command *cobra.Command, arguments []string) error {
	separator := command.ArgsLenAtDash()
	if separator < 0 || separator >= len(arguments) {
		return errors.New("run requires -- followed by a command")
	}
	if separator != 0 {
		return fmt.Errorf("unknown run arguments: %v", arguments[:separator])
	}

	return nil
}

func (application Application) runCommand(execution *commandExecution) *cobra.Command {
	options := runOptions{}
	command := &cobra.Command{
		Use:   "run -- <command> [arguments...]",
		Short: "Run a command under resource supervision",
		Args:  requireGuardedCommand,
		RunE: func(_ *cobra.Command, arguments []string) error {
			options.command = append([]string{}, arguments...)

			return executeHandler(execution, func() (int, error) {
				return application.run(options)
			})
		},
	}
	command.Flags().StringVar(&options.class, "class", "ephemeral", "task class")
	command.Flags().StringVar(&options.workingDir, "cwd", "", "child working directory")
	command.Flags().StringVar(&options.diskPath, "disk-path", "", "path whose free space is measured")
	command.Flags().IntVar(&options.leasePort, "lease-port", 0, "service port to lease")
	command.Flags().StringVar(&options.leaseOwner, "lease-owner", "", "service port owner")
	command.Flags().IntVar(&options.leaseMinimum, "lease-min", 0, "minimum allowed leased port")
	command.Flags().IntVar(&options.leaseMaximum, "lease-max", 0, "maximum allowed leased port")
	addConfigFlags(command, &options.configOptions)

	return command
}

func (application Application) releaseCommand(execution *commandExecution) *cobra.Command {
	command := &cobra.Command{
		Use:   "release",
		Short: "Check and monitor release resource safety",
	}
	command.AddCommand(
		application.releaseCheckCommand(execution),
		application.releaseAssessCommand(execution),
		application.releaseMonitorCommand(execution),
	)

	return command
}

func (application Application) releaseCheckCommand(execution *commandExecution) *cobra.Command {
	options := releaseCheckOptions{}
	command := &cobra.Command{
		Use:   "check",
		Short: "Check release admission and stability",
		RunE: func(_ *cobra.Command, _ []string) error {
			return executeHandler(execution, func() (int, error) {
				return application.releaseCheck(options)
			})
		},
	}
	command.Flags().StringVar(&options.diskPath, "disk-path", ".", "deployment path")
	addConfigFlags(command, &options.configOptions)

	return command
}

func (application Application) releaseAssessCommand(execution *commandExecution) *cobra.Command {
	options := releaseAssessOptions{}
	command := &cobra.Command{
		Use:   "assess",
		Short: "Assess a release evidence summary",
		RunE: func(_ *cobra.Command, _ []string) error {
			return executeHandler(execution, func() (int, error) {
				return application.releaseAssess(options)
			})
		},
	}
	command.Flags().StringVar(&options.summaryPath, "summary", "", "summary JSON path")
	addConfigFlags(command, &options.configOptions)

	return command
}

func (application Application) releaseMonitorCommand(execution *commandExecution) *cobra.Command {
	environment := environmentMap(application.Environment)
	options := releaseMonitorOptions{
		healthURL:    environment["RESOURCE_GUARD_HEALTH_URL"],
		routedOrigin: environment["RESOURCE_GUARD_ROUTED_ORIGIN"],
	}
	command := &cobra.Command{
		Use:   "monitor",
		Short: "Capture release overlap evidence",
		RunE: func(_ *cobra.Command, _ []string) error {
			return executeHandler(execution, func() (int, error) {
				return application.releaseMonitor(options)
			})
		},
	}
	command.Flags().StringVar(&options.outputPath, "output", "", "sample output")
	command.Flags().StringVar(&options.summaryPath, "summary", "", "summary output")
	command.Flags().StringVar(&options.deploymentRoot, "deployment-root", "", "deployment root")
	command.Flags().Int64Var(&options.durationMs, "duration-ms", 0, "optional duration in milliseconds")
	command.Flags().StringVar(&options.healthURL, "health-url", options.healthURL, "local health URL")
	command.Flags().StringVar(&options.routedOrigin, "routed-origin", options.routedOrigin, "bare HTTPS routed origin")
	command.Flags().IntSliceVar(&options.servicePorts, "service-port", nil, "service port included in RSS accounting; repeatable")
	addConfigFlags(command, &options.configOptions)

	return command
}
