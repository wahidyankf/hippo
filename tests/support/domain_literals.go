package support

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// The domain literal analysis refuses two shapes in production code under cmd/
// and internal/, so a domain concept cannot be carried as a bare string,
// integer, or boolean and then keyed on by name:
//
//   - literal-comparison: an == or != between a domain value and a string or
//     integer literal, or a case clause listing such a literal in a switch over
//     a domain value. A domain value is an expression whose type is a defined
//     string or integer type declared in this module, or a variable, field, or
//     parameter whose name is on the list below. The empty string and zero are
//     not refused: they test presence, not identity.
//   - raw-field: a struct field or function parameter whose name is on the
//     list, declared as string, int, or bool.
//
// Files a build constraint excludes on the running platform are not analysed;
// internal/adapters/host holds the platform-specific collector files and no domain name. Outside both
// rules, and so not refused: a negative literal such as -1, a rune literal, a
// comparison through a conversion such as string(x), and a domain name declared
// as a named result.
const (
	ruleLiteralComparison = "literal-comparison"
	ruleRawField          = "raw-field"
)

// domainNames are the identifiers that name a domain concept, matched as whole
// identifiers without regard to case. Path, Reason, and State are left off on
// purpose: they name file paths, human-readable text, and receipt words this
// module does not type. Raw command-line text lives in fields named ...Flag,
// which the list does not match.
var domainNames = []string{
	"Outcome",
	"BudgetOutcome",
	"Profile",
	"ResolvedProfile",
	"RequestedProfile",
	"Class",
	"TaskClass",
	"Decision",
	"Lineage",
	"AdmissionPath",
}

func isDomainName(name string) bool {
	return slices.ContainsFunc(domainNames, func(domain string) bool { return strings.EqualFold(domain, name) })
}

// domainFinding is one violation.
type domainFinding struct {
	Path       string
	Line       int
	Rule       string
	Identifier string
}

func (finding domainFinding) String() string {
	return fmt.Sprintf("%s:%d: %s: %s", finding.Path, finding.Line, finding.Rule, finding.Identifier)
}

func compareDomainFindings(left, right domainFinding) int {
	return cmp.Or(
		cmp.Compare(left.Path, right.Path),
		cmp.Compare(left.Line, right.Line),
		cmp.Compare(left.Rule, right.Rule),
		cmp.Compare(left.Identifier, right.Identifier),
	)
}

// domainPackage is one package's production sources, keyed by module-relative
// slash path.
type domainPackage struct {
	importPath string
	sources    map[string]string
}

// analysedDomainPath admits the production files the analysis reads.
func analysedDomainPath(path string) bool {
	return !strings.HasSuffix(path, "_test.go") && (strings.HasPrefix(path, "cmd/") || strings.HasPrefix(path, "internal/"))
}

// analyzeDomainPackage parses and type-checks one package, then returns its
// findings sorted by path and line. A package that does not type-check is an
// error: a partial analysis would read as a clean one.
func analyzeDomainPackage(modulePath string, pkg domainPackage, imports types.Importer) ([]domainFinding, error) {
	fset := token.NewFileSet()

	var files []*ast.File

	for _, name := range slices.Sorted(maps.Keys(pkg.sources)) {
		if !analysedDomainPath(name) {
			continue
		}

		file, err := parser.ParseFile(fset, name, pkg.sources[name], parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}

		files = append(files, file)
	}

	if len(files) == 0 {
		return nil, nil
	}

	info := &types.Info{Types: make(map[ast.Expr]types.TypeAndValue), Uses: make(map[*ast.Ident]types.Object)}

	var failures []error

	checker := types.Config{Importer: imports, Error: func(err error) { failures = append(failures, err) }}
	if _, err := checker.Check(pkg.importPath, fset, files, info); err != nil {
		return nil, fmt.Errorf("type-check %s: %w", pkg.importPath, errors.Join(failures...))
	}

	walker := &domainWalker{fset: fset, info: info, modulePath: modulePath}
	for _, file := range files {
		walker.walk(file)
	}

	slices.SortFunc(walker.findings, compareDomainFindings)

	return walker.findings, nil
}

// domainWalker visits one package's syntax with its type information.
type domainWalker struct {
	fset       *token.FileSet
	info       *types.Info
	modulePath string
	findings   []domainFinding
}

func (walker *domainWalker) walk(file *ast.File) {
	ast.Inspect(file, func(node ast.Node) bool {
		if node != nil {
			walker.visit(node)
		}

		return true
	})
}

func (walker *domainWalker) visit(node ast.Node) {
	switch typed := node.(type) {
	case *ast.StructType:
		walker.checkRawNames(typed.Fields)
	case *ast.FuncType:
		walker.checkRawNames(typed.Params)
	case *ast.BinaryExpr:
		walker.checkComparison(typed)
	case *ast.SwitchStmt:
		walker.checkSwitch(typed)
	}
}

// checkRawNames reports each field or parameter that carries a domain name as a
// raw string, int, or bool, whichever list it is in.
func (walker *domainWalker) checkRawNames(list *ast.FieldList) {
	if list == nil {
		return
	}

	for _, field := range list.List {
		if !walker.rawType(field.Type) {
			continue
		}

		for _, name := range field.Names {
			if isDomainName(name.Name) {
				walker.add(name.Pos(), ruleRawField, name.Name)
			}
		}
	}
}

// rawType reports whether the type expression is the predeclared string, int,
// or bool, not a defined type or a shadowing declaration of the same name.
func (walker *domainWalker) rawType(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok || (ident.Name != "string" && ident.Name != "int" && ident.Name != "bool") {
		return false
	}

	return walker.info.Uses[ident] == types.Universe.Lookup(ident.Name)
}

func (walker *domainWalker) checkComparison(comparison *ast.BinaryExpr) {
	if comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return
	}

	if identityLiteral(comparison.Y) {
		walker.checkDomainOperand(comparison.Pos(), comparison.X)
	}

	if identityLiteral(comparison.X) {
		walker.checkDomainOperand(comparison.Pos(), comparison.Y)
	}
}

func (walker *domainWalker) checkDomainOperand(position token.Pos, operand ast.Expr) {
	if name, ok := walker.domainValue(operand); ok {
		walker.add(position, ruleLiteralComparison, name)
	}
}

func (walker *domainWalker) checkSwitch(statement *ast.SwitchStmt) {
	if statement.Tag == nil {
		return
	}

	name, ok := walker.domainValue(statement.Tag)
	if !ok {
		return
	}

	for _, clause := range statement.Body.List {
		cases, isCase := clause.(*ast.CaseClause)
		if !isCase {
			continue
		}

		for _, value := range cases.List {
			if identityLiteral(value) {
				walker.add(value.Pos(), ruleLiteralComparison, name)
			}
		}
	}
}

// identityLiteral reports whether the expression is a string or integer
// literal that names a value. The empty string and zero test presence.
func identityLiteral(expr ast.Expr) bool {
	literal, ok := ast.Unparen(expr).(*ast.BasicLit)
	if !ok {
		return false
	}

	if literal.Kind == token.STRING {
		text, err := strconv.Unquote(literal.Value)

		return err != nil || text != ""
	}

	if literal.Kind == token.INT {
		number, err := strconv.ParseInt(literal.Value, 0, 64)

		return err != nil || number != 0
	}

	return false
}

// domainValue reports whether the expression is a domain value, and the name to
// print for it: a variable, field, or parameter on the name list, or any
// expression of a defined string or integer type this module declares.
func (walker *domainWalker) domainValue(expr ast.Expr) (string, bool) {
	expr = ast.Unparen(expr)

	if name := walker.variableName(expr); isDomainName(name) {
		return name, true
	}

	if walker.moduleScalar(walker.info.TypeOf(expr)) {
		if name := walker.variableName(expr); name != "" {
			return name, true
		}

		return types.ExprString(expr), true
	}

	return "", false
}

// variableName returns the name of the variable, field, or parameter the
// expression reads, or the empty string for any other expression.
func (walker *domainWalker) variableName(expr ast.Expr) string {
	var ident *ast.Ident

	switch typed := expr.(type) {
	case *ast.Ident:
		ident = typed
	case *ast.SelectorExpr:
		ident = typed.Sel
	default:
		return ""
	}

	if _, isVariable := walker.info.Uses[ident].(*types.Var); !isVariable {
		return ""
	}

	return ident.Name
}

// moduleScalar reports whether the type is a defined string or integer type
// declared in this module.
func (walker *domainWalker) moduleScalar(candidate types.Type) bool {
	named, isNamed := types.Unalias(candidate).(*types.Named)
	if !isNamed {
		return false
	}

	declaring := named.Obj().Pkg()
	if declaring == nil || (declaring.Path() != walker.modulePath && !strings.HasPrefix(declaring.Path(), walker.modulePath+"/")) {
		return false
	}

	basic, isBasic := named.Underlying().(*types.Basic)

	return isBasic && basic.Info()&(types.IsString|types.IsInteger) != 0
}

func (walker *domainWalker) add(position token.Pos, rule, identifier string) {
	location := walker.fset.Position(position)
	walker.findings = append(walker.findings, domainFinding{
		Path: location.Filename, Line: location.Line, Rule: rule, Identifier: identifier,
	})
}

// listedPackage is the part of `go list -json` the loader reads.
type listedPackage struct {
	ImportPath string        `json:"ImportPath"`
	Dir        string        `json:"Dir"`
	Export     string        `json:"Export"`
	GoFiles    []string      `json:"GoFiles"`
	DepOnly    bool          `json:"DepOnly"`
	Module     *listedModule `json:"Module"`
	Error      *listedError  `json:"Error"`
}

type listedModule struct {
	Path string `json:"Path"`
}

type listedError struct {
	Err string `json:"Err"`
}

// analyzeDomainLiterals analyses every production package under cmd/ and
// internal/ of the module at root. It lists them with their dependencies' export
// data, so the standard library parses and type-checks each one without
// compiling a second time.
func analyzeDomainLiterals(root string) ([]domainFinding, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}

	listed, err := listDomainPackages(absolute)
	if err != nil {
		return nil, err
	}

	exports := make(map[string]string, len(listed))

	for _, pkg := range listed {
		if pkg.Export != "" {
			exports[pkg.ImportPath] = pkg.Export
		}
	}

	imports := importer.ForCompiler(token.NewFileSet(), "gc", func(path string) (io.ReadCloser, error) {
		file, ok := exports[path]
		if !ok {
			return nil, fmt.Errorf("no export data for %s", path)
		}

		return os.Open(file)
	})

	var findings []domainFinding

	for _, pkg := range listed {
		if pkg.DepOnly {
			continue
		}

		if pkg.Module == nil {
			return nil, fmt.Errorf("go list %s: package is outside any module", pkg.ImportPath)
		}

		sources, sourceErr := readDomainSources(absolute, pkg)
		if sourceErr != nil {
			return nil, sourceErr
		}

		found, analyzeErr := analyzeDomainPackage(pkg.Module.Path, domainPackage{importPath: pkg.ImportPath, sources: sources}, imports)
		if analyzeErr != nil {
			return nil, analyzeErr
		}

		findings = append(findings, found...)
	}

	slices.SortFunc(findings, compareDomainFindings)

	return findings, nil
}

// listDomainPackages lists the packages to analyse with their dependencies. The
// gc importer resolves every import, the standard library included, through an
// export file, and -export without -deps lists none for the dependencies. A
// pattern that matches no directory is an error, not a warning.
func listDomainPackages(root string) ([]listedPackage, error) {
	command := exec.Command("go", "list", "-export", "-deps", "-json", "./cmd/...", "./internal/...")
	command.Dir = root

	var stdout, stderr bytes.Buffer

	command.Stdout, command.Stderr = &stdout, &stderr

	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("go list: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	var listed []listedPackage

	decoder := json.NewDecoder(&stdout)

	for {
		var pkg listedPackage

		err := decoder.Decode(&pkg)
		if errors.Is(err, io.EOF) {
			return listed, nil
		}

		if err != nil {
			return nil, fmt.Errorf("decode go list output: %w", err)
		}

		if pkg.Error != nil {
			return nil, fmt.Errorf("go list %s: %s", pkg.ImportPath, pkg.Error.Err)
		}

		listed = append(listed, pkg)
	}
}

// readDomainSources reads a listed package's non-test Go files, keyed by their
// slash path relative to the module root.
func readDomainSources(root string, pkg listedPackage) (map[string]string, error) {
	directory, err := filepath.EvalSymlinks(pkg.Dir)
	if err != nil {
		return nil, err
	}

	sources := make(map[string]string, len(pkg.GoFiles))

	for _, name := range pkg.GoFiles {
		relative, relErr := filepath.Rel(root, filepath.Join(directory, name))
		if relErr != nil {
			return nil, relErr
		}

		data, readErr := os.ReadFile(filepath.Join(directory, name))
		if readErr != nil {
			return nil, readErr
		}

		sources[filepath.ToSlash(relative)] = string(data)
	}

	return sources, nil
}

func (driver *Driver) runDomainLiteralAnalysis() error {
	findings, err := analyzeDomainLiterals(toolRoot())
	if err != nil {
		return err
	}

	driver.domainFindings = findings

	return nil
}

// requireNoDomainLiteralFinding fails on any finding: production code holds none.
func (driver *Driver) requireNoDomainLiteralFinding() error {
	if len(driver.domainFindings) == 0 {
		return nil
	}

	lines := make([]string, 0, len(driver.domainFindings))
	for _, finding := range driver.domainFindings {
		lines = append(lines, finding.String())
	}

	return fmt.Errorf("domain literal analysis found %d violations:\n%s", len(lines), strings.Join(lines, "\n"))
}
