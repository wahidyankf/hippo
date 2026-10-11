package tui

import (
	"errors"
	"io"

	"golang.org/x/sys/unix"
)

type runnerDescriptor interface{ Fd() uintptr }

type runnerTerminalState struct {
	descriptorNumber int
	mode             unix.Termios
	foreground       int
}

type runnerTerminal struct{ states []runnerTerminalState }

// TerminalCapabilities inspects only the supplied stream descriptors; it opens no terminal.
func TerminalCapabilities(input io.Reader, output io.Writer) (bool, bool, error) {
	inputTTY, err := runnerIsTerminal(input)
	if err != nil {
		return false, false, err
	}
	outputTTY, err := runnerIsTerminal(output)
	return inputTTY, outputTTY, err
}

func runnerIsTerminal(stream any) (bool, error) {
	descriptor, ok := stream.(runnerDescriptor)
	if !ok {
		return false, nil
	}
	_, err := runnerGetTermios(int(descriptor.Fd()))
	return runnerTerminalCapabilityResult(err)
}

func runnerTerminalCapabilityResult(err error) (bool, error) {
	if errors.Is(err, unix.ENOTTY) || errors.Is(err, unix.ENODEV) {
		return false, nil
	}
	return err == nil, err
}

func captureRunnerTerminal(input io.Reader, output io.Writer) (runnerTerminal, error) {
	terminal := runnerTerminal{}
	for _, stream := range []any{input, output} {
		descriptor, ok := stream.(runnerDescriptor)
		if !ok {
			return terminal, errors.New("terminal descriptor unavailable")
		}
		descriptorNumber := int(descriptor.Fd())
		mode, err := runnerGetTermios(descriptorNumber)
		if err != nil {
			return terminal, err
		}
		foreground, err := unix.IoctlGetInt(descriptorNumber, unix.TIOCGPGRP)
		if err != nil {
			return terminal, err
		}
		terminal.states = append(terminal.states, runnerTerminalState{descriptorNumber: descriptorNumber, mode: *mode, foreground: foreground})
	}
	return terminal, nil
}

type runnerTerminalOperations struct {
	getMode       func(int) (*unix.Termios, error)
	setMode       func(int, *unix.Termios) error
	getForeground func(int) (int, error)
	setForeground func(int, int) error
}

func (terminal runnerTerminal) restore() error {
	return terminal.restoreWithOperations(runnerTerminalOperations{
		getMode: runnerGetTermios,
		setMode: runnerSetTermios,
		getForeground: func(descriptorNumber int) (int, error) {
			return unix.IoctlGetInt(descriptorNumber, unix.TIOCGPGRP)
		},
		setForeground: func(descriptorNumber, foreground int) error {
			return unix.IoctlSetPointerInt(descriptorNumber, unix.TIOCSPGRP, foreground)
		},
	})
}

func (terminal runnerTerminal) restoreWithOperations(operations runnerTerminalOperations) error {
	var failures []error
	for _, state := range terminal.states {
		if err := operations.setMode(state.descriptorNumber, &state.mode); err != nil {
			failures = append(failures, err)
		}
		foreground, err := operations.getForeground(state.descriptorNumber)
		if err != nil {
			failures = append(failures, err)
		} else if foreground != state.foreground {
			if err := operations.setForeground(state.descriptorNumber, state.foreground); err != nil {
				failures = append(failures, err)
			}
		}
	}
	// Descriptors can share terminal state; verify every snapshot after all mutations.
	for _, state := range terminal.states {
		mode, err := operations.getMode(state.descriptorNumber)
		if err != nil {
			failures = append(failures, err)
		} else if *mode != state.mode {
			failures = append(failures, errors.New("terminal modes differ"))
		}
		foreground, err := operations.getForeground(state.descriptorNumber)
		if err != nil {
			failures = append(failures, err)
		} else if foreground != state.foreground {
			failures = append(failures, errors.New("terminal foreground differs"))
		}
	}
	return errors.Join(failures...)
}
