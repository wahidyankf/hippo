package architecture_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// dependencyViolations applies capability rules to every source file, including
// mutually exclusive Darwin/Linux files; build selection cannot hide an edge.
func dependencyViolations(owner string, source []byte) ([]string, error) {
	parsed, err := parser.ParseFile(token.NewFileSet(), owner, source, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	var violations []string
	for _, declaration := range parsed.Imports {
		imported, quoteError := strconv.Unquote(declaration.Path.Value)
		if quoteError != nil {
			return nil, quoteError
		}
		if forbiddenImport(owner, imported) {
			violations = append(violations, fmt.Sprintf("%s imports %s", owner, imported))
		}
	}
	return violations, nil
}

func pureOwner(owner string) bool {
	return strings.HasPrefix(owner, "internal/domain/") || owner == "internal/policy" ||
		owner == "internal/identity" || owner == "internal/status"
}

func concreteIOImport(imported string) bool {
	return imported == "os" || strings.HasPrefix(imported, "os/") || imported == "syscall" ||
		imported == "unsafe" || imported == "net" || strings.HasPrefix(imported, "net/") && imported != "net/url" ||
		imported == "io/fs" || imported == "runtime" || imported == "golang.org/x/sys/unix" ||
		imported == "golang.org/x/term"
}

func forbiddenImport(owner, imported string) bool {
	const module = "github.com/wahidyankf/hippo/"
	if strings.HasPrefix(imported, "charm.land/") || strings.HasPrefix(imported, "github.com/charmbracelet/") {
		return owner != "internal/adapters/tui"
	}
	if imported == "github.com/spf13/cobra" || imported == "github.com/spf13/pflag" {
		return owner != "internal/adapters/cli"
	}
	application := strings.HasPrefix(owner, "internal/application")
	if pureOwner(owner) || application {
		if concreteIOImport(imported) {
			return true
		}
		if strings.HasPrefix(imported, module+"internal/") {
			target := strings.TrimPrefix(imported, module)
			inward := pureOwner(target) || application && strings.HasPrefix(target, "internal/application")
			if !inward {
				return true
			}
		}
	}
	if strings.HasPrefix(imported, module+"internal/bootstrap") && !strings.HasPrefix(owner, "cmd/") {
		return true
	}
	for _, legacy := range legacyPackages {
		if imported == module+"internal/"+legacy {
			return true
		}
	}
	return false
}

var legacyPackages = []string{"guard", "cli", "config", "host", "evidence", "release", "conformance"}

func TestProductionDependencies(t *testing.T) {
	root, err := os.OpenRoot(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeError := root.Close(); closeError != nil {
			t.Error(closeError)
		}
	})
	for _, legacy := range legacyPackages {
		if _, err := root.Stat(filepath.Join("internal", legacy)); err == nil {
			t.Errorf("mixed legacy package internal/%s remains", legacy)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	for _, owner := range []string{"internal/application", "internal/domain/coordination", "internal/domain/evidence", "internal/bootstrap"} {
		if _, err := root.Stat(owner); err != nil {
			t.Errorf("required migrated package %s: %v", owner, err)
		}
	}
	for _, tree := range []string{"internal", "cmd"} {
		err := fs.WalkDir(root.FS(), tree, func(name string, entry fs.DirEntry, walkError error) error {
			if walkError != nil {
				return walkError
			}
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				return nil
			}
			source, readError := root.ReadFile(name)
			if readError != nil {
				return readError
			}
			violations, parseError := dependencyViolations(path.Dir(name), source)
			if parseError != nil {
				return parseError
			}
			for _, violation := range violations {
				t.Errorf("%s: %s", name, violation)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestFixtureDependencies(t *testing.T) {
	cases := []struct {
		name, owner string
		forbidden   bool
	}{
		{"application-runtime", "internal/application", true},
		{"domain-filesystem", "internal/domain/coordination", true},
		{"application-cobra", "internal/application", true},
		{"application-charm", "internal/application", true},
		{"inward", "internal/application", false},
	}
	for _, fixture := range cases {
		t.Run(fixture.name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join("testdata", fixture.name+".go.txt"))
			if err != nil {
				t.Fatal(err)
			}
			violations, err := dependencyViolations(fixture.owner, source)
			if err != nil {
				t.Fatal(err)
			}
			if (len(violations) > 0) != fixture.forbidden {
				t.Fatalf("dependency fixture %s: forbidden=%t, violations=%v", fixture.name, fixture.forbidden, violations)
			}
		})
	}
}
