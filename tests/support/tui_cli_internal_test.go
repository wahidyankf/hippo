package support

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/cucumber/godog"
	"github.com/wahidyankf/hippo/tests/contract"
)

func TestTUIBDDCLIHelpExecutesCanonicalScenario(t *testing.T) {
	adapter, err := contract.AdapterByName(contract.Unit)
	if err != nil {
		t.Fatal(err)
	}
	driver := NewDriver(adapter)
	defer driver.Close()
	options := contract.SuiteOptions(adapter)
	steps := canonicalTUIHelpSteps(t, options)
	bindings := driver.Bindings()
	selectedBindings := make([]contract.StepBinding, 0, len(bindings))
	for _, binding := range bindings {
		pattern, err := regexp.Compile(binding.Pattern)
		if err != nil {
			t.Fatal(err)
		}
		if slices.ContainsFunc(steps, pattern.MatchString) {
			selectedBindings = append(selectedBindings, binding)
		}
	}
	if errorsFound := contract.ValidateBindings(selectedBindings, steps); len(errorsFound) != 0 {
		t.Fatalf("canonical selected binding resolution failed: %v", errorsFound)
	}
	var transcript bytes.Buffer
	options.Output, options.TestingT, options.DefaultContext = &transcript, t, t.Context()
	result := godog.TestSuite{
		Name: "hippo-tui-help-unit",
		ScenarioInitializer: func(scenarioContext *godog.ScenarioContext) {
			scenarioContext.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
				driver.Reset()
				return ctx, nil
			})
			for _, binding := range bindings {
				scenarioContext.Step(binding.Pattern, binding.Handler)
			}
		},
		Options: options,
	}.Run()
	t.Log(transcript.String())
	if result != 0 {
		t.Fatalf("executing canonical TUI help scenario exited %d", result)
	}
}

func canonicalTUIHelpSteps(t *testing.T, options *godog.Options) []string {
	t.Helper()
	const scenarioName = "TUI help exposes observation options without opening a terminal"
	path := filepath.Join(contract.FeaturePath, "public-cli.feature")
	options.Paths = []string{path}
	features, err := (godog.TestSuite{Options: options}).RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	line := int64(0)
	for _, feature := range features {
		for _, child := range feature.Feature.Children {
			if child.Scenario != nil && child.Scenario.Name == scenarioName {
				line = child.Scenario.Location.Line
			}
		}
	}
	if line == 0 {
		t.Fatal("canonical TUI help scenario is absent")
	}
	options.Paths = []string{fmt.Sprintf("%s:%d", path, line)}
	selected, err := (godog.TestSuite{Options: options}).RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	var steps []string
	scenarios := 0
	for _, feature := range selected {
		for _, scenario := range feature.Pickles {
			if scenario.Name != scenarioName {
				t.Fatalf("selected unrelated scenario %q", scenario.Name)
			}
			scenarios++
			for _, step := range scenario.Steps {
				steps = append(steps, step.Text)
			}
		}
	}
	if scenarios != 1 || len(steps) != 6 {
		t.Fatalf("selected %d scenarios and %d steps, want the complete six-step help scenario", scenarios, len(steps))
	}
	return steps
}
