package tui

import "golang.org/x/sys/unix"

// Darwin <sys/filio.h> defines FIONREAD as _IOR('f', 127, int).
// x/sys does not export this request on Darwin.
const runnerQueuedInput = 0x4004667f

func runnerGetTermios(descriptorNumber int) (*unix.Termios, error) {
	return unix.IoctlGetTermios(descriptorNumber, unix.TIOCGETA)
}

func runnerSetTermios(descriptorNumber int, mode *unix.Termios) error {
	if err := unix.IoctlSetTermios(descriptorNumber, unix.TIOCSETA, mode); err != nil {
		return err
	}
	// Darwin marks queued input pending when canonical mode is restored.
	// Querying its size reprocesses it under the restored line discipline;
	// this performs no userspace read or explicit input flush.
	if mode.Lflag&unix.PENDIN == 0 {
		_, err := unix.IoctlGetInt(descriptorNumber, runnerQueuedInput)
		return err
	}
	return nil
}
