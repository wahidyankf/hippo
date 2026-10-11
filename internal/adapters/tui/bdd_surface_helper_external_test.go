package tui_test

import (
	"flag"
	"os"
	"testing"

	"github.com/wahidyankf/hippo/tests/contract"
	"github.com/wahidyankf/hippo/tests/support"
)

func TestTUIBDDArgumentSurfaceHelper(t *testing.T) {
	if os.Getenv("HIPPO_TUI_ARGUMENT_SURFACE_HELPER") == "" {
		t.Skip("runs only as owned native surface helper")
	}
	if err := support.RunTUIArgumentSurfaceHelper(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestTUIBDDNativeUnsupportedSurfaceContract(t *testing.T) {
	adapter, err := executingBDDSuite(os.Getenv("HIPPO_BDD_SUITE"))
	if err != nil {
		t.Fatal(err)
	}
	if adapter.Name == contract.Unit {
		if flag.Lookup("test.run").Value.String() == "^TestTUIBDDNativeUnsupportedSurfaceContract$" {
			t.Fatal("explicit native surface contract requires HIPPO_BDD_SUITE=integration or e2e")
		}
		t.Skip("requires explicit native executing suite")
	}
	for _, surface := range []string{"non-terminal input", "non-terminal output", "TERM set to dumb"} {
		t.Run(surface, func(t *testing.T) {
			if err := support.CheckTUIUnsupportedSurface(t.Context(), adapter, os.Getenv("HIPPO_BIN"), surface); err != nil {
				t.Fatal(err)
			}
		})
	}
}
