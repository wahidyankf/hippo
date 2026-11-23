package main

import (
	"strings"
	"testing"
)

func TestCoveragePackagesGroupsBlocksByImportPath(t *testing.T) {
	profile := "mode: set\n" +
		"module/a/x.go:1.1,2.2 2 1\n" +
		"module/a/y.go:3.1,4.2 1 0\n" +
		"module/b/z.go:1.1,2.2 1 0\n" +
		"module/a/x.go:1.1,2.2 2 0\n"
	counts, err := countPackages(strings.NewReader(profile), map[string]bool{"module/a": true, "module/c": true})
	if err != nil {
		t.Fatal(err)
	}
	if got := counts["module/a"]; got.covered != 2 || got.total != 3 {
		t.Fatalf("package a counts %+v, want 2 of 3", got)
	}
	if got, present := counts["module/b"]; present || got.total != 0 {
		t.Fatalf("unselected package b counted: %+v", got)
	}
	if got := counts["module/c"]; got.total != 0 {
		t.Fatalf("package without blocks counted: %+v", got)
	}
}

func TestCoveragePackagesRefusesConflictingBlockStatementCounts(t *testing.T) {
	profile := "mode: set\nmodule/a/x.go:1.1,2.2 2 1\nmodule/a/x.go:1.1,2.2 3 0\n"
	if _, err := countPackages(strings.NewReader(profile), map[string]bool{"module/a": true}); err == nil {
		t.Fatal("conflicting statement counts accepted")
	}
}

func TestCoveragePercentageOfPackage(t *testing.T) {
	if got := (statementCoverage{covered: 99, total: 100}).percentage(); got != 99 {
		t.Fatalf("percentage %v, want 99", got)
	}
}
