package runtime //nolint:testpackage // Floor leg adds the missing unit branches, as run_test.go does.

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestFloorAttemptPortLeaseReportsOwnerWriteFailure(t *testing.T) {
	host, port := floorDoublesSystem()
	host.Random.Bytes = []byte("0123456789abcdef")
	previous := writeLeaseOwnerFile
	writeLeaseOwnerFile = func(string, []byte, fs.FileMode) error {
		return errors.New("floor: owner write refused")
	}
	t.Cleanup(func() { writeLeaseOwnerFile = previous })
	root := t.TempDir()
	if _, _, err := attemptPortLease(port, root, 49152, "floor"); err == nil {
		t.Fatal("attemptPortLease with a refused owner write = nil, want error")
	}
	if _, statErr := os.Stat(filepath.Join(root, "49152.lock")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("attemptPortLease left the lease path behind: %v", statErr)
	}
}
