package tui_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/cucumber/godog"
	"github.com/wahidyankf/hippo/tests/contract"
)

const remainingCanonicalCorpus = "../../../specs/behaviours"

// This source-preparation RED exposes the missing local component at the
// settled suite location. GREEN must compose real local handlers with support.
// It must preserve complete registry verification before filtering execution.
func TestTUIBDDRemainingFullRegistry(t *testing.T) {
	adapter, err := contract.AdapterByName(contract.Unit)
	if err != nil {
		t.Fatal(err)
	}
	driver := newComposedBDDDriver(adapter)
	defer driver.Close()
	if err := contract.VerifyAt(adapter, driver.Bindings(), remainingCanonicalCorpus); err != nil {
		t.Fatal(err)
	}
}

func TestTUIBDDRemainingCanonicalRefreshExecutes(t *testing.T) {
	adapter, err := contract.AdapterByName(contract.Unit)
	if err != nil {
		t.Fatal(err)
	}
	driver := newComposedBDDDriver(adapter)
	defer driver.Close()
	options := contract.SuiteOptionsAt(adapter, remainingCanonicalCorpus)
	if err := contract.VerifyAt(adapter, driver.Bindings(), remainingCanonicalCorpus); err != nil {
		t.Fatal(err)
	}
	options.Tags = "@tui-refresh-01"
	features, err := (godog.TestSuite{Options: options}).RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	rows, steps := 0, 0
	for _, feature := range features {
		for _, scenario := range feature.Pickles {
			if scenario.Name != "Default refresh starts five seconds after completion" {
				t.Fatalf("selected unrelated scenario %q", scenario.Name)
			}
			rows++
			steps += len(scenario.Steps)
		}
	}
	if rows != 1 || steps != 3 {
		t.Fatalf("selected %d scenarios/%d steps, want canonical refresh 1/3", rows, steps)
	}
	var transcript bytes.Buffer
	options.Output, options.TestingT, options.DefaultContext = &transcript, t, t.Context()
	executedRows := 0
	result := godog.TestSuite{
		Name: "hippo-tui-refresh-unit",
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
	if result != 0 || executedRows != 1 {
		t.Fatalf("canonical refresh execution exited %d and visited %d rows, want 0/1", result, executedRows)
	}
}

func TestTUIBDDAnnotationCanonicalExecutes(t *testing.T) {
	adapter, err := executingBDDSuite(os.Getenv("HIPPO_BDD_SUITE"))
	if err != nil {
		t.Fatal(err)
	}
	driver := newComposedBDDDriver(adapter)
	defer driver.Close()
	// Verify the complete recursive registry before selecting the seven exact
	// original definitions by their canonical source lines. No corpus is copied.
	if err := contract.VerifyAt(adapter, driver.Bindings(), remainingCanonicalCorpus); err != nil {
		t.Fatal(err)
	}
	options := contract.SuiteOptionsAt(adapter, remainingCanonicalCorpus)
	options.Paths = nil
	for _, line := range []int{206, 213, 220, 242, 249, 256, 282} {
		options.Paths = append(options.Paths, fmt.Sprintf("%s/execution.feature:%d", remainingCanonicalCorpus, line))
	}
	features, err := (godog.TestSuite{Options: options}).RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	wantNames := map[string]int{
		"Explicit display labels are visible before reservation ownership is published": 1,
		"Queued work publishes its display label before admission":                      1,
		"A default display label uses only the executable basename":                     1,
		"Exclusive owners publish the same advisory label metadata":                     1,
		"Inherited runs preserve the original owner's display label":                    1,
		"Owned display annotations retire with their owned tokens":                      4,
		"Annotation cleanup requires positive stale ownership evidence":                 1,
	}
	actualNames := map[string]int{}
	rows, steps := 0, 0
	for _, feature := range features {
		for _, scenario := range feature.Pickles {
			actualNames[scenario.Name]++
			rows++
			steps += len(scenario.Steps)
		}
	}
	if len(actualNames) != len(wantNames) || rows != 10 || steps != 40 {
		t.Fatalf("canonical annotation selection=%v rows=%d steps=%d, want7/10/40", actualNames, rows, steps)
	}
	for name, want := range wantNames {
		if actualNames[name] != want {
			t.Fatalf("canonical annotation %q selected%d rows want%d", name, actualNames[name], want)
		}
	}
	var transcript bytes.Buffer
	options.Output, options.TestingT, options.DefaultContext = &transcript, t, t.Context()
	executed := 0
	result := godog.TestSuite{Name: "hippo-annotation-" + adapter.Name, ScenarioInitializer: func(scenarioContext *godog.ScenarioContext) {
		scenarioContext.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
			driver.Reset()
			executed++
			return ctx, nil
		})
		for _, binding := range driver.Bindings() {
			scenarioContext.Step(binding.Pattern, binding.Handler)
		}
	}, Options: options}.Run()
	t.Log(transcript.String())
	if result != 0 || executed != 10 {
		t.Fatalf("actual %s canonical annotation execution=%d rows=%d want0/10", adapter.Name, result, executed)
	}
}
