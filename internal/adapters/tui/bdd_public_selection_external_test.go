package tui_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cucumber/godog"
	"github.com/wahidyankf/hippo/tests/contract"
)

// Select original public rows only after complete registry/exemption verification.
// Actual bindings own every observation and native receipt; this helper supplies none.
func runPublicCanonicalSelection(t *testing.T, tag string, wantNames map[string]int, wantRows, wantSteps int) *composedBDDDriver {
	t.Helper()
	return runPublicCanonicalSelectionAtLine(t, tag, "", 0, wantNames, wantRows, wantSteps)
}

// A focused original-line selection still verifies the complete canonical corpus.
func runPublicCanonicalSelectionAtLine(t *testing.T, tag, featureFile string, line int, wantNames map[string]int, wantRows, wantSteps int) *composedBDDDriver {
	t.Helper()
	if os.Getenv("HIPPO_BDD_SUITE") != contract.E2E {
		t.Fatal("public canonical selection requires explicit HIPPO_BDD_SUITE=e2e")
	}
	if tag == "" || len(wantNames) == 0 || wantRows < 1 || wantSteps < 1 {
		t.Fatal("public canonical selection requires named original rows and steps")
	}
	adapter, err := contract.AdapterByName(contract.E2E)
	if err != nil {
		t.Fatal(err)
	}
	driver := newComposedBDDDriver(adapter)
	// Join and retain actual fixture receipts before returning to the caller.
	// This also retires ownership if registry, selection or execution fails.
	defer driver.Close()
	bindings := driver.Bindings()
	if err := contract.VerifyAt(adapter, bindings, remainingCanonicalCorpus); err != nil {
		t.Fatal(err)
	}
	options := contract.SuiteOptionsAt(adapter, remainingCanonicalCorpus)
	if featureFile != "" {
		if filepath.Base(featureFile) != featureFile || line < 1 {
			t.Fatal("focused original feature line is invalid")
		}
		options.Paths = []string{fmt.Sprintf("%s:%d", filepath.Join(remainingCanonicalCorpus, featureFile), line)}
	}
	if options.Tags == "" {
		options.Tags = tag
	} else {
		options.Tags = options.Tags + " && " + tag
	}
	features, err := (godog.TestSuite{Options: options}).RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	actualNames := make(map[string]int)
	rows, steps := 0, 0
	for _, feature := range features {
		for _, scenario := range feature.Pickles {
			actualNames[scenario.Name]++
			rows++
			steps += len(scenario.Steps)
		}
	}
	requirePublicCanonicalSelectionCounts(t, actualNames, rows, steps, wantNames, wantRows, wantSteps)
	var transcript bytes.Buffer
	options.Output, options.TestingT, options.DefaultContext = &transcript, t, t.Context()
	executed := make(map[string]int)
	executedRows := 0
	result := godog.TestSuite{
		Name: "hippo-public-canonical-e2e",
		ScenarioInitializer: func(scenarioContext *godog.ScenarioContext) {
			scenarioContext.Before(func(ctx context.Context, scenario *godog.Scenario) (context.Context, error) {
				driver.Reset()
				executedRows++
				executed[scenario.Name]++
				return ctx, nil
			})
			// Godog runs After before canceling the scenario context. Finalize live
			// public actors here, including after a failed original assertion.
			// Godog retains the step error and joins any returned cleanup error.
			scenarioContext.After(driver.finalizePublicScenario)
			for _, binding := range bindings {
				scenarioContext.Step(binding.Pattern, binding.Handler)
			}
		},
		Options: options,
	}.Run()
	t.Log(transcript.String())
	if result != 0 || executedRows != wantRows || len(executed) != len(wantNames) {
		t.Fatalf("public canonical execution exited %d and visited %v/%d rows, want exit 0/%v/%d", result, executed, executedRows, wantNames, wantRows)
	}
	for name, want := range wantNames {
		if executed[name] != want {
			t.Fatalf("public canonical %q executed %d rows, want %d", name, executed[name], want)
		}
	}
	return driver
}

func requirePublicCanonicalSelectionCounts(t *testing.T, actualNames map[string]int, rows, steps int, wantNames map[string]int, wantRows, wantSteps int) {
	t.Helper()
	if rows != wantRows || steps != wantSteps || len(actualNames) != len(wantNames) {
		t.Fatalf("public canonical selection=%v rows=%d steps=%d, want %v/%d/%d", actualNames, rows, steps, wantNames, wantRows, wantSteps)
	}
	for name, want := range wantNames {
		if want < 1 || actualNames[name] != want {
			t.Fatalf("public canonical %q selected %d rows, want %d", name, actualNames[name], want)
		}
	}
}

func (driver *composedBDDDriver) finalizePublicScenario(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
	return ctx, driver.public.finalizeScenario(ctx)
}
