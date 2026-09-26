package support

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/wahidyankf/hippo/internal/policy"
)

// darwinPlatform is the platform whose host probes a caller replaces
// through PATH.
const darwinPlatform = "darwin"

// linuxPlatform is the platform whose kernel evidence a test build of HIPPO
// reads from a fixed root.
const linuxPlatform = "linux"

// linuxEvidenceRootEnvironment is the variable a test build of HIPPO, and
// only a test build, reads its Linux kernel evidence root from.
const linuxEvidenceRootEnvironment = "HIPPO_TEST_LINUX_EVIDENCE_ROOT"

// The quiet Linux host's memory: ample for every profile, with no pressure.
const (
	quietLinuxTotalBytes     = 16 * policy.GiB
	quietLinuxAvailableBytes = 12 * policy.GiB
)

// quietLinuxEvidence answers the Linux collector's static reads, relative to
// the evidence root, as an idle host with ample memory, idle swap, and no
// memory pressure. /proc/stat is served by serveIdleCPU instead, because an
// idle CPU is a delta between two reads.
var quietLinuxEvidence = map[string]string{
	"proc/meminfo": fmt.Sprintf("MemTotal: %d kB\nMemAvailable: %d kB\nSwapTotal: 1048576 kB\nSwapFree: 1048576 kB\n",
		quietLinuxTotalBytes/1024, quietLinuxAvailableBytes/1024),
	"proc/self/cgroup":     "0::/\n",
	"proc/pressure/memory": "some avg10=0.00 avg60=0.00 avg300=0.00 total=0\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=0\n",
	"proc/vmstat":          "pswpin 0\npswpout 0\n",
}

// quietHostProbes answer HIPPO's macOS host probes as an idle host with ample
// memory and no swap or compressor activity. The darwin collector runs each of
// them by name through PATH, so a caller that places these first on PATH
// decides the host evidence a compiled run samples. Any query the collector
// does not make passes through to the real tool.
var quietHostProbes = map[string]string{
	"ps": `if [ "$*" = "-A -o %cpu=" ]; then echo 0.0; exit 0; fi
exec /bin/ps "$@"`,
	"memory_pressure": `if [ "$*" = "-Q" ]; then echo "System-wide memory free percentage: 80%"; exit 0; fi
exec /usr/bin/memory_pressure "$@"`,
	"vm_stat": `printf '%s\n' "Mach Virtual Memory Statistics: (page size of 16384 bytes)" \
	"Pages stored in compressor: 0." "Pages occupied by compressor: 0." "Swapins: 0." "Swapouts: 0."`,
	"sysctl": `if [ "$1" = "-n" ] && [ "$#" -eq 2 ]; then
	case "$2" in
	vm.swapusage) echo "total = 1024.00M  used = 0.00M  free = 1024.00M  (encrypted)"; exit 0 ;;
	kern.memorystatus_vm_pressure_level) echo 1; exit 0 ;;
	vm.compressor_available) echo 1; exit 0 ;;
	vm.compressor_bytes_used) echo 0; exit 0 ;;
	esac
fi
exec /usr/sbin/sysctl "$@"`,
}

// quietHostEnvironment returns environment with quiet host probes first on
// PATH, for a compiled run whose subject is not host admission. HIPPO defers
// a run until consecutive samples look safe, so a scenario that samples the
// runner's live load can fail for the runner's reasons: a busy macOS runner
// deferred both the terminal and the schema-five summary scenarios. With the
// evidence fixed, admission is decided by the scenario's own inputs.
//
// On Linux the collector reads kernel files, which a caller cannot shadow
// through PATH, so a test build instead reads them beneath the evidence root
// this publishes. Other platforms keep the environment unchanged.
func (driver *Driver) quietHostEnvironment(environment []string) ([]string, error) {
	if runtime.GOOS == linuxPlatform {
		return driver.quietLinuxEnvironment(environment)
	}
	if runtime.GOOS != darwinPlatform {
		return environment, nil
	}
	directory, err := driver.temporaryRoot()
	if err != nil {
		return nil, err
	}
	for name, body := range quietHostProbes {
		if err = os.WriteFile(filepath.Join(directory, name), []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
			return nil, err
		}
	}

	path := os.Getenv("PATH")
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if value, found := strings.CutPrefix(entry, "PATH="); found {
			path = value

			continue
		}
		result = append(result, entry)
	}

	return append(result, "PATH="+directory+string(os.PathListSeparator)+path), nil
}

// quietLinuxEnvironment publishes a quiet Linux evidence root and names it in
// environment for a test build of HIPPO. The root lives until the scenario is
// cleaned up.
func (driver *Driver) quietLinuxEnvironment(environment []string) ([]string, error) {
	root, err := driver.temporaryRoot()
	if err != nil {
		return nil, err
	}
	for name, body := range quietLinuxEvidence {
		path := filepath.Join(root, name)
		if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
		if err = os.WriteFile(path, []byte(body), 0o600); err != nil {
			return nil, err
		}
	}
	stop, err := serveIdleCPU(filepath.Join(root, "proc", "stat"))
	if err != nil {
		return nil, err
	}
	driver.stops = append(driver.stops, stop)

	return append(environment, linuxEvidenceRootEnvironment+"="+root), nil
}

// serveIdleCPU publishes path as a FIFO that answers each read with CPU
// counters in which only idle time has advanced, so every sample after the
// first reports an idle CPU. It is synchronised on the reads themselves: the
// counters advance once per read, however slowly the host schedules either
// side, so no rewrite interval can leave two samples reading the same counters.
// The returned stop ends the server; the reader must be gone by then.
func serveIdleCPU(path string) (func(), error) {
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		return nil, err
	}
	stopping := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for idle := uint64(1_000); ; idle += 100 {
			// Opening the write side blocks until a reader opens the FIFO.
			writer, err := os.OpenFile(path, os.O_WRONLY, 0)
			if err != nil {
				return
			}
			select {
			case <-stopping:
				_ = writer.Close()

				return
			default:
			}
			_, _ = fmt.Fprintf(writer, "cpu  0 0 0 %d 0 0 0 0 0 0\n", idle)
			_ = writer.Close()
		}
	}()

	return func() {
		close(stopping)
		// A server blocked opening the write side waits for a reader. This one
		// releases it without blocking, and stays open until the server has
		// seen the stop, so the server cannot block again.
		release, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		<-finished
		if err == nil {
			_ = release.Close()
		}
	}, nil
}
