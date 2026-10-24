package support

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wahidyankf/hippo/internal/policy"
	"github.com/wahidyankf/hippo/tests/contract"
)

// swapStateUnavailable is the swap state a host reports when its swap cannot be read.
const swapStateUnavailable = "unavailable"

// swapCountingBindings are the steps for the optional swap allowance: a Linux
// or macOS host sample with swap evidence, the setting switched on, and the
// allowance, headroom, state and advisory the assessment reports.
func (driver *Driver) swapCountingBindings() []contract.StepBinding {
	return []contract.StepBinding{
		step(`^a Linux host with (\S+ \S+) of memory and (\S+ \S+) of available memory$`, driver.linuxSwapHost),
		step(`^a macOS host with (\S+ \S+) of memory and (\S+ \S+) of available memory$`, driver.darwinSwapHost),
		step(`^that host has (\S+ \S+) of swap of which (\S+ \S+) is in use$`, driver.hostSwapInUse),
		step(`^that host has (\S+ \S+) of swap$`, driver.hostSwap),
		step(`^that host has no swap$`, driver.hostWithoutSwap),
		step(`^that host has swap that is unreadable$`, driver.hostUnreadableSwap),
		step(`^swap-outs grow by 128 MiB over 15 seconds on that host$`, driver.hostSwapOutGrowth),
		step(`^memory PSI some average is (\d+) percent$`, driver.hostPSISome),
		step(`^swap counting is on$`, driver.countSwap),
		step(`^the swap allowance is (\d+ (?:MiB|GiB)|0 bytes)$`, driver.requireSwapAllowance),
		step(`^the swap allowance is absent$`, driver.requireSwapAllowanceAbsent),
		step(`^the swap headroom is (\d+ (?:MiB|GiB))$`, driver.requireSwapHeadroom),
		step(`^the resource state is (normal|warning|critical) with reason "([^"]+)"$`, driver.requireResourceState),
		step(`^the advisory "([^"]+)" is reported$`, driver.requireAdvisory),
	}
}

// parseSwapBytes reads a byte count written as "N MiB", "N GiB", or "0 bytes".
func parseSwapBytes(text string) (int64, error) {
	if text == "0 bytes" {
		return 0, nil
	}
	count, unit, found := strings.Cut(text, " ")
	if !found {
		return 0, fmt.Errorf("byte count %q has no unit", text)
	}
	value, err := strconv.ParseInt(count, 10, 64)
	if err != nil {
		return 0, err
	}
	switch unit {
	case "MiB":
		return value * policy.MiB, nil
	case "GiB":
		return value * policy.GiB, nil
	}
	return 0, fmt.Errorf("unknown byte unit %q", unit)
}

// currentSwapSample is the host sample the scenario builds next.
func (driver *Driver) currentSwapSample() (*policy.Sample, error) {
	if len(driver.samples) == 0 {
		return nil, errors.New("the scenario has no host sample")
	}

	return &driver.samples[len(driver.samples)-1], nil
}

func (driver *Driver) hostSample(platform, memory, available string) error {
	memoryBytes, err := parseSwapBytes(memory)
	if err != nil {
		return err
	}
	availableBytes, err := parseSwapBytes(available)
	if err != nil {
		return err
	}
	sample := capacitySample(memoryBytes, availableBytes)
	sample.Platform = platform
	sample.SwapState = swapStateUnavailable
	driver.samples = []policy.Sample{sample}

	return nil
}

func (driver *Driver) linuxSwapHost(memory, available string) error {
	return driver.hostSample("linux", memory, available)
}

func (driver *Driver) darwinSwapHost(memory, available string) error {
	return driver.hostSample("darwin", memory, available)
}

func (driver *Driver) hostSwap(total string) error {
	return driver.hostSwapInUse(total, "0 bytes")
}

func (driver *Driver) hostSwapInUse(total, used string) error {
	sample, err := driver.currentSwapSample()
	if err != nil {
		return err
	}
	totalBytes, err := parseSwapBytes(total)
	if err != nil {
		return err
	}
	usedBytes, err := parseSwapBytes(used)
	if err != nil {
		return err
	}
	freeBytes := max(int64(0), totalBytes-usedBytes)
	sample.SwapTotalBytes = new(totalBytes)
	sample.SwapUsedBytes = new(usedBytes)
	sample.SwapFreeBytes = new(freeBytes)
	sample.SwapState = "idle"
	if usedBytes > 0 {
		sample.SwapState = "active"
	}

	return nil
}

func (driver *Driver) hostWithoutSwap() error {
	sample, err := driver.currentSwapSample()
	if err != nil {
		return err
	}
	sample.SwapTotalBytes = new(int64(0))
	sample.SwapUsedBytes = new(int64(0))
	sample.SwapFreeBytes = new(int64(0))
	sample.SwapState = swapStateUnavailable

	return nil
}

func (driver *Driver) hostUnreadableSwap() error {
	sample, err := driver.currentSwapSample()
	if err != nil {
		return err
	}
	sample.SwapTotalBytes = new(8 * policy.GiB)
	sample.SwapUsedBytes = nil
	sample.SwapFreeBytes = nil
	sample.SwapState = "idle"

	return nil
}

// hostSwapOutGrowth turns the current sample into a 15-second window in which
// swap-outs grow by the 128 MiB the policy window normalizes.
func (driver *Driver) hostSwapOutGrowth() error {
	base, err := driver.currentSwapSample()
	if err != nil {
		return err
	}
	first, second := *base, *base
	first.MeasuredAt = time.Unix(0, 0).UTC().Format(time.RFC3339Nano)
	second.MeasuredAt = time.Unix(15, 0).UTC().Format(time.RFC3339Nano)
	first.SwapOuts = new(int64(0))
	second.SwapOuts = new(128 * policy.MiB / 16_384)
	driver.samples = []policy.Sample{first, second}

	return nil
}

func (driver *Driver) hostPSISome(percent string) error {
	sample, err := driver.currentSwapSample()
	if err != nil {
		return err
	}
	value, err := strconv.ParseFloat(percent, 64)
	if err != nil {
		return err
	}
	sample.MemoryPSISomeAvg10 = new(value)

	return nil
}

// countSwap applies the optional allowance to every sample, as the collector
// does when coordination.countSwap is on.
func (driver *Driver) countSwap() error {
	for index := range driver.samples {
		driver.samples[index] = policy.WithSwapAllowance(driver.samples[index], true)
	}

	return nil
}

func (driver *Driver) requireSwapAllowance(text string) error {
	want, err := parseSwapBytes(text)
	if err != nil {
		return err
	}
	sample, err := driver.currentSwapSample()
	if err != nil {
		return err
	}
	if sample.SwapAllowanceBytes == nil || *sample.SwapAllowanceBytes != want {
		return fmt.Errorf("got swap allowance %v, want %d", sample.SwapAllowanceBytes, want)
	}

	return nil
}

func (driver *Driver) requireSwapAllowanceAbsent() error {
	sample, err := driver.currentSwapSample()
	if err != nil {
		return err
	}
	if sample.SwapAllowanceBytes != nil {
		return fmt.Errorf("got swap allowance %d with counting off", *sample.SwapAllowanceBytes)
	}

	return nil
}

func (driver *Driver) requireSwapHeadroom(text string) error {
	want, err := parseSwapBytes(text)
	if err != nil {
		return err
	}
	sample, err := driver.currentSwapSample()
	if err != nil {
		return err
	}
	headroom := policy.SwapHeadroomBytes(*sample)
	if headroom == nil || *headroom != want {
		return fmt.Errorf("got swap headroom %v, want %d", headroom, want)
	}

	return nil
}

func (driver *Driver) requireResourceState(state, reason string) error {
	if string(driver.assessment.State) != state || driver.assessment.Reason != reason {
		return fmt.Errorf("got state %q reason %q, want %q %q", driver.assessment.State, driver.assessment.Reason, state, reason)
	}

	return nil
}

func (driver *Driver) requireAdvisory(advisory string) error {
	if !slices.Contains(driver.assessment.Advisories, advisory) {
		return fmt.Errorf("advisories %v do not include %q", driver.assessment.Advisories, advisory)
	}

	return nil
}
