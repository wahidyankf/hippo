//go:build darwin || linux

package host

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

const filePollQuantum = 10 * time.Millisecond

// fileReadOperations keeps rare syscall failures testable without expanding the
// application port or exposing operating-system types outside the host adapter.
type fileReadOperations struct {
	open  func(string, int) (int, error)
	poll  func(int, int) (bool, error)
	read  func(int, []byte) (int, error)
	close func(int) error
}

type fileReadResult struct {
	data []byte
	err  error
}

// RootedContextFileReader maps cleaned absolute kernel paths beneath root with
// the same lexical containment as RootedFileReader. Each platform retires its
// owned acquisition before returning.
func RootedContextFileReader(root string) ContextFileReader {
	return func(ctx context.Context, path string) ([]byte, error) {
		return readFileContext(ctx, filepath.Join(root, filepath.Clean("/"+path)))
	}
}

func (collector SystemCollector) contextFileReader() ContextFileReader {
	selected := collector.ReadFileContext
	if selected == nil && collector.ReadFile != nil {
		selected = func(_ context.Context, path string) ([]byte, error) {
			return collector.ReadFile(path)
		}
	}
	if selected == nil {
		selected = readFileContext
	}

	return func(ctx context.Context, path string) ([]byte, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := selected(ctx, path)
		if cancelled := ctx.Err(); cancelled != nil {
			return nil, cancelled
		}
		return data, err
	}
}

func readUnixFileContext(ctx context.Context, path string) ([]byte, error) {
	operations := fileReadOperations{
		open:  func(path string, flags int) (int, error) { return unix.Open(path, flags, 0) },
		poll:  pollFileDescriptor,
		read:  unix.Read,
		close: unix.Close,
	}
	return readFileWithOperations(ctx, path, operations)
}

func readFileWithOperations(ctx context.Context, path string, operations fileReadOperations) ([]byte, error) {
	descriptor, err := openFileContext(ctx, path, operations)
	if err != nil {
		return nil, err
	}
	result := readOwnedFile(ctx, path, descriptor, operations)
	return result.data, result.err
}

func openFileContext(ctx context.Context, path string, operations fileReadOperations) (int, error) {
	for {
		if err := ctx.Err(); err != nil {
			return -1, err
		}
		descriptor, err := operations.open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC)
		if err == nil {
			return descriptor, nil
		}
		if cancelled := ctx.Err(); cancelled != nil {
			return -1, cancelled
		}
		if !errors.Is(err, unix.EINTR) {
			return -1, &os.PathError{Op: "open", Path: path, Err: err}
		}
	}
}

// The returned owned result lets deferred close report its actual outcome
// before the caller observes bytes or an error, without named return values.
func readOwnedFile(ctx context.Context, path string, descriptor int, operations fileReadOperations) *fileReadResult {
	result := &fileReadResult{}
	defer func() {
		closeError := operations.close(descriptor)
		if cancelled := ctx.Err(); cancelled != nil {
			result.data, result.err = nil, cancelled
		}
		if closeError != nil {
			result.data = nil
			result.err = errors.Join(result.err, &os.PathError{Op: "close", Path: path, Err: closeError})
		}
	}()
	result.data, result.err = drainFileContext(ctx, path, descriptor, operations)
	return result
}

func drainFileContext(ctx context.Context, path string, descriptor int, operations fileReadOperations) ([]byte, error) {
	buffer := make([]byte, 4096)
	data := []byte{}
	for {
		if err := waitFileContext(ctx, path, descriptor, operations); err != nil {
			return nil, err
		}
		bytesRead, err := readReadyFile(ctx, descriptor, buffer, operations)
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
			continue
		}
		if err != nil {
			return nil, &os.PathError{Op: "read", Path: path, Err: err}
		}
		if bytesRead == 0 {
			return data, nil
		}
		data = append(data, buffer[:bytesRead]...)
	}
}

func readReadyFile(ctx context.Context, descriptor int, buffer []byte, operations fileReadOperations) (int, error) {
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		bytesRead, err := operations.read(descriptor, buffer)
		if cancelled := ctx.Err(); cancelled != nil {
			return 0, cancelled
		}
		if !errors.Is(err, unix.EINTR) {
			return bytesRead, err
		}
	}
}

func waitFileContext(ctx context.Context, path string, descriptor int, operations fileReadOperations) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		quantum := filePollQuantum
		if deadline, exists := ctx.Deadline(); exists {
			quantum = min(quantum, max(0, time.Until(deadline)))
		}
		// Round down to avoid polling beyond an earlier context deadline.
		ready, err := operations.poll(descriptor, int(quantum/time.Millisecond))
		if cancelled := ctx.Err(); cancelled != nil {
			return cancelled
		}
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return &os.PathError{Op: "poll", Path: path, Err: err}
		}
		if ready {
			return nil
		}
	}
}

func pollFileDescriptor(descriptor, timeout int) (bool, error) {
	// Open returned this native descriptor; it fits the pollfd ABI.
	value := []unix.PollFd{{Fd: int32(descriptor), Events: unix.POLLIN}} //nolint:gosec // Native open descriptors fit int32.
	if _, err := unix.Poll(value, timeout); err != nil {
		return false, err
	}
	return classifyFilePoll(value[0].Revents)
}

func classifyFilePoll(events int16) (bool, error) {
	if events&unix.POLLNVAL != 0 {
		return false, unix.EIO
	}
	// Readiness permits a read even with POLLERR; only the read determines
	// bytes, actual EOF or an acquisition failure.
	if events&(unix.POLLIN|unix.POLLHUP) != 0 {
		return true, nil
	}
	if events&unix.POLLERR != 0 {
		return false, unix.EIO
	}
	return false, nil
}
