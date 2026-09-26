package support

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// testRunningScripts are every script a gate runs `go test` from. A package
// none of them names is compiled by the gate but its tests never execute.
var testRunningScripts = []string{
	filepath.Join("scripts", "test-quick.sh"),
	filepath.Join("scripts", "test.sh"),
	filepath.Join("tests", "e2e", "run.sh"),
}

// goTestInvocation finds a `go test` command, including the end-to-end
// script's configurable Go binary.
var goTestInvocation = regexp.MustCompile(`(^|\s)(go|"\$go_binary") test\s`)

// compileOnly is the gate line that builds every test binary and runs none.
const compileOnly = `-run '^$'`

// gatedTestPatterns returns the package patterns the gates pass to `go test`
// with tests enabled, joining continued shell lines first.
func gatedTestPatterns(root string) ([]string, error) {
	var patterns []string
	for _, script := range testRunningScripts {
		data, err := os.ReadFile(filepath.Join(root, script))
		if err != nil {
			return nil, err
		}
		for line := range strings.SplitSeq(strings.ReplaceAll(string(data), "\\\n", " "), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") || !goTestInvocation.MatchString(line) || strings.Contains(line, compileOnly) {
				continue
			}
			for field := range strings.FieldsSeq(line) {
				if strings.HasPrefix(field, "./") {
					patterns = append(patterns, field)
				}
			}
		}
	}

	return patterns, nil
}

// packagesWithTests lists every module directory holding a _test.go file, as
// the ./relative form a go test pattern uses.
func packagesWithTests(root string) ([]string, error) {
	var packages []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkError error) error {
		if walkError != nil {
			return walkError
		}
		name := entry.Name()
		if entry.IsDir() {
			// The go tool ignores the same directories.
			if path != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") ||
				name == "testdata" || name == "node_modules" || name == "vendor") {
				return filepath.SkipDir
			}

			return nil
		}
		if !strings.HasSuffix(name, "_test.go") {
			return nil
		}
		relative, relativeError := filepath.Rel(root, filepath.Dir(path))
		if relativeError != nil {
			return relativeError
		}
		pattern := "./" + filepath.ToSlash(relative)
		if !slices.Contains(packages, pattern) {
			packages = append(packages, pattern)
		}

		return nil
	})

	return packages, err
}

// patternMatches reports whether a go test pattern names a package: exactly,
// or through a trailing /... that includes the directory and all below it.
func patternMatches(pattern, pkg string) bool {
	if prefix, recursive := strings.CutSuffix(pattern, "/..."); recursive {
		return prefix == "." || pkg == prefix || strings.HasPrefix(pkg, prefix+"/")
	}

	return pattern == pkg
}

func (driver *Driver) inspectGatePackages() error {
	root := toolRoot()
	packages, err := packagesWithTests(root)
	if err != nil {
		return err
	}
	patterns, err := gatedTestPatterns(root)
	if err != nil {
		return err
	}
	driver.ungatedPackages = nil
	for _, pkg := range packages {
		if !slices.ContainsFunc(patterns, func(pattern string) bool { return patternMatches(pattern, pkg) }) {
			driver.ungatedPackages = append(driver.ungatedPackages, pkg)
		}
	}
	if len(packages) == 0 {
		return fmt.Errorf("no package with tests was found under %s", root)
	}

	return nil
}

func (driver *Driver) requireEveryTestPackageGated() error {
	if len(driver.ungatedPackages) > 0 {
		return fmt.Errorf("packages whose tests no gate runs: %s", strings.Join(driver.ungatedPackages, ", "))
	}

	return nil
}
