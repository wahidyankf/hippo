package unit_test

import (
	"math"
	"testing"

	"github.com/wahidyankf/resource-guard/internal/guard"
	"github.com/wahidyankf/resource-guard/internal/host"
)

func TestMacOSMetricParsers(t *testing.T) {
	if value := host.ParseAvailableEstimate("System-wide memory free percentage: 65%", 32*guard.GiB); value == nil ||
		*value != 20*guard.GiB+guard.GiB*8/10 {
		t.Fatalf("unexpected estimate %v", value)
	}

	for _, text := range []string{"missing", "System-wide memory free percentage: 101%"} {
		if host.ParseAvailableEstimate(text, 32*guard.GiB) != nil {
			t.Fatalf("accepted %q", text)
		}
	}
	if host.ParseAvailableEstimate("System-wide memory free percentage: 999999999999999999999999999999999999%", 32*guard.GiB) != nil {
		t.Fatal("overflowing memory percentage accepted")
	}

	vmStats := host.ParseVMStat("Mach Virtual Memory Statistics: (page size of 16384 bytes)\nPages stored in compressor: 10.\nPages occupied by compressor: 5.\nSwapins: 3.\nSwapouts: 4.\n")
	if vmStats == nil ||
		vmStats.PageSizeBytes != 16_384 ||
		vmStats.CompressorStoredPages != 10 ||
		vmStats.CompressorOccupiedPages != 5 ||
		vmStats.SwapIns != 3 ||
		vmStats.SwapOuts != 4 {
		t.Fatalf("unexpected vm stat %+v", vmStats)
	}
	if host.ParseVMStat("missing") != nil || host.ParseVMStat("page size of bad bytes") != nil || host.ParseVMStat("page size of 999999999999999999999999 bytes") != nil {
		t.Fatal("malformed VM stat accepted")
	}
	if host.ParseVMStat("page size of 4096 bytes\nSwapouts: 1.\n") != nil {
		t.Fatal("VM stat with missing counters accepted")
	}

	invalidVM := "page size of 4096 bytes\nPages stored in compressor: 999999999999999999999999.\nPages occupied by compressor: 1.\nSwapins: 1.\nSwapouts: 1.\n"
	if host.ParseVMStat(invalidVM) != nil {
		t.Fatal("overflowing VM counter accepted")
	}

	swap := host.ParseSwapUsage("total = 4096.00M  used = 2562.38M  free = 1533.62M")
	if swap == nil || swap.Total != 4096*guard.MiB || math.Abs(float64(swap.Used)-2562.38*float64(guard.MiB)) > 1 {
		t.Fatalf("unexpected swap %+v", swap)
	}
	if host.ParseSwapUsage("missing") != nil || host.ParseSwapUsage("total = badM used = 1M free = 1M") != nil || host.ParseSwapUsage("total = 999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999M used = 1M free = 1M") != nil {
		t.Fatal("malformed swap accepted")
	}
}

func TestCPUParsersAndEvidenceRoot(t *testing.T) {
	if value := host.CPUUtilization(guard.CPUState{10, 0, 10, 70, 10}, guard.CPUState{20, 0, 20, 140, 20}); value == nil || *value != 30 {
		t.Fatalf("unexpected CPU %v", value)
	}

	for _, pair := range [][2]guard.CPUState{{nil, {1, 2, 3, 4}}, {{2, 0, 0, 0}, {1, 0, 0, 0}}, {{1, 1, 1, 1}, {1, 1, 1, 1}}} {
		if host.CPUUtilization(pair[0], pair[1]) != nil {
			t.Fatal("invalid CPU counters accepted")
		}
	}

	if value := host.ParseProcessCPU("120.0 60.0 0.0", 12); value == nil || *value != 15 {
		t.Fatalf("unexpected process CPU %v", value)
	}
	if host.ParseProcessCPU("bad", 12) != nil || host.ParseProcessCPU("1", 0) != nil || host.ParseProcessCPU("-1", 4) != nil {
		t.Fatal("invalid process CPU accepted")
	}
	if value := host.ParseProcessCPU("", 4); value == nil || *value != 0 {
		t.Fatalf("unexpected empty process CPU %v", value)
	}

	if root := host.DefaultEvidenceRoot(map[string]string{"RESOURCE_GUARD_ROOT": "/generic/root"}); root != "/generic/root" {
		t.Fatalf("unexpected root %q", root)
	}
}
