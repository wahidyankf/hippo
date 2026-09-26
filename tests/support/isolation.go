package support

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// isolatedRootVariable marks a test process that has already isolated itself,
// and holds the root it chose. A helper that a test re-executes starts from its
// parent's environment, inherits the marker, and so keeps the flags its parent
// set for it rather than losing them to a second scrub.
const isolatedRootVariable = "HIPPO_TEST_ISOLATED_ROOT"

// harnessInputs are the only HIPPO_ variables a test package keeps from the
// shell that started it: each one chooses what the harness tests rather than
// how the product behaves. Every other HIPPO_ variable is removed, so one the
// product learns to read later is isolated without an edit here.
var harnessInputs = map[string]bool{
	"HIPPO_BIN":             true, // the binary the adapters run
	"HIPPO_BDD_ADAPTER":     true, // the boundary the behaviour suite executes at
	"HIPPO_E2E_TEMP_PARENT": true, // where tests/e2e/run.sh builds its binary
	"HIPPO_GO_BINARY":       true, // the Go toolchain tests/e2e/run.sh invokes
	"HIPPO_LOAD_SATURATED":  true, // scripts/test-loaded.sh declaring a saturated host
}

// RunIsolated runs a test package against coordination state of its own: no
// HIPPO_ variable from the invoking shell or an enclosing guard reaches the
// binary under test, except the harness inputs, and HIPPO_ROOT names a fresh
// directory removed when the run ends. Scenarios that set their own HIPPO_ROOT
// keep doing so; this root is only the default.
//
// It is the whole body of each such package's TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(support.RunIsolated(m)) }
func RunIsolated(m *testing.M) int {
	if os.Getenv(isolatedRootVariable) != "" {
		return m.Run()
	}

	root, err := os.MkdirTemp("", "hippo-root-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "isolate test coordination state: %v\n", err)

		return 1
	}

	if err := isolateEnvironment(root); err != nil {
		fmt.Fprintf(os.Stderr, "isolate test coordination state: %v\n", err)
		_ = os.RemoveAll(root)

		return 1
	}

	code := m.Run()
	// A leftover root is reported rather than failed: it costs temporary space,
	// not a verdict about the code under test.
	if err := os.RemoveAll(root); err != nil {
		fmt.Fprintf(os.Stderr, "remove isolated test root: %v\n", err)
	}

	return code
}

func isolateEnvironment(root string) error {
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "HIPPO_") && !harnessInputs[name] {
			if err := os.Unsetenv(name); err != nil {
				return err
			}
		}
	}

	if err := os.Setenv("HIPPO_ROOT", root); err != nil {
		return err
	}

	return os.Setenv(isolatedRootVariable, root)
}
