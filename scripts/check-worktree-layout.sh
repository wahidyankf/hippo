#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
common=$(git -C "$root" rev-parse --path-format=absolute --git-common-dir)
repository_location=$(dirname -- "$common")
failed=0

if [ "$root" != "$repository_location" ]; then
	case "$root" in
	"$repository_location"/worktrees/*) ;;
	*)
		echo "current worktree is outside $repository_location/worktrees: $root" >&2
		failed=1
		;;
	esac
fi

if git -C "$root" grep -n -E \
	'git worktree add \.\./[^ ]*-worktrees|cd \.\./[^ ]*-worktrees|Worktree path:.*-worktrees' \
	-- '*.md' '*.yml' '*.yaml'; then
	echo "tracked instructions still prescribe a sibling *-worktrees layout" >&2
	failed=1
fi

if ! grep -qx '/worktrees/' "$root/.gitignore"; then
	echo ".gitignore must contain /worktrees/" >&2
	failed=1
fi

exit "$failed"
