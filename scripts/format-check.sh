#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

# CI checks the same Go formatter pair used by pre-commit without rewriting the
# worktree. Shell files come from the one list the shell-lint gate analyses, so
# the formatter and the analyser cannot drift apart, and generated Husky shims
# are never treated as repository-owned source.
go tool golangci-lint fmt --diff
shell_files=$(./scripts/shell-files.sh)
set --
while IFS= read -r file; do
	set -- "$@" "$file"
done <<EOF
$shell_files
EOF
go tool shfmt -d -- "$@"
npm exec -- prettier --check --ignore-unknown .
