package support

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/cli"
	"github.com/wahidyankf/hippo/internal/status"
	"github.com/wahidyankf/hippo/tests/contract"
)

// usageAttempt is one invocation that must be refused as a usage mistake,
// with what it printed and the status it returned.
type usageAttempt struct {
	arguments []string
	exitCode  int
	stdout    string
	stderr    string
}

const (
	usageChildOutput      = "payload-ran"
	completionCommandName = "completion"
	unknownSubcommand     = "bogus"
	intervalFlagName      = "--interval"
	sourceFlagName        = "--source"
	leaseOwnerFlagName    = "--lease-owner"
	leasePortFlagName     = "--lease-port"
	watchCommandName      = "watch"
	missingGuardedCommand = "/nonexistent/guarded-command"
	failureBodyMarker     = `"schemaVersion"`
	usageAttemptBound     = 30 * time.Second
	leaseOwnerName        = "svc"
	tagFlagName           = "--tag"
	sinceFlagName         = "--since"
	outputJSONValue       = "json"
	releaseCheckName      = "check"
)

// commandGroupMistakes are invocations of a command that only groups other
// commands. Naming no subcommand, or one that does not exist, is a usage
// mistake: it must not print help to stdout and report success.
func commandGroupMistakes() [][]string {
	return [][]string{
		{releaseCommandName},
		{releaseCommandName, unknownSubcommand},
		{completionCommandName},
		{completionCommandName, unknownSubcommand},
	}
}

// invalidFlagValues are invocations whose flags parse but whose values the
// command cannot accept. Each is the caller's mistake, found before any work
// starts, so each is a usage mistake and never HIPPO's own failure.
func invalidFlagValues() [][]string {
	payload := []string{"--", shellPath, "-c", "printf " + usageChildOutput}

	invocations := [][]string{
		append([]string{runCommandName, taskClassFlag, unknownSubcommand}, payload...),
		append([]string{runCommandName, leasePortFlagName, "-1", leaseOwnerFlagName, leaseOwnerName}, payload...),
		append([]string{runCommandName, leasePortFlagName, "8080", leaseOwnerFlagName, leaseOwnerName}, payload...),
		append([]string{
			runCommandName, leasePortFlagName, "8080", leaseOwnerFlagName, "Bad Owner",
			"--lease-min", "8000", "--lease-max", "9000",
		}, payload...),
		append([]string{runCommandName, tagFlagName, "no-equals-sign"}, payload...),
		append([]string{runCommandName, "--resource-tier", unknownSubcommand}, payload...),
		append([]string{runCommandName, sourceFlagName, "Not A Source"}, payload...),
		{monitorCommandName, intervalFlagName, "0s"},
	}
	return invocations
}

func (driver *Driver) attemptUsage(arguments []string) (usageAttempt, error) {
	root, err := driver.temporaryRoot()
	if err != nil {
		return usageAttempt{}, err
	}
	attempt := usageAttempt{arguments: arguments}
	environment := []string{"HIPPO_ROOT=" + root, "HOME=" + root, "PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	// watch and monitor run until cancelled once they accept their arguments,
	// so an invocation that should have been refused and was not would hang
	// the suite. The bound turns that into an ordinary failed assertion.
	ctx, cancel := context.WithTimeout(context.Background(), usageAttemptBound)
	defer cancel()
	if driver.mode == contract.E2E {
		command := exec.CommandContext(ctx, driver.binary, arguments...)
		command.Env = environment
		command.Dir = root
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		command.Stdout, command.Stderr = stdout, stderr
		runError := command.Run()
		attempt.stdout, attempt.stderr = stdout.String(), stderr.String()
		exitError := &exec.ExitError{}
		switch {
		case runError == nil:
		case errors.As(runError, &exitError):
			attempt.exitCode = exitError.ExitCode()
		default:
			return usageAttempt{}, runError
		}

		return attempt, nil
	}

	// Configuration and identity are discovered from the process's working
	// directory, and this checkout tracks both. The scenario must not inherit
	// them, so the in-process run happens from the scratch root; scenarios run
	// one at a time, so the change is not observed by another.
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return usageAttempt{}, err
	}
	if err = os.Chdir(root); err != nil {
		return usageAttempt{}, err
	}
	defer func() { _ = os.Chdir(workingDirectory) }()
	attempt.exitCode, _ = (cli.Application{
		Stdout: stdout, Stderr: stderr, Environment: environment,
	}).Run(ctx, arguments)
	attempt.stdout, attempt.stderr = stdout.String(), stderr.String()

	return attempt, nil
}

func (driver *Driver) attemptEach(invocations [][]string) error {
	driver.usageAttempts = nil
	for _, arguments := range invocations {
		attempt, err := driver.attemptUsage(arguments)
		if err != nil {
			return err
		}
		driver.usageAttempts = append(driver.usageAttempts, attempt)
	}

	return nil
}

func (driver *Driver) requestCommandGroupMistakes() error {
	return driver.attemptEach(commandGroupMistakes())
}

func (driver *Driver) requestInvalidFlagValues() error {
	return driver.attemptEach(invalidFlagValues())
}

func requireArgsInvalid(attempt usageAttempt) error {
	if attempt.exitCode != status.CallerError ||
		!strings.Contains(attempt.stderr, "hippo: ["+string(status.CodeArgsInvalid)+"]") {
		return fmt.Errorf("%q was not refused as a usage mistake: exit=%d stdout=%q stderr=%q",
			strings.Join(attempt.arguments, " "), attempt.exitCode, attempt.stdout, attempt.stderr)
	}

	return nil
}

func (driver *Driver) requireCommandGroupMistakesRefused() error {
	if len(driver.usageAttempts) == 0 {
		return errors.New("no command group invocation was attempted")
	}
	for _, attempt := range driver.usageAttempts {
		if err := requireArgsInvalid(attempt); err != nil {
			return err
		}
		if attempt.stdout != "" || !strings.Contains(attempt.stderr, usageBlockMarker) {
			return fmt.Errorf("%q must print its usage on stderr and nothing on stdout: stdout=%q stderr=%q",
				strings.Join(attempt.arguments, " "), attempt.stdout, attempt.stderr)
		}
	}

	return nil
}

func (driver *Driver) requireInvalidFlagValuesRefused() error {
	if len(driver.usageAttempts) == 0 {
		return errors.New("no invalid flag value was attempted")
	}
	for _, attempt := range driver.usageAttempts {
		if err := requireArgsInvalid(attempt); err != nil {
			return err
		}
		if strings.Contains(attempt.stdout, usageChildOutput) {
			return fmt.Errorf("%q started its payload before refusing it", strings.Join(attempt.arguments, " "))
		}
	}

	return nil
}

// requestChildArgumentsThatLookLikeGlobalFlags gives a guarded command its own
// --output and --color. They are the child's arguments, and HIPPO must not
// read them as its own: the missing command makes HIPPO report a failure,
// which is where it would show.
func (driver *Driver) requestChildArgumentsThatLookLikeGlobalFlags() error {
	return driver.attemptEach([][]string{{
		runCommandName, diskPathFlag, ".", "--", missingGuardedCommand, outputFlag, outputJSONValue, "--color", "always",
	}})
}

func (driver *Driver) requireChildArgumentsIgnored() error {
	if len(driver.usageAttempts) == 0 {
		return errors.New("no guarded command was attempted")
	}
	attempt := driver.usageAttempts[0]
	switch {
	case attempt.exitCode == 0:
		return fmt.Errorf("a missing guarded command exited 0: stderr=%q", attempt.stderr)
	case strings.Contains(attempt.stderr, "\x1b"):
		return fmt.Errorf("the child's --color coloured HIPPO's diagnostic: stderr=%q", attempt.stderr)
	case strings.Contains(attempt.stderr, failureBodyMarker):
		return fmt.Errorf("the child's --output added HIPPO's failure body: stderr=%q", attempt.stderr)
	case !strings.HasPrefix(attempt.stderr, "hippo: ["):
		return fmt.Errorf("HIPPO wrote no diagnostic of its own: stderr=%q", attempt.stderr)
	}

	return nil
}

// requestReleaseMonitorRawOutputJSON names json as the raw sample file. It is
// release monitor's own flag, so it asks for no body, and the missing inputs
// make the command refuse before it samples anything.
func (driver *Driver) requestReleaseMonitorRawOutputJSON() error {
	return driver.attemptEach([][]string{{releaseCommandName, monitorCommandName, outputFlag, outputJSONValue}})
}

func (driver *Driver) requireNoFailureBody() error {
	if len(driver.usageAttempts) == 0 {
		return errors.New("no release monitor invocation was attempted")
	}
	for _, attempt := range driver.usageAttempts {
		if err := requireArgsInvalid(attempt); err != nil {
			return err
		}
		if strings.Contains(attempt.stderr, failureBodyMarker) {
			return fmt.Errorf("%q wrote a failure body it was never asked for: stderr=%q",
				strings.Join(attempt.arguments, " "), attempt.stderr)
		}
	}

	return nil
}

// requestHistoryMistakesWithJSON places --output json on either side of the
// command name. Both placements are documented, so both must describe the
// same failure the same way.
func (driver *Driver) requestHistoryMistakesWithJSON() error {
	return driver.attemptEach([][]string{
		{outputFlag, outputJSONValue, historyCommandName, sinceFlagName, "nope"},
		{historyCommandName, sinceFlagName, "nope", outputFlag, outputJSONValue},
		{outputFlag + "=json", historyCommandName, unexpectedArgument},
	})
}

func (driver *Driver) requireHistoryNamedInEveryBody() error {
	if len(driver.usageAttempts) == 0 {
		return errors.New("no history invocation was attempted")
	}
	for _, attempt := range driver.usageAttempts {
		index := strings.Index(attempt.stderr, "\n{")
		if index < 0 {
			return fmt.Errorf("%q wrote no failure body: stderr=%q", strings.Join(attempt.arguments, " "), attempt.stderr)
		}
		body := struct {
			Command string `json:"command"`
		}{}
		if err := json.NewDecoder(strings.NewReader(attempt.stderr[index+1:])).Decode(&body); err != nil {
			return fmt.Errorf("%q wrote an unreadable failure body: %w", strings.Join(attempt.arguments, " "), err)
		}
		if body.Command != "hippo history" {
			return fmt.Errorf("%q named %q as the command", strings.Join(attempt.arguments, " "), body.Command)
		}
	}

	return nil
}
