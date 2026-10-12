//go:build darwin || linux

package integration_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/host"
)

type hostReadResult struct {
	data []byte
	err  error
}

// A real acknowledged writer remains held until after cancellation and reader
// retirement. Releasing it during failure cleanup cannot earn cancellation proof.
func TestContextReaderHeldWriterCancellation(t *testing.T) {
	root := hostReaderFixture(t)
	path := filepath.Join(root, "held-fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	finished := make(chan hostReadResult, 1)
	read := host.RootedContextFileReader(root)
	go func() {
		data, err := read(ctx, "/held-fifo")
		finished <- hostReadResult{data, err}
	}()
	writer := -1
	joined := false
	defer func() {
		cancel()
		if writer >= 0 {
			if err := syscall.Close(writer); err != nil {
				t.Errorf("close held writer: %v", err)
			}
		}
		if !joined {
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Error("reader did not join after negative-control cleanup")
			}
		}
	}()
	writer = openHostFIFOWriter(t, path)
	cancel()
	select {
	case result := <-finished:
		joined = true
		if !errors.Is(result.err, context.Canceled) || len(result.data) != 0 {
			t.Errorf("held writer cancellation returned %q, %v", result.data, result.err)
		}
		if _, err := syscall.Write(writer, []byte("retirement probe")); !errors.Is(err, syscall.EPIPE) {
			t.Errorf("reader descriptor still present with writer held: %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Error("cancellation did not retire the reader before writer release")
	}
}

func openHostFIFOWriter(t *testing.T, path string) int {
	t.Helper()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		descriptor, err := syscall.Open(path, syscall.O_WRONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
		if err == nil {
			return descriptor
		}
		if !errors.Is(err, syscall.ENXIO) && !errors.Is(err, syscall.EINTR) {
			t.Fatalf("open writer: %v", err)
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("reader never acquired the FIFO")
		}
	}
}

func hostReaderFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Errorf("remove owned reader fixture: %v", err)
		}
		if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("owned reader fixture remains: %v", err)
		}
	})
	return root
}

func TestContextReaderRegularFilesAndRootMapping(t *testing.T) {
	root := hostReaderFixture(t)
	if err := os.Mkdir(filepath.Join(root, "proc"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"meminfo": "rooted bytes", "empty": ""} {
		if err := os.WriteFile(filepath.Join(root, "proc", name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	read := host.RootedContextFileReader(root)
	for _, row := range []struct{ path, want string }{{"/proc/meminfo", "rooted bytes"}, {"/../proc/meminfo", "rooted bytes"}, {"/proc/empty", ""}} {
		data, err := read(t.Context(), row.path)
		if err != nil || string(data) != row.want {
			t.Fatalf("root read %s: %q/%v", row.path, data, err)
		}
	}
	if _, err := read(t.Context(), "/proc/absent"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing evidence: %v", err)
	}
}

func TestContextReaderNoWriterCancellation(t *testing.T) {
	root := hostReaderFixture(t)
	path := filepath.Join(root, "no-writer")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	baseline := contextReaderBaseline(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	finished := make(chan hostReadResult, 1)
	go func() {
		data, err := host.RootedContextFileReader(root)(ctx, "/no-writer")
		finished <- hostReadResult{data, err}
	}()
	joined := false
	defer func() {
		cancel()
		if !joined {
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Error("no-writer reader survived cleanup")
			}
		}
	}()
	identity := waitHostReaderAcquisition(t, path, finished, baseline)
	cancel()
	select {
	case result := <-finished:
		joined = true
		if len(result.data) != 0 || !errors.Is(result.err, context.Canceled) {
			t.Fatalf("writerless FIFO became synthetic EOF: %q/%v", result.data, result.err)
		}
		assertHostReaderRetired(t, identity)
	case <-time.After(250 * time.Millisecond):
		t.Error("writerless FIFO cancellation did not join")
	}
}

func TestContextReaderFIFOTrueEOFAndBytes(t *testing.T) {
	for _, content := range []string{"", "FIFO bytes"} {
		t.Run(map[bool]string{true: "empty writer EOF", false: "content and EOF"}[content == ""], func(t *testing.T) {
			exerciseContextFIFOCompletion(t, content)
		})
	}
}

func exerciseContextFIFOCompletion(t *testing.T, content string) {
	t.Helper()
	root := hostReaderFixture(t)
	path := filepath.Join(root, "complete-fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	finished := make(chan hostReadResult, 1)
	go func() {
		data, err := host.RootedContextFileReader(root)(ctx, "/complete-fifo")
		finished <- hostReadResult{data, err}
	}()
	joined := false
	defer func() {
		cancel()
		if !joined {
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Error("EOF reader survived cleanup")
			}
		}
	}()
	writer := openHostFIFOWriter(t, path)
	if content != "" {
		n, err := syscall.Write(writer, []byte(content))
		if err != nil || n != len(content) {
			t.Errorf("write content: n=%d/%d err=%v", n, len(content), err)
		}
	}
	if err := syscall.Close(writer); err != nil {
		t.Error(err)
	}
	select {
	case result := <-finished:
		joined = true
		if result.err != nil || string(result.data) != content {
			t.Fatalf("true writer EOF: %q/%v", result.data, result.err)
		}
	case <-time.After(time.Second):
		t.Error("true writer EOF did not finish")
	}
}

func TestContextReaderPartialFIFOCancellation(t *testing.T) {
	root := hostReaderFixture(t)
	path := filepath.Join(root, "partial-fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	finished := make(chan hostReadResult, 1)
	go func() {
		data, err := host.RootedContextFileReader(root)(ctx, "/partial-fifo")
		finished <- hostReadResult{data, err}
	}()
	joined := false
	defer func() {
		cancel()
		if !joined {
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Error("partial reader survived cleanup")
			}
		}
	}()
	writer := openHostFIFOWriter(t, path)
	defer func() {
		if err := syscall.Close(writer); err != nil {
			t.Errorf("close partial writer: %v", err)
		}
	}()
	if n, err := syscall.Write(writer, []byte("partial")); err != nil || n != 7 {
		t.Errorf("write partial bytes: %d/%v", n, err)
	}
	cancel()
	select {
	case result := <-finished:
		joined = true
		if result.data != nil || !errors.Is(result.err, context.Canceled) {
			t.Errorf("partial cancellation: %q/%v", result.data, result.err)
		}
		if _, err := syscall.Write(writer, []byte("retirement probe")); !errors.Is(err, syscall.EPIPE) {
			t.Errorf("partial reader descriptor remains: %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Error("partial FIFO cancellation did not retire before writer release")
	}
}

func TestContextReaderCancellationConcurrentWithTrueEOF(t *testing.T) {
	root := hostReaderFixture(t)
	path := filepath.Join(root, "race-fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	finished := make(chan hostReadResult, 1)
	go func() {
		data, err := host.RootedContextFileReader(root)(ctx, "/race-fifo")
		finished <- hostReadResult{data, err}
	}()
	joined := false
	defer func() {
		cancel()
		if !joined {
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Error("EOF-race reader survived cleanup")
			}
		}
	}()
	writer := openHostFIFOWriter(t, path)
	ready := make(chan struct{})
	writerDone := make(chan error, 1)
	cancelDone := make(chan struct{})
	go func() { <-ready; writerDone <- syscall.Close(writer) }()
	go func() { <-ready; cancel(); close(cancelDone) }()
	close(ready)
	if err := <-writerDone; err != nil {
		t.Error(err)
	}
	<-cancelDone
	select {
	case result := <-finished:
		joined = true
		if result.err != nil && !errors.Is(result.err, context.Canceled) {
			t.Errorf("EOF race lost cancellation/EOF outcome: %v", result.err)
		}
		if len(result.data) != 0 {
			t.Errorf("empty EOF race returned partial bytes: %q", result.data)
		}
	case <-time.After(250 * time.Millisecond):
		t.Error("EOF race did not join")
	}
}
