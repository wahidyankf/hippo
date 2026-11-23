#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

# A Git hook running inside a linked worktree exports GIT_DIR, and Git prefers
# it over both the working directory and -C. Everything below creates or
# inspects repositories by path; without this, a fixture's `git init` would
# re-initialize the repository under test and its commits would land on the
# branch being pushed.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY \
	GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_COMMON_DIR GIT_NAMESPACE GIT_PREFIX

# External helper compilation runs once before any dependent test process.
# Admission and finite process-group supervision belong to the gate caller.
. ./tests/support/hostprobe/prepare.sh
trap public_host_helper_cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

# The quick contract: typecheck, strict lint, unit tests with -race, the static
# behaviour adapters, the behaviour coverage validation, and 99% unit coverage
# of every authored package, measured last. Formatting, layout, hook transport, architecture,
# contract, artifact, and wiring checks are registry entries of the
# pull-request surface (C14), so they are not repeated here.
public_host_helper_prepare "$root"
go vet ./...
go test -run '^$' ./...
go tool golangci-lint run
# NilAway follows nil flow across functions and packages, which golangci-lint's
# per-function nilness cannot, so it runs beside the linter rather than inside
# it. It exits 3 on a diagnostic and 1 when loading fails, either of which stops
# this script; -json is not used because it always exits 0, so a finding would
# pass. A false positive is excluded where it stands with a reasoned
# //nolint:nilaway directive.
go tool nilaway -include-pkgs=github.com/wahidyankf/hippo -pretty-print=false ./...
# Every package's own tests, with the race detector. The scenario "Every package
# with tests runs in a gate" fails if a package holding tests falls outside
# these patterns.
go test -race -count=1 ./cmd/... ./internal/... ./tests/support ./tests/support/hostprobe ./tests/support/oldidentity ./tests/unit ./tests/coverage ./tests/bdd \
	-skip '^(TestUnitBehaviours|TestIntegrationBehaviours|TestE2EBehaviours|TestFeatureCompliance)$'
HIPPO_BDD_SUITE=unit go test -race -count=1 -run '^TestUnitBehaviours$' ./tests/bdd
# Resolve the complete composed registry without executing a corpus or candidate.
HIPPO_BDD_ADAPTER=unit go test -count=1 -run '^TestFeatureCompliance$' ./tests/bdd
HIPPO_BDD_ADAPTER=integration go test -count=1 -run '^TestFeatureCompliance$' ./tests/bdd
HIPPO_BDD_ADAPTER=e2e go test -count=1 -run '^TestFeatureCompliance$' ./tests/bdd
# test:behaviour:coverage, the static check; it runs no test (C14).
./rhino behaviour coverage validate

# Unit coverage is measured last, so every step above still gates on its own. The package set is what go list reports
# for the authored production tree, minus the named test-support packages, so a new package under cmd, internal, or
# scripts/release-archive is measured without editing this script. The unit run instruments that set with -coverpkg,
# and the floor applies to every package and to the aggregate; a package below it is named by the step.
listed=$(go list ./cmd/... ./internal/... ./scripts/release-archive)
packages=$(printf '%s\n' "$listed" | grep -v -e '/internal/adapters/system/systemtest$' | paste -sd, -)
mkdir -p coverage
go test -count=1 -coverpkg=./cmd/...,./internal/...,./scripts/release-archive \
	-coverprofile=coverage/unit.out \
	-skip '^(TestIntegrationBehaviours|TestE2EBehaviours|TestFeatureCompliance)$' \
	./tests/unit ./tests/bdd ./cmd/... ./internal/... ./scripts/release-archive
go run ./tests/coverage --profile coverage/unit.out --packages "$packages" --minimum 99
