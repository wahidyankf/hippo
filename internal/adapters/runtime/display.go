package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/evidence"
	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/identity"
)

func displayRegistrationEffects(root string, commit func() error) application.DisplayRegistrationEffects {
	return application.DisplayRegistrationEffects{
		Publish: func(runID string, label identity.DisplayLabel) error {
			return evidence.WriteDisplay(root, runID, label)
		},
		Commit: commit,
		Remove: func(runID string) error { return evidence.RemoveDisplay(root, runID) },
	}
}

func displayRetirementEffects(root string, removeOwnership func() (application.DisplayRetirementResult, error)) application.DisplayRetirementEffects {
	return application.DisplayRetirementEffects{
		RemoveOwnership: removeOwnership,
		Remove:          func(runID string) error { return evidence.RemoveDisplay(root, runID) },
	}
}

func (state *admissionState) displayAdvisories() *application.DisplayAdvisories {
	if state.advisories == nil {
		state.advisories = &application.DisplayAdvisories{}
	}
	return state.advisories
}

func (state *admissionState) reportDisplayAdvisories() {
	if state.advisories != nil {
		state.advisories.Report(state.config.Stderr)
	}
}

func retireUnregisteredDisplay(
	port system.System, root, runID string, wait time.Duration, advisories *application.DisplayAdvisories,
) {
	present, err := displayRegularPath(port, filepath.Join(root, "run-display", runID+".json"))
	if err != nil || !present {
		return
	}
	lock, err := acquireCoordinationLock(context.Background(), root, wait)
	if err != nil {
		return
	}
	application.DiscardUnregisteredDisplay(
		application.DisplayRetirement{RunID: runID, Ownership: application.DisplayOwnershipOwned},
		func(id string) error { return evidence.RemoveDisplay(root, id) }, advisories,
	)
	if err = releaseCoordinationLock(lock); err != nil {
		return
	}
}

func displayRegularPath(port system.System, path string) (bool, error) {
	info, err := port.Files.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, errors.New("display census record is not a regular file")
	}
	return true, nil
}

func readDisplayCensusRecord(port system.System, path string, destination any) (bool, error) {
	present, err := displayRegularPath(port, path)
	if err != nil || !present {
		return present, err
	}
	data, err := port.Files.ReadFile(path)
	if err != nil {
		return false, err
	}
	unique := json.NewDecoder(bytes.NewReader(data))
	if err = consumeUniqueReservationJSON(unique); err != nil {
		return false, err
	}
	if _, err = unique.Token(); !errors.Is(err, io.EOF) {
		return false, errors.New("display census record contains multiple values")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(destination); err != nil {
		return false, err
	}
	return true, nil
}

func displayReservationCensus(port system.System, root string, live *[]application.DisplayLiveness) error {
	var ledger reservationLedger
	present, err := readDisplayCensusRecord(port, reservationLedgerPath(root), &ledger)
	if err != nil || !present {
		return err
	}
	if err = coordination.ValidateLedger(ledger); err != nil {
		return err
	}
	for _, owner := range ledger.Owners {
		alive, aliveError := reservationIdentityAlive(root, owner.Token, owner.IdentityDevice, owner.IdentityInode)
		if aliveError != nil {
			return aliveError
		}
		*live = append(*live, application.DisplayLiveness{RunID: owner.Token, Live: alive})
	}
	for _, waiter := range ledger.Waiters {
		alive, aliveError := reservationIdentityAlive(root, waiter.Token, waiter.IdentityDevice, waiter.IdentityInode)
		if aliveError != nil {
			return aliveError
		}
		*live = append(*live, application.DisplayLiveness{RunID: waiter.Token, Live: alive})
	}
	return nil
}

func displayExclusiveCensus(port system.System, root string, live *[]application.DisplayLiveness) error {
	entries, err := displayDirectoryEntries(port, filepath.Join(root, "sessions"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		runID, suffix := strings.CutSuffix(entry.Name(), ".json")
		if !suffix || !sessionTokenPattern.MatchString(runID) || !entry.Type().IsRegular() {
			return errors.New("display census session inventory is unverifiable")
		}
		var owner leaseOwner
		present, readError := readDisplayCensusRecord(port, filepath.Join(root, "sessions", entry.Name()), &owner)
		if readError != nil || !present || !validSessionRecord(&owner, runID) {
			return errors.New("display census session record is unverifiable")
		}
		alive, aliveError := compatibilityOwnerAlive(root, &owner)
		if aliveError != nil {
			return aliveError
		}
		*live = append(*live, application.DisplayLiveness{RunID: runID, Live: alive})
	}
	var heavy leaseOwner
	heavyEntries, err := displayDirectoryEntries(port, filepath.Join(root, "heavy.lock"))
	if err != nil {
		return err
	}
	for _, entry := range heavyEntries {
		if entry.Name() != "owner.json" || !entry.Type().IsRegular() {
			return errors.New("display census heavy inventory is unverifiable")
		}
	}
	present, err := readDisplayCensusRecord(port, filepath.Join(root, "heavy.lock", "owner.json"), &heavy)
	if err != nil {
		return err
	}
	if !present {
		if _, statError := port.Files.Lstat(filepath.Join(root, "heavy.lock")); !errors.Is(statError, os.ErrNotExist) {
			return errors.New("display census heavy ownership is unverifiable")
		}
		return nil
	}
	if !sessionTokenPattern.MatchString(heavy.Token) || !validSessionRecord(&heavy, heavy.Token) {
		return errors.New("display census heavy owner is unverifiable")
	}
	alive, err := compatibilityOwnerAlive(root, &heavy)
	if err != nil {
		return err
	}
	*live = append(*live, application.DisplayLiveness{RunID: heavy.Token, Live: alive})
	return nil
}

func displayDirectoryEntries(port system.System, directory string) ([]fs.DirEntry, error) {
	info, err := port.Files.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []fs.DirEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return nil, errors.New("display census inventory is not a directory")
	}
	return port.Files.ReadDir(directory)
}

func displayIdentityCensus(port system.System, root string, live *[]application.DisplayLiveness) error {
	entries, err := displayDirectoryEntries(port, filepath.Join(root, "reservation-identities"))
	if err != nil {
		return err
	}
	parts := make(map[string]int, len(entries)/2)
	for _, entry := range entries {
		extension := filepath.Ext(entry.Name())
		runID := strings.TrimSuffix(entry.Name(), extension)
		if !entry.Type().IsRegular() || !sessionTokenPattern.MatchString(runID) {
			return errors.New("display census identity inventory is unverifiable")
		}
		switch extension {
		case ".lock":
			parts[runID] |= 1
		case ".anchor":
			parts[runID] |= 2
		default:
			return errors.New("display census identity filename is unverifiable")
		}
	}
	for runID, pair := range parts {
		if pair != 3 {
			return errors.New("display census identity pair is incomplete")
		}
		alive, aliveError := reservationIdentityAlive(root, runID, 0, 0)
		if aliveError != nil {
			return aliveError
		}
		*live = append(*live, application.DisplayLiveness{RunID: runID, Live: alive})
	}
	return nil
}

func displayCensus(port system.System, root string) (application.DisplayCensus, error) {
	// The marker is validated even though every registry is inspected in all
	// modes. Unknown protocols prevent claiming that the census is complete.
	present, err := displayRegularPath(port, coordinationMarkerPath(root))
	if err != nil {
		return application.DisplayCensus{}, err
	}
	if present {
		if _, _, err = readCoordinationMarker(root); err != nil {
			return application.DisplayCensus{}, err
		}
	}
	var live []application.DisplayLiveness
	if err = displayReservationCensus(port, root, &live); err != nil {
		return application.DisplayCensus{}, err
	}
	if err = displayExclusiveCensus(port, root, &live); err != nil {
		return application.DisplayCensus{}, err
	}
	if err = displayIdentityCensus(port, root, &live); err != nil {
		return application.DisplayCensus{}, err
	}
	return application.DisplayCensus{Complete: true, Liveness: live}, nil
}

func housekeepDisplayLocked(port system.System, root string, now time.Time) {
	if err := application.CleanupDisplay(application.DisplayCleanupEffects{
		PruneTemporaries: func() error { return evidence.PruneDisplay(root, nil, now) },
		Census:           func() (application.DisplayCensus, error) { return displayCensus(port, root) },
		SupportedIDs:     func() ([]string, error) { return evidence.DisplayTokens(root) },
		Prune:            func(ids []string) error { return evidence.PruneDisplay(root, ids, now) },
	}); err != nil {
		// Cleanup is advisory and retried on the next invoked housekeeping.
		return
	}
}

func housekeepIdleDisplay(ctx context.Context, port system.System, root string) {
	info, err := port.Files.Lstat(filepath.Join(root, "run-display"))
	if err != nil || !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return
	}
	// Real publishers already created this lock. An ordinary absent root or
	// state with no lock is never initialized just to prune annotations.
	present, err := displayRegularPath(port, filepath.Join(root, "coordination.lock"))
	if err != nil || !present {
		return
	}
	lock, err := acquireCoordinationLock(ctx, root, coordinationObservationWait)
	if err != nil {
		return
	}
	housekeepDisplayLocked(port, root, time.Now())
	if err = releaseCoordinationLock(lock); err != nil {
		return
	}
}
