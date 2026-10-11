package contract

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

type corpusFixtureDriver struct {
	bindings                  []StepBinding
	value, resets, closed     int
	givens, actions, outcomes int
}

func newCorpusFixtureDriver() *corpusFixtureDriver {
	driver := &corpusFixtureDriver{}
	driver.bindings = []StepBinding{
		{Pattern: `^a portable fixture seed$`, Handler: func() { driver.value = 10; driver.givens++ }},
		{Pattern: `^the portable fixture advances$`, Handler: func() { driver.value++; driver.actions++ }},
		{Pattern: `^the portable fixture reaches eleven$`, Handler: func() error {
			driver.outcomes++
			if driver.value != 11 {
				return errors.New("fixture action did not advance its seed")
			}
			return nil
		}},
	}
	return driver
}

func (driver *corpusFixtureDriver) Bindings() []StepBinding { return driver.bindings }
func (driver *corpusFixtureDriver) Reset()                  { driver.value = 0; driver.resets++ }
func (driver *corpusFixtureDriver) Close()                  { driver.closed++ }

func portableCorpusFixture(t *testing.T, nestedOutcome string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "explicit-corpus")
	for _, row := range []struct{ path, name, outcome string }{
		{"root.feature", "Root portable scenario", "the portable fixture reaches eleven"},
		{filepath.Join("nested", "child.feature"), "Nested portable scenario", nestedOutcome},
	} {
		path := filepath.Join(root, row.path)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		contents := "Feature: " + row.name + "\n  Scenario: " + row.name + "\n    Given a portable fixture seed\n    When the portable fixture advances\n    Then " + row.outcome + "\n"
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestSuiteOptionsAtDiscoversCompleteExplicitRecursiveCorpus(t *testing.T) {
	root := portableCorpusFixture(t, "the portable fixture reaches eleven")
	options := SuiteOptionsAt(Adapter{Name: Unit}, root)
	if !options.Strict || options.Tags != "" {
		t.Fatalf("Unit corpus options loosen strictness or filter scenarios: strict=%t tags=%q", options.Strict, options.Tags)
	}
	features, err := (godog.TestSuite{Options: options}).RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, feature := range features {
		for _, scenario := range feature.Pickles {
			names = append(names, scenario.Name)
		}
	}
	slices.Sort(names)
	want := []string{"Nested portable scenario", "Root portable scenario"}
	if len(features) != 2 || !slices.Equal(names, want) {
		t.Fatalf("explicit recursive corpus was not selected completely: paths=%q features=%d scenarios=%q; want both root and nested fixtures", options.Paths, len(features), names)
	}
}

func TestVerifyAtAcceptsCompleteExplicitRecursiveCorpus(t *testing.T) {
	root := portableCorpusFixture(t, "the portable fixture reaches eleven")
	if err := VerifyAt(Adapter{Name: Unit}, newCorpusFixtureDriver().Bindings(), root); err != nil {
		t.Fatalf("complete explicit recursive corpus was rejected: %v", err)
	}
}

func TestVerifyAtRejectsMissingExplicitCorpus(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing-corpus")
	err := VerifyAt(Adapter{Name: Unit}, newCorpusFixtureDriver().Bindings(), root)
	if err == nil || !strings.Contains(err.Error(), root) {
		t.Fatalf("missing explicit corpus was not refused at its own path: error=%v; want missing path %q", err, root)
	}
}

func TestVerifyAtDiscriminatesExplicitRecursiveBindingFailures(t *testing.T) {
	for _, defect := range []string{"undefined", "ambiguous", "nil", "unused"} {
		t.Run(defect, func(t *testing.T) {
			driver := newCorpusFixtureDriver()
			outcome := "the portable fixture reaches eleven"
			want := ""
			switch defect {
			case "undefined":
				outcome = "the nested portable fixture is undefined"
				want = `undefined behavior step "the nested portable fixture is undefined"`
			case "ambiguous":
				driver.bindings = append(driver.bindings, driver.bindings[2])
				want = `ambiguous behavior step "the portable fixture reaches eleven"`
			case "nil":
				driver.bindings[2].Handler = nil
				want = `binding "^the portable fixture reaches eleven$" requires a nonnil function handler`
			case "unused":
				driver.bindings = append(driver.bindings, StepBinding{Pattern: `^an unused portable fixture$`, Handler: func() {}})
				want = `unused behavior binding "^an unused portable fixture$"`
			}
			root := portableCorpusFixture(t, outcome)
			err := VerifyAt(Adapter{Name: Unit}, driver.Bindings(), root)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("explicit recursive %s defect was not identified: error=%v; want %q", defect, err, want)
			}
			if defect == "unused" && strings.Contains(err.Error(), `unused behavior binding "^a portable fixture seed$"`) {
				t.Fatalf("used root/nested handler was misclassified as unused: %v", err)
			}
		})
	}
}

func TestRunAtExecutesBothExplicitRootAndNestedScenarios(t *testing.T) {
	root := portableCorpusFixture(t, "the portable fixture reaches eleven")
	driver := newCorpusFixtureDriver()
	t.Run("explicit-corpus", func(t *testing.T) { RunAt(t, Adapter{Name: Unit}, driver, root) })
	if driver.resets != 2 || driver.givens != 2 || driver.actions != 2 || driver.outcomes != 2 || driver.closed != 1 {
		t.Fatalf("explicit corpus did not execute both complete scenarios with reset and cleanup: resets=%d given=%d when=%d then=%d close=%d", driver.resets, driver.givens, driver.actions, driver.outcomes, driver.closed)
	}
}
