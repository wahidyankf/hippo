package support

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// redirectingGitVariables are the environment variables that move a Git command
// off the directory it was given. A gate script must clear all of them, because
// clearing some would leave the same failure reachable by a different name.
var redirectingGitVariables = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_COMMON_DIR",
	"GIT_NAMESPACE",
	"GIT_PREFIX",
}

// gateScriptNames are the entry points a Git hook or CI job invokes directly.
// Anything they call inherits the environment they leave behind.
var gateScriptNames = []string{
	filepath.Join("scripts", "test-quick.sh"),
	filepath.Join("scripts", "test.sh"),
	filepath.Join("scripts", "build-release.sh"),
}

// adoptHookGitEnvironment stands up a repository and then names it the way a
// pre-push hook inside a linked worktree names its own: through GIT_DIR, which
// Git prefers over the working directory and over -C alike.
func (driver *Driver) adoptHookGitEnvironment() error {
	directory, err := os.MkdirTemp("", "hippo-ambient-")
	if err != nil {
		return err
	}

	driver.temporaryPaths = append(driver.temporaryPaths, directory)
	driver.ambientRepository = directory
	for _, arguments := range fixtureRepositoryCommands("ambient") {
		if output, commandError := GitCommand(directory, arguments...).CombinedOutput(); commandError != nil {
			return fmt.Errorf("prepare ambient repository: %s: %w", output, commandError)
		}
	}

	head, err := GitCommand(directory, "rev-parse", "HEAD").Output()
	if err != nil {
		return fmt.Errorf("read ambient head: %w", err)
	}

	driver.ambientHead = strings.TrimSpace(string(head))
	driver.hookEnvironment = append(os.Environ(), "GIT_DIR="+filepath.Join(directory, ".git"))

	return nil
}

// initializeFixtureUnderHookEnvironment does what every fixture in this suite
// does — create a repository of its own and commit into it — while the ambient
// repository is named in the environment.
func (driver *Driver) initializeFixtureUnderHookEnvironment() error {
	directory, err := os.MkdirTemp("", "hippo-fixture-")
	if err != nil {
		return err
	}

	driver.temporaryPaths = append(driver.temporaryPaths, directory)
	driver.fixtureCheckout = directory
	for _, arguments := range fixtureRepositoryCommands("fixture") {
		command := gitCommandWithEnvironment(driver.hookEnvironment, directory, arguments...)
		if output, commandError := command.CombinedOutput(); commandError != nil {
			return fmt.Errorf("initialize fixture checkout: %s: %w", output, commandError)
		}
	}

	return nil
}

func (driver *Driver) requireFixtureHoldsCommit() error {
	subject, err := GitCommand(driver.fixtureCheckout, "log", "-1", "--format=%s").Output()
	if err != nil {
		return fmt.Errorf("read fixture head: %w", err)
	}
	if strings.TrimSpace(string(subject)) != "fixture" {
		return fmt.Errorf("fixture checkout holds %q rather than its own commit", strings.TrimSpace(string(subject)))
	}

	return nil
}

func (driver *Driver) requireNamedRepositoryUnchanged() error {
	head, err := GitCommand(driver.ambientRepository, "rev-parse", "HEAD").Output()
	if err != nil {
		return fmt.Errorf("read ambient head: %w", err)
	}
	if strings.TrimSpace(string(head)) != driver.ambientHead {
		return fmt.Errorf("the repository named in the environment moved from %s to %s",
			driver.ambientHead, strings.TrimSpace(string(head)))
	}

	return nil
}

func (driver *Driver) inspectGateScriptIsolation() error {
	driver.gateScripts = make(map[string]string, len(gateScriptNames))
	for _, name := range gateScriptNames {
		data, err := os.ReadFile(filepath.Join(toolRoot(), name))
		if err != nil {
			return err
		}
		driver.gateScripts[name] = string(data)
	}

	return nil
}

func (driver *Driver) requireGateScriptsUnsetGitEnvironment() error {
	for _, name := range gateScriptNames {
		source, ok := driver.gateScripts[name]
		if !ok {
			return fmt.Errorf("%s was not inspected", name)
		}
		for _, variable := range redirectingGitVariables {
			if !strings.Contains(source, variable) {
				return fmt.Errorf("%s does not unset %s", name, variable)
			}
		}
		if !strings.Contains(source, "unset GIT_DIR") {
			return fmt.Errorf("%s names the variables without unsetting them", name)
		}
	}

	return nil
}

// fixtureRepositoryCommands is one commit in a repository of its own. The
// identity is supplied per invocation so the fixture works on a machine with no
// ambient Git identity without configuring one anywhere.
func fixtureRepositoryCommands(subject string) [][]string {
	return [][]string{
		{"init", "-q", "-b", "main"},
		{
			"-c", "user.name=HIPPO fixture", "-c", "user.email=fixture@example.invalid",
			"commit", "--allow-empty", "-q", "-m", subject,
		},
	}
}
