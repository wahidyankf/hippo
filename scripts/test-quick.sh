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
go test -count=1 ./internal/evidence ./internal/release ./tests/unit
mkdir -p coverage
go test -count=1 -coverpkg=./internal/policy,./internal/config,./internal/host,./internal/evidence -coverprofile=coverage/unit.out ./tests/unit
go run ./tests/coverage --profile coverage/unit.out --directories internal/policy,internal/config --files internal/host/collector.go,internal/host/linux_parsers.go,internal/evidence/histogram.go --minimum 99
HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd
HIPPO_BDD_ADAPTER=integration go test -count=1 ./tests/bdd
# The end-to-end adapter runs a real binary, and which binary it runs is the
# whole question. HIPPO_BIN is already in the environment here -- the guard that
# wraps this script exports it -- and it points at the checksum-pinned release
# in .cache, so inheriting it would measure the version already installed and
# tell us nothing about the working tree. Build what is being tested, with the
# same embedded identity tests/e2e/run.sh uses, and hand that to the adapter.
bdd_temporary=$(mktemp -d "${TMPDIR:-/tmp}/hippo-bdd.XXXXXX")
trap 'rm -rf -- "$bdd_temporary"' EXIT
go build -trimpath \
	-ldflags "-X github.com/wahidyankf/hippo/internal/cli.Version=v0.0.0-test -X github.com/wahidyankf/hippo/internal/cli.Commit=0000000000000000000000000000000000000000" \
	-o "$bdd_temporary/hippo" ./cmd/hippo
HIPPO_BDD_ADAPTER=e2e HIPPO_BIN="$bdd_temporary/hippo" go test -count=1 ./tests/bdd

./tests/artifacts/run.sh
