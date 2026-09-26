package support

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// helpExitLine is one status line of the --help exit block: the status, then
// what it means.
var helpExitLine = regexp.MustCompile(`^\s+([0-9]+)\s+(.+)$`)

// requireHelpExitBlockMatchesReference holds --help to the published exit
// table. The exit block is the one place a caller reads the contract without
// the documentation, so every numbered line must name a status the reference
// publishes and open with the meaning the reference gives it, and every
// published status must have its line.
func (driver *Driver) requireHelpExitBlockMatchesReference() error {
	if driver.exitCode != 0 {
		return fmt.Errorf("help exited %d: %s", driver.exitCode, driver.errorOutput)
	}
	document, err := os.ReadFile(filepath.Join(toolRoot(), "docs", "reference", "exit-codes.md"))
	if err != nil {
		return err
	}
	// Only the exit-status table publishes the vocabulary; later tables map
	// retired numbers onto it and must not be read as statuses.
	_, table, found := strings.Cut(string(document), "## Exit statuses\n")
	if !found {
		return errors.New("the exit-code reference has no exit-status section")
	}
	table, _, _ = strings.Cut(table, "\n## ")
	rows := map[string]string{}
	for _, row := range regexp.MustCompile("(?m)^\\| `([0-9]+)` +\\| ([^|]+?) +\\|").FindAllStringSubmatch(table, -1) {
		rows[row[1]] = row[2]
	}
	if len(rows) == 0 {
		return errors.New("the exit-code reference publishes no statuses")
	}

	_, block, hasBlock := strings.Cut(driver.output, "Exit statuses:\n")
	if !hasBlock {
		return fmt.Errorf("help has no exit block: %q", driver.output)
	}
	block, _, _ = strings.Cut(block, "\n\n")
	seen := map[string]bool{}
	for line := range strings.SplitSeq(block, "\n") {
		match := helpExitLine.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		meaning, published := rows[match[1]]
		if !published {
			return fmt.Errorf("help lists status %s, which the exit-code reference does not publish", match[1])
		}
		if !strings.HasPrefix(strings.ToLower(match[2]), strings.ToLower(meaning)) {
			return fmt.Errorf("help says %s %q; the exit-code reference says %q", match[1], match[2], meaning)
		}
		seen[match[1]] = true
	}
	for status := range rows {
		if !seen[status] {
			return fmt.Errorf("help has no line for published status %s", status)
		}
	}

	return nil
}
