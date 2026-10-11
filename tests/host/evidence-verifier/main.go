// Command evidence-verifier checks recorded TUI host evidence.
package main

import (
	"context"
	"io"
	"io/fs"
	"os"
	"time"
)

// Candidate is the independently observed source, binary, and configuration cut.
type Candidate struct {
	Host          string
	Revision      string
	DirtyPaths    map[string]string
	ConfigPaths   map[string]string
	BinarySHA256  string
	BinaryVersion string
	BinaryCommit  string
	Now           time.Time
}

// Verify checks a document against the current cut and its referenced artifacts.
func Verify(reader io.Reader, current Candidate, artifacts fs.FS) error {
	return verifyDocument(reader, current, artifacts)
}

func main() {
	os.Exit(runCLI(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
