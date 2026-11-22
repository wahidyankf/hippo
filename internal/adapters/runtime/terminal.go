package runtime

import (
	"os"
	"sync"
	"syscall"

	"github.com/wahidyankf/hippo/internal/adapters/system"
)

var terminalForegroundMutex sync.Mutex

type terminalForeground struct {
	file          *os.File
	originalGroup int
	locked        bool
}

func prepareTerminalForeground(port system.System, stdin any) *terminalForeground {
	file, ok := stdin.(*os.File)
	if !ok {
		return &terminalForeground{}
	}
	foregroundGroup, err := port.Terminal.ForegroundGroup(file)
	if err != nil {
		return &terminalForeground{}
	}
	if foregroundGroup != port.Processes.Getpgrp() {
		return &terminalForeground{}
	}

	terminalForegroundMutex.Lock()

	return &terminalForeground{file: file, originalGroup: foregroundGroup, locked: true}
}

// ForegroundProcessGroup reports the terminal foreground group for integration verification.
func ForegroundProcessGroup(file *os.File) (int, error) {
	return system.Production.Terminal.ForegroundGroup(file)
}

// CurrentProcessGroup reports HIPPO's process group for integration verification.
func CurrentProcessGroup() int { return system.Production.Processes.Getpgrp() }

func (terminal *terminalForeground) configure(attributes *syscall.SysProcAttr) {
	if terminal == nil || terminal.file == nil {
		return
	}
	attributes.Setpgid = true
	attributes.Foreground = true
	attributes.Ctty = int(terminal.file.Fd())
}

func (terminal *terminalForeground) restore(port system.System) error {
	if terminal == nil || !terminal.locked {
		return nil
	}
	defer func() {
		terminal.locked = false
		terminalForegroundMutex.Unlock()
	}()

	ignored := port.Signals.Ignored(syscall.SIGTTOU)
	if !ignored {
		port.Signals.Ignore(syscall.SIGTTOU)
		defer port.Signals.Reset(syscall.SIGTTOU)
	}

	return port.Terminal.SetForegroundGroup(terminal.file, terminal.originalGroup)
}
