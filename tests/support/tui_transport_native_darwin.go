package support

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Each caller supplies a fixed payload matching its platform ioctl. The kernel
// uses the pointer synchronously; KeepAlive retains it through the entire call.
func nativeTUIIoctl(descriptor, command uintptr, payload unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, descriptor, command, uintptr(payload))
	runtime.KeepAlive(payload)
	if errno != 0 {
		return errno
	}
	return nil
}

func nativeTUIDevice(stat syscall.Stat_t) uint64 {
	return nativeTUISignedDevice(stat.Dev)
}

func nativeTUIRawInput(descriptor int) (bool, error) {
	mode, err := unix.IoctlGetTermios(descriptor, unix.TIOCGETA)
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
	if err := unix.IoctlSetInt(descriptor, unix.TIOCPTYGRANT, 0); err != nil {
		return fail(err)
	}
	if err := unix.IoctlSetInt(descriptor, unix.TIOCPTYUNLK, 0); err != nil {
		return fail(err)
	}
	name, err := nativeTUIPTYSlaveName(master)
	if err != nil {
		return fail(err)
	}
	slave, err := os.OpenFile(name, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return fail(err)
	}
	return master, slave, nil
}

func nativeTUISession(pid int) (int, error) {
	return syscall.Getsid(pid)
}

func publicTUITermios(descriptor int) (*unix.Termios, error) {
	return unix.IoctlGetTermios(descriptor, unix.TIOCGETA)
}

// nativeTUIPTYSlaveName reads the same fixed Darwin ABI payload used by allocation.
func nativeTUIPTYSlaveName(master *os.File) (string, error) {
	var name [128]byte
	// TIOCPTYGNAME copies exactly the SDK-declared 128-byte payload into name.
	// The synchronous ioctl retains no pointer; KeepAlive covers the call and
	// the NUL check below rejects empty or unterminated names before use.
	// #nosec G103 -- No typed name getter exists; the fixed payload matches the Darwin ioctl ABI.
	if err := nativeTUIIoctl(master.Fd(), syscall.TIOCPTYGNAME, unsafe.Pointer(&name[0])); err != nil {
		return "", err
	}
	end := 0
	for end < len(name) && name[end] != 0 {
		end++
	}
	if end == 0 || end == len(name) {
		return "", errors.New("native TUI slave pathname is unavailable")
	}
	return string(name[:end]), nil
}

// Preparation owns only its anonymous socket; the caller retains both PTY descriptors.
func prepareNativeTUIInspectionSlave(master, slave *os.File) (*os.File, error) {
	original, err := nativeTUIInspectionSlaveIdentity(master, slave)
	if err != nil {
		return nil, err
	}
	input, err := nativeTUIInspectionSocketInput()
	if err != nil {
		return nil, err
	}
	current, err := nativeTUIInspectionSlaveIdentity(master, slave)
	if err != nil {
		return nil, errors.Join(err, input.Close())
	}
	if !os.SameFile(original, current) {
		return nil, errors.Join(errors.New("inspection acquisition refuses a replaced original slave"), input.Close())
	}
	return input, nil
}

// Socketpair creates only anonymous local endpoints; no path, bind or connect is used.
// ForkLock excludes descriptor inheritance until both checked CLOEXEC flags are installed.
func nativeTUIInspectionSocketInput() (*os.File, error) {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	pair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		return nil, err
	}
	fail := func(cause error) (*os.File, error) {
		for index, descriptor := range pair {
			if descriptor >= 0 && (index == 0 || descriptor != pair[0]) {
				cause = errors.Join(cause, unix.Close(descriptor))
			}
		}
		return nil, cause
	}
	if pair[0] < 0 || pair[1] < 0 || pair[0] == pair[1] {
		return fail(errors.New("inspection socketpair lacks two distinct valid descriptors"))
	}
	for _, descriptor := range pair {
		if _, err := unix.FcntlInt(uintptr(descriptor), unix.F_SETFD, unix.FD_CLOEXEC); err != nil {
			return fail(err)
		}
		flags, err := unix.FcntlInt(uintptr(descriptor), unix.F_GETFD, 0)
		if err != nil {
			return fail(err)
		}
		if flags&unix.FD_CLOEXEC == 0 {
			return fail(errors.New("inspection socket lacks close-on-exec ownership"))
		}
	}
	input := os.NewFile(uintptr(pair[0]), "hippo-inspection-anonymous-input")
	if input == nil {
		return fail(errors.New("inspection socket descriptor is unavailable"))
	}
	if err := unix.Close(pair[1]); err != nil {
		return nil, errors.Join(err, input.Close())
	}
	return input, nil
}

func nativeTUIInspectionSlaveIdentity(master, slave *os.File) (os.FileInfo, error) {
	if master == nil || slave == nil {
		return nil, errors.New("inspection acquisition requires the acquired PTY pair")
	}
	allocated, err := nativeTUIPTYSlaveName(master)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(allocated) || filepath.Clean(allocated) != allocated || slave.Name() != allocated {
		return nil, errors.New("inspection slave does not match its allocated master")
	}
	opened, err := slave.Stat()
	if err != nil {
		return nil, err
	}
	path, err := os.Lstat(allocated)
	if err != nil {
		return nil, err
	}
	owner, owned := opened.Sys().(*syscall.Stat_t)
	named, identified := path.Sys().(*syscall.Stat_t)
	if !owned || owner == nil || !identified || named == nil ||
		int64(owner.Uid) != int64(os.Geteuid()) || named.Uid != owner.Uid || named.Rdev != owner.Rdev ||
		opened.Mode()&os.ModeCharDevice == 0 || path.Mode()&os.ModeCharDevice == 0 ||
		!os.SameFile(opened, path) {
		return nil, errors.New("inspection acquisition refuses an unowned or replaced slave device")
	}
	return opened, nil
}
