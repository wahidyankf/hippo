package support

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2EBinaryCanonicalizesOwnedAliasParent(t *testing.T) {
	parent := publicCandidateAliasParent(t)
	t.Setenv("HIPPO_E2E_TEMP_PARENT", parent)
	binary, cleanup, err := e2eBinary("")
	defer cleanup()
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(binary)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(binary) || filepath.Clean(binary) != binary || canonical != binary {
		t.Fatalf("owned candidate producer returned a lexical alias: %s != %s", binary, canonical)
	}
	if err := requireTestStamp(binary); err != nil {
		t.Fatal("real current candidate has no required test identity", err)
	}
	cleanup()
	if _, err := os.Stat(filepath.Dir(binary)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("candidate cleanup retained its owned build directory: %v", err)
	}
}

func TestE2EBinaryBuildFailureRemovesOwnedDirectory(t *testing.T) {
	parent := publicCandidateAliasParent(t)
	t.Setenv("HIPPO_E2E_TEMP_PARENT", parent)
	t.Setenv("HIPPO_GO_BINARY", "/bin/false") // A real failed subprocess, never a fabricated candidate.
	binary, cleanup, err := e2eBinary("")
	defer cleanup()
	if err == nil || binary != "" {
		t.Fatalf("failed compiler acquired a candidate: %q %v", binary, err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed build retained owned directories: %+v", entries)
	}
}

func publicCandidateAliasParent(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	physical := filepath.Join(root, "physical")
	if err := os.Mkdir(physical, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(physical, alias); err != nil {
		t.Fatal(err)
	}
	return alias
}

// writeVersionFixture stages an executable that answers only the version query e2eBinary asks, as an inherited
// binary of the given identity.
func writeVersionFixture(t *testing.T, version, commit string) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "hippo")
	program := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' '{\"schemaVersion\":1,\"version\":%q,\"commit\":%q}'\n", version, commit)
	if err := os.WriteFile(binary, []byte(program), 0o700); err != nil {
		t.Fatal(err)
	}

	return binary
}

// TestE2EBinaryRunsATestStampedBuild covers the removed scenario "The end-to-end adapter runs a test-stamped build".
// With no inherited binary, or an unstamped one, the adapter builds the working tree with the harness stamp. A
// stamped inherited binary is kept unchanged.
func TestE2EBinaryRunsATestStampedBuild(t *testing.T) {
	t.Setenv("HIPPO_E2E_TEMP_PARENT", publicCandidateAliasParent(t))

	for _, row := range []struct{ name, inherited string }{
		{"no binary", ""},
		{"an unstamped build", writeVersionFixture(t, "dev", "unknown")},
	} {
		t.Run(row.name, func(t *testing.T) {
			binary, cleanup, err := e2eBinary(row.inherited)
			defer cleanup()
			if err != nil {
				t.Fatal(err)
			}
			if binary == row.inherited {
				t.Fatalf("the adapter kept %q instead of building the working tree", binary)
			}
			if err := requireTestStamp(binary); err != nil {
				t.Fatalf("the built binary lacks the harness stamp: %v", err)
			}
		})
	}

	t.Run("a build stamped v0.0.0-test", func(t *testing.T) {
		inherited := writeVersionFixture(t, testBuildVersion, strings.Repeat("0", 40))
		binary, cleanup, err := e2eBinary(inherited)
		defer cleanup()
		if err != nil {
			t.Fatal(err)
		}
		if binary != inherited {
			t.Fatalf("the adapter replaced the stamped binary it was handed with %q", binary)
		}
	})
}
