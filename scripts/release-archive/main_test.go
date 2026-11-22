package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system/systemtest"
)

// testBinary is a member body long enough for gzip to emit several blocks, and not a repetition of one byte, so a
// change in the compressed stream shows.
func testBinary() []byte {
	data := make([]byte, 70_000)
	seed := uint32(1)
	for index := range data {
		seed = seed*1664525 + 1013904223
		data[index] = byte(seed >> 24)
	}

	return data
}

// streamingWrite is the archive writer before the file port: it streams the binary from one real file into another.
// The test keeps it as the reference that the port-based write must match byte for byte.
func streamingWrite(binary, output string, modified time.Time) (err error) {
	source, err := os.Open(binary)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, source.Close()) }()

	info, err := source.Stat()
	if err != nil {
		return err
	}

	destination, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, destination.Close()) }()

	compressed := gzip.NewWriter(destination)
	compressed.OS = unixOS

	archive := tar.NewWriter(compressed)
	header := &tar.Header{
		Typeflag: tar.TypeReg,
		Name:     "hippo",
		Mode:     0o755,
		Uname:    "root",
		Gname:    "root",
		Size:     info.Size(),
		ModTime:  modified,
		Format:   tar.FormatUSTAR,
	}
	if err = archive.WriteHeader(header); err != nil {
		return err
	}
	if _, err = io.Copy(archive, source); err != nil {
		return err
	}
	if err = archive.Close(); err != nil {
		return err
	}

	return compressed.Close()
}

func TestWriteMatchesTheBytesTheStreamingCodeWrites(t *testing.T) {
	directory := t.TempDir()
	binary := filepath.Join(directory, "hippo")
	if err := os.WriteFile(binary, testBinary(), 0o755); err != nil {
		t.Fatal(err)
	}
	modified := time.Unix(1_800_000_000, 0)
	reference := filepath.Join(directory, "reference.tar.gz")
	if err := streamingWrite(binary, reference, modified); err != nil {
		t.Fatalf("streaming write: %v", err)
	}
	want, err := os.ReadFile(reference)
	if err != nil {
		t.Fatal(err)
	}

	host := systemtest.NewHost(time.Time{})
	host.Files.Contents["build/hippo"] = testBinary()
	if err := write(host.Port().Files, "build/hippo", modified, "dist/hippo.tar.gz"); err != nil {
		t.Fatalf("write() = %v, want nil", err)
	}
	if got := host.Files.Contents["dist/hippo.tar.gz"]; !bytes.Equal(got, want) {
		t.Fatalf("archive has %d bytes, want the %d bytes the streaming code writes", len(got), len(want))
	}
	if mode := host.Files.Modes["dist/hippo.tar.gz"]; mode != 0o644 {
		t.Fatalf("archive mode = %o, want 644", mode)
	}
}

func TestWriteReportsAMissingBinaryWithoutWritingTheArchive(t *testing.T) {
	host := systemtest.NewHost(time.Time{})

	err := write(host.Port().Files, "build/hippo", time.Unix(0, 0), "dist/hippo.tar.gz")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("write() = %v, want a not-exist error", err)
	}
	if _, written := host.Files.Contents["dist/hippo.tar.gz"]; written {
		t.Fatal("archive written for a missing binary")
	}
}

func TestWriteReportsAModificationTimeTheTarHeaderCannotHold(t *testing.T) {
	host := systemtest.NewHost(time.Time{})
	host.Files.Contents["build/hippo"] = []byte("hippo")

	// The USTAR header stores the modification time in 33 bits of seconds, so this value (year 2286) does not fit.
	err := write(host.Port().Files, "build/hippo", time.Unix(10_000_000_000, 0), "dist/hippo.tar.gz")
	if err == nil {
		t.Fatal("write() = nil, want a header error")
	}
	if _, written := host.Files.Contents["dist/hippo.tar.gz"]; written {
		t.Fatal("archive written after a header error")
	}
}

func TestWriteReturnsAFailedArchiveWrite(t *testing.T) {
	host := systemtest.NewHost(time.Time{})
	host.Files.Contents["build/hippo"] = []byte("hippo")
	files := host.Port().Files
	failure := errors.New("disk full")
	files.WriteFile = func(string, []byte, fs.FileMode) error { return failure }

	if err := write(files, "build/hippo", time.Unix(0, 0), "dist/hippo.tar.gz"); !errors.Is(err, failure) {
		t.Fatalf("write() = %v, want %v", err, failure)
	}
}

func TestRunRejectsBadArguments(t *testing.T) {
	for _, row := range []struct {
		name      string
		arguments []string
	}{
		{"no flags", []string{}},
		{"unknown flag", []string{"-bogus"}},
		{"stray argument", []string{"-binary", "build/hippo", "-mtime", "1", "-output", "dist/out", "extra"}},
		{"negative mtime", []string{"-binary", "build/hippo", "-mtime", "-1", "-output", "dist/out"}},
	} {
		t.Run(row.name, func(t *testing.T) {
			host := systemtest.NewHost(time.Time{})
			if err := run(host.Port().Files, row.arguments); err == nil {
				t.Fatal("run() = nil, want an argument error")
			}
		})
	}
}

func TestMainWritesTheArchiveAndExitsOnlyOnFailure(t *testing.T) {
	host := systemtest.NewHost(time.Time{})
	host.Files.Contents["build/hippo"] = []byte("hippo")
	original := process
	t.Cleanup(func() { process = original })

	// The port copies the arguments when it is built, so each run builds a fresh one.
	host.Environment.Arguments = []string{
		"release-archive", "-binary", "build/hippo", "-mtime", "1800000000", "-output", "dist/hippo.tar.gz",
	}
	process = host.Port()
	main()
	if want := []int(nil); !reflect.DeepEqual(host.Environment.Exits, want) {
		t.Fatalf("exit codes after a successful run = %v, want %v", host.Environment.Exits, want)
	}
	if _, written := host.Files.Contents["dist/hippo.tar.gz"]; !written {
		t.Fatal("successful run wrote no archive")
	}

	host.Environment.Arguments = []string{"release-archive"}
	process = host.Port()
	main()
	if want := []int{1}; !reflect.DeepEqual(host.Environment.Exits, want) {
		t.Fatalf("exit codes after a failed run = %v, want %v", host.Environment.Exits, want)
	}
	if got := host.Environment.Errors.String(); !strings.Contains(got, "release-archive:") {
		t.Fatalf("stderr = %q, want it to name the program", got)
	}
}
