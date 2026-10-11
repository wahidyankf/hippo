package support

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const refusalFixtureModule = "github.com/wahidyankf/hippo"

func nearestRootRefusalFixture(t *testing.T, withCorpus bool) (string, string, string) {
	t.Helper()
	outer := filepath.Join(t.TempDir(), "primary")
	nearest := filepath.Join(outer, "worktrees", "nearest")
	fixtureHIPPORepository(t, outer, refusalFixtureModule, true)
	fixtureHIPPORepository(t, nearest, refusalFixtureModule, withCorpus)
	return outer, nearest, fixturePackageDirectory(t, nearest)
}

func writeRootFixtureModule(t *testing.T, root, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func requireNearestRootRefusal(t *testing.T, got string, err error, nearest, outer, diagnostic string) {
	t.Helper()
	if err == nil || got != "" {
		t.Fatalf("invalid nearest repository returned root=%q error=%v; must refuse %q before valid outer %q", got, err, nearest, outer)
	}
	if !strings.Contains(err.Error(), diagnostic) || !strings.Contains(err.Error(), nearest) {
		t.Fatalf("nearest refusal names the wrong boundary: error=%v; want diagnostic %q at %q", err, diagnostic, nearest)
	}
}

func TestRepositoryRootAtRefusesMalformedModuleDirectives(t *testing.T) {
	for _, fixture := range []struct{ name, directive string }{
		{"missing operand", "module\n"},
		{"extra operand", "module " + refusalFixtureModule + " unexpected\n"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			outer, nearest, start := nearestRootRefusalFixture(t, true)
			writeRootFixtureModule(t, nearest, fixture.directive)
			got, err := repositoryRootAt(start)
			requireNearestRootRefusal(t, got, err, nearest, outer, "has an invalid module directive")
		})
	}
}

func TestRepositoryRootAtRefusesDuplicateModuleDirectives(t *testing.T) {
	outer, nearest, start := nearestRootRefusalFixture(t, true)
	writeRootFixtureModule(t, nearest, "module "+refusalFixtureModule+"\nmodule "+refusalFixtureModule+"\n")
	got, err := repositoryRootAt(start)
	requireNearestRootRefusal(t, got, err, nearest, outer, "has an invalid module directive")
}

func TestRepositoryRootAtRefusesMalformedQuotedModule(t *testing.T) {
	outer, nearest, start := nearestRootRefusalFixture(t, true)
	writeRootFixtureModule(t, nearest, `module "github.com/wahidyankf/hippo\q"`+"\n")
	got, err := repositoryRootAt(start)
	requireNearestRootRefusal(t, got, err, nearest, outer, "read nearest module identity")
}

func TestRepositoryRootAtAcceptsQuotedAndCommentedModuleControls(t *testing.T) {
	for _, fixture := range []struct{ name, directive string }{
		{"quoted", `module "github.com/wahidyankf/hippo"` + "\n"},
		{"commented", "// fixture module identity\nmodule\t" + refusalFixtureModule + "\t// identity comment\n"},
		{"quoted and commented", `module "github.com/wahidyankf/hippo" // identity comment` + "\n"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			outer, nearest, start := nearestRootRefusalFixture(t, true)
			writeRootFixtureModule(t, nearest, fixture.directive)
			got, err := repositoryRootAt(start)
			if err != nil || got != nearest {
				t.Fatalf("valid module control did not select nearest repository: root=%q error=%v; want %q before valid outer %q", got, err, nearest, outer)
			}
		})
	}
}

func TestRepositoryRootAtRefusesNonDirectoryStart(t *testing.T) {
	outer, nearest, directory := nearestRootRefusalFixture(t, true)
	start := filepath.Join(directory, "start-file")
	if err := os.WriteFile(start, []byte("owned start fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := repositoryRootAt(start)
	requireNearestRootRefusal(t, got, err, nearest, outer, fmt.Sprintf("repository start %q is not a directory", start))
}

func TestRepositoryRootAtRefusesNonDirectoryCanonicalCorpus(t *testing.T) {
	outer, nearest, start := nearestRootRefusalFixture(t, false)
	if err := os.Mkdir(filepath.Join(nearest, "specs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nearest, "specs", "behaviours"), []byte("owned corpus file"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := repositoryRootAt(start)
	corpus := filepath.Join(nearest, "specs", "behaviours")
	requireNearestRootRefusal(t, got, err, nearest, outer, fmt.Sprintf("nearest module canonical corpus %q is not a directory", corpus))
}

func TestRepositoryRootAtRefusesNearestModuleReadFailure(t *testing.T) {
	outer, nearest, start := nearestRootRefusalFixture(t, true)
	modulePath := filepath.Join(nearest, "go.mod")
	if err := os.Remove(modulePath); err != nil {
		t.Fatal(err)
	}
	// Reading a directory fails natively even when the fixture runs as root;
	// permission-bit fixtures would depend on the invoking account.
	if err := os.Mkdir(modulePath, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := repositoryRootAt(start)
	requireNearestRootRefusal(t, got, err, nearest, outer, "read nearest module ")
	var readFailure *os.PathError
	if !errors.As(err, &readFailure) || readFailure.Path != modulePath {
		t.Fatalf("nearest module refusal lost its native read error: error=%v; want path %q", err, modulePath)
	}
}
