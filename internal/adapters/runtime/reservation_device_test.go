package runtime //nolint:testpackage // Deterministic launcher and lifecycle seams are intentionally exercised inside the production package.

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/policy"
)

func reservationDevicePlan() ReservationPlan {
	return ReservationPlan{
		Capacity:  ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
		Requested: ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
	}
}

func acquireDeviceOwnerForTest(t *testing.T, root string) *Session {
	t.Helper()
	owner, err := AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, "minimal", "owner", reservationDevicePlan(), 1, time.Second,
	)
	if err != nil || owner == nil {
		t.Fatalf("device owner acquire: %v", err)
	}

	return owner
}

// rewriteReservationIdentityDeviceForTest simulates the volume reporting a new
// device number: the identity files keep their device and inode while the
// ledger's recorded device moves. The comparison outcome is identical to a real
// remount, without needing one.
func rewriteReservationIdentityDeviceForTest(t *testing.T, root, token string) {
	t.Helper()
	ledger, err := readReservationLedger(root)
	if err != nil {
		t.Fatal(err)
	}
	rewritten := false
	for i := range ledger.Owners {
		if ledger.Owners[i].Token == token {
			ledger.Owners[i].IdentityDevice++
			rewritten = true
		}
	}
	for i := range ledger.Waiters {
		if ledger.Waiters[i].Token == token {
			ledger.Waiters[i].IdentityDevice++
			rewritten = true
		}
	}
	if !rewritten {
		t.Fatalf("device rewrite found no record for token in ledger")
	}
	if err = writeReservationLedger(root, ledger); err != nil {
		t.Fatal(err)
	}
}

func identityPathsExist(root, token string) bool {
	lock, err := reservationIdentityPath(root, token)
	if err != nil {
		return false
	}
	anchor, err := reservationIdentityAnchorPath(root, token)
	if err != nil {
		return false
	}
	if _, err = os.Stat(lock); err != nil {
		return false
	}
	_, err = os.Stat(anchor)

	return err == nil
}

func TestReservationIdentityDeviceChangeRetiresDeadOwner(t *testing.T) {
	root := t.TempDir()
	owner := acquireDeviceOwnerForTest(t, root)
	// The guard died with its lock while both identity links stayed intact.
	if err := abandonReservationIdentity(owner); err != nil {
		t.Fatal(err)
	}
	rewriteReservationIdentityDeviceForTest(t, root, owner.Token)
	totals, statusError := ReservationStatus(context.Background(), root)
	if statusError != nil || totals.ActiveOwners != 0 {
		t.Fatalf("drifted dead owner was not reconciled: %+v error=%v", totals, statusError)
	}
	if identityPathsExist(root, owner.Token) {
		t.Fatal("drifted dead owner kept its identity files")
	}
	replacement, acquireError := AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, "minimal", "replacement", reservationDevicePlan(), 1, time.Second,
	)
	if acquireError != nil || replacement == nil {
		t.Fatalf("admission after drifted dead owner: %v", acquireError)
	}
	if releaseError := ReleaseReservation(root, replacement); releaseError != nil {
		t.Fatal(releaseError)
	}
}

func TestReservationIdentityDeviceChangeKeepsLiveOwner(t *testing.T) {
	root := t.TempDir()
	owner := acquireDeviceOwnerForTest(t, root)
	defer func() {
		_ = ReleaseReservation(root, owner)
	}()
	rewriteReservationIdentityDeviceForTest(t, root, owner.Token)
	before, err := os.ReadFile(reservationLedgerPath(root))
	if err != nil {
		t.Fatal(err)
	}
	totals, statusError := ReservationStatus(context.Background(), root)
	if statusError != nil || totals.ActiveOwners != 1 {
		t.Fatalf("drifted live owner was not kept: %+v error=%v", totals, statusError)
	}
	after, err := os.ReadFile(reservationLedgerPath(root))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("drifted live owner changed ledger bytes: read=%v", err)
	}
	blocked, blockedError := AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, "minimal", "blocked", reservationDevicePlan(), 1, 0,
	)
	if blocked != nil {
		_ = ReleaseReservation(root, blocked)
	}
	if blocked != nil || !errors.Is(blockedError, ErrReservationDeferred) {
		t.Fatalf("competitor bypassed drifted live owner: session=%v error=%v", blocked != nil, blockedError)
	}
	if !identityPathsExist(root, owner.Token) {
		t.Fatal("drifted live owner lost its identity files")
	}
}

// applyDeviceFaultForTest leaves one token's identity evidence incomplete behind
// a changed device number: a missing or replaced link, or no links at all.
func applyDeviceFaultForTest(t *testing.T, root, token, fault string) {
	t.Helper()
	lock, err := reservationIdentityPath(root, token)
	if err != nil {
		t.Fatal(err)
	}
	anchor, err := reservationIdentityAnchorPath(root, token)
	if err != nil {
		t.Fatal(err)
	}
	rewriteReservationIdentityDeviceForTest(t, root, token)
	replaceLink := func(path string) {
		t.Helper()
		if err = os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	switch fault {
	case "missing-lock":
		if err = os.Remove(lock); err != nil {
			t.Fatal(err)
		}
	case "missing-anchor":
		if err = os.Remove(anchor); err != nil {
			t.Fatal(err)
		}
	case "replaced-lock":
		replaceLink(lock)
	case "replaced-anchor":
		replaceLink(anchor)
	case "missing-pair":
		if err = os.Remove(lock); err != nil {
			t.Fatal(err)
		}
		if err = os.Remove(anchor); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown fault %q", fault)
	}
}

// requireDriftClosedForTest pins the closed outcome: the first sentence stays,
// the cause names the missing proof without identity, and the ledger is
// untouched.
func requireDriftClosedForTest(t *testing.T, root, token string, before []byte, statusError error) {
	t.Helper()
	if statusError == nil {
		t.Fatal("incomplete drifted identity did not fail closed")
	}
	message := statusError.Error()
	if !strings.Contains(message, "reservation identity state is unverifiable") {
		t.Fatalf("closed message lost its first sentence: %q", message)
	}
	if !strings.Contains(message, "reservation identity link pair is incomplete") {
		t.Fatalf("closed message names no proof: %q", message)
	}
	if strings.Contains(message, token) || strings.Contains(message, root) {
		t.Fatalf("closed message exposed identity: %q", message)
	}
	after, err := os.ReadFile(reservationLedgerPath(root))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("incomplete drifted identity changed ledger bytes: read=%v", err)
	}
}

func TestReservationIdentityDeviceChangeIncompleteStaysClosed(t *testing.T) {
	for _, fault := range []string{"missing-lock", "missing-anchor", "replaced-lock", "replaced-anchor", "missing-pair"} {
		t.Run(fault, func(t *testing.T) {
			root := t.TempDir()
			owner := acquireDeviceOwnerForTest(t, root)
			defer func() {
				_ = abandonReservationIdentity(owner)
			}()
			applyDeviceFaultForTest(t, root, owner.Token, fault)
			before, err := os.ReadFile(reservationLedgerPath(root))
			if err != nil {
				t.Fatal(err)
			}
			_, statusError := ReservationStatus(context.Background(), root)
			requireDriftClosedForTest(t, root, owner.Token, before, statusError)
		})
	}
}

// fabricateReservationWaiterForTest records a waiter with intact identity files
// without parking it in a retry pause. A live waiter keeps the lock; a dead one
// releases it, as a killed guard would.
func fabricateReservationWaiterForTest(t *testing.T, root string, live bool) (string, *os.File) {
	t.Helper()
	value, err := token()
	if err != nil {
		t.Fatal(err)
	}
	identity, err := openReservationIdentity(root, value)
	if err != nil {
		t.Fatal(err)
	}
	device, inode, err := reservationIdentityMetadata(identity)
	if err != nil {
		t.Fatal(err)
	}
	if !live {
		if err = identity.Close(); err != nil {
			t.Fatal(err)
		}
		identity = nil
	}
	ledger, err := readReservationLedger(root)
	if err != nil {
		t.Fatal(err)
	}
	ledger.NextSequence++
	ledger.Waiters = append(ledger.Waiters, reservationWaiter{
		Token: value, PID: os.Getpid(), Class: policy.TaskService, Profile: "minimal",
		Requested: reservationDevicePlan().Requested, Sequence: ledger.NextSequence, MaxOwners: 1,
		IdentityDevice: device, IdentityInode: inode,
	})
	if err = writeReservationLedger(root, ledger); err != nil {
		t.Fatal(err)
	}

	return value, identity
}

func requireDriftedLiveWaiterKept(t *testing.T, root, value string, before, after []byte, totals ReservationTotals) {
	t.Helper()
	if totals.WaitingOwners != 1 || totals.ActiveOwners != 1 {
		t.Fatalf("drifted live waiter was not kept: %+v", totals)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("drifted live waiter changed ledger bytes")
	}
	if !identityPathsExist(root, value) {
		t.Fatal("drifted live waiter lost its identity files")
	}
}

func requireDriftedDeadWaiterRetired(t *testing.T, root, value, ownerToken string, totals ReservationTotals) {
	t.Helper()
	if totals.WaitingOwners != 0 || totals.ActiveOwners != 1 {
		t.Fatalf("drifted dead waiter was not retired: %+v", totals)
	}
	if identityPathsExist(root, value) {
		t.Fatal("drifted dead waiter kept its identity files")
	}
	if !identityPathsExist(root, ownerToken) {
		t.Fatal("drifted dead waiter took the live owner with it")
	}
}

func checkDriftedWaiterForTest(t *testing.T, live bool) {
	t.Helper()
	root := t.TempDir()
	owner := acquireDeviceOwnerForTest(t, root)
	defer func() {
		_ = ReleaseReservation(root, owner)
	}()
	value, identity := fabricateReservationWaiterForTest(t, root, live)
	if identity != nil {
		defer func() {
			_ = abandonReservationIdentity(&Session{identityLock: identity})
		}()
	}
	rewriteReservationIdentityDeviceForTest(t, root, value)
	before, err := os.ReadFile(reservationLedgerPath(root))
	if err != nil {
		t.Fatal(err)
	}
	totals, statusError := ReservationStatus(context.Background(), root)
	if statusError != nil {
		t.Fatalf("drifted waiter failed: %v", statusError)
	}
	after, err := os.ReadFile(reservationLedgerPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if live {
		requireDriftedLiveWaiterKept(t, root, value, before, after, totals)
	} else {
		requireDriftedDeadWaiterRetired(t, root, value, owner.Token, totals)
	}
}

func TestReservationIdentityDeviceChangeWaiter(t *testing.T) {
	t.Run("live", func(t *testing.T) {
		checkDriftedWaiterForTest(t, true)
	})
	t.Run("dead", func(t *testing.T) {
		checkDriftedWaiterForTest(t, false)
	})
}

func TestReservationIdentityDeviceChangeInheritance(t *testing.T) {
	root := t.TempDir()
	owner := acquireDeviceOwnerForTest(t, root)
	rewriteReservationIdentityDeviceForTest(t, root, owner.Token)
	live, err := AcquireReservation(
		context.Background(), root, owner.Token, policy.TaskEphemeral, "minimal", "child", reservationDevicePlan(), 1, time.Second,
	)
	if err != nil || live == nil || !live.Inherited || live.Token != owner.Token {
		t.Fatalf("drifted live owner refused inheritance: session=%+v error=%v", live, err)
	}
	// The guard died with its lock while both identity links stayed intact.
	if err = abandonReservationIdentity(owner); err != nil {
		t.Fatal(err)
	}
	// Inheritance is refused without failure: no error, and admission falls
	// through to a fresh owner instead of the dead token.
	dead, err := AcquireReservation(
		context.Background(), root, owner.Token, policy.TaskEphemeral, "minimal", "orphan", reservationDevicePlan(), 1, time.Second,
	)
	if err != nil || dead == nil || dead.Inherited || dead.Token == owner.Token {
		t.Fatalf("drifted dead owner was inherited or failed: session=%+v error=%v", dead, err)
	}
	if releaseError := ReleaseReservation(root, dead); releaseError != nil {
		t.Fatal(releaseError)
	}
}
