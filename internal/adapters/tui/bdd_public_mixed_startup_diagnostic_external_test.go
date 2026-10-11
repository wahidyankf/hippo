package tui_test

import (
	"context"
	"flag"
	"os"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/tests/contract"
	"github.com/wahidyankf/hippo/tests/support"
)

// Task-only exact-selected causal startup control; the unchanged full canonical
// suite remains the product acceptance gate, including its Mixed scenario.
func TestTUIBDDPublicMixedOwnerStartupDiagnostic(t *testing.T) {
	selected := flag.Lookup("test.run")
	if selected == nil || selected.Value.String() != "^TestTUIBDDPublicMixedOwnerStartupDiagnostic$" {
		return
	}
	if os.Getenv("HIPPO_BDD_SUITE") != contract.E2E || os.Getenv("HIPPO_BDD_ADAPTER") != contract.E2E {
		t.Fatal("focused genuine mixed startup requires explicit E2E isolation")
	}
	driver := support.NewDriver(contract.Adapter{Name: contract.E2E})
	driver.Reset()
	t.Cleanup(driver.Close)
	fixture := &publicBDDObservation{driver: driver}
	retainedMixed := fixture
	scenario, cancelScenario := context.WithCancel(t.Context())
	defer cancelScenario()
	defer func() { finalizePublicMixedStartupDiagnostic(t, fixture, retainedMixed) }()
	scenario, err := fixture.prepareCase(scenario, "mixed", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(fixture.mixed) != 2 || fixture.mixed[0].command.Args[0] == fixture.mixed[1].command.Args[0] || fixture.mixed[0].token == fixture.mixed[1].token || fixture.mixed[0].payloadPID <= 0 || fixture.mixed[1].payloadPID <= 0 || fixture.mixed[0].payloadPID == fixture.mixed[1].payloadPID {
		t.Fatal("startup must authenticate distinct old/current real commands, tokens and live payload PIDs")
	}
	expected := fixture.ledger.Expected()
	if expected.ActiveOwners != 2 || expected.WaitingOwners != 3 || expected.Allocated.CPU != 4 || expected.Allocated.MemoryBytes != 2*1024*1024*1024 || expected.Waiting.CPU != 6 || expected.Waiting.MemoryBytes != 3*1024*1024*1024 {
		t.Fatalf("genuine mixed startup changed original2/3/vectors: %+v", expected)
	}
	for _, job := range fixture.mixed {
		t.Logf("genuine mixed startup authenticated=%s", fixture.mixedStartupReceipt(job))
	}
	if err := fixture.observe(scenario); err != nil {
		t.Fatal(err)
	}
	if err := fixture.checkRows(scenario); err != nil {
		t.Fatal(err)
	}
	if err := fixture.finishObservation(scenario); err != nil {
		t.Fatal(err)
	}
	// Preserve actual joined command/probe assertions and receipts across Reset.
	// This view owns no second cleanup; only fixture.close retires the actors.
	retainedMixed = &publicBDDObservation{root: fixture.root, ledger: fixture.ledger, mixed: fixture.mixed}
	cancelScenario()
	fixture.Reset()
	t.Log("original Mixed observation/Then and canceled-scenario Reset completed without cleanup failure")
}

func finalizePublicMixedStartupDiagnostic(t *testing.T, fixture, retainedMixed *publicBDDObservation) {
	t.Helper()
	for _, job := range retainedMixed.mixed {
		t.Logf("actual mixed startup before cleanup=%s", retainedMixed.mixedStartupReceipt(job))
	}
	cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	err := fixture.close(cleanup)
	for _, job := range retainedMixed.mixed {
		t.Logf("actual mixed startup after cleanup=%s", retainedMixed.mixedStartupReceipt(job))
	}
	if err != nil {
		t.Errorf("independent mixed startup cleanup failure: %v", err)
	}
	for _, job := range retainedMixed.mixed {
		if !job.joined {
			t.Error("actual mixed Wait did not join")
		}
		if job.probe != nil && !job.probe.Trace().Closed {
			t.Error("actual mixed collector actors did not close/join")
		}
	}
}
