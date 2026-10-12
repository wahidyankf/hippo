//go:build linux

package integration_test

import (
	"errors"
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"
)

type hostReaderIdentity struct {
	descriptor int
	metadata   syscall.Stat_t
}

func contextReaderBaseline(_ *testing.T) map[int]bool { return nil }

// Fstat compares metadata of our unique FIFO only. It does not open or read
// any process descriptor's contents, and acknowledgement precedes cancellation.
func waitHostReaderAcquisition(t *testing.T, path string, finished chan hostReadResult, _ map[int]bool) hostReaderIdentity {
	t.Helper()
	var identity syscall.Stat_t
	if err := syscall.Stat(path, &identity); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		directory, err := os.Open("/dev/fd")
		if err != nil {
			t.Fatal(err)
		}
		entries, readError := directory.Readdirnames(-1)
		closeError := directory.Close()
		if readError != nil || closeError != nil {
			t.Fatalf("enumerate descriptor names: %v, close: %v", readError, closeError)
		}
		for _, entry := range entries {
			descriptor, err := strconv.Atoi(entry)
			if err != nil {
				continue
			}
			var current syscall.Stat_t
			if err := syscall.Fstat(descriptor, &current); err == nil && current.Dev == identity.Dev && current.Ino == identity.Ino {
				assertHostReaderFlags(t, descriptor)
				return hostReaderIdentity{descriptor: descriptor, metadata: current}
			}
		}
		select {
		case result := <-finished:
			finished <- result
			t.Fatalf("reader ended before FIFO acquisition acknowledgement: %q/%v", result.data, result.err)
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("reader descriptor acquisition never acknowledged")
		}
	}
}

func assertHostReaderFlags(t *testing.T, descriptor int) {
	t.Helper()
	flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(descriptor), syscall.F_GETFL, 0)
	if errno != 0 || int(flags)&(syscall.O_ACCMODE|syscall.O_NONBLOCK) != syscall.O_RDONLY|syscall.O_NONBLOCK {
		t.Errorf("actual reader access/nonblock flags=%#x err=%v", flags, errno)
	}
	descriptorFlags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(descriptor), syscall.F_GETFD, 0)
	if errno != 0 || descriptorFlags&syscall.FD_CLOEXEC == 0 {
		t.Errorf("actual reader close-on-exec flags=%#x err=%v", descriptorFlags, errno)
	}
}

func assertHostReaderRetired(t *testing.T, identity hostReaderIdentity) {
	t.Helper()
	var current syscall.Stat_t
	err := syscall.Fstat(identity.descriptor, &current)
	if err == nil && current.Dev == identity.metadata.Dev && current.Ino == identity.metadata.Ino {
		t.Error("owned FIFO descriptor remains after reader return")
	}
	if err != nil && !errors.Is(err, syscall.EBADF) {
		t.Errorf("inspect reader retirement: %v", err)
	}
}
