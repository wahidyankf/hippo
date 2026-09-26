#!/bin/sh
# The shell-lint gate: the pinned ShellCheck at warning severity over the one
# shell file list. A warning or an error fails, per Lint Strictness; notes and
# style suggestions stay below that threshold and never fail the gate.
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

# Capture the list first, so a failure to produce it stops the gate instead of
# analysing an empty or partial list and passing.
list=$(./scripts/shell-files.sh)
set --
while IFS= read -r file; do
	set -- "$@" "$file"
done <<EOF
$list
EOF

exec ./scripts/shellcheck.sh --severity=warning --format=gcc -- "$@"
