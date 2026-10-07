package support

import (
	"context"
	"fmt"
	"os/exec"
)

func runArchitecture(ctx context.Context, selected string) error {
	command := exec.CommandContext(ctx, "go", "test", "-count=1", "./tests/architecture", "-run", selected)
	command.Dir = toolRoot()
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("architecture %s: %w\n%s", selected, err, output)
	}
	return nil
}

func (driver *Driver) inspectArchitecture(ctx context.Context) error {
	driver.architectureGraphError = runArchitecture(ctx, "Production")
	driver.architectureFixtureError = runArchitecture(ctx, "Fixture")
	return nil
}

func (driver *Driver) requireArchitectureGraph() error    { return driver.architectureGraphError }
func (driver *Driver) requireArchitectureFixtures() error { return driver.architectureFixtureError }
