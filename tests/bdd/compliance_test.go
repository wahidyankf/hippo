package bdd_test

import (
	"os"
	"strings"
	"testing"

	"github.com/wahidyankf/resource-guard/tests/contract"
	"github.com/wahidyankf/resource-guard/tests/support"
)

var _ contract.Driver = (*support.Driver)(nil)

func TestFeatureCompliance(t *testing.T) {
	requested := os.Getenv("RESOURCE_GUARD_BDD_ADAPTER")

	if requested != "" {
		adapter, err := contract.AdapterByName(requested)
		if err != nil {
			t.Fatal(err)
		}

		verifyAdapter(t, adapter)

		return
	}

	for _, adapter := range contract.Adapters() {
		t.Run(adapter.Name, func(t *testing.T) { verifyAdapter(t, adapter) })
	}
}

func TestScenarioWithoutActionAndOutcomeIsRejected(t *testing.T) {
	err := contract.ValidateScenarioStructure("incomplete", []string{"Context", "Context"})
	if err == nil {
		t.Fatal("scenario without explicit When and Then was accepted")
	}
}

func TestUndefinedBindingIsRejected(t *testing.T) {
	errorsFound := contract.ValidateBindings(
		[]contract.StepBinding{testBinding(`^known$`)},
		[]string{"known", "unknown"},
	)
	requireErrorContaining(t, errorsFound, "undefined behavior step")
}

func TestAmbiguousBindingIsRejected(t *testing.T) {
	errorsFound := contract.ValidateBindings(
		[]contract.StepBinding{testBinding(`^duplicate$`), testBinding(`^duplicate$`)},
		[]string{"duplicate"},
	)
	requireErrorContaining(t, errorsFound, "ambiguous behavior step")
}

func TestUnusedBindingIsRejected(t *testing.T) {
	errorsFound := contract.ValidateBindings(
		[]contract.StepBinding{testBinding(`^used$`), testBinding(`^unused$`)},
		[]string{"used"},
	)
	requireErrorContaining(t, errorsFound, "unused behavior binding")
}

func TestInvalidBindingExpressionIsRejected(t *testing.T) {
	errorsFound := contract.ValidateBindings(
		[]contract.StepBinding{testBinding(`(`)},
		[]string{"anything"},
	)
	requireErrorContaining(t, errorsFound, "invalid binding")
}

func TestMissingBindingHandlerIsRejected(t *testing.T) {
	errorsFound := contract.ValidateHandlers([]contract.StepBinding{{Pattern: `^missing$`}})
	requireErrorContaining(t, errorsFound, "requires a nonnil function handler")
}

func TestNonFunctionBindingHandlerIsRejected(t *testing.T) {
	errorsFound := contract.ValidateHandlers([]contract.StepBinding{{Pattern: `^invalid$`, Handler: "not a function"}})
	requireErrorContaining(t, errorsFound, "requires a nonnil function handler")
}

func TestBindingOrderDoesNotAffectValidation(t *testing.T) {
	bindings := []contract.StepBinding{testBinding(`^second$`), testBinding(`^first$`)}
	errorsFound := contract.ValidateBindings(bindings, []string{"first", "second"})

	if len(errorsFound) != 0 {
		t.Fatalf("reordered paired bindings were rejected: %v", errorsFound)
	}
}

func TestUnapprovedExemptionIsRejected(t *testing.T) {
	adapter, err := contract.AdapterByName(contract.Integration)
	if err != nil {
		t.Fatal(err)
	}

	errorsFound := contract.ValidateExemptions(adapter, []contract.Scenario{{
		Name: "unapproved",
		Tags: []string{"@integration-exempt"},
	}})
	requireErrorContaining(t, errorsFound, "integration exemption mismatch")
}

func TestUnitExemptionTagIsRejected(t *testing.T) {
	adapter, err := contract.AdapterByName(contract.Unit)
	if err != nil {
		t.Fatal(err)
	}

	errorsFound := contract.ValidateExemptions(adapter, []contract.Scenario{{
		Name: "unit must run this scenario",
		Tags: []string{"@unit-exempt"},
	}})
	requireErrorContaining(t, errorsFound, "unknown exemption tag")
}

func TestUnitExemptionInventoryIsRejected(t *testing.T) {
	adapter, err := contract.AdapterByName(contract.Unit)
	if err != nil {
		t.Fatal(err)
	}

	original := contract.ApprovedExemptions[contract.Unit]
	contract.ApprovedExemptions[contract.Unit] = []contract.Exemption{{
		Scenario: "unit must run this scenario",
		Boundary: "test fixture",
		Reason:   "a unit exemption must never be accepted",
	}}
	t.Cleanup(func() { contract.ApprovedExemptions[contract.Unit] = original })

	errorsFound := contract.ValidateExemptions(adapter, []contract.Scenario{{Name: "unit must run this scenario"}})
	requireErrorContaining(t, errorsFound, "unit adapter does not permit exemptions")
}

func TestExemptionRequiresBoundaryAndReason(t *testing.T) {
	adapter, err := contract.AdapterByName(contract.Integration)
	if err != nil {
		t.Fatal(err)
	}

	original := contract.ApprovedExemptions[contract.Integration]
	contract.ApprovedExemptions[contract.Integration] = []contract.Exemption{{Scenario: "incomplete rationale"}}
	t.Cleanup(func() { contract.ApprovedExemptions[contract.Integration] = original })

	errorsFound := contract.ValidateExemptions(adapter, []contract.Scenario{{
		Name: "incomplete rationale",
		Tags: []string{"@integration-exempt"},
	}})
	requireErrorContaining(t, errorsFound, "has no boundary")
	requireErrorContaining(t, errorsFound, "has no reason")
}

func verifyAdapter(t *testing.T, adapter contract.Adapter) {
	t.Helper()
	driver := support.NewDriver(adapter)
	defer driver.Close()

	if !contract.SuiteOptions(adapter).Strict {
		t.Fatalf("%s adapter is not strict", adapter.Name)
	}
	if err := contract.Verify(adapter, driver.Bindings()); err != nil {
		t.Fatal(err)
	}
}

func testBinding(pattern string) contract.StepBinding {
	return contract.StepBinding{Pattern: pattern, Handler: func() {}}
}

func requireErrorContaining(t *testing.T, errorsFound []error, expected string) {
	t.Helper()

	for _, err := range errorsFound {
		if strings.Contains(err.Error(), expected) {
			return
		}
	}

	t.Fatalf("expected error containing %q, got %v", expected, errorsFound)
}
