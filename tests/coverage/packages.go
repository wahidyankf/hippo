package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
)

// reportPackages prints one statement figure for each named import path and the
// figure over all of them together. It fails when the aggregate or any package
// that holds statements falls below the minimum, and it names every such package.
// A package whose profile holds no block has no executable statements (only
// declarations), so it is printed and not gated. The go list selection gives
// -coverpkg the same packages, so a package that no test reaches appears with
// zero covered statements rather than being absent.
func reportPackages(profilePath, list string, minimum float64) int {
	selected := map[string]bool{}
	for name := range strings.SplitSeq(list, ",") {
		if cleaned := strings.TrimSpace(name); cleaned != "" {
			selected[cleaned] = true
		}
	}
	if len(selected) == 0 {
		_, _ = fmt.Fprintln(os.Stderr, "coverage: no packages selected")
		return 1
	}

	input, err := os.Open(profilePath)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "coverage:", err)
		return 1
	}
	defer func() { _ = input.Close() }()

	counts, err := countPackages(input, selected)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "coverage:", err)
		return 1
	}

	names := make([]string, 0, len(selected))
	for name := range selected {
		names = append(names, name)
	}
	sort.Strings(names)

	var total statementCoverage
	var below []string
	for _, name := range names {
		count := counts[name]
		total.covered += count.covered
		total.total += count.total
		if count.total == 0 {
			_, _ = fmt.Fprintf(os.Stdout, "package %s: no executable statements\n", name)
			continue
		}
		percentage := count.percentage()
		_, _ = fmt.Fprintf(os.Stdout, "package %s: %.4f%% (%d/%d statements)\n", name, percentage, count.covered, count.total)
		if percentage < minimum {
			below = append(below, name)
		}
	}
	if total.total == 0 {
		_, _ = fmt.Fprintln(os.Stderr, "coverage: the selected packages hold no statements")
		return 1
	}

	aggregate := total.percentage()
	_, _ = fmt.Fprintf(os.Stdout, "selected production line coverage: %.4f%% (%d/%d statements)\n", aggregate, total.covered, total.total)
	if len(below) > 0 {
		_, _ = fmt.Fprintf(os.Stderr, "packages below %g%%: %s\n", minimum, strings.Join(below, ", "))
	}
	if aggregate < minimum || len(below) > 0 {
		return 1
	}

	return 0
}

// countPackages unions the blocks of the profile by import path. A block belongs
// to the package whose import path is the directory of its file, and a block
// counts once however many test binaries record it.
func countPackages(input io.Reader, selected map[string]bool) (map[string]statementCoverage, error) {
	owners := map[string]string{}
	blocks := map[string]coverageBlock{}
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 || fields[0] == "mode:" {
			continue
		}
		location := fields[0]
		colon := strings.LastIndex(location, ":")
		if colon < 0 {
			continue
		}
		owner := path.Dir(location[:colon])
		if !selected[owner] {
			continue
		}
		statements, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, err
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, err
		}
		block, exists := blocks[location]
		if exists && block.statements != statements {
			return nil, errors.New("inconsistent coverage block statement count for " + location)
		}
		blocks[location] = coverageBlock{statements: statements, covered: block.covered || count > 0}
		owners[location] = owner
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	counts := map[string]statementCoverage{}
	for location, block := range blocks {
		count := counts[owners[location]]
		count.total += block.statements
		if block.covered {
			count.covered += block.statements
		}
		counts[owners[location]] = count
	}

	return counts, nil
}

func (count statementCoverage) percentage() float64 {
	return float64(count.covered) * 100 / float64(count.total)
}
