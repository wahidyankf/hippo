//go:build linux

package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/wahidyankf/hippo/internal/policy"
)

func readOptionalFiles(ctx context.Context, read ContextFileReader, paths ...string) ([][]byte, error) {
	values := make([][]byte, len(paths))
	for index, path := range paths {
		value, err := read(ctx, path)
		if cancelled := ctx.Err(); cancelled != nil {
			return nil, cancelled
		}
		if err == nil {
			values[index] = value
		}
	}
	return values, nil
}

func linuxMemoryPressure(ctx context.Context, read ContextFileReader, cgroup string) (*float64, *float64, int, error) {
	values, readError := readOptionalFiles(ctx, read, filepath.Join(cgroup, "memory.pressure"))
	if readError != nil {
		return nil, nil, 0, readError
	}
	if len(values[0]) == 0 {
		values, readError = readOptionalFiles(ctx, read, "/proc/pressure/memory")
		if readError != nil {
			return nil, nil, 0, readError
		}
	}

	some, full, pressureLevel := linuxMemoryPressureValues(values[0])
	return some, full, pressureLevel, nil
}

func linuxMemoryPressureValues(data []byte) (*float64, *float64, int) {
	pressureLevel := 1
	psi, err := ParsePSI(string(data))
	if err != nil {
		return nil, nil, pressureLevel
	}

	some, full := &psi.SomeAvg10, &psi.FullAvg10
	if psi.SomeAvg10 >= 25 || psi.FullAvg10 >= 5 {
		pressureLevel = 4
	} else if psi.SomeAvg10 >= 10 {
		pressureLevel = 2
	}

	return some, full, pressureLevel
}

// collect gathers one Linux reading from procfs, cgroup v2, PSI, and statfs.
func (collector SystemCollector) collect(ctx context.Context, previous CPUState, diskPath string) (Reading, error) {
	if err := ctx.Err(); err != nil {
		return Reading{}, err
	}

	read := collector.contextFileReader()

	now := collector.Now
	if now == nil {
		now = time.Now
	}

	memData, err := read(ctx, "/proc/meminfo")
	if err != nil {
		return Reading{}, fmt.Errorf("read Linux memory: %w", err)
	}

	memory, err := ParseMemInfo(string(memData))
	if err != nil {
		return Reading{}, err
	}

	statData, err := read(ctx, "/proc/stat")
	if err != nil {
		return Reading{}, fmt.Errorf("read Linux CPU: %w", err)
	}

	currentCPU, err := ParseProcStat(string(statData))
	if err != nil {
		return Reading{}, err
	}

	cgroupValues, err := readOptionalFiles(ctx, read, "/proc/self/cgroup")
	if err != nil {
		return Reading{}, err
	}
	cgroup := CgroupRoot(string(cgroupValues[0]))
	capacity, err := readOptionalFiles(ctx, read,
		filepath.Join(cgroup, "memory.max"), filepath.Join(cgroup, "memory.high"),
		filepath.Join(cgroup, "memory.current"), filepath.Join(cgroup, "memory.stat"), filepath.Join(cgroup, "cpu.max"))
	if err != nil {
		return Reading{}, err
	}
	effective, available, parallelism := linuxCgroupCapacity(memory, capacity, runtime.NumCPU())

	some, full, pressureLevel, err := linuxMemoryPressure(ctx, read, cgroup)
	if err != nil {
		return Reading{}, err
	}

	swap, err := readOptionalFiles(ctx, read, filepath.Join(cgroup, "memory.events"),
		filepath.Join(cgroup, "memory.swap.max"), filepath.Join(cgroup, "memory.swap.current"))
	if err != nil {
		return Reading{}, err
	}
	events := ParseMemoryEvents(string(swap[0]))
	oom, oomKill := events["oom"], events["oom_kill"]

	swapMax, swapMaxFinite := ParseCgroupLimit(swap[1])
	swapCurrent, swapCurrentFinite := ParseCgroupLimit(swap[2])
	swapState := "idle"
	if memory.SwapTotal == 0 || swapMaxFinite && swapMax == 0 {
		swapState = "unavailable"
	} else if swapCurrentFinite && swapCurrent > 0 || memory.SwapFree < memory.SwapTotal {
		swapState = "active"
	}

	if err := ctx.Err(); err != nil {
		return Reading{}, err
	}
	diskFree, diskTotal, diskError := diskCapacity(diskPath)
	if diskError != nil {
		return Reading{}, fmt.Errorf("read disk capacity: %w", diskError)
	}

	pageSize := int64(os.Getpagesize())
	vmstat, err := readOptionalFiles(ctx, read, "/proc/vmstat")
	if err != nil {
		return Reading{}, err
	}
	swapIns, swapOuts := ParseLinuxVMStat(string(vmstat[0]))
	swapUsed := max(int64(0), memory.SwapTotal-memory.SwapFree)

	sample := policy.Sample{
		SchemaVersion:             3,
		MeasuredAt:                now().UTC().Format(time.RFC3339Nano),
		Platform:                  "linux",
		Capabilities:              []string{"cgroup-v2", "memory-psi"},
		EffectiveMemoryLimitBytes: effective,
		AvailableMemoryBytes:      &available,
		PhysicalMemoryBytes:       memory.Total,
		AvailableParallelism:      parallelism,
		CPUUtilizationPercent:     CPUUtilization(previous, currentCPU),
		DiskFreeBytes:             diskFree,
		DiskTotalBytes:            diskTotal,
		MemoryPressureLevel:       &pressureLevel,
		PageSizeBytes:             &pageSize,
		SwapIns:                   &swapIns,
		SwapOuts:                  &swapOuts,
		SwapTotalBytes:            &memory.SwapTotal,
		SwapUsedBytes:             &swapUsed,
		SwapFreeBytes:             &memory.SwapFree,
		SwapState:                 swapState,
		MemoryPSISomeAvg10:        some,
		MemoryPSIFullAvg10:        full,
		OOMEvents:                 &oom,
		OOMKillEvents:             &oomKill,
	}

	return Reading{CPUState: currentCPU, Sample: sample}, nil
}

func linuxCgroupCapacity(memory LinuxMemory, capacity [][]byte, parallelism int) (int64, int64, int) {
	maximum, maximumFinite := ParseCgroupLimit(capacity[0])
	high, highFinite := ParseCgroupLimit(capacity[1])
	if !maximumFinite {
		maximum = 0
	}
	if !highFinite {
		high = 0
	}

	effective := EffectiveMemoryLimit(memory.Total, maximum, high)
	available := CgroupAvailableMemory(
		memory.Available,
		effective,
		capacity[2], capacity[3],
	)

	if quota, finite := ParseCPUMax(string(capacity[4])); finite {
		parallelism = min(parallelism, quota)
	}
	parallelism = max(1, parallelism)

	return effective, available, parallelism
}
