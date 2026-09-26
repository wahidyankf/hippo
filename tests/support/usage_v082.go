package support

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

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
	payload := []string{"--", "/bin/sh", "-c", "printf " + usageChildOutput}

	return [][]string{
		append([]string{runCommandName, taskClassFlag, unknownSubcommand}, payload...),
		append([]string{runCommandName, leasePortFlagName, "-1", leaseOwnerFlagName, "svc"}, payload...),
		append([]string{runCommandName, leasePortFlagName, "8080", leaseOwnerFlagName, "svc"}, payload...),
		append([]string{
			runCommandName, leasePortFlagName, "8080", leaseOwnerFlagName, "Bad Owner",
			"--lease-min", "8000", "--lease-max", "9000",
		}, payload...),
		append([]string{runCommandName, "--tag", "no-equals-sign"}, payload...),
		append([]string{runCommandName, "--resource-tier", unknownSubcommand}, payload...),
		append([]string{runCommandName, sourceFlagName, "Not A Source"}, payload...),
		{monitorCommandName, intervalFlagName, "0s"},
	}
}

func (driver *Driver) attemptUsage(arguments []string) (usageAttempt, error) {
	root, err := driver.temporaryRoot()
	if err != nil {
		return usageAttempt{}, err
	}
	attempt := usageAttempt{arguments: arguments}
	environment := []string{"HIPPO_ROOT=" + root, "HOME=" + root, "PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	if driver.mode == contract.E2E {
		command := exec.Command(driver.binary, arguments...)
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
	}).Run(context.Background(), arguments)
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
