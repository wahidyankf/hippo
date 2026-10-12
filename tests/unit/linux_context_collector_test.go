//go:build linux

package unit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/wahidyankf/hippo/internal/adapters/host"
)

func TestLinuxOptionalCancellationStopsFurtherAcquisition(t *testing.T) {
	for _, stopAt := range []string{
		"/proc/self/cgroup", "/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory.high",
		"/sys/fs/cgroup/memory.current", "/sys/fs/cgroup/memory.stat", "/sys/fs/cgroup/cpu.max",
		"/sys/fs/cgroup/memory.pressure", "/proc/pressure/memory", "/sys/fs/cgroup/memory.events",
		"/sys/fs/cgroup/memory.swap.max", "/sys/fs/cgroup/memory.swap.current", "/proc/vmstat",
	} {
		t.Run(stopAt, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			stopped, callsAfter := false, 0
			collector := host.SystemCollector{ReadFileContext: func(selected context.Context, path string) ([]byte, error) {
				if selected != ctx {
					t.Error("collector replaced caller context")
				}
				if stopped {
					callsAfter++
				}
				if path == stopAt {
					stopped = true
					cancel()
					return []byte("discard"), nil
				}
				switch path {
				case "/proc/meminfo":
					return []byte("MemTotal: 16777216 kB\nMemAvailable: 8388608 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n"), nil
				case "/proc/stat":
					return []byte("cpu 10 0 10 80 0\n"), nil
				default:
					return nil, errors.New("optional evidence absent")
				}
			}}
			reading, err := collector.Collect(ctx, nil, "/")
			if !stopped || callsAfter != 0 || !errors.Is(err, context.Canceled) || reading.Sample.Platform != "" {
				t.Fatalf("optional cancellation: stopped=%t later=%d sample=%+v err=%v", stopped, callsAfter, reading.Sample, err)
			}
		})
	}
}
