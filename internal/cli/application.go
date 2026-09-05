package cli

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	resourceconfig "github.com/wahidyankf/hippo/internal/config"
	"github.com/wahidyankf/hippo/internal/host"
	"github.com/wahidyankf/hippo/internal/policy"
	releaseguard "github.com/wahidyankf/hippo/internal/release"
)

const unavailableValue = "unavailable"

// Version is replaced by release builds through ldflags.
var Version = "dev"

// Commit is replaced by release builds through ldflags.
var Commit = "unknown"

// Application supplies the command's injectable host and I/O dependencies.
type Application struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	Environment    []string
	Collector      policy.Collector
	MonitorRelease func(context.Context, releaseguard.MonitorConfig) error
	Sleep          func(time.Duration)
	Now            func() time.Time
	Version        string
	Commit         string
}

type commandExecution struct {
	exitCode int
}

func environmentMap(environment []string) map[string]string {
	result := map[string]string{}

	for _, entry := range environment {
		for index := range entry {
			if entry[index] == '=' {
				result[entry[:index]] = entry[index+1:]
				break
			}
		}
	}

	return result
}

func (application Application) defaults() Application {
	if application.Stdin == nil {
		application.Stdin = os.Stdin
	}
	if application.Stdout == nil {
		application.Stdout = os.Stdout
	}
	if application.Stderr == nil {
		application.Stderr = os.Stderr
	}

	if application.Environment == nil {
		application.Environment = os.Environ()
	}

	if application.Collector == nil {
		application.Collector = host.SystemCollector{}
	}
	if application.MonitorRelease == nil {
		application.MonitorRelease = releaseguard.RunMonitor
	}

	if application.Now == nil {
		application.Now = time.Now
	}

	if application.Version == "" {
		application.Version = Version
	}
	if application.Commit == "" {
		application.Commit = Commit
	}

	return application
}

func (application Application) loadConfig(path string) (resourceconfig.Result, error) {
	environment := environmentMap(application.Environment)
	resolvedPath, explicit := resourceconfig.Path(path, environment)

	return resourceconfig.Load(resolvedPath, explicit)
}

func executeHandler(command *cobra.Command, execution *commandExecution, handler func() (int, error)) error {
	// Cobra diagnoses argument and flag mistakes before RunE runs, so every
	// failure from here on is a runtime outcome. Printing the usage block for
	// those reads as though the caller mistyped the command and buries the real
	// diagnostic under a flag list in consumer logs.
	command.SilenceUsage = true
	exitCode, err := handler()
	execution.exitCode = exitCode

	return err
}

// Run executes one hippo command and returns its process exit code.
func (application Application) Run(ctx context.Context, arguments []string) (int, error) {
	application = application.defaults()
	execution := &commandExecution{}
	// Cobra propagates ExecuteContext to every RunE callback through
	// command.Context; contextcheck cannot follow that framework boundary.
	command := application.rootCommand(execution) //nolint:contextcheck // Cobra carries ExecuteContext through command.Context.

	// A non-nil empty slice prevents Cobra from falling back to the test
	// process's os.Args when an injected application runs without arguments.
	command.SetArgs(append([]string{}, arguments...))
	command.SetOut(application.Stdout)
	command.SetErr(application.Stderr)

	err := command.ExecuteContext(ctx)
	if err != nil && execution.exitCode == 0 {
		execution.exitCode = 1
	}

	return execution.exitCode, err
}

// Execute runs the production application. Cobra owns diagnostics so each
// command error is rendered exactly once before this function returns its code.
func Execute(ctx context.Context, arguments []string) int {
	exitCode, _ := (Application{}).Run(ctx, arguments)

	return exitCode
}
