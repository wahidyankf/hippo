package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"time"

	runtimeadapter "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/tests/contract"
	"github.com/wahidyankf/hippo/tests/support/runtimewiring"
)

// PublicTUILedger owns native peer setup only. Snapshot below validates setup;
// it never supplies an observation, display row or expected product result.
// Deterministic capacity/vector staging retains actual held identity locks and
// their same-inode anchors under the live test process, not PID-only records.
type PublicTUILedger struct {
	root, directory, configuration string
	owners                         []*runtimeadapter.Session
	waiters                        []*publicTUILedgerWaiter
	expected                       coordination.ReservationTotals
	prepared                       bool
	variant                        string
	annotationFault                string
	labels                         map[string]string
	foreignOwners                  map[string]int
	largeIdentities                []*os.File
	closeMu                        sync.Mutex
}

type publicTUILedgerWaiter struct {
	cancel    context.CancelFunc
	done      chan error
	joined    bool
	joinError error
}

// NewPublicTUILedger is passive: it rejects foreign boundaries and roots before
// any resource acquisition. The Driver retains the one eventual cleanup owner.
func (driver *Driver) NewPublicTUILedger(ctx context.Context, root, directory string) (*PublicTUILedger, error) {
	if driver == nil || driver.mode != contract.E2E || !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || !slices.Contains(driver.temporaryPaths, directory) || root != filepath.Join(directory, "public-state") {
		return nil, errors.New("public TUI ledger requires this E2E driver's owned root")
	}
	fixture := &PublicTUILedger{root: root, directory: directory, configuration: filepath.Join(directory, "observation.json")}
	driver.stops = append(driver.stops, func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if err := fixture.Close(cleanup); err != nil {
			panic(fmt.Errorf("owned public TUI ledger cleanup: %w", err))
		}
	})
	return fixture, nil
}

// Configuration returns the owned observation configuration path.
func (fixture *PublicTUILedger) Configuration() string { return fixture.configuration }

// Expected returns independently owned fixture inputs, never a product snapshot.
func (fixture *PublicTUILedger) Expected() coordination.ReservationTotals {
	expected := fixture.expected
	expected.Owners = slices.Clone(expected.Owners)
	expected.Waiters = slices.Clone(expected.Waiters)
	for index := range expected.Owners {
		expected.Owners[index].Tags = maps.Clone(expected.Owners[index].Tags)
	}
	for index := range expected.Waiters {
		expected.Waiters[index].Tags = maps.Clone(expected.Waiters[index].Tags)
	}
	return expected
}

// PrepareKnownRows creates the original two Running/three FIFO fixture. Runtime
// admission is used only to construct authentic peer state, as permitted by the
// public-boundary architecture; the selected public binary remains the reader.
func (fixture *PublicTUILedger) PrepareKnownRows(ctx context.Context) error {
	return fixture.PrepareObservation(ctx, "known", "")
}

func (fixture *PublicTUILedger) prepareWaiter(parent context.Context, plan coordination.ReservationPlan, position int) error {
	owned, cancel := context.WithCancel(parent)
	waiter := &publicTUILedgerWaiter{cancel: cancel, done: make(chan error, 1)}
	fixture.waiters = append(fixture.waiters, waiter)
	metadata := coordination.ReservationMetadata{Source: tuiOtherRole, Tags: map[string]string{tuiRoleTag: tuiOtherRole}}
	if fixture.variant == "filtered" && position == 3 {
		metadata.Source = "matching"
		metadata.Tags = map[string]string{tuiRoleTag: "matching"}
	}
	ready := make(chan coordination.ReservationWaitStatus, 1)
	go func() {
		session, acquireError := runtimewiring.AcquireReservationWithOptions(owned, fixture.root, "", policy.TaskEphemeral, profileBalanced, "", plan, 2, 30*time.Second, runtimeadapter.ReservationAdmissionOptions{Metadata: metadata, Pause: func(waitCtx context.Context, _ time.Duration) error {
			// Keep the genuinely queued peer live without repeated ledger rewrites.
			// Admission releases coordination before calling this owned pause.
			<-waitCtx.Done()
			return waitCtx.Err()
		}, Heartbeat: func(status coordination.ReservationWaitStatus) {
			select {
			case ready <- status:
			default:
			}
		}})
		if session != nil {
			acquireError = errors.Join(acquireError, runtimeadapter.ReleaseReservationContext(owned, fixture.root, session))
		}
		waiter.done <- acquireError
	}()
	bound, stop := context.WithTimeout(parent, 3*time.Second)
	defer stop()
	select {
	case heartbeat := <-ready:
		if heartbeat.RunID == "" || heartbeat.Position != position {
			return fmt.Errorf("public TUI peer FIFO heartbeat=%+v, want position %d", heartbeat, position)
		}
		fixture.expected.Waiters = append(fixture.expected.Waiters, coordination.ReservationEntry{RunID: heartbeat.RunID, State: "waiting", Position: position, Class: policy.TaskEphemeral, Profile: profileBalanced, Source: metadata.Source, Tags: maps.Clone(metadata.Tags), Requested: plan.Requested})
		return nil
	case outcome := <-waiter.done:
		waiter.joined, waiter.joinError = true, outcome
		return fmt.Errorf("public TUI peer waiter ended before FIFO registration: %w", outcome)
	case <-bound.Done():
		return bound.Err()
	}
}

// VerifyLiveness is setup validation, separate from public status/frame checks.
// Snapshot must authenticate the real locks and anchors; PID-shaped JSON alone
// is insufficient. The persisted ledger's device/inode fields are retained too.
func (fixture *PublicTUILedger) VerifyLiveness(ctx context.Context) error {
	actual, err := runtimeadapter.NewEngine().Snapshot(ctx, fixture.root, fixture.expected.Mode)
	if err != nil {
		return err
	}
	if err := validatePublicTUILedgerRows(actual, fixture.expected); err != nil {
		return fmt.Errorf("public TUI ledger setup: %w", err)
	}
	if fixture.expected.Mode == exclusiveMode || fixture.expected.ActiveOwners == 0 {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(fixture.root, "reservations.json"))
	if err != nil {
		return err
	}
	var ledger coordination.Ledger
	if err := json.Unmarshal(data, &ledger); err != nil {
		return err
	}
	if err := coordination.ValidateLedger(ledger); err != nil {
		return err
	}
	for _, owner := range ledger.Owners {
		if owner.PID != fixture.ownerPID(owner.Token) || owner.IdentityDevice == 0 || owner.IdentityInode == 0 {
			return errors.New("public TUI peer owner lacks live owning-process/kernel identity")
		}
		if err := fixture.requireAnchor(owner.Token); err != nil {
			return err
		}
	}
	for _, waiter := range ledger.Waiters {
		if waiter.PID != os.Getpid() || waiter.IdentityDevice == 0 || waiter.IdentityInode == 0 {
			return errors.New("public TUI peer waiter lacks live owning-process/kernel identity")
		}
		if err := fixture.requireAnchor(waiter.Token); err != nil {
			return err
		}
	}
	return nil
}

func (fixture *PublicTUILedger) requireAnchor(token string) error {
	lock, err := os.Lstat(filepath.Join(fixture.root, "reservation-identities", token+".lock"))
	if err != nil {
		return err
	}
	anchor, err := os.Lstat(filepath.Join(fixture.root, "reservation-identities", token+".anchor"))
	if err != nil {
		return err
	}
	if !lock.Mode().IsRegular() || !anchor.Mode().IsRegular() || !os.SameFile(lock, anchor) {
		return errors.New("public TUI peer identity anchor is not the held regular inode")
	}
	return nil
}

func validatePublicTUILedgerRows(actual, expected coordination.ReservationTotals) error {
	if actual.SchemaVersion != 5 || actual.Mode != expected.Mode || actual.LegacyEntries != expected.LegacyEntries || actual.Ephemeral != expected.Ephemeral || actual.Service != expected.Service || actual.Transactional != expected.Transactional || actual.ActiveOwners != expected.ActiveOwners || actual.WaitingOwners != expected.WaitingOwners || actual.Capacity != expected.Capacity || actual.Allocated != expected.Allocated || actual.Waiting != expected.Waiting || len(actual.Owners) != len(expected.Owners) || len(actual.Waiters) != len(expected.Waiters) {
		return errors.New("public TUI ledger changed global counts or reservation vectors")
	}
	if err := validatePublicTUIOwnerRows(actual.Owners, expected.Owners); err != nil {
		return err
	}
	return validatePublicTUIWaiterRows(actual.Waiters, expected.Waiters)
}

// Close cancels and joins every queued actor before releasing any Running owner,
// so teardown cannot accidentally admit a queued peer. All waits share the
// caller's existing three-second cleanup budget; no new fallback deadline exists.
func (fixture *PublicTUILedger) Close(ctx context.Context) error {
	fixture.closeMu.Lock()
	defer fixture.closeMu.Unlock()
	for _, waiter := range fixture.waiters {
		waiter.cancel()
	}
	var result error
	allJoined := true
	for index, waiter := range fixture.waiters {
		if !waiter.joined {
			select {
			case outcome := <-waiter.done:
				waiter.joined, waiter.joinError = true, outcome
			case <-ctx.Done():
				result = errors.Join(result, fmt.Errorf("join public TUI peer waiter %d: %w", index, ctx.Err()))
				allJoined = false
			}
		}
		if waiter.joined {
			result = errors.Join(result, unexpectedTUIWaiterCleanupError(waiter.joinError))
		}
	}
	if !allJoined {
		return result
	}
	if len(fixture.largeIdentities) > 0 {
		result = errors.Join(result, fixture.closeLargeLedger())
		if result != nil {
			return result
		}
	}
	if err := fixture.removeOwnedAnnotationFault(); err != nil {
		return errors.Join(result, err)
	}
	var retained []*runtimeadapter.Session
	for _, owner := range fixture.owners {
		var releaseError error
		if fixture.expected.Mode == exclusiveMode {
			releaseError = runtimeadapter.ReleaseSessionContext(ctx, fixture.root, owner)
		} else {
			releaseError = runtimeadapter.ReleaseReservationContext(ctx, fixture.root, owner)
		}
		if releaseError != nil {
			result = errors.Join(result, fmt.Errorf("release public TUI peer owner %s: %w", owner.Token, releaseError))
			retained = append(retained, owner)
		}
	}
	fixture.owners = retained
	return result
}

// QueuedActorsJoined reports actual observed waiter joins after Close, including
// its failure paths. A Close error alone does not settle this ownership fact.
func (fixture *PublicTUILedger) QueuedActorsJoined() bool {
	fixture.closeMu.Lock()
	defer fixture.closeMu.Unlock()
	for _, waiter := range fixture.waiters {
		if !waiter.joined {
			return false
		}
	}
	return true
}

// RequirePublicTotals compares actual selected-binary status against immutable fixture inputs.
func (fixture *PublicTUILedger) RequirePublicTotals(actual coordination.ReservationTotals) error {
	return validatePublicTUILedgerRows(actual, fixture.expected)
}

func validatePublicTUIOwnerRows(actual, expected []coordination.ReservationEntry) error {
	for index, want := range expected {
		row := actual[index]
		if row.RunID != want.RunID || row.State != want.State || row.Class != want.Class || row.Profile != want.Profile || row.Source != want.Source || !reflect.DeepEqual(row.Tags, want.Tags) || row.Requested != want.Requested || row.Allocated != want.Allocated || row.Legacy != want.Legacy {
			return fmt.Errorf("public TUI ledger Running row %d changed identity/metadata/vector", index)
		}
	}
	return nil
}

func validatePublicTUIWaiterRows(actual, expected []coordination.ReservationEntry) error {
	for index, want := range expected {
		row := actual[index]
		if row.RunID != want.RunID || row.State != want.State || row.Position != index+1 || row.Class != want.Class || row.Profile != want.Profile || row.Source != want.Source || !reflect.DeepEqual(row.Tags, want.Tags) || row.Requested != want.Requested {
			return fmt.Errorf("public TUI ledger Queue row %d changed identity/FIFO/metadata/vector", index)
		}
	}
	return nil
}
