package support

import (
	"fmt"
	"strconv"

	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/tests/contract"
)

// thresholdScenario is the scenario's optional admission knobs: the
// RAM-proportional thresholds and the swap-use multiple. Both are zero when
// the scenario sets neither, which keeps the legacy behavior.
type thresholdScenario struct {
	memory       policy.MemoryPolicy
	swapMultiple float64
}

// thresholdBindings are the steps for the optional RAM-proportional admission
// thresholds and the per-machine swap gate: a warning or critical percent of
// the effective memory limit with an optional floor, and a multiple of physical
// memory within which swap-out pressure is waived.
func (driver *Driver) thresholdBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^the warning memory threshold is (\d+) percent of memory$`, driver.warningMemoryPercent),
		step(`^the warning memory floor is (\S+ \S+)$`, driver.warningMemoryFloor),
		step(`^the critical memory threshold is (\d+) percent of memory$`, driver.criticalMemoryPercent),
		step(`^swap use within (\S+) times physical memory is allowed$`, driver.swapUseMultiple),
	}
}

func (driver *Driver) warningMemoryPercent(percent string) error {
	value, err := strconv.ParseFloat(percent, 64)
	if err != nil {
		return err
	}
	driver.thresholds.memory.WarningPercent = value

	return nil
}

func (driver *Driver) warningMemoryFloor(text string) error {
	floor, err := parseSwapBytes(text)
	if err != nil {
		return err
	}
	driver.thresholds.memory.WarningFloorBytes = floor

	return nil
}

func (driver *Driver) criticalMemoryPercent(percent string) error {
	value, err := strconv.ParseFloat(percent, 64)
	if err != nil {
		return err
	}
	driver.thresholds.memory.CriticalPercent = value

	return nil
}

func (driver *Driver) swapUseMultiple(multiple string) error {
	value, err := strconv.ParseFloat(multiple, 64)
	if err != nil {
		return err
	}
	if value <= 0 {
		return fmt.Errorf("swap multiple %q must be positive", multiple)
	}
	driver.thresholds.swapMultiple = value

	return nil
}
