package unit_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/wahidyankf/hippo/internal/status"
)

// TestPublishedVocabularyMatchesTheCodeTable holds the closed error-code list
// in docs/reference/exit-codes.md to status.All: the same codes, each with the
// status the code returns. A code added on one side only is a code a caller
// either cannot look up or cannot receive.
func TestPublishedVocabularyMatchesTheCodeTable(t *testing.T) {
	document, err := os.ReadFile(filepath.Join("..", "..", "docs", "reference", "exit-codes.md"))
	if err != nil {
		t.Fatal(err)
	}
	published := map[status.Code]int{}
	for _, row := range regexp.MustCompile("(?m)^\\| `(hippo\\.[a-z]+\\.[a-z-]+)` +\\| `([0-9]+)` +\\|").FindAllStringSubmatch(string(document), -1) {
		value, convertError := strconv.Atoi(row[2])
		if convertError != nil {
			t.Fatal(convertError)
		}
		published[status.Code(row[1])] = value
	}
	if len(published) == 0 {
		t.Fatal("exit-codes.md publishes no error codes")
	}
	for _, code := range status.All {
		documented, found := published[code]
		if !found {
			t.Errorf("%s is returned but not published in exit-codes.md", code)
			continue
		}
		if documented != status.Status(code) {
			t.Errorf("%s returns %d but exit-codes.md says %d", code, status.Status(code), documented)
		}
		delete(published, code)
	}
	for code := range published {
		t.Errorf("exit-codes.md publishes %s, which hippo never returns", code)
	}
}
