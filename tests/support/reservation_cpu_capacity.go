package support

import (
	"fmt"
	"strconv"

	coordination "github.com/wahidyankf/hippo/internal/domain/coordination"

	guard "github.com/wahidyankf/hippo/internal/adapters/runtime"
	"github.com/wahidyankf/hippo/internal/policy"
)

// Automatic CPU capacity is every host CPU, lowered only by the configured
// maxCpu. These bindings plan one reservation on a host with a stated CPU count
// and read back the capacity the planner reports.

func (driver *Driver) hostWithCPUs(cpus string) error {
	return driver.prepareCPUCapacity(cpus, 0)
}

func (driver *Driver) hostWithCPUsAndMaximum(cpus, maximum string) error {
	limit, err := strconv.Atoi(maximum)
	if err != nil {
		return err
	}

	return driver.prepareCPUCapacity(cpus, limit)
}

func (driver *Driver) prepareCPUCapacity(cpus string, maximum int) error {
	parallelism, err := strconv.Atoi(cpus)
	if err != nil {
		return err
	}
	sample := v04ReservationSample()
	sample.AvailableParallelism = parallelism
	driver.cpuSample, driver.cpuMaximum = sample, maximum
	driver.cpuCapacity, driver.cpuCapacityError = 0, nil

	return nil
}

func (driver *Driver) planAutomaticCPUCapacity() error {
	settings := guard.ReservationPolicy{
		Enabled: true, MaxActiveOwners: ownerShareLimit, MaxCPU: driver.cpuMaximum,
		OwnerShares: map[policy.ProfileName]int{},
	}
	plan, err := coordination.PlanReservation(
		driver.cpuSample, policy.Resolution{ResolvedProfile: profileBalanced, MemoryReserve: 4 * policy.GiB}, settings, 0, 0,
	)
	driver.cpuCapacity, driver.cpuCapacityError = plan.Capacity.CPU, err

	return nil
}

// resolveBalancedProfileConcurrency resolves the built-in balanced profile on
// the prepared host, the profile whose concurrency has no cap of its own.
func (driver *Driver) resolveBalancedProfileConcurrency() error {
	resolution, err := policy.BuiltinCatalog().Resolve(profileBalanced, policy.TaskEphemeral, driver.cpuSample)
	driver.cpuConcurrency, driver.cpuCapacityError = resolution.Concurrency, err

	return nil
}

func (driver *Driver) requireProfileConcurrency(cpus string) error {
	want, err := strconv.Atoi(cpus)
	if err != nil {
		return err
	}
	if driver.cpuCapacityError != nil {
		return driver.cpuCapacityError
	}
	if driver.cpuConcurrency != want {
		return fmt.Errorf("resolved profile concurrency %d, want %d", driver.cpuConcurrency, want)
	}

	return nil
}

func (driver *Driver) requireCPUCapacity(cpus string) error {
	want, err := strconv.Atoi(cpus)
	if err != nil {
		return err
	}
	if driver.cpuCapacityError != nil {
		return driver.cpuCapacityError
	}
	if driver.cpuCapacity != want {
		return fmt.Errorf("planned CPU capacity %d, want %d", driver.cpuCapacity, want)
	}

	return nil
}
