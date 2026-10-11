package support

import (
	"fmt"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// This static guard needs no nested Go invocation. The independently retained
// go list test closure must also prove Standard for every outward dependency.
func TestPublicTUIHostHelperLeanBoundary(t *testing.T) {
	if err := validatePublicHostHelperSource("hostprobe"); err != nil {
		t.Fatalf("lean host-query boundary is not available: %v", err)
	}
}

func validatePublicHostHelperSource(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read dedicated stdlib-only helper package: %w", err)
	}
	for _, name := range []string{"protocol.go", "client.go", "helper_test.go", "protocol_test.go", "README.md"} {
		info, statError := os.Lstat(filepath.Join(directory, name))
		if statError != nil || info == nil || !info.Mode().IsRegular() {
			return fmt.Errorf("required lean helper source %s is not regular: %w", name, statError)
		}
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		file, parseError := parser.ParseFile(token.NewFileSet(), filepath.Join(directory, entry.Name()), nil, parser.ImportsOnly)
		if parseError != nil {
			return parseError
		}
		for _, imported := range file.Imports {
			name, unquoteError := strconv.Unquote(imported.Path.Value)
			if unquoteError != nil {
				return unquoteError
			}
			if name == "github.com/wahidyankf/hippo/tests/support/hostprobe" {
				continue // The lean package's own external test variant is allowed.
			}
			// Module paths cannot masquerade as standard-library packages. The
			// real Go9 source directory check also refuses invented local names.
			if strings.Contains(strings.Split(name, "/")[0], ".") || !filepath.IsLocal(name) {
				return fmt.Errorf("helper source %s imports outside the standard library: %s", entry.Name(), name)
			}
			standard, importError := build.Default.Import(name, "", build.FindOnly)
			if importError != nil || !standard.Goroot {
				return fmt.Errorf("helper source %s imports a nonstandard package: %s", entry.Name(), name)
			}
		}
	}
	return nil
}

func TestPublicTUIHostHelperStaticBoundaryRefusesHeavyImports(t *testing.T) {
	for _, imported := range []string{
		"github.com/wahidyankf/hippo/tests/support",
		"github.com/wahidyankf/hippo/internal/adapters/tui",
		"charm.land/lipgloss/v2", "github.com/mattn/go-runewidth", "invented/local",
	} {
		t.Run(imported, func(t *testing.T) {
			directory := t.TempDir()
			for _, name := range []string{"protocol.go", "client.go", "helper_test.go", "protocol_test.go"} {
				if err := os.WriteFile(filepath.Join(directory, name), []byte("package hostprobe\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(directory, "README.md"), []byte("Import refusal fixture.\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "client.go"), []byte("package hostprobe\nimport "+strconv.Quote(imported)+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := validatePublicHostHelperSource(directory); err == nil {
				t.Fatal("heavy or invented helper dependency escaped the static boundary")
			}
		})
	}
}
