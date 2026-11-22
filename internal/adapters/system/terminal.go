package system

import (
	"os"

	"golang.org/x/sys/unix"
)

// foregroundGroup reports the terminal foreground process group of file. It is
// the production binding behind the terminal port: the only place that touches
// the kernel for foreground queries.
func foregroundGroup(file *os.File) (int, error) {
	return unix.IoctlGetInt(int(file.Fd()), unix.TIOCGPGRP)
}

// setForegroundGroup places group in the foreground of file's terminal. It is
// the production binding behind the terminal port.
func setForegroundGroup(file *os.File, group int) error {
	return unix.IoctlSetPointerInt(int(file.Fd()), unix.TIOCSPGRP, group)
}
