package support

import (
	"strings"
	"testing"
)

// lintWiringMutation edits the real .golangci.yml or scripts/test-quick.sh text so that one wiring promise is false
// while a comment or a lookalike still mentions it. The step under test must name the lie rather than read the
// mention.
type lintWiringMutation struct {
	name string
	// target is "configuration" for .golangci.yml and "command" for scripts/test-quick.sh.
	target   string
	from, to string
	step     func(*Driver) error
}

func lintWiringDriver(t *testing.T) *Driver {
	t.Helper()

	driver := &Driver{}
	if err := driver.inspectLintEnforcement(); err != nil {
		t.Fatal(err)
	}

	return driver
}

func TestLintWiringStepsPassOnTheCommittedFiles(t *testing.T) {
	driver := lintWiringDriver(t)

	if err := driver.requireNilnessAndExhaustiveMaps(); err != nil {
		t.Fatalf("the committed .golangci.yml fails the nilness and exhaustive step: %v", err)
	}

	if err := driver.requirePinnedNilAway(); err != nil {
		t.Fatalf("the committed scripts/test-quick.sh fails the NilAway step: %v", err)
	}
}

func TestLintWiringStepsRejectAPromiseOnlyMentioned(t *testing.T) {
	const (
		nilness       = "        - nilness\n"
		checkKey      = "      check:\n"
		mapItem       = "        - map\n"
		switchItem    = "        - switch\n"
		govetEnable   = "    govet:\n      enable:\n"
		nilawayPrefix = "go tool nilaway "
		nilawayLine   = "-include-pkgs=github.com/wahidyankf/hippo -pretty-print=false ./..."
	)

	nilnessStep := (*Driver).requireNilnessAndExhaustiveMaps
	nilawayStep := (*Driver).requirePinnedNilAway

	for _, mutation := range []lintWiringMutation{
		{"check key removed", "configuration", checkKey, "", nilnessStep},
		{"map item deleted", "configuration", mapItem, "", nilnessStep},
		{"switch item deleted", "configuration", switchItem, "", nilnessStep},
		{"nilness commented out", "configuration", nilness, "        # - nilness\n", nilnessStep},
		{"enable renamed disable", "configuration", govetEnable, "    govet:\n      disable:\n", nilnessStep},
		{
			"nilness enabled and disabled", "configuration", nilness,
			nilness + "      disable:\n        - nilness\n", nilnessStep,
		},
		{"nilaway failure swallowed", "command", nilawayLine, nilawayLine + " || true", nilawayStep},
		{"nilaway backgrounded", "command", nilawayLine, nilawayLine + " &", nilawayStep},
		{"nilaway followed by another command", "command", nilawayLine, nilawayLine + "; true", nilawayStep},
		{"nilaway piped", "command", nilawayLine, nilawayLine + " | cat", nilawayStep},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			driver := lintWiringDriver(t)

			switch mutation.target {
			case "configuration":
				driver.lintConfiguration = replaceOnce(t, driver.lintConfiguration, mutation.from, mutation.to)
			default:
				if !strings.Contains(driver.lintCommand, nilawayPrefix+mutation.from) {
					t.Fatalf("scripts/test-quick.sh has no %q to mutate", nilawayPrefix+mutation.from)
				}

				driver.lintCommand = replaceOnce(t, driver.lintCommand, mutation.from, mutation.to)
			}

			if err := mutation.step(driver); err == nil {
				t.Fatalf("the step passed although %s", mutation.name)
			}
		})
	}
}

// replaceOnce fails the test when from is not present exactly once, so a mutation can never silently change nothing.
func replaceOnce(t *testing.T, text, from, replacement string) string {
	t.Helper()

	if count := strings.Count(text, from); count != 1 {
		t.Fatalf("the file holds %d copies of %q, want 1", count, from)
	}

	return strings.Replace(text, from, replacement, 1)
}
