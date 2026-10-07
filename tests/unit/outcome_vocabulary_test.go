package unit_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/wahidyankf/hippo/internal/adapters/evidence"
)

// TestPublishedOutcomesMatchTheOutcomeList holds the closed outcome list in
// docs/reference/json-schemas.md to evidence.Outcomes(): the same ten words. An
// outcome added on one side only is one a consumer either cannot look up or
// cannot receive.
func TestPublishedOutcomesMatchTheOutcomeList(t *testing.T) {
	document, err := os.ReadFile(filepath.Join("..", "..", "docs", "reference", "json-schemas.md"))
	if err != nil {
		t.Fatal(err)
	}
	section := regexp.MustCompile("(?s)`outcome` is one of these values, and no other:\n(.*?)\n\n").FindSubmatch(document)
	if section == nil {
		t.Fatal("json-schemas.md no longer lists the values `outcome` can take")
	}
	rows := regexp.MustCompile("(?m)^- `([a-z]+(?:-[a-z]+)*)`$").FindAllSubmatch(section[1], -1)
	published := make([]string, 0, len(rows))
	for _, row := range rows {
		published = append(published, string(row[1]))
	}
	if len(published) == 0 {
		t.Fatal("json-schemas.md lists no outcome")
	}

	recorded := evidence.Outcomes()
	written := make([]string, 0, len(recorded))
	for _, outcome := range recorded {
		written = append(written, outcome.String())
	}
	for _, outcome := range written {
		if !slices.Contains(published, outcome) {
			t.Errorf("%s is recorded but not published in json-schemas.md", outcome)
		}
	}
	for _, outcome := range published {
		if !slices.Contains(written, outcome) {
			t.Errorf("json-schemas.md publishes %s, which hippo never records", outcome)
		}
	}
	if len(published) != len(written) {
		t.Errorf("json-schemas.md lists %d outcomes and hippo records %d: %v against %v", len(published), len(written), published, written)
	}
}
