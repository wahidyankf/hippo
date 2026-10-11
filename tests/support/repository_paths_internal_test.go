package support

import (
	"os"
	"path/filepath"
	"testing"
)

func fixtureHIPPORepository(t *testing.T, root, module string, withSpecs bool) {
	t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module "+module+"\n\ngo 1.26.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if withSpecs {
		corpus := filepath.Join(root, "specs", "behaviours")
		if err := os.MkdirAll(corpus, 0o700); err != nil {
			t.Fatal(err)
		}
		feature := "Feature: Root identity fixture\n  Scenario: A root is readable\n    Given a root\n    When it is read\n    Then it remains a root\n"
		if err := os.WriteFile(filepath.Join(corpus, "root.feature"), []byte(feature), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func fixturePackageDirectory(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "internal", "adapters", "tui")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRepositoryRootAtPrefersNearestContainedWorktree(t *testing.T) {
	const module = "github.com/wahidyankf/hippo"
	primary := filepath.Join(t.TempDir(), "primary")
	worktree := filepath.Join(primary, "worktrees", "task")
	fixtureHIPPORepository(t, primary, module, true)
	fixtureHIPPORepository(t, worktree, module, true)
	got, err := repositoryRootAt(fixturePackageDirectory(t, worktree))
	if err != nil || got != worktree {
		t.Fatalf("nearest contained HIPPO worktree was not selected: root=%q error=%v; want %q rather than valid outer %q", got, err, worktree, primary)
	}
}

func TestRepositoryRootAtRejectsNearestForeignModule(t *testing.T) {
	primary := filepath.Join(t.TempDir(), "primary")
	foreign := filepath.Join(primary, "nested", "foreign")
	fixtureHIPPORepository(t, primary, "github.com/wahidyankf/hippo", true)
	fixtureHIPPORepository(t, foreign, "example.invalid/foreign", true)
	got, err := repositoryRootAt(fixturePackageDirectory(t, foreign))
	if err == nil || got != "" {
		t.Fatalf("nearest foreign module was accepted or skipped: root=%q error=%v; must refuse before valid outer %q", got, err, primary)
	}
}

func TestRepositoryRootAtRejectsMissingCanonicalSpecs(t *testing.T) {
	primary := filepath.Join(t.TempDir(), "primary")
	worktree := filepath.Join(primary, "worktrees", "without-specs")
	fixtureHIPPORepository(t, primary, "github.com/wahidyankf/hippo", true)
	fixtureHIPPORepository(t, worktree, "github.com/wahidyankf/hippo", false)
	got, err := repositoryRootAt(fixturePackageDirectory(t, worktree))
	if err == nil || got != "" {
		t.Fatalf("nearest HIPPO module without canonical specs was accepted or skipped: root=%q error=%v; must refuse before valid outer %q", got, err, primary)
	}
}
