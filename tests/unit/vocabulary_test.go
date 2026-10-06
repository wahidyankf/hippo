package unit_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

// TestRetryableMarksOnlyTheCodesWhoseConditionLiftsOnItsOwn holds status.Retryable to the two codes whose condition
// can clear without the caller changing anything. The table behind it lists every code, false ones included, so a
// new code needs a decision; this test is the decision a flipped or newly added true entry must justify.
func TestRetryableMarksOnlyTheCodesWhoseConditionLiftsOnItsOwn(t *testing.T) {
	retryable := []status.Code{status.CodeLimitCapacityDeferred, status.CodeLimitPressureShed}
	for _, code := range status.All {
		if got, want := status.Retryable(code), slices.Contains(retryable, code); got != want {
			t.Errorf("Retryable(%s) = %t, want %t", code, got, want)
		}
	}
	for _, code := range retryable {
		if !slices.Contains(status.All, code) {
			t.Errorf("%s is expected to be retryable but is not in status.All", code)
		}
	}
}
