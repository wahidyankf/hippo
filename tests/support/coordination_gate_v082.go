package support

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/sys/unix"

	"github.com/wahidyankf/hippo/internal/guard"
	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/tests/contract"
)

// coordinationGateScenario records what the free-gate scenario observed.
type coordinationGateScenario struct {
	root                 string
	admitted, deferred   int
	coordinationDeferred int
	other                []error
}

func (driver *Driver) coordinationGateBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^a reservation root that no other admission is updating$`, driver.freeCoordinationRootV082),
		step(
			`^a fitting reservation reaches the coordination gate with one nanosecond of its wait left, (\d+) times$`,
			driver.nearlySpentGateAttemptsV082,
		),
		step(`^every attempt is admitted and none is deferred for coordination$`, driver.requireEveryAttemptAdmittedV082),
		step(
			`^with the coordination lock held by another admission, the same wait is deferred for coordination$`,
			driver.requireHeldGateDefersV082,
		),
	}
}

func (driver *Driver) freeCoordinationRootV082() error {
	root, err := driver.temporaryRoot()
	driver.gate = coordinationGateScenario{root: root}

	return err
}

// gatePlan is a reservation that always fits the capacity it declares, so
// only the coordination gate can stand between it and admission.
func gatePlan() guard.ReservationPlan {
	return guard.ReservationPlan{
		Capacity:  guard.ReservationVector{CPU: 4, MemoryBytes: 4 * policy.GiB},
		Requested: guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
		Allocated: guard.ReservationVector{CPU: 1, MemoryBytes: 256 * policy.MiB},
	}
}

// admitWithNearlySpentWait asks for admission with a one-nanosecond wait and
// a clock that does not move, so the gate is reached with exactly that much
// of the wait left: the budget a bounded wait's last pass arrives with.
func admitWithNearlySpentWait(root string) (*guard.Session, error) {
	clock := time.Now()

	return guard.AcquireReservationWithOptions(
		context.Background(), root, "", policy.TaskEphemeral, "balanced", "", gatePlan(), 3, time.Nanosecond,
		guard.ReservationAdmissionOptions{
			Metadata: guard.ReservationMetadata{Source: "gate-fixture"},
			Now:      func() time.Time { return clock },
		},
	)
}

func (driver *Driver) nearlySpentGateAttemptsV082(count string) error {
	attempts, err := strconv.Atoi(count)
	if err != nil {
		return err
	}
	for range attempts {
		session, admitError := admitWithNearlySpentWait(driver.gate.root)
		switch {
		case admitError == nil && session != nil:
			driver.gate.admitted++
			if releaseError := guard.ReleaseReservation(driver.gate.root, session); releaseError != nil {
				return releaseError
			}
		case guard.IsCoordinationDeferred(admitError):
			driver.gate.coordinationDeferred++
		case errors.Is(admitError, guard.ErrReservationDeferred):
			driver.gate.deferred++
		default:
			driver.gate.other = append(driver.gate.other, admitError)
		}
	}

	return nil
}

func (driver *Driver) requireEveryAttemptAdmittedV082() error {
	gate := driver.gate
	if gate.admitted == 0 || gate.coordinationDeferred != 0 || gate.deferred != 0 || len(gate.other) != 0 {
		return fmt.Errorf(
			"a free gate with one nanosecond left: admitted=%d coordination-deferred=%d capacity-deferred=%d other=%v",
			gate.admitted, gate.coordinationDeferred, gate.deferred, errors.Join(gate.other...),
		)
	}

	return nil
}

// holdCoordinationLock takes the root's coordination lock through a file
// description of its own, the way another admitting process holds it, and
// keeps it until the scenario ends.
func (driver *Driver) holdCoordinationLock(root string) error {
	//nolint:gosec // G304: the lock path is inside the scenario's own temporary root.
	lock, err := os.OpenFile(filepath.Join(root, "coordination.lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = lock.Close()

		return fmt.Errorf("hold the coordination lock: %w", err)
	}
	driver.stops = append(driver.stops, func() {
		_ = unix.Flock(int(lock.Fd()), unix.LOCK_UN)
		_ = lock.Close()
	})

	return nil
}

func (driver *Driver) requireHeldGateDefersV082() error {
	if err := driver.holdCoordinationLock(driver.gate.root); err != nil {
		return err
	}
	session, err := admitWithNearlySpentWait(driver.gate.root)
	if session != nil {
		_ = guard.ReleaseReservation(driver.gate.root, session)

		return errors.New("a held coordination lock admitted a second admission")
	}
	if !guard.IsCoordinationDeferred(err) {
		return fmt.Errorf("a held coordination lock returned %v, want a coordination deferral", err)
	}

	return nil
}
