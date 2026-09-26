#!/bin/sh
# Prove the worktree layout check refuses tracked instructions that prescribe
# a sibling *-worktrees/ directory, in the wordings they have actually taken,
# while the convention that forbids that layout by naming it still passes.
# Each case runs against a copy of the check in a scratch repository, so this
# checkout's own files and worktrees are never read.
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)

work=$(mktemp -d "${TMPDIR:-/tmp}/hippo-worktree-layout.XXXXXX")
trap 'rm -rf -- "$work"' EXIT HUP INT TERM
# The check compares its own path with Git's, and Git reports the physical
# one, so a temporary directory behind a symbolic link must be resolved first.
work=$(CDPATH='' cd -- "$work" && pwd -P)

# check_text <case> <expected exit> <tracked Markdown text>
check_text() {
	tree="$work/$1"
	mkdir -p "$tree/scripts"
	cp "$repo_root/scripts/check-worktree-layout.sh" "$tree/scripts/check-worktree-layout.sh"
	printf '/worktrees/\n' >"$tree/.gitignore"
	printf '%s\n' "$3" >"$tree/instructions.md"
	git -C "$tree" init --quiet
	git -C "$tree" add .gitignore instructions.md scripts
	status=0
	sh "$tree/scripts/check-worktree-layout.sh" >"$work/stdout" 2>"$work/stderr" || status=$?
	if [ "$status" -ne "$2" ] || { [ "$2" -ne 0 ] && ! grep -q 'sibling \*-worktrees layout' "$work/stderr"; }; then
		echo "worktree layout, $1: expected exit $2, got $status" >&2
		cat "$work/stdout" "$work/stderr" >&2
		exit 1
	fi
}

check_text command 1 'git worktree add ../hippo-worktrees/feature -b worktree/feature origin/main'
check_text change-directory 1 'cd ../hippo-worktrees/feature && npm ci'
check_text provision-at 1 '1. **Provision** at `../hippo-worktrees/<name>`, beside the checkout.'
check_text create-in 1 'Create the worktree in ../hippo-worktrees/<name>.'
check_text convention 0 'Sibling directories such as `../hippo-worktrees/` are forbidden.'
check_text canonical 0 'git worktree add worktrees/<task> -b worktree/<task> origin/main'
