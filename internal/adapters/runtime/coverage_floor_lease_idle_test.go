package runtime //nolint:testpackage // Floor leg adds the missing unit branches, as run_test.go does.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFloorRemoveExclusiveMarkerIfIdleReportsSessionEnumerationFailure(t *testing.T) {
	_, port := floorDoublesSystem()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sessions", "floor.json"), []byte("[[["), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeExclusiveMarkerIfIdle(port, root); err == nil {
		t.Fatal("removeExclusiveMarkerIfIdle with a garbage record = nil, want error")
	}
}

func TestFloorInheritedSessionFallsBackToLiveHeavyLease(t *testing.T) {
	host, port := floorDoublesSystem()
	host.Processes.Alive[os.Getpid()] = true
	root := t.TempDir()
	heavyPath := filepath.Join(root, "heavy.lock")
	if err := os.Mkdir(heavyPath, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := writeLeaseOwner(heavyPath, floorLiveOwner(os.Getpid(), floorToken)); err != nil {
		t.Fatal(err)
	}
	if !inheritedSession(port, root, floorToken) {
		t.Fatal("inheritedSession with a live heavy lease = false, want true")
	}
	if inheritedSession(port, root, floorDead) {
		t.Fatal("inheritedSession with an unknown candidate = true, want false")
	}
}
