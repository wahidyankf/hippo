//go:build mixedversion

package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

type mixedV085Owner struct {
	command             *exec.Cmd
	cancel              context.CancelFunc
	done                chan struct{}
	release, token      string
	payloadPID          int
	startedAt, readyAt  time.Time
	output, diagnostics mixedV085Capture
	waitError           error
}

type mixedV085Capture struct{ buffer bytes.Buffer }

func (capture *mixedV085Capture) Bytes() []byte { return capture.buffer.Bytes() }
func (capture *mixedV085Capture) Len() int      { return capture.buffer.Len() }

func (capture *mixedV085Capture) Write(data []byte) (int, error) {
	if len(data) > 8*1024*1024-capture.Len() {
		return 0, errors.New("mixed command stream exceeds eight MiB")
	}
	return capture.buffer.Write(data)
}

func mixedV085OwnerRetirement(job *mixedV085Owner, inspect func(int) error) error {
	var result error
	if job.waitError != nil || job.command.ProcessState == nil || job.command.ProcessState.ExitCode() != 0 {
		result = errors.New("mixed owner did not naturally exit zero")
		if job.waitError != nil {
			result = fmt.Errorf("mixed owner did not naturally exit zero: %w", job.waitError)
		}
	}
	if job.payloadPID <= 0 {
		return errors.Join(result, errors.New("mixed payload retirement UNKNOWN: payload PID unobserved"))
	}
	err := inspect(job.payloadPID)
	if err == nil {
		result = errors.Join(result, errors.New("mixed payload survived actual joined Wait"))
	} else if !errors.Is(err, syscall.ESRCH) {
		result = errors.Join(result, fmt.Errorf("mixed payload retirement UNKNOWN: inspect PID %d: %w", job.payloadPID, err))
	}
	return result
}

func mixedV085ExecutableDigest(t *testing.T, path string) string {
	t.Helper()
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		t.Fatal("mixed executable requires a clean absolute path")
	}
	file, info, err := mixedV085OpenRegular(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		t.Fatal("mixed executable must be executable")
	}
	hash := sha256.New()
	_, readError := io.Copy(hash, file)
	if err := errors.Join(readError, file.Close()); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// Open relative to an explicit parent capability, rejecting symlinks and a
// replacement between lstat and open. The caller owns and closes the returned fd.
func mixedV085OpenRegular(path string) (*os.File, os.FileInfo, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, nil, errors.New("mixed input requires a clean absolute path")
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, nil, err
	}
	name := filepath.Base(path)
	info, err := root.Lstat(name)
	if err != nil {
		return nil, nil, errors.Join(err, root.Close())
	}
	if !info.Mode().IsRegular() {
		return nil, nil, errors.Join(errors.New("mixed input must be a regular file"), root.Close())
	}
	file, openError := root.Open(name)
	if err := errors.Join(openError, root.Close()); err != nil {
		if file != nil {
			err = errors.Join(err, file.Close())
		}
		return nil, nil, err
	}
	opened, err := file.Stat()
	if err != nil {
		return nil, nil, errors.Join(err, file.Close())
	}
	if !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		return nil, nil, errors.Join(errors.New("mixed input identity changed during open"), file.Close())
	}
	return file, opened, nil
}
