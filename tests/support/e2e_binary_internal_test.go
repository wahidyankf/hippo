package support

import (
	"errors"
	"os"
	"path/filepath"
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
