package support

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// These recording-shell checks prove mandatory routing and failure propagation;
// genuine release provenance and two-binary behaviour require the native leaf.
func TestBDDGateV085InteroperabilityPreparesBothReleasesAndCurrentCandidate(t *testing.T) {
	inputs := []string{
		"HIPPO_PUBLIC_V085_BIN=inherited-unverified-binary",
		"HIPPO_PUBLIC_V085_BIN_SHA256=inherited-unverified-digest",
		"HIPPO_PUBLIC_V085_VERSION_RECEIPT=inherited-unverified-receipt",
	}
	calls, code := recordBDDGateInputs(t, "scripts/test.sh", "", inputs)
	if code != 0 {
		t.Fatalf("recorded required full gate exited %d", code)
	}
	candidate := bddGateFlag(requireBDDGateCall(t, calls, "build", "./cmd/hippo"), "-o")
	count := 0
	for _, call := range calls {
		if bddGateFlag(call, "-run") != "^TestMixedVersionV085DisplayInteroperability$" {
			continue
		}
		count++
		if call.args[0] != "test" || call.binary != candidate || bddGateFlag(call, "-tags") != "mixedversion" || bddGateFlag(call, "-count") != "1" || bddGateFlag(call, "-timeout") != "120s" || bddGateFlag(call, "-p") != "2" {
			t.Errorf("required v0.8.5 leaf lost actual uncached bounded candidate routing: %+v", call)
		}
		if !slices.Contains(call.args, "./tests/integration") || len(bddGateFlagValues(call, "-run")) != 1 || len(bddGateFlagValues(call, "-skip")) != 0 || slices.Contains(call.args, "-c") || len(bddGateFlagValues(call, "-list")) != 0 {
			t.Errorf("required v0.8.5 leaf became nonexecuting or lost its sole selector: %+v", call)
		}
		if call.v085Binary != filepath.Join(filepath.Dir(candidate), "recorded-v085-hippo") || call.v085SHA != "recorded-v085-digest" || call.v085Receipt != filepath.Join(filepath.Dir(candidate), "recorded-v085-version") {
			t.Errorf("required v0.8.5 leaf used inherited inputs or lost preparation: %+v", call)
		}
		if call.oldSHA != "recorded-old-digest" || call.oldBinary == call.v085Binary || call.oldReceipt == call.v085Receipt || call.helper == "" || call.helperSHA == "" {
			t.Errorf("v0.8.5 leaf overwrote retained v0.8.4 proof or lost collector: %+v", call)
		}
	}
	if count != 1 {
		t.Fatalf("required v0.8.5 compatibility executions=%d, want one", count)
	}
}

func TestBDDGateV085InteroperabilityFailureStopsAndCleansCandidate(t *testing.T) {
	calls, code := recordBDDGate(t, "tests/e2e/run.sh", "test:^TestMixedVersionV085DisplayInteroperability$")
	if code != 73 || len(calls) == 0 || bddGateFlag(calls[len(calls)-1], "-run") != "^TestMixedVersionV085DisplayInteroperability$" {
		t.Fatalf("required compatibility failure was not preserved: exit=%d calls=%+v", code, calls)
	}
	candidate := bddGateFlag(requireBDDGateCall(t, calls, "build", "./cmd/hippo"), "-o")
	if _, err := os.Stat(filepath.Dir(candidate)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed compatibility leaf leaked gate-owned candidate: %v", err)
	}
}

func TestBDDGateV085PreparationFailureStopsBeforeCompatibilityLeaf(t *testing.T) {
	calls, code := recordBDDGateInputs(t, "tests/e2e/run.sh", "", []string{"HIPPO_GATE_V085_FAIL=73"})
	if code != 73 {
		t.Fatalf("v0.8.5 preparation failure exit=%d, want 73", code)
	}
	for _, call := range calls {
		if call.args[0] == "test" && !slices.Contains(call.args, "-c") {
			t.Fatalf("gate tested after unqualified v0.8.5 preparation: %+v", call)
		}
	}
	candidate := bddGateFlag(requireBDDGateCall(t, calls, "build", "./cmd/hippo"), "-o")
	if _, err := os.Stat(filepath.Dir(candidate)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed v0.8.5 preparation leaked candidate directory: %v", err)
	}
}
