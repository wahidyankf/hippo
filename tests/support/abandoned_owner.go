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
// reservation whose owning guard is gone. Deleting the identity lock is what a
// killed guard does implicitly, because the kernel drops its flock, so the record
// is reached the same way a crash reaches it rather than by hand-writing a
// ledger.
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

	if err = guard.ReleaseReservation(root, session); err != nil {
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

	// A guard that is killed outright leaves its identity file behind with the
	// kernel's flock already dropped. That, not a missing file, is what a dead
	// owner looks like, so the fixture reproduces exactly that state.
	owner["processGroup"] = group
	delete(owner, "identityDevice")
	delete(owner, "identityInode")

	if err = writeSingleOwner(root, ledger, owner); err != nil {
		return err
	}

	identities := filepath.Join(root, "reservation-identities")
	if err = os.MkdirAll(identities, 0o700); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(identities, session.Token+".lock"), nil, 0o600)
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

	if driver.abandonedTotals.ActiveOwners != 0 {
		return fmt.Errorf("capacity was not reclaimed: activeOwners=%d", driver.abandonedTotals.ActiveOwners)
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
	if driver.abandonedPayload == nil {
		return
	}

	_ = syscall.Kill(-driver.abandonedGroup, syscall.SIGKILL)
	_, _ = driver.abandonedPayload.Process.Wait()
	driver.abandonedPayload = nil
}
