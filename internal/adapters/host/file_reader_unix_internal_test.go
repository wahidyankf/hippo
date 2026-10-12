//go:build darwin || linux

package host

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
)

func controlledFileOperations() fileReadOperations {
	return fileReadOperations{
		open:  func(string, int) (int, error) { return 42, nil },
		poll:  func(int, int) (bool, error) { return true, nil },
		read:  func(int, []byte) (int, error) { return 0, nil },
		close: func(int) error { return nil },
	}
}

func TestContextFileReaderAlreadyCancelledAcquiresNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	operations := controlledFileOperations()
	operations.open = func(string, int) (int, error) { t.Error("cancelled reader opened evidence"); return 42, nil }
	operations.close = func(int) error { t.Error("cancelled reader closed unacquired evidence"); return nil }
	data, err := readFileWithOperations(ctx, "/evidence", operations)
	if data != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read: %q, %v", data, err)
	}
}

func TestContextFileReaderOwnsFlagsBytesAndOneClose(t *testing.T) {
	operations := controlledFileOperations()
	opens, reads, closes := 0, 0, 0
	operations.open = func(path string, flags int) (int, error) {
		opens++
		if path != "/evidence" || flags != syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC {
			t.Errorf("acquisition: %q flags=%#x", path, flags)
		}
		return 42, nil
	}
	operations.poll = func(fd, timeout int) (bool, error) {
		if fd != 42 || timeout < 0 || timeout > 10 {
			t.Errorf("unbounded or foreign poll: fd=%d timeout=%d", fd, timeout)
		}
		return true, nil
	}
	operations.read = func(fd int, buffer []byte) (int, error) {
		reads++
		if fd != 42 {
			t.Error("read used another descriptor")
		}
		if reads == 1 {
			return copy(buffer, "complete bytes"), nil
		}
		return 0, nil
	}
	operations.close = func(fd int) error {
		closes++
		if fd != 42 {
			t.Error("closed another descriptor")
		}
		return nil
	}
	data, err := readFileWithOperations(t.Context(), "/evidence", operations)
	if err != nil || string(data) != "complete bytes" || opens != 1 || reads != 2 || closes != 1 {
		t.Fatalf("read=%q err=%v ownership=%d/%d/%d", data, err, opens, reads, closes)
	}
}

func TestContextFileReaderRetryOwnership(t *testing.T) {
	operations := controlledFileOperations()
	opens, polls, reads, closes := 0, 0, 0, 0
	var firstBuffer *byte
	operations.open = func(string, int) (int, error) {
		opens++
		if opens == 1 {
			return -1, syscall.EINTR
		}
		return 42, nil
	}
	operations.poll = func(int, int) (bool, error) {
		polls++
		if polls == 1 {
			return false, syscall.EINTR
		}
		if polls == 2 {
			return false, nil
		}
		return true, nil
	}
	operations.read = func(fd int, buffer []byte) (int, error) {
		reads++
		if fd != 42 {
			t.Error("retry changed read descriptor")
		}
		if firstBuffer == nil {
			firstBuffer = &buffer[0]
		} else if firstBuffer != &buffer[0] {
			t.Error("retry changed read buffer identity")
		}
		switch reads {
		case 1:
			return 0, syscall.EINTR
		case 2:
			return 0, syscall.EAGAIN
		case 3:
			return copy(buffer, "retried"), nil
		default:
			return 0, nil
		}
	}
	operations.close = func(int) error { closes++; return nil }
	data, err := readFileWithOperations(t.Context(), "/evidence", operations)
	if string(data) != "retried" || err != nil || opens != 2 || polls != 5 || reads != 4 || closes != 1 {
		t.Fatalf("retry %q/%v, operations=%d/%d/%d/%d", data, err, opens, polls, reads, closes)
	}
}

func TestContextFileReaderErrorsRetainIdentityAndClose(t *testing.T) {
	for _, phase := range []string{"open", "poll", "read", "close"} {
		t.Run(phase, func(t *testing.T) {
			operations := controlledFileOperations()
			closes := 0
			operations.close = func(int) error {
				closes++
				if phase == "close" {
					return syscall.EIO
				}
				return nil
			}
			switch phase {
			case "open":
				operations.open = func(string, int) (int, error) { return -1, syscall.EIO }
			case "poll":
				operations.poll = func(int, int) (bool, error) { return false, syscall.EIO }
			case "read":
				operations.read = func(int, []byte) (int, error) { return 0, syscall.EIO }
			case "close":
			default:
				t.Fatal("unknown failure phase")
			}
			data, err := readFileWithOperations(t.Context(), "/evidence", operations)
			pathError, ok := errors.AsType[*os.PathError](err)
			wantCloses := 1
			if phase == "open" {
				wantCloses = 0
			}
			if data != nil || !errors.Is(err, syscall.EIO) || !ok || pathError.Op != phase || pathError.Path != "/evidence" || closes != wantCloses {
				t.Fatalf("failure %q err=%v closes=%d", data, err, closes)
			}
		})
	}
}

func cancellationFileOperations(phase string, cancel context.CancelFunc, reads, closes *int) fileReadOperations {
	operations := controlledFileOperations()
	operations.open = func(string, int) (int, error) {
		if phase == "open failure" {
			cancel()
			return -1, syscall.EIO
		}
		if phase == "successful open" {
			cancel()
		}
		return 42, nil
	}
	operations.poll = func(int, int) (bool, error) {
		if phase == "poll" {
			cancel()
			return false, syscall.EIO
		}
		return true, nil
	}
	operations.read = func(_ int, buffer []byte) (int, error) {
		*reads++
		if phase == "read" {
			cancel()
			return copy(buffer, "partial"), syscall.EIO
		}
		if phase == "EOF" {
			cancel()
		}
		return 0, nil
	}
	operations.close = func(int) error {
		*closes++
		if phase == "close" {
			cancel()
		}
		return syscall.EBADF
	}
	return operations
}

func TestContextFileReaderCancellationWinsAndJoinsCloseFailure(t *testing.T) {
	for _, phase := range []string{"open failure", "successful open", "poll", "read", "EOF", "close"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			closes, reads := 0, 0
			operations := cancellationFileOperations(phase, cancel, &reads, &closes)
			data, err := readFileWithOperations(ctx, "/evidence", operations)
			if data != nil || !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation lost: %q, %v", data, err)
			}
			if phase == "open failure" {
				if closes != 0 || reads != 0 {
					t.Fatal("failed acquisition incurred effects")
				}
				return
			}
			if closes != 1 || !errors.Is(err, syscall.EBADF) {
				t.Fatalf("close was hidden or repeated: %v closes=%d", err, closes)
			}
		})
	}
}

func TestContextFileReaderDiscardsPreviouslyReadBytesOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	operations := controlledFileOperations()
	reads, polls, closes := 0, 0, 0
	operations.poll = func(int, int) (bool, error) {
		polls++
		if polls == 2 {
			cancel()
			return false, nil
		}
		return true, nil
	}
	operations.read = func(_ int, buffer []byte) (int, error) { reads++; return copy(buffer, "already read"), nil }
	operations.close = func(int) error { closes++; return nil }
	data, err := readFileWithOperations(ctx, "/evidence", operations)
	if data != nil || !errors.Is(err, context.Canceled) || reads != 1 || polls != 2 || closes != 1 {
		t.Fatalf("partial cancellation %q/%v reads/polls/closes=%d/%d/%d", data, err, reads, polls, closes)
	}
}

func TestContextFileReaderDeadlineBoundsPoll(t *testing.T) {
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(5*time.Millisecond))
	defer cancel()
	operations := controlledFileOperations()
	operations.poll = func(_, timeout int) (bool, error) {
		if timeout < 0 || timeout > 5 {
			t.Errorf("deadline exceeded by poll: %d", timeout)
		}
		cancel()
		return false, nil
	}
	_, err := readFileWithOperations(ctx, "/evidence", operations)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("deadline control: %v", err)
	}
}

func TestContextFileReaderSelectionPreservesLegacyCompatibility(t *testing.T) {
	for _, explicitContext := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "context precedence"}[explicitContext], func(t *testing.T) {
			legacyCalls, contextCalls := 0, 0
			collector := SystemCollector{ReadFile: func(path string) ([]byte, error) { legacyCalls++; return []byte(path), nil }}
			if explicitContext {
				collector.ReadFileContext = func(ctx context.Context, path string) ([]byte, error) {
					contextCalls++
					if ctx != t.Context() {
						t.Error("context was replaced")
					}
					return []byte(path), nil
				}
			}
			data, err := collector.contextFileReader()(t.Context(), "/evidence")
			if err != nil || string(data) != "/evidence" || legacyCalls+contextCalls != 1 || explicitContext && contextCalls != 1 {
				t.Fatalf("selection %q/%v legacy/context=%d/%d", data, err, legacyCalls, contextCalls)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	collector := SystemCollector{ReadFile: func(string) ([]byte, error) { cancel(); return []byte("discard"), nil }}
	data, err := collector.contextFileReader()(ctx, "/evidence")
	if data != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("legacy post-read cancellation: %q/%v", data, err)
	}
	calls := 0
	collector.ReadFile = func(string) ([]byte, error) { calls++; return nil, nil }
	_, err = collector.contextFileReader()(ctx, "/evidence")
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("legacy already-cancelled acquisition: %v calls=%d", err, calls)
	}
}

// These masks are the shared Darwin/Linux poll ABI values. Keeping the tests
// standard-library-only preserves the host test dependency boundary.
const (
	testPollInput    int16 = 0x0001
	testPollPriority int16 = 0x0002
	testPollError    int16 = 0x0008
	testPollHangup   int16 = 0x0010
	testPollInvalid  int16 = 0x0020
)

func TestContextFileReaderPollMaskClassification(t *testing.T) {
	cases := []struct {
		name    string
		events  int16
		ready   bool
		failure bool
	}{
		{"zero", 0, false, false},
		{"priority alone", testPollPriority, false, false},
		{"input", testPollInput, true, false},
		{"hangup", testPollHangup, true, false},
		{"input and hangup", testPollInput | testPollHangup, true, false},
		{"input and priority", testPollInput | testPollPriority, true, false},
		{"hangup and priority", testPollHangup | testPollPriority, true, false},
		{"input hangup and priority", testPollInput | testPollHangup | testPollPriority, true, false},
		{"error alone", testPollError, false, true},
		{"error and priority", testPollError | testPollPriority, false, true},
		{"input and error", testPollInput | testPollError, true, false},
		{"hangup and error", testPollHangup | testPollError, true, false},
		{"input hangup and error", testPollInput | testPollHangup | testPollError, true, false},
		{"input error and priority", testPollInput | testPollError | testPollPriority, true, false},
		{"hangup error and priority", testPollHangup | testPollError | testPollPriority, true, false},
		{"input hangup error and priority", testPollInput | testPollHangup | testPollError | testPollPriority, true, false},
		{"invalid alone", testPollInvalid, false, true},
		{"invalid and input", testPollInvalid | testPollInput, false, true},
		{"invalid and hangup", testPollInvalid | testPollHangup, false, true},
		{"invalid and error", testPollInvalid | testPollError, false, true},
		{"invalid and priority", testPollInvalid | testPollPriority, false, true},
		{"invalid input and priority", testPollInvalid | testPollInput | testPollPriority, false, true},
		{"invalid error and priority", testPollInvalid | testPollError | testPollPriority, false, true},
		{"invalid input and error", testPollInvalid | testPollInput | testPollError, false, true},
		{"invalid input error and priority", testPollInvalid | testPollInput | testPollError | testPollPriority, false, true},
		{"invalid input and hangup", testPollInvalid | testPollInput | testPollHangup, false, true},
		{"invalid hangup and priority", testPollInvalid | testPollHangup | testPollPriority, false, true},
		{"invalid input hangup and priority", testPollInvalid | testPollInput | testPollHangup | testPollPriority, false, true},
		{"invalid hangup and error", testPollInvalid | testPollHangup | testPollError, false, true},
		{"invalid input hangup and error", testPollInvalid | testPollInput | testPollHangup | testPollError, false, true},
		{"invalid hangup error and priority", testPollInvalid | testPollHangup | testPollError | testPollPriority, false, true},
		{"all bits with invalid", testPollInvalid | testPollInput | testPollHangup | testPollError | testPollPriority, false, true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			ready, err := classifyFilePoll(test.events)
			if ready != test.ready || (err != nil) != test.failure {
				t.Fatalf("mask=%#x ready=%t error=%v; want ready=%t failure=%t", test.events, ready, err, test.ready, test.failure)
			}
			if test.failure && !errors.Is(err, syscall.EIO) {
				t.Fatalf("mask=%#x lost existing error identity: %v", test.events, err)
			}
		})
	}
}

func TestContextFileReaderMixedPollReadinessDrainsBeforeClose(t *testing.T) {
	masks := []struct {
		name   string
		events int16
	}{
		{"input and error", testPollInput | testPollError},
		{"hangup and error", testPollHangup | testPollError},
		{"input hangup and error", testPollInput | testPollHangup | testPollError},
		{"input error and priority", testPollInput | testPollError | testPollPriority},
		{"hangup error and priority", testPollHangup | testPollError | testPollPriority},
	}
	for _, mask := range masks {
		for _, failure := range []bool{false, true} {
			outcome := map[bool]string{false: "bytes then true EOF", true: "genuine read failure"}[failure]
			t.Run(mask.name+"/"+outcome, func(t *testing.T) {
				operations := controlledFileOperations()
				polls, reads, closes := 0, 0, 0
				operations.poll = func(int, int) (bool, error) {
					polls++
					return classifyFilePoll(mask.events)
				}
				operations.read = func(_ int, buffer []byte) (int, error) {
					reads++
					if failure {
						return 0, syscall.EACCES
					}
					if reads == 1 {
						return copy(buffer, "drained evidence"), nil
					}
					return 0, nil
				}
				operations.close = func(int) error { closes++; return nil }
				data, err := readFileWithOperations(t.Context(), "/evidence", operations)
				if failure {
					pathError, ok := errors.AsType[*os.PathError](err)
					if data != nil || !errors.Is(err, syscall.EACCES) || !ok || pathError.Op != "read" || pathError.Path != "/evidence" || reads != 1 || polls != 1 || closes != 1 {
						t.Fatalf("read failure data=%q err=%v polls/reads/closes=%d/%d/%d", data, err, polls, reads, closes)
					}
					return
				}
				if string(data) != "drained evidence" || err != nil || reads != 2 || polls != 2 || closes != 1 {
					t.Fatalf("readiness did not drain bytes then EOF: data=%q err=%v polls/reads/closes=%d/%d/%d", data, err, polls, reads, closes)
				}
			})
		}
	}
}

func TestContextFileReaderFatalPollMasksNeverReadAndCloseOnce(t *testing.T) {
	for _, events := range []int16{testPollError, testPollError | testPollPriority, testPollInvalid | testPollInput | testPollHangup | testPollError | testPollPriority} {
		operations := controlledFileOperations()
		reads, closes := 0, 0
		operations.poll = func(int, int) (bool, error) { return classifyFilePoll(events) }
		operations.read = func(int, []byte) (int, error) { reads++; return 0, nil }
		operations.close = func(int) error { closes++; return nil }
		data, err := readFileWithOperations(t.Context(), "/evidence", operations)
		pathError, ok := errors.AsType[*os.PathError](err)
		if data != nil || !errors.Is(err, syscall.EIO) || !ok || pathError.Op != "poll" || reads != 0 || closes != 1 {
			t.Fatalf("fatal mask=%#x data=%q err=%v reads/closes=%d/%d", events, data, err, reads, closes)
		}
	}
}
