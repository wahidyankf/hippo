package health_test

import (
	"context"
	"testing"
	"time"

	releaseguard "github.com/wahidyankf/hippo/internal/adapters/health"
	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
)

func TestLoadAverageReadsTheFirstFieldOfProcLoadavgThroughTheFileDouble(t *testing.T) {
	t.Parallel()

	host := systemtest.NewHost(time.Time{})
	host.Files.Contents["/proc/loadavg"] = []byte("0.52 0.58 0.59 1/1 1\n")

	probe := releaseguard.Probe{Host: host.Port()}
	if got := probe.LoadAverage(context.Background()); got != 0.52 {
		t.Fatalf("LoadAverage() = %v, want 0.52 from the /proc/loadavg double", got)
	}
}
