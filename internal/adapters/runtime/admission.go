package runtime

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/wahidyankf/hippo/internal/application"
	"github.com/wahidyankf/hippo/internal/domain/coordination"
	"golang.org/x/sys/unix"
)

type admissionState struct {
	engine            *Engine
	config            application.RunConfig
	minimum, maximum  coordination.ReservationVector
	value             string
	identity          *os.File
	queued            bool
	started, deadline time.Time
	cleanupWait       time.Duration
	closed            bool
	advisories        *application.DisplayAdvisories
}

func (engine *Engine) rememberSession(session *Session) *application.Lease {
	if session == nil {
		return nil
	}
	engine.leases[session.Token] = session
	return &application.Lease{Token: session.Token, Inherited: session.Inherited, Allocation: session.Allocation, Requested: session.Requested, WaitDuration: session.WaitDuration}
}

// BeginAdmission prepares one identity; each Attempt preserves one serialized transaction.
func (engine *Engine) BeginAdmission(ctx context.Context, config application.RunConfig) (application.Admission, error) {
	if err := ctx.Err(); err != nil {
		return application.Admission{}, err
	}
	wait := config.Policy.LeaseWait
	if wait < 0 {
		return application.Admission{}, errors.New(map[bool]string{true: "reservation wait must be nonnegative", false: "lease wait must be nonnegative"}[config.ReservationPolicy.Enabled])
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	config.Label = application.ResolveDisplayLabel(config.Command, config.Label)
	state := &admissionState{engine: engine, config: config, cleanupWait: coordinationLifecycleWait}
	if config.AdmissionCleanupWait != 0 {
		state.cleanupWait = config.AdmissionCleanupWait
	}
	if !config.ReservationPolicy.Enabled {
		if err := os.MkdirAll(config.EvidenceRoot, 0o700); err != nil {
			return application.Admission{}, err
		}
		return application.Admission{Attempt: state.attemptExclusive, Receipt: state.receipt, Close: state.close}, nil
	}
	if wait < 0 {
		return application.Admission{}, errors.New("reservation wait must be nonnegative")
	}
	minimum, maximum, err := coordination.PlanBounds(config.ReservationPlan)
	if err != nil {
		return application.Admission{}, err
	}
	state.minimum, state.maximum = minimum, maximum
	if err = os.MkdirAll(config.EvidenceRoot, 0o700); err != nil {
		return application.Admission{}, err
	}
	inheritedToken := environmentValue(config.Environment, "HIPPO_SESSION")
	if inheritedToken != "" {
		session, inheritError := state.inheritedAdmission(ctx, inheritedToken, wait)
		if inheritError != nil {
			return application.Admission{}, inheritError
		}
		if session != nil {
			return application.Admission{Admitted: engine.rememberSession(session), Close: func() {}}, nil
		}
	}
	state.value, err = token()
	if err != nil {
		return application.Admission{}, err
	}
	state.started = config.Now()
	state.deadline = state.started.Add(wait)
	return application.Admission{Attempt: state.attemptReservation, Receipt: state.receipt, Close: state.close}, nil
}

func (state *admissionState) inheritedAdmission(ctx context.Context, inheritedToken string, wait time.Duration) (*Session, error) {
	lock, lockError := acquireCoordinationLock(ctx, state.config.EvidenceRoot, wait)
	if lockError != nil {
		return nil, lockError
	}
	ledger, readError := readReservationLedger(state.config.EvidenceRoot)
	if readError == nil {
		readError = reconcileReservationLedger(state.config.EvidenceRoot, &ledger)
	}
	session, inheritError := inheritedReservation(state.config.EvidenceRoot, inheritedToken, ledger)
	var registeredSession *Session
	if readError == nil && inheritError == nil && session != nil {
		inheritError = application.RegisterDisplay(
			application.DisplayRegistration{RunID: session.Token, Label: state.config.Label, Kind: application.DisplayReservationRegistration, Ownership: application.DisplayOwnershipInherited},
			displayRegistrationEffects(state.config.EvidenceRoot, func() error { registeredSession = session; return nil }), state.displayAdvisories(),
		)
	}
	if err := errors.Join(readError, inheritError, releaseCoordinationLock(lock)); err != nil {
		return nil, err
	}
	return registeredSession, nil
}

func (state *admissionState) attemptExclusive(ctx context.Context, remaining time.Duration, _ time.Time) (application.AdmissionResult, error) {
	defer state.reportDisplayAdvisories()
	config := state.config
	lock, err := acquireCoordinationLock(ctx, config.EvidenceRoot, remaining)
	if err != nil {
		return application.AdmissionResult{}, err
	}
	housekeepDisplayLocked(config.EvidenceRoot, config.Now())
	session, deferred, acquireError := acquireSessionLocked(config.EvidenceRoot, environmentValue(config.Environment, "HIPPO_SESSION"), config.TaskClass, config.Label, state)
	if err = errors.Join(acquireError, releaseCoordinationLock(lock)); err != nil {
		return application.AdmissionResult{}, err
	}
	if !deferred {
		return application.AdmissionResult{Lease: state.engine.rememberSession(session)}, nil
	}
	return application.AdmissionResult{Waiting: true}, nil
}

func (state *admissionState) attemptReservation(ctx context.Context, remaining time.Duration, current time.Time) (application.AdmissionResult, error) {
	defer state.reportDisplayAdvisories()
	config := state.config
	root := config.EvidenceRoot
	value := state.value
	identity, queued := state.identity, state.queued
	defer func() { state.identity, state.queued = identity, queued }()
	minimum, maximum := state.minimum, state.maximum
	started, deadline := state.started, state.deadline
	now := func() time.Time { return current }
	plan := config.ReservationPlan
	class, profile, configHash := config.TaskClass, config.Resolution.ResolvedProfile, config.ConfigHash
	maxActiveOwners := coordination.NormalizedOwnerLimit(config.ReservationPolicy.MaxActiveOwners)
	options := ReservationAdmissionOptions{Metadata: config.ReservationMetadata}
	var err error
	coordinationLock, lockError := acquireCoordinationLock(ctx, root, remaining)
	if lockError != nil {
		return application.AdmissionResult{}, neverStartedAtCoordinationLock(root, value, options.Metadata, class, now(), lockError)
	}
	if lockError = ensureReservationCoordination(root); lockError != nil {
		_ = releaseCoordinationLock(coordinationLock)

		return application.AdmissionResult{}, lockError
	}
	ledger, readError := readReservationLedger(root)
	if readError != nil {
		_ = releaseCoordinationLock(coordinationLock)

		return application.AdmissionResult{}, readError
	}
	if reconcileError := reconcileReservationLedger(root, &ledger); reconcileError != nil {
		_ = releaseCoordinationLock(coordinationLock)

		return application.AdmissionResult{}, reconcileError
	}
	housekeepDisplayLocked(root, current)
	if identity == nil {
		identity, lockError = openReservationIdentity(root, value)
		if lockError != nil {
			_ = releaseCoordinationLock(coordinationLock)

			return application.AdmissionResult{}, lockError
		}
	}
	identityDevice, identityInode, identityError := reservationIdentityMetadata(identity)
	if identityError != nil {
		_ = releaseCoordinationLock(coordinationLock)

		return application.AdmissionResult{}, identityError
	}
	if !coordination.UpdateCapacity(&ledger, plan.Capacity) {
		_ = releaseCoordinationLock(coordinationLock)

		return application.AdmissionResult{}, ErrReservationDeferred
	}
	if !queued {
		if options.Metadata.Source != "" {
			if lockError = writeReservationMetadata(
				root, value, options.Metadata, minimum, maximum, started, deadline,
			); lockError != nil {
				_ = releaseCoordinationLock(coordinationLock)

				return application.AdmissionResult{}, lockError
			}
		}
	}
	var allocation coordination.ReservationVector
	admitted := false
	writeError := application.RegisterDisplay(
		application.DisplayRegistration{RunID: value, Label: config.Label, Kind: application.DisplayWaiterRegistration, Ownership: application.DisplayOwnershipOwned, Registered: queued},
		displayRegistrationEffects(root, func() error {
			if !queued {
				if enqueueError := coordination.Enqueue(&ledger, coordination.Waiter{Token: value, PID: os.Getpid(), Class: class, Profile: profile, Requested: minimum, ConfigHash: configHash, MaxOwners: maxActiveOwners, IdentityDevice: identityDevice, IdentityInode: identityInode}); enqueueError != nil {
					return enqueueError
				}
				queued = true
			}
			var sumError error
			allocation, admitted, sumError = coordination.AdmitHead(&ledger, value, minimum, maximum, maxActiveOwners)
			if sumError != nil {
				return sumError
			}
			return writeReservationLedger(root, ledger)
		}), state.displayAdvisories(),
	)
	releaseError := releaseCoordinationLock(coordinationLock)
	if err = errors.Join(writeError, releaseError); err != nil {
		return application.AdmissionResult{}, err
	}
	if admitted {
		queued = false
		session := &Session{
			Token: value, Allocation: allocation, Requested: minimum,
			WaitDuration: now().Sub(started), identityLock: identity,
			advisories: state.advisories,
		}
		identity = nil

		return application.AdmissionResult{Lease: state.engine.rememberSession(session)}, nil
	}

	position := 0
	for index, waiter := range ledger.Waiters {
		if waiter.Token == value {
			position = index + 1
			break
		}
	}
	return application.AdmissionResult{Waiting: true, Status: coordination.ReservationWaitStatus{RunID: value, Position: position, Remaining: max(deadline.Sub(current), 0)}}, nil
}

func (state *admissionState) receipt(reason string, now time.Time) error {
	if !state.config.ReservationPolicy.Enabled {
		return nil
	}
	return writeSafetyReceipt(state.config.EvidenceRoot, state.value, "never-started", reason, state.config.ReservationMetadata, state.config.TaskClass, now)
}

func (state *admissionState) close() {
	defer state.reportDisplayAdvisories()
	if state.closed {
		return
	}
	state.closed = true
	if state.queued {
		if err := removeReservationWaiterWithAdvisories(state.config.EvidenceRoot, state.value, state.cleanupWait, state.advisories); err != nil && state.identity != nil {
			retainReservationWaiterUntilCleanup(state.config.EvidenceRoot, state.value, state.identity)
			state.identity = nil
		}
	}
	if state.identity != nil {
		if !state.queued {
			retireUnregisteredDisplay(state.config.EvidenceRoot, state.value, state.cleanupWait, state.advisories)
		}
		_ = unix.Flock(int(state.identity.Fd()), unix.LOCK_UN)
		_ = state.identity.Close()
		_ = removeReservationIdentity(state.config.EvidenceRoot, state.value)
		state.identity = nil
	}
}

// TakeSession transfers a positively acquired runtime identity to an outer owner.
func (engine *Engine) TakeSession(lease *application.Lease) *Session {
	if lease == nil {
		return nil
	}
	session := engine.leases[lease.Token]
	delete(engine.leases, lease.Token)
	return session
}
