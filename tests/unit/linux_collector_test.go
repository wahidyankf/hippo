//go:build linux

package unit_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/host"
	"github.com/wahidyankf/hippo/internal/policy"
)

func TestLinuxCollectorUsesCgroupCapacityAndAllowsNoSwap(t *testing.T) {
	files := map[string]string{
		"/proc/meminfo":                                  "MemTotal: 16777216 kB\nMemAvailable: 8388608 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n",
		"/proc/stat":                                     "cpu 10 0 10 80 0\n",
		"/proc/self/cgroup":                              "0::/actions/job\n",
		"/sys/fs/cgroup/actions/job/memory.max":          "4294967296\n",
		"/sys/fs/cgroup/actions/job/memory.high":         "max\n",
		"/sys/fs/cgroup/actions/job/memory.current":      "1073741824\n",
		"/sys/fs/cgroup/actions/job/memory.swap.max":     "0\n",
		"/sys/fs/cgroup/actions/job/memory.swap.current": "0\n",
		"/sys/fs/cgroup/actions/job/cpu.max":             "200000 100000\n",
		"/sys/fs/cgroup/actions/job/memory.pressure":     "some avg10=0.00 avg60=0.00 total=0\nfull avg10=0.00 avg60=0.00 total=0\n",
		"/sys/fs/cgroup/actions/job/memory.events":       "oom 0\noom_kill 0\n",
		"/proc/vmstat":                                   "pswpin 3\npswpout 4\n",
	}

	read := func(path string) ([]byte, error) {
		value, exists := files[path]
		if !exists {
			return nil, errors.New("fixture path is unavailable")
		}

		return []byte(value), nil
	}

	collector := host.SystemCollector{
		ReadFile: read,
		Now:      func() time.Time { return time.Unix(0, 0) },
	}
	reading, err := collector.Collect(context.Background(), nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if reading.Sample.Platform != "linux" ||
		reading.Sample.EffectiveMemoryLimitBytes != 4*policy.GiB ||
		reading.Sample.AvailableMemoryBytes == nil ||
		*reading.Sample.AvailableMemoryBytes != 3*policy.GiB ||
		reading.Sample.AvailableParallelism != 2 ||
		reading.Sample.SwapState != "unavailable" {
		t.Fatalf("unexpected Linux sample %+v", reading.Sample)
	}

	files["/proc/stat"] = "cpu 20 0 20 140 0\n"
	second, err := collector.Collect(context.Background(), reading.CPUState, t.TempDir())

	if err != nil || second.Sample.CPUUtilizationPercent == nil || *second.Sample.CPUUtilizationPercent != 25 {
		t.Fatalf("unexpected Linux CPU sample %+v error=%v", second.Sample, err)
	}
}

func TestLinuxCollectorExcludesInactiveFileCacheFromCgroupUsage(t *testing.T) {
	// The page-cache defect's container reproduction: a 6 GiB cgroup filled by
	// a file read back, on a host with far more memory available.
	files := map[string]string{
		"/proc/meminfo":                 "MemTotal: 25165824 kB\nMemAvailable: 23802675 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n",
		"/proc/stat":                    "cpu 10 0 10 80 0\n",
		"/proc/self/cgroup":             "0::/\n",
		"/sys/fs/cgroup/memory.max":     "6442450944\n",
		"/sys/fs/cgroup/memory.high":    "max\n",
		"/sys/fs/cgroup/memory.current": "6441140224\n",
		"/sys/fs/cgroup/memory.stat":    "anon 122880\nfile 6438256640\nactive_file 0\ninactive_file 6438256640\n",
	}
	read := func(path string) ([]byte, error) {
		value, exists := files[path]
		if !exists {
			return nil, errors.New("fixture path is unavailable")
		}

		return []byte(value), nil
	}

	collector := host.SystemCollector{ReadFile: read, Now: func() time.Time { return time.Unix(0, 0) }}
	reading, err := collector.Collect(context.Background(), nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	available := reading.Sample.AvailableMemoryBytes
	if available == nil {
		t.Fatalf("no Linux available memory in %+v", reading.Sample)
	}
	// Only the 2883584 bytes outside the inactive file cache count as used.
	if want := 6*policy.GiB - 2883584; *available != want {
		t.Fatalf("Linux available memory %d, want %d", *available, want)
	}
}
