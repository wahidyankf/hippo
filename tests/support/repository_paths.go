package support

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const hippoModulePath = "github.com/wahidyankf/hippo"

// repositoryRootAt selects the nearest module and refuses to cross its boundary.
func repositoryRootAt(start string) (string, error) {
	directory, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve repository start: %w", err)
	}
	entry, err := os.Stat(directory)
	if err != nil {
		return "", fmt.Errorf("inspect repository start: %w", err)
	}
	if !entry.IsDir() {
		return "", fmt.Errorf("repository start %q is not a directory", directory)
	}
	for {
		modulePath := filepath.Join(directory, "go.mod")
		data, readError := os.ReadFile(modulePath)
		if readError == nil {
			if err := validateHIPPORepository(directory, data); err != nil {
				return "", err
			}
			return directory, nil
		}
		if !errors.Is(readError, os.ErrNotExist) {
			return "", fmt.Errorf("read nearest module %q: %w", modulePath, readError)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", fmt.Errorf("no module root above %q", start)
		}
		directory = parent
	}
}

func validateHIPPORepository(root string, moduleData []byte) error {
	module := ""
	for line := range strings.SplitSeq(string(moduleData), "\n") {
		line, _, _ = strings.Cut(line, "//")
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "module" {
			continue
		}
		if len(fields) != 2 || module != "" {
			return fmt.Errorf("nearest module %q has an invalid module directive", root)
		}
		module = fields[1]
		if strings.HasPrefix(module, `"`) || strings.HasPrefix(module, "`") {
			decoded, err := strconv.Unquote(module)
			if err != nil {
				return fmt.Errorf("read nearest module identity in %q: %w", root, err)
			}
			module = decoded
		}
	}
	if module != hippoModulePath {
		return fmt.Errorf("nearest module %q names %q, require %q", root, module, hippoModulePath)
	}
	corpus := filepath.Join(root, "specs", "behaviours")
	entry, err := os.Stat(corpus)
	if err != nil {
		return fmt.Errorf("inspect nearest module canonical corpus: %w", err)
	}
	if !entry.IsDir() {
		return fmt.Errorf("nearest module canonical corpus %q is not a directory", corpus)
	}
	return nil
}

func toolRoot() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return repositoryRootAt(directory)
}
