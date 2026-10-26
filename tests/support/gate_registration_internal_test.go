package support

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/tests/contract"
)

// These fixtures test shell orchestration, not HIPPO behaviour: copied actual
// scripts call a recording Go stand-in, never build or execute a candidate.
type bddGateCall struct {
	suite, adapter, binary           string
	helper, helperSHA                string
	oldBinary, oldSHA, oldReceipt    string
	v085Binary, v085SHA, v085Receipt string
	args                             []string
}

func TestBDDGateQuickRegistration(t *testing.T) {
	calls, code := recordBDDGate(t, "scripts/test-quick.sh", "")
	if code != 0 {
		t.Fatalf("recorded quick script exited %d", code)
	}
	t.Run("ordinary tests retain exact exclusions and negatives", func(t *testing.T) {
		requireBDDGateQuickOrdinary(t, calls)
	})
	t.Run("relocated Unit executes once in measured coverage", func(t *testing.T) {
		requireBDDGateQuickMeasuredUnit(t, calls)
	})
	t.Run("static compliance uses composed registry serially", func(t *testing.T) {
		requireBDDGateQuickCompliance(t, calls)
	})
	t.Run("quick authored shell does not dispatch candidate builds or native corpora", func(t *testing.T) {
		requireBDDGateQuickNoNative(t, calls)
	})
}

func TestBDDGateFullRegistration(t *testing.T) {
	calls, code := recordBDDGate(t, "scripts/test.sh", "")
	if code != 0 {
		t.Fatalf("recorded full script exited %d", code)
	}
	t.Run("ordinary Integration and race remain", func(t *testing.T) {
		requireBDDGateFullOrdinary(t, calls)
	})
	for _, row := range []struct {
		name, suite, entry string
		race               bool
	}{
		{"Integration", "integration", "TestIntegrationBehaviours", false},
		{"Unit race", "unit", "TestUnitBehaviours", true},
		{"Integration race", "integration", "TestIntegrationBehaviours", true},
	} {
		t.Run(row.name, func(t *testing.T) {
			count := 0
			for _, call := range calls {
				if call.suite != row.suite || !slices.Contains(call.args, "./tests/bdd") || slices.Contains(call.args, "-race") != row.race {
					continue
				}
				count++
				if bddGateFlag(call, "-run") != "^"+row.entry+"$" {
					t.Errorf("wrong explicit selector: %+v", call)
				}
			}
			if count != 1 {
				t.Errorf("explicit %s executions = %d, want one", row.name, count)
			}
		})
	}
	t.Run("contract race and vulnerability scan remain", func(t *testing.T) {
		requireBDDGateFullContract(t, calls)
	})
}

func TestBDDGateE2ECandidateRegistration(t *testing.T) {
	calls, code := recordBDDGate(t, "tests/e2e/run.sh", "")
	if code != 0 {
		t.Fatalf("recorded E2E script exited %d", code)
	}
	builds := 0
	var candidate string
	for _, call := range calls {
		if call.args[0] != "build" {
			continue
		}
		builds++
		candidate = bddGateFlag(call, "-o")
		stamp := bddGateFlag(call, "-ldflags")
		if !slices.Contains(call.args, "-trimpath") || !strings.Contains(stamp, ".Version=v0.0.0-test") || !strings.Contains(stamp, ".Commit=0000000000000000000000000000000000000000") {
			t.Errorf("candidate stamp changed: %v", call.args)
		}
	}
	if builds != 1 || candidate == "" {
		t.Fatalf("builds=%d candidate=%q", builds, candidate)
	}
	ordinary := 0
	var composed []string
	for _, call := range calls {
		if call.args[0] != "test" || slices.Contains(call.args, "-c") {
			continue
		}
		if call.binary != candidate {
			t.Errorf("E2E candidate differs from build: %q != %q", call.binary, candidate)
		}
		if slices.Contains(call.args, "./tests/e2e") {
			ordinary++
			requireBDDGateSelection(t, call, []string{"TestOrdinaryE2E"}, bddGateEntryNames())
		}
		if slices.Contains(call.args, "./tests/bdd") {
			composed = append(composed, bddGateFlag(call, "-run"))
			if call.suite != "e2e" || len(bddGateFlagValues(call, "-run")) != 1 || len(bddGateFlagValues(call, "-skip")) != 0 {
				t.Errorf("wrong public corpus selection: %+v", call)
			}
			if call.adapter != "" {
				t.Errorf("direct public entry carries an explicit adapter: %+v", call)
			}
		}
	}
	wanted := []string{"^TestE2EBehaviours$"}
	if ordinary != 1 || !slices.Equal(composed, wanted) {
		t.Errorf("ordinary E2E=%d composed sequence=%v, want one ordinary and %v", ordinary, composed, wanted)
	}
	if _, err := os.Stat(filepath.Dir(candidate)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("candidate directory cleanup: %v", err)
	}
}

func TestBDDGateFailuresStopActualScripts(t *testing.T) {
	for _, row := range []struct{ script, fail string }{
		{"scripts/test-quick.sh", "test:-run"},
		{"scripts/test-quick.sh", "tool:golangci-lint"},
		{"scripts/test-quick.sh", "run:./tests/coverage"},
		{"scripts/test.sh", "tool:govulncheck"},
		{"tests/e2e/run.sh", "build:-p=1"},
		{"tests/e2e/run.sh", "test:-count=1"},
	} {
		t.Run(row.script+"/"+row.fail, func(t *testing.T) {
			calls, code := recordBDDGate(t, row.script, row.fail)
			if code != 73 {
				t.Fatalf("failure exit=%d, want 73; calls=%+v", code, calls)
			}
			last := calls[len(calls)-1]
			command, argument, _ := strings.Cut(row.fail, ":")
			if last.args[0] != command || !slices.Contains(last.args[1:], argument) {
				t.Fatalf("gate continued after failing invocation: %+v", last)
			}
		})
	}
}

type gateFixtureScripts struct{ quick, full, compiled string }

func checkGateQuickOrdinary(source string) error {
	calls, valid := gateTestCommands(source)
	if !valid {
		return errors.New("quick gate contains an invalid test command")
	}
	for _, call := range calls {
		if call.suite == contract.Integration || call.suite == contract.E2E {
			return errors.New("quick gate invokes a native executing corpus")
		}
		if slices.Contains(call.args, "./cmd/...") && slices.Contains(call.args, "./internal/...") && gateOrdinarySelection(call.args, gateOrdinaryEntries()) {
			return nil
		}
	}
	return errors.New("quick gate omits unrestricted deterministic tests with exact corpus exclusions")
}

func checkGateFullIntegration(source string) error {
	calls, valid := gateTestCommands(source)
	if !valid {
		return errors.New("full gate contains an invalid test command")
	}
	ordinary, composed := false, 0
	for _, call := range calls {
		if slices.Contains(call.args, "./tests/integration") && !slices.Contains(call.args, "-race") && len(gateFlagValues(call.args, "-run")) == 0 {
			ordinary = true
		}
		if gateExecutingEntry(call, contract.Integration, "TestIntegrationBehaviours", false) {
			composed++
		}
	}
	if !ordinary {
		return errors.New("full gate omits ordinary integration without exclusive selectors")
	}
	if composed != 1 {
		return errors.New("integration corpus requires one explicit mode and exclusive composed entry")
	}
	return nil
}

func TestBDDGateInspectorsRejectNonexecutingRegistrations(t *testing.T) {
	quick := "go test -count=1 ./cmd/... ./internal/... ./tests/support ./tests/support/hostprobe ./tests/support/oldidentity ./tests/coverage ./tests/bdd -skip '^(TestUnitBehaviours|TestIntegrationBehaviours|TestE2EBehaviours|TestFeatureCompliance)$'\n"
	integration := "HIPPO_BDD_SUITE=integration go test -count=1 -run '^TestIntegrationBehaviours$' ./tests/bdd\n"
	e2e := "HIPPO_BDD_SUITE=e2e HIPPO_BIN=\"$temporary_dir/hippo\" \"$go_binary\" test -count=1 -run '^TestE2EBehaviours$' ./tests/bdd\n"
	state := gateFixtureScripts{quick: quick, full: "go test -count=1 ./tests/integration\n" + integration + "./tests/e2e/run.sh\n", compiled: "public_old_release_prepare \"$tool_dir\" \"$temporary_dir\"\n" + e2e}
	t.Run("exact ordinary selection oracle", func(t *testing.T) {
		requireBDDGateOrdinarySelectionOracle(t)
	})
	t.Run("supported registration control", func(t *testing.T) {
		if err := checkGateQuickOrdinary(state.quick); err != nil {
			t.Error(err)
		}
		if err := checkGateFullIntegration(state.full); err != nil {
			t.Error(err)
		}
		if err := checkStandaloneGatePublicEntries(normalizedBDDGateCommands(state.compiled)); err != nil {
			t.Error(err)
		}
	})
	for _, row := range []struct{ name, target, from, to string }{
		{"ordinary exact exclusions removed", "quick", " -skip '^(TestUnitBehaviours|TestIntegrationBehaviours|TestE2EBehaviours|TestFeatureCompliance)$'", ""},
		{"ordinary exclusions broadened", "quick", "^(TestUnitBehaviours|TestIntegrationBehaviours|TestE2EBehaviours|TestFeatureCompliance)$", "Test"},
		{"Integration mode changed", "full", "HIPPO_BDD_SUITE=integration", "HIPPO_BDD_SUITE=integrationX"},
		{"Integration only commented", "full", integration, "# " + integration},
		{"Integration wrong entry", "full", "^TestIntegrationBehaviours$", "^TestUnitBehaviours$"},
		{"Integration exclusive selector missing", "full", " -run '^TestIntegrationBehaviours$'", ""},
		{"Integration failure swallowed", "full", strings.TrimSuffix(integration, "\n"), strings.TrimSuffix(integration, "\n") + " || true"},
		{"E2E mode changed", "compiled", e2e, strings.Replace(e2e, "HIPPO_BDD_SUITE=e2e", "HIPPO_BDD_SUITE=e2eX", 1)},
		{"E2E only commented", "compiled", e2e, "# " + e2e},
		{"E2E wrong entry", "compiled", "^TestE2EBehaviours$", "^TestUnitBehaviours$"},
		{"E2E exclusive selector missing", "compiled", " -run '^TestE2EBehaviours$'", ""},
		{"E2E failure swallowed", "compiled", strings.TrimSuffix(e2e, "\n"), strings.TrimSuffix(e2e, "\n") + " || true"},
	} {
		t.Run(row.name, func(t *testing.T) {
			mutated := state
			var source *string
			switch row.target {
			case "quick":
				source = &mutated.quick
			case "full":
				source = &mutated.full
			case "compiled":
				source = &mutated.compiled
			}
			if source == nil || strings.Count(*source, row.from) != 1 {
				t.Fatal("mutation does not name exactly one registration")
			}
			*source = strings.Replace(*source, row.from, row.to, 1)
			// The require functions receive normalized command text. Comment
			// mutations are existing rejection controls, not RED.
			*source = normalizedBDDGateCommands(*source)
			var err error
			switch row.target {
			case "quick":
				err = checkGateQuickOrdinary(*source)
			case "full":
				err = checkGateFullIntegration(*source)
			case "compiled":
				err = checkStandaloneGatePublicEntries(*source)
			}
			if err == nil {
				t.Fatal("nonexecuting, wrong-boundary or overbroad registration accepted")
			}
		})
	}
}

func TestBDDGateCoreCoverageRequiresRelocatedUnit(t *testing.T) {
	script := lintWiringDriver(t).lintCommand
	if strings.Count(script, "./tests/unit ") != 1 {
		t.Fatal("fixture must identify one measured package selector")
	}
	positive := script
	if !validCoreCoverageSelectors(positive) {
		t.Fatal("existing denominator and relocated Unit host rejected")
	}
	for _, row := range []struct{ name, from, to string }{
		{"relocated host removed", "./tests/unit ./tests/bdd ", "./tests/unit "},
		{"Unit entry excluded", "^(TestIntegrationBehaviours|TestE2EBehaviours|TestFeatureCompliance)$", "^(TestUnitBehaviours|TestIntegrationBehaviours|TestE2EBehaviours|TestFeatureCompliance)$"},
		{"coverage made static-only", "-coverprofile=coverage/unit.out", "-run '^TestFeatureCompliance$' -coverprofile=coverage/unit.out"},
	} {
		t.Run(row.name, func(t *testing.T) {
			if strings.Count(positive, row.from) != 1 {
				t.Fatal("coverage mutation does not identify exactly one selector")
			}
			if validCoreCoverageSelectors(strings.Replace(positive, row.from, row.to, 1)) {
				t.Fatal("measured Unit corpus was omitted but coverage selector accepted")
			}
		})
	}
}

func TestBDDGateLegacyCorpusEntriesRetired(t *testing.T) {
	root, err := toolRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ path, name string }{
		{"tests/unit/behaviour_test.go", "TestUnitBehaviours"},
		{"tests/integration/behaviour_test.go", "TestIntegrationBehaviours"},
		{"tests/e2e/behaviour_test.go", "TestE2EBehaviours"},
		{"tests/bdd/compliance_test.go", "TestFeatureCompliance"},
	} {
		t.Run(row.name, func(t *testing.T) {
			file, parseErr := parser.ParseFile(token.NewFileSet(), filepath.Join(root, filepath.FromSlash(row.path)), nil, 0)
			if errors.Is(parseErr, os.ErrNotExist) && row.name != "TestFeatureCompliance" {
				return
			}
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			for _, declaration := range file.Decls {
				if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == row.name {
					t.Errorf("duplicate whole-corpus entry remains at %s", row.path)
				}
			}
			if row.name == "TestFeatureCompliance" {
				requireBDDGateNegativeValidators(t, file)
			}
		})
	}
}

func bddGateEntryNames() []string {
	return []string{"TestUnitBehaviours", "TestIntegrationBehaviours", "TestE2EBehaviours", "TestFeatureCompliance"}
}

func bddGateOrdinaryEntryNames() []string {
	return bddGateEntryNames()
}

func bddGateFlag(call bddGateCall, name string) string {
	values := bddGateFlagValues(call, name)
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

func bddGateFlagValues(call bddGateCall, name string) []string {
	var values []string
	for index := 0; index < len(call.args); index++ {
		argument := call.args[index]
		// Match cmd/go's two-dash reduction and the shared test.run/test.skip
		// aliases. Count every accepted spelling, even when mixed with plain flags.
		if strings.HasPrefix(argument, "--") {
			argument = argument[1:]
		}
		if (name == "-run" || name == "-skip") && strings.HasPrefix(argument, "-test.") {
			argument = "-" + strings.TrimPrefix(argument, "-test.")
		}
		if value, found := strings.CutPrefix(argument, name+"="); found {
			values = append(values, value)
		} else if argument == name {
			if index+1 >= len(call.args) {
				return []string{"invalid selector"}
			}
			index++
			values = append(values, call.args[index])
		}
	}
	return values
}

func bddGateNamesPackage(call bddGateCall, pkg string) bool {
	if call.args[0] != "test" || bddGateFlag(call, "-run") == "^$" {
		return false
	}
	return slices.ContainsFunc(call.args, func(pattern string) bool { return patternMatches(pattern, pkg) })
}

func bddGateSelects(call bddGateCall, name string) bool {
	run, skip := bddGateFlag(call, "-run"), bddGateFlag(call, "-skip")
	if run != "" {
		pattern, err := regexp.Compile(run)
		if err != nil || !pattern.MatchString(name) {
			return false
		}
	}
	if skip != "" {
		pattern, err := regexp.Compile(skip)
		if err != nil || pattern.MatchString(name) {
			return false
		}
	}
	return true
}

func requireBDDGateSelection(t *testing.T, call bddGateCall, included, excluded []string) {
	t.Helper()
	if !bddGateSelectionIsExact(call, excluded) {
		t.Errorf("ordinary/measured selection must have unrestricted -run and exact anchored corpus-only -skip: %v", call.args)
	}
	for _, name := range append(included, "TestOrdinaryDirect", "TestUnitBehavioursExtra", "PrefixTestFeatureCompliance", "TestAnnotationCanonicalExecutes") {
		if !bddGateSelects(call, name) {
			t.Errorf("ordinary test %s excluded: %v", name, call.args)
		}
	}
	for _, name := range excluded {
		if bddGateSelects(call, name) {
			t.Errorf("corpus entry %s not excluded: %v", name, call.args)
		}
	}
}

func bddGateSelectionIsExact(call bddGateCall, excluded []string) bool {
	run, skip := bddGateFlagValues(call, "-run"), bddGateFlagValues(call, "-skip")
	if len(run) > 1 || (len(run) == 1 && run[0] != "" && run[0] != ".*" && run[0] != "^.*$") || len(skip) != 1 {
		return false
	}
	inner, begin := strings.CutPrefix(skip[0], "^(")
	inner, end := strings.CutSuffix(inner, ")$")
	if !begin || !end {
		return false
	}
	actual := strings.Split(inner, "|")
	wanted := append([]string(nil), excluded...)
	slices.Sort(actual)
	slices.Sort(wanted)
	return slices.Equal(actual, wanted)
}

// Match the current inspectTUIGates preprocessing exactly. This source-only
// fixture intentionally does not refactor or strengthen the production scanner.
func normalizedBDDGateCommands(script string) string {
	var commands []string
	for line := range strings.SplitSeq(strings.ReplaceAll(script, "\\\n", ""), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			commands = append(commands, line)
		}
	}
	return strings.Join(commands, "\n")
}

func requireBDDGateCall(t *testing.T, calls []bddGateCall, command, argument string) bddGateCall {
	t.Helper()
	for _, call := range calls {
		if call.args[0] == command && slices.Contains(call.args, argument) {
			return call
		}
	}
	t.Fatalf("missing %s invocation containing %s: %+v", command, argument, calls)
	return bddGateCall{}
}

// v0100ReleaseStandIn records the v0.10.0 preparation without acquiring an archive.
const v0100ReleaseStandIn = `#!/bin/sh
# Recording stand-in only: actual provenance belongs to preparation controls.
public_v0100_release_prepare() {
 [ "${HIPPO_GATE_V0100_FAIL:-}" != 73 ] || return 73
 HIPPO_PUBLIC_V0100_BIN=$2/recorded-v0100-hippo
 HIPPO_PUBLIC_V0100_BIN_SHA256=recorded-v0100-digest
 HIPPO_PUBLIC_V0100_VERSION_RECEIPT=$2/recorded-v0100-version
 export HIPPO_PUBLIC_V0100_BIN HIPPO_PUBLIC_V0100_BIN_SHA256 HIPPO_PUBLIC_V0100_VERSION_RECEIPT
 printf '%s\n' V0100_PREPARED >>"$HIPPO_GATE_RECORD"
}
`

func recordBDDGate(t *testing.T, script, fail string) ([]bddGateCall, int) {
	t.Helper()
	return recordBDDGateInputs(t, script, fail, nil)
}

func recordBDDGateInputs(t *testing.T, script, fail string, inputs []string) ([]bddGateCall, int) {
	t.Helper()
	owner, err := toolRoot()
	if err != nil {
		t.Fatal(err)
	}
	root, fixtureRoot := newBDDGateFixture(t)
	write := func(path string, content []byte) {
		t.Helper()
		relative, err := filepath.Rel(root, filepath.Join(root, filepath.FromSlash(path)))
		if err != nil || !filepath.IsLocal(relative) {
			t.Fatalf("gate fixture path escapes owned root: %q: %v", path, err)
		}
		if err := fixtureRoot.MkdirAll(filepath.Dir(relative), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := fixtureRoot.WriteFile(relative, content, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range testRunningScripts {
		data, err := os.ReadFile(filepath.Join(owner, path))
		if err != nil {
			t.Fatal(err)
		}
		write(path, data)
	}
	helperPreparation, err := os.ReadFile(filepath.Join(owner, "tests/support/hostprobe/prepare.sh"))
	if err != nil {
		t.Fatal(err)
	}
	write("tests/support/hostprobe/prepare.sh", helperPreparation)
	write("tests/e2e/old-release.sh", []byte(`#!/bin/sh
# Recording stand-in only: no archive or identity acceptance is established.
public_old_release_prepare() {
 [ "${HIPPO_GATE_OLD_FAIL:-}" != 73 ] || return 73
 HIPPO_PUBLIC_OLD_BIN=$2/recorded-old-hippo
 HIPPO_PUBLIC_OLD_BIN_SHA256=recorded-old-digest
 HIPPO_PUBLIC_OLD_VERSION_RECEIPT=$2/recorded-old-version
 export HIPPO_PUBLIC_OLD_BIN HIPPO_PUBLIC_OLD_BIN_SHA256 HIPPO_PUBLIC_OLD_VERSION_RECEIPT
 printf '%s\n' OLD_PREPARED >>"$HIPPO_GATE_RECORD"
}
`))
	write("tests/e2e/v085-release.sh", []byte(`#!/bin/sh
# Recording stand-in only: actual provenance belongs to preparation controls.
public_v085_release_prepare() {
 [ "${HIPPO_GATE_V085_FAIL:-}" != 73 ] || return 73
 HIPPO_PUBLIC_V085_BIN=$2/recorded-v085-hippo
 HIPPO_PUBLIC_V085_BIN_SHA256=recorded-v085-digest
 HIPPO_PUBLIC_V085_VERSION_RECEIPT=$2/recorded-v085-version
 export HIPPO_PUBLIC_V085_BIN HIPPO_PUBLIC_V085_BIN_SHA256 HIPPO_PUBLIC_V085_VERSION_RECEIPT
 printf '%s\n' V085_PREPARED >>"$HIPPO_GATE_RECORD"
}
`))
	write("tests/e2e/v0100-release.sh", []byte(v0100ReleaseStandIn))
	for _, path := range []string{"go.mod", "go.sum", "tests/support/hostprobe/protocol.go"} {
		write(path, []byte("recording preparation source fixture\n"))
	}
	for _, path := range []string{"scripts/check-worktree-layout.sh", "scripts/format-check.sh", "tests/artifacts/run.sh", ".commandcode/hooks/agent-policy-selector.test.sh", ".commandcode/hooks/git-fixture-isolation.test.sh", ".muse/hooks/run-policy-hook.test.sh"} {
		write(path, []byte("#!/bin/sh\nset -eu\nexit 0\n"))
	}
	write("bin/go", []byte(`#!/bin/sh
set -eu
{
printf '%s\n' CALL "suite=${HIPPO_BDD_SUITE:-}" "adapter=${HIPPO_BDD_ADAPTER:-}" "binary=${HIPPO_BIN:-}" "helper=${HIPPO_PUBLIC_HOST_HELPER:-}" "helper-sha=${HIPPO_PUBLIC_HOST_HELPER_SHA256:-}" "old-binary=${HIPPO_PUBLIC_OLD_BIN:-}" "old-sha=${HIPPO_PUBLIC_OLD_BIN_SHA256:-}" "old-receipt=${HIPPO_PUBLIC_OLD_VERSION_RECEIPT:-}" "v085-binary=${HIPPO_PUBLIC_V085_BIN:-}" "v085-sha=${HIPPO_PUBLIC_V085_BIN_SHA256:-}" "v085-receipt=${HIPPO_PUBLIC_V085_VERSION_RECEIPT:-}"
for arg
do
printf 'arg=%s\n' "$arg"
done
} >> "$HIPPO_GATE_RECORD"
if [ "${1:-}:${2:-}" = "${HIPPO_GATE_FAIL_MATCH:-}" ]; then exit 73; fi
for arg
do
 if [ "${1:-}:$arg" = "${HIPPO_GATE_FAIL_MATCH:-}" ]; then exit 73; fi
done
if [ "${1:-}:${2:-}" = 'test:-c' ]; then
 output=
 while [ "$#" -gt 0 ]; do
  if [ "$1" = '-o' ]; then shift; output=$1; break; fi
  shift
 done
 [ -n "$output" ]
 printf '#!/bin/sh\nexit 0\n' >"$output"
 chmod 700 "$output"
fi
if [ "${1:-}" = 'list' ]; then
 printf '%s\n' "${HIPPO_GATE_HELPER_CLOSURE:-github.com/wahidyankf/hippo/tests/support/hostprobe}"
fi
`))
	log := filepath.Join(root, "calls")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/bin/sh", filepath.Join(root, filepath.FromSlash(script)))
	command.Env = []string{"PATH=" + filepath.Join(root, "bin") + ":/usr/bin:/bin", "TMPDIR=" + root, "HIPPO_E2E_TEMP_PARENT=" + root, "HIPPO_GO_BINARY=" + filepath.Join(root, "bin/go"), "HIPPO_BIN=inherited-candidate-must-not-be-used", "HIPPO_GATE_RECORD=" + log, "HIPPO_GATE_FAIL_MATCH=" + fail}
	command.Env = append(command.Env, inputs...)
	output, err := command.CombinedOutput()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("start gate fixture: %v: %s", err, output)
		}
		code = exit.ExitCode()
	}
	if ctx.Err() != nil {
		t.Fatalf("gate fixture exceeded bound: %v: %s", ctx.Err(), output)
	}
	data, err := os.ReadFile(log)
	if errors.Is(err, os.ErrNotExist) {
		return nil, code
	}
	if err != nil {
		t.Fatal(err)
	}
	return parseBDDGateRecording(t, data), code
}

func requireBDDGateQuickOrdinary(t *testing.T, calls []bddGateCall) {
	t.Helper()
	call := requireBDDGateCall(t, calls, "test", "./cmd/...")
	requireBDDGateSelection(t, call, nil, bddGateOrdinaryEntryNames())
	for _, pkg := range []string{"./internal/...", "./tests/support", "./tests/support/hostprobe", "./tests/support/oldidentity", "./tests/coverage", "./tests/bdd"} {
		if !slices.Contains(call.args, pkg) {
			t.Errorf("ordinary package %s missing: %v", pkg, call.args)
		}
	}
}

func requireBDDGateQuickMeasuredUnit(t *testing.T, calls []bddGateCall) {
	t.Helper()
	var unitCalls []bddGateCall
	for _, call := range calls {
		if bddGateNamesPackage(call, "./tests/bdd") && bddGateSelects(call, "TestUnitBehaviours") {
			unitCalls = append(unitCalls, call)
		}
	}
	if len(unitCalls) != 1 {
		t.Fatalf("Unit executions = %d, want one: %+v", len(unitCalls), unitCalls)
	}
	call := unitCalls[0]
	if bddGateFlag(call, "-coverprofile") != "coverage/unit.out" {
		t.Errorf("Unit is not measured: %v", call.args)
	}
	requireBDDGateSelection(t, call, []string{"TestUnitBehaviours", "TestOrdinaryDirect"}, bddGateOrdinaryEntryNames()[1:])
	for _, pkg := range []string{"./tests/unit", "./internal/application", "./internal/domain/coordination", "./internal/domain/evidence", "./internal/identity", "./internal/status"} {
		if !slices.Contains(call.args, pkg) {
			t.Errorf("measured package %s missing", pkg)
		}
	}
}

func requireBDDGateQuickCompliance(t *testing.T, calls []bddGateCall) {
	t.Helper()
	var adapters []string
	for _, call := range calls {
		if call.adapter == "" {
			continue
		}
		adapters = append(adapters, call.adapter)
		if !slices.Contains(call.args, "./tests/bdd") || bddGateFlag(call, "-run") != "^TestFeatureCompliance$" {
			t.Errorf("static check uses wrong host/selector: %+v", call)
		}
		if call.binary != "inherited-candidate-must-not-be-used" {
			t.Errorf("static check was handed a built candidate: %+v", call)
		}
	}
	if !slices.Equal(adapters, []string{"unit", "integration", "e2e"}) {
		t.Errorf("serial compliance order = %v", adapters)
	}
}

func requireBDDGateQuickNoNative(t *testing.T, calls []bddGateCall) {
	t.Helper()
	for _, call := range calls {
		if call.args[0] == "build" || call.suite == "integration" || call.suite == "e2e" {
			t.Errorf("quick authored shell has native/build invocation: %+v", call)
		}
	}
}

func requireBDDGateFullOrdinary(t *testing.T, calls []bddGateCall) {
	t.Helper()
	var ordinary, race bool
	for _, call := range calls {
		if !slices.Contains(call.args, "./tests/integration") {
			continue
		}
		if run := bddGateFlag(call, "-run"); run == "^TestMixedVersionV0100VeryHeavyCompatibility$" {
			continue // The separate required leaf is checked by its registration control.
		}
		excluded := bddGateEntryNames()
		if slices.Contains(call.args, "./internal/...") {
			excluded = bddGateOrdinaryEntryNames()
		}
		requireBDDGateSelection(t, call, []string{"TestOrdinaryIntegration"}, excluded)
		if slices.Contains(call.args, "-race") {
			race = true
			for _, pkg := range []string{"./cmd/...", "./internal/...", "./tests/support", "./tests/support/hostprobe", "./tests/support/oldidentity", "./tests/unit", "./tests/bdd"} {
				if !slices.Contains(call.args, pkg) {
					t.Errorf("ordinary race package missing: %s", pkg)
				}
			}
		} else {
			ordinary = true
		}
	}
	if !ordinary || !race {
		t.Errorf("ordinary Integration=%t race=%t", ordinary, race)
	}
}

func requireBDDGateFullContract(t *testing.T, calls []bddGateCall) {
	t.Helper()
	call := requireBDDGateCall(t, calls, "test", "./tests/contract")
	var contractRace bool
	for _, candidate := range calls {
		if slices.Contains(candidate.args, "./tests/contract") && slices.Contains(candidate.args, "-race") {
			contractRace = true
		}
	}
	if !contractRace || !slices.Contains(call.args, "-count=1") {
		t.Error("contract obligations missing")
	}
	requireBDDGateCall(t, calls, "tool", "govulncheck")
}

func requireBDDGateOrdinarySelectionOracle(t *testing.T) {
	t.Helper()
	positive := bddGateCall{args: []string{"test", "-skip", "^(TestUnitBehaviours|TestIntegrationBehaviours|TestE2EBehaviours|TestFeatureCompliance)$", "./internal/..."}}
	if !bddGateSelectionIsExact(positive, bddGateOrdinaryEntryNames()) || !bddGateSelects(positive, "TestAnnotationCanonicalExecutes") {
		t.Fatal("exact corpus exclusions must retain real ordinary annotation entry")
	}
	for _, args := range [][]string{
		{"test", "-skip", "^(TestUnitBehaviours|TestIntegrationBehaviours|TestE2EBehaviours|TestFeatureCompliance|TestAnnotationCanonicalExecutes)$", "./internal/..."},
		{"test", "-skip", "^(TestUnitBehaviours|TestIntegrationBehaviours|TestE2EBehaviours|TestFeatureCompliance)$", "-run", "^(TestOrdinaryDirect|TestUnitBehavioursExtra|PrefixTestFeatureCompliance)$", "./internal/..."},
		{"test", "-skip", "^(TestUnitBehaviours|TestIntegrationBehaviours|TestE2EBehaviours|TestFeatureCompliance)$", "-skip", "^TestAnnotationCanonicalExecutes$", "./internal/..."},
		{"test", "-skip", "^(TestUnitBehaviours|TestIntegrationBehaviours|TestE2EBehaviours|TestFeatureCompliance)$", "-run", "", "-run", "^TestOrdinaryDirect$", "./internal/..."},
	} {
		if bddGateSelectionIsExact(bddGateCall{args: args}, bddGateOrdinaryEntryNames()) {
			t.Fatalf("ordinary test restriction accepted: %v", args)
		}
	}
	requireBDDGateSelectorAliases(t, positive)
}

func requireBDDGateSelectorAliases(t *testing.T, positive bddGateCall) {
	t.Helper()
	for _, prefix := range []string{"-", "--", "-test.", "--test."} {
		for _, equal := range []bool{false, true} {
			flag := func(name, value string) []string {
				if equal {
					return []string{prefix + name + "=" + value}
				}
				return []string{prefix + name, value}
			}
			args := make([]string, 1, 9)
			args[0] = "test"
			args = append(args, flag("skip", "^(TestUnitBehaviours|TestIntegrationBehaviours|TestE2EBehaviours|TestFeatureCompliance)$")...)
			args = append(args, flag(runCommandName, ".*")...)
			call := bddGateCall{args: args}
			if !bddGateSelectionIsExact(call, bddGateOrdinaryEntryNames()) || !bddGateSelects(call, "TestAnnotationCanonicalExecutes") {
				t.Fatalf("healthy selector alias rejected: %v", args)
			}
			annotationSkip := flag("skip", "^TestAnnotationCanonicalExecutes$")
			narrowRun := flag(runCommandName, "^(TestOrdinaryDirect|TestUnitBehavioursExtra|PrefixTestFeatureCompliance)$")
			for _, restriction := range [][]string{annotationSkip, narrowRun} {
				mutated := bddGateCall{args: append(append([]string(nil), positive.args...), restriction...)}
				if bddGateSelectionIsExact(mutated, bddGateOrdinaryEntryNames()) || bddGateSelects(mutated, "TestAnnotationCanonicalExecutes") {
					t.Fatalf("alias restriction ignored: %v", mutated.args)
				}
			}
		}
	}
}

func requireBDDGateNegativeValidators(t *testing.T, file *ast.File) {
	t.Helper()
	for _, name := range []string{"TestScenarioWithoutActionAndOutcomeIsRejected", "TestUndefinedBindingIsRejected", "TestAmbiguousBindingIsRejected", "TestUnusedBindingIsRejected", "TestInvalidBindingExpressionIsRejected", "TestMissingBindingHandlerIsRejected", "TestNonFunctionBindingHandlerIsRejected", "TestBindingOrderDoesNotAffectValidation", "TestUnapprovedExemptionIsRejected", "TestUnitExemptionTagIsRejected", "TestUnitExemptionInventoryIsRejected", "TestExemptionRequiresBoundaryAndReason"} {
		found := false
		for _, declaration := range file.Decls {
			if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == name {
				found = true
			}
		}
		if !found {
			t.Errorf("negative validation test removed: %s", name)
		}
	}
}

func newBDDGateFixture(t *testing.T) (string, *os.Root) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "gate fixture with spaces")
	if err := os.Mkdir(root, 0o750); err != nil {
		t.Fatal(err)
	}
	fixtureRoot, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fixtureRoot.Close(); err != nil {
			t.Errorf("close owned gate fixture root: %v", err)
		}
	})
	return root, fixtureRoot
}

func TestBDDGateLeanHelperPreparedOnceAndSharedBeforeDependentTests(t *testing.T) {
	for _, script := range testRunningScripts {
		t.Run(script, func(t *testing.T) {
			calls, code := recordBDDGate(t, script, "")
			if code != 0 {
				t.Fatalf("gate failed: %d", code)
			}
			builds := 0
			helper := ""
			digest := ""
			for _, call := range calls {
				checkPreparedBDDGateHelper(t, call, &builds, &helper, &digest)
			}
			if builds != 1 {
				t.Fatalf("helper builds=%d", builds)
			}
			if _, err := os.Stat(filepath.Dir(helper)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("owner helper cleanup: %v", err)
			}
		})
	}
}

func TestBDDGateLeanHelperBuildFailureStopsBeforeDependentTests(t *testing.T) {
	for _, script := range testRunningScripts {
		calls, code := recordBDDGate(t, script, "test:-c")
		if code != 73 || len(calls) == 0 {
			t.Fatalf("helper build failure lost: %s %d", script, code)
		}
		last := calls[len(calls)-1]
		if last.args[0] != "test" || last.args[1] != "-c" {
			t.Fatalf("gate continued after helper build refusal: %+v", last)
		}
		if _, err := os.Stat(filepath.Dir(bddGateFlag(last, "-o"))); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("failed helper directory survived: %v", err)
		}
	}
}

func TestBDDGateInvalidInheritedHelperRefusesWithoutRebuild(t *testing.T) {
	for _, inputs := range [][]string{
		{"HIPPO_PUBLIC_HOST_HELPER=", "HIPPO_PUBLIC_HOST_HELPER_SHA256="},
		{"HIPPO_PUBLIC_HOST_HELPER=/missing/lean-helper"},
		{"HIPPO_PUBLIC_HOST_HELPER_SHA256=" + strings.Repeat("a", 64)},
		{"HIPPO_PUBLIC_HOST_HELPER=relative/helper", "HIPPO_PUBLIC_HOST_HELPER_SHA256=" + strings.Repeat("a", 64)},
	} {
		for _, script := range testRunningScripts {
			calls, code := recordBDDGateInputs(t, script, "", inputs)
			if code == 0 || len(calls) != 0 {
				t.Fatalf("invalid inherited helper caused fallback/rebuild: %s calls=%+v exit=%d", script, calls, code)
			}
		}
	}
}

func TestBDDGateLeanHelperClosureRefusesHeavyImports(t *testing.T) {
	for _, script := range testRunningScripts {
		calls, code := recordBDDGateInputs(t, script, "", []string{"HIPPO_GATE_HELPER_CLOSURE=github.com/mattn/go-runewidth"})
		if code == 0 || len(calls) == 0 {
			t.Fatalf("heavy helper closure passed: %s %d", script, code)
		}
		builds := 0
		for _, call := range calls {
			if call.args[0] == "test" {
				if !slices.Contains(call.args, "-c") {
					t.Fatalf("dependent test ran after invalid closure: %+v", call)
				}
				builds++
			}
		}
		if builds != 1 {
			t.Fatalf("closure refusal rebuilt helper: %d", builds)
		}
	}
}

func TestBDDGateInheritedHelperDigestMismatchRefusesWithoutRebuild(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "helper with spaces")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, script := range testRunningScripts {
		calls, code := recordBDDGateInputs(t, script, "", []string{"HIPPO_PUBLIC_HOST_HELPER=" + executable, "HIPPO_PUBLIC_HOST_HELPER_SHA256=" + strings.Repeat("0", 64)})
		if code == 0 || len(calls) != 0 {
			t.Fatalf("digest mismatch fell back to build: %s %d %+v", script, code, calls)
		}
		if _, err := os.Stat(executable); err != nil {
			t.Fatal("child deleted inherited helper", err)
		}
	}
}

// The helper source is deliberately absent. This drives the actual earliest
// dot/source refusal before any mktemp acquisition or Go invocation.
func TestBDDGateE2EEarlyHelperSourceFailureAcquiresNoTemporaryDirectory(t *testing.T) {
	owner, err := toolRoot()
	if err != nil {
		t.Fatal(err)
	}
	root, fixtureRoot := newBDDGateFixture(t)
	script, err := os.ReadFile(filepath.Join(owner, "tests/e2e/run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	subject := filepath.Join(root, "tests/e2e/run.sh")
	if err := os.MkdirAll(filepath.Dir(subject), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := fixtureRoot.WriteFile("tests/e2e/run.sh", script, 0o700); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	recording := `#!/bin/sh
set -eu
actual=$(/usr/bin/mktemp "$@")
printf 'actual temporary marker\n' >"$actual/actual-marker"
printf '%s\n' "$actual" >>"$E2E_TEMP_RECORD"
printf '%s\n' "$actual"
`
	if err := os.WriteFile(filepath.Join(bin, "mktemp"), []byte(recording), 0o700); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(root, "created-temporary-directory")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/bin/sh", subject)
	command.Dir = root
	command.Env = []string{"PATH=" + bin + ":/usr/bin:/bin", "TMPDIR=" + root, "HIPPO_E2E_TEMP_PARENT=" + root, "E2E_TEMP_RECORD=" + record}
	command.WaitDelay = 3 * time.Second
	output, runError := command.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(runError, &exit) || exit.ExitCode() == 0 || ctx.Err() != nil || !strings.Contains(string(output), "prepare.sh") || strings.Contains(string(output), "public_host_helper_cleanup: not found") {
		t.Fatalf("original source refusal was replaced by cleanup/setup failure: %v %q", runError, output)
	}
	if _, err := os.Stat(record); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("mktemp ran before passive helper source: %v", err)
	}
	acquired, err := filepath.Glob(filepath.Join(root, "hippo-e2e.*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(acquired) != 0 {
		t.Fatalf("early source failure acquired an E2E artifact: %v", acquired)
	}
}

func parseBDDGateRecording(t *testing.T, data []byte) []bddGateCall {
	t.Helper()
	var calls []bddGateCall
	for line := range strings.SplitSeq(strings.TrimSuffix(string(data), "\n"), "\n") {
		if line == "OLD_PREPARED" || line == "V085_PREPARED" || line == "V0100_PREPARED" {
			continue
		}
		if line == "CALL" {
			calls = append(calls, bddGateCall{})
			continue
		}
		if len(calls) == 0 {
			t.Fatalf("invalid recording: %q", line)
		}
		call := &calls[len(calls)-1]
		name, value, found := strings.Cut(line, "=")
		if !found {
			t.Fatalf("invalid recording: %q", line)
		}
		switch name {
		case "suite":
			call.suite = value
		case "adapter":
			call.adapter = value
		case "binary":
			call.binary = value
		case "helper":
			call.helper = value
		case "helper-sha":
			call.helperSHA = value
		case "old-binary":
			call.oldBinary = value
		case "old-sha":
			call.oldSHA = value
		case "old-receipt":
			call.oldReceipt = value
		case "v085-binary":
			call.v085Binary = value
		case "v085-sha":
			call.v085SHA = value
		case "v085-receipt":
			call.v085Receipt = value
		case "arg":
			call.args = append(call.args, value)
		default:
			t.Fatalf("unknown recording field %q", name)
		}
	}
	for _, call := range calls {
		if len(call.args) < 1 {
			t.Fatalf("incomplete invocation: %+v", call)
		}
	}
	return calls
}

func checkPreparedBDDGateHelper(t *testing.T, call bddGateCall, builds *int, helper, digest *string) {
	t.Helper()
	if call.args[0] == "test" && slices.Contains(call.args, "-c") {
		*builds++
		if *builds != 1 || !slices.Contains(call.args, "-race") || !slices.Contains(call.args, "-p=1") || !slices.Contains(call.args, "-trimpath") || !slices.Contains(call.args, "-buildvcs=false") || !slices.Contains(call.args, "./tests/support/hostprobe") {
			t.Fatalf("helper preparation changed: %+v", call)
		}
		*helper = bddGateFlag(call, "-o")
		return
	}
	if call.args[0] == "test" {
		if *builds != 1 || *helper == "" || call.helper != *helper || len(call.helperSHA) != 64 {
			t.Fatalf("dependent test ran without one pinned prepared helper: %+v", call)
		}
		if *digest == "" {
			*digest = call.helperSHA
		}
		if call.helperSHA != *digest {
			t.Fatal("child selected another helper digest")
		}
	}
}
