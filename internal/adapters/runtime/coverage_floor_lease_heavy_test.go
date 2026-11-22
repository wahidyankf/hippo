package runtime //nolint:testpackage // Floor leg adds the missing unit branches, as run_test.go does.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFloorDescribeHeavyLeaseReportsDeadOwner(t *testing.T) {
	root := t.TempDir()
	heavyPath := filepath.Join(root, "heavy.lock")
	if err := os.Mkdir(heavyPath, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := writeLeaseOwner(heavyPath, floorLiveOwner(1<<30, floorToken)); err != nil {
		t.Fatal(err)
	}
	if got := DescribeHeavyLease(root); got != "the heavy-work lease reports no live owner" {
		t.Fatalf("DescribeHeavyLease with a dead owner = %q, want no live owner", got)
	}
}

func TestFloorHeavyLeaseHeldRefusesMissingSchema(t *testing.T) {
	_, port := floorDoublesSystem()
	root := t.TempDir()
	heavyPath := filepath.Join(root, "heavy.lock")
	if err := os.Mkdir(heavyPath, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := writeLeaseOwner(heavyPath, leaseOwner{PID: os.Getpid(), Token: floorToken}); err != nil {
		t.Fatal(err)
	}
	if _, err := heavyLeaseHeld(port, heavyPath); err == nil {
		t.Fatal("heavyLeaseHeld with a missing owner schema = nil, want error")
	}
}
