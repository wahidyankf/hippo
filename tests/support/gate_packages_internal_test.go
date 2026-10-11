package support

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// A task worktree below the checkout carries its own go.mod, so the go tool
// treats it as another module that no pattern here can reach. The wiring scan
// must judge this module's packages only, or every checkout holding a
// worktree fails the quick gate on packages it never touched.
func TestPackagesWithTestsLeavesOutNestedModules(t *testing.T) {
	root := t.TempDir()
	for _, file := range []string{
		"go.mod",
		"internal/owned/owned_test.go",
		"worktrees/task/go.mod",
		"worktrees/task/internal/owned/owned_test.go",
		"tools/nested/go.mod",
		"tools/nested/tool_test.go",
		"tools/shared/shared_test.go",
	} {
		path := filepath.Join(root, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package fixture\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	packages, err := packagesWithTests(root)
	if err != nil {
		t.Fatal(err)
	}

	slices.Sort(packages)
	want := []string{"./internal/owned", "./tools/shared"}
	if !slices.Equal(packages, want) {
		t.Fatalf("packages with tests = %v, want %v", packages, want)
	}
}

func TestGateCommandWordsSeparateQuotedRegexFromShellEffects(t *testing.T) {
	for _, line := range []string{
		"go test -skip '^(TestUnitBehaviours|TestFeatureCompliance)$' ./internal/adapters/tui",
		"HIPPO_BDD_SUITE=e2e HIPPO_BIN=\"$temporary_dir/hippo\" \"$go_binary\" test -run '^TestE2EBehaviours$' ./internal/adapters/tui",
	} {
		if _, valid := gateCommandWords(line); !valid {
			t.Fatalf("literal gate command rejected: %s", line)
		}
	}
	for _, suffix := range []string{" | cat", " || true", " &", "; true", " > result", " -skip \"$SELECTOR\"", " -run $(echo Test)"} {
		if _, valid := gateCommandWords("go test ./internal/adapters/tui" + suffix); valid {
			t.Fatalf("operator/dynamic selector accepted: %s", suffix)
		}
	}
	words, valid := gateCommandWords(`go test -skip "^(TestUnitBehaviours\|TestIntegrationBehaviours\|TestE2EBehaviours\|TestFeatureCompliance)$" ./internal/adapters/tui`)
	if !valid || gateOrdinarySelection(words, gateCorpusEntries()) {
		t.Fatal("double-quoted regex escapes changed into alternatives")
	}
	for _, row := range []struct{ command, want string }{
		{`go test -run ^$ ./internal/adapters/tui`, `^$`},
		{`go test -run "^$" ./internal/adapters/tui`, `^$`},
		{`go test -run "^TestFeatureCompliance$" ./internal/adapters/tui`, `^TestFeatureCompliance$`},
		{`go test -run "^(First\|Second)$" ./internal/adapters/tui`, `^(First\|Second)$`},
		{`go test -run "^Test\$" ./internal/adapters/tui`, `^Test$`},
	} {
		words, valid := gateCommandWords(row.command)
		if !valid || gateFlag(words, "-run") != row.want {
			t.Fatalf("literal dollar/regex bytes changed for %s: %v", row.command, words)
		}
	}
	for _, selector := range []string{
		`$SELECTOR`, `${SELECTOR}`, `$(printf Test)`, `$((1+1))`, `$[1+1]`,
		`$?`, `$$`, `$1`, `$@`, `$#`, `$!`, `$-`, `$*`, `$_`,
	} {
		for _, value := range []string{selector, `"` + selector + `"`} {
			if _, valid := gateCommandWords("go test -run " + value + " ./internal/adapters/tui"); valid {
				t.Fatalf("true shell expansion accepted: %s", value)
			}
		}
	}
	for _, value := range []string{`$'^Test$'`, `$"^Test$"`, `"` + "`printf Test`" + `"`} {
		if _, valid := gateCommandWords("go test -run " + value + " ./internal/adapters/tui"); valid {
			t.Fatalf("unsupported dollar quoting or command substitution accepted: %s", value)
		}
	}
}

func TestGateSerialComplianceRejectsRemovedOrNonexecutingRegistration(t *testing.T) {
	lines := make([]string, 0, 3)
	for _, adapter := range []string{"unit", "integration", "e2e"} {
		lines = append(lines, "HIPPO_BDD_ADAPTER="+adapter+" go test -count=1 -run '^TestFeatureCompliance$' ./internal/adapters/tui")
	}
	positive := strings.Join(lines, "\n")
	if !validSerialComposedCompliance(positive) {
		t.Fatal("three serial composed static checks rejected")
	}
	for _, line := range lines {
		for _, replacement := range []string{"", "# " + line, line + " || true", strings.Replace(line, "./internal/adapters/tui", "./tests/bdd", 1), strings.Replace(line, "^TestFeatureCompliance$", "^TestUnitBehaviours$", 1)} {
			if validSerialComposedCompliance(strings.Replace(positive, line, replacement, 1)) {
				t.Fatalf("removed/nonstatic registration accepted: %s", replacement)
			}
		}
	}
}

func TestGateTestPatternsRejectCompileOnlyAndSkipAll(t *testing.T) {
	flags := make([]string, 0, 37)
	flags = append(flags, "-run '^$'", "-test.run=^$", "--test.run=^$", "-skip '.*'", "--test.skip=.*")
	for _, prefix := range []string{"-", "--", "-test.", "--test."} {
		for _, pattern := range []string{".*", "^.*", ".*$", "^.*$"} {
			flags = append(flags, prefix+"skip='"+pattern+"'", prefix+"skip '"+pattern+"'")
		}
	}
	for _, flag := range flags {
		patterns := fixtureGatePatterns(t, flag)
		if len(patterns) != 0 {
			t.Fatalf("nonexecuting pattern counted for %s: %v", flag, patterns)
		}
	}
	for _, prefix := range []string{"-", "--", "-test.", "--test."} {
		for _, pattern := range []string{"^TestFeatureCompliance$", "^(TestUnitBehaviours|TestIntegrationBehaviours|TestE2EBehaviours|TestFeatureCompliance)$"} {
			for _, flag := range []string{prefix + "skip='" + pattern + "'", prefix + "skip '" + pattern + "'"} {
				patterns := fixtureGatePatterns(t, flag)
				if !slices.Equal(patterns, []string{"./internal/adapters/tui", "./internal/adapters/tui", "./internal/adapters/tui"}) {
					t.Fatalf("healthy bounded skip rejected for %s: %v", flag, patterns)
				}
			}
		}
	}
}

func fixtureGatePatterns(t *testing.T, flag string) []string {
	t.Helper()
	root := t.TempDir()
	for _, script := range testRunningScripts {
		path := filepath.Join(root, script)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("go test "+flag+" ./internal/adapters/tui\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	patterns, err := gatedTestPatterns(root)
	if err != nil {
		t.Fatal(err)
	}
	return patterns
}
