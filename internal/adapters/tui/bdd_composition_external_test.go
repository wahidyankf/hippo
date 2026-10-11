package tui_test

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/wahidyankf/hippo/internal/adapters/tui"
	"github.com/wahidyankf/hippo/tests/contract"
	"github.com/wahidyankf/hippo/tests/support"
)

type composedBDDDriver struct {
	neutral     *support.Driver
	local       *tui.BDDTestDriver
	annotations *annotationBDD
	native      *nativeBDDViewports
	faults      *nativeBDDFaults
	public      *publicBDDObservation
}

var _ contract.Driver = (*composedBDDDriver)(nil)

func newComposedBDDDriver(adapter contract.Adapter) *composedBDDDriver {
	neutral := support.NewDriver(adapter)
	local := tui.NewBDDTestDriver(adapter)
	local.SetGuardedJobFactory(neutral.NewGuardedTUIJob)
	local.SetObservationFactory(neutral.TUIObservationWithSampleTime)
	driver := &composedBDDDriver{neutral: neutral, local: local, annotations: newAnnotationBDD(adapter, neutral)}
	if adapter.Name == contract.Integration {
		driver.native = &nativeBDDViewports{}
		driver.native.refresh = &nativeBDDRefresh{fixture: driver.native, observation: neutral.TUIObservationWithSampleTime}
		driver.native.lifecycle = &nativeBDDLifecycle{fixture: driver.native, guardedJob: neutral.NewGuardedTUIJob}
		driver.faults = &nativeBDDFaults{fixture: driver.native}
		local.SetNativeStepFactory(func(pattern string) any {
			if handler := driver.faults.handler(pattern); handler != nil {
				return handler
			}
			return driver.native.handler(pattern)
		}, func() {
			driver.native.cleanupError = nil
			driver.native.closeSession()
			driver.native.lifecycle.close()
			driver.faults.close()
			if driver.native.cleanupError != nil {
				panic(fmt.Errorf("native TUI composed cleanup: %w", driver.native.cleanupError))
			}
		})
	}
	if adapter.Name == contract.E2E {
		driver.public = &publicBDDObservation{driver: neutral}
		// The composed driver owns cleanup; the existing local factory replaces
		// actual E2E handlers without reaching the in-process observer.
		local.SetNativeStepFactory(driver.public.interactiveHandler, nil)
	}
	return driver
}

func (driver *composedBDDDriver) Bindings() []contract.StepBinding {
	neutral := driver.neutral.Bindings()
	if driver.public != nil {
		neutral = driver.neutral.BindingsWithPublicTUIObservation(driver.public.handler)
	}
	if driver.faults != nil {
		neutral = driver.neutral.BindingsWithNativeTUIArguments(driver.faults.handler)
	}
	bindings := append([]contract.StepBinding(nil), neutral...)
	bindings = append(bindings, driver.local.Bindings()...)
	return append(bindings, driver.annotations.Bindings()...)
}

func (driver *composedBDDDriver) Reset() {
	if driver.public != nil {
		driver.public.Reset()
	}
	driver.annotations.Reset()
	driver.local.Reset()
	driver.neutral.Reset()
}

func (driver *composedBDDDriver) Close() {
	if driver.public != nil {
		driver.public.Close()
	}
	driver.annotations.Close()
	driver.local.Close()
	driver.neutral.Close()
}

func executingBDDSuite(value string) (contract.Adapter, error) {
	if value == "" {
		value = contract.Unit
	}
	return contract.AdapterByName(value)
}

func TestMain(main *testing.M) {
	flag.Parse()
	adapter, err := executingBDDSuite(os.Getenv("HIPPO_BDD_SUITE"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "select executing TUI BDD suite: %v\n", err)
		os.Exit(2)
	}
	if adapter.Name == contract.E2E && !complianceOnlySelection(flag.Lookup("test.run").Value.String()) {
		os.Exit(support.RunCompiled(main))
	}
	os.Exit(support.RunIsolated(main))
}

func runComposedBDD(t *testing.T, expected string) {
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
	// RunAt validates the complete recursive registry before applying the exact
	// reviewed adapter exemption inventory. No TUI-only corpus is copied here.
	contract.RunAt(t, adapter, newComposedBDDDriver(adapter), remainingCanonicalCorpus)
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

func TestUnitBehaviours(t *testing.T)        { runComposedBDD(t, contract.Unit) }
func TestIntegrationBehaviours(t *testing.T) { runComposedBDD(t, contract.Integration) }
func TestE2EBehaviours(t *testing.T)         { runComposedBDD(t, contract.E2E) }

func TestFeatureCompliance(t *testing.T) {
	name := os.Getenv("HIPPO_BDD_ADAPTER")
	if name == "" {
		name = contract.Unit
	}
	adapter, err := contract.AdapterByName(name)
	if err != nil {
		t.Fatal(err)
	}
	driver := newComposedBDDDriver(adapter)
	defer driver.Close()
	if err := contract.VerifyAt(adapter, driver.Bindings(), remainingCanonicalCorpus); err != nil {
		t.Fatal(err)
	}
}

func TestTUIBDDExecutingSuiteSelection(t *testing.T) {
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

func TestTUIBDDEntrySelectorsRefuseSilentNativeSkip(t *testing.T) {
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
