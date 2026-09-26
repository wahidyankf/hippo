package support

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/host"
	"github.com/wahidyankf/hippo/internal/policy"
)

// The e2e fixtures rely on the collector resolving its probes through PATH. If
// it stopped doing so, those scenarios would silently sample the live runner
// again, so this pins the evidence the collector reads through the probes.
func TestQuietHostProbesDecideCollectedEvidence(t *testing.T) {
	if runtime.GOOS != darwinPlatform {
		t.Skip("only the darwin collector resolves its host probes through PATH")
	}
	driver := &Driver{}
	t.Cleanup(driver.cleanup)
	environment, err := driver.quietHostEnvironment(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range environment {
		if path, found := strings.CutPrefix(entry, "PATH="); found {
			t.Setenv("PATH", path)
		}
	}

	reading, err := host.SystemCollector{}.Collect(context.Background(), policy.CPUState{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sample := reading.Sample
	if sample.CPUUtilizationPercent == nil || *sample.CPUUtilizationPercent != 0 ||
		sample.MemoryPressureLevel == nil || *sample.MemoryPressureLevel != 1 ||
		sample.AvailableMemoryBytes == nil || *sample.AvailableMemoryBytes != sample.PhysicalMemoryBytes*80/100 ||
		sample.CompressorPayloadBytes == nil || *sample.CompressorPayloadBytes != 0 ||
		sample.SwapOuts == nil || *sample.SwapOuts != 0 || sample.SwapState != "idle" {
		t.Fatalf("collector did not read the quiet host probes: %+v", sample)
	}
}
