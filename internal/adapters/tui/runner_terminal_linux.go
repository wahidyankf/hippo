package tui

import "golang.org/x/sys/unix"

func runnerGetTermios(descriptorNumber int) (*unix.Termios, error) {
	return unix.IoctlGetTermios(descriptorNumber, unix.TCGETS)
}

func runnerSetTermios(descriptorNumber int, mode *unix.Termios) error {
	return unix.IoctlSetTermios(descriptorNumber, unix.TCSETS, mode)
}
