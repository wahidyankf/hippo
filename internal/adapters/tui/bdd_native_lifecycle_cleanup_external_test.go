package tui_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/tests/contract"
)

func TestNativeLifecycleRegisteredCleanupRetainsFailedRoot(t *testing.T) {
	if os.Getenv("HIPPO_BDD_SUITE") != contract.Integration {
		t.Skip("native lifecycle cleanup requires explicit Integration")
	}
	adapter, err := contract.AdapterByName(contract.Integration)
	if err != nil {
		t.Fatal(err)
	}
	driver := newComposedBDDDriver(adapter)
	parent := t.TempDir()
	owned := filepath.Join(parent, "owned-native-lifecycle")
	if err := os.Mkdir(owned, 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(parent, "preserve-sibling")
	if err := os.WriteFile(foreign, []byte("belongs to another owner"), 0o600); err != nil {
		t.Fatal(err)
	}
	driver.native.lifecycle.directory = owned
	t.Cleanup(func() {
		if err := os.Chmod(parent, 0o700); err != nil {
			t.Errorf("restore test-owned parent permissions: %v", err)
		}
		// The RED must retire its real permission fault and leftover test-owned
		// root even when the subject loses the handle being tested.
		if err := os.RemoveAll(owned); err != nil {
			t.Errorf("retire test-owned root after assertions: %v", err)
		}
	})
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	failure := nativeLifecycleCloseFailure(t, driver)
	_, pathError := errors.AsType[*os.PathError](failure)
	if failure == nil {
		t.Error("registered final Driver.Close hid an actual directory-removal failure")
	} else if !errors.Is(failure, os.ErrPermission) || !pathError || !strings.Contains(failure.Error(), "cleanup") {
		t.Errorf("final cleanup did not preserve its actual permission cause with context: %v", failure)
	}
	if driver.native.lifecycle.directory != owned {
		t.Error("failed registered cleanup lost its owned directory handle")
	}
	if info, err := os.Stat(owned); err != nil || !info.IsDir() {
		t.Errorf("permission-denied owned root was not retained: %v", err)
	}
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	if failure := nativeLifecycleCloseFailure(t, driver); failure != nil {
		t.Errorf("recovered actual Driver.Close failed: %v", failure)
	}
	if _, err := os.Stat(owned); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("recovered Driver.Close did not retire its actual owned root: %v", err)
	}
	if driver.native.lifecycle.directory != "" {
		t.Error("successful registered cleanup retained its directory handle")
	}
	if data, err := os.ReadFile(foreign); err != nil || string(data) != "belongs to another owner" {
		t.Errorf("registered cleanup altered its foreign sibling: %q/%v", data, err)
	}
}

func nativeLifecycleCloseFailure(t *testing.T, driver *composedBDDDriver) error {
	t.Helper()
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		driver.Close()
	}()
	if recovered == nil {
		return nil
	}
	failure, ok := recovered.(error)
	if !ok {
		t.Fatalf("registered cleanup produced an unclassified panic: %v", recovered)
	}
	return failure
}
