package support

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const mixedVersionLeafSelector = "^TestMixedVersionV085DisplayInteroperability$"

// These recording-shell checks prove the pull-request gate's bounded share of the
// v0.8.5 compatibility leaf. The main workflow still runs the same selector through
// tests/e2e/run.sh after merge, so the pull request gate never replaces that run.
func TestPullRequestMixedVersionLeafRunsExactlyOnceWithCandidate(t *testing.T) {
	calls, code := recordBDDGate(t, "tests/e2e/mixed-version.sh", "")
	if code != 0 {
		t.Fatalf("recorded pull-request mixed-version script exited %d", code)
	}
	candidate := bddGateFlag(requireBDDGateCall(t, calls, "build", "./cmd/hippo"), "-o")
	count := 0
	for _, call := range calls {
		if bddGateFlag(call, "-run") != mixedVersionLeafSelector {
			continue
		}
		count++
		if call.args[0] != "test" || call.binary != candidate || bddGateFlag(call, "-tags") != "mixedversion" || bddGateFlag(call, "-count") != "1" || bddGateFlag(call, "-timeout") != "120s" || bddGateFlag(call, "-p") != "2" {
			t.Errorf("pull-request leaf lost uncached bounded candidate routing: %+v", call)
		}
		if !slices.Contains(call.args, "./tests/integration") || len(bddGateFlagValues(call, "-run")) != 1 || len(bddGateFlagValues(call, "-skip")) != 0 || slices.Contains(call.args, "-c") {
			t.Errorf("pull-request leaf became nonexecuting or lost its sole selector: %+v", call)
		}
		if call.v085Binary != filepath.Join(filepath.Dir(candidate), "recorded-v085-hippo") || call.v085SHA != "recorded-v085-digest" || call.v085Receipt != filepath.Join(filepath.Dir(candidate), "recorded-v085-version") {
			t.Errorf("pull-request leaf used inherited inputs or lost v0.8.5 preparation: %+v", call)
		}
	}
	if count != 1 {
		t.Fatalf("pull-request v0.8.5 leaf executions=%d, want one", count)
	}
	for _, call := range calls {
		if call.args[0] == "test" && !slices.Contains(call.args, "-c") && bddGateFlag(call, "-run") != mixedVersionLeafSelector {
			t.Fatalf("pull-request mixed-version script ran a broader test than its leaf: %+v", call)
		}
	}
}

func TestPullRequestMixedVersionLeafFailureStopsAndCleansCandidate(t *testing.T) {
	calls, code := recordBDDGate(t, "tests/e2e/mixed-version.sh", "test:"+mixedVersionLeafSelector)
	if code != 73 || len(calls) == 0 || bddGateFlag(calls[len(calls)-1], "-run") != mixedVersionLeafSelector {
		t.Fatalf("pull-request compatibility failure was not preserved: exit=%d calls=%+v", code, calls)
	}
	candidate := bddGateFlag(requireBDDGateCall(t, calls, "build", "./cmd/hippo"), "-o")
	if _, err := os.Stat(filepath.Dir(candidate)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed pull-request compatibility leaf leaked gate-owned candidate: %v", err)
	}
}

func TestPullRequestMixedVersionPreparationFailureStopsBeforeLeaf(t *testing.T) {
	calls, code := recordBDDGateInputs(t, "tests/e2e/mixed-version.sh", "", []string{"HIPPO_GATE_V085_FAIL=73"})
	if code != 73 {
		t.Fatalf("pull-request v0.8.5 preparation failure exit=%d, want 73", code)
	}
	for _, call := range calls {
		if call.args[0] == "test" && !slices.Contains(call.args, "-c") {
			t.Fatalf("pull-request gate tested after unqualified v0.8.5 preparation: %+v", call)
		}
	}
	candidate := bddGateFlag(requireBDDGateCall(t, calls, "build", "./cmd/hippo"), "-o")
	if _, err := os.Stat(filepath.Dir(candidate)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed pull-request v0.8.5 preparation leaked candidate directory: %v", err)
	}
}

// The pull-request gate gains exactly one job on the operating system it already
// uses, and the single required check waits for it. No full suite and no matrix.
func TestPullRequestGateRunsMixedVersionLeafOnItsOwnJob(t *testing.T) {
	root, err := toolRoot()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "pr-quality-gate.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	if !containsAll(workflow,
		"\n  mixed-version:\n",
		"name: Mixed-version compatibility",
		"runs-on: ubuntu-24.04",
		"timeout-minutes: 20",
		"run: ./tests/e2e/mixed-version.sh",
		"        release-build,\n        mixed-version,\n      ]",
	) {
		t.Fatal("pull-request gate does not run the mixed-version leaf on ubuntu-24.04 and wait for it in the quality gate")
	}
	if strings.Contains(workflow, "./scripts/test.sh") || strings.Count(workflow, "mixed-version:") != 1 {
		t.Fatal("pull-request gate runs the full suite or declares the mixed-version job more than once")
	}
}
