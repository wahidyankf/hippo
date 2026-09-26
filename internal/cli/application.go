package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	resourceconfig "github.com/wahidyankf/hippo/internal/config"
	"github.com/wahidyankf/hippo/internal/host"
	"github.com/wahidyankf/hippo/internal/policy"
	releaseguard "github.com/wahidyankf/hippo/internal/release"
	"github.com/wahidyankf/hippo/internal/status"
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
	// childStatus holds the status a started child produced, and is set only
	// then. Once a child has run, its status is the answer and hippo has
	// nothing to add: it is passed through exactly, with no diagnostic and no
	// body, because any reason hippo attached would be a guess about someone
	// else's program.
	childStatus *int
	// handlerRan records that a command's own handler was entered. It is what
	// separates a mistyped invocation from a command that ran and then
	// rejected its arguments: only the first deserves the usage block, and the
	// exit status alone cannot tell them apart.
	handlerRan bool
	// usage is the help block the failing command would print. It is captured
	// when the invocation is rejected so the boundary can put it after the
	// diagnostic rather than before it: a caller reading stderr should meet
	// what went wrong first, and the flag list only once it wants it.
	usage string
	// machineReadable is set when the caller asked for --output json, and is
	// what decides whether a failure carries its body. The diagnostic line is
	// never behind this flag: a caller reading stderr for the first time
	// should not have to know about a flag to find out what went wrong.
	machineReadable bool
	// colour is the rendering the invocation asked for, resolved once the
	// flags are parsed so the boundary can use it while reporting a failure.
	colour bool
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
	execution.handlerRan = true
	exitCode, err := handler()
	execution.exitCode = exitCode

	return err
}

// Run executes one hippo command and returns its process exit code.
//
// Every status hippo returns is decided here and nowhere else. The layers
// below still speak in the numbers they always used; this function is the one
// place that turns those into the closed vocabulary a caller sees, and into
// the reason that travels beside it.
//
//nolint:nonamedreturns // The deferred panic handler has to replace both results.
func (application Application) Run(ctx context.Context, arguments []string) (exitCode int, runError error) {
	application = application.defaults()
	execution := &commandExecution{}
	// Cobra propagates ExecuteContext to every RunE callback through
	// command.Context; contextcheck cannot follow that framework boundary.
	command := application.rootCommand(execution) //nolint:contextcheck // Cobra carries ExecuteContext through command.Context.

	environment := environmentMap(application.Environment)
	execution.colour = wantsColour(colourRequest(arguments), environment)
	execution.machineReadable = flagRequest(arguments, "--output") == "json"

	// A fault in hippo is still hippo's answer to give. Without this the
	// runtime writes a stack trace to stderr and exits 2 on its own terms,
	// which is a status no caller can tell from a real one; with it the caller
	// gets the same shape as every other failure.
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}

		failure := status.Fail(status.CodeInternalFailure, "hippo failed internally: %v", recovered)
		report(application.Stderr, commandPath(arguments), failure, failure.Status(), execution)
		exitCode, runError = failure.Status(), failure
	}()

	// A non-nil empty slice prevents Cobra from falling back to the test
	// process's os.Args when an injected application runs without arguments.
	command.SetArgs(append([]string{}, arguments...))
	command.SetOut(application.Stdout)
	// Cobra prints the usage block to its out writer, which is stdout. A
	// mistyped flag is not a result, so sending it there would put a flag list
	// where a consumer is reading the answer. Everything hippo says about a
	// failed invocation goes to stderr, and the root silences Cobra's own
	// rendering so this function is the only thing that writes it.
	command.SetErr(application.Stderr)
	// Cobra adds its completion command only when it executes, so add it here
	// to hold it to the same rule as every other command group. It binds its
	// output writer when it is created, so this must follow SetOut.
	command.InitDefaultCompletionCmd()
	requireSubcommands(command, execution)
	command.SilenceErrors = true
	command.SetFlagErrorFunc(func(failing *cobra.Command, flagError error) error {
		execution.usage = failing.UsageString()

		return flagError
	})

	err := command.ExecuteContext(ctx)

	if execution.childStatus != nil {
		return *execution.childStatus, nil
	}

	failure := classify(execution, err)
	// The usage block belongs to a mistyped invocation and nothing else. A
	// handler that rejects its arguments after Cobra accepted them has a
	// precise complaint to make, and burying it under a flag list would be the
	// same unhelpfulness this contract exists to remove.
	if failure != nil && execution.usage == "" && !execution.handlerRan {
		execution.usage = command.UsageString()
	}

	if failure != nil {
		report(application.Stderr, commandPath(arguments), *failure, failure.Status(), execution)
		if execution.usage != "" {
			_, _ = fmt.Fprint(application.Stderr, "\n", execution.usage)
		}
		if err == nil {
			// The guard returns no error when it sheds work against a limit,
			// because being shed is an outcome and not a fault. Reporting it
			// is this boundary's job; turning it into an error here would tell
			// every in-process caller that something went wrong.
			return failure.Status(), nil
		}

		return failure.Status(), *failure
	}

	return execution.exitCode, err
}

// colourRequest reads --color before Cobra parses anything, so a failure that
// happens during parsing is rendered the way the caller asked for.
func colourRequest(arguments []string) string {
	if value := flagRequest(arguments, "--color"); value != "" {
		return value
	}

	return "auto"
}

// flagRequest reads one persistent flag's value straight from argv. Cobra
// cannot supply it when the failure being reported is Cobra refusing to parse
// argv at all, and a caller that asked for JSON deserves it most in exactly
// that case.
func flagRequest(arguments []string, name string) string {
	for index, argument := range arguments {
		if value, found := strings.CutPrefix(argument, name+"="); found {
			return value
		}
		if argument == name && index+1 < len(arguments) {
			return arguments[index+1]
		}
	}

	return ""
}

// commandPath is the subcommand the caller asked for, for the body's command
// field. It is the leading non-flag arguments, which is what hippo's own
// commands are; it reports "hippo" when there are none.
func commandPath(arguments []string) string {
	path := []string{"hippo"}
	for _, argument := range arguments {
		if strings.HasPrefix(argument, "-") {
			break
		}
		path = append(path, argument)
	}

	return strings.Join(path, " ")
}

// Execute runs the production application. Cobra owns diagnostics so each
// command error is rendered exactly once before this function returns its code.
func Execute(ctx context.Context, arguments []string) int {
	exitCode, _ := (Application{}).Run(ctx, arguments)

	return exitCode
}
