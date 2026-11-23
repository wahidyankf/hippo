// Package main writes one release archive whose bytes depend only on its
// inputs: the binary, and the time recorded for it.
//
// The host's tar and gzip cannot do this. GNU tar and bsdtar lay out headers
// differently, each gzip implementation compresses differently, and both stamp
// the build time unless told otherwise, so the same binary archived on Linux
// and on macOS produced different checksums. Go's archive/tar and
// compress/gzip are the same code on every host, and the release script pins
// the Go toolchain that builds this program.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"time"

	"github.com/wahidyankf/hippo/internal/adapters/system"
)

// unixOS is the gzip header's operating-system byte for Unix, the value the
// archives carried when GNU tar on the Linux release runner wrote them.
const unixOS = 3

// process is the host the command runs against. A test replaces it with an in-memory host.
var process = system.Production

func main() {
	if err := run(process.Files, process.Environment.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(process.Environment.Stderr, "release-archive:", err)
		process.Environment.Exit(1)
	}
}

func run(files system.FileSystem, arguments []string) error {
	flags := flag.NewFlagSet("release-archive", flag.ContinueOnError)
	binary := flags.String("binary", "", "the binary to archive as hippo")
	modified := flags.Int64("mtime", -1, "the member's modification time, in Unix seconds")
	output := flags.String("output", "", "the .tar.gz file to write")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *binary == "" || *output == "" || *modified < 0 || flags.NArg() != 0 {
		return errors.New("usage: release-archive -binary <path> -mtime <unix-seconds> -output <path>")
	}

	return write(files, *binary, time.Unix(*modified, 0), *output)
}

// write archives the binary as the single member hippo: a regular file, mode
// 0755, owned by root, in the ustar format every tar implementation reads. It
// reads the binary and writes the archive as whole files, so a failed encoding
// leaves no archive behind.
func write(files system.FileSystem, binary string, modified time.Time, output string) error {
	data, err := files.ReadFile(binary)
	if err != nil {
		return err
	}
	archive, err := encode(data, modified)
	if err != nil {
		return err
	}

	return files.WriteFile(output, archive, 0o644)
}

// encode returns the gzip-compressed tar archive of the member. The header
// write decides whether the body is written; the closing calls always run, and
// the joined error keeps every failure that occurred.
func encode(data []byte, modified time.Time) ([]byte, error) {
	var buffer bytes.Buffer
	compressed := gzip.NewWriter(&buffer)
	compressed.OS = unixOS

	archive := tar.NewWriter(compressed)
	header := &tar.Header{
		Typeflag: tar.TypeReg,
		Name:     "hippo",
		Mode:     0o755,
		Uname:    "root",
		Gname:    "root",
		Size:     int64(len(data)),
		ModTime:  modified,
		Format:   tar.FormatUSTAR,
	}
	err := archive.WriteHeader(header)
	if err == nil {
		_, err = archive.Write(data)
	}
	err = errors.Join(err, archive.Close())
	err = errors.Join(err, compressed.Close())

	return buffer.Bytes(), err
}
