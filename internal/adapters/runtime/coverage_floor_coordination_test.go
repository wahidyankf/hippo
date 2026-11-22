package runtime //nolint:testpackage // Floor leg adds the missing unit branches, as run_test.go does.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
)

func floorDoublesSystem() (*systemtest.Host, system.System) {
	host := systemtest.NewHost(time.Unix(1700000000, 0).UTC())

	return host, host.Port()
}

func writeFloorMarker(t *testing.T, root, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "coordination-mode.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFloorReadCoordinationMarkerMatrix(t *testing.T) {
	cases := map[string]struct {
		content     string
		wantPresent bool
		wantErr     bool
		useFileRoot bool
	}{
		"unreadable root":      {useFileRoot: true, wantErr: true},
		"invalid JSON":         {content: "!!!", wantErr: true},
		"non object":           {content: "[1]", wantErr: true},
		"not an object either": {content: "[[[", wantErr: true},
		"duplicate field":      {content: `{"schemaVersion":1,"schemaVersion":1,"mode":"exclusive"}`, wantErr: true},
		"unknown field":        {content: `{"schemaVersion":1,"mode":"exclusive","bogus":1}`, wantErr: true},
		"field decode failure": {content: `{"schemaVersion":"x","mode":"exclusive"}`, wantErr: true},
		"truncated object":     {content: `{"schemaVersion":1,"mode":"exclusive`, wantErr: true},
		"truncated end":        {content: `{"schemaVersion":1`, wantErr: true},
		"trailing garbage":     {content: `{"schemaVersion":1,"mode":"exclusive"} !!!`, wantErr: true},
		"trailing value":       {content: `{"schemaVersion":1,"mode":"exclusive"} {}`, wantErr: true},
		"future schema":        {content: `{"schemaVersion":2,"mode":"exclusive"}`, wantErr: true},
		"missing schema":       {content: `{}`, wantErr: true},
		"unknown mode":         {content: `{"schemaVersion":1,"mode":"bogus"}`, wantErr: true},
		"missing mode":         {content: `{"schemaVersion":1}`, wantErr: true},
		"exclusive marker":     {content: `{"schemaVersion":1,"mode":"exclusive"}`, wantPresent: true},
		"reservation marker":   {content: `{"schemaVersion":1,"mode":"reservation"}`, wantPresent: true},
	}
	for name, kase := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if kase.useFileRoot {
				marker, err := os.CreateTemp(t.TempDir(), "root-*")
				if err != nil {
					t.Fatal(err)
				}
				_ = marker.Close()
				root = marker.Name()
			} else if kase.content != "" {
				writeFloorMarker(t, root, kase.content)
			}
			marker, present, err := readCoordinationMarker(root)
			if kase.wantErr && err == nil {
				t.Fatalf("readCoordinationMarker(%q) = %+v, %v, nil, want error", kase.content, marker, present)
			}
			if !kase.wantErr && err != nil {
				t.Fatalf("readCoordinationMarker(%q): %v", kase.content, err)
			}
			if present != kase.wantPresent {
				t.Fatalf("readCoordinationMarker(%q) present = %v, want %v", kase.content, present, kase.wantPresent)
			}
		})
	}
}

func TestFloorWriteCoordinationMarkerRefusesFileRoot(t *testing.T) {
	marker, err := os.CreateTemp(t.TempDir(), "root-*")
	if err != nil {
		t.Fatal(err)
	}
	_ = marker.Close()
	if err := writeCoordinationMarker(marker.Name(), NewExclusiveCoordinationMarker()); err == nil {
		t.Fatal("writeCoordinationMarker under a file = nil, want error")
	}
}

func TestFloorEnsureExclusiveCoordinationReportsUnreadableMarker(t *testing.T) {
	marker, err := os.CreateTemp(t.TempDir(), "root-*")
	if err != nil {
		t.Fatal(err)
	}
	_ = marker.Close()
	if err := ensureExclusiveCoordination(marker.Name()); err == nil {
		t.Fatal("ensureExclusiveCoordination under a file = nil, want error")
	}
}

func TestFloorCoordinationProcessGateRefusesClosedFile(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "gate-*.lock")
	if err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if err := acquireCoordinationProcessGate(context.Background(), file, 0); err == nil {
		t.Fatal("acquireCoordinationProcessGate on a closed file = nil, want error")
	}
}

func floorHeldLock(t *testing.T, root string) {
	t.Helper()
	first, err := acquireCoordinationLock(system.Production, context.Background(), root, 0)
	if err != nil {
		t.Fatalf("seed coordination lock: %v", err)
	}
	t.Cleanup(func() { _ = releaseCoordinationLock(system.Production, first) })
}

func TestFloorCoordinationGateRefusesContendedWait(t *testing.T) {
	root := t.TempDir()
	floorHeldLock(t, root)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := acquireCoordinationProcessGate(ctx, floorOpenLock(t, root), 0); !errors.Is(err, errCoordinationDeferred) {
		t.Fatalf("contended gate with no wait = %v, want deferred", err)
	}
}

func TestFloorCoordinationGateTimesOutOnContention(t *testing.T) {
	root := t.TempDir()
	floorHeldLock(t, root)
	if err := acquireCoordinationProcessGate(context.Background(), floorOpenLock(t, root), 50*time.Millisecond); !errors.Is(
		err, errCoordinationDeferred,
	) {
		t.Fatalf("contended gate with a short wait = %v, want deferred", err)
	}
}

func TestFloorCoordinationGateHonoursCancellation(t *testing.T) {
	root := t.TempDir()
	floorHeldLock(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := acquireCoordinationProcessGate(ctx, floorOpenLock(t, root), 5*time.Second); err == nil {
		t.Fatal("contended gate with a cancelled context = nil, want error")
	}
}

func floorOpenLock(t *testing.T, root string) *os.File {
	t.Helper()
	file, err := openCoordinationLock(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })

	return file
}

func TestFloorAcquireCoordinationLockReportsFlockContention(t *testing.T) {
	host, port := floorDoublesSystem()
	host.Files.FlockErr = syscall.EWOULDBLOCK
	if _, err := acquireCoordinationLock(port, context.Background(), t.TempDir(), 0); !errors.Is(err, errCoordinationDeferred) {
		t.Fatalf("locked coordination root with no wait = %v, want deferred", err)
	}
}

func TestFloorAcquireCoordinationLockReportsFlockFailure(t *testing.T) {
	host, port := floorDoublesSystem()
	boom := errors.New("floor: flock refused")
	host.Files.FlockErr = boom
	if _, err := acquireCoordinationLock(port, context.Background(), t.TempDir(), 0); !errors.Is(err, boom) {
		t.Fatalf("failing coordination flock = %v, want the flock error", err)
	}
}

func TestFloorAcquireCoordinationLockHonoursCancellation(t *testing.T) {
	host, port := floorDoublesSystem()
	host.Files.FlockErr = syscall.EWOULDBLOCK
	ctx, cancel := context.WithCancel(context.Background())
	timer := time.AfterFunc(50*time.Millisecond, cancel)
	defer timer.Stop()
	if _, err := acquireCoordinationLock(port, ctx, t.TempDir(), 5*time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled coordination wait = %v, want cancellation", err)
	}
}
