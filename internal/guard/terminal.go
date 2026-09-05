package guard

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

var terminalForegroundMutex sync.Mutex

type terminalForeground struct {
	file          *os.File
	originalGroup int
	locked        bool
}

func prepareTerminalForeground(stdin any) *terminalForeground {
	file, ok := stdin.(*os.File)
	if !ok {
		return &terminalForeground{}
	}
	foregroundGroup, err := unix.IoctlGetInt(int(file.Fd()), unix.TIOCGPGRP)
	if err != nil {
		return &terminalForeground{}
	}
	if foregroundGroup != unix.Getpgrp() {
		return &terminalForeground{}
	}

	terminalForegroundMutex.Lock()

	return &terminalForeground{file: file, originalGroup: foregroundGroup, locked: true}
}

// ForegroundProcessGroup reports the terminal foreground group for integration verification.
func ForegroundProcessGroup(file *os.File) (int, error) {
	return unix.IoctlGetInt(int(file.Fd()), unix.TIOCGPGRP)
}

// CurrentProcessGroup reports HIPPO's process group for integration verification.
func CurrentProcessGroup() int { return unix.Getpgrp() }

func (terminal *terminalForeground) configure(attributes *syscall.SysProcAttr) {
	if terminal == nil || terminal.file == nil {
		return
	}
	attributes.Setpgid = true
	attributes.Foreground = true
	attributes.Ctty = int(terminal.file.Fd())
}

func (terminal *terminalForeground) restore() error {
	if terminal == nil || !terminal.locked {
		return nil
	}
	defer func() {
		terminal.locked = false
		terminalForegroundMutex.Unlock()
	}()

	ignored := signal.Ignored(syscall.SIGTTOU)
	if !ignored {
		signal.Ignore(syscall.SIGTTOU)
		defer signal.Reset(syscall.SIGTTOU)
	}

	return unix.IoctlSetPointerInt(int(terminal.file.Fd()), unix.TIOCSPGRP, terminal.originalGroup)
}
