package support

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// These tests verify recorded script orchestration and literal gate inspectors.
// A recording Go stand-in never builds or executes a candidate.
func TestBDDGatePublicEntriesRegisteredOnceWithSameCandidate(t *testing.T) {
	calls, code := recordBDDGate(t, "tests/e2e/run.sh", "")
	if code != 0 {
		t.Fatalf("recording E2E script exited %d", code)
	}
	candidate := bddGateFlag(requireBDDGateCall(t, calls, "build", "./cmd/hippo"), "-o")
	var oldBinary, oldReceipt string
	wanted := []string{"^TestE2EBehaviours$"}
	var actual []string
	for _, call := range calls {
		if call.args[0] != "test" || !slices.Contains(call.args, "./tests/bdd") {
			continue
		}
		actual = append(actual, bddGateFlag(call, "-run"))
		if oldBinary == "" {
			oldBinary, oldReceipt = call.oldBinary, call.oldReceipt
		}
		if call.oldBinary == "" || call.oldBinary != oldBinary || call.oldReceipt == "" || call.oldReceipt != oldReceipt || call.oldSHA != "recorded-old-digest" {
			t.Errorf("public entry lost recorded immutable old prerequisite: %+v", call)
		}
		if call.binary != candidate || call.suite != "e2e" || bddGateFlag(call, "-count") != "1" || len(bddGateFlagValues(call, "-skip")) != 0 {
			t.Errorf("public entry lost candidate/mode/exact execution: %+v", call)
		}
		if call.adapter != "" {
			t.Errorf("direct public entry carries an explicit adapter: %+v", call)
		}
	}
	if !slices.Equal(actual, wanted) {
		t.Fatalf("required serial entries = %v, want %v", actual, wanted)
	}
}

func TestBDDGatePublicInspectorRejectsEachMissingOrNonexecutingEntry(t *testing.T) {
	legacy := "HIPPO_BDD_SUITE=e2e HIPPO_BIN=\"$temporary_dir/hippo\" \"$go_binary\" test -count=1 -run '^TestE2EBehaviours$' ./tests/bdd\n"
	preparation := "public_old_release_prepare \"$tool_dir\" \"$temporary_dir\"\n"
	compiled := preparation + legacy
	check := func(source string) error {
		return checkStandaloneGatePublicEntries(normalizedBDDGateCommands(source))
	}
	if err := check(compiled); err != nil {
		t.Fatalf("complete public registration rejected: %v", err)
	}
	for _, invalid := range []string{
		strings.TrimPrefix(compiled, preparation),
		"# " + compiled,
		strings.TrimPrefix(compiled, preparation) + preparation,
		preparation + compiled,
		strings.Replace(compiled, `public_old_release_prepare "$tool_dir" "$temporary_dir"`, `public_old_release_prepare "$tool_dir" other-directory`, 1),
	} {
		if err := check(invalid); err == nil {
			t.Fatal("missing, commented, late, duplicate or wrong owned old preparation accepted")
		}
	}
	for line := range strings.SplitSeq(strings.TrimSuffix(legacy, "\n"), "\n") {
		_, selector, _ := strings.Cut(line, "-run '^")
		name, _, _ := strings.Cut(selector, "$'")
		t.Run(name, func(t *testing.T) {
			requireInvalidPublicEntryMutations(t, compiled, line, name, check)
		})
	}
}

func TestBDDGatePublicOrdinarySelectorsRetainUnitAndPureControls(t *testing.T) {
	for _, script := range []string{"scripts/test-quick.sh", "scripts/test.sh"} {
		calls, code := recordBDDGate(t, script, "")
		if code != 0 {
			t.Fatalf("recording %s exited %d", script, code)
		}
		for _, call := range calls {
			if call.args[0] != "test" || !slices.Contains(call.args, "./internal/...") {
				continue
			}
			for _, name := range gateCorpusEntries() {
				if bddGateFlag(call, "-run") == "^"+name+"$" {
					t.Errorf("%s executes behaviour %s ordinarily", script, name)
				}
			}
			for _, name := range []string{"TestOrdinaryDirect", "TestAnnotationCanonicalExecutes", "TestUnitBehavioursExtra", "PrefixTestFeatureCompliance"} {
				if !bddGateSelects(call, name) {
					t.Errorf("%s drops pure or prefix control %s", script, name)
				}
			}
		}
	}
}

func TestBDDGatePublicEntryFailureStopsAndCleansCandidate(t *testing.T) {
	for _, name := range []string{"TestE2EBehaviours"} {
		t.Run(name, func(t *testing.T) {
			calls, code := recordBDDGate(t, "tests/e2e/run.sh", "test:^"+name+"$")
			if code != 73 || len(calls) == 0 || bddGateFlag(calls[len(calls)-1], "-run") != "^"+name+"$" {
				t.Fatalf("entry failure did not stop actual script: exit=%d calls=%+v", code, calls)
			}
			candidate := bddGateFlag(requireBDDGateCall(t, calls, "build", "./cmd/hippo"), "-o")
			if _, err := os.Stat(filepath.Dir(candidate)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed public entry leaked owned candidate and old artifact directory: %v", err)
			}
		})
	}
}

func TestBDDGateOldPreparationFailureStopsE2EGate(t *testing.T) {
	calls, code := recordBDDGateInputs(t, "tests/e2e/run.sh", "", []string{"HIPPO_GATE_OLD_FAIL=73"})
	if code != 73 {
		t.Fatalf("old preparation failure status=%d, want73", code)
	}
	for _, call := range calls {
		if slices.Contains(call.args, "./tests/e2e") || slices.Contains(call.args, "./tests/bdd") {
			t.Fatalf("E2E ran after immutable old preparation failed: %+v", call)
		}
	}
	candidate := bddGateFlag(requireBDDGateCall(t, calls, "build", "./cmd/hippo"), "-o")
	if _, err := os.Stat(filepath.Dir(candidate)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old preparation failure leaked owned E2E artifacts: %v", err)
	}
}

func requireInvalidPublicEntryMutations(t *testing.T, compiled, line, name string, check func(string) error) {
	t.Helper()
	for _, row := range []struct{ name, replacement string }{
		{"removed", ""},
		{"commented", "# " + line},
		{"wrong mode", strings.Replace(line, "HIPPO_BDD_SUITE=e2e", "HIPPO_BDD_SUITE=unit", 1)},
		{"wrong candidate", strings.Replace(line, "HIPPO_BIN=\"$temporary_dir/hippo\"", "HIPPO_BIN=other-candidate", 1)},
		{"wrong host", strings.Replace(line, "./tests/bdd", "./tests/e2e", 1)},
		{"compile only", strings.Replace(line, " test ", " test -c ", 1)},
		{"compile only alias", line + " --c=true"},
		{"race alias", line + " --race=true"},
		{"list only", line + " -list '.*'"},
		{"skipped", line + " -skip '^" + name + "$'"},
		{"duplicate run", line + " -run '^" + name + "$'"},
		{"nonexclusive run", strings.Replace(line, "^"+name+"$", name, 1)},
		{"extra host", line + " ./tests/e2e"},
		{"cached run", strings.Replace(line, "-count=1", "-count=0", 1)},
		{"duplicate count", line + " -count=1"},
		{"duplicate entry", line + "\n" + line},
		{"swallowed failure", line + " || true"},
	} {
		t.Run(row.name, func(t *testing.T) {
			if strings.Count(compiled, line+"\n") != 1 {
				t.Fatal("mutation does not identify one required entry")
			}
			if err := check(strings.Replace(compiled, line+"\n", row.replacement+"\n", 1)); err == nil {
				t.Fatal("invalid required public registration accepted")
			}
		})
	}
}
