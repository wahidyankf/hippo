package host

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
)

func refusedReader(t *testing.T) ContextFileReader {
	t.Helper()

	return func(context.Context, string) ([]byte, error) {
		t.Error("read a file the route refused")

		return nil, nil
	}
}

func TestReadRoutedChoosesTheReaderByFileKind(t *testing.T) {
	machine := systemtest.NewHost(time.Unix(0, 0))
	machine.Files.Contents["/evidence/regular"] = []byte("regular")
	machine.Files.Contents["/evidence/pipe"] = []byte{}
	machine.Files.Modes["/evidence/pipe"] = fs.ModeNamedPipe
	route := fileRoute{
		Abs:     filepath.Abs,
		Stat:    machine.Port().Files.Stat,
		Regular: func(_ context.Context, path string) ([]byte, error) { return []byte("regular " + path), nil },
		FIFO:    func(_ context.Context, path string) ([]byte, error) { return []byte("fifo " + path), nil },
	}
	for _, test := range []struct{ path, want string }{
		{"/evidence/regular", "regular /evidence/regular"},
		{"/evidence/pipe", "fifo /evidence/pipe"},
	} {
		data, err := readRouted(t.Context(), test.path, route)
		if err != nil || string(data) != test.want {
			t.Fatalf("%s routed to %q error=%v, want %q", test.path, data, err, test.want)
		}
	}
}

func TestReadRoutedCancellationWinsOverItsOwnFailures(t *testing.T) {
	machine := systemtest.NewHost(time.Unix(0, 0))

	t.Run("cancelled before the path is resolved", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		route := fileRoute{
			Abs: func(string) (string, error) { t.Error("resolved a path after cancellation"); return "", nil },
			Stat: func(string) (fs.FileInfo, error) {
				t.Error("stat after cancellation")
				return nil, errors.New("stat after cancellation")
			},
			Regular: refusedReader(t),
			FIFO:    refusedReader(t),
		}
		if _, err := readRouted(ctx, "/evidence/regular", route); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled read returned %v", err)
		}
	})

	t.Run("cancelled while the path is resolved", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		absFailure := errors.New("no working directory")
		route := fileRoute{
			Abs:     func(string) (string, error) { cancel(); return "", absFailure },
			Stat:    machine.Port().Files.Stat,
			Regular: refusedReader(t),
			FIFO:    refusedReader(t),
		}
		if _, err := readRouted(ctx, "/evidence/regular", route); !errors.Is(err, context.Canceled) || errors.Is(err, absFailure) {
			t.Fatalf("cancellation did not win over the path failure: %v", err)
		}
	})

	t.Run("cancelled while the metadata is read", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		route := fileRoute{
			Abs: filepath.Abs,
			Stat: func(name string) (fs.FileInfo, error) {
				cancel()

				return machine.Port().Files.Stat(name)
			},
			Regular: refusedReader(t),
			FIFO:    refusedReader(t),
		}
		if _, err := readRouted(ctx, "/evidence/absent", route); !errors.Is(err, context.Canceled) || errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("cancellation did not win over the stat failure: %v", err)
		}
	})
}

func TestReadRoutedReportsPathAndStatFailures(t *testing.T) {
	machine := systemtest.NewHost(time.Unix(0, 0))
	absFailure := errors.New("no working directory")
	route := fileRoute{
		Abs:     func(string) (string, error) { return "", absFailure },
		Stat:    machine.Port().Files.Stat,
		Regular: refusedReader(t),
		FIFO:    refusedReader(t),
	}
	_, err := readRouted(t.Context(), "/evidence/regular", route)
	pathError, ok := errors.AsType[*fs.PathError](err)
	if !ok || pathError.Op != "abs" || !errors.Is(pathError, absFailure) {
		t.Fatalf("path resolution failure reported %v", err)
	}

	route.Abs = filepath.Abs
	if _, err := readRouted(t.Context(), "/evidence/absent", route); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("absent metadata reported %v, want the missing file", err)
	}
}
