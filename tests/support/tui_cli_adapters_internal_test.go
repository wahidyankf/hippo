package support

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wahidyankf/hippo/internal/adapters/tui"
	"github.com/wahidyankf/hippo/tests/contract"
)

func TestTUIBDDCLIHelpExecutesRealAdapterBranches(t *testing.T) {
	for _, mode := range []string{contract.Integration, contract.E2E} {
		t.Run(mode, func(t *testing.T) {
			adapter, err := contract.AdapterByName(mode)
			if err != nil {
				t.Fatal(err)
			}
			driver := NewDriver(adapter)
			defer driver.Close()
			driver.Reset()
			if mode == contract.E2E {
				if err := requireTestStamp(driver.binary); err != nil {
					t.Fatalf("selected compiled candidate is unavailable: %v", err)
				}
			}
			for _, binding := range driver.tuiCLIHelpBindings() {
				var stepError error
				switch handler := binding.Handler.(type) {
				case func() error:
					stepError = handler()
				case func(context.Context) error:
					stepError = handler(t.Context())
				default:
					t.Fatalf("unexpected TUI-help handler signature for %s", binding.Pattern)
				}
				if stepError != nil {
					t.Fatalf("%s executing help step %s: %v", mode, binding.Pattern, stepError)
				}
			}
			t.Logf("adapter=%s status=%d stdout=%q stderr=%q", mode, driver.exitCode, driver.output, driver.errorOutput)
		})
	}
}

func TestTUIBDDCLIHelpE2ERequiresSelectedCandidate(t *testing.T) {
	adapter, err := contract.AdapterByName(contract.E2E)
	if err != nil {
		t.Fatal(err)
	}
	driver := NewDriver(adapter)
	defer driver.Close()
	if err := driver.prepareTUIHelp(); err != nil {
		t.Fatal(err)
	}
	if err := driver.requestTUIHelp(t.Context()); err == nil {
		t.Fatal("compiled TUI-help binding accepted an uninitialized candidate")
	}
}

func TestTUIBDDCLIHelpE2EPreservesCallerCancellation(t *testing.T) {
	adapter, err := contract.AdapterByName(contract.E2E)
	if err != nil {
		t.Fatal(err)
	}
	driver := NewDriver(adapter)
	defer driver.Close()
	driver.Reset()
	if err := requireTestStamp(driver.binary); err != nil {
		t.Fatal(err)
	}
	if err := driver.prepareTUIHelp(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := driver.requestTUIHelp(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("compiled help ignored caller cancellation: %v", err)
	}
}

func TestTUIBDDCLIHelpCommandFixturePreservesLegacyCallers(t *testing.T) {
	adapter, err := contract.AdapterByName(contract.E2E)
	if err != nil {
		t.Fatal(err)
	}
	driver := NewDriver(adapter)
	defer driver.Close()
	driver.Reset()
	if err := requireTestStamp(driver.binary); err != nil {
		t.Fatal(err)
	}
	arguments := []string{"tui", "--help"}
	legacy, err := driver.attemptUsage(arguments)
	if err != nil {
		t.Fatal(err)
	}
	withContext, err := driver.attemptUsageContext(t.Context(), arguments)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.exitCode != 0 || legacy.stderr != "" || legacy.stdout == "" || legacy.exitCode != withContext.exitCode || legacy.stdout != withContext.stdout || legacy.stderr != withContext.stderr {
		t.Fatalf("bounded command fixture changed legacy callers: legacy=%+v context=%+v", legacy, withContext)
	}
}

func TestTUISurfaceOwnedRootFromSymlinkTemporaryParent(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "private-temp")
	if err = os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(parent, "temporary-alias")
	if err = os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", alias)
	driver := NewDriver(contract.Adapter{Name: contract.Integration})
	defer driver.Close()
	root, err := driver.temporaryRoot()
	if err != nil {
		t.Fatal(err)
	}
	actual, err := tuiSurfaceOwnedRoot(root)
	if err != nil {
		t.Fatalf("actual owned surface root was refused: %v", err)
	}
	if actual != root || filepath.Dir(actual) != target {
		t.Fatalf("surface root lost canonical ownership: got %q, want %q", actual, root)
	}
	driver.Close()
	if _, err = os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned surface root survived cleanup: %v", err)
	}
	if _, err = os.Stat(target); err != nil {
		t.Fatalf("surface cleanup changed temporary parent: %v", err)
	}
}

func TestTUISurfaceOwnedRootRejectsUnownedShapes(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", base)
	for _, row := range []struct{ name, root string }{
		{"empty", ""},
		{"relative", "hippo-lease-relative"},
		{"wrong prefix", filepath.Join(base, "foreign-fixture")},
		{"nested", filepath.Join(base, "nested", "hippo-lease-child")},
		{"outside", filepath.Join(filepath.Dir(base), "hippo-lease-outside")},
	} {
		t.Run(row.name, func(t *testing.T) {
			actual, rootError := tuiSurfaceOwnedRoot(row.root)
			if rootError == nil || actual != "" {
				t.Fatalf("unowned surface root accepted: input=%q actual=%q error=%v", row.root, actual, rootError)
			}
		})
	}
}

func TestPublicTUIInspectionInputProducesKernelError(t *testing.T) {
	state := &tuiArgumentState{}
	input, err := publicTUIInspectionInput(state)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeError := input.Close(); closeError != nil {
			t.Error(closeError)
		}
	})
	_, kernelError := publicTUITermios(int(input.Fd()))
	inputTTY, outputTTY, capabilityError := tui.TerminalCapabilities(input, nil)
	if kernelError == nil || capabilityError == nil || inputTTY || outputTTY || !errors.Is(capabilityError, kernelError) {
		t.Fatalf("actual owned descriptor did not retain its kernel inspection failure: %t/%t kernel=%v capability=%v", inputTTY, outputTTY, kernelError, capabilityError)
	}
	t.Log(state.inspectionEvidence)
}

func TestPublicTUIInspectionPreparationPreservesOwnership(t *testing.T) {
	master, slave, err := openNativeTUIPTY()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeError := errors.Join(master.Close(), slave.Close()); closeError != nil {
			t.Error(closeError)
		}
	})
	original, err := slave.Stat()
	if err != nil {
		t.Fatal(err)
	}
	input, err := prepareNativeTUIInspectionSlave(master, slave)
	if err != nil {
		t.Fatal(err)
	}
	if input != slave {
		t.Cleanup(func() {
			if closeError := input.Close(); closeError != nil {
				t.Error(closeError)
			}
		})
	}
	current, originalError := slave.Stat()
	candidate, candidateError := input.Stat()
	if originalError != nil || candidateError != nil || !os.SameFile(original, current) {
		t.Fatalf("acquisition lost original slave ownership: %v/%v", originalError, candidateError)
	}
	if runtime.GOOS == "darwin" {
		if input == slave || os.SameFile(original, candidate) || candidate.Mode()&os.ModeSocket == 0 {
			t.Fatal("Darwin acquisition did not return its distinct owned anonymous socket")
		}
		requireNativeTUIInspectionPairRefusals(t, master, slave)
	} else if input != slave || !os.SameFile(original, candidate) {
		t.Fatal("Linux preparation replaced its original hangup descriptor")
	}
}

func requireNativeTUIInspectionPairRefusals(t *testing.T, master, slave *os.File) {
	t.Helper()
	otherMaster, otherSlave, err := openNativeTUIPTY()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeError := errors.Join(otherMaster.Close(), otherSlave.Close()); closeError != nil {
			t.Error(closeError)
		}
	})
	for _, pair := range []struct {
		name          string
		master, slave *os.File
	}{
		{"missing master", nil, slave},
		{"missing slave", master, nil},
		{"mismatched pair", master, otherSlave},
	} {
		t.Run(pair.name, func(t *testing.T) {
			input, refusal := prepareNativeTUIInspectionSlave(pair.master, pair.slave)
			if input != nil || refusal == nil {
				t.Fatalf("unowned pair accepted an inspection descriptor: input=%v error=%v", input, refusal)
			}
		})
	}
	if _, err := slave.Stat(); err != nil {
		t.Fatalf("refusal closed caller-owned slave: %v", err)
	}
	if _, err := otherSlave.Stat(); err != nil {
		t.Fatalf("refusal closed other caller-owned slave: %v", err)
	}
	if _, err := master.Stat(); err != nil {
		t.Fatalf("refusal closed caller-owned master: %v", err)
	}
	if _, err := otherMaster.Stat(); err != nil {
		t.Fatalf("refusal closed other caller-owned master: %v", err)
	}
}
