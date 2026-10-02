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
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

// unixOS is the gzip header's operating-system byte for Unix, the value the
// archives carried when GNU tar on the Linux release runner wrote them.
const unixOS = 3

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "release-archive:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
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

	return write(*binary, time.Unix(*modified, 0), *output)
}

// write archives the binary as the single member hippo: a regular file, mode
// 0755, owned by root, in the ustar format every tar implementation reads.
func write(binary string, modified time.Time, output string) (err error) {
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
