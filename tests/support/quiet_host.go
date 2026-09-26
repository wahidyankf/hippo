package support

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// darwinPlatform is the only platform whose host probes a caller can replace.
const darwinPlatform = "darwin"

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
// The Linux collector reads /proc directly, which a caller cannot redirect, so
// there the environment is returned unchanged and the run still samples the
// live host.
func (driver *Driver) quietHostEnvironment(environment []string) ([]string, error) {
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
