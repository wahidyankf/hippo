package support

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// A task worktree below the checkout carries its own go.mod, so the go tool
// treats it as another module that no pattern here can reach. The wiring scan
// must judge this module's packages only, or every checkout holding a
// worktree fails the quick gate on packages it never touched.
func TestPackagesWithTestsLeavesOutNestedModules(t *testing.T) {
	root := t.TempDir()
	for _, file := range []string{
		"go.mod",
		"internal/owned/owned_test.go",
		"worktrees/task/go.mod",
		"worktrees/task/internal/owned/owned_test.go",
		"tools/nested/go.mod",
		"tools/nested/tool_test.go",
		"tools/shared/shared_test.go",
	} {
		path := filepath.Join(root, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package fixture\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	packages, err := packagesWithTests(root)
	if err != nil {
		t.Fatal(err)
	}

	slices.Sort(packages)
	want := []string{"./internal/owned", "./tools/shared"}
	if !slices.Equal(packages, want) {
		t.Fatalf("packages with tests = %v, want %v", packages, want)
	}
}
