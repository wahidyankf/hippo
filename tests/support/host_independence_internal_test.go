package support

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wahidyankf/hippo/tests/contract"
)

// loadedHostProbes answer HIPPO's macOS host probes as a runner that is short
// of memory: a low free percentage and a memory pressure warning. They are the
// opposite of quietHostProbes, and a guarded run that samples the live host
// through PATH defers under them, as it does on a loaded runner.
var loadedHostProbes = map[string]string{
	darwinMemoryPressureTool: `if [ "$*" = "-Q" ]; then echo "System-wide memory free percentage: 6%"; exit 0; fi
exec /usr/bin/memory_pressure "$@"`,
	darwinSysctlTool: `if [ "$1" = "-n" ] && [ "$#" -eq 2 ] && [ "$2" = "kern.memorystatus_vm_pressure_level" ]; then
	echo 2; exit 0
fi
exec /usr/sbin/sysctl "$@"`,
}

// shadowLiveHostWithLoadedProbes places loadedHostProbes first on PATH for the test.
func shadowLiveHostWithLoadedProbes(t *testing.T) {
	t.Helper()
	directory := t.TempDir()
	for name, body := range loadedHostProbes {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// A scenario whose subject is a guarded child's usage, or the tier a run asks
// for, is not about host admission. Admission has to be decided by the
// scenario's own inputs, so a host short of memory cannot defer it: a runner
// that did deferred these scenarios on every commit after they were added.
func TestInProcessGuardedRunsAreAdmittedWhateverTheHostMemoryState(t *testing.T) {
	if runtime.GOOS != darwinPlatform {
		t.Skip("only the darwin collector resolves its host probes through PATH")
	}
	shadowLiveHostWithLoadedProbes(t)

	t.Run("a child usage run", func(t *testing.T) {
		driver := &Driver{mode: contract.Unit}
		t.Cleanup(driver.cleanup)
		if err := driver.emptyGuardedRunRoot(); err != nil {
			t.Fatal(err)
		}
		if err := driver.runGuardedChild("exit 0"); err != nil {
			t.Fatal(err)
		}
		if driver.exitCode != 0 {
			t.Fatalf("exit=%d stderr=%q, want an admitted run that exits 0", driver.exitCode, driver.errorOutput)
		}
	})

	t.Run("a tier run", func(t *testing.T) {
		driver := &Driver{mode: contract.Unit}
		t.Cleanup(driver.cleanup)
		if err := driver.stageTierConfiguration(""); err != nil {
			t.Fatal(err)
		}
		if err := driver.runTierRun(lightTierName, nil); err != nil {
			t.Fatal(err)
		}
		if driver.exitCode != 0 {
			t.Fatalf("exit=%d stderr=%q, want an admitted run that exits 0", driver.exitCode, driver.errorOutput)
		}
	})
}
