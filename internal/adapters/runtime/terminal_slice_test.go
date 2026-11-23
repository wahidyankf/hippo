package runtime //nolint:testpackage // Slice (b-rest) doubles run inside the production package, as run_test.go does.

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
)

var errTerminalSliceBoom = errors.New("systemtest: terminal failure")

func terminalSliceSystem() (*systemtest.Host, system.System) {
	host := systemtest.NewHost(time.Unix(1700000000, 0).UTC())
	host.Processes.Pid = 100
	host.Processes.Groups[100] = 7

	return host, host.Port()
}

func terminalSliceFile(t *testing.T) *os.File {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "terminal-*.lock")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })

	return file
}

func TestPrepareTerminalForegroundLocksTheMatchingGroup(t *testing.T) {
	host, port := terminalSliceSystem()
	file := terminalSliceFile(t)
	host.Terminal.Groups[file.Name()] = 7
	terminal := prepareTerminalForeground(port, file)
	if !terminal.locked || terminal.file != file || terminal.originalGroup != 7 {
		t.Fatalf("foreground = %+v, want the locked file at group 7", terminal)
	}
	t.Cleanup(func() { _ = terminal.restore(port) })
}

func TestPrepareTerminalForegroundRefusesAGroupMismatch(t *testing.T) {
	host, port := terminalSliceSystem()
	file := terminalSliceFile(t)
	host.Terminal.Groups[file.Name()] = 8
	terminal := prepareTerminalForeground(port, file)
	if terminal.locked {
		_ = terminal.restore(port)
		t.Fatal("foreground mismatch locked, want unlocked")
	}
}

func TestPrepareTerminalForegroundReportsDoubleFailure(t *testing.T) {
	host, port := terminalSliceSystem()
	host.Terminal.Groups[terminalSliceFile(t).Name()] = 7
	host.Terminal.ForegroundErr = errTerminalSliceBoom
	terminal := prepareTerminalForeground(port, terminalSliceFile(t))
	if terminal.locked {
		_ = terminal.restore(port)
		t.Fatal("foreground failure locked, want unlocked")
	}
}

func TestPrepareTerminalForegroundRefusesANonFile(t *testing.T) {
	_, port := terminalSliceSystem()
	if terminal := prepareTerminalForeground(port, "not-a-file"); terminal.locked {
		t.Fatal("non-file stdin locked, want unlocked")
	}
}

func TestTerminalConfigurePlacesTheChildInTheForeground(t *testing.T) {
	file := terminalSliceFile(t)
	terminal := &terminalForeground{file: file, originalGroup: 7, locked: true}
	attributes := &syscall.SysProcAttr{}
	terminal.configure(attributes)
	if !attributes.Setpgid || !attributes.Foreground {
		t.Error("child was not placed in its own foreground group")
	}
	if attributes.Ctty != int(file.Fd()) {
		t.Errorf("controlling terminal = %d, want fd %d", attributes.Ctty, file.Fd())
	}
	var nilTerminal *terminalForeground
	nilTerminal.configure(&syscall.SysProcAttr{})
	(&terminalForeground{}).configure(&syscall.SysProcAttr{})
}

func TestTerminalRestoreReleasesTheLockAndTheGroup(t *testing.T) {
	host, port := terminalSliceSystem()
	file := terminalSliceFile(t)
	host.Terminal.Groups[file.Name()] = 7
	terminal := prepareTerminalForeground(port, file)
	if err := terminal.restore(port); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if terminal.locked {
		t.Error("restore left the mutex locked")
	}
	placed := host.Terminal.Set
	if len(placed) != 1 || placed[0].Name != file.Name() || placed[0].Group != 7 {
		t.Fatalf("foreground placements = %+v, want one restoring group 7", placed)
	}
	if len(host.Signals.Ignored) != 1 || host.Signals.Ignored[0] != syscall.SIGTTOU {
		t.Fatalf("ignored signals = %v, want one SIGTTOU", host.Signals.Ignored)
	}
	if len(host.Signals.Restored) != 1 || host.Signals.Restored[0] != syscall.SIGTTOU {
		t.Fatalf("restored signals = %v, want one SIGTTOU", host.Signals.Restored)
	}
}

func TestTerminalRestoreKeepsAnIgnoredSignalAlone(t *testing.T) {
	host, port := terminalSliceSystem()
	host.Signals.Ignored = []os.Signal{syscall.SIGTTOU}
	file := terminalSliceFile(t)
	host.Terminal.Groups[file.Name()] = 7
	terminal := prepareTerminalForeground(port, file)
	if err := terminal.restore(port); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if len(host.Signals.Restored) != 0 {
		t.Fatalf("restored signals = %v, want none for an ignored SIGTTOU", host.Signals.Restored)
	}
	if len(host.Terminal.Set) != 1 {
		t.Fatalf("foreground placements = %+v, want one", host.Terminal.Set)
	}
}

func TestTerminalRestoreReportsDoubleFailure(t *testing.T) {
	host, port := terminalSliceSystem()
	host.Terminal.SetErr = errTerminalSliceBoom
	file := terminalSliceFile(t)
	host.Terminal.Groups[file.Name()] = 7
	terminal := prepareTerminalForeground(port, file)
	if err := terminal.restore(port); err == nil {
		t.Fatal("restore with double failure = nil, want error")
	}
	if terminal.locked {
		t.Error("failed restore left the mutex locked")
	}
}

func TestTerminalRestoreWithoutALockIsANoOp(t *testing.T) {
	host, port := terminalSliceSystem()
	if err := (&terminalForeground{}).restore(port); err != nil {
		t.Fatalf("restore without a lock: %v", err)
	}
	var nilTerminal *terminalForeground
	if err := nilTerminal.restore(port); err != nil {
		t.Fatalf("restore on nil: %v", err)
	}
	if len(host.Terminal.Set) != 0 || len(host.Signals.Ignored) != 0 {
		t.Fatal("restore without a lock touched the doubles")
	}
}
