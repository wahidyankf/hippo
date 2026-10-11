// Command oldidentity parses the actual immutable old-release version receipt.
// It is an E2E harness utility, never a HIPPO product command or host protocol.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func verifyOldReceipt(reader io.Reader) error {
	return verifyReleaseReceipt(reader, "v0.8.4", "abde2ecbeff7513549cfd1c0805fd20b5d232c01")
}

func verifyV085Receipt(reader io.Reader) error {
	return verifyReleaseReceipt(reader, "v0.8.5", "456d24bc2a20ee23a7b81746c1789133cd5c3a97")
}

func verifyReleaseReceipt(reader io.Reader, version, commit string) error {
	var identity struct {
		SchemaVersion int    `json:"schemaVersion"`
		Version       string `json:"version"`
		Commit        string `json:"commit"`
	}
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&identity); err != nil {
		return fmt.Errorf("parse old release identity: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("old release identity contains trailing JSON or invalid data")
	}
	if identity.SchemaVersion != 1 || identity.Version != version || identity.Commit != commit {
		return errors.New("old release identity does not match schema, version and immutable commit")
	}
	return nil
}

func readOldReceipt(path string) error {
	return readReleaseReceipt(path, verifyOldReceipt)
}

func readReleaseReceipt(path string, verify func(io.Reader) error) error {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open old release receipt directory: %w", err)
	}
	file, err := root.Open(filepath.Base(path))
	if err != nil {
		return errors.Join(fmt.Errorf("open old release receipt: %w", err), root.Close())
	}
	return errors.Join(verify(file), file.Close(), root.Close())
}

func main() {
	if len(os.Args) == 3 && os.Args[1] == "--v085" {
		if err := readReleaseReceipt(os.Args[2], verifyV085Receipt); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "oldidentity requires exactly one version receipt path")
		os.Exit(1)
	}
	if err := readOldReceipt(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
