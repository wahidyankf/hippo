package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/internal/identity"

	configadapter "github.com/wahidyankf/hippo/internal/adapters/config"
	evidenceadapter "github.com/wahidyankf/hippo/internal/adapters/evidence"
	runtimeadapter "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/tests/support/runtimewiring"
)

type tuiNativeObservation struct {
	root, config  string
	configuration configadapter.Provider
	runtime       *runtimeadapter.Engine
	history       evidenceadapter.HistoryRepository
	display       evidenceadapter.DisplayRepository
	before        map[string]string
	watching      bool
	sessions      []*runtimeadapter.Session
	cancels       []context.CancelFunc
	joined        []<-chan error
	waiterTokens  []string
}

func (driver *Driver) prepareTUINativeObservation(ctx context.Context) error {
	state := driver.tuiObservation
	scratch, err := driver.temporaryRoot()
	if err != nil {
		return err
	}
	native := &tuiNativeObservation{root: filepath.Join(scratch, "observation-state"), config: filepath.Join(scratch, "observation.json"), runtime: runtimeadapter.NewEngine()}
	state.native = native
	driver.stops = append(driver.stops, native.cleanup)
	if err = os.Mkdir(native.root, 0o700); err != nil {
		return err
	}
	document := interruptionConfig
	if state.configurationMode == exclusiveMode {
		document = boundarySchemaOne
	}
	if err = os.WriteFile(native.config, []byte(document), 0o600); err != nil {
		return err
	}
	state.request.ConfigPath = native.config
	expected := state.totals
	oldAnnotations := state.annotations
	state.annotations = map[string]application.DisplayAnnotation{}
	if err = native.prepareOwners(ctx, state, expected, oldAnnotations); err != nil {
		return err
	}
	if err = native.prepareWaiters(ctx, expected); err != nil {
		return err
	}
	if err = native.prepareAnnotations(state, oldAnnotations); err != nil {
		return err
	}
	actual, err := native.runtime.Snapshot(ctx, native.root, state.configurationMode)
	if err != nil {
		return err
	}
	if err = native.requireOriginalFixture(actual, expected); err != nil {
		return err
	}
	state.totals = actual
	if len(expected.Owners) == 0 {
		native.before, err = tuiNativeTree(native.root)
	}
	return err
}

func (native *tuiNativeObservation) close() error {
	for _, cancel := range native.cancels {
		cancel()
	}
	native.cancels = nil
	var cleanupError error
	for _, joined := range native.joined {
		cleanupError = errors.Join(cleanupError, unexpectedTUIWaiterCleanupError(<-joined))
	}
	native.joined = nil
	var retained []*runtimeadapter.Session
	for _, session := range native.sessions {
		var releaseError error
		if session.Requested == (coordination.ReservationVector{}) {
			releaseError = runtimeadapter.ReleaseSession(native.root, session)
		} else {
			releaseError = runtimeadapter.ReleaseReservation(native.root, session)
		}
		if releaseError != nil {
			cleanupError = errors.Join(cleanupError, fmt.Errorf("release native TUI owner %s: %w", session.Token, releaseError))
			retained = append(retained, session)
		}
	}
	native.sessions = retained
	return cleanupError
}

// Cancellation is the expected waiter completion during teardown. A joined or
// wrapped cancellation must not hide an independent acquisition/release fault.
func unexpectedTUIWaiterCleanupError(err error) error {
	if err == nil {
		return nil
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var unexpected error
		for _, cause := range joined.Unwrap() {
			unexpected = errors.Join(unexpected, unexpectedTUIWaiterCleanupError(cause))
		}
		return unexpected
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		cause := wrapped.Unwrap()
		if cause != nil && unexpectedTUIWaiterCleanupError(cause) == nil {
			return nil
		}
		return err
	}
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func (native *tuiNativeObservation) writeAnnotationFault(kind, runID string) error {
	directory := filepath.Join(native.root, "run-display")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	path := filepath.Join(directory, runID+".json")
	switch kind {
	case annotationOperationMissing:
		return nil
	case tuiSymlinkAnnotation:
		return os.Symlink(filepath.Join(directory, "private-target"), path)
	case tuiUnreadableAnnotation:
		return os.Mkdir(path, 0o700)
	}
	record := map[string]any{schemaVersionField: 1, tuiRunIDField: runID, tuiLabelField: "unchecked-payload"}
	if kind == tuiMismatchedRunID {
		record[tuiRunIDField] = strings.Repeat("f", 32)
	}
	if kind == tuiUnsupportedSchema {
		record["schemaVersion"] = 99
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if kind == tuiMalformedAnnotation {
		data = []byte(`{"label":"unchecked-payload",`)
	}
	if kind == tuiOversizedAnnotation {
		data = append(data, []byte(strings.Repeat(" ", 513))...)
	}
	return os.WriteFile(path, data, 0o600)
}

func tuiNativeTree(root string) (map[string]string, error) {
	directory, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = directory.Close() }()
	result := map[string]string{}
	err = fs.WalkDir(directory.FS(), ".", func(path string, entry fs.DirEntry, walkError error) error {
		if walkError != nil {
			return walkError
		}
		if entry.IsDir() {
			result[path] = "directory"
			return nil
		}
		data, readError := directory.ReadFile(path)
		if readError != nil {
			return readError
		}
		result[path] = string(data)
		return nil
	})
	return result, err
}

func (driver *Driver) requireTUINativeReadOnly() error {
	if err := driver.requireTUIIdleSnapshot(); err != nil {
		return err
	}
	state := driver.tuiObservation
	if state.native == nil {
		return errors.New("native observation has no owned root")
	}
	after, err := tuiNativeTree(state.native.root)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(after, state.native.before) {
		return errors.New("observation mutated the actual idle coordination root")
	}
	return nil
}

// rekeyOwner constructs a shared-prefix identity fixture while retaining the
// identity inode and its acquired kernel lock. Only this private owned root is
// changed; the runtime reader remains the subject of every observation.
func (native *tuiNativeObservation) rekeyOwner(session *runtimeadapter.Session, runID string) error {
	old := session.Token
	ledgerPath := filepath.Join(native.root, "reservations.json")
	data, err := os.ReadFile(ledgerPath)
	if err != nil {
		return err
	}
	var ledger coordination.Ledger
	if err = json.Unmarshal(data, &ledger); err != nil {
		return err
	}
	for i := range ledger.Owners {
		if ledger.Owners[i].Token == old {
			ledger.Owners[i].Token = runID
		}
	}
	if err = coordination.ValidateLedger(ledger); err != nil {
		return err
	}
	for _, suffix := range []string{".lock", ".anchor"} {
		if err = os.Rename(filepath.Join(native.root, "reservation-identities", old+suffix), filepath.Join(native.root, "reservation-identities", runID+suffix)); err != nil {
			return err
		}
	}
	for _, directory := range []string{"owner-metadata", "run-display"} {
		path := filepath.Join(native.root, directory, old+".json")
		data, err = os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		data = []byte(strings.ReplaceAll(string(data), old, runID))
		ownedRoot, openError := os.OpenRoot(native.root)
		if openError != nil {
			return openError
		}
		writeError := ownedRoot.WriteFile(filepath.Join(directory, runID+".json"), data, 0o600)
		err = errors.Join(writeError, ownedRoot.Close())
		if err != nil {
			return err
		}
		if err = os.Remove(path); err != nil {
			return err
		}
	}
	data, err = json.Marshal(ledger)
	if err != nil {
		return err
	}
	if err = os.WriteFile(ledgerPath, data, 0o600); err != nil {
		return err
	}
	session.Token = runID
	return nil
}

func (native *tuiNativeObservation) writeLabel(runID string, label identity.DisplayLabel) error {
	return evidenceadapter.WriteDisplay(native.root, runID, label)
}

func (native *tuiNativeObservation) mutateSources() error {
	lock, err := os.OpenFile(filepath.Join(native.root, "coordination.lock"), os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	if err = native.rekeyOwner(native.sessions[0], strings.Repeat("e", 32)); err != nil {
		return err
	}
	path := filepath.Join(native.root, "owner-metadata", native.sessions[0].Token+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var record map[string]json.RawMessage
	if err = json.Unmarshal(data, &record); err != nil {
		return err
	}
	record["tags"] = json.RawMessage(`{"role":"changed"}`)
	data, err = json.Marshal(record)
	if err != nil {
		return err
	}
	if err = os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	path = filepath.Join(native.root, "reservations.json")
	data, err = os.ReadFile(path)
	if err != nil {
		return err
	}
	var ledger coordination.Ledger
	if err = json.Unmarshal(data, &ledger); err != nil {
		return err
	}
	if len(ledger.Waiters) != 3 {
		return errors.New("freeze fixture lost original FIFO waiters")
	}
	ledger.Waiters[0], ledger.Waiters[2] = ledger.Waiters[2], ledger.Waiters[0]
	ledger.Waiters[0].Sequence, ledger.Waiters[2].Sequence = ledger.Waiters[2].Sequence, ledger.Waiters[0].Sequence
	if err = coordination.ValidateLedger(ledger); err != nil {
		return err
	}
	data, err = json.Marshal(ledger)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (native *tuiNativeObservation) cleanup() {
	if err := native.close(); err != nil {
		panic(fmt.Errorf("clean up native TUI observation: %w", err))
	}
}

func (native *tuiNativeObservation) prepareOwners(ctx context.Context, state *tuiObservationState, expected coordination.ReservationTotals, oldAnnotations map[string]application.DisplayAnnotation) error {
	var err error
	for _, entry := range expected.Owners {
		var session *runtimeadapter.Session
		if expected.Mode == exclusiveMode {
			session, err = runtimewiring.AcquireSession(ctx, native.root, "", entry.Class, 0)
		} else {
			plan := coordination.ReservationPlan{Capacity: expected.Capacity, Requested: entry.Requested, Allocated: entry.Allocated}
			session, err = runtimewiring.AcquireReservationWithOptions(ctx, native.root, "", entry.Class, profileBalanced, "", plan, len(expected.Owners), 0, runtimeadapter.ReservationAdmissionOptions{Metadata: coordination.ReservationMetadata{Source: entry.Source, Tags: entry.Tags}, Now: func() time.Time { return state.now }})
		}
		if err != nil {
			return err
		}
		if session == nil {
			return errors.New("native observation failed to acquire owner")
		}
		native.sessions = append(native.sessions, session)
		if expected.Mode == reservationMode {
			if err = native.rekeyOwner(session, entry.RunID); err != nil {
				return err
			}
		}
		if entry.Legacy && expected.Mode == reservationMode {
			if err = os.Remove(filepath.Join(native.root, "owner-metadata", session.Token+".json")); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		if annotation, ok := oldAnnotations[entry.RunID]; ok {
			state.annotations[session.Token] = annotation
		}
	}
	return nil
}

func (native *tuiNativeObservation) prepareWaiters(ctx context.Context, expected coordination.ReservationTotals) error {
	var err error
	for _, entry := range expected.Waiters {
		owned, cancel := context.WithCancel(ctx)
		native.cancels = append(native.cancels, cancel)
		ready := make(chan coordination.ReservationWaitStatus, 1)
		done := make(chan error, 1)
		native.joined = append(native.joined, done)
		plan := coordination.ReservationPlan{Capacity: expected.Capacity, Requested: entry.Requested, Allocated: entry.Requested}
		go func() {
			session, acquireError := runtimewiring.AcquireReservationWithOptions(owned, native.root, "", entry.Class, profileBalanced, "", plan, len(expected.Owners), 30*time.Second, runtimeadapter.ReservationAdmissionOptions{Metadata: coordination.ReservationMetadata{Source: entry.Source, Tags: entry.Tags}, Heartbeat: func(value coordination.ReservationWaitStatus) {
				select {
				case ready <- value:
				default:
				}
			}})
			if session != nil {
				acquireError = errors.Join(acquireError, runtimeadapter.ReleaseReservation(native.root, session)) //nolint:contextcheck // Runtime release has no context parameter and must retire acquired ownership after cancellation.
			}
			done <- acquireError
		}()
		timer := time.NewTimer(5 * time.Second)
		select {
		case registered := <-ready:
			native.waiterTokens = append(native.waiterTokens, registered.RunID)
			timer.Stop()
		case err = <-done:
			timer.Stop()
			native.joined = native.joined[:len(native.joined)-1]
			return fmt.Errorf("FIFO waiter returned before registration: %w", err)
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			return errors.New("native FIFO waiter did not register")
		}
	}
	return nil
}

func (native *tuiNativeObservation) prepareAnnotations(state *tuiObservationState, oldAnnotations map[string]application.DisplayAnnotation) error {
	var err error
	for runID, annotation := range oldAnnotations {
		if _, ok := state.annotations[runID]; !ok {
			live := false
			for _, session := range native.sessions {
				live = live || session.Token == runID
			}
			if !live {
				state.annotations[runID] = annotation
			}
		}
	}
	for _, session := range native.sessions {
		if err = evidenceadapter.RemoveDisplay(native.root, session.Token); err != nil {
			return err
		}
	}
	for runID, annotation := range state.annotations {
		if annotation.Availability == application.DisplayAvailable {
			if err = evidenceadapter.WriteDisplay(native.root, runID, annotation.Label); err != nil {
				return err
			}
		}
	}
	if state.annotationState != "" {
		if err = native.writeAnnotationFault(state.annotationState, native.sessions[0].Token); err != nil {
			return err
		}
	}
	return nil
}

func (native *tuiNativeObservation) requireOriginalFixture(actual, expected coordination.ReservationTotals) error {
	if actual.ActiveOwners != len(expected.Owners) || actual.WaitingOwners != len(expected.Waiters) {
		return errors.New("native fixture changed original owner/waiter counts")
	}
	if expected.Mode != reservationMode {
		return nil
	}
	var allocated, waiting coordination.ReservationVector
	for index, entry := range expected.Owners {
		if err := coordination.CheckedAddVector(&allocated, entry.Allocated); err != nil {
			return err
		}
		row := actual.Owners[index]
		if !tuiNativeOwnerMatches(row, entry) {
			return fmt.Errorf("native owner %d changed the declared identity/metadata/vector fixture", index)
		}
	}
	for index, entry := range expected.Waiters {
		if err := coordination.CheckedAddVector(&waiting, entry.Requested); err != nil {
			return err
		}
		row := actual.Waiters[index]
		if !tuiNativeWaiterMatches(row, entry, native.waiterTokens[index], index+1) {
			return fmt.Errorf("native waiter %d changed its registered identity/FIFO/metadata/vector fixture", index)
		}
	}
	if actual.Capacity != expected.Capacity || actual.Allocated != allocated || actual.Waiting != waiting {
		return errors.New("native fixture changed original capacity/allocation/queued-demand vectors")
	}
	return nil
}

func tuiNativeOwnerMatches(row, expected coordination.ReservationEntry) bool {
	return row.RunID == expected.RunID && row.Class == expected.Class && row.Profile == profileBalanced && row.Source == expected.Source && reflect.DeepEqual(row.Tags, expected.Tags) && row.Requested == expected.Requested && row.Allocated == expected.Allocated && row.Legacy == expected.Legacy
}

func tuiNativeWaiterMatches(row, expected coordination.ReservationEntry, token string, position int) bool {
	return row.RunID == token && row.Position == position && row.Class == expected.Class && row.Profile == profileBalanced && row.Source == expected.Source && reflect.DeepEqual(row.Tags, expected.Tags) && row.Requested == expected.Requested
}
