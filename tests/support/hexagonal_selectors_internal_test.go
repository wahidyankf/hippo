package support

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestHexagonalCoreCoverageSelectorsFollowActualPackageOwners(t *testing.T) {
	driver := &Driver{}
	if err := driver.inspectContributorEnforcement(); err != nil {
		t.Fatal(err)
	}
	if err := driver.requireCoreCoverage(); err != nil {
		t.Fatal(err)
	}
}

func TestHexagonalEveryPackageWithTestsExecutesInAGate(t *testing.T) {
	driver := &Driver{}
	if err := driver.inspectGatePackages(); err != nil {
		t.Fatal(err)
	}
	if err := driver.requireEveryTestPackageGated(); err != nil {
		t.Fatal(err)
	}
}

func TestHexagonalSelectorsRejectStaleAndNonexecutingPromises(t *testing.T) {
	script := lintWiringDriver(t).lintCommand
	for _, row := range []struct{ name, from, to string }{
		{"old config package", "./internal/adapters/config,", "./internal/config,"},
		{"old parser file", "internal/adapters/host/linux_parsers.go", "internal/host/linux_parsers.go"},
		{"domain directory missing", "internal/domain/coordination,internal/domain/evidence", "internal/domain/evidence"},
		{"application tests missing", "./tests/unit ./internal/application ", "./tests/unit "},
		{"threshold reduced", "--minimum 99", "--minimum 98"},
		{"coverage command commented", "go test -count=1 \\\n", "# go test -count=1 \\\n"},
		{"threshold failure swallowed", "--minimum 99", "--minimum 99 || true"},
	} {
		t.Run(row.name, func(t *testing.T) {
			mutated := replaceOnce(t, script, row.from, row.to)
			if validCoreCoverageSelectors(mutated) {
				t.Fatal("nonexecuting or incomplete selector accepted")
			}
		})
	}
}

func TestHexagonalProductionImportGateOrdering(t *testing.T) {
	driver := lintWiringDriver(t)
	if err := driver.requirePinnedNilAway(); err != nil {
		t.Fatal(err)
	}
	architecture := "go test -count=1 ./tests/architecture\n"
	nilaway := "go tool nilaway -include-pkgs=github.com/wahidyankf/hippo -pretty-print=false ./...\n"
	for _, row := range []struct{ name, script string }{
		{"only commented", replaceOnce(t, driver.lintCommand, architecture, "# "+architecture)},
		{"fixture-only filter", replaceOnce(t, driver.lintCommand, architecture, strings.TrimSpace(architecture)+" -run Fixture\n")},
		{"before NilAway", strings.Replace(replaceOnce(t, driver.lintCommand, architecture, ""), nilaway, architecture+nilaway, 1)},
	} {
		t.Run(row.name, func(t *testing.T) {
			mutated := *driver
			mutated.lintCommand = row.script
			if err := mutated.requirePinnedNilAway(); err == nil {
				t.Fatal("import check ordering or complete graph execution not enforced")
			}
		})
	}
}

func TestHexagonalEveryDomainLeafIsInMeasuredCoverage(t *testing.T) {
	script := lintWiringDriver(t).lintCommand
	joined := strings.ReplaceAll(script, "\\\n", " ")
	coverage, threshold := "", ""
	for line := range strings.SplitSeq(joined, "\n") {
		if strings.HasPrefix(line, "go test ") && strings.Contains(line, "-coverprofile=") {
			coverage = line
		}
		if strings.HasPrefix(line, "go run ./tests/coverage ") {
			threshold = line
		}
	}
	root := toolRoot()
	err := filepath.WalkDir(filepath.Join(root, "internal", "domain"), func(path string, entry fs.DirEntry, walkError error) error {
		if walkError != nil {
			return walkError
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		leaf := filepath.ToSlash(relative)
		if !csvSelectorContains(selectorFlag(coverage, "-coverpkg="), "./"+leaf) || !csvSelectorContains(selectorFlag(threshold, "--directories"), leaf) {
			return fmt.Errorf("domain leaf omitted from measured core: %s", leaf)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
