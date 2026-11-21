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
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/cli"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/bootstrap"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/internal/status"
	"github.com/wahidyankf/hippo/tests/contract"
)

// publicCLIAddState is the scenario state the public command boundary steps
// share. Reset clears it with the rest of the driver.
type publicCLIAddState struct {
	rootVersionText      string
	rootVersionExit      int
	versionText          string
	versionExit          int
	colourValues         map[string]string
	colourRemoved        []string
	faultingCollector    bool
	waits                []time.Duration
	unknownProfileSource string
	cwdDirectory         string
	childMarker          string
	conformanceBinary    string
	configurationPaths   map[string]string
	endpointEnvironment  map[string]string
	identityEnvironment  map[string]string
	capturedMonitor      *application.MonitorConfig
	servicePort          int
}

// faultingCollector stands for a host sampler that faults in the middle of a
// reading, which no compiled binary can be made to do.
type faultingCollector struct{}

func (faultingCollector) Collect(context.Context, policy.CPUState, string) (policy.Reading, error) {
	panic("injected host sampler fault")
}

func (driver *Driver) publicCLIAddBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^a HIPPO build made from source without release stamps$`, driver.sourceBuild),
		step(`^JSON version is requested of that build$`, driver.jsonVersionOfSourceBuild),
		step(`^the version reports dev with the commit unknown$`, driver.requireDevVersion),
		step(`^the root version flag is requested$`, driver.rootVersionFlagRequested),
		step(`^its output matches the version command and exits 0$`, driver.requireRootVersionMatchesCommand),
		step(`^(Bash|Fish|PowerShell) completion is requested$`, driver.completionRequested),
		step(`^a (Bash|Fish|PowerShell) completion script is emitted$`, driver.requireCompletionScript),
		step(`^history is requested for a window of (\d+) days$`, driver.historyWindowRequested),
		step(`^history is requested with both --json and --jsonl$`, driver.historyJSONAndJSONLRequested),
		step(`^history is requested for thirty days from a source that matches no run$`, driver.historyFromUnmatchedSource),
		step(`^it exits 1 with nothing on standard output$`, driver.requireExitOneWithoutOutput),
		step(`^JSON Lines history is requested for thirty days$`, driver.jsonLinesHistoryRequested),
		step(`^each history line is one summary object$`, driver.requireOneSummaryObjectPerLine),
		step(`^the colour environment is (.+)$`, driver.setColourEnvironment),
		step(`^a usage mistake is reported with --color (auto|always|never)$`, driver.usageMistakeWithColour),
		step(`^the diagnostic is (coloured|plain)$`, driver.requireDiagnosticColour),
		step(`^the failure body's error field is (--[a-z]+|absent)$`, driver.requireFailureBodyField),
		step(`^the JSON failure body parses and holds no escape byte$`, driver.requireUncolouredFailureBody),
		step(`^a host sampler that faults during a reading$`, driver.faultingSampler),
		step(`^JSON status is requested with that sampler$`, driver.jsonStatusWithFaultingSampler),
		step(`^it exits 2 naming hippo\.internal\.failure$`, driver.requireInternalFailure),
		step(`^the compiled hippo-conformance tool$`, driver.conformanceBinaryRequested),
		step(`^hippo-conformance is invoked with (no argument|two manifests)$`, driver.conformanceUsageRequested),
		step(`^it exits 2 with the conformance usage line and nothing on stdout$`, driver.requireConformanceUsage),
	}
}

// sourceBuild makes the binary the scenario runs a build from source: the
// command-line package's own defaults, with no release stamps. In process, the
// default identity is already that build's; the compiled binary is rebuilt
// without its harness stamp.
func (driver *Driver) sourceBuild() error {
	if driver.mode != contract.E2E {
		return nil
	}
	binary, err := driver.buildUnstampedPackage("./cmd/hippo", "hippo")
	if err != nil {
		return err
	}
	driver.binary = binary

	return nil
}

// buildUnstampedPackage compiles one package of the working tree with no
// release or harness stamp, into a directory the driver owns, and returns the
// binary's path. The package is a path relative to the repository root.
func (driver *Driver) buildUnstampedPackage(pkg, name string) (string, error) {
	repository, err := toolRoot()
	if err != nil {
		return "", err
	}
	directory, err := driver.temporaryRoot()
	if err != nil {
		return "", err
	}
	goBinary := os.Getenv("HIPPO_GO_BINARY")
	if goBinary == "" {
		goBinary = "go"
	}
	binary := filepath.Join(directory, name)
	build := exec.Command(goBinary, "build", "-p=1", "-trimpath", "-o", binary, pkg) //nolint:gosec // HIPPO_GO_BINARY is the harness input naming the Go toolchain, as the binary builder reads it.
	build.Dir = repository
	build.Env = append(os.Environ(), "GOMAXPROCS=2")
	if output, buildError := build.CombinedOutput(); buildError != nil {
		return "", fmt.Errorf("build %s without stamps: %w: %s", pkg, buildError, output)
	}

	return binary, nil
}

// conformanceBinaryRequested compiles the consumer conformance tool for the
// end-to-end run. In process, the tool's own entry point is called instead.
func (driver *Driver) conformanceBinaryRequested() error {
	if driver.mode != contract.E2E {
		return nil
	}
	binary, err := driver.buildUnstampedPackage("./cmd/hippo-conformance", "hippo-conformance")
	if err != nil {
		return err
	}
	driver.addCLI.conformanceBinary = binary

	return nil
}

// conformanceUsageRequested runs the conformance tool with the named arguments.
// Its usage refusal happens before any service call, so the in-process entry
// point needs no service.
func (driver *Driver) conformanceUsageRequested(arguments string) error {
	var operands []string
	if arguments == "two manifests" {
		operands = []string{"first.json", "second.json"}
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := 0
	if driver.mode == contract.E2E {
		command := exec.Command(driver.addCLI.conformanceBinary, operands...)
		command.Stdout, command.Stderr = stdout, stderr
		if err := command.Run(); err != nil {
			exitError := &exec.ExitError{}
			if !errors.As(err, &exitError) {
				return err
			}
			code = exitError.ExitCode()
		}
	} else {
		code = cli.ConformanceMain(context.Background(), operands, stdout, stderr, application.ConformanceService{})
	}
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()

	return nil
}

func (driver *Driver) requireConformanceUsage() error {
	const usage = "usage: hippo-conformance <manifest.json>"
	if driver.exitCode != status.CallerError || driver.output != "" || !strings.Contains(driver.errorOutput, usage) {
		return fmt.Errorf("exit=%d stdout=%q stderr=%q, want exit 2 with the usage line and no stdout", driver.exitCode, driver.output, driver.errorOutput)
	}

	return nil
}

func (driver *Driver) jsonVersionOfSourceBuild() error {
	return driver.runCLI(versionCommandName, jsonFlag)
}

func (driver *Driver) requireDevVersion() error {
	var payload struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
	}
	if err := json.Unmarshal([]byte(driver.output), &payload); err != nil {
		return err
	}
	if driver.exitCode != 0 || payload.Version != "dev" || payload.Commit != "unknown" {
		return fmt.Errorf("source build version: exit=%d payload=%+v", driver.exitCode, payload)
	}

	return nil
}

// rootVersionFlagRequested runs the root --version flag and then the version
// command, and keeps both texts for the comparison.
func (driver *Driver) rootVersionFlagRequested() error {
	if err := driver.runCLI("--version"); err != nil {
		return err
	}
	driver.addCLI.rootVersionText = driver.output
	driver.addCLI.rootVersionExit = driver.exitCode
	if err := driver.runCLI(versionCommandName); err != nil {
		return err
	}
	driver.addCLI.versionText = driver.output
	driver.addCLI.versionExit = driver.exitCode

	return nil
}

func (driver *Driver) requireRootVersionMatchesCommand() error {
	if driver.addCLI.rootVersionExit != 0 || driver.addCLI.versionExit != 0 {
		return fmt.Errorf("version exits: root=%d command=%d", driver.addCLI.rootVersionExit, driver.addCLI.versionExit)
	}
	if driver.addCLI.rootVersionText == "" || driver.addCLI.rootVersionText != driver.addCLI.versionText {
		return fmt.Errorf("root version %q does not match the version command %q", driver.addCLI.rootVersionText, driver.addCLI.versionText)
	}

	return nil
}

func (driver *Driver) completionRequested(shell string) error {
	return driver.runCLI("completion", strings.ToLower(shell))
}

// completionMarkers holds, per shell, a line only that shell's generated script
// contains, so a script for another shell cannot pass.
var completionMarkers = map[string]string{
	"Bash":       "complete -o default -F __start_hippo hippo",
	"Fish":       "complete -c hippo",
	"PowerShell": "Register-ArgumentCompleter -CommandName 'hippo'",
}

func (driver *Driver) requireCompletionScript(shell string) error {
	marker := completionMarkers[shell]
	if driver.exitCode != 0 || driver.errorOutput != "" || !strings.Contains(driver.output, marker) {
		return fmt.Errorf("%s completion exit=%d stderr=%q: stdout lacks %q", shell, driver.exitCode, driver.errorOutput, marker)
	}

	return nil
}

// runIsolatedCLI runs one invocation against the history root the scenario
// staged, or an empty root of its own, so no run on the host is read.
func (driver *Driver) runIsolatedCLI(arguments ...string) error {
	root := driver.leaseRoot
	if root == "" {
		created, err := driver.temporaryRoot()
		if err != nil {
			return err
		}
		root = created
		driver.leaseRoot = root
	}
	if driver.mode == contract.E2E {
		return driver.runBinaryInRoot(root, arguments...)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	// A refused invocation is an outcome the scenario asserts, so its exit status
	// is recorded, not returned as a step failure.
	code, _ := bootstrap.WithDefaults(cli.Application{
		Stdout: stdout, Stderr: stderr, Environment: []string{hippoRootEnvironment + "=" + root},
	}).Run(context.Background(), arguments)
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()

	return nil
}

func (driver *Driver) historyWindowRequested(days string) error {
	return driver.runIsolatedCLI(historyCommandName, sinceFlagName, days+"d")
}

func (driver *Driver) historyJSONAndJSONLRequested() error {
	return driver.runIsolatedCLI(historyCommandName, sinceFlagName, historyWindow, jsonFlag, "--jsonl")
}

func (driver *Driver) historyFromUnmatchedSource() error {
	return driver.runIsolatedCLI(historyCommandName, sinceFlagName, historyWindow, "--source", "no-such-source")
}

func (driver *Driver) requireExitOneWithoutOutput() error {
	if driver.exitCode != 1 || driver.output != "" {
		return fmt.Errorf("exit=%d stdout=%q stderr=%q, want exit 1 and no stdout", driver.exitCode, driver.output, driver.errorOutput)
	}

	return nil
}

func (driver *Driver) jsonLinesHistoryRequested() error {
	return driver.runIsolatedCLI(historyCommandName, sinceFlagName, historyWindow, "--jsonl")
}

// requireOneSummaryObjectPerLine holds the JSON Lines output to the staged
// summary: one line, and that line is one JSON object for that run.
func (driver *Driver) requireOneSummaryObjectPerLine() error {
	if driver.exitCode != 0 {
		return fmt.Errorf("history exit=%d stderr=%q", driver.exitCode, driver.errorOutput)
	}
	lines := strings.Split(strings.TrimSuffix(driver.output, "\n"), "\n")
	if len(lines) != 1 {
		return fmt.Errorf("want one line, got %d: %q", len(lines), driver.output)
	}
	var row historyRow
	if err := json.Unmarshal([]byte(lines[0]), &row); err != nil {
		return fmt.Errorf("line is not one JSON object: %w", err)
	}
	if row.RunID != recordedOutcomeRun {
		return fmt.Errorf("line describes run %q, want %q", row.RunID, recordedOutcomeRun)
	}

	return nil
}

// colourEnvironmentName and noColourEnvironmentName are the colour variables the
// colour scenarios set or remove.
const (
	colourEnvironmentName   = "HIPPO_COLOR"
	noColourEnvironmentName = "NO_COLOR"
	terminalEnvironmentName = "TERM"
	terminalType            = "xterm-256color"
)

// colourEnvironments names the environment each phrase of a colour scenario
// sets, and the variables it removes, so the scenario never reads the runner's
// own terminal settings.
var colourEnvironments = map[string]struct {
	set     map[string]string
	removed []string
}{
	"HIPPO_COLOR=1 and TERM set": {
		set:     map[string]string{colourEnvironmentName: "1", terminalEnvironmentName: terminalType},
		removed: []string{noColourEnvironmentName},
	},
	"HIPPO_COLOR=1, TERM set, and NO_COLOR set": {
		set: map[string]string{colourEnvironmentName: "1", terminalEnvironmentName: terminalType, noColourEnvironmentName: "1"},
	},
	"HIPPO_COLOR=1 and TERM unset": {
		set:     map[string]string{colourEnvironmentName: "1"},
		removed: []string{terminalEnvironmentName, noColourEnvironmentName},
	},
	"HIPPO_COLOR unset and TERM set": {
		set:     map[string]string{terminalEnvironmentName: terminalType},
		removed: []string{colourEnvironmentName, noColourEnvironmentName},
	},
	"no colour variables": {
		removed: []string{colourEnvironmentName, terminalEnvironmentName, noColourEnvironmentName},
	},
}

func (driver *Driver) setColourEnvironment(phrase string) error {
	environment, known := colourEnvironments[phrase]
	if !known {
		return fmt.Errorf("unknown colour environment %q", phrase)
	}
	driver.addCLI.colourValues = environment.set
	driver.addCLI.colourRemoved = environment.removed

	return nil
}

// usageMistakeWithColour runs an unknown flag with the given colour setting. The
// unknown flag is a usage mistake, so its diagnostic is the one line colour applies to.
func (driver *Driver) usageMistakeWithColour(colour string) error {
	arguments := []string{annotationColorFlag, colour, "--no-such-flag"}
	if driver.mode != contract.E2E {
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		code, _ := bootstrap.WithDefaults(cli.Application{
			Stdout: stdout, Stderr: stderr, Environment: environmentPairs(driver.addCLI.colourValues),
		}).Run(context.Background(), arguments)
		driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()

		return nil
	}
	environment := filterEnvironment(environmentWith(driver.addCLI.colourValues), driver.addCLI.colourRemoved)
	command := exec.Command(driver.binary, arguments...)
	command.Env = environment
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command.Stdout, command.Stderr = stdout, stderr
	err := command.Run()
	driver.exitCode, driver.output, driver.errorOutput = 0, stdout.String(), stderr.String()
	if exitError := (&exec.ExitError{}); errors.As(err, &exitError) {
		driver.exitCode = exitError.ExitCode()
	}

	return nil
}

func environmentPairs(values map[string]string) []string {
	pairs := make([]string, 0, len(values))
	for name, value := range values {
		pairs = append(pairs, name+"="+value)
	}

	return pairs
}

// filterEnvironment drops every entry whose name is in removed.
func filterEnvironment(environment, removed []string) []string {
	kept := make([]string, 0, len(environment))
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if !contains(removed, name) {
			kept = append(kept, entry)
		}
	}

	return kept
}

func contains(values []string, want string) bool {
	return slices.Contains(values, want)
}

func (driver *Driver) requireDiagnosticColour(want string) error {
	if driver.exitCode != status.CallerError {
		return fmt.Errorf("exit=%d stderr=%q, want a usage mistake", driver.exitCode, driver.errorOutput)
	}
	coloured := strings.Contains(driver.errorOutput, "\x1b[")
	if coloured != (want == "coloured") {
		return fmt.Errorf("diagnostic coloured=%t, want %s: %q", coloured, want, driver.errorOutput)
	}

	return nil
}

// failureBodyLine returns the JSON failure body on stderr, the one line that
// starts with an object brace.
func (driver *Driver) failureBodyLine() (string, error) {
	for line := range strings.SplitSeq(driver.errorOutput, "\n") {
		if strings.HasPrefix(line, "{") {
			return line, nil
		}
	}

	return "", fmt.Errorf("no JSON failure body on stderr: %q", driver.errorOutput)
}

func (driver *Driver) requireFailureBodyField(want string) error {
	line, err := driver.failureBodyLine()
	if err != nil {
		return err
	}
	var body struct {
		Error map[string]any `json:"error"`
	}
	if err = json.Unmarshal([]byte(line), &body); err != nil {
		return err
	}
	field, present := body.Error["field"]
	if want == "absent" {
		if present {
			return fmt.Errorf("error.field is %v, want it absent: %s", field, line)
		}

		return nil
	}
	if !present || field != want {
		return fmt.Errorf("error.field is %v (present=%t), want %s: %s", field, present, want, line)
	}

	return nil
}

func (driver *Driver) requireUncolouredFailureBody() error {
	line, err := driver.failureBodyLine()
	if err != nil {
		return err
	}
	if strings.Contains(line, "\x1b") {
		return fmt.Errorf("the failure body carries an escape byte: %q", line)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err = json.Unmarshal([]byte(line), &body); err != nil {
		return fmt.Errorf("the failure body is not JSON: %w", err)
	}
	if body.Error.Code != string(status.CodeArgsInvalid) {
		return fmt.Errorf("failure body code %q, want %s", body.Error.Code, status.CodeArgsInvalid)
	}

	return nil
}

func (driver *Driver) faultingSampler() error {
	driver.addCLI.faultingCollector = true

	return nil
}

// jsonStatusWithFaultingSampler runs status in process with a sampler that
// panics. Only the in-process boundary can inject that fault.
func (driver *Driver) jsonStatusWithFaultingSampler() error {
	root, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code, _ := bootstrap.WithDefaults(cli.Application{
		Stdout: stdout, Stderr: stderr, Environment: []string{hippoRootEnvironment + "=" + root},
		Collector: faultingCollector{}, Sleep: func(time.Duration) {},
	}).Run(context.Background(), []string{statusCommandName, jsonFlag, diskPathFlag, "."})
	driver.exitCode, driver.output, driver.errorOutput = code, stdout.String(), stderr.String()

	return nil
}

func (driver *Driver) requireInternalFailure() error {
	if driver.exitCode != status.CallerError || !strings.Contains(driver.errorOutput, "hippo: ["+string(status.CodeInternalFailure)+"]") {
		return fmt.Errorf("exit=%d stderr=%q, want exit 2 naming %s", driver.exitCode, driver.errorOutput, status.CodeInternalFailure)
	}

	return nil
}
