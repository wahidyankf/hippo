package guard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wahidyankf/hippo/internal/policy"
)

// ExclusiveStatus reads live schema-one compatibility ownership without
// pruning or otherwise mutating the shared coordination root.
func ExclusiveStatus(ctx context.Context, root string) (ReservationTotals, error) {
	totals := ReservationTotals{SchemaVersion: 5, Mode: coordinationModeExclusive}
	present, err := exclusiveMarkerPresent(root)
	if err != nil || !present {
		return totals, err
	}
	lock, err := acquireCoordinationLock(ctx, root, coordinationObservationWait)
	if err != nil {
		return ReservationTotals{}, err
	}
	defer releaseCoordinationLock(lock) //nolint:errcheck // Read-only status already has its result.

	present, err = exclusiveMarkerPresent(root)
	if err != nil || !present {
		return totals, err
	}

	owners, err := liveExclusiveSessionOwners(root)
	if err != nil {
		return ReservationTotals{}, err
	}
	seen := make(map[string]leaseOwner, len(owners))
	for _, owner := range owners {
		seen[owner.Token] = owner
		appendExclusiveOwner(&totals, owner.Token, owner)
	}
	heavy, live, err := liveExclusiveHeavyOwner(root)
	if err != nil {
		return ReservationTotals{}, err
	}
	if !live {
		return totals, nil
	}
	existing, duplicate := seen[heavy.Token]
	if duplicate && (existing.PID != heavy.PID || existing.Class != heavy.Class) {
		return ReservationTotals{}, compatibilityStateError("exclusive compatibility heavy and session ownership disagree")
	}
	if duplicate {
		return totals, nil
	}
	runID := heavy.Token
	if !sessionTokenPattern.MatchString(runID) {
		runID = "legacy-heavy"
	}
	appendExclusiveOwner(&totals, runID, *heavy)

	return totals, nil
}

func exclusiveMarkerPresent(root string) (bool, error) {
	marker, present, err := readCoordinationMarker(root)
	if err != nil || !present {
		return present, err
	}
	if marker.Mode != coordinationModeExclusive {
		return false, errors.New("coordination mode changed while reading exclusive status")
	}

	return true, nil
}

func liveExclusiveSessionOwners(root string) ([]leaseOwner, error) {
	owners := []leaseOwner{}
	entries, err := os.ReadDir(filepath.Join(root, "sessions"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, compatibilityStateError("exclusive compatibility session inventory cannot be enumerated")
	}
	for _, entry := range entries {
		recordToken := strings.TrimSuffix(entry.Name(), ".json")
		owner, alive, readError := liveExclusiveSessionOwner(root, recordToken)
		if readError != nil {
			return nil, readError
		}
		if alive {
			owners = append(owners, *owner)
		}
	}

	return owners, nil
}

func liveExclusiveSessionOwner(root, token string) (*leaseOwner, bool, error) {
	owner, err := readSessionRecord(root, token)
	if err != nil {
		return nil, false, compatibilityStateError("exclusive compatibility session record cannot be decoded")
	}
	if owner.SchemaVersion != 1 {
		if owner.SchemaVersion > 0 {
			return nil, false, coordinationProtocolMismatch(
				fmt.Sprintf("unsupported compatibility session schema %d", owner.SchemaVersion),
			)
		}

		return nil, false, compatibilityStateError("exclusive compatibility session schema is missing")
	}
	if !validSessionRecord(owner, token) {
		return nil, false, compatibilityStateError("exclusive compatibility session record is invalid")
	}
	alive, err := compatibilityOwnerAlive(root, owner)
	if err != nil {
		return nil, false, compatibilityStateError("exclusive compatibility session identity is unverifiable")
	}

	return owner, alive, nil
}

func liveExclusiveHeavyOwner(root string) (*leaseOwner, bool, error) {
	heavyPath := filepath.Join(root, "heavy.lock")
	if _, err := os.Stat(heavyPath); errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, compatibilityStateError("exclusive compatibility heavy ownership cannot be inspected")
	}
	owner, err := readLeaseOwner(heavyPath)
	if err != nil {
		return nil, false, compatibilityStateError("exclusive compatibility heavy owner cannot be decoded")
	}
	if owner.SchemaVersion != 1 {
		if owner.SchemaVersion > 0 {
			return nil, false, coordinationProtocolMismatch(
				fmt.Sprintf("unsupported compatibility heavy-owner schema %d", owner.SchemaVersion),
			)
		}

		return nil, false, compatibilityStateError("exclusive compatibility heavy owner schema is missing")
	}
	alive, err := compatibilityOwnerAlive(root, owner)
	if err != nil {
		return nil, false, compatibilityStateError("exclusive compatibility heavy owner identity is unverifiable")
	}

	return owner, alive, nil
}

func appendExclusiveOwner(totals *ReservationTotals, runID string, owner leaseOwner) {
	class := policy.TaskClass(owner.Class)
	totals.ActiveOwners++
	totals.LegacyEntries++
	switch class {
	case policy.TaskEphemeral:
		totals.Ephemeral++
	case policy.TaskService:
		totals.Service++
	case policy.TaskTransactional:
		totals.Transactional++
	case policy.TaskRelease:
		// Release workloads use the separate strict release guard.
	}
	totals.Owners = append(totals.Owners, ReservationEntry{
		RunID: runID, State: "active", Class: class, Legacy: true,
	})
}
