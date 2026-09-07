package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/policy"
)

// abandonOwner builds the one state a guard cannot clean up after itself: a real
// reservation whose owning guard is gone while its payload keeps running.
//
// The shape here is dictated by the real process tree, which is guard ->
// internal lifetime launcher -> payload group. The launcher inherits the identity
// lock's open file description and outlives a guard that is killed outright, so
// the flock is still held afterwards and the reservation stays correctly counted
// against the work that is still running. A fixture that drops the lock instead
// describes a host where the payload died too, which is the other scenario. Only
// the guard's own process is gone, so that is the one thing this fixture ends.
func (driver *Driver) abandonOwner(withLivePayload bool) error {
	root, err := os.MkdirTemp("", "hippo-abandoned-owner-")
	if err != nil {
		return err
	}

	driver.temporaryPaths = append(driver.temporaryPaths, root)
	driver.leaseRoot = root

	// Take and give back a real reservation so the ledger, its capacity, and the
	// owner record are all shaped by the product rather than by this fixture.
	session, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, profileBalanced, "",
		v04Plan(1, 256*policy.MiB), 20, 0,
	)
	if err != nil {
		return err
	}

	// Capture the ledger before releasing: an emptied ledger is removed, so the
	// document has to be kept from while the reservation was genuinely held.
	ledger, err := readLedgerDocument(root)
	if err != nil {
		return err
	}

	owner, err := ownerRecord(ledger, session.Token)
	if err != nil {
		return err
	}

	// Hard-link the identity the product issued so release cannot destroy it. The
	// inode is what the ledger recorded, and a guard that dies never reaches
	// release at all, so restoring that exact object is what its death looks like.
	identities := filepath.Join(root, "reservation-identities")
	lockPath := filepath.Join(identities, session.Token+".lock")
	preserved := filepath.Join(root, "preserved-identity.lock")

	if err = os.Link(lockPath, preserved); err != nil {
		return err
	}

	if err = guard.ReleaseReservation(root, session); err != nil {
		return err
	}

	if err = os.MkdirAll(identities, 0o700); err != nil {
		return err
	}

	if err = os.Link(preserved, lockPath); err != nil {
		return err
	}

	if err = os.Remove(preserved); err != nil {
		return err
	}

	// Releasing the final session removes the coordination marker, so re-advertise
	// reservation mode afterwards. Without it status stops at the mode check and
	// never reaches the ledger this scenario is about.
	if err = os.WriteFile(
		filepath.Join(root, coordinationModeMarker),
		[]byte("{\"schemaVersion\":1,\"mode\":\"reservation\"}\n"),
		0o600,
	); err != nil {
		return err
	}

	payload := exec.Command(shellPath, "-c", "while :; do sleep 0.05; done")
	payload.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = payload.Start(); err != nil {
		return err
	}

	group, err := syscall.Getpgid(payload.Process.Pid)
	if err != nil {
		return err
	}

	driver.abandonedGroup = group
	driver.abandonedPayload = payload

	if !withLivePayload {
		_ = syscall.Kill(-group, syscall.SIGKILL)
		_, _ = payload.Process.Wait()
		driver.abandonedPayload = nil
		if err = awaitProcessGroupExit(group, 5*time.Second); err != nil {
			return err
		}
	}

	owner["processGroup"] = group

	deadGuard, err := exitedProcessID()
	if err != nil {
		return err
	}

	owner["pid"] = deadGuard

	if err = writeSingleOwner(root, ledger, owner); err != nil {
		return err
	}

	// A live payload means a live launcher, so the identity lock is still held.
	// A payload that died took its launcher with it, which frees the lock.
	if !withLivePayload {
		return nil
	}

	return driver.holdIdentityLock(lockPath)
}

// holdIdentityLock stands in for the lifetime launcher, which is what actually
// keeps the flock after the guard is gone.
func (driver *Driver) holdIdentityLock(path string) error {
	held, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		return err
	}

	if err = syscall.Flock(int(held.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = held.Close()

		return err
	}

	driver.abandonedIdentityLock = held

	return nil
}

// exitedProcessID returns the identifier of a process that has already been
// reaped, which is what the ledger holds once a guard is killed outright.
func exitedProcessID() (int, error) {
	finished := exec.Command(shellPath, "-c", shellExitZero)
	if err := finished.Run(); err != nil {
		return 0, err
	}

	return finished.Process.Pid, nil
}

// liveGuardOwner holds a real reservation whose guard is this process, so the
// recorded process group is running under a guard that is demonstrably alive.
// It is the control for the abandonment report: without it, an implementation
// that named every recorded group would still satisfy the other two scenarios.
func (driver *Driver) liveGuardOwner() error {
	root, err := os.MkdirTemp("", "hippo-live-guard-owner-")
	if err != nil {
		return err
	}

	driver.temporaryPaths = append(driver.temporaryPaths, root)
	driver.leaseRoot = root

	session, err := guard.AcquireReservation(
		context.Background(), root, "", policy.TaskEphemeral, profileBalanced, "",
		v04Plan(1, 256*policy.MiB), 20, 0,
	)
	if err != nil {
		return err
	}

	payload := exec.Command(shellPath, "-c", "while :; do sleep 0.05; done")
	payload.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = payload.Start(); err != nil {
		return err
	}

	group, err := syscall.Getpgid(payload.Process.Pid)
	if err != nil {
		return err
	}

	driver.abandonedGroup = group
	driver.abandonedPayload = payload

	// Record the process group the way activation does, leaving every other part
	// of the owner record exactly as the product wrote it.
	ledger, err := readLedgerDocument(root)
	if err != nil {
		return err
	}

	owner, err := ownerRecord(ledger, session.Token)
	if err != nil {
		return err
	}

	owner["processGroup"] = group

	return writeSingleOwner(root, ledger, owner)
}

// requireHeldOwnerNotNamed asserts a live guard's payload is never reported.
func (driver *Driver) requireHeldOwnerNotNamed() error {
	defer driver.stopAbandonedPayload()

	if driver.abandonedTotals.ActiveOwners != 1 {
		return fmt.Errorf(
			"a held reservation was not counted: activeOwners=%d",
			driver.abandonedTotals.ActiveOwners,
		)
	}

	if len(driver.abandonedTotals.AbandonedProcessGroups) != 0 {
		return fmt.Errorf(
			"a payload supervised by a live guard was reported as abandoned: %v",
			driver.abandonedTotals.AbandonedProcessGroups,
		)
	}

	return nil
}

// ownerRecord returns the live owner record the product just wrote.
func ownerRecord(ledger ledgerDocument, token string) (map[string]any, error) {
	for _, raw := range ledger.Owners {
		owner := map[string]any{}
		if err := json.Unmarshal(raw, &owner); err != nil {
			return nil, err
		}

		if owner["token"] == token {
			return owner, nil
		}
	}

	return nil, fmt.Errorf("owner %q was not recorded", token)
}

// writeSingleOwner restores the captured ledger carrying one abandoned record.
func writeSingleOwner(root string, ledger ledgerDocument, owner map[string]any) error {
	encoded, err := json.Marshal(owner)
	if err != nil {
		return err
	}

	ledger.Owners = []json.RawMessage{encoded}

	document, err := json.Marshal(ledger)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(root, "reservations.json"), document, 0o600)
}

type ledgerDocument struct {
	SchemaVersion int               `json:"schemaVersion"`
	Capacity      json.RawMessage   `json:"capacity"`
	NextSequence  uint64            `json:"nextSequence"`
	Owners        []json.RawMessage `json:"owners"`
	Waiters       []json.RawMessage `json:"waiters"`
}

func readLedgerDocument(root string) (ledgerDocument, error) {
	data, err := os.ReadFile(filepath.Join(root, "reservations.json"))
	if err != nil {
		return ledgerDocument{}, err
	}

	var ledger ledgerDocument
	if err = json.Unmarshal(data, &ledger); err != nil {
		return ledgerDocument{}, err
	}

	return ledger, nil
}

// awaitProcessGroupExit waits for a group to stop existing so the scenario never
// depends on how promptly the host reaps it.
func awaitProcessGroupExit(group int, wait time.Duration) error {
	deadline := time.Now().Add(wait)
	for {
		if errors.Is(syscall.Kill(-group, 0), syscall.ESRCH) {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("process group %d never exited", group)
		}

		time.Sleep(time.Millisecond)
	}
}

func (driver *Driver) readAbandonedOwnerStatus() error {
	totals, err := guard.ReservationStatus(context.Background(), driver.leaseRoot)
	driver.abandonedTotals = totals

	return err
}

func (driver *Driver) requireAbandonedGroupNamed() error {
	defer driver.stopAbandonedPayload()

	// The capacity stays held on purpose: the payload is still running and still
	// consuming the host, so releasing it here would oversubscribe the root.
	if driver.abandonedTotals.ActiveOwners != 1 {
		return fmt.Errorf(
			"capacity for still-running work was not held: activeOwners=%d",
			driver.abandonedTotals.ActiveOwners,
		)
	}
	if !slices.Contains(driver.abandonedTotals.AbandonedProcessGroups, driver.abandonedGroup) {
		return fmt.Errorf(
			"process group %d still running under a dead guard was not named: %v",
			driver.abandonedGroup,
			driver.abandonedTotals.AbandonedProcessGroups,
		)
	}

	return nil
}

func (driver *Driver) requireNoAbandonedGroupNamed() error {
	defer driver.stopAbandonedPayload()

	if driver.abandonedTotals.ActiveOwners != 0 {
		return fmt.Errorf("capacity was not reclaimed: activeOwners=%d", driver.abandonedTotals.ActiveOwners)
	}
	if len(driver.abandonedTotals.AbandonedProcessGroups) != 0 {
		return fmt.Errorf(
			"a payload that died with its guard was reported as abandoned: %v",
			driver.abandonedTotals.AbandonedProcessGroups,
		)
	}

	return nil
}

func (driver *Driver) stopAbandonedPayload() {
	if driver.abandonedIdentityLock != nil {
		_ = driver.abandonedIdentityLock.Close()
		driver.abandonedIdentityLock = nil
	}

	if driver.abandonedPayload == nil {
		return
	}

	_ = syscall.Kill(-driver.abandonedGroup, syscall.SIGKILL)
	_, _ = driver.abandonedPayload.Process.Wait()
	driver.abandonedPayload = nil
}
