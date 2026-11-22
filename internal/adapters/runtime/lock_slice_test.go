package runtime //nolint:testpackage // Slice (c) doubles run inside the production package, as run_test.go does.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
	"github.com/wahidyankf/hippo/internal/application"
	"golang.org/x/sys/unix" //nolint:depguard // Slice doubles must observe the same flock errors and flags as production.
)

func lockSliceSystem() (*systemtest.Host, system.System) {
	host := systemtest.NewHost(time.Unix(1700000000, 0).UTC())
	host.Random.Bytes = []byte("0123456789abcdef")

	return host, host.Port()
}

func TestLivePIDReadsTheDouble(t *testing.T) {
	host, port := lockSliceSystem()
	host.Processes.Alive[4242] = true
	if !livePID(port, 4242) {
		t.Error("live double pid reads as dead")
	}
	if livePID(port, 9999) {
		t.Error("absent pid reads as live")
	}
	if livePID(port, 0) || livePID(port, -3) {
		t.Error("non-positive pid reads as live")
	}
}

func TestLivePIDTreatsDeniedAsLive(t *testing.T) {
	host, port := lockSliceSystem()
	host.Processes.NoAccess[4243] = true
	if !livePID(port, 4243) {
		t.Error("denied pid reads as dead, want live under another owner")
	}
	if !processExists(port, 4243) {
		t.Error("denied pid reads as absent, want present")
	}
}

func TestProcessGroupExistsReadsTheDouble(t *testing.T) {
	host, port := lockSliceSystem()
	host.Processes.Alive[-4242] = true
	if !processGroupExists(port, 4242) {
		t.Error("live double group reads as retired")
	}
	if processGroupExists(port, 9999) {
		t.Error("absent group reads as live")
	}
}

func TestAcquireCoordinationLockGrantsThroughTheDouble(t *testing.T) {
	_, port := lockSliceSystem()
	root := t.TempDir()
	lock, err := acquireCoordinationLock(port, context.Background(), root, time.Second)
	if err != nil || lock == nil {
		t.Fatalf("acquire coordination lock = %v, %v, want a lock", lock, err)
	}
	if _, err = os.Stat(filepath.Join(root, "coordination.lock")); err != nil {
		t.Fatalf("coordination lock file: %v", err)
	}
	if err = releaseCoordinationLock(port, lock); err != nil {
		t.Fatalf("release coordination lock: %v", err)
	}
}

func TestAcquireCoordinationLockReportsContention(t *testing.T) {
	host, port := lockSliceSystem()
	host.Files.FlockErr = unix.EWOULDBLOCK
	_, err := acquireCoordinationLock(port, context.Background(), t.TempDir(), 0)
	if !isCoordinationDeferred(err) {
		t.Fatalf("contended acquire = %v, want a capacity deferral", err)
	}
}

func TestAcquireCoordinationLockHonoursCancellation(t *testing.T) {
	host, port := lockSliceSystem()
	host.Files.FlockErr = unix.EWOULDBLOCK
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := acquireCoordinationLock(port, cancelled, t.TempDir(), time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled acquire = %v, want context canceled", err)
	}
}

func TestAcquireCoordinationLockReportsCancellationWhileWaiting(t *testing.T) {
	_, port := lockSliceSystem()
	root := t.TempDir()
	holder, err := acquireCoordinationLock(port, context.Background(), root, time.Hour)
	if err != nil || holder == nil {
		t.Fatalf("acquire coordination lock = %v, %v, want a held lock", holder, err)
	}
	defer func() { _ = releaseCoordinationLock(port, holder) }()
	cancelled, cancel := context.WithCancel(context.Background())
	waiter := make(chan error, 1)
	go func() {
		_, waiterError := acquireCoordinationLock(port, cancelled, root, time.Hour)
		waiter <- waiterError
	}()
	time.Sleep(500 * time.Millisecond)
	cancel()
	if err = <-waiter; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting acquire = %v, want context canceled", err)
	}
}

func TestReleaseCoordinationLockToleratesNil(t *testing.T) {
	_, port := lockSliceSystem()
	if err := releaseCoordinationLock(port, nil); err != nil {
		t.Fatalf("release nil lock: %v", err)
	}
}

func isCoordinationDeferred(err error) bool { return err != nil && IsCoordinationDeferred(err) }

func TestOpenPortLeaseIdentityLocksThroughTheDouble(t *testing.T) {
	_, port := lockSliceSystem()
	path := t.TempDir()
	identity, err := openPortLeaseIdentity(port, path)
	if err != nil || identity == nil {
		t.Fatalf("open port lease identity = %v, %v, want a lock", identity, err)
	}
	defer func() { _ = identity.Close() }()
	device, inode, err := reservationIdentityMetadata(identity)
	if err != nil || device == 0 || inode == 0 {
		t.Fatalf("port lease identity = %d/%d, %v, want real file identity", device, inode, err)
	}
}

func TestPortLeaseIdentityLiveThroughTheDouble(t *testing.T) {
	host, port := lockSliceSystem()
	path := t.TempDir()
	identity, err := openPortLeaseIdentity(port, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = identity.Close() }()
	device, inode, err := reservationIdentityMetadata(identity)
	if err != nil {
		t.Fatal(err)
	}
	value, err := token(port)
	if err != nil {
		t.Fatal(err)
	}
	owner := &leaseOwner{Token: value, IdentityDevice: device, IdentityInode: inode}
	live, err := portLeaseIdentityLive(port, path, owner)
	if err != nil || live {
		t.Fatalf("free port lease identity live = %v, %v, want false with no error", live, err)
	}
	host.Files.FlockErr = unix.EWOULDBLOCK
	live, err = portLeaseIdentityLive(port, path, owner)
	if err != nil || !live {
		t.Fatalf("held port lease identity live = %v, %v, want true with no error", live, err)
	}
}

func TestAttemptPortLeaseAdmitsAndReconcilesThroughTheDouble(t *testing.T) {
	host, port := lockSliceSystem()
	root := t.TempDir()
	lease, retry, err := attemptPortLease(port, root, 8080, "owner")
	if err != nil || lease == nil || retry {
		t.Fatalf("first attempt = %+v, %v, %v, want a lease", lease, retry, err)
	}
	if lease.Token == "" || lease.Port != 8080 {
		t.Fatalf("admitted lease = %+v, want token and port 8080", lease)
	}
	host.Files.FlockErr = unix.EWOULDBLOCK
	_, _, err = attemptPortLease(port, root, 8080, "owner")
	if !isCoordinationDeferred(err) {
		t.Fatalf("held port attempt = %v, want a capacity deferral", err)
	}
	host.Files.FlockErr = nil
	recovered, retry, err := attemptPortLease(port, root, 8080, "owner")
	if err != nil || recovered != nil || !retry {
		t.Fatalf("second attempt = %+v, %v, %v, want a stale recovery retry", recovered, retry, err)
	}
}

func TestReservationIdentityAliveThroughTheDouble(t *testing.T) {
	host, port := lockSliceSystem()
	root := t.TempDir()
	value, err := token(port)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := openReservationIdentity(port, root, value)
	if err != nil || identity == nil {
		t.Fatalf("open reservation identity = %v, %v, want a lock", identity, err)
	}
	_ = identity.Close()
	device, inode, err := reservationIdentityMetadata(mustOpenForTest(t, root, value))
	if err != nil {
		t.Fatal(err)
	}
	alive, err := reservationIdentityAlive(port, root, value, device, inode)
	if err != nil || alive {
		t.Fatalf("free reservation identity alive = %v, %v, want false with no error", alive, err)
	}
	host.Files.FlockErr = unix.EWOULDBLOCK
	alive, err = reservationIdentityAlive(port, root, value, device, inode)
	if err != nil || !alive {
		t.Fatalf("held reservation identity alive = %v, %v, want true with no error", alive, err)
	}
	if _, err = reservationIdentityAlive(port, root, "not-a-token", 0, 0); err == nil {
		t.Fatal("invalid reservation identity token = nil, want error")
	}
}

func mustOpenForTest(t *testing.T, root, value string) *os.File {
	t.Helper()
	path, err := reservationIdentityPath(root, value)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })

	return file
}

func TestEnsureReservationIdentityLockedThroughTheDouble(t *testing.T) {
	host, port := lockSliceSystem()
	file, err := os.CreateTemp(t.TempDir(), "identity-*.lock")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if err = ensureReservationIdentityLocked(port, file); err != nil {
		t.Fatalf("ensure locked through the double: %v", err)
	}
	host.Files.FlockErr = unix.EWOULDBLOCK
	if err = ensureReservationIdentityLocked(port, file); err == nil {
		t.Fatal("ensure locked on a held lock = nil, want error")
	}
	if err = ensureReservationIdentityLocked(port, nil); err == nil {
		t.Fatal("ensure locked on nil = nil, want error")
	}
}

func TestAdmissionStateCloseReleasesThroughTheDouble(t *testing.T) {
	_, port := lockSliceSystem()
	root := t.TempDir()
	value, err := token(port)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := os.CreateTemp(t.TempDir(), "admission-*.lock")
	if err != nil {
		t.Fatal(err)
	}
	state := &admissionState{
		config: application.RunConfig{EvidenceRoot: root}, value: value, identity: identity,
		cleanupWait: time.Second, system: port,
	}
	state.close()
	if !state.closed {
		t.Error("admission state did not mark itself closed")
	}
	if err = identity.Close(); err == nil {
		t.Error("admission identity still open after close")
	}
	path, pathError := reservationIdentityPath(root, value)
	if pathError != nil {
		t.Fatal(pathError)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("reservation identity still present: %v", err)
	}
}

func TestLockCoordinationHelpersTakeThePort(t *testing.T) {
	_, port := lockSliceSystem()
	root := t.TempDir()
	lock, err := lockCoordinationForRelease(port, root)
	if err != nil || lock == nil {
		t.Fatalf("lock for release = %v, %v, want a lock", lock, err)
	}
	if err = releaseCoordinationLock(port, lock); err != nil {
		t.Fatalf("release after lock for release: %v", err)
	}
	activation, err := lockCoordinationForActivation(port, root)
	if err != nil || activation == nil {
		t.Fatalf("lock for activation = %v, %v, want a lock", activation, err)
	}
	if err = releaseCoordinationLock(port, activation); err != nil {
		t.Fatalf("release after lock for activation: %v", err)
	}
}
