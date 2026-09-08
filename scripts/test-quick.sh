#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

# A Git hook running inside a linked worktree exports GIT_DIR, and Git prefers
# it over both the working directory and -C. Everything below creates or
# inspects repositories by path; without this, a fixture's `git init` would
# re-initialize the repository under test and its commits would land on the
# branch being pushed.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY \
	GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_COMMON_DIR GIT_NAMESPACE GIT_PREFIX

# The quick contract covers formatting, compilation, strict lint, unit tests,
# 99% deterministic-core coverage, behavior adapters, repository policy, and
# documentation hygiene before a push is allowed.
./scripts/format-check.sh
go test -run '^$' ./...
go tool golangci-lint run
go test -count=1 ./internal/evidence ./internal/release ./tests/unit
mkdir -p coverage
go test -count=1 -coverpkg=./internal/policy,./internal/config,./internal/host,./internal/evidence -coverprofile=coverage/unit.out ./tests/unit
go run ./tests/coverage --profile coverage/unit.out --directories internal/policy,internal/config --files internal/host/collector.go,internal/host/linux_parsers.go,internal/evidence/histogram.go --minimum 99
HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd
HIPPO_BDD_ADAPTER=integration go test -count=1 ./tests/bdd
HIPPO_BDD_ADAPTER=e2e go test -count=1 ./tests/bdd
./tests/artifacts/run.sh

# Documentation hygiene, against the pinned RHINO release in rhino.lock. Kept in
# a script of its own so the pull-request gate can name it as a job without
# holding a second copy of the invocation.
./scripts/docs-check.sh
