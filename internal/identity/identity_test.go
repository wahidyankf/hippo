package identity_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wahidyankf/hippo/internal/identity"
)

func TestLoadAndMerge(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "hippo.identity.json")
	data := []byte(`{
  "schemaVersion": 1,
  "source": "ose-public",
  "tags": {"group": "ose", "surface": "repository"}
}
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := identity.Load(path, "", []string{"surface=plan", "runner=codex", "runner=terra"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "ose-public" {
		t.Fatalf("source = %q", got.Source)
	}
	if got.Tags["group"] != "ose" || got.Tags["surface"] != "plan" || got.Tags["runner"] != "terra" {
		t.Fatalf("unexpected merged tags: %#v", got.Tags)
	}
}

func TestOverrideSourceWithoutFile(t *testing.T) {
	t.Parallel()

	got, err := identity.Load(filepath.Join(t.TempDir(), "missing.json"), "fixture-source", []string{"group=local"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "fixture-source" || got.Tags["group"] != "local" {
		t.Fatalf("unexpected identity: %#v", got)
	}
}

func TestRejectsInvalidIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		source   string
		tags     []string
		contains string
	}{
		{name: "missing source", contains: "source is required"},
		{name: "path source", source: "../repo", contains: "source"},
		{name: "path value", source: "repo", tags: []string{"path=../secret"}, contains: "tag value"},
		{name: "invalid key", source: "repo", tags: []string{"Bad Key=value"}, contains: "tag key"},
		{name: "too many tags", source: "repo", tags: []string{
			"a=1", "b=2", "c=3", "d=4", "e=5", "f=6", "g=7", "h=8", "i=9",
		}, contains: "at most 8 tags"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := identity.Load(filepath.Join(t.TempDir(), "missing.json"), test.source, test.tags)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("error = %v, want containing %q", err, test.contains)
			}
		})
	}
}

func TestRejectsDuplicateAndOversizedFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
	}{
		{name: "duplicate", data: `{"schemaVersion":1,"source":"one","source":"two"}`},
		{name: "oversized", data: `{"schemaVersion":1,"source":"repo","tags":{"note":"` + strings.Repeat("x", 500) + `"}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "hippo.identity.json")
			if err := os.WriteFile(path, []byte(test.data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := identity.Load(path, "", nil); err == nil {
				t.Fatal("expected invalid identity")
			}
		})
	}
}
