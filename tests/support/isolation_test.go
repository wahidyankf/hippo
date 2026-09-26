package support_test

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/tests/support"
)

// isolatedRootVariable is the marker RunIsolated sets, and the one a
// re-executed helper inherits from the test process that started it.
const isolatedRootVariable = "HIPPO_TEST_ISOLATED_ROOT"

// probeOutputVariable asks a re-executed test binary to report the HIPPO_
// environment RunIsolated left it. It deliberately lacks the HIPPO_ prefix so
// the isolation under test cannot remove it.
const probeOutputVariable = "ISOLATION_PROBE_OUTPUT"

// rootExistsKey records, in the probe report, whether HIPPO_ROOT named an
// existing directory while the tests ran.
const rootExistsKey = "probe.root-exists"

// This package isolates itself exactly as the packages that start the product do.
func TestMain(m *testing.M) {
	os.Exit(support.RunIsolated(m))
}

func TestRunIsolatedRemovesInheritedCoordination(t *testing.T) {
	inheritedRoot := t.TempDir()
	report := runIsolationProbe(t, map[string]string{
		"HIPPO_CONFIG":  "fixture-config.json",
		"HIPPO_SESSION": "fixture-session",
		"HIPPO_PROFILE": "fixture-profile",
		"HIPPO_ROOT":    inheritedRoot,
	})

	for _, name := range []string{"HIPPO_CONFIG", "HIPPO_SESSION", "HIPPO_PROFILE"} {
		if value, found := report[name]; found {
			t.Errorf("%s reached the tests as %q, want it removed", name, value)
		}
	}

	root := report["HIPPO_ROOT"]
	if root == "" || root == inheritedRoot {
		t.Fatalf("HIPPO_ROOT = %q, want a directory created for this run instead of %q", root, inheritedRoot)
	}
	if report[isolatedRootVariable] != root {
		t.Errorf("%s = %q, want the run root %q", isolatedRootVariable, report[isolatedRootVariable], root)
	}
	if report[rootExistsKey] != "true" {
		t.Errorf("the run root did not exist while the tests ran")
	}
	if _, err := os.Stat(root); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the run root survived the run: stat error %v", err)
	}
}

func TestRunIsolatedKeepsHarnessInputs(t *testing.T) {
	inputs := map[string]string{
		"HIPPO_BIN":             "/fixture/bin/hippo",
		"HIPPO_BDD_ADAPTER":     "e2e",
		"HIPPO_E2E_TEMP_PARENT": "/fixture/tmp",
		"HIPPO_GO_BINARY":       "/fixture/bin/go",
		"HIPPO_LOAD_SATURATED":  "1",
	}
	report := runIsolationProbe(t, inputs)

	for name, want := range inputs {
		if got := report[name]; got != want {
			t.Errorf("%s = %q, want the harness input %q kept", name, got, want)
		}
	}
}

func TestRunIsolatedLeavesMarkedHelpersUnchanged(t *testing.T) {
	parentRoot := t.TempDir()
	helper := map[string]string{
		isolatedRootVariable:             parentRoot,
		"HIPPO_ROOT":                     parentRoot,
		"HIPPO_STALE_RESERVATION_HELPER": "1",
		"HIPPO_CONFIG":                   "fixture-helper-config.json",
	}
	report := runIsolationProbe(t, helper)

	for name, want := range helper {
		if got := report[name]; got != want {
			t.Errorf("%s = %q in the helper, want its parent's %q", name, got, want)
		}
	}
	if _, err := os.Stat(parentRoot); err != nil {
		t.Errorf("the helper removed its parent's root: %v", err)
	}
}

// TestIsolationProbe is the re-executed half of the tests above. It runs only
// when a parent test asks for a report, and writes every HIPPO_ variable it can
// see, one per line, after RunIsolated has prepared the process.
func TestIsolationProbe(t *testing.T) {
	output := os.Getenv(probeOutputVariable)
	if output == "" {
		t.Skip("runs only as a re-executed probe")
	}

	var lines []string
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "HIPPO_") {
			lines = append(lines, entry)
		}
	}
	info, err := os.Stat(os.Getenv("HIPPO_ROOT")) //nolint:gosec // The probe reports on the root RunIsolated chose; it reads nothing inside it.
	lines = append(lines, rootExistsKey+"="+strconv.FormatBool(err == nil && info.IsDir()))
	slices.Sort(lines)

	if err := os.WriteFile(output, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil { //nolint:gosec // The parent test chose this path inside its own temporary directory.
		t.Fatal(err)
	}
}

// runIsolationProbe re-executes this test binary as a fresh test process whose
// only HIPPO_ variables are the ones given, and returns what the probe saw.
func runIsolationProbe(t *testing.T, environment map[string]string) map[string]string {
	t.Helper()

	output := filepath.Join(t.TempDir(), "probe-report")
	command := exec.Command(os.Args[0], "-test.run=^TestIsolationProbe$", "-test.count=1") //nolint:gosec // The current test binary is the intentional subprocess boundary.
	command.Env = []string{probeOutputVariable + "=" + output}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "HIPPO_") {
			command.Env = append(command.Env, entry)
		}
	}
	for name, value := range environment {
		command.Env = append(command.Env, name+"="+value)
	}

	if combined, err := command.CombinedOutput(); err != nil {
		t.Fatalf("isolation probe failed: %v\n%s", err, combined)
	}

	return readProbeReport(t, output)
}

func readProbeReport(t *testing.T, path string) map[string]string {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("the probe wrote no report: %v", err)
	}
	defer func() { _ = file.Close() }()

	report := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		name, value, _ := strings.Cut(scanner.Text(), "=")
		report[name] = value
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	return report
}
