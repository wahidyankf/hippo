//go:build linux

package host

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
	"github.com/wahidyankf/hippo/internal/status"
)

const linuxMeminfo = "MemTotal: 16777216 kB\nMemAvailable: 8388608 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n"

// newLinuxMachine returns an in-memory host whose kernel files are complete and readable, so a test edits one entry.
func newLinuxMachine() *systemtest.Host {
	machine := systemtest.NewHost(time.Unix(0, 0))
	machine.Files.Contents["/proc/meminfo"] = []byte(linuxMeminfo)
	machine.Files.Contents["/proc/stat"] = []byte("cpu 10 0 10 80 0\n")
	machine.Files.Contents["/proc/self/cgroup"] = []byte("0::/\n")

	return machine
}

func linuxCollector(machine *systemtest.Host) SystemCollector {
	port := machine.Port()

	return SystemCollector{ReadFile: port.Files.ReadFile, Now: port.Clock.Now}
}

func TestLinuxCollectorReadsEachMemoryPressureLevel(t *testing.T) {
	for _, test := range []struct {
		name  string
		avg10 string
		level int
	}{
		{"low", "1.00", 1},
		{"moderate", "12.00", 2},
		{"severe", "30.00", 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			machine := newLinuxMachine()
			machine.Files.Contents["/proc/pressure/memory"] = []byte("some avg10=" + test.avg10 + " avg60=0.00 total=0\n")
			reading, err := linuxCollector(machine).Collect(t.Context(), nil, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if level := reading.Sample.MemoryPressureLevel; level == nil || *level != test.level {
				t.Fatalf("pressure level %v, want %d", level, test.level)
			}
		})
	}
}

func TestLinuxCollectorRefusesUnreadableCapacityEvidence(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(files map[string][]byte)
	}{
		{"meminfo absent", func(files map[string][]byte) { delete(files, "/proc/meminfo") }},
		{"meminfo invalid", func(files map[string][]byte) { files["/proc/meminfo"] = []byte("MemTotal: x kB\n") }},
		{"stat absent", func(files map[string][]byte) { delete(files, "/proc/stat") }},
		{"stat invalid", func(files map[string][]byte) { files["/proc/stat"] = []byte("cpu 1 2 3 4 x\n") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			machine := newLinuxMachine()
			test.edit(machine.Files.Contents)
			_, err := linuxCollector(machine).Collect(t.Context(), nil, t.TempDir())
			failure, classified := errors.AsType[status.Failure](err)
			if !classified || failure.Code != status.CodeHostUnreadable {
				t.Fatalf("%s reported %v, want %s", test.name, err, status.CodeHostUnreadable)
			}
		})
	}
}

func TestLinuxCollectorReportsAbsentDiskPath(t *testing.T) {
	_, err := linuxCollector(newLinuxMachine()).Collect(t.Context(), nil, filepath.Join(t.TempDir(), "absent"))
	failure, classified := errors.AsType[status.Failure](err)
	if !classified || failure.Code != status.CodeHostUnreadable {
		t.Fatalf("an absent measured path reported %v, want %s", err, status.CodeHostUnreadable)
	}
}

func TestLinuxCollectReturnsCancellationBeforeReading(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	collector := SystemCollector{ReadFile: func(string) ([]byte, error) {
		t.Error("a cancelled collection read the host")

		return nil, nil
	}}
	_, err := collector.collect(ctx, nil, t.TempDir())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled collection returned %v, want the cancellation", err)
	}
}

func TestRootedContextFileReaderMapsPathsBeneathRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "proc"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "proc", "stat"), []byte("cpu 1 2 3 4 5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := RootedContextFileReader(root)(t.Context(), "/../proc/stat")
	if err != nil || string(data) != "cpu 1 2 3 4 5\n" {
		t.Fatalf("rooted context read returned %q error=%v", data, err)
	}
}
