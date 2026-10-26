package bdd_test

import (
	"flag"
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/wahidyankf/hippo/tests/contract"
	"github.com/wahidyankf/hippo/tests/support"
)

// behavioursCorpus is the shared Gherkin corpus every adapter executes.
const behavioursCorpus = "../../specs/behaviours"

func executingBDDSuite(value string) (contract.Adapter, error) {
	if value == "" {
		value = contract.Unit
	}
	return contract.AdapterByName(value)
}

func runBehaviours(t *testing.T, expected string) {
	t.Helper()
	adapter, err := executingBDDSuite(os.Getenv("HIPPO_BDD_SUITE"))
	if err != nil {
		t.Fatal(err)
	}
	if adapter.Name != expected {
		if exclusiveExecutingSelection(flag.Lookup("test.run").Value.String(), expected) {
			t.Fatalf("explicit %s entry requires HIPPO_BDD_SUITE=%s; selected %s", expected, expected, adapter.Name)
		}
		t.Skipf("executing suite is %s; this entry owns %s", adapter.Name, expected)
	}
	// RunAt validates the complete recursive corpus before applying the exact
	// reviewed adapter exemption inventory.
	contract.RunAt(t, adapter, support.NewDriver(adapter), behavioursCorpus)
}

func exclusiveExecutingSelection(selector, expected string) bool {
	if selector == "" {
		return false
	}
	pattern, err := regexp.Compile(selector)
	if err != nil {
		return false
	}
	matched := 0
	selected := ""
	for mode, name := range map[string]string{contract.Unit: "TestUnitBehaviours", contract.Integration: "TestIntegrationBehaviours", contract.E2E: "TestE2EBehaviours"} {
		if pattern.MatchString(name) {
			matched++
			selected = mode
		}
	}
	return matched == 1 && selected == expected
}

func complianceOnlySelection(selector string) bool {
	if selector == "" {
		return false
	}
	pattern, err := regexp.Compile(selector)
	if err != nil || !pattern.MatchString("TestFeatureCompliance") {
		return false
	}
	return !slices.ContainsFunc([]string{"TestUnitBehaviours", "TestIntegrationBehaviours", "TestE2EBehaviours"}, pattern.MatchString)
}

func TestUnitBehaviours(t *testing.T)        { runBehaviours(t, contract.Unit) }
func TestIntegrationBehaviours(t *testing.T) { runBehaviours(t, contract.Integration) }
func TestE2EBehaviours(t *testing.T)         { runBehaviours(t, contract.E2E) }

func TestFeatureCompliance(t *testing.T) {
	name := os.Getenv("HIPPO_BDD_ADAPTER")
	if name == "" {
		name = contract.Unit
	}
	adapter, err := contract.AdapterByName(name)
	if err != nil {
		t.Fatal(err)
	}
	driver := support.NewDriver(adapter)
	defer driver.Close()
	if err := contract.VerifyAt(adapter, driver.Bindings(), behavioursCorpus); err != nil {
		t.Fatal(err)
	}
}

func TestBDDExecutingSuiteSelection(t *testing.T) {
	for _, test := range []struct{ value, want string }{{"", contract.Unit}, {contract.Unit, contract.Unit}, {contract.Integration, contract.Integration}, {contract.E2E, contract.E2E}} {
		t.Run("accept-"+test.value, func(t *testing.T) {
			adapter, err := executingBDDSuite(test.value)
			if err != nil || adapter.Name != test.want {
				t.Fatalf("selection %q = %s/%v, want %s", test.value, adapter.Name, err, test.want)
			}
		})
	}
	for _, value := range []string{"unknown", "Unit", " unit", "integration ", "@e2e"} {
		t.Run("refuse-"+value, func(t *testing.T) {
			if adapter, err := executingBDDSuite(value); err == nil {
				t.Fatalf("invalid suite %q selected %+v", value, adapter)
			}
		})
	}
}

func TestBDDEntrySelectorsRefuseSilentSkip(t *testing.T) {
	for _, test := range []struct {
		selector, expected string
		want               bool
	}{
		{"^TestIntegrationBehaviours$", contract.Integration, true},
		{"^TestE2EBehaviours$", contract.E2E, true},
		{"^TestUnitBehaviours$", contract.Unit, true},
		{"^TestIntegrationBehaviours$", contract.Unit, false},
		{"", contract.Integration, false},
		{"Behaviours", contract.Integration, false},
	} {
		t.Run(test.selector+"/"+test.expected, func(t *testing.T) {
			if got := exclusiveExecutingSelection(test.selector, test.expected); got != test.want {
				t.Fatalf("exclusive selection=%t want=%t", got, test.want)
			}
		})
	}
	for _, test := range []struct {
		selector string
		want     bool
	}{{"^TestFeatureCompliance$", true}, {"", false}, {"Test", false}, {"^TestE2EBehaviours$", false}} {
		t.Run("static/"+test.selector, func(t *testing.T) {
			if got := complianceOnlySelection(test.selector); got != test.want {
				t.Fatalf("static-only selection=%t want=%t", got, test.want)
			}
		})
	}
}
