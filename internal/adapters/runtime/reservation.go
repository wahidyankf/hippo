package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
	coordination "github.com/wahidyankf/hippo/internal/domain/coordination"

	"github.com/wahidyankf/hippo/internal/adapters/evidence"
	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/policy"
	"golang.org/x/sys/unix"
)

// ReservationAdmissionOptions supplies schema-3 metadata and deterministic wait seams.
type ReservationAdmissionOptions struct {
	Metadata  ReservationMetadata
	Now       func() time.Time
	Pause     func(context.Context, time.Duration) error
	Heartbeat func(ReservationWaitStatus)
	// CleanupWait bounds the coordination-lock wait of the cleanup that removes a queued waiter when its acquisition
	// returns. Zero means coordinationLifecycleWait; only deterministic fixtures set it.
	CleanupWait time.Duration
}

// abandonedProcessGroups reports process groups whose owning guard is gone while
// the group itself is still running.
//
// Liveness for accounting is the identity flock, and that is deliberately not the
// test here. The lifetime launcher inherits the lock's open file description and
// outlives a guard that is killed outright, so the flock stays held for as long
// as the payload runs. That is the correct answer for capacity, because the work
// really is still consuming the host and releasing its reservation would
// oversubscribe the root. It is the wrong answer for this report: judged by the
// lock, a guard that died never looks dead, and nothing is ever named.
//
// So this asks after the guard's own process instead. Nothing reaps the payload
// once its guard is gone, and nothing releases the reservation either, so a
// shared root fills with owners no one will ever clean up until every later
// admission defers forever. An operator has no other way to see that.
//
// A recycled guard identifier reads as alive and is passed over, which loses a
// report rather than inventing one.
//
// This reports and never signals. The record holds a bare process group with no
// start-time identity, so a group the kernel has since recycled would name an
// unrelated process, and killing that is far worse than leaking the original. The
// caller can confirm the group and decide; HIPPO will not guess.
func abandonedProcessGroups(port system.System, ledger reservationLedger) []int {
	groups := make([]int, 0, len(ledger.Owners))
	for _, owner := range ledger.Owners {
		if owner.ProcessGroup <= 0 {
			continue
		}

		if owner.PID > 0 && processExists(port, owner.PID) {
			continue
		}

		if processGroupExists(port, owner.ProcessGroup) {
			groups = append(groups, owner.ProcessGroup)
		}
	}

	slices.Sort(groups)

	return slices.Compact(groups)
}

// processExists reports whether a process identifier still names a live process.
// EPERM means it exists under another user, which is still alive.
func processExists(port system.System, pid int) bool {
	err := port.Processes.Kill(pid, 0)

	return err == nil || errors.Is(err, syscall.EPERM)
}

// processGroupExists reports whether any process remains in a process group.
func processGroupExists(port system.System, group int) bool {
	err := port.Processes.Kill(-group, 0)

	return err == nil || errors.Is(err, syscall.EPERM)
}

func reservationLedgerPath(root string) string {
	return filepath.Join(root, "reservations.json")
}

func reservationIdentityPath(root, value string) (string, error) {
	if !sessionTokenPattern.MatchString(value) {
		return "", errors.New("invalid reservation identity token")
	}

	return filepath.Join(root, "reservation-identities", value+".lock"), nil
}

func reservationIdentityAnchorPath(root, value string) (string, error) {
	path, err := reservationIdentityPath(root, value)
	if err != nil {
		return "", err
	}

	return strings.TrimSuffix(path, ".lock") + ".anchor", nil
}

func openReservationIdentity(port system.System, root, value string) (*os.File, error) {
	path, err := reservationIdentityPath(root, value)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}

	identity, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err = port.Files.Flock(identity, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = identity.Close()
		_ = os.Remove(path)

		return nil, err
	}
	anchor, err := reservationIdentityAnchorPath(root, value)
	if err != nil || os.Link(path, anchor) != nil {
		_ = identity.Close()
		_ = os.Remove(path)
		_ = os.Remove(anchor)

		return nil, errors.New("reservation identity anchor could not be created")
	}
	return identity, nil
}

func reservationIdentityUnverifiable(cause string) error {
	return errors.New("reservation identity state is unverifiable: " + cause)
}

// observeReservationIdentity stats one identity path for the coordination-domain
// judgement. A missing path is absent evidence, not an error; anything the
// adapter cannot read stays closed without exposing what it saw.
func observeReservationIdentity(candidate string) (coordination.ReservationIdentityEvidence, *os.File, error) {
	opened, openError := os.OpenFile(candidate, os.O_RDWR, 0o600)
	if errors.Is(openError, os.ErrNotExist) {
		return coordination.ReservationIdentityEvidence{}, nil, nil
	}
	if openError != nil {
		return coordination.ReservationIdentityEvidence{}, nil,
			reservationIdentityUnverifiable("reservation identity evidence cannot be read")
	}
	var info unix.Stat_t
	// Fstat fails only on an invalid descriptor or address. The descriptor was
	// just opened successfully above and the buffer is kernel-provided, so
	// neither failure is possible here; the error is discarded, not exempted.
	_ = unix.Fstat(int(opened.Fd()), &info)
	// Fstat above reports kernel-assigned device and inode numbers, which are
	// non-negative on every supported platform. Their Sprint forms are digit
	// strings, which ParseUint accepts, so both errors below are discarded,
	// not exempted.
	device, _ := filesystemIdentifier(info.Dev)
	inode, _ := filesystemIdentifier(info.Ino)

	return coordination.ReservationIdentityEvidence{Present: true, Device: device, Inode: inode}, opened, nil
}

func closeReservationIdentity(identity *os.File) {
	if identity != nil {
		_ = identity.Close()
	}
}

func reservationIdentityAlive(
	port system.System, root, value string, expectedDevice, expectedInode uint64,
) (bool, error) {
	path, err := reservationIdentityPath(root, value)
	if err != nil {
		return false, err
	}
	anchor, _ := reservationIdentityAnchorPath(root, value)
	lockEvidence, lockFile, err := observeReservationIdentity(path)
	if err != nil {
		return false, err
	}
	anchorEvidence, anchorFile, err := observeReservationIdentity(anchor)
	if err != nil {
		closeReservationIdentity(lockFile)

		return false, err
	}
	judgement := coordination.JudgeReservationIdentity(expectedDevice, expectedInode, lockEvidence, anchorEvidence)
	if judgement == coordination.ReservationIdentityAbsent {
		return false, nil
	}
	if judgement == coordination.ReservationIdentityIncomplete {
		closeReservationIdentity(lockFile)
		closeReservationIdentity(anchorFile)

		return false, reservationIdentityUnverifiable(coordination.IncompleteReservationIdentityCause)
	}
	// The probe keeps the first evidence that exactly matches the ledger, as
	// before; a drifted or expectation-free pair names one file either way.
	probe, other := lockFile, anchorFile
	if probe == nil ||
		(lockEvidence.Present && anchorEvidence.Present &&
			(lockEvidence.Device != expectedDevice || lockEvidence.Inode != expectedInode) &&
			anchorEvidence.Device == expectedDevice && anchorEvidence.Inode == expectedInode) {
		probe, other = anchorFile, lockFile
	}
	closeReservationIdentity(other)
	// A nil probe means both observations missed their files, which the
	// judgement above already reports as absent; reaching here with no file
	// and no error is impossible, so the guard is removed, not exempted.
	defer closeReservationIdentity(probe)
	err = port.Files.Flock(probe, unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return true, nil
	}
	if err != nil {
		return false, reservationIdentityUnverifiable("reservation identity lock cannot be proved")
	}
	unlockError := port.Files.Flock(probe, unix.LOCK_UN)

	return false, unlockError
}

func reservationIdentityMetadata(identity *os.File) (uint64, uint64, error) {
	if identity == nil {
		return 0, 0, errors.New("reservation identity is missing")
	}
	var info unix.Stat_t
	if err := unix.Fstat(int(identity.Fd()), &info); err != nil {
		return 0, 0, err
	}

	// Fstat above reports kernel-assigned device and inode numbers, which are
	// non-negative on every supported platform. Their Sprint forms are digit
	// strings, which ParseUint accepts, so both errors below are discarded,
	// not exempted.
	device, _ := filesystemIdentifier(info.Dev)
	inode, _ := filesystemIdentifier(info.Ino)

	return device, inode, nil
}

func ensureReservationIdentityLocked(port system.System, identity *os.File) error {
	if identity == nil {
		return errors.New("reservation identity is missing")
	}
	if err := port.Files.Flock(identity, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.New("reservation identity ownership could not be retained")
	}

	return nil
}

func consumeUniqueReservationJSON(decoder *json.Decoder) error {
	tokenValue, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, compound := tokenValue.(json.Delim)
	if !compound {
		return nil
	}

	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			keyValue, keyError := decoder.Token()
			if keyError != nil {
				return keyError
			}
			// A non-string object key fails the Token call above (encoding/json
			// ErrNonStringName), so the assertion below cannot fail.
			key := keyValue.(string) //nolint:forcetypeassert // Token guarantees a string key when it returns nil.

			if seen[key] {
				return fmt.Errorf("duplicate reservation ledger field %q", key)
			}
			seen[key] = true
			if valueError := consumeUniqueReservationJSON(decoder); valueError != nil {
				return valueError
			}
		}
	case '[':
		for decoder.More() {
			if itemError := consumeUniqueReservationJSON(decoder); itemError != nil {
				return itemError
			}
		}
	}
	_, err = decoder.Token()

	return err
}

// coordination.ValidReservationClass reports whether a ledger may hold the class: ephemeral,
// service, and transactional work reserves, release work is guarded by the
// release commands and never does, and a class with no member, the empty one a
// ledger owner or waiter without a class decodes to included, is not one a
// ledger holds. The switch names every member so that a class added later must
// be decided here, and one nobody decided is refused.
func initializeMissingReservationLedger(port system.System, root string) (reservationLedger, error) {
	directory := filepath.Join(root, "reservation-identities")
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return reservationLedger{SchemaVersion: reservationLedgerSchemaVersion}, nil
	}
	if err != nil {
		return reservationLedger{}, errors.New("reservation identity state is unverifiable")
	}
	const (
		identityLockEntry = 1 << iota
		identityAnchorEntry
		completeIdentityEntries = identityLockEntry | identityAnchorEntry
	)
	identities := make(map[string]int, len(entries)/2)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			return reservationLedger{}, errors.New("reservation accounting is missing with unverifiable identity state")
		}
		extension := filepath.Ext(name)
		value := strings.TrimSuffix(name, extension)
		if !sessionTokenPattern.MatchString(value) {
			return reservationLedger{}, errors.New("reservation accounting is missing with unverifiable identity state")
		}
		switch extension {
		case ".lock":
			identities[value] |= identityLockEntry
		case ".anchor":
			identities[value] |= identityAnchorEntry
		default:
			return reservationLedger{}, errors.New("reservation accounting is missing with unverifiable identity state")
		}
	}
	tokens := make([]string, 0, len(identities))
	for value, parts := range identities {
		if parts != completeIdentityEntries {
			return reservationLedger{}, errors.New("reservation accounting is missing with unverifiable identity state")
		}
		tokens = append(tokens, value)
	}
	slices.Sort(tokens)
	for _, value := range tokens {
		alive, aliveError := reservationIdentityAlive(port, root, value, 0, 0)
		if aliveError != nil {
			return reservationLedger{}, aliveError
		}
		if alive {
			return reservationLedger{}, errors.New("reservation accounting is missing while an owner identity remains live")
		}
		if removeError := removeReservationIdentity(root, value); removeError != nil {
			return reservationLedger{}, errors.New("reservation stale identity could not be reconciled")
		}
	}

	return reservationLedger{SchemaVersion: reservationLedgerSchemaVersion}, nil
}

func readReservationLedger(port system.System, root string) (reservationLedger, error) {
	data, err := os.ReadFile(reservationLedgerPath(root))
	if errors.Is(err, os.ErrNotExist) {
		return initializeMissingReservationLedger(port, root)
	}
	if err != nil {
		return reservationLedger{}, err
	}

	uniqueDecoder := json.NewDecoder(bytes.NewReader(data))
	if err = consumeUniqueReservationJSON(uniqueDecoder); err != nil {
		return reservationLedger{}, fmt.Errorf("decode reservation ledger: %w", err)
	}
	if _, err = uniqueDecoder.Token(); !errors.Is(err, io.EOF) {
		return reservationLedger{}, errors.New("reservation ledger must contain one JSON value")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var ledger reservationLedger
	if err = decoder.Decode(&ledger); err != nil {
		return reservationLedger{}, fmt.Errorf("decode reservation ledger: %w", err)
	}
	if err = coordination.ValidateLedger(ledger); err != nil {
		return reservationLedger{}, err
	}

	return ledger, nil
}

func writeReservationLedger(root string, ledger reservationLedger) (returnError error) {
	temporary, err := os.CreateTemp(root, ".reservations-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		returnError = errors.Join(returnError, temporary.Close())
		if removeError := os.Remove(temporaryPath); !errors.Is(removeError, os.ErrNotExist) {
			returnError = errors.Join(returnError, removeError)
		}
	}()

	if err = temporary.Chmod(0o600); err != nil {
		return err
	}
	if err = json.NewEncoder(temporary).Encode(ledger); err != nil {
		return err
	}
	if err = temporary.Sync(); err != nil {
		return err
	}

	return os.Rename(temporaryPath, reservationLedgerPath(root))
}

func removeReservationIdentity(root, value string) error {
	path, err := reservationIdentityPath(root, value)
	if err != nil {
		return err
	}
	anchor, _ := reservationIdentityAnchorPath(root, value)
	var returnError error
	for _, candidate := range []string{path, anchor} {
		if removeError := os.Remove(candidate); removeError != nil && !errors.Is(removeError, os.ErrNotExist) {
			returnError = errors.Join(returnError, removeError)
		}
	}
	returnError = errors.Join(returnError, removeReservationMetadata(system.Production, root, value))

	return returnError
}

func reconcileReservationLedger(port system.System, root string, ledger *reservationLedger) error {
	staleTokens := make([]string, 0)
	owners := ledger.Owners[:0]
	for _, owner := range ledger.Owners {
		alive, err := reservationIdentityAlive(port, root, owner.Token, owner.IdentityDevice, owner.IdentityInode)
		if err != nil {
			return err
		}
		if alive {
			owners = append(owners, owner)

			continue
		}
		staleTokens = append(staleTokens, owner.Token)
	}
	ledger.Owners = owners
	waiters := ledger.Waiters[:0]
	for _, waiter := range ledger.Waiters {
		alive, err := reservationIdentityAlive(port, root, waiter.Token, waiter.IdentityDevice, waiter.IdentityInode)
		if err != nil {
			return err
		}
		if alive {
			waiters = append(waiters, waiter)

			continue
		}
		staleTokens = append(staleTokens, waiter.Token)
	}
	ledger.Waiters = waiters
	if len(staleTokens) == 0 {
		return nil
	}
	// Commit accounting retirement before removing the corresponding identity
	// paths. A failed ledger write must leave the old identity evidence intact;
	// otherwise a later reader would see a live record pointing at missing state.
	if err := writeReservationLedger(root, *ledger); err != nil {
		return errors.New("reservation stale accounting could not be reconciled")
	}
	for _, value := range staleTokens {
		if err := removeReservationIdentity(root, value); err != nil {
			return errors.New("reservation stale identity could not be reconciled")
		}
	}

	return nil
}

func ensureReservationCoordination(port system.System, root string) error {
	marker, present, err := readCoordinationMarker(root)
	if err != nil {
		return err
	}
	if present && marker.Mode == coordinationModeReservation {
		return nil
	}
	if err = pruneSessionRecords(port, root); err != nil {
		return err
	}
	heavyPath := filepath.Join(root, "heavy.lock")
	_, heavyError := os.Stat(heavyPath)
	heavyActive := false
	if heavyError == nil {
		heavyActive, err = heavyLeaseHeld(port, heavyPath)
		if err != nil {
			return err
		}
	}
	if heavyError != nil && !errors.Is(heavyError, os.ErrNotExist) {
		return heavyError
	}
	liveSessions, sessionError := hasLiveSessionRecords(port, root)
	if sessionError != nil {
		return sessionError
	}
	if liveSessions || heavyActive {
		return coordinationProtocolMismatch("exclusive mode has a live owner")
	}
	if err = os.RemoveAll(heavyPath); err != nil {
		return compatibilityStateError("stale compatibility heavy state cannot be removed")
	}

	return writeCoordinationMarker(root, coordinationMarker{
		SchemaVersion: coordinationSchemaVersion,
		Mode:          coordinationModeReservation,
	})
}

func inheritedReservation(
	port system.System, root, candidate string, ledger reservationLedger,
) (*Session, error) {
	if candidate == "" {
		return nil, nil //nolint:nilnil // Absence of an inheritance candidate is an expected non-error state.
	}
	for _, owner := range ledger.Owners {
		if owner.Token == candidate {
			alive, err := reservationIdentityAlive(port, root, candidate, owner.IdentityDevice, owner.IdentityInode)
			if err != nil || !alive {
				return nil, err
			}
			return &Session{
				Inherited:  true,
				Token:      candidate,
				Allocation: owner.Allocated,
				Requested:  owner.Requested,
			}, nil
		}
	}

	return nil, nil //nolint:nilnil // A live candidate not present in this ledger is an expected non-error state.
}

// neverStartedAtCoordinationLock leaves the never-started receipt a waiter
// owes when it stops at the coordination lock rather than in its retry wait.
// A lock another admission held until the wait ran out is the wait reaching
// its deadline, and a signal while waiting for it cancels admission; either
// way nothing started, and the receipt is what a caller reads before
// requeueing. Any other failure keeps its own shape.
func neverStartedAtCoordinationLock(
	port system.System, root, runID string, metadata ReservationMetadata, class policy.TaskClass,
	now time.Time, lockError error,
) error {
	var reason string
	cancelled := evidence.OutcomeAdmissionCancelled.String()
	switch {
	case errors.Is(lockError, errCoordinationDeferred):
		reason = "admission-deadline"
	case errors.Is(lockError, context.Canceled), errors.Is(lockError, context.DeadlineExceeded):
		reason = cancelled
	default:
		return lockError
	}
	receiptError := writeSafetyReceipt(port, root, runID, "never-started", reason, metadata, class, now)
	if receiptError == nil {
		return lockError
	}
	refused := refusedEvidenceWrite("writing the never-started receipt", receiptError)
	if reason == cancelled {
		return errors.Join(lockError, refused)
	}

	// As at the deadline of the retry wait, a refused receipt outranks the
	// deferral: the caller must not requeue on a receipt that is missing.
	return refused
}

func retainReservationWaiterUntilCleanup(port system.System, root, value string, identity *os.File) {
	if identity == nil {
		return
	}
	_ = ensureReservationIdentityLocked(port, identity)
	go func() {
		for {
			if err := ensureReservationIdentityLocked(port, identity); err != nil {
				time.Sleep(coordinationPollInterval)

				continue
			}
			if err := removeReservationWaiterAfterCancellation(port, root, value, coordinationLifecycleWait); err == nil {
				_ = identity.Close()
				_ = removeReservationIdentity(root, value)

				return
			}
			time.Sleep(coordinationPollInterval)
		}
	}()
}

func removeReservationWaiterAfterCancellation(
	port system.System, root, value string, wait time.Duration,
) error {
	return removeReservationWaiterWithAdvisories(port, root, value, wait, nil)
}

func removeReservationWaiterWithAdvisories(
	port system.System, root, value string, wait time.Duration, advisories *application.DisplayAdvisories,
) (returnError error) {
	lock, err := acquireCoordinationLock(port, context.Background(), root, wait)
	if err != nil {
		return err
	}
	defer func() { returnError = errors.Join(returnError, releaseCoordinationLock(port, lock)) }()
	return application.RetireDisplay(
		application.DisplayRetirement{RunID: value, Ownership: application.DisplayOwnershipOwned},
		displayRetirementEffects(root, func() (application.DisplayRetirementResult, error) {
			ledger, err := readReservationLedger(port, root)
			if err != nil {
				return application.DisplayRetirementResult{}, err
			}
			found := false
			ledger.Waiters = slices.DeleteFunc(ledger.Waiters, func(waiter reservationWaiter) bool {
				if waiter.Token == value {
					found = true
					return true
				}
				return false
			})
			if !found {
				alive, identityError := reservationIdentityAlive(port, root, value, 0, 0)
				// Unknown identity retains advisory context without introducing a
				// new ownership-cleanup failure or changing ledger error precedence.
				found = identityError == nil && alive
			}
			if err = writeReservationLedger(root, ledger); err != nil {
				return application.DisplayRetirementResult{}, err
			}
			return application.DisplayRetirementResult{Confirmed: found}, nil
		}), advisories,
	)
}

// ActivateReservation records the supervised child process group without changing allocation.
func ActivateReservation(root string, session *Session, processGroup int) (returnError error) {
	if session == nil || session.Inherited || processGroup <= 0 {
		return nil
	}
	lock, err := lockCoordinationForActivation(system.Production, root)
	if err != nil {
		return err
	}
	defer func() { returnError = errors.Join(returnError, releaseCoordinationLock(system.Production, lock)) }()

	ledger, err := readReservationLedger(system.Production, root)
	if err != nil {
		return err
	}
	for index := range ledger.Owners {
		if ledger.Owners[index].Token == session.Token {
			ledger.Owners[index].ProcessGroup = processGroup

			return writeReservationLedger(root, ledger)
		}
	}

	return errors.New("reservation owner disappeared before child activation")
}

func releaseReservationIdentity(root string, session *Session) error {
	if session == nil || session.identityLock == nil {
		return nil
	}
	err := session.identityLock.Close()
	session.identityLock = nil

	return errors.Join(err, removeReservationIdentity(root, session.Token))
}

func abandonReservationIdentity(session *Session) error {
	if session == nil || session.identityLock == nil {
		return nil
	}
	err := session.identityLock.Close()
	session.identityLock = nil

	return err
}

// retainReleaseForReconciliation keeps an unreleasable owner mark alive so a
// peer completes the release, and reports shared-lock contention as a deferred
// cleanup rather than a caller failure.
func retainReleaseForReconciliation(
	port system.System, ctx context.Context, root string, session *Session, lockError error, //nolint:revive // The system port leads every adapter signature; the context follows it.
) error {
	if retainError := retainReservationOwnerUntilRelease(port, ctx, root, session); retainError != nil {
		return errors.Join(lockError, retainError)
	}
	if errors.Is(lockError, errCoordinationDeferred) {
		return fmt.Errorf("%w: %w", ErrCoordinationCleanupDeferred, lockError)
	}

	return lockError
}

// ReleaseReservation removes exactly one token-authenticated owner and its liveness lock.
func ReleaseReservation(root string, session *Session) error {
	return releaseReservation(system.Production, root, session, true)
}

// ReleaseReservationContext retires the owned reservation using the caller's
// context values and the existing bounded cleanup wait despite cancellation.
func ReleaseReservationContext(ctx context.Context, root string, session *Session) error {
	return releaseReservationContext(system.Production, ctx, root, session, true)
}

func releaseReservation(port system.System, root string, session *Session, retainOnFailure bool) error {
	return releaseReservationContext(port, context.Background(), root, session, retainOnFailure)
}

func releaseReservationContext(
	port system.System, ctx context.Context, root string, session *Session, retainOnFailure bool, //nolint:revive // The system port leads every adapter signature; the context follows it.
) (returnError error) {
	if session == nil {
		return nil
	}
	if session.Inherited {
		return application.RetireDisplay(application.DisplayRetirement{RunID: session.Token, Ownership: application.DisplayOwnershipInherited}, application.DisplayRetirementEffects{}, session.advisories)
	}
	lock, err := lockCoordinationForReleaseContext(port, ctx, root)
	if err != nil {
		if !retainOnFailure || session.identityLock == nil {
			return err
		}

		return retainReleaseForReconciliation(port, ctx, root, session, err)
	}
	defer func() { returnError = errors.Join(returnError, releaseCoordinationLock(port, lock)) }()
	return application.RetireDisplay(
		application.DisplayRetirement{RunID: session.Token, Ownership: application.DisplayOwnershipOwned},
		displayRetirementEffects(root, func() (application.DisplayRetirementResult, error) {
			authenticated := session.identityLock != nil
			err := releaseReservationOwnershipLocked(port, root, session)
			return application.DisplayRetirementResult{Confirmed: authenticated && err == nil}, err
		}), session.advisories,
	)
}

func releaseReservationOwnershipLocked(port system.System, root string, session *Session) error {
	ledger, err := readReservationLedger(port, root)
	if err != nil {
		return err
	}
	found := false
	ledger.Owners = slices.DeleteFunc(ledger.Owners, func(owner ReservationOwner) bool {
		if owner.Token == session.Token {
			found = true

			return true
		}

		return false
	})
	if !found {
		return errors.New("refusing to release a reservation owned by another process")
	}
	if err = writeReservationLedger(root, ledger); err != nil {
		return err
	}
	returnError := releaseReservationIdentity(root, session)
	if len(ledger.Owners) == 0 && len(ledger.Waiters) == 0 {
		returnError = errors.Join(returnError, os.Remove(reservationLedgerPath(root)))
		if marker, present, markerError := readCoordinationMarker(root); markerError != nil {
			returnError = errors.Join(returnError, markerError)
		} else if present && marker.Mode == coordinationModeReservation {
			removeError := os.Remove(coordinationMarkerPath(root))
			if !errors.Is(removeError, os.ErrNotExist) {
				returnError = errors.Join(returnError, removeError)
			}
		}
	}

	return returnError
}

func retainReservationOwnerUntilRelease(
	port system.System, ctx context.Context, root string, session *Session, //nolint:revive // The system port leads every adapter signature; the context follows it.
) error {
	retained := &Session{Token: session.Token, identityLock: session.identityLock}
	session.identityLock = nil
	lockError := ensureReservationIdentityLocked(port, retained.identityLock)
	cleanup := context.WithoutCancel(ctx)
	go func() {
		for retained.identityLock != nil {
			if err := ensureReservationIdentityLocked(port, retained.identityLock); err != nil {
				time.Sleep(coordinationPollInterval)

				continue
			}
			releaseError := releaseReservationContext(port, cleanup, root, retained, false)
			if releaseError == nil || retained.identityLock == nil {
				return
			}
			time.Sleep(coordinationPollInterval)
		}
	}()

	return lockError
}

// ReservationStatus reads a reconciled, privacy-safe shared-root summary.
func ReservationStatus(ctx context.Context, root string) (ReservationTotals, error) {
	totals := ReservationTotals{SchemaVersion: 5, Mode: "exclusive"}
	marker, present, err := readCoordinationMarker(root)
	if err != nil || !present || marker.Mode != coordinationModeReservation {
		return totals, err
	}
	totals.Mode = "reservation"
	lock, err := acquireCoordinationLock(system.Production, ctx, root, coordinationObservationWait)
	if err != nil {
		return ReservationTotals{}, err
	}
	defer releaseCoordinationLock(system.Production, lock) //nolint:errcheck // Read-only status already has its result.

	ledger, err := readReservationLedger(system.Production, root)
	if err != nil {
		return ReservationTotals{}, err
	}
	// Read the abandoned groups before reconciliation, which is what drops the
	// stale records and with them the only handle on the orphaned payload.
	abandoned := abandonedProcessGroups(system.Production, ledger)
	if err = reconcileReservationLedger(system.Production, root, &ledger); err != nil {
		return ReservationTotals{}, err
	}
	if err = pruneReservationMetadata(system.Production, root, ledger); err != nil {
		return ReservationTotals{}, err
	}
	housekeepDisplayLocked(system.Production, root, time.Now())
	totals.AbandonedProcessGroups = abandoned
	totals.Capacity = ledger.Capacity
	totals.ActiveOwners = len(ledger.Owners)
	totals.WaitingOwners = len(ledger.Waiters)
	for _, owner := range ledger.Owners {
		// The ledger was validated at read and reconciliation only removes
		// owners, so running owner totals stay within capacity and the checked
		// addition below cannot overflow; the error is discarded, not exempted.
		_ = coordination.CheckedAddVector(&totals.Allocated, owner.Allocated)
		switch owner.Class {
		case policy.TaskEphemeral:
			totals.Ephemeral++
		case policy.TaskService:
			totals.Service++
		case policy.TaskTransactional:
			totals.Transactional++
		case policy.TaskRelease:
			// Release workloads use the separate strict release guard.
		}
		entry, entryError := reservationEntry(
			system.Production, root, owner.Token, "active", 0, owner.Class, owner.Profile, owner.Requested,
			owner.Allocated,
		)
		if entryError != nil {
			return ReservationTotals{}, entryError
		}
		if entry.Legacy {
			totals.LegacyEntries++
		}
		totals.Owners = append(totals.Owners, entry)
	}
	for index, waiter := range ledger.Waiters {
		if err = coordination.CheckedAddVector(&totals.Waiting, waiter.Requested); err != nil {
			return ReservationTotals{}, err
		}
		entry, entryError := reservationEntry(
			system.Production, root, waiter.Token, "waiting", index+1, waiter.Class, waiter.Profile,
			waiter.Requested, ReservationVector{},
		)
		if entryError != nil {
			return ReservationTotals{}, entryError
		}
		if entry.Legacy {
			totals.LegacyEntries++
		}
		totals.Waiters = append(totals.Waiters, entry)
	}

	return totals, nil
}

// ReservationOwnerPeak returns the event-complete peak active-owner count
// retained for one live token without exposing any peer identity.
func ReservationOwnerPeak(ctx context.Context, root string, session *Session) (int, error) {
	if session == nil || session.Inherited {
		return 0, nil
	}
	lock, err := acquireCoordinationLock(system.Production, ctx, root, coordinationLifecycleWait)
	if err != nil {
		return 0, err
	}
	defer releaseCoordinationLock(system.Production, lock) //nolint:errcheck // Read-only peak lookup returns its result.
	ledger, err := readReservationLedger(system.Production, root)
	if err != nil {
		return 0, err
	}
	if err = reconcileReservationLedger(system.Production, root, &ledger); err != nil {
		return 0, err
	}
	for _, owner := range ledger.Owners {
		if owner.Token == session.Token {
			return max(owner.PeakOwners, 1), nil
		}
	}

	return 0, errors.New("reservation owner disappeared during peak observation")
}

func reservationVictimPresent(
	port system.System, ctx context.Context, root string, victim ReservationOwner, //nolint:revive // The system port leads every adapter signature; the context follows it.
) (bool, error) {
	lock, err := acquireCoordinationLock(port, ctx, root, coordinationLifecycleWait)
	if err != nil {
		return false, err
	}
	finish := func(present bool, outcome error) (bool, error) {
		return present, errors.Join(outcome, releaseCoordinationLock(port, lock))
	}

	ledger, err := readReservationLedger(port, root)
	if err != nil {
		return finish(false, err)
	}
	if err = reconcileReservationLedger(port, root, &ledger); err != nil {
		return finish(false, err)
	}
	for _, owner := range ledger.Owners {
		if owner.Token != victim.Token {
			continue
		}
		if owner.ProcessGroup != victim.ProcessGroup || !owner.Shedding || owner.SheddingCause != victim.SheddingCause {
			return finish(false, errors.New("selected reservation victim ownership changed during shedding"))
		}

		return finish(true, nil)
	}

	return finish(false, nil)
}

// ReservationSheddingSelection returns the privacy-safe cause selected for this owner.
// Only the owning guard uses this mark to stop and reap its own supervised child.
func ReservationSheddingSelection(root string, session *Session) (bool, ShedCause, error) {
	if session == nil || session.Inherited {
		return false, ShedCauseNone, nil
	}
	lock, err := lockCoordinationForRelease(system.Production, root)
	if err != nil {
		return false, ShedCauseNone, err
	}
	finish := func(selected bool, cause ShedCause, outcome error) (bool, ShedCause, error) {
		return selected, cause, errors.Join(outcome, releaseCoordinationLock(system.Production, lock))
	}

	ledger, err := readReservationLedger(system.Production, root)
	if err != nil {
		return finish(false, ShedCauseNone, err)
	}
	if err = reconcileReservationLedger(system.Production, root, &ledger); err != nil {
		return finish(false, ShedCauseNone, err)
	}
	for _, owner := range ledger.Owners {
		if owner.Token == session.Token {
			return finish(owner.Shedding, owner.SheddingCause, nil)
		}
	}

	return finish(false, ShedCauseNone, errors.New("reservation owner disappeared during supervision"))
}

// SelectPressureVictim atomically selects at most one newest revocable owner.
func SelectPressureVictim(root string, cause ShedCause) (ReservationOwner, bool, error) {
	return selectPressureVictim(system.Production, root, cause, false)
}

// SelectEmergencyPressureVictim permits transactional work only after all
// revocable classes have been considered at the configured emergency floor.
func SelectEmergencyPressureVictim(root string, cause ShedCause) (ReservationOwner, bool, error) {
	return selectPressureVictim(system.Production, root, cause, true)
}

func selectPressureVictim(
	port system.System, root string, cause ShedCause, includeTransactional bool,
) (ReservationOwner, bool, error) {
	if cause.Reason() == policy.ReasonNone {
		return ReservationOwner{}, false, errors.New("reservation shed cause is invalid")
	}
	lock, err := acquireCoordinationLock(port, context.Background(), root, coordinationSelectionWait)
	if err != nil {
		return ReservationOwner{}, false, err
	}
	defer releaseCoordinationLock(port, lock) //nolint:errcheck // A selection error is returned from ledger mutation.

	ledger, err := readReservationLedger(port, root)
	if err != nil {
		return ReservationOwner{}, false, err
	}
	if err = reconcileReservationLedger(port, root, &ledger); err != nil {
		return ReservationOwner{}, false, err
	}
	victim, selected, electionError := coordination.ElectVictim(&ledger, cause, includeTransactional)
	if electionError != nil || !selected {
		return victim, selected, electionError
	}
	if err = writeReservationLedger(root, ledger); err != nil {
		return ReservationOwner{}, false, err
	}

	return victim, true, nil
}
