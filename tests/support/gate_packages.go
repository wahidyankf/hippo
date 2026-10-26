package support

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/wahidyankf/hippo/tests/contract"
)

// testRunningScripts are every script a gate runs `go test` from. A package
// none of them names is compiled by the gate but its tests never execute.
var testRunningScripts = []string{
	filepath.Join("scripts", "test-quick.sh"),
	filepath.Join("scripts", "test.sh"),
	filepath.Join("tests", "e2e", "run.sh"),
	filepath.Join("tests", "e2e", "mixed-version.sh"),
}

// goTestInvocation finds a `go test` command, including the end-to-end
// script's configurable Go binary.
var goTestInvocation = regexp.MustCompile(`(^|\s)(go|"\$go_binary") test\s`)

// gatedTestPatterns returns the package patterns the gates pass to `go test`
// with tests enabled, joining continued shell lines first.
func gatedTestPatterns(root string) ([]string, error) {
	var patterns []string
	for _, script := range testRunningScripts {
		data, err := os.ReadFile(filepath.Join(root, script))
		if err != nil {
			return nil, err
		}
		calls, valid := gateTestCommands(string(data))
		if !valid {
			return nil, fmt.Errorf("invalid go test gate command in %s", script)
		}
		for _, call := range calls {
			skip := gateFlag(call.args, "-skip")
			if gateFlag(call.args, "-run") == "^$" || slices.Contains([]string{".*", "^.*", ".*$", "^.*$"}, skip) {
				continue
			}
			for _, field := range call.args {
				if strings.HasPrefix(field, "./") {
					patterns = append(patterns, field)
				}
			}
		}
	}

	return patterns, nil
}

// gateCommandWords accepts only literal single commands, with the two owned
// E2E wrapper expansions. Quoted regexp alternatives are words, not pipelines;
// unquoted shell operators or dynamic selectors cannot count as gate evidence.
func gateCommandWords(line string) ([]string, bool) {
	parser := gateWordParser{}
	for index := 0; index < len(line); index++ {
		character := line[index]
		if parser.quote == '\'' {
			parser.singleQuoted(character)
			continue
		}
		if character == '\\' {
			var valid bool
			index, valid = parser.escape(line, index)
			if !valid {
				return nil, false
			}
			continue
		}
		if parser.quote == '"' {
			parser.doubleQuoted(line, index)
			continue
		}
		valid, done := parser.unquoted(line, index)
		if !valid {
			return nil, false
		}
		if done {
			return parser.words, len(parser.words) > 0
		}
	}
	if parser.quote != 0 || !parser.finish() {
		return nil, false
	}
	return parser.words, len(parser.words) > 0
}

type gateWordParser struct {
	words            []string
	word             strings.Builder
	quote            byte
	started, dynamic bool
}

func (parser *gateWordParser) finish() bool {
	if !parser.started {
		return true
	}
	value := parser.word.String()
	if parser.dynamic && value != "$go_binary" && value != "HIPPO_BIN=$temporary_dir/hippo" {
		return false
	}
	parser.words = append(parser.words, value)
	parser.word.Reset()
	parser.started, parser.dynamic = false, false
	return true
}

func (parser *gateWordParser) singleQuoted(character byte) {
	if character == '\'' {
		parser.quote = 0
		return
	}
	parser.word.WriteByte(character)
}

func (parser *gateWordParser) doubleQuoted(line string, index int) {
	character := line[index]
	if character == '"' {
		parser.quote = 0
		return
	}
	if character == '`' || (character == '$' && gateDollarExpands(line, index, parser.quote)) {
		parser.dynamic = true
	}
	parser.word.WriteByte(character)
}

func (parser *gateWordParser) escape(line string, index int) (int, bool) {
	if index+1 >= len(line) {
		return index, false
	}
	// Preserve shell's literal backslash before non-special double-quoted bytes.
	if parser.quote == '"' && !strings.ContainsRune("$`\"\\", rune(line[index+1])) {
		parser.word.WriteByte(line[index])
		return index, true
	}
	index++
	parser.word.WriteByte(line[index])
	parser.started = true
	return index, true
}

func (parser *gateWordParser) unquoted(line string, index int) (bool, bool) {
	character := line[index]
	switch character {
	case '\'', '"':
		parser.quote, parser.started = character, true
	case ' ', '\t', '\r':
		if !parser.finish() {
			return false, false
		}
	case '#':
		if !parser.started {
			return parser.finish(), true
		}
		parser.word.WriteByte(character)
	case '|', '&', ';', '<', '>', '(', ')':
		return false, false
	case '$':
		parser.dynamic = parser.dynamic || gateDollarExpands(line, index, parser.quote)
		parser.started = true
		parser.word.WriteByte(character)
	case '`':
		parser.dynamic, parser.started = true, true
		parser.word.WriteByte(character)
	default:
		parser.started = true
		parser.word.WriteByte(character)
	}
	return true, false
}

// gateDollarExpands distinguishes a literal regexp end anchor from shell
// parameter, command and arithmetic expansion. Unsupported dollar quoting is
// rejected outside quotes; a dollar before a closing double quote is literal.
func gateDollarExpands(line string, index int, quote byte) bool {
	if index+1 >= len(line) {
		return false
	}
	next := line[index+1]
	if next == '_' || (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') || (next >= '0' && next <= '9') {
		return true
	}
	if strings.ContainsRune("{([@*#?$!-", rune(next)) {
		return true
	}
	return quote == 0 && (next == '\'' || next == '"')
}

type gateTestCommand struct {
	args                   []string
	suite, adapter, binary string
}

func gateTestCommands(script string) ([]gateTestCommand, bool) {
	var calls []gateTestCommand
	for line := range strings.SplitSeq(strings.ReplaceAll(script, "\\\n", " "), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || !goTestInvocation.MatchString(line) {
			continue
		}
		words, valid := gateCommandWords(line)
		if !valid {
			return nil, false
		}
		call := gateTestCommand{}
		index := 0
		for index < len(words) && strings.Contains(words[index], "=") && !strings.HasPrefix(words[index], "-") {
			name, value, _ := strings.Cut(words[index], "=")
			switch name {
			case "HIPPO_BDD_SUITE":
				call.suite = value
			case "HIPPO_BDD_ADAPTER":
				call.adapter = value
			case "HIPPO_BIN":
				call.binary = value
			}
			index++
		}
		if index+1 >= len(words) || (words[index] != "go" && words[index] != "$go_binary") || words[index+1] != "test" {
			return nil, false
		}
		call.args = words[index+2:]
		calls = append(calls, call)
	}
	return calls, true
}

func gateFlagValues(words []string, name string) []string {
	name = strings.TrimSuffix(name, "=")
	if strings.HasPrefix(name, "--") {
		name = name[1:]
	}
	var values []string
	for index := 0; index < len(words); index++ {
		argument := words[index]
		if strings.HasPrefix(argument, "--") {
			argument = argument[1:]
		}
		if suffix, found := strings.CutPrefix(argument, "-test."); found {
			argument = "-" + suffix
		}
		if value, found := strings.CutPrefix(argument, name+"="); found {
			values = append(values, value)
		} else if argument == name {
			if index+1 >= len(words) {
				return []string{"invalid selector"}
			}
			index++
			values = append(values, words[index])
		}
	}
	return values
}

func gateFlag(words []string, name string) string {
	values := gateFlagValues(words, name)
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

func gateOrdinarySelection(words, excluded []string) bool {
	run, skip := gateFlagValues(words, "-run"), gateFlagValues(words, "-skip")
	if len(run) > 1 || (len(run) == 1 && run[0] != "" && run[0] != ".*" && run[0] != "^.*$") || len(skip) != 1 {
		return false
	}
	inner, begin := strings.CutPrefix(skip[0], "^(")
	inner, end := strings.CutSuffix(inner, ")$")
	if !begin || !end {
		return false
	}
	actual := strings.Split(inner, "|")
	wanted := append([]string(nil), excluded...)
	slices.Sort(actual)
	slices.Sort(wanted)
	return slices.Equal(actual, wanted)
}

func gateCorpusEntries() []string {
	return []string{"TestUnitBehaviours", "TestIntegrationBehaviours", e2eBehaviourEntry, "TestFeatureCompliance"}
}

// gatePublicEntries names direct compiled-only product tests and task diagnostics.
// Ordinary pure and helper controls remain selected; these exact names do not.
func gatePublicEntries() []string {
	return []string{}
}

func gateOrdinaryEntries() []string {
	return append(gateCorpusEntries(), gatePublicEntries()...)
}

func gateMeasuredEntries() []string {
	return append(gateCorpusEntries()[1:], gatePublicEntries()...)
}

func gateRequiredPublicEntries() []string {
	return []string{e2eBehaviourEntry}
}

func gateExecutingEntry(call gateTestCommand, suite, entry string, race bool) bool {
	return call.suite == suite && slices.Contains(call.args, "./tests/bdd") &&
		len(gateFlagValues(call.args, "-c")) == 0 && len(gateFlagValues(call.args, "-list")) == 0 &&
		((race && slices.Contains(call.args, "-race")) || (!race && len(gateFlagValues(call.args, "-race")) == 0)) &&
		len(gateFlagValues(call.args, "-run")) == 1 &&
		gateFlag(call.args, "-run") == "^"+entry+"$" && len(gateFlagValues(call.args, "-skip")) == 0
}

// checkStandaloneGatePublicEntries validates the compiled script's public
// behavior entries: one owned old-release preparation, then each required
// entry exactly once against the built candidate.
func checkStandaloneGatePublicEntries(compiled string) error {
	if !gateHasOldReleasePreparation(compiled) {
		return errors.New("compiled public journey lacks owned old preparation")
	}
	calls, valid := gateTestCommands(compiled)
	if !valid {
		return errors.New("compiled gate contains an invalid test command")
	}
	entries := gateRequiredPublicEntries()
	seen := make(map[string]int, len(entries))
	for _, call := range calls {
		if !slices.Contains(call.args, "./tests/bdd") {
			continue
		}
		entry := ""
		for _, required := range entries {
			if gateExecutingEntry(call, contract.E2E, required, false) {
				entry = required
			}
		}
		if entry == "" || call.binary != "$temporary_dir/hippo" ||
			len(gateFlagValues(call.args, "-count")) != 1 || gateFlag(call.args, "-count") != "1" ||
			call.adapter != "" {
			return errors.New("compiled candidate contains a nonexecuting or unowned public entry")
		}
		for _, argument := range call.args {
			if strings.HasPrefix(argument, "./") && argument != "./tests/bdd" {
				return errors.New("compiled public entry contains another package selector")
			}
		}
		seen[entry]++
	}
	for _, entry := range entries {
		if seen[entry] != 1 {
			return fmt.Errorf("required public entry %s executions = %d, want one", entry, seen[entry])
		}
	}
	return nil
}

func gateHasOldReleasePreparation(script string) bool {
	count := 0
	for line := range strings.SplitSeq(strings.ReplaceAll(script, "\\\n", " "), "\n") {
		line = strings.TrimSpace(line)
		if line == `public_old_release_prepare "$tool_dir" "$temporary_dir"` {
			count++
		}
		if goTestInvocation.MatchString(line) && !strings.HasPrefix(line, "#") && count != 1 {
			return false
		}
	}
	return count == 1
}

func gateHasE2ECommand(script string) bool {
	for line := range strings.SplitSeq(strings.ReplaceAll(script, "\\\n", " "), "\n") {
		words, valid := gateCommandWords(strings.TrimSpace(line))
		if valid && len(words) == 1 && words[0] == "./tests/e2e/run.sh" {
			return true
		}
	}
	return false
}

func validSerialComposedCompliance(script string) bool {
	calls, valid := gateTestCommands(script)
	if !valid {
		return false
	}
	var adapters []string
	for _, call := range calls {
		if call.adapter == "" {
			continue
		}
		if !slices.Contains(call.args, "./tests/bdd") || len(gateFlagValues(call.args, "-run")) != 1 ||
			gateFlag(call.args, "-run") != "^TestFeatureCompliance$" || len(gateFlagValues(call.args, "-skip")) != 0 || call.binary != "" {
			return false
		}
		adapters = append(adapters, call.adapter)
	}
	return slices.Equal(adapters, []string{"unit", "integration", "e2e"})
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
			// A directory holding its own go.mod, such as a task worktree, is
			// another module: ./... stops there, so no gate pattern here can run
			// its tests and they are not this module's to count.
			if path != root {
				if _, statError := os.Stat(filepath.Join(path, "go.mod")); statError == nil {
					return filepath.SkipDir
				}
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
	repository, rootError := toolRoot()
	if rootError != nil {
		return rootError
	}
	root := repository
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
