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

# The quick contract covers formatting, compilation, strict lint, unit tests,
# 99% deterministic-core coverage, behavior adapters, and repository policy.
#
# Documentation hygiene is not here. It is declared in repo-config.yml on the
# same `pre-push` surface that dispatches this script, so the push runs it
# either way; calling it from inside a gate would make this file re-enter the
# dispatcher that invoked it.
./scripts/check-worktree-layout.sh
./scripts/format-check.sh
go test -run '^$' ./...
go tool golangci-lint run
# NilAway follows nil flow across functions and packages, which golangci-lint's
# per-function nilness cannot, so it runs beside the linter rather than inside
# it. It exits 3 on a diagnostic and 1 when loading fails, either of which stops
# this script; -json is not used because it always exits 0, so a finding would
# pass. A false positive is excluded where it stands with a reasoned
# //nolint:nilaway directive.
go tool nilaway -include-pkgs=github.com/wahidyankf/hippo -pretty-print=false ./...
# The production graph and negative fixtures cover every supported platform,
# even when the current host excludes a platform's build-tagged Go files.
go test -count=1 ./tests/architecture
# Every package's own tests. The scenario "Every package with tests runs in a
# gate" fails if a package holding tests falls outside these patterns.
go test -count=1 ./cmd/... ./internal/... ./tests/support ./tests/coverage
# The unit corpus and pure package tests share one measured coverage execution.
# Explicit leaf directories keep the threshold helper's nonrecursive discovery
# complete as domain packages acquire new deterministic functions.
mkdir -p coverage
go test -count=1 \
	-coverpkg=./internal/policy,./internal/adapters/config,./internal/adapters/host,./internal/adapters/evidence,./internal/domain/coordination,./internal/domain/evidence,./internal/identity,./internal/status,./internal/application \
	-coverprofile=coverage/unit.out \
	./tests/unit ./internal/application ./internal/domain/coordination ./internal/domain/evidence \
	./internal/identity ./internal/status
go run ./tests/coverage --profile coverage/unit.out \
	--directories internal/policy,internal/domain/coordination,internal/domain/evidence,internal/identity,internal/status,internal/application \
	--files internal/adapters/config/config.go,internal/adapters/host/collector.go,internal/adapters/host/linux_parsers.go --minimum 99
HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd
HIPPO_BDD_ADAPTER=integration go test -count=1 ./tests/bdd
# The end-to-end adapter runs a real binary, and which binary it runs is the
# whole question. HIPPO_BIN may already be in the environment -- a guarded
# shell exports it, pointing at whatever binary that guard resolved -- so
# inheriting it could measure some other build and tell us nothing about the
# working tree. Build what is being tested, with the
# same embedded identity tests/e2e/run.sh uses, and hand that to the adapter.
bdd_temporary=$(mktemp -d "${TMPDIR:-/tmp}/hippo-bdd.XXXXXX")
trap 'rm -rf -- "$bdd_temporary"' EXIT
go build -trimpath \
	-ldflags "-X github.com/wahidyankf/hippo/internal/adapters/cli.Version=v0.0.0-test -X github.com/wahidyankf/hippo/internal/adapters/cli.Commit=0000000000000000000000000000000000000000" \
	-o "$bdd_temporary/hippo" ./cmd/hippo
HIPPO_BDD_ADAPTER=e2e HIPPO_BIN="$bdd_temporary/hippo" go test -count=1 ./tests/bdd

./tests/artifacts/run.sh
