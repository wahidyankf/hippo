package support

import (
	"context"
	"errors"
	"fmt"

	"github.com/wahidyankf/hippo/tests/support/runtimewiring"

	guard "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/tests/contract"
)

// sharedPoolState is what the scenario observed of the second client. The live
// owner is the driver's admission session, which cleanup releases.
type sharedPoolState struct {
	admitted *guard.Session
	err      error
}

// The live owner's pool holds one CPU and one GiB. The second client's pool holds
// two CPUs and four GiB, and its minimum needs two CPUs, which only that pool holds.
var (
	smallerPoolCapacity = guard.ReservationVector{CPU: 1, MemoryBytes: policy.GiB}
	smallerPoolRequest  = guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB}
	largerPoolCapacity  = guard.ReservationVector{CPU: 2, MemoryBytes: 4 * policy.GiB}
	largerPoolMinimum   = guard.ReservationVector{CPU: 2, MemoryBytes: 256 * policy.MiB}
)

func (driver *Driver) ledgerCapacityBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^a live owner holds a shared root whose pool is smaller than a second client's pool$`, driver.holdSmallerPoolOwnerV011),
		step(`^the second client requests a minimum that only its own pool can hold$`, driver.requestLargerPoolMinimumV011),
		step(`^the request is deferred with exit 124 naming hippo\.limit\.capacity-deferred and queues no waiter$`, driver.requireLargerPoolDeferredV011),
		step(`^the shared root still reads with the live owner alone$`, driver.requireSharedRootReadableV011),
		step(`^the live owner releases its reservation$`, driver.releaseSmallerPoolOwnerV011),
		step(`^the second client requests the same minimum again$`, driver.requestLargerPoolMinimumV011),
		step(`^the second client is admitted$`, driver.requireLargerPoolAdmittedV011),
	}
}

// holdSmallerPoolOwnerV011 gives the scenario a fresh shared root and a live owner
// that holds the smaller pool, so the epoch's capacity is that smaller pool.
func (driver *Driver) holdSmallerPoolOwnerV011() error {
	if err := driver.emptyCoordinationRoot(); err != nil {
		return err
	}
	driver.sharedPool = sharedPoolState{}
	session, err := runtimewiring.AcquireReservation(
		context.Background(), driver.leaseRoot, "", policy.TaskService, "balanced", "",
		guard.ReservationPlan{Capacity: smallerPoolCapacity, Requested: smallerPoolRequest, Allocated: smallerPoolRequest}, 20, 0,
	)
	if err != nil {
		return err
	}
	if session == nil {
		return errors.New("the smaller pool's owner was not admitted")
	}
	driver.admissionSession = session

	return nil
}

// requestLargerPoolMinimumV011 asks for the second client's minimum with no wait, so
// the outcome is the attempt's own: a deferral, or an admission once the root is free.
func (driver *Driver) requestLargerPoolMinimumV011() error {
	root := driver.leaseRoot
	session, err := runtimewiring.AcquireReservation(
		context.Background(), root, "", policy.TaskService, "balanced", "",
		guard.ReservationPlan{Capacity: largerPoolCapacity, Requested: largerPoolMinimum, Allocated: largerPoolMinimum}, 20, 0,
	)
	driver.sharedPool = sharedPoolState{admitted: session, err: err}
	if session != nil {
		driver.stops = append(driver.stops, func() { _ = guard.ReleaseReservation(root, session) })
	}

	return nil
}

func (driver *Driver) requireLargerPoolDeferredV011() error {
	if driver.sharedPool.admitted != nil {
		return errors.New("the second client was admitted, want a capacity deferral")
	}
	if !errors.Is(driver.sharedPool.err, guard.ErrReservationDeferred) {
		return fmt.Errorf("the second client returned a different outcome, want a capacity deferral: %w", driver.sharedPool.err)
	}
	totals, err := guard.ReservationStatus(context.Background(), driver.leaseRoot)
	if err != nil {
		return err
	}
	if totals.WaitingOwners != 0 {
		return fmt.Errorf("the deferred request left %d waiter(s) queued", totals.WaitingOwners)
	}

	return nil
}

func (driver *Driver) requireSharedRootReadableV011() error {
	totals, err := guard.ReservationStatus(context.Background(), driver.leaseRoot)
	if err != nil {
		return err
	}
	if totals.ActiveOwners != 1 || totals.WaitingOwners != 0 {
		return fmt.Errorf("shared root totals %+v, want the live owner alone", totals)
	}

	return nil
}

func (driver *Driver) releaseSmallerPoolOwnerV011() error {
	if driver.admissionSession == nil {
		return errors.New("no live owner holds the shared root")
	}
	err := guard.ReleaseReservation(driver.leaseRoot, driver.admissionSession)
	driver.admissionSession = nil

	return err
}

func (driver *Driver) requireLargerPoolAdmittedV011() error {
	if driver.sharedPool.admitted == nil {
		return fmt.Errorf("the second client was not admitted after the owner released: %w", driver.sharedPool.err)
	}

	return nil
}
