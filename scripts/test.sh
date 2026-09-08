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

# Extend the quick gate with process boundaries, race detection, and a current
# vulnerability scan. This is the release gate used by CI on every platform.
./scripts/test-quick.sh
go test -count=1 ./tests/integration
./tests/e2e/run.sh
go test -race -count=1 ./internal/evidence ./internal/release ./tests/unit ./tests/integration
go tool govulncheck ./...
