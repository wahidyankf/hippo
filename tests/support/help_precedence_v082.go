package support

import (
	"fmt"
	"strings"

	"github.com/wahidyankf/hippo/internal/status"
	"github.com/wahidyankf/hippo/tests/contract"
)

func (driver *Driver) helpPrecedenceBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^hippo is invoked as "([^"]*)"$`, driver.invokeAs),
		step(`^it prints the help of "([^"]*)" to stdout, nothing to stderr, and exits 0$`, driver.requireHelpOf),
		step(`^it exits 2 naming hippo\.args\.invalid with nothing on stdout$`, driver.requireUsageMistake),
	}
}

// invokeAs runs hippo with a whitespace-separated argument vector.
func (driver *Driver) invokeAs(arguments string) error {
	_ = driver.runCLI(strings.Fields(arguments)...)

	return nil
}

// requireHelpOf holds requested help to the payload contract: the named
// command's own usage on stdout, nothing on stderr, and a successful status.
func (driver *Driver) requireHelpOf(command string) error {
	if driver.exitCode != 0 || driver.errorOutput != "" {
		return fmt.Errorf("help exited %d with stderr %q", driver.exitCode, driver.errorOutput)
	}
	if !strings.Contains(driver.output, "Usage:\n  "+command+" ") {
		return fmt.Errorf("stdout is not the help of %q: %q", command, driver.output)
	}

	return nil
}

func (driver *Driver) requireUsageMistake() error {
	if driver.exitCode != status.CallerError || driver.output != "" ||
		!strings.Contains(driver.errorOutput, "hippo: ["+string(status.CodeArgsInvalid)+"]") {
		return fmt.Errorf("exit=%d stdout=%q stderr=%q", driver.exitCode, driver.output, driver.errorOutput)
	}

	return nil
}
