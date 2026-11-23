package support

import (
	"os"
	"strings"
	"testing"
)

// removeTemporaryPaths deletes the directories a driver created, so a test leaves no fixture repository behind.
func removeTemporaryPaths(t *testing.T, driver *Driver) {
	t.Helper()

	t.Cleanup(func() {
		for _, path := range driver.temporaryPaths {
			if err := os.RemoveAll(path); err != nil {
				t.Errorf("remove temporary fixture %s: %v", path, err)
			}
		}
	})
}

// TestFixtureGitWorkIgnoresRepositoryNamedByHook covers the removed scenario "Fixture Git work ignores the repository
// a hook names". A fixture committed while a hook names another repository lands in its own checkout, and the named
// repository keeps its head.
func TestFixtureGitWorkIgnoresRepositoryNamedByHook(t *testing.T) {
	driver := &Driver{}
	removeTemporaryPaths(t, driver)

	if err := driver.adoptHookGitEnvironment(); err != nil {
		t.Fatal(err)
	}

	if err := driver.initializeFixtureUnderHookEnvironment(); err != nil {
		t.Fatal(err)
	}

	if err := driver.requireFixtureHoldsCommit(); err != nil {
		t.Fatalf("the fixture checkout does not hold its commit: %v", err)
	}

	if err := driver.requireNamedRepositoryUnchanged(); err != nil {
		t.Fatalf("the repository named by the hook changed: %v", err)
	}
}

// TestGateScriptsClearRedirectingGitEnvironment covers the removed scenario "Gate scripts clear the redirecting Git
// environment". Every gate script the hooks and CI invoke unsets every redirecting Git variable, and a script that
// names them without unsetting them is refused.
func TestGateScriptsClearRedirectingGitEnvironment(t *testing.T) {
	driver := &Driver{}
	if err := driver.inspectGateScriptIsolation(); err != nil {
		t.Fatal(err)
	}

	if err := driver.requireGateScriptsUnsetGitEnvironment(); err != nil {
		t.Fatalf("a gate script keeps a redirecting Git variable: %v", err)
	}

	named := &Driver{gateScripts: map[string]string{}}
	for _, name := range gateScriptNames {
		named.gateScripts[name] = strings.ReplaceAll(driver.gateScripts[name], "unset GIT_DIR", "keep GIT_DIR")
	}

	if err := named.requireGateScriptsUnsetGitEnvironment(); err == nil {
		t.Fatal("gate scripts that name the Git variables without unsetting them were accepted")
	}
}
