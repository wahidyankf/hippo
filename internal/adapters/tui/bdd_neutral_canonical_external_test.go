package tui_test

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/cucumber/godog"
	"github.com/wahidyankf/hippo/tests/contract"
)

// These tests execute the canonical steps through the complete composed registry.
// Undefined steps are behavioral RED; a registry-only check cannot pass them.
func TestTUIBDDRemainingNeutralCanonicalScenarios(t *testing.T) {
	cases := []struct {
		feature, scenario string
		rows, steps       int
	}{
		{"public-cli.feature", "Invalid TUI options are rejected before observation and terminal entry", 6, 18},
		{"public-cli.feature", "Source validation precedes tag validation for TUI requests", 1, 4},
		{"execution.feature", "Inherited runs preserve the original owner's display label", 1, 4},
		{"terminal.feature", "TUI refuses unsupported terminal surfaces before observation", 3, 12},
	}
	for _, test := range cases {
		t.Run(test.scenario, func(t *testing.T) {
			runRemainingNeutralScenario(t, test.feature, test.scenario, test.rows, test.steps)
		})
	}
}

func runRemainingNeutralScenario(t *testing.T, feature, name string, wantRows, wantSteps int) {
	t.Helper()
	adapter, err := contract.AdapterByName(contract.Unit)
	if err != nil {
		t.Fatal(err)
	}
	driver := newComposedBDDDriver(adapter)
	defer driver.Close()
	if err := contract.VerifyAt(adapter, driver.Bindings(), remainingCanonicalCorpus); err != nil {
		t.Fatal(err)
	}
	options := contract.SuiteOptionsAt(adapter, remainingCanonicalCorpus)
	path := filepath.Join(remainingCanonicalCorpus, feature)
	options.Paths = []string{path}
	features, err := (godog.TestSuite{Options: options}).RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	line := int64(0)
	for _, parsed := range features {
		for _, child := range parsed.Feature.Children {
			if child.Scenario != nil && child.Scenario.Name == name {
				if line != 0 {
					t.Fatalf("canonical scenario %q is ambiguous", name)
				}
				line = child.Scenario.Location.Line
			}
		}
	}
	if line == 0 {
		t.Fatalf("canonical scenario %q is absent", name)
	}
	options.Paths = []string{fmt.Sprintf("%s:%d", path, line)}
	selected, err := (godog.TestSuite{Options: options}).RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	rows, steps := 0, 0
	for _, parsed := range selected {
		for _, scenario := range parsed.Pickles {
			if scenario.Name != name {
				t.Fatalf("selected unrelated scenario %q", scenario.Name)
			}
			rows++
			steps += len(scenario.Steps)
		}
	}
	if rows != wantRows || steps != wantSteps {
		t.Fatalf("canonical selection has %d rows/%d steps, want %d/%d", rows, steps, wantRows, wantSteps)
	}
	var transcript bytes.Buffer
	options.Output, options.TestingT, options.DefaultContext = &transcript, t, t.Context()
	executedRows := 0
	result := godog.TestSuite{
		Name: "hippo-tui-remaining-unit",
		ScenarioInitializer: func(scenarioContext *godog.ScenarioContext) {
			scenarioContext.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
				driver.Reset()
				executedRows++
				return ctx, nil
			})
			for _, binding := range driver.Bindings() {
				scenarioContext.Step(binding.Pattern, binding.Handler)
			}
		},
		Options: options,
	}.Run()
	t.Log(transcript.String())
	if result != 0 || executedRows != wantRows {
		t.Fatalf("canonical execution exited %d and visited %d rows, want exit 0/%d rows", result, executedRows, wantRows)
	}
}

type r256BControl struct {
	feature, name, refusal string
	rows, steps            int
}

func r256BControls() []r256BControl {
	return []r256BControl{
		{"quality-gates.feature", "TUI verification retains model terminal and host boundaries", "e2e TUI gate inspection requires repository files outside the public binary boundary", 1, 6},
		{"public-cli.feature", "Display annotations do not change public status or watch comparison", "public observation requires owned command/frame fixture", 1, 4},
		{"tui.feature", "Completed observations remain unchanged when source values are reused", "public observation requires owned command/frame fixture", 1, 3},
		{"tui.feature", "Unknown sample time remains unavailable", "TUI e2e requires actual native/public scenario transport; private model fallback refused", 2, 6},
		{"tui.feature", "Missing host readings are distinct from accounting zero", "TUI e2e requires actual native/public scenario transport; private model fallback refused", 1, 3},
		{"tui.feature", "Synthetic unsafe display text cannot escape its cell budget", "TUI e2e requires actual native/public scenario transport; private model fallback refused", 3, 9},
	}
}

func TestTUIBDDR256BUnitCanonicalRows(t *testing.T) {
	for _, control := range r256BControls() {
		t.Run(control.name, func(t *testing.T) {
			runRemainingNeutralScenario(t, control.feature, control.name, control.rows, control.steps)
		})
	}
}

func TestTUIBDDR256BCanonicalRemovalAndRefusal(t *testing.T) {
	adapter, err := contract.AdapterByName(contract.E2E)
	if err != nil {
		t.Fatal(err)
	}
	driver := newComposedBDDDriver(adapter)
	defer driver.Close()
	// Verify the complete original recursive registry before selecting any negative.
	if err := contract.VerifyAt(adapter, driver.Bindings(), remainingCanonicalCorpus); err != nil {
		t.Fatal(err)
	}
	scenarios := r256BCanonicalScenarios(t, adapter)
	if found := contract.ValidateExemptions(adapter, scenarios); len(found) != 0 {
		t.Fatalf("complete canonical inventory: %v", found)
	}
	r256BRetainedAdapters(t, driver.Bindings())
	for _, control := range r256BControls() {
		t.Run(control.name, func(t *testing.T) {
			selected := r256BSelectedRows(t, control, adapter, scenarios)
			r256BRemovalChecks(t, control, adapter, scenarios, selected)
			r256BRunRefusal(t, control, adapter, driver)
		})
	}
	r256BMandatoryASCII(t, scenarios, adapter)
}

func r256BCanonicalScenarios(t *testing.T, adapter contract.Adapter) []contract.Scenario {
	t.Helper()
	options := contract.SuiteOptionsAt(adapter, remainingCanonicalCorpus)
	options.Tags = ""
	features, err := (godog.TestSuite{Options: options}).RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	var scenarios []contract.Scenario
	for _, feature := range features {
		for _, pickle := range feature.Pickles {
			scenario := contract.Scenario{Name: pickle.Name}
			for _, tag := range pickle.Tags {
				scenario.Tags = append(scenario.Tags, tag.Name)
			}
			for _, step := range pickle.Steps {
				scenario.Steps = append(scenario.Steps, step.Text)
			}
			scenarios = append(scenarios, scenario)
		}
	}
	return scenarios
}

func r256BRetainedAdapters(t *testing.T, bindings []contract.StepBinding) {
	t.Helper()
	// These two adapters must still retain all nine canonical rows. This static
	// selection assertion does not claim genuine Integration execution passed.
	for _, name := range []string{contract.Unit, contract.Integration} {
		actual, err := contract.AdapterByName(name)
		if err != nil {
			t.Fatal(err)
		}
		if len(contract.ApprovedExemptions[name]) != 0 {
			t.Fatalf("%s must retain its zero exemption inventory", name)
		}
		if err := contract.VerifyAt(actual, bindings, remainingCanonicalCorpus); err != nil {
			t.Fatal(err)
		}
	}
}

func r256BSelectedRows(t *testing.T, control r256BControl, adapter contract.Adapter, scenarios []contract.Scenario) []int {
	t.Helper()
	expectedTag := map[string]string{
		"TUI verification retains model terminal and host boundaries":           "@tui-gates",
		"Display annotations do not change public status or watch comparison":   "@tui-compatibility",
		"Completed observations remain unchanged when source values are reused": "@tui-snapshot-07",
		"Unknown sample time remains unavailable":                               "@tui-refresh-08",
		"Missing host readings are distinct from accounting zero":               "@tui-resources-01",
		"Synthetic unsafe display text cannot escape its cell budget":           "@tui-synthetic-text",
	}[control.name]
	var selected []int
	for index, scenario := range scenarios {
		if scenario.Name == control.name {
			selected = append(selected, index)
			if !slices.Contains(scenario.Tags, adapter.ExemptionTag) || !slices.Contains(scenario.Tags, expectedTag) {
				t.Fatalf("canonical row lacks reviewed exemption: %+v", scenario)
			}
		}
	}
	if len(selected) != control.rows {
		t.Fatalf("canonical row count %d, want %d", len(selected), control.rows)
	}
	if control.rows == 3 {
		want := []string{"a TUI row and detail containing wide Unicode characters", "a TUI row and detail containing combining Unicode characters", "a TUI row and detail containing ANSI escapes and control bytes"}
		for index, row := range selected {
			if scenarios[row].Steps[0] != want[index] {
				t.Fatalf("synthetic canonical parameter %q, want %q", scenarios[row].Steps[0], want[index])
			}
		}
	}
	return selected
}

func r256BRemovalChecks(t *testing.T, control r256BControl, adapter contract.Adapter, scenarios []contract.Scenario, selected []int) {
	t.Helper()
	for _, index := range selected {
		changed := slices.Clone(scenarios)
		changed[index].Tags = slices.DeleteFunc(slices.Clone(changed[index].Tags), func(tag string) bool { return tag == adapter.ExemptionTag })
		if found := contract.ValidateExemptions(adapter, changed); !r256BExemptionMismatch(found, control.name) {
			t.Fatalf("removing canonical row tag did not produce exact mismatch: %v", found)
		}
	}
	original := contract.ApprovedExemptions[adapter.Name]
	contract.ApprovedExemptions[adapter.Name] = slices.DeleteFunc(slices.Clone(original), func(row contract.Exemption) bool { return row.Scenario == control.name })
	found := contract.ValidateExemptions(adapter, scenarios)
	contract.ApprovedExemptions[adapter.Name] = original
	if !r256BExemptionMismatch(found, control.name) {
		t.Fatalf("removing approved canonical inventory row did not produce exact mismatch: %v", found)
	}
}

func r256BRunRefusal(t *testing.T, control r256BControl, adapter contract.Adapter, driver *composedBDDDriver) {
	t.Helper()
	path := filepath.Join(remainingCanonicalCorpus, control.feature)
	retrieval := contract.SuiteOptionsAt(adapter, remainingCanonicalCorpus)
	retrieval.Paths, retrieval.Tags = []string{path}, ""
	features, err := (godog.TestSuite{Options: retrieval}).RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	line := int64(0)
	for _, feature := range features {
		for _, child := range feature.Feature.Children {
			if child.Scenario != nil && child.Scenario.Name == control.name {
				if line != 0 {
					t.Fatalf("ambiguous canonical name %q", control.name)
				}
				line = child.Scenario.Location.Line
			}
		}
	}
	if line == 0 {
		t.Fatalf("missing canonical definition %q", control.name)
	}
	bounded := contract.SuiteOptionsAt(adapter, remainingCanonicalCorpus)
	bounded.Paths = []string{fmt.Sprintf("%s:%d", path, line)}
	// Override only the ordinary exemption selector for this expected
	// negative. The actual canonical steps and composed handlers are kept.
	bounded.Tags = ""
	var transcript bytes.Buffer
	bounded.Output, bounded.DefaultContext = &transcript, t.Context()
	visited, classified := 0, 0
	var unexpected []string
	result := godog.TestSuite{Name: "hippo-r256-b-refusal", Options: bounded, ScenarioInitializer: func(scenarioContext *godog.ScenarioContext) {
		scenarioContext.Before(func(ctx context.Context, scenario *godog.Scenario) (context.Context, error) {
			driver.Reset()
			visited++
			if scenario.Name != control.name {
				unexpected = append(unexpected, "unrelated canonical row: "+scenario.Name)
			}
			return ctx, nil
		})
		scenarioContext.StepContext().After(func(ctx context.Context, _ *godog.Step, status godog.StepResultStatus, err error) (context.Context, error) {
			if status == godog.StepFailed {
				if err != nil && err.Error() == control.refusal {
					classified++
				} else {
					unexpected = append(unexpected, fmt.Sprintf("unclassified handler failure: %v", err))
				}
			}
			return ctx, nil
		})
		for _, binding := range driver.Bindings() {
			scenarioContext.Step(binding.Pattern, binding.Handler)
		}
	}}.Run()
	t.Log(transcript.String())
	if result != 1 || visited != control.rows || classified != control.rows || len(unexpected) != 0 {
		t.Fatalf("actual canonical refusal: status=%d rows=%d classified=%d unexpected=%v; want 1/%d/%d", result, visited, classified, unexpected, control.rows, control.rows)
	}
}

func r256BMandatoryASCII(t *testing.T, scenarios []contract.Scenario, adapter contract.Adapter) {
	t.Helper()
	mandatory := 0
	for _, scenario := range scenarios {
		if scenario.Name == "Untrusted display text cannot escape its cell budget" {
			mandatory++
			if slices.Contains(scenario.Tags, adapter.ExemptionTag) {
				t.Fatal("original unsafe ASCII scenario must remain mandatory")
			}
		}
	}
	if mandatory != 1 {
		t.Fatalf("unsafe ASCII canonical row count=%d, want1", mandatory)
	}
}

func r256BExemptionMismatch(found []error, name string) bool {
	want := fmt.Sprintf("e2e exemption mismatch for scenario %q", name)
	for _, err := range found {
		if err.Error() == want {
			return true
		}
	}
	return false
}
