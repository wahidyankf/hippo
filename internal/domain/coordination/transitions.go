package coordination

import (
	"errors"
	"slices"

	"github.com/wahidyankf/hippo/internal/policy"
)

// Enqueue registers one identity in FIFO order without performing effects.
func Enqueue(ledger *Ledger, waiter Waiter) error {
	if ledger.NextSequence == ^uint64(0) {
		if len(ledger.Owners) != 0 || len(ledger.Waiters) != 0 {
			return ErrReservationDeferred
		}
		ledger.NextSequence = 0
	}
	ledger.NextSequence++
	waiter.Sequence = ledger.NextSequence
	ledger.Waiters = append(ledger.Waiters, waiter)
	return nil
}

// AdmitHead allocates the largest safe vector only to the current FIFO head.
func AdmitHead(ledger *Ledger, token string, minimum, maximum ReservationVector, ownerLimit int) (ReservationVector, bool, error) {
	used, err := SumOwners(ledger.Owners)
	if err != nil {
		return ReservationVector{}, false, err
	}
	allocation, fits := LargestAvailableAllocation(used, minimum, maximum, ledger.Capacity)
	if len(ledger.Waiters) == 0 || ledger.Waiters[0].Token != token || len(ledger.Owners) >= SharedOwnerLimit(*ledger, ownerLimit) || !fits {
		return allocation, false, nil
	}
	waiter := ledger.Waiters[0]
	ledger.Waiters = ledger.Waiters[1:]
	active := len(ledger.Owners) + 1
	for i := range ledger.Owners {
		ledger.Owners[i].PeakOwners = max(ledger.Owners[i].PeakOwners, active)
	}
	ledger.Owners = append(ledger.Owners, ReservationOwner{Token: token, PID: waiter.PID, Class: waiter.Class, Profile: waiter.Profile, Requested: allocation, Allocated: allocation, Sequence: waiter.Sequence, ConfigHash: waiter.ConfigHash, MaxOwners: ownerLimit, PeakOwners: active, IdentityDevice: waiter.IdentityDevice, IdentityInode: waiter.IdentityInode})
	return allocation, true, nil
}

// ElectVictim marks one newest revocable owner without signaling a process.
func ElectVictim(ledger *Ledger, cause ShedCause, includeTransactional bool) (ReservationOwner, bool, error) {
	if cause.Reason() == policy.ReasonNone {
		return ReservationOwner{}, false, errors.New("reservation shed cause is invalid")
	}
	if slices.ContainsFunc(ledger.Owners, func(owner ReservationOwner) bool { return owner.Shedding }) {
		return ReservationOwner{}, false, nil
	}
	selected := -1
	classes := []policy.TaskClass{policy.TaskEphemeral, policy.TaskService}
	if includeTransactional {
		classes = append(classes, policy.TaskTransactional)
	}
	for _, class := range classes {
		for index := range ledger.Owners {
			owner := ledger.Owners[index]
			if owner.Class == class && !owner.Shedding && owner.ProcessGroup > 0 &&
				(selected < 0 || owner.Sequence > ledger.Owners[selected].Sequence) {
				selected = index
			}
		}
		if selected >= 0 {
			break
		}
	}
	if selected < 0 {
		return ReservationOwner{}, false, nil
	}
	ledger.Owners[selected].Shedding = true
	ledger.Owners[selected].SheddingCause = cause

	return ledger.Owners[selected], true, nil
}
