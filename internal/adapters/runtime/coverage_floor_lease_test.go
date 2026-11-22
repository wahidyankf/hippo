package runtime //nolint:testpackage // Floor leg adds the missing unit branches, as run_test.go does.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/application"
	coordination "github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/identity"
	"github.com/wahidyankf/hippo/internal/policy"
)

const (
	floorToken = "0123456789abcdef0123456789abcdef"
	floorDead  = "fedcba9876543210fedcba9876543210"
)

func floorLiveOwner(pid int, token string) leaseOwner {
	return NewExclusiveSessionRecord(pid, token, policy.TaskEphemeral, 0, 0)
}

func TestFloorSessionRecordPathRefusesBadToken(t *testing.T) {
	if _, valid := sessionRecordPath(t.TempDir(), "../escape"); valid {
		t.Fatal("sessionRecordPath on an invalid token is valid, want refusal")
	}
}

func TestFloorReadSessionRecordRefusesBadToken(t *testing.T) {
	if _, err := readSessionRecord(t.TempDir(), "../escape"); err == nil {
		t.Fatal("readSessionRecord on an invalid token = nil, want error")
	}
}

func TestFloorWriteSessionRecordRefusesBadToken(t *testing.T) {
	if _, err := writeSessionRecord(t.TempDir(), leaseOwner{Token: "../escape"}); err == nil {
		t.Fatal("writeSessionRecord on an invalid token = nil, want error")
	}
}

func TestFloorWriteSessionRecordRefusesFileRoot(t *testing.T) {
	marker, err := os.CreateTemp(t.TempDir(), "root-*")
	if err != nil {
		t.Fatal(err)
	}
	_ = marker.Close()
	if _, err := writeSessionRecord(marker.Name(), floorLiveOwner(os.Getpid(), floorToken)); err == nil {
		t.Fatal("writeSessionRecord under a file = nil, want error")
	}
}

func TestFloorCompatibilityOwnerAliveRefusesNil(t *testing.T) {
	_, port := floorDoublesSystem()
	if _, err := compatibilityOwnerAlive(port, t.TempDir(), nil); err == nil {
		t.Fatal("compatibilityOwnerAlive(nil) = nil, want error")
	}
}

func TestFloorPruneSessionRecordsRefusesFileSessions(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sessions"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, port := floorDoublesSystem()
	if err := pruneSessionRecords(port, root); err == nil {
		t.Fatal("pruneSessionRecords with a file sessions inventory = nil, want error")
	}
}

func TestFloorHasLiveSessionRecordsMatrix(t *testing.T) {
	write := func(t *testing.T, root, name, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, "sessions"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "sessions", name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("unreadable inventory", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "sessions"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, port := floorDoublesSystem()
		if _, err := hasLiveSessionRecords(port, root); err == nil {
			t.Fatal("hasLiveSessionRecords with a file inventory = nil, want error")
		}
	})
	t.Run("undecodable record", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, floorToken+".json", "[[[")
		_, port := floorDoublesSystem()
		if _, err := hasLiveSessionRecords(port, root); err == nil {
			t.Fatal("hasLiveSessionRecords with garbage = nil, want error")
		}
	})
	t.Run("future schema", func(t *testing.T) {
		root := t.TempDir()
		owner := floorLiveOwner(os.Getpid(), floorToken)
		owner.SchemaVersion = 2
		if _, err := writeSessionRecord(root, owner); err != nil {
			t.Fatal(err)
		}
		_, port := floorDoublesSystem()
		if _, err := hasLiveSessionRecords(port, root); err == nil {
			t.Fatal("hasLiveSessionRecords with schema 2 = nil, want error")
		}
	})
	t.Run("invalid record", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, floorToken+".json", `{"schemaVersion":1,"pid":1,"token":"`+floorDead+`","class":"ephemeral"}`)
		_, port := floorDoublesSystem()
		if _, err := hasLiveSessionRecords(port, root); err == nil {
			t.Fatal("hasLiveSessionRecords with a token mismatch = nil, want error")
		}
	})
	t.Run("unverifiable identity", func(t *testing.T) {
		root := t.TempDir()
		owner := floorLiveOwner(os.Getpid(), floorToken)
		owner.IdentityDevice = 5
		if _, err := writeSessionRecord(root, owner); err != nil {
			t.Fatal(err)
		}
		_, port := floorDoublesSystem()
		if _, err := hasLiveSessionRecords(port, root); err == nil {
			t.Fatal("hasLiveSessionRecords with mismatched identity = nil, want error")
		}
	})
}

func TestFloorRemoveExclusiveMarkerIfIdleMatrix(t *testing.T) {
	_, port := floorDoublesSystem()
	t.Run("prune failure", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "sessions"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := removeExclusiveMarkerIfIdle(port, root); err == nil {
			t.Fatal("removeExclusiveMarkerIfIdle with a file inventory = nil, want error")
		}
	})
	t.Run("marker failure", func(t *testing.T) {
		root := t.TempDir()
		writeFloorMarker(t, root, "[[[")
		if err := removeExclusiveMarkerIfIdle(port, root); err == nil {
			t.Fatal("removeExclusiveMarkerIfIdle with a corrupt marker = nil, want error")
		}
	})
	t.Run("remove failure", func(t *testing.T) {
		root := t.TempDir()
		writeFloorMarker(t, root, `{"schemaVersion":1,"mode":"exclusive"}`)
		if err := os.Chmod(root, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(root, 0o750) })
		if err := removeExclusiveMarkerIfIdle(port, root); err == nil {
			t.Fatal("removeExclusiveMarkerIfIdle under a read-only root = nil, want error")
		}
	})
}

func TestFloorInheritedSessionWithoutCandidateIsFalse(t *testing.T) {
	_, port := floorDoublesSystem()
	if inheritedSession(port, t.TempDir(), "") {
		t.Fatal("inheritedSession with an empty candidate = true, want false")
	}
}

func TestFloorDescribeHeavyLeaseWithoutOwner(t *testing.T) {
	if got := DescribeHeavyLease(t.TempDir()); got != "the heavy-work lease reports no live owner" {
		t.Fatalf("DescribeHeavyLease without an owner = %q", got)
	}
}

func TestFloorDescribeHeavyLeaseNamesUnknownClass(t *testing.T) {
	root := t.TempDir()
	heavyPath := filepath.Join(root, "heavy.lock")
	if err := os.Mkdir(heavyPath, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := writeLeaseOwner(heavyPath, leaseOwner{SchemaVersion: 1, PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	got := DescribeHeavyLease(root)
	if want := "the heavy-work lease is held by pid " + strconv.Itoa(os.Getpid()) + " (class unknown)"; got != want {
		t.Fatalf("DescribeHeavyLease with an empty class = %q, want %q", got, want)
	}
}

func TestFloorHeavyLeaseHeldMatrix(t *testing.T) {
	_, port := floorDoublesSystem()
	t.Run("future schema", func(t *testing.T) {
		root := t.TempDir()
		heavyPath := filepath.Join(root, "heavy.lock")
		if err := os.Mkdir(heavyPath, 0o750); err != nil {
			t.Fatal(err)
		}
		owner := floorLiveOwner(os.Getpid(), floorToken)
		owner.SchemaVersion = 2
		if err := writeLeaseOwner(heavyPath, owner); err != nil {
			t.Fatal(err)
		}
		if _, err := heavyLeaseHeld(port, heavyPath); err == nil {
			t.Fatal("heavyLeaseHeld with schema 2 = nil, want error")
		}
	})
	t.Run("unverifiable identity", func(t *testing.T) {
		root := t.TempDir()
		heavyPath := filepath.Join(root, "heavy.lock")
		if err := os.Mkdir(heavyPath, 0o750); err != nil {
			t.Fatal(err)
		}
		owner := floorLiveOwner(os.Getpid(), floorToken)
		owner.IdentityDevice = 5
		if err := writeLeaseOwner(heavyPath, owner); err != nil {
			t.Fatal(err)
		}
		if _, err := heavyLeaseHeld(port, heavyPath); err == nil {
			t.Fatal("heavyLeaseHeld with mismatched identity = nil, want error")
		}
	})
}

func TestFloorAcquireHeavySessionLockedMatrix(t *testing.T) {
	_, port := floorDoublesSystem()
	t.Run("uncreatable lock", func(t *testing.T) {
		marker, err := os.CreateTemp(t.TempDir(), "root-*")
		if err != nil {
			t.Fatal(err)
		}
		_ = marker.Close()
		if _, _, err := acquireHeavySessionLocked(
			port, marker.Name(), policy.TaskEphemeral, identity.DisplayLabel{}, nil,
		); err == nil {
			t.Fatal("acquireHeavySessionLocked under a file = nil, want error")
		}
	})
	t.Run("unreadable owner", func(t *testing.T) {
		root := t.TempDir()
		heavyPath := filepath.Join(root, "heavy.lock")
		if err := os.Mkdir(heavyPath, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(heavyPath, "owner.json"), []byte("[[["), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := acquireHeavySessionLocked(
			port, root, policy.TaskEphemeral, identity.DisplayLabel{}, nil,
		); err == nil {
			t.Fatal("acquireHeavySessionLocked with garbage owner = nil, want error")
		}
	})
	t.Run("stale lock without removal", func(t *testing.T) {
		root := t.TempDir()
		heavyPath := filepath.Join(root, "heavy.lock")
		if err := os.Mkdir(heavyPath, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := writeLeaseOwner(heavyPath, floorLiveOwner(1<<30, floorToken)); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(root, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(root, 0o750) })
		_, deferred, err := acquireHeavySessionLocked(
			port, root, policy.TaskEphemeral, identity.DisplayLabel{}, nil,
		)
		if err == nil || deferred {
			t.Fatalf("acquireHeavySessionLocked on a stale read-only lock = %v, %v, want error", deferred, err)
		}
	})
}

func TestFloorAcquireSessionLockedMatrix(t *testing.T) {
	t.Run("prune failure", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "sessions"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		state := &admissionState{system: system.Production, config: application.RunConfig{EvidenceRoot: root}}
		if _, _, err := acquireSessionLocked(root, "", policy.TaskEphemeral, identity.DisplayLabel{}, state); err == nil {
			t.Fatal("acquireSessionLocked with a file inventory = nil, want error")
		}
	})
	t.Run("heavy stat failure", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Symlink("heavy.lock", filepath.Join(root, "heavy.lock")); err != nil {
			t.Fatal(err)
		}
		state := &admissionState{system: system.Production, config: application.RunConfig{EvidenceRoot: root}}
		if _, _, err := acquireSessionLocked(root, "", policy.TaskEphemeral, identity.DisplayLabel{}, state); err == nil {
			t.Fatal("acquireSessionLocked with a looping heavy lock = nil, want error")
		}
	})
	t.Run("service registration failure", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "reservation-identities"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		state := &admissionState{system: system.Production, config: application.RunConfig{EvidenceRoot: root}}
		if _, _, err := acquireSessionLocked(
			root, "", policy.TaskService, identity.DisplayLabel{}, state,
		); err == nil {
			t.Fatal("acquireSessionLocked with an uncreatable identity = nil, want error")
		}
	})
}

func TestFloorRegisterSessionRefusesEmptyRandom(t *testing.T) {
	host, port := floorDoublesSystem()
	host.Random.Bytes = nil
	if _, err := registerSession(port, "/root", "", policy.TaskEphemeral, identity.DisplayLabel{}, nil); err == nil {
		t.Fatal("registerSession without random bytes = nil, want error")
	}
}

func TestFloorReleaseSessionContextRefusesFileRoot(t *testing.T) {
	marker, err := os.CreateTemp(t.TempDir(), "root-*")
	if err != nil {
		t.Fatal(err)
	}
	_ = marker.Close()
	if err := ReleaseSessionContext(t.Context(), marker.Name(), &Session{Token: floorToken}); err == nil {
		t.Fatal("ReleaseSessionContext under a file = nil, want error")
	}
}

func TestFloorReleaseSessionOwnershipLockedMatrix(t *testing.T) { //nolint:gocognit // One release-ownership matrix covers every refusal in a single fixture root.
	_, port := floorDoublesSystem()
	t.Run("invalid record path", func(t *testing.T) {
		session := &Session{RecordPath: "elsewhere.json", Token: "../escape"}
		if err := releaseSessionOwnershipLocked(port, t.TempDir(), session); err == nil {
			t.Fatal("releaseSessionOwnershipLocked with a forged record = nil, want error")
		}
	})
	t.Run("unremovable record", func(t *testing.T) {
		root := t.TempDir()
		sessions := filepath.Join(root, "sessions")
		if err := os.Mkdir(sessions, 0o750); err != nil {
			t.Fatal(err)
		}
		if _, err := writeSessionRecord(root, floorLiveOwner(os.Getpid(), floorToken)); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(sessions, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(sessions, 0o750) })
		session := &Session{RecordPath: filepath.Join(sessions, floorToken+".json"), Token: floorToken}
		if err := releaseSessionOwnershipLocked(port, root, session); err == nil {
			t.Fatal("releaseSessionOwnershipLocked with an unremovable record = nil, want error")
		}
	})
	t.Run("foreign heavy lock", func(t *testing.T) {
		session := &Session{Path: filepath.Join(t.TempDir(), "heavy.lock"), Token: floorToken}
		if err := releaseSessionOwnershipLocked(port, t.TempDir(), session); err == nil {
			t.Fatal("releaseSessionOwnershipLocked with a foreign lock = nil, want error")
		}
	})
	t.Run("unreadable heavy owner", func(t *testing.T) {
		root := t.TempDir()
		heavyPath := filepath.Join(root, "heavy.lock")
		if err := os.Mkdir(heavyPath, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(heavyPath, "owner.json"), []byte("[[["), 0o600); err != nil {
			t.Fatal(err)
		}
		session := &Session{Path: heavyPath, Token: floorToken}
		if err := releaseSessionOwnershipLocked(port, root, session); err == nil {
			t.Fatal("releaseSessionOwnershipLocked with garbage owner = nil, want error")
		}
	})
	t.Run("mismatched heavy owner", func(t *testing.T) {
		root := t.TempDir()
		heavyPath := filepath.Join(root, "heavy.lock")
		if err := os.Mkdir(heavyPath, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := writeLeaseOwner(heavyPath, floorLiveOwner(os.Getpid(), floorDead)); err != nil {
			t.Fatal(err)
		}
		session := &Session{Path: heavyPath, Token: floorToken}
		if err := releaseSessionOwnershipLocked(port, root, session); err == nil {
			t.Fatal("releaseSessionOwnershipLocked with a foreign owner = nil, want error")
		}
	})
	t.Run("unremovable heavy lock", func(t *testing.T) {
		root := t.TempDir()
		heavyPath := filepath.Join(root, "heavy.lock")
		if err := os.Mkdir(heavyPath, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := writeLeaseOwner(heavyPath, floorLiveOwner(os.Getpid(), floorToken)); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(root, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(root, 0o750) })
		session := &Session{Path: heavyPath, Token: floorToken}
		if err := releaseSessionOwnershipLocked(port, root, session); err == nil {
			t.Fatal("releaseSessionOwnershipLocked under a read-only root = nil, want error")
		}
	})
}

func TestFloorOpenPortLeaseIdentityMatrix(t *testing.T) {
	t.Run("uncreatable identity", func(t *testing.T) {
		marker, err := os.CreateTemp(t.TempDir(), "root-*")
		if err != nil {
			t.Fatal(err)
		}
		_ = marker.Close()
		if _, err := openPortLeaseIdentity(system.Production, marker.Name()); err == nil {
			t.Fatal("openPortLeaseIdentity under a file = nil, want error")
		}
	})
	t.Run("held identity", func(t *testing.T) {
		dir := t.TempDir()
		holder, err := os.OpenFile(filepath.Join(dir, "identity.lock"), os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = holder.Close() })
		if err := system.Production.Files.Flock(holder, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			t.Fatal(err)
		}
		identity, err := openPortLeaseIdentity(system.Production, dir)
		if err == nil {
			_ = identity.Close()
			t.Fatal("openPortLeaseIdentity on a held lock = nil, want error")
		}
	})
}

func TestFloorPortLeaseIdentityLiveMatrix(t *testing.T) {
	t.Run("invalid token", func(t *testing.T) {
		_, port := floorDoublesSystem()
		if _, err := portLeaseIdentityLive(port, t.TempDir(), &leaseOwner{Token: "../escape"}); err == nil {
			t.Fatal("portLeaseIdentityLive on an invalid token = nil, want error")
		}
	})
	t.Run("unopenable identity", func(t *testing.T) {
		marker, err := os.CreateTemp(t.TempDir(), "root-*")
		if err != nil {
			t.Fatal(err)
		}
		_ = marker.Close()
		_, port := floorDoublesSystem()
		if _, err := portLeaseIdentityLive(port, marker.Name(), &leaseOwner{Token: floorToken}); err == nil {
			t.Fatal("portLeaseIdentityLive under a file = nil, want error")
		}
	})
	t.Run("held identity is live", func(t *testing.T) {
		dir := t.TempDir()
		holder, err := os.OpenFile(filepath.Join(dir, "identity.lock"), os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = holder.Close() })
		if err := system.Production.Files.Flock(holder, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			t.Fatal(err)
		}
		var info syscall.Stat_t
		if err := syscall.Fstat(int(holder.Fd()), &info); err != nil {
			t.Fatal(err)
		}
		owner := &leaseOwner{
			SchemaVersion: 1, PID: os.Getpid(), Token: floorToken,
			IdentityDevice: info.Dev, IdentityInode: info.Ino,
		}
		live, err := portLeaseIdentityLive(system.Production, dir, owner)
		if err != nil || !live {
			t.Fatalf("portLeaseIdentityLive on a held identity = %v, %v, want true, nil", live, err)
		}
	})
	t.Run("flock failure is unverifiable", func(t *testing.T) {
		dir := t.TempDir()
		holder, err := os.OpenFile(filepath.Join(dir, "identity.lock"), os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = holder.Close() })
		var info syscall.Stat_t
		if err := syscall.Fstat(int(holder.Fd()), &info); err != nil {
			t.Fatal(err)
		}
		host, port := floorDoublesSystem()
		host.Files.FlockErr = errors.New("floor: flock refused")
		owner := &leaseOwner{
			SchemaVersion: 1, PID: os.Getpid(), Token: floorToken,
			IdentityDevice: info.Dev, IdentityInode: info.Ino,
		}
		if _, err := portLeaseIdentityLive(port, dir, owner); err == nil {
			t.Fatal("portLeaseIdentityLive with a failing flock = nil, want error")
		}
	})
}

func TestFloorPruneSessionRecordsStaleMatrix(t *testing.T) { //nolint:gocognit // One stale-inventory matrix covers every pruning failure in a single fixture root.
	_, port := floorDoublesSystem()
	t.Run("unverifiable identity", func(t *testing.T) {
		root := t.TempDir()
		owner := floorLiveOwner(os.Getpid(), floorToken)
		owner.IdentityDevice = 5
		if _, err := writeSessionRecord(root, owner); err != nil {
			t.Fatal(err)
		}
		if err := pruneSessionRecords(port, root); err == nil {
			t.Fatal("pruneSessionRecords with a mismatched identity = nil, want error")
		}
	})
	t.Run("stale heavy removal failure", func(t *testing.T) {
		root := t.TempDir()
		if _, err := writeSessionRecord(root, floorLiveOwner(1<<30, floorToken)); err != nil {
			t.Fatal(err)
		}
		heavyPath := filepath.Join(root, "heavy.lock")
		if err := os.Mkdir(heavyPath, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := writeLeaseOwner(heavyPath, floorLiveOwner(1<<30, floorToken)); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(root, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(root, 0o750) })
		if err := pruneSessionRecords(port, root); err == nil {
			t.Fatal("pruneSessionRecords with an unremovable heavy lock = nil, want error")
		}
	})
	t.Run("stale heavy unverifiable", func(t *testing.T) {
		root := t.TempDir()
		if _, err := writeSessionRecord(root, floorLiveOwner(1<<30, floorToken)); err != nil {
			t.Fatal(err)
		}
		heavyPath := filepath.Join(root, "heavy.lock")
		if err := os.Mkdir(heavyPath, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(heavyPath, "owner.json"), []byte("[[["), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := pruneSessionRecords(port, root); err == nil {
			t.Fatal("pruneSessionRecords with garbage heavy owner = nil, want error")
		}
	})
	t.Run("stale record removal failure", func(t *testing.T) {
		root := t.TempDir()
		if _, err := writeSessionRecord(root, floorLiveOwner(1<<30, floorToken)); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(root, "sessions"), 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "sessions"), 0o750) })
		if err := pruneSessionRecords(port, root); err == nil {
			t.Fatal("pruneSessionRecords with an unremovable record = nil, want error")
		}
	})
	t.Run("stale identity removal failure", func(t *testing.T) {
		root := t.TempDir()
		if _, err := writeSessionRecord(root, floorLiveOwner(1<<30, floorToken)); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "owner-metadata"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := pruneSessionRecords(port, root); err == nil {
			t.Fatal("pruneSessionRecords with an unremovable identity = nil, want error")
		}
	})
}

func TestFloorAttemptReservationMatrix(t *testing.T) {
	minimum, maximum, err := coordination.PlanBounds(validFloorPlan())
	if err != nil {
		t.Fatal(err)
	}
	newState := func(root string, metadata ReservationMetadata) *admissionState {
		return &admissionState{
			system: system.Production,
			config: application.RunConfig{
				EvidenceRoot:        root,
				ReservationPolicy:   ReservationPolicy{Enabled: true},
				ReservationPlan:     validFloorPlan(),
				ReservationMetadata: metadata,
				TaskClass:           policy.TaskEphemeral,
				Resolution:          policy.Resolution{ResolvedProfile: "floor"},
				ConfigHash:          "floor",
			},
			value:    floorToken,
			started:  time.Now(),
			deadline: time.Now().Add(time.Hour),
			minimum:  minimum,
			maximum:  maximum,
		}
	}
	t.Run("identity open failure", func(t *testing.T) {
		root := t.TempDir()
		floorWriteLedger(t, root, floorEmptyLedger())
		if err := os.WriteFile(filepath.Join(root, "reservation-identities"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		state := newState(root, ReservationMetadata{})
		if _, err := state.attemptReservation(context.Background(), time.Hour, time.Now()); err == nil {
			t.Fatal("attemptReservation with an uncreatable identity = nil, want error")
		}
	})
	t.Run("metadata write failure", func(t *testing.T) {
		root := t.TempDir()
		floorWriteLedger(t, root, floorEmptyLedger())
		if err := os.MkdirAll(filepath.Join(root, "owner-metadata"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(root, "owner-metadata"), 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "owner-metadata"), 0o750) })
		state := newState(root, ReservationMetadata{Source: "hippo"})
		if _, err := state.attemptReservation(context.Background(), time.Hour, time.Now()); err == nil {
			t.Fatal("attemptReservation with an unwritable metadata directory = nil, want error")
		}
	})
}

func TestFloorAttemptPortLeaseMatrix(t *testing.T) {
	t.Run("token failure", func(t *testing.T) {
		host, port := floorDoublesSystem()
		host.Random.Bytes = nil
		if _, _, err := attemptPortLease(port, t.TempDir(), 8080, "fixture"); err == nil {
			t.Fatal("attemptPortLease without random bytes = nil, want error")
		}
	})
	t.Run("identity failure", func(t *testing.T) {
		host, port := floorDoublesSystem()
		host.Random.Bytes = []byte("0123456789abcdef")
		host.Files.FlockErr = errors.New("floor: identity locked")
		if _, _, err := attemptPortLease(port, t.TempDir(), 8080, "fixture"); err == nil {
			t.Fatal("attemptPortLease with a failing identity lock = nil, want error")
		}
	})
	t.Run("leased marker", func(t *testing.T) {
		root := t.TempDir()
		leased := filepath.Join(root, "8080.lock")
		if err := os.Mkdir(leased, 0o750); err != nil {
			t.Fatal(err)
		}
		owner := floorLiveOwner(os.Getpid(), floorToken)
		owner.Port = 9090
		if err := writeLeaseOwner(leased, owner); err != nil {
			t.Fatal(err)
		}
		_, port := floorDoublesSystem()
		if _, _, err := attemptPortLease(port, root, 8080, "fixture"); err == nil {
			t.Fatal("attemptPortLease on a foreign lease = nil, want error")
		}
	})
	t.Run("stale lease retries", func(t *testing.T) {
		root := t.TempDir()
		leased := filepath.Join(root, "8080.lock")
		if err := os.Mkdir(leased, 0o750); err != nil {
			t.Fatal(err)
		}
		owner := floorLiveOwner(1<<30, floorToken)
		owner.Token = ""
		owner.Port = 8080
		if err := writeLeaseOwner(leased, owner); err != nil {
			t.Fatal(err)
		}
		_, port := floorDoublesSystem()
		_, retry, err := attemptPortLease(port, root, 8080, "fixture")
		if err != nil || !retry {
			t.Fatalf("attemptPortLease on a stale lease = %v, %v, want retry", retry, err)
		}
	})
	t.Run("stale lease removal failure", func(t *testing.T) {
		root := t.TempDir()
		leased := filepath.Join(root, "8080.lock")
		if err := os.Mkdir(leased, 0o750); err != nil {
			t.Fatal(err)
		}
		owner := floorLiveOwner(1<<30, floorToken)
		owner.Token = ""
		owner.Port = 8080
		if err := writeLeaseOwner(leased, owner); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(root, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(root, 0o750) })
		_, port := floorDoublesSystem()
		if _, _, err := attemptPortLease(port, root, 8080, "fixture"); err == nil {
			t.Fatal("attemptPortLease with an unremovable stale lease = nil, want error")
		}
	})
}

func TestFloorReleasePortLeaseMatrix(t *testing.T) {
	t.Run("foreign owner", func(t *testing.T) {
		root := t.TempDir()
		leased := filepath.Join(root, "1.lock")
		if err := os.Mkdir(leased, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := writeLeaseOwner(leased, floorLiveOwner(os.Getpid(), floorDead)); err != nil {
			t.Fatal(err)
		}
		lease := &PortLease{Path: leased, Port: 1, Owner: "fixture", Token: floorToken}
		if err := ReleasePortLease(root, lease); err == nil {
			t.Fatal("ReleasePortLease with a foreign owner = nil, want error")
		}
	})
	t.Run("close failure", func(t *testing.T) {
		root := t.TempDir()
		leased := filepath.Join(root, "1.lock")
		if err := os.Mkdir(leased, 0o750); err != nil {
			t.Fatal(err)
		}
		owner := floorLiveOwner(os.Getpid(), floorToken)
		owner.Port = 1
		if err := writeLeaseOwner(leased, owner); err != nil {
			t.Fatal(err)
		}
		closed, err := os.CreateTemp(t.TempDir(), "lease-*.lock")
		if err != nil {
			t.Fatal(err)
		}
		name := closed.Name()
		_ = closed.Close()
		closed, err = os.OpenFile(name, os.O_RDONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		_ = closed.Close()
		lease := &PortLease{Path: leased, Port: 1, Owner: "", Token: floorToken, identityLock: closed}
		if err := ReleasePortLease(root, lease); err == nil {
			t.Fatal("ReleasePortLease with a closed identity = nil, want error")
		}
	})
	t.Run("abandon nil is nil", func(t *testing.T) {
		if err := abandonPortLeaseIdentity(nil); err != nil {
			t.Fatalf("abandonPortLeaseIdentity(nil) = %v, want nil", err)
		}
	})
}
