package runtime //nolint:testpackage // Floor leg adds the missing unit branches, as run_test.go does.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFloorReleasePortLeaseNilIsNil(t *testing.T) {
	if err := ReleasePortLease(t.TempDir(), nil); err != nil {
		t.Fatalf("ReleasePortLease(nil) = %v, want nil", err)
	}
}

func TestFloorReleasePortLeaseRefusesForeignPath(t *testing.T) {
	root := t.TempDir()
	lease := &PortLease{Path: filepath.Join(root, "other.lock"), Port: 999}
	if err := ReleasePortLease(root, lease); err == nil {
		t.Fatal("ReleasePortLease with a foreign path = nil, want error")
	}
}

func TestFloorReleasePortLeaseRefusesMissingOwner(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "999.lock")
	if err := os.Mkdir(path, 0o750); err != nil {
		t.Fatal(err)
	}
	lease := &PortLease{Path: path, Port: 999, Owner: "floor", Token: floorToken}
	if err := ReleasePortLease(root, lease); err == nil {
		t.Fatal("ReleasePortLease without an owner record = nil, want error")
	}
}
