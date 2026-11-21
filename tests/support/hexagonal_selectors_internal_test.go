package support

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestHexagonalEveryPackageWithTestsExecutesInAGate(t *testing.T) {
	driver := &Driver{}
	if err := driver.inspectGatePackages(); err != nil {
		t.Fatal(err)
	}
	if err := driver.requireEveryTestPackageGated(); err != nil {
		t.Fatal(err)
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
	root, err := toolRoot()
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(filepath.Join(root, "internal", "domain"), func(path string, entry fs.DirEntry, walkError error) error {
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
