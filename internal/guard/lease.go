package guard

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/internal/policy"
	"golang.org/x/sys/unix"
)

type leaseOwner struct {
	SchemaVersion  int    `json:"schemaVersion"`
	PID            int    `json:"pid"`
	Token          string `json:"token,omitempty"`
	Port           int    `json:"port,omitempty"`
	Owner          string `json:"owner,omitempty"`
	Class          string `json:"class,omitempty"`
	IdentityDevice uint64 `json:"identityDevice,omitempty"`
	IdentityInode  uint64 `json:"identityInode,omitempty"`
}

// Session identifies an owned or inherited guarded session.
type Session struct {
	Inherited    bool
	Path, Token  string
	RecordPath   string
	Requested    ReservationVector
	Allocation   ReservationVector
	WaitDuration time.Duration
	identityLock *os.File
}

// SerializesHeavyWork reports whether class competes for the single heavy-work lease.
// Long-lived services stay outside that lease so they never starve guarded gates.
func SerializesHeavyWork(class policy.TaskClass) bool {
	return class != policy.TaskService
}

// PortLease identifies ownership of one bounded service port.
type PortLease struct {
	Path, Owner, Token string
	Port               int
	identityLock       *os.File
}

func livePID(pid int) bool {
	if pid <= 0 {
		return false
	}

	err := syscall.Kill(pid, 0)

	return err == nil || errors.Is(err, syscall.EPERM)
}

func readLeaseOwner(path string) (*leaseOwner, error) {
	data, err := os.ReadFile(filepath.Join(path, "owner.json"))
	if err != nil {
		return nil, err
	}

	var owner leaseOwner
	if err = json.Unmarshal(data, &owner); err != nil {
		return nil, err
	}

	return &owner, nil
}

func writeLeaseOwner(path string, owner leaseOwner) error {
	data, err := json.Marshal(owner)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(path, "owner.json"), append(data, '\n'), 0o600)
}

func token() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	return hex.EncodeToString(bytes), nil
}

var sessionTokenPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func sessionRecordPath(root, sessionToken string) (string, bool) {
	if !sessionTokenPattern.MatchString(sessionToken) {
		return "", false
	}
	return filepath.Join(root, "sessions", sessionToken+".json"), true
}

func readSessionRecord(root, sessionToken string) (*leaseOwner, error) {
	path, valid := sessionRecordPath(root, sessionToken)
	if !valid {
		return nil, errors.New("invalid resource session token")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var owner leaseOwner
	if err = json.Unmarshal(data, &owner); err != nil {
		return nil, err
	}

	return &owner, nil
}

func writeSessionRecord(root string, owner leaseOwner) (string, error) {
	path, valid := sessionRecordPath(root, owner.Token)
	if !valid {
		return "", errors.New("invalid resource session token")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}

	data, err := json.Marshal(owner)
	if err != nil {
		return "", err
	}
	if err = os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", err
	}

	return path, nil
}

func validSessionRecord(owner *leaseOwner, recordToken string) bool {
	if owner == nil || owner.SchemaVersion != 1 || owner.PID <= 0 || owner.Token != recordToken {
		return false
	}

	switch policy.TaskClass(owner.Class) {
	case policy.TaskEphemeral, policy.TaskService, policy.TaskTransactional, policy.TaskRelease:
		return true
	default:
		return false
	}
}

func compatibilityOwnerAlive(root string, owner *leaseOwner) (bool, error) {
	if owner == nil {
		return false, errors.New("compatibility owner is missing")
	}
	if (owner.IdentityDevice == 0) != (owner.IdentityInode == 0) {
		return false, errors.New("compatibility owner identity metadata is invalid")
	}
	if owner.IdentityDevice == 0 {
		return livePID(owner.PID), nil
	}

	return reservationIdentityAlive(root, owner.Token, owner.IdentityDevice, owner.IdentityInode)
}

func compatibilityStateDeferred(reason string) error {
	return fmt.Errorf("%w: %s; inspect the shared HIPPO state before retrying", errCoordinationDeferred, reason)
}

// pruneSessionRecords removes only structurally valid records whose owning process is gone.
// Unreadable or malformed records remain fail-closed evidence until an operator resolves them.
func pruneSessionRecords(root string) error {
	directory := filepath.Join(root, "sessions")
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return compatibilityStateDeferred("exclusive compatibility session inventory cannot be enumerated")
	}

	for _, entry := range entries {
		recordToken := strings.TrimSuffix(entry.Name(), ".json")
		owner, readError := readSessionRecord(root, recordToken)
		if readError != nil || !validSessionRecord(owner, recordToken) {
			continue
		}
		alive, aliveError := compatibilityOwnerAlive(root, owner)
		if aliveError != nil {
			return compatibilityStateDeferred("exclusive compatibility session identity is unverifiable")
		}
		if !alive { //nolint:nestif // Stale session cleanup must retain heavy/session/identity ordering in one pass.
			heavyPath := filepath.Join(root, "heavy.lock")
			heavyOwner, heavyError := readLeaseOwner(heavyPath)
			if heavyError == nil && heavyOwner.Token == owner.Token {
				if err = os.RemoveAll(heavyPath); err != nil {
					return compatibilityStateDeferred("stale compatibility heavy ownership cannot be removed")
				}
			} else if heavyError != nil && !errors.Is(heavyError, os.ErrNotExist) {
				return compatibilityStateDeferred("exclusive compatibility heavy ownership is unverifiable")
			}
			if err = os.Remove(filepath.Join(directory, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				return compatibilityStateDeferred("a stale compatibility session record cannot be removed")
			}
			if err = removeReservationIdentity(root, owner.Token); err != nil {
				return compatibilityStateDeferred("a stale compatibility session identity cannot be removed")
			}
		}
	}

	return nil
}

func hasLiveSessionRecords(root string) (bool, error) {
	entries, err := os.ReadDir(filepath.Join(root, "sessions"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, compatibilityStateDeferred("exclusive compatibility session inventory cannot be enumerated")
	}

	for _, entry := range entries {
		recordToken := strings.TrimSuffix(entry.Name(), ".json")
		owner, readError := readSessionRecord(root, recordToken)
		if readError != nil || !validSessionRecord(owner, recordToken) {
			return true, nil //nolint:nilerr // Unreadable records are deliberately retained as live fail-closed evidence.
		}
		alive, aliveError := compatibilityOwnerAlive(root, owner)
		if aliveError != nil || alive {
			return true, nil //nolint:nilerr // Unverifiable identities are deliberately retained as live fail-closed evidence.
		}
	}

	return false, nil
}

func removeExclusiveMarkerIfIdle(root string) error {
	if err := pruneSessionRecords(root); err != nil {
		return err
	}
	liveSessions, err := hasLiveSessionRecords(root)
	if err != nil {
		return err
	}
	if liveSessions {
		return nil
	}

	marker, present, err := readCoordinationMarker(root)
	if err != nil || !present || marker.Mode != coordinationModeExclusive {
		return err
	}

	if err = os.Remove(coordinationMarkerPath(root)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

// InheritedSession reports whether candidate belongs to a live guarded owner.
func InheritedSession(root, candidate string) bool {
	if candidate == "" {
		return false
	}

	if owner, err := readSessionRecord(root, candidate); err == nil &&
		owner.SchemaVersion == 1 && owner.Token == candidate {
		alive, aliveError := compatibilityOwnerAlive(root, owner)
		if aliveError == nil && alive {
			return true
		}
	}

	owner, err := readLeaseOwner(filepath.Join(root, "heavy.lock"))

	if err != nil || owner.SchemaVersion != 1 || owner.Token != candidate {
		return false
	}
	alive, aliveError := compatibilityOwnerAlive(root, owner)

	return aliveError == nil && alive
}

// DescribeHeavyLease explains who holds the heavy-work lease, for actionable deferrals.
func DescribeHeavyLease(root string) string {
	owner, err := readLeaseOwner(filepath.Join(root, "heavy.lock"))
	alive := false
	if err == nil && owner.SchemaVersion == 1 {
		alive, err = compatibilityOwnerAlive(root, owner)
	}
	if errors.Is(err, os.ErrNotExist) {
		return "the heavy-work lease reports no live owner"
	}
	if err != nil || owner.SchemaVersion != 1 {
		return "the heavy-work lease owner cannot be verified; inspect the shared HIPPO state before retrying"
	}
	if !alive {
		return "the heavy-work lease reports no live owner"
	}

	class := owner.Class
	if class == "" {
		class = "unknown"
	}

	return fmt.Sprintf("the heavy-work lease is held by pid %d (class %s)", owner.PID, class)
}

func heavyLeaseHeld(lockPath string) bool {
	owner, err := readLeaseOwner(lockPath)
	if err != nil || owner.SchemaVersion != 1 {
		return true
	}
	root := filepath.Dir(lockPath)
	alive, aliveError := compatibilityOwnerAlive(root, owner)

	return aliveError != nil || alive
}

func acquireHeavySessionLocked(root string, class policy.TaskClass) (*Session, bool, error) {
	lockPath := filepath.Join(root, "heavy.lock")

	for range 2 {
		if err := os.Mkdir(lockPath, 0o700); err == nil {
			session, registerError := registerSession(root, lockPath, class)
			if registerError != nil {
				_ = os.RemoveAll(lockPath)
				_ = removeExclusiveMarkerIfIdle(root)
			}

			return session, false, registerError
		} else if !errors.Is(err, os.ErrExist) {
			_ = removeExclusiveMarkerIfIdle(root)

			return nil, false, err
		}

		if heavyLeaseHeld(lockPath) {
			return nil, true, nil
		}
		if err := os.RemoveAll(lockPath); err != nil {
			return nil, false, err
		}
	}

	return nil, true, nil
}

func acquireSessionLocked(root, inheritedToken string, class policy.TaskClass) (*Session, bool, error) {
	if err := pruneSessionRecords(root); err != nil {
		return nil, false, err
	}
	if err := ensureExclusiveCoordination(root); err != nil {
		return nil, false, err
	}

	if InheritedSession(root, inheritedToken) {
		return &Session{
			Inherited: true,
			Token:     inheritedToken,
		}, false, nil
	}

	if SerializesHeavyWork(class) {
		return acquireHeavySessionLocked(root, class)
	}

	session, err := registerSession(root, "", class)
	if err != nil {
		_ = removeExclusiveMarkerIfIdle(root)
	}

	return session, false, err
}

func waitForLeaseRetry(ctx context.Context, deadline time.Time) error {
	delay := min(time.Second, time.Until(deadline))
	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// AcquireSession obtains a guarded session, returning nil after the lease wait expires.
// Heavy classes serialize on one lease; services record an inheritable session only.
func AcquireSession(ctx context.Context, root, inheritedToken string, class policy.TaskClass, wait time.Duration) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if wait < 0 {
		return nil, errors.New("lease wait must be nonnegative")
	}

	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(wait)

	for {
		remaining := max(time.Until(deadline), 0)
		coordinationLock, err := acquireCoordinationLock(ctx, root, remaining)
		if err != nil {
			return nil, err
		}

		session, deferred, acquireError := acquireSessionLocked(root, inheritedToken, class)
		if err = errors.Join(acquireError, releaseCoordinationLock(coordinationLock)); err != nil {
			return nil, err
		}
		if !deferred {
			return session, nil
		}
		if wait == 0 || !time.Now().Before(deadline) {
			return nil, nil
		}

		if err = waitForLeaseRetry(ctx, deadline); err != nil {
			return nil, err
		}
	}
}

// registerSession mints one session, recording it on the heavy lease when lockPath is set.
func registerSession(root, lockPath string, class policy.TaskClass) (*Session, error) {
	value, tokenError := token()
	if tokenError != nil {
		return nil, tokenError
	}

	identity, identityError := openReservationIdentity(root, value)
	if identityError != nil {
		return nil, identityError
	}
	identityDevice, identityInode, identityError := reservationIdentityMetadata(identity)
	if identityError != nil {
		_ = identity.Close()
		_ = removeReservationIdentity(root, value)

		return nil, identityError
	}
	owner := leaseOwner{
		SchemaVersion:  1,
		PID:            os.Getpid(),
		Token:          value,
		Class:          string(class),
		IdentityDevice: identityDevice,
		IdentityInode:  identityInode,
	}
	if lockPath != "" {
		if writeError := writeLeaseOwner(lockPath, owner); writeError != nil {
			_ = identity.Close()
			_ = removeReservationIdentity(root, value)

			return nil, writeError
		}
	}

	recordPath, recordError := writeSessionRecord(root, owner)
	if recordError != nil {
		_ = identity.Close()
		_ = removeReservationIdentity(root, value)

		return nil, recordError
	}

	return &Session{
		Path:         lockPath,
		Token:        value,
		RecordPath:   recordPath,
		identityLock: identity,
	}, nil
}

// ReleaseSession removes only the session record and heavy-work lease owned by session.
func ReleaseSession(root string, session *Session) (returnError error) {
	if session == nil || session.Inherited {
		return nil
	}

	coordinationLock, err := lockCoordinationForRelease(root)
	if err != nil {
		return err
	}
	defer func() {
		returnError = errors.Join(returnError, releaseCoordinationLock(coordinationLock))
	}()

	if session.RecordPath != "" {
		expectedRecord, valid := sessionRecordPath(root, session.Token)
		if !valid || session.RecordPath != expectedRecord {
			return errors.New("refusing to release an invalid resource session")
		}
		if err = os.Remove(session.RecordPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	if session.Path == "" {
		return errors.Join(releaseReservationIdentity(root, session), removeExclusiveMarkerIfIdle(root))
	}

	expected := filepath.Join(root, "heavy.lock")
	if session.Path != expected {
		return errors.New("refusing to release an invalid resource session")
	}

	owner, err := readLeaseOwner(expected)
	if err != nil {
		return err
	}
	if owner.PID != os.Getpid() || owner.Token != session.Token {
		return errors.New("refusing to release a resource session owned by another process")
	}

	if err = os.RemoveAll(expected); err != nil {
		return err
	}

	return errors.Join(releaseReservationIdentity(root, session), removeExclusiveMarkerIfIdle(root))
}

var portOwnerPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

func openPortLeaseIdentity(path string) (*os.File, error) {
	identityPath := filepath.Join(path, "identity.lock")
	identity, err := os.OpenFile(identityPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(identity.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = identity.Close()

		return nil, err
	}

	return identity, nil
}

func portLeaseIdentityLive(path string, owner *leaseOwner) (bool, error) {
	if owner.Token == "" {
		return livePID(owner.PID), nil
	}
	if !sessionTokenPattern.MatchString(owner.Token) {
		return true, errors.New("port lease identity is unverifiable")
	}
	identity, err := os.OpenFile(filepath.Join(path, "identity.lock"), os.O_RDWR, 0o600)
	if errors.Is(err, os.ErrNotExist) {
		return true, errors.New("port lease identity is unverifiable")
	}
	if err != nil {
		return true, errors.New("port lease identity is unverifiable")
	}
	defer func() { _ = identity.Close() }()
	var identityInfo unix.Stat_t
	if err = unix.Fstat(int(identity.Fd()), &identityInfo); err != nil {
		return true, errors.New("port lease identity is unverifiable")
	}
	device, deviceError := filesystemIdentifier(identityInfo.Dev)
	inode, inodeError := filesystemIdentifier(identityInfo.Ino)
	if deviceError != nil || inodeError != nil || owner.IdentityDevice == 0 || owner.IdentityInode == 0 ||
		owner.IdentityDevice != device || owner.IdentityInode != inode {
		return true, errors.New("port lease identity is unverifiable")
	}
	if err = unix.Flock(int(identity.Fd()), unix.LOCK_EX|unix.LOCK_NB); errors.Is(err, unix.EWOULDBLOCK) {
		return true, nil
	}
	if err != nil {
		return true, errors.New("port lease identity is unverifiable")
	}
	unlockError := unix.Flock(int(identity.Fd()), unix.LOCK_UN)

	return false, unlockError
}

// AcquirePortLease obtains one validated port lease and reclaims a stale owner once.
func AcquirePortLease(root string, port int, ownerName string, minimum, maximum int) (*PortLease, error) { //nolint:gocognit // Creation and stale-owner recovery are one bounded ownership transaction.
	if port < minimum || port > maximum {
		return nil, fmt.Errorf("port must be between %d and %d", minimum, maximum)
	}

	if !portOwnerPattern.MatchString(ownerName) {
		return nil, errors.New("port lease owner is invalid")
	}

	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}

	path := filepath.Join(root, fmt.Sprintf("%d.lock", port))

	for range 2 {
		if err := os.Mkdir(path, 0o700); err == nil { //nolint:nestif // Creation and stale-owner recovery are one bounded ownership transaction.
			value, tokenError := token()
			if tokenError != nil {
				_ = os.RemoveAll(path)

				return nil, tokenError
			}
			identity, identityError := openPortLeaseIdentity(path)
			if identityError != nil {
				_ = os.RemoveAll(path)

				return nil, identityError
			}
			var identityInfo unix.Stat_t
			if identityError = unix.Fstat(int(identity.Fd()), &identityInfo); identityError != nil {
				_ = identity.Close()
				_ = os.RemoveAll(path)

				return nil, identityError
			}
			device, deviceError := filesystemIdentifier(identityInfo.Dev)
			inode, inodeError := filesystemIdentifier(identityInfo.Ino)
			if deviceError != nil || inodeError != nil {
				_ = identity.Close()
				_ = os.RemoveAll(path)

				return nil, errors.New("port lease identity metadata is unavailable")
			}
			owner := leaseOwner{
				SchemaVersion:  1,
				PID:            os.Getpid(),
				Token:          value,
				Port:           port,
				Owner:          ownerName,
				IdentityDevice: device,
				IdentityInode:  inode,
			}
			if writeError := writeLeaseOwner(path, owner); writeError != nil {
				_ = unix.Flock(int(identity.Fd()), unix.LOCK_UN)
				_ = identity.Close()
				_ = os.RemoveAll(path)
				return nil, writeError
			}

			return &PortLease{
				Path: path, Port: port, Owner: ownerName, Token: value, identityLock: identity,
			}, nil
		} else if !errors.Is(err, os.ErrExist) {
			return nil, err
		}

		marker, markerError := readLeaseOwner(path)
		if markerError != nil || marker.SchemaVersion != 1 || marker.Port != port {
			return nil, fmt.Errorf("port %d is already leased", port)
		}
		live, identityError := portLeaseIdentityLive(path, marker)
		if identityError != nil {
			return nil, fmt.Errorf("port %d is already leased", port)
		}
		if live {
			// A live peer owning this port is retryable lease pressure, which the
			// public contract reports as exit 75, not a failure the caller has no
			// basis to retry.
			return nil, fmt.Errorf("%w: port %d is already leased", errCoordinationDeferred, port)
		}
		if err := os.RemoveAll(path); err != nil {
			return nil, err
		}
	}

	return nil, fmt.Errorf("port %d could not be leased", port)
}

// ReleasePortLease removes only the port marker owned by lease.
func ReleasePortLease(root string, lease *PortLease) error {
	if lease == nil {
		return nil
	}

	expected := filepath.Join(root, fmt.Sprintf("%d.lock", lease.Port))
	if lease.Path != expected {
		return errors.New("refusing to release an invalid port lease")
	}

	owner, err := readLeaseOwner(expected)
	if err != nil {
		return err
	}
	if owner.PID != os.Getpid() || owner.Port != lease.Port || owner.Owner != lease.Owner || owner.Token != lease.Token {
		return errors.New("refusing to release a port lease owned by another process")
	}
	var releaseError error
	if lease.identityLock != nil {
		releaseError = lease.identityLock.Close()
		lease.identityLock = nil
	}

	return errors.Join(releaseError, os.RemoveAll(expected))
}

func abandonPortLeaseIdentity(lease *PortLease) error {
	if lease == nil || lease.identityLock == nil {
		return nil
	}
	err := lease.identityLock.Close()
	lease.identityLock = nil

	return err
}
