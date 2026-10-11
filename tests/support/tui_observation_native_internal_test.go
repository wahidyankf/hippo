package support

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/tests/contract"
)

func TestNeutralNativeObservation(t *testing.T) {
	for _, row := range []struct {
		name    string
		prepare func(*Driver) error
		require func(*Driver) error
	}{
		{"known rows", (*Driver).prepareTUIKnownRows, (*Driver).requireTUIRows},
		{"filtered FIFO", (*Driver).prepareTUIFilteredRows, (*Driver).requireTUIFilteredRows},
		{"active mode", (*Driver).prepareTUIModeDifference, (*Driver).requireTUIObservationWorkflow},
		{"idle read only", (*Driver).prepareTUIIdleRoot, (*Driver).requireTUIObservationReadOnly},
		{"exclusive", (*Driver).prepareTUIExclusiveRoot, (*Driver).requireTUIExclusiveSnapshot},
		{"legacy", (*Driver).prepareTUILegacyRow, (*Driver).requireTUILegacyRow},
		{"exact identity", (*Driver).prepareTUIIdentityJoin, (*Driver).requireTUIIdentityJoin},
		{"mixed owners", (*Driver).prepareTUIMixedRows, (*Driver).requireTUIMixedLabels},
	} {
		t.Run(row.name, func(t *testing.T) {
			driver := NewDriver(contract.Adapter{Name: contract.Integration})
			defer driver.Close()
			if err := row.prepare(driver); err != nil {
				t.Fatal(err)
			}
			if err := driver.observeTUI(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := row.require(driver); err != nil {
				t.Fatalf("%v labels=%+v annotations=%+v", err, driver.tuiObservation.snapshot.Running, driver.tuiObservation.annotations)
			}
		})
	}
}

func TestNeutralNativeAnnotationsAndFrozenSnapshot(t *testing.T) {
	for _, kind := range []string{"missing", tuiMalformedAnnotation, "mismatched run ID", "unsupported schema", "larger than 512 bytes", "unreadable", tuiSymlinkAnnotation} {
		t.Run(kind, func(t *testing.T) {
			driver := NewDriver(contract.Adapter{Name: contract.Integration})
			defer driver.Close()
			if err := driver.prepareTUIUnavailableAnnotation(kind); err != nil {
				t.Fatal(err)
			}
			if err := driver.observeTUI(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := driver.requireTUIUnavailableAnnotation(); err != nil {
				t.Fatal(err)
			}
			if err := driver.requireTUIPrivateAdvisory(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNeutralNativeFrozenSnapshotAndStatusWatch(t *testing.T) {
	t.Run("frozen", func(t *testing.T) {
		driver := NewDriver(contract.Adapter{Name: contract.Integration})
		defer driver.Close()
		if err := driver.prepareTUICompletedObservation(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := driver.mutateTUIObservationSources(); err != nil {
			t.Fatal(err)
		}
		if err := driver.requireTUIFrozenObservation(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("status watch", func(t *testing.T) {
		driver := NewDriver(contract.Adapter{Name: contract.Integration})
		defer driver.Close()
		if err := driver.prepareTUIStatusCompatibility(); err != nil {
			t.Fatal(err)
		}
		if err := driver.inspectTUIStatusCompatibility(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := driver.requireTUIWatchCompatibility(); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"status.json", "watch.jsonl"} {
			data, err := os.ReadFile(filepath.Join(driver.tuiObservation.native.root, name))
			if err != nil || len(data) == 0 {
				t.Fatalf("actual CLI output %s unavailable: %v", name, err)
			}
		}
	})
	for _, kind := range []string{"absent", tuiMalformedAnnotation} {
		t.Run("sample "+kind, func(t *testing.T) {
			driver := NewDriver(contract.Adapter{Name: contract.Integration})
			defer driver.Close()
			if _, err := driver.TUIObservationWithSampleTime(t.Context(), kind); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNeutralNativeGuardedJob(t *testing.T) {
	driver := NewDriver(contract.Adapter{Name: contract.Integration})
	defer driver.Close()
	probe, finish, err := driver.NewGuardedTUIJob(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = probe(); err != nil {
		t.Fatal(err)
	}
	if err = finish(); err != nil {
		t.Fatal(err)
	}
	if err = finish(); err != nil {
		t.Fatal(err)
	}
	if err = probe(); err == nil {
		t.Fatal("retired job survived")
	}
}

func TestNeutralNativeCapabilityInspectionFailure(t *testing.T) {
	driver := NewDriver(contract.Adapter{Name: contract.Integration})
	defer driver.Close()
	if err := driver.prepareTUIInspectionFailure(); err != nil {
		t.Fatal(err)
	}
	if err := driver.requestTUIPlain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := driver.requireTUIInspectionFailure(); err != nil {
		t.Fatal(err)
	}
}

// TestNeutralUnitGuardedJobSurvivesParentCancellation keeps the independent
// application's lifetime separate from the terminal scenario that observes it.
func TestNeutralUnitGuardedJobSurvivesParentCancellation(t *testing.T) {
	driver := NewDriver(contract.Adapter{Name: contract.Unit})
	parent, cancel := context.WithCancel(t.Context())
	defer cancel()
	probe, finish, err := driver.NewGuardedTUIJob(parent)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cleanupError := finish(); cleanupError != nil {
			t.Errorf("independent guarded completion: %v", cleanupError)
		}
	})
	cancel()
	window := time.NewTimer(100 * time.Millisecond)
	defer window.Stop()
	ticks := time.NewTicker(5 * time.Millisecond)
	defer ticks.Stop()
	for {
		if err = probe(); err != nil {
			t.Fatalf("parent cancellation retired independent job: %v", err)
		}
		select {
		case <-ticks.C:
		case <-window.C:
			if err = finish(); err != nil {
				t.Fatalf("explicit independent completion: %v", err)
			}
			if err = finish(); err != nil {
				t.Fatalf("repeated independent completion: %v", err)
			}
			if err = probe(); err == nil {
				t.Fatal("completed independent job survived")
			}
			driver.Close()
			return
		}
	}
}
