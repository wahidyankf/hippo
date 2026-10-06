package support

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	fixtureModule  = "example.test/mod"
	fixturePackage = "example.test/mod/internal/fixture"
)

// analyzeFixture type-checks one in-memory package of the fixture module. Every fixture file imports nothing, so no
// importer is needed.
func analyzeFixture(t *testing.T, files map[string]string) []string {
	t.Helper()

	findings, err := analyzeDomainPackage(fixtureModule, domainPackage{importPath: fixturePackage, sources: files}, nil)
	if err != nil {
		t.Fatal(err)
	}

	return findingLines(findings)
}

// findingLines prints findings as the analysis reports them, one <path>:<line>: <rule>: <identifier> line each.
func findingLines(findings []domainFinding) []string {
	lines := make([]string, 0, len(findings))
	for _, finding := range findings {
		lines = append(lines, finding.String())
	}

	return lines
}

// lineOf returns the one-based line of the first occurrence of marker in source, so an expectation names the line the
// fixture put the violation on without counting lines by hand.
func lineOf(t *testing.T, source, marker string) int {
	t.Helper()

	offset := strings.Index(source, marker)
	if offset < 0 {
		t.Fatalf("fixture has no %q", marker)
	}

	return strings.Count(source[:offset], "\n") + 1
}

func requireFindings(t *testing.T, got, want []string) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Fatalf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func fixtureLine(path string, line int, rule, identifier string) string {
	return domainFinding{Path: path, Line: line, Rule: rule, Identifier: identifier}.String()
}

func TestDomainLiteralFlagsProfileComparison(t *testing.T) {
	const path = "internal/fixture/profile.go"
	source := `package fixture

type Resolution struct {
	ResolvedProfile string
}

func balanced(resolution Resolution) bool {
	return resolution.ResolvedProfile == "balanced"
}
`
	got := analyzeFixture(t, map[string]string{path: source})
	requireFindings(t, got, []string{
		fixtureLine(path, lineOf(t, source, "ResolvedProfile string"), ruleRawField, "ResolvedProfile"),
		fixtureLine(path, lineOf(t, source, `== "balanced"`), ruleLiteralComparison, "ResolvedProfile"),
	})
}

func TestDomainLiteralFlagsCaseOverProfile(t *testing.T) {
	const path = "internal/fixture/rank.go"
	source := `package fixture

func rank(profile string) int {
	switch profile {
	case "constrained":
		return 1
	case "":
		return 2
	}

	return 0
}
`
	got := analyzeFixture(t, map[string]string{path: source})
	requireFindings(t, got, []string{
		fixtureLine(path, lineOf(t, source, "profile string"), ruleRawField, "profile"),
		fixtureLine(path, lineOf(t, source, `"constrained"`), ruleLiteralComparison, "profile"),
	})
}

func TestDomainLiteralFlagsRawDomainFields(t *testing.T) {
	const path = "internal/fixture/summary.go"
	source := `package fixture

type Summary struct {
	Outcome     string
	Decision    bool
	Lineage     int
	Count       int
	Reason      string
	Path        string
	State       string
	outcomeFlag string
	Typed       Mode
}

type Mode string
`
	got := analyzeFixture(t, map[string]string{path: source})
	requireFindings(t, got, []string{
		fixtureLine(path, lineOf(t, source, "Outcome     string"), ruleRawField, "Outcome"),
		fixtureLine(path, lineOf(t, source, "Decision    bool"), ruleRawField, "Decision"),
		fixtureLine(path, lineOf(t, source, "Lineage     int"), ruleRawField, "Lineage"),
	})
}

func TestDomainLiteralFlagsRawDomainParameters(t *testing.T) {
	const path = "internal/fixture/admit.go"
	source := `package fixture

func admit(class string, count int) bool { return count > 0 }

type Admitter interface {
	Admit(budgetOutcome string) bool
}

var literal = func(AdmissionPath int, admitted bool) bool { return admitted }
`
	got := analyzeFixture(t, map[string]string{path: source})
	requireFindings(t, got, []string{
		fixtureLine(path, lineOf(t, source, "class string"), ruleRawField, "class"),
		fixtureLine(path, lineOf(t, source, "budgetOutcome string"), ruleRawField, "budgetOutcome"),
		fixtureLine(path, lineOf(t, source, "AdmissionPath int"), ruleRawField, "AdmissionPath"),
	})
}

func TestDomainLiteralFlagsDefinedTypeComparedWithLiteral(t *testing.T) {
	const path = "internal/fixture/mode.go"
	source := `package fixture

type Mode string

type Level int

const ModeFast Mode = "turbo"

func fast(m Mode) bool { return m == "fast" }

func high(l Level) bool { return 3 != l }

func named(m Mode) bool { return m == ModeFast }

func present(m Mode, l Level) bool { return m != "" || l == 0 }

func chosen(m Mode) int {
	switch m {
	case "fast":
		return 1
	case ModeFast:
		return 2
	}

	return 0
}
`
	got := analyzeFixture(t, map[string]string{path: source})
	requireFindings(t, got, []string{
		fixtureLine(path, lineOf(t, source, `m == "fast"`), ruleLiteralComparison, "m"),
		fixtureLine(path, lineOf(t, source, "3 != l"), ruleLiteralComparison, "l"),
		fixtureLine(path, lineOf(t, source, `case "fast"`), ruleLiteralComparison, "m"),
	})
}

func TestDomainLiteralIgnoresTestFilesOtherTreesAndUnlistedNames(t *testing.T) {
	files := map[string]string{
		"internal/fixture/unlisted.go": `package fixture

type Holder struct {
	Outcome fmtString
	Path    string
	State   string
	Reason  string
}

type fmtString interface{ String() string }

func compare(mode, outcomeFlag string, count int) bool {
	return mode == "fast" || outcomeFlag == "passed" || count == 4
}
`,
		"internal/fixture/violation_test.go": `package fixture

type Summary struct{ Outcome string }

func balanced(profile string) bool { return profile == "balanced" }
`,
		"tests/support/outside.go": `package fixture

type Summary struct{ Outcome string }
`,
	}

	requireFindings(t, analyzeFixture(t, files), nil)
}

func TestDomainLiteralAllowsZeroAndEmptyComparisons(t *testing.T) {
	const path = "internal/fixture/presence.go"
	source := `package fixture

type count int

type text string

func absent(outcome, class count) bool {
	return outcome == 0 || class != 0
}

func empty(profile text) bool {
	return profile == ""
}
`
	requireFindings(t, analyzeFixture(t, map[string]string{path: source}), nil)
}

func TestDomainLiteralRefusesPackagesThatDoNotTypeCheck(t *testing.T) {
	_, err := analyzeDomainPackage(fixtureModule, domainPackage{
		importPath: fixturePackage,
		sources:    map[string]string{"internal/fixture/broken.go": "package fixture\n\nvar broken = missing\n"},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "broken.go") {
		t.Fatalf("a package that does not type-check was analysed: %v", err)
	}
}

func TestDomainLiteralStepReportsEveryFindingAndNothingElse(t *testing.T) {
	driver := &Driver{}
	if err := driver.requireNoDomainLiteralFinding(); err != nil {
		t.Fatalf("no finding was refused: %v", err)
	}

	driver.domainFindings = []domainFinding{
		{Path: "internal/a/a.go", Line: 12, Rule: ruleRawField, Identifier: "Outcome"},
		{Path: "internal/a/a.go", Line: 30, Rule: ruleLiteralComparison, Identifier: "profile"},
	}
	err := driver.requireNoDomainLiteralFinding()
	if err == nil {
		t.Fatal("findings were accepted")
	}

	for _, line := range []string{
		"found 2 violations",
		"internal/a/a.go:12: raw-field: Outcome",
		"internal/a/a.go:30: literal-comparison: profile",
	} {
		if !strings.Contains(err.Error(), line) {
			t.Errorf("the refusal does not name %q: %v", line, err)
		}
	}
}

func TestDomainLiteralLoadsAModuleThroughExportData(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.test/loader\n\ngo 1.26.1\n",
		"internal/probe/probe.go": `package probe

import "strings"

func Check(profile string) bool {
	return strings.ToLower(profile) == "balanced" || profile == "minimal"
}
`,
		"internal/probe/probe_test.go": "package probe\n\nfunc violation(profile string) bool { return profile == \"x\" }\n",
		"cmd/tool/main.go":             "package main\n\ntype run struct{ Outcome string }\n\nfunc main() { _ = run{} }\n",
		"scripts/tool/tool.go":         "package tool\n\ntype Summary struct{ Outcome string }\n",
	}
	for name, contents := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	findings, err := analyzeDomainLiterals(root)
	if err != nil {
		t.Fatal(err)
	}

	source := files["internal/probe/probe.go"]
	requireFindings(t, findingLines(findings), []string{
		fixtureLine("cmd/tool/main.go", 3, ruleRawField, "Outcome"),
		fixtureLine("internal/probe/probe.go", lineOf(t, source, "profile string"), ruleRawField, "profile"),
		fixtureLine("internal/probe/probe.go", lineOf(t, source, `profile == "minimal"`), ruleLiteralComparison, "profile"),
	})
}

func TestDomainLiteralFailsWhereNoModuleLoads(t *testing.T) {
	if _, err := analyzeDomainLiterals(t.TempDir()); err == nil {
		t.Fatal("a directory without a module was analysed")
	}
}
