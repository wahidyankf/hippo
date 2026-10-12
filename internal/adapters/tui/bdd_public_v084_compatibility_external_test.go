package tui_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	configadapter "github.com/wahidyankf/hippo/internal/adapters/config"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/tests/contract"
)

// Exercise the actual public Given and configuration before any executable is
// trusted. A missing old artifact must fail with zero owners, never fall back.
func TestPublicV084CompatibilityRouteKeepsNativeBudgetSeparate(t *testing.T) {
	t.Setenv("HIPPO_PUBLIC_OLD_BIN", "")
	for _, test := range []struct {
		name, variant string
		ownerCPU      int
		memoryCap     int64
	}{
		{"canonical-v084", publicBDDMixedV084Variant, 1, 3 * policy.GiB},
		{"native-default", "mixed", 2, 8 * policy.GiB},
	} {
		t.Run(test.name, func(t *testing.T) {
			driver := newComposedBDDDriver(contract.Adapter{Name: contract.E2E})
			t.Cleanup(driver.Close)
			fixture := driver.public
			var err error
			if test.variant == publicBDDMixedV084Variant {
				binding := fixture.handler(`^old and current owners sharing unchanged coordination schemas$`)
				prepare, ok := binding.(func(context.Context) (context.Context, error))
				if !ok {
					t.Fatal("canonical old/current Given lost its public handler")
				}
				_, err = prepare(t.Context())
			} else {
				_, err = fixture.prepareCase(t.Context(), "mixed", "")
			}
			if err == nil || !strings.Contains(err.Error(), "explicitly pinned old executable") {
				t.Fatalf("missing old artifact did not fail before owner launch: %v", err)
			}
			if fixture.variant != test.variant || !fixture.isMixedObservation() || len(fixture.mixed) != 0 || fixture.probe != nil || fixture.transport != nil {
				t.Fatal("public route changed variant or launched an actor before artifact authentication")
			}
			configured, err := configadapter.Load(fixture.ledger.Configuration(), true)
			if err != nil {
				t.Fatal(err)
			}
			if configured.Coordination.MaxMemoryBytes != test.memoryCap || configured.Coordination.MaxCPU != 8 || configured.Coordination.MaxActiveOwners != 2 || publicBDDMixedOwnerCPU(fixture.variant) != test.ownerCPU {
				t.Fatalf("public/native routing changed the exact scoped budget: %+v", configured.Coordination)
			}
			if config, err := readPublicMixedConfiguration(fixture.directory, fixture.ledger.Configuration()); err != nil || len(config) == 0 {
				t.Fatalf("actual owned observation configuration missing from diagnostic: %v", err)
			}
		})
	}
}

func TestPublicMixedConfigurationReceiptKeepsOwnedPathBoundary(t *testing.T) {
	directory := t.TempDir()
	owned, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := owned.WriteFile("observation.json", []byte("owned fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := owned.Symlink("/etc/hosts", "outside.json"); err != nil {
		t.Fatal(err)
	}
	if err := owned.Close(); err != nil {
		t.Fatal(err)
	}
	configuration := filepath.Join(directory, "observation.json")
	got, err := readPublicMixedConfiguration(directory, configuration)
	if err != nil || string(got) != "owned fixture" {
		t.Fatalf("owned configuration diagnostic failed: %q/%v", got, err)
	}
	for _, path := range []string{"observation.json", filepath.Join(directory, "other.json"), filepath.Join(directory, "outside.json"), filepath.Join(filepath.Dir(directory), "observation.json")} {
		if got, err := readPublicMixedConfiguration(directory, path); err == nil || got != nil {
			t.Fatalf("foreign configuration path accepted: %q/%v", got, err)
		}
	}
	if got, err := readPublicMixedConfiguration("relative", "relative/observation.json"); err == nil || got != nil {
		t.Fatal("relative diagnostic root accepted")
	}
	owned, err = os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := owned.Remove("observation.json"); err != nil {
		t.Fatal(err)
	}
	if err := owned.Symlink("/etc/hosts", "observation.json"); err != nil {
		t.Fatal(err)
	}
	if err := owned.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := readPublicMixedConfiguration(directory, configuration); err == nil || got != nil {
		t.Fatal("owned filename escaped its root through a symlink")
	}
}
