package support

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func nativeTUIDevice(stat syscall.Stat_t) uint64 {
	return stat.Dev
}

func nativeTUIRawInput(descriptor int) (bool, error) {
	mode, err := unix.IoctlGetTermios(descriptor, unix.TCGETS)
	if err != nil {
		return false, err
	}
	return mode.Lflag&(syscall.ICANON|syscall.ECHO|syscall.ISIG) == 0, nil
}

func openNativeTUIPTY() (*os.File, *os.File, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, err
	}
	fail := func(cause error) (*os.File, *os.File, error) { return nil, nil, errors.Join(cause, master.Close()) }
	descriptor, err := nativeTUIDescriptor(master.Fd())
	if err != nil {
		return fail(err)
	}
	if err := unix.IoctlSetPointerInt(descriptor, unix.TIOCSPTLCK, 0); err != nil {
		return fail(err)
	}
	number, err := unix.IoctlGetUint32(descriptor, unix.TIOCGPTN)
	if err != nil {
		return fail(err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return fail(err)
	}
	return master, slave, nil
}

func nativeTUISession(pid int) (int, error) {
	return unix.Getsid(pid)
}

func publicTUITermios(descriptor int) (*unix.Termios, error) {
	return unix.IoctlGetTermios(descriptor, unix.TCGETS)
}

// Linux already reports a genuine slave inspection error after master hangup.
func prepareNativeTUIInspectionSlave(_, slave *os.File) (*os.File, error) {
	return slave, nil
}

// The original acquired writer stays open through this observation. Events=0
// requests no data readiness; Linux reports POLLERR when no readers remain.
func observePublicHostFIFOReaderAbsent(ctx context.Context, descriptor uintptr) (publicHostFIFOObservation, error) {
	number, err := nativeTUIDescriptor(descriptor)
	if err != nil {
		return publicHostFIFOObservation{}, err
	}
	return waitOriginalFIFOReaderAbsent(ctx, number, func(original, timeout int) (int16, error) {
		if original < 0 || original > math.MaxInt32 {
			return 0, errors.New("original FIFO descriptor exceeds signed pollfd bounds")
		}
		value := []unix.PollFd{{Fd: int32(original), Events: 0}}
		_, err := unix.Poll(value, timeout)
		return value[0].Revents, err
	})
}
