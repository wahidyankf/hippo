package host

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
	"github.com/wahidyankf/hippo/internal/policy"
)

const darwinPhysicalBytes = 16 * policy.GiB

// darwinProbeTable answers every macOS probe the collector runs, keyed by the command line. A row edits one entry.
func darwinProbeTable() map[string]string {
	return map[string]string{
		"ps -A -o %cpu=":         "120.0 60.0 0.0\n",
		"memory_pressure -Q":     "System-wide memory free percentage: 50%\n",
		"vm_stat":                "Mach Virtual Memory Statistics: (page size of 16384 bytes)\nPages stored in compressor: 10.\nPages occupied by compressor: 5.\nSwapins: 3.\nSwapouts: 4.\n",
		"sysctl -n vm.swapusage": "total = 4096.00M  used = 2562.38M  free = 1533.62M\n",
		"sysctl -n kern.memorystatus_vm_pressure_level": "2\n",
		"sysctl -n vm.compressor_available":             "1\n",
		"sysctl -n vm.compressor_bytes_used":            "4096\n",
	}
}

// cannedRunner answers each probe from the table and refuses the ones named in failures.
func cannedRunner(t *testing.T, outputs map[string]string, failures map[string]bool) CommandRunner {
	t.Helper()

	return func(_ context.Context, name string, arguments ...string) ([]byte, error) {
		key := strings.Join(append([]string{name}, arguments...), " ")
		if failures[key] {
			return nil, errors.New("probe refused: " + key)
		}
		output, known := outputs[key]
		if !known {
			t.Errorf("unexpected probe %q", key)
		}

		return []byte(output), nil
	}
}

// neverRunner fails the test if the collector runs a probe through it.
func neverRunner(t *testing.T) CommandRunner {
	t.Helper()

	return func(_ context.Context, name string, _ ...string) ([]byte, error) {
		t.Errorf("collector used the default runner for %s", name)

		return nil, errors.New("unexpected probe")
	}
}

func physicalMemory(value uint64, failure error) func(string, ...int) (uint64, error) {
	return func(string, ...int) (uint64, error) { return value, failure }
}

type darwinRow struct {
	name     string
	edit     func(outputs map[string]string, failures map[string]bool)
	physical func(string, ...int) (uint64, error)
	absent   bool
	cancel   bool
	useRun   bool
	noClock  bool
	wantErr  string
	inspect  func(t *testing.T, sample policy.Sample)
}

func darwinRows() []darwinRow {
	return append(darwinCollectRows(), darwinSampleRows()...)
}

func darwinCollectRows() []darwinRow {
	return []darwinRow{
		{name: "healthy probes", inspect: func(t *testing.T, sample policy.Sample) {
			t.Helper()
			if sample.Platform != "darwin" || sample.AvailableParallelism != 4 || sample.MeasuredAt != "1970-01-01T00:00:00Z" ||
				sample.AvailableMemoryBytes == nil || *sample.AvailableMemoryBytes != darwinPhysicalBytes/2 ||
				sample.MemoryPressureLevel == nil || *sample.MemoryPressureLevel != 2 ||
				sample.CompressorAvailable == nil || !*sample.CompressorAvailable ||
				sample.CompressorPayloadBytes == nil || *sample.CompressorPayloadBytes != 4096 ||
				sample.CPUUtilizationPercent == nil || *sample.CPUUtilizationPercent != 45 ||
				sample.PageSizeBytes == nil || *sample.PageSizeBytes != 16384 || sample.SwapState != "active" {
				t.Fatalf("healthy sample %+v", sample)
			}
		}},
		{name: "probes through the collector's runner", useRun: true, noClock: true, inspect: func(t *testing.T, sample policy.Sample) {
			t.Helper()
			if sample.MeasuredAt == "" || sample.CPUUtilizationPercent == nil || sample.Platform != "darwin" {
				t.Fatalf("explicit runner and default clock sample %+v", sample)
			}
		}},
		{name: "physical memory unreadable", physical: physicalMemory(0, errors.New("sysctl refused")), wantErr: "read physical memory"},
		{name: "physical memory beyond int64", physical: physicalMemory(math.MaxUint64, nil), wantErr: "exceeds supported range"},
		{name: "cancelled before probing", cancel: true, wantErr: "context canceled"},
		{name: "memory pressure probe refused", edit: func(_ map[string]string, failures map[string]bool) {
			failures["memory_pressure -Q"] = true
		}, wantErr: "estimate is unavailable"},
		{name: "memory pressure output unreadable", edit: func(outputs map[string]string, _ map[string]bool) {
			outputs["memory_pressure -Q"] = "no percentage\n"
		}, wantErr: "estimate is unavailable"},
		{name: "disk path absent", absent: true, wantErr: "read disk capacity"},
		{name: "CPU probe refused", edit: func(_ map[string]string, failures map[string]bool) {
			failures["ps -A -o %cpu="] = true
		}, inspect: func(t *testing.T, sample policy.Sample) {
			t.Helper()
			if sample.CPUUtilizationPercent != nil {
				t.Fatalf("CPU reading without its probe: %+v", sample.CPUUtilizationPercent)
			}
		}},
		{name: "vm statistics refused", edit: func(_ map[string]string, failures map[string]bool) {
			failures["vm_stat"] = true
		}, inspect: func(t *testing.T, sample policy.Sample) {
			t.Helper()
			if sample.PageSizeBytes != nil || sample.SwapIns != nil {
				t.Fatalf("compressor counters without vm_stat: %+v", sample)
			}
		}},
		{name: "vm statistics unreadable", edit: func(outputs map[string]string, _ map[string]bool) {
			outputs["vm_stat"] = "no counters\n"
		}, inspect: func(t *testing.T, sample policy.Sample) {
			t.Helper()
			if sample.PageSizeBytes != nil {
				t.Fatalf("compressor counters from unreadable vm_stat: %+v", sample)
			}
		}},
	}
}

func darwinSampleRows() []darwinRow {
	return []darwinRow{
		{name: "swap probe refused", edit: func(_ map[string]string, failures map[string]bool) {
			failures["sysctl -n vm.swapusage"] = true
		}, inspect: func(t *testing.T, sample policy.Sample) {
			t.Helper()
			if sample.SwapState != "unknown" || sample.SwapTotalBytes != nil {
				t.Fatalf("swap without its probe: %+v", sample)
			}
		}},
		{name: "swap output unreadable", edit: func(outputs map[string]string, _ map[string]bool) {
			outputs["sysctl -n vm.swapusage"] = "unknown\n"
		}, inspect: func(t *testing.T, sample policy.Sample) {
			t.Helper()
			if sample.SwapState != "unknown" {
				t.Fatalf("swap from unreadable output: %+v", sample)
			}
		}},
		{name: "no swap configured", edit: func(outputs map[string]string, _ map[string]bool) {
			outputs["sysctl -n vm.swapusage"] = "total = 0.00M  used = 0.00M  free = 0.00M\n"
		}, inspect: func(t *testing.T, sample policy.Sample) {
			t.Helper()
			if sample.SwapState != "unavailable" {
				t.Fatalf("empty swap reported %q", sample.SwapState)
			}
		}},
		{name: "idle swap", edit: func(outputs map[string]string, _ map[string]bool) {
			outputs["sysctl -n vm.swapusage"] = "total = 100.00M  used = 0.00M  free = 100.00M\n"
		}, inspect: func(t *testing.T, sample policy.Sample) {
			t.Helper()
			if sample.SwapState != "idle" {
				t.Fatalf("unused swap reported %q", sample.SwapState)
			}
		}},
		{name: "pressure level refused", edit: func(_ map[string]string, failures map[string]bool) {
			failures["sysctl -n kern.memorystatus_vm_pressure_level"] = true
		}, inspect: func(t *testing.T, sample policy.Sample) {
			t.Helper()
			if sample.MemoryPressureLevel != nil {
				t.Fatalf("pressure level without its probe: %d", *sample.MemoryPressureLevel)
			}
		}},
		{name: "pressure level unreadable", edit: func(outputs map[string]string, _ map[string]bool) {
			outputs["sysctl -n kern.memorystatus_vm_pressure_level"] = "high\n"
		}, inspect: func(t *testing.T, sample policy.Sample) {
			t.Helper()
			if sample.MemoryPressureLevel != nil {
				t.Fatalf("pressure level from unreadable output: %d", *sample.MemoryPressureLevel)
			}
		}},
		{name: "compressor availability out of range", edit: func(outputs map[string]string, _ map[string]bool) {
			outputs["sysctl -n vm.compressor_available"] = "2\n"
		}, inspect: func(t *testing.T, sample policy.Sample) {
			t.Helper()
			if sample.CompressorAvailable != nil {
				t.Fatalf("compressor availability %v from an out-of-range value", *sample.CompressorAvailable)
			}
		}},
		{name: "compressor disabled", edit: func(outputs map[string]string, _ map[string]bool) {
			outputs["sysctl -n vm.compressor_available"] = "0\n"
		}, inspect: func(t *testing.T, sample policy.Sample) {
			t.Helper()
			if sample.CompressorAvailable == nil || *sample.CompressorAvailable {
				t.Fatalf("disabled compressor reported %v", sample.CompressorAvailable)
			}
		}},
		{name: "compressor payload refused", edit: func(_ map[string]string, failures map[string]bool) {
			failures["sysctl -n vm.compressor_bytes_used"] = true
		}, inspect: func(t *testing.T, sample policy.Sample) {
			t.Helper()
			if sample.CompressorPayloadBytes != nil {
				t.Fatalf("compressor payload without its probe: %d", *sample.CompressorPayloadBytes)
			}
		}},
	}
}

func TestCollectDarwinFromCannedProbes(t *testing.T) {
	for _, test := range darwinRows() {
		t.Run(test.name, func(t *testing.T) {
			machine := systemtest.NewHost(time.Unix(0, 0))
			machine.Disk.CPUs = 4
			port := machine.Port()
			outputs, failures := darwinProbeTable(), map[string]bool{}
			if test.edit != nil {
				test.edit(outputs, failures)
			}

			collector := SystemCollector{CPUCount: port.Disk.CPUCount}
			if !test.noClock {
				collector.Now = port.Clock.Now
			}
			calls := darwinCalls{Physical: physicalMemory(uint64(darwinPhysicalBytes), nil), Command: cannedRunner(t, outputs, failures)}
			if test.physical != nil {
				calls.Physical = test.physical
			}
			if test.useRun {
				collector.Run = calls.Command
				calls.Command = neverRunner(t)
			}

			diskPath := t.TempDir()
			if test.absent {
				diskPath = filepath.Join(diskPath, "absent")
			}
			ctx := t.Context()
			if test.cancel {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}

			reading, err := collector.collectDarwin(ctx, nil, diskPath, calls)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error %v, want one naming %q", err, test.wantErr)
				}

				return
			}
			if err != nil {
				t.Fatal(err)
			}
			test.inspect(t, reading.Sample)
		})
	}
}
