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
	source     string
	tags       []string
}

type watchOptions struct {
	statusOptions

	interval time.Duration
}

type historyOptions struct {
	jsonOutput   bool
	jsonLines    bool
	since        string
	source       string
	tags         []string
	taskClass    string
	resourceTier string
	outcome      string
}

type monitorOptions struct {
	configOptions

	jsonOutput bool
	diskPath   string
	interval   time.Duration
}

type runOptions struct {
	configOptions

	command                []string
	class                  string
	workingDir             string
	diskPath               string
	leasePort              int
	leaseOwner             string
	leaseMinimum           int
	leaseMaximum           int
	reserveCPU             int
	reserveMemoryMiB       int64
	resourceTier           string
	source                 string
	tags                   []string
	concurrencyEnvironment []string
	waitForAdmission       time.Duration
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
		Use:   "hippo",
		Short: "Protect local development work from resource pressure",
		Long:  "HIPPO — Host Infrastructure Pressure & Process Orchestrator — admits, supervises, and sheds local development work from host resource evidence.",
	}

	command.AddCommand(
		application.versionCommand(execution),
		application.statusCommand(execution),
		application.watchCommand(execution),
		application.historyCommand(execution),
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
		RunE: func(command *cobra.Command, _ []string) error {
			return executeHandler(command, execution, func() (int, error) {
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
		RunE: func(command *cobra.Command, _ []string) error {
			return executeHandler(command, execution, func() (int, error) {
				return application.status(command.Context(), options)
			})
		},
	}
	command.Flags().BoolVar(&options.jsonOutput, "json", false, "emit JSON")
	command.Flags().StringVar(&options.diskPath, "disk-path", ".", "path whose free space is measured")
	command.Flags().StringVar(&options.source, "source", "", "filter owner and waiter rows by source")
	command.Flags().StringArrayVar(&options.tags, "tag", nil, "filter owner and waiter rows by key=value; repeatable")
	addConfigFlags(command, &options.configOptions)

	return command
}

func (application Application) watchCommand(execution *commandExecution) *cobra.Command {
	options := watchOptions{}
	command := &cobra.Command{
		Use:   "watch",
		Short: "Watch resource, admission, and queue transitions",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return executeHandler(command, execution, func() (int, error) {
				return application.watch(command.Context(), options)
			})
		},
	}
	command.Flags().DurationVar(&options.interval, "interval", 5*time.Second, "minimum interval between snapshots")
	command.Flags().BoolVar(&options.jsonOutput, "json", false, "emit one JSON object per changed snapshot")
	command.Flags().StringVar(&options.diskPath, "disk-path", ".", "path whose free space is measured")
	command.Flags().StringVar(&options.source, "source", "", "filter owner and waiter rows by source")
	command.Flags().StringArrayVar(&options.tags, "tag", nil, "filter owner and waiter rows by key=value; repeatable")
	addConfigFlags(command, &options.configOptions)

	return command
}

func (application Application) historyCommand(execution *commandExecution) *cobra.Command {
	options := historyOptions{}
	command := &cobra.Command{
		Use:   "history",
		Short: "Query bounded shared run summaries",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return executeHandler(command, execution, func() (int, error) {
				return application.history(options)
			})
		},
	}
	command.Flags().StringVar(&options.since, "since", "30d", "rolling duration such as 12h or 30d")
	command.Flags().StringVar(&options.source, "source", "", "filter by source")
	command.Flags().StringArrayVar(&options.tags, "tag", nil, "filter by key=value; repeatable")
	command.Flags().StringVar(&options.taskClass, "class", "", "filter by task class")
	command.Flags().StringVar(&options.resourceTier, "resource-tier", "", "filter by resource tier")
	command.Flags().StringVar(&options.outcome, "outcome", "", "filter by outcome")
	command.Flags().BoolVar(&options.jsonOutput, "json", false, "emit one JSON document")
	command.Flags().BoolVar(&options.jsonLines, "jsonl", false, "emit one JSON object per row")

	return command
}

func (application Application) monitorCommand(execution *commandExecution) *cobra.Command {
	options := monitorOptions{}
	command := &cobra.Command{
		Use:   "monitor",
		Short: "Monitor resource-state transitions",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return executeHandler(command, execution, func() (int, error) {
				return application.monitor(command.Context(), options)
			})
		},
	}
	command.Flags().StringVar(&options.diskPath, "disk-path", ".", "path whose free space is measured")
	command.Flags().DurationVar(&options.interval, "interval", time.Second, "sample interval")
	command.Flags().BoolVar(&options.jsonOutput, "json", false, "emit one JSON object per transition")
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
		RunE: func(command *cobra.Command, arguments []string) error {
			options.command = append([]string{}, arguments...)

			return executeHandler(command, execution, func() (int, error) {
				return application.run(command.Context(), options)
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
	command.Flags().IntVar(&options.reserveCPU, "reserve-cpu", 0, "fixed CPU reservation; zero selects an automatic fair share")
	command.Flags().Int64Var(&options.reserveMemoryMiB, "reserve-memory-mib", 0, "fixed memory reservation in MiB; zero selects an automatic fair share")
	command.Flags().StringVar(&options.resourceTier, "resource-tier", "", "resource tier: light, standard, or heavy")
	command.Flags().StringVar(&options.source, "source", "", "privacy-safe source label overriding hippo.identity.json")
	command.Flags().StringArrayVar(&options.tags, "tag", nil, "privacy-safe key=value label; repeatable")
	command.Flags().DurationVar(
		&options.waitForAdmission,
		"wait-for-admission",
		0,
		"legacy schema-2 queue deadline; schema 3 uses the selected resource tier",
	)
	command.Flags().StringArrayVar(
		&options.concurrencyEnvironment,
		"concurrency-env",
		nil,
		"child environment variable that receives resolved concurrency; repeatable",
	)
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
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return executeHandler(command, execution, func() (int, error) {
				return application.releaseCheck(command.Context(), options)
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
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return executeHandler(command, execution, func() (int, error) {
				return application.releaseAssess(command.Context(), options)
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
		healthURL:    environment["HIPPO_HEALTH_URL"],
		routedOrigin: environment["HIPPO_ROUTED_ORIGIN"],
	}
	command := &cobra.Command{
		Use:   "monitor",
		Short: "Capture release overlap evidence",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return executeHandler(command, execution, func() (int, error) {
				return application.releaseMonitor(command.Context(), options)
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
