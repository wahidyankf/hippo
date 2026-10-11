package contract

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/cucumber/godog"
)

// SuiteOptionsAt returns strict execution options for an explicit recursive corpus.
func SuiteOptionsAt(adapter Adapter, featurePath string) *godog.Options {
	tags := ""
	if adapter.ExemptionTag != "" {
		tags = "~" + adapter.ExemptionTag
	}

	return &godog.Options{
		Format: "progress",
		Paths:  []string{featurePath},
		Tags:   tags,
		Strict: true,
	}
}

// RunAt validates and executes the complete explicit corpus through one strict adapter.
func RunAt(t *testing.T, adapter Adapter, driver Driver, featurePath string) {
	t.Helper()
	defer driver.Close()

	bindings := driver.Bindings()
	if err := VerifyAt(adapter, bindings, featurePath); err != nil {
		t.Fatal(err)
	}

	options := SuiteOptionsAt(adapter, featurePath)
	options.TestingT = t
	initialize := func(scenarioContext *godog.ScenarioContext) {
		scenarioContext.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
			driver.Reset()

			return ctx, nil
		})

		for _, binding := range bindings {
			scenarioContext.Step(binding.Pattern, binding.Handler)
		}
	}

	status := godog.TestSuite{
		Name:                "hippo-" + adapter.Name,
		ScenarioInitializer: initialize,
		Options:             options,
	}.Run()

	if status != 0 {
		t.Fatalf("%s behavior suite exited %d", adapter.Name, status)
	}
}

// VerifyAt validates the complete recursive corpus before any execution filtering.
func VerifyAt(adapter Adapter, bindings []StepBinding, featurePath string) error {
	corpusOptions := &godog.Options{Paths: []string{featurePath}, Strict: true}
	features, err := (godog.TestSuite{Options: corpusOptions}).RetrieveFeatures()
	if err != nil {
		return fmt.Errorf("retrieve behavior corpus: %w", err)
	}

	diskFeatures, err := countDiskFeaturesAt(featurePath)
	if err != nil {
		return fmt.Errorf("walk behavior corpus: %w", err)
	}
	if diskFeatures == 0 || len(features) != diskFeatures {
		return fmt.Errorf("parsed %d of %d canonical behavior features", len(features), diskFeatures)
	}

	inspection := corpusInspection{}

	for _, feature := range features {
		mergeInspection(&inspection, inspectFeature(feature.Uri, feature.Feature, feature.Pickles))
	}

	if inspection.astScenarios == 0 || len(inspection.scenarios) == 0 {
		inspection.structuralError = append(inspection.structuralError, errors.New("behavior corpus has no scenarios"))
	}
	inspection.structuralError = append(inspection.structuralError, ValidateBindings(bindings, inspection.steps)...)
	inspection.structuralError = append(inspection.structuralError, ValidateExemptions(adapter, inspection.scenarios)...)
	if runnableScenarios(adapter, inspection.scenarios) == 0 {
		inspection.structuralError = append(inspection.structuralError, fmt.Errorf("%s adapter runs no scenarios", adapter.Name))
	}

	return combineErrors(inspection.structuralError)
}
