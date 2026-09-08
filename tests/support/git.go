package support

import (
	"os"
	"os/exec"
	"strings"
)

// gitEnvironmentPrefix names the variables Git uses to redirect a command away
// from the directory it was started in.
const gitEnvironmentPrefix = "GIT_"

// GitCommand builds a Git invocation bound to directory and to nothing else.
//
// A pre-push hook running inside a linked worktree exports GIT_DIR, and Git
// prefers that over both the working directory and -C. A fixture that creates
// its own repository therefore re-initializes the repository under test and
// commits onto its checked-out branch instead. Every fixture here is correct
// when a person runs it and destructive when the hook does, so the isolation
// has to be established in the environment rather than in the path.
func GitCommand(directory string, arguments ...string) *exec.Cmd {
	return gitCommandWithEnvironment(os.Environ(), directory, arguments...)
}

// gitCommandWithEnvironment is GitCommand over a supplied environment, so a
// behavior can hand it the variables a hook exports and observe the result.
func gitCommandWithEnvironment(environment []string, directory string, arguments ...string) *exec.Cmd {
	command := exec.Command("git", arguments...)
	command.Dir = directory
	command.Env = withoutGitEnvironment(environment)

	return command
}

// withoutGitEnvironment drops every GIT_* variable rather than the handful known
// to redirect a write today. The set grows with Git, and a fixture that quietly
// followed a newly added one would fail the same way for a new reason.
func withoutGitEnvironment(environment []string) []string {
	kept := make([]string, 0, len(environment))
	for _, entry := range environment {
		if strings.HasPrefix(entry, gitEnvironmentPrefix) {
			continue
		}
		kept = append(kept, entry)
	}

	return kept
}
