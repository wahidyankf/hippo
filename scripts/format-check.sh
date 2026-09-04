#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

# CI checks the same Go formatter pair used by pre-commit without rewriting the
# worktree. Shell entrypoints are listed explicitly so generated Husky shims are
# never treated as repository-owned source.
go tool golangci-lint fmt --diff
go tool shfmt -d resource-guard scripts tests \
	.husky/commit-msg .husky/pre-commit .husky/pre-push
npm exec -- prettier --check --ignore-unknown .
