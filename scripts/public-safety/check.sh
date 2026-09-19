#!/usr/bin/env bash
# ==============================================================================
# check.sh — this repository's public-safety gate
# ==============================================================================
# Usage: RHINO_GATE_SURFACE=<surface> scripts/public-safety/check.sh
#
#   commit-msg   PUBLIC_SAFETY_MESSAGE holds the declared message text
#   pre-commit   no arguments; the staged tree is the subject
#   pre-push     PUBLIC_SAFETY_BASE and PUBLIC_SAFETY_HEAD hold one declared range
#   pull-request the declared tree, message, and range gates run separately
#   main         no arguments; the checked-out tree is the subject
#
# The surface arrives in the environment and nowhere else. It is never inferred
# from an argument's filename, from which hook happens to be running, or from
# whether a remote is reachable — a gate that guesses its own surface will
# eventually guess a weaker one, and that is precisely the case where guessing
# is expensive.
#
# This file decides *what is outbound* at each surface. `outbound-preflight.sh`
# decides whether any of it is prohibited. Keeping those apart is what lets the
# leaf be tested against synthetic inputs with no repository at all.
#
# Exit codes pass through from the leaf: 0 clean, 1 blocked, 2 scan error.
# ==============================================================================

set -uo pipefail

here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
root=$(git -C "$here" rev-parse --show-toplevel 2>/dev/null) || {
	printf '[public-safety] blocked scan-error not inside a Git repository\n' >&2
	exit 2
}
leaf="$here/outbound-preflight.sh"
[[ -x "$leaf" ]] || {
	printf '[public-safety] blocked scan-error the leaf wrapper is missing or not executable\n' >&2
	exit 2
}

surface="${RHINO_GATE_SURFACE:-}"
case "$surface" in
commit-msg | pre-commit | pre-push | pull-request | main) ;;
"")
	printf '[public-safety] blocked scan-error RHINO_GATE_SURFACE is unset\n' >&2
	exit 2
	;;
*)
	printf '[public-safety] blocked scan-error RHINO_GATE_SURFACE is not a known surface\n' >&2
	exit 2
	;;
esac

cd "$root" || exit 2

# ------------------------------------------------------------------------------
# Input collection. Every helper appends to `args`, an argument vector — never a
# string. A path containing a space or a shell metacharacter is data here, and
# building a command line out of it would make it an instruction.
# ------------------------------------------------------------------------------

declare -a args=()

# Paths reach the leaf as NUL-delimited list files, never as one argument per
# path: a large tracked tree overflows the host's argument limit, and a gate
# that cannot start screens nothing.
lists=$(mktemp -d "${TMPDIR:-/tmp}/public-safety-lists.XXXXXX") || {
	printf '[public-safety] blocked scan-error cannot create a working directory\n' >&2
	exit 2
}
trap 'rm -rf "$lists"' EXIT INT TERM
list_count=0

add_paths() {
	# add_paths <command...>: the command prints NUL-delimited paths. Every
	# existing file is content, and every path is a name: a name is outbound
	# material too, and a directory named after something private leaks whether
	# or not any file inside it does.
	local files names f
	list_count=$((list_count + 1))
	files="$lists/files-$list_count"
	names="$lists/names-$list_count"
	while IFS= read -r -d '' f; do
		[[ -f "$f" ]] && printf '%s\0' "$f" >&3
		printf '%s\0' "$f" >&4
	done < <("$@") 3>"$files" 4>"$names"
	[[ -s "$files" ]] && args+=(--file-list "$files")
	[[ -s "$names" ]] && args+=(--names-list "$names")
}

add_tracked_tree() {
	add_paths git ls-files -z
}

add_staged() {
	add_paths git diff --cached --name-only -z --diff-filter=ACMR
}

add_range() {
	local base=$1 head=$2 range
	[[ "$base" =~ ^[0-9a-fA-F]{7,64}$ && "$head" =~ ^[0-9a-fA-F]{7,64}$ ]] || {
		printf '[public-safety] blocked scan-error range input is not a commit ID\n' >&2
		exit 2
	}
	range="$base..$head"
	add_paths git diff --name-only -z --diff-filter=ACMR "$range" --
}

run_leaf() {
	# run_leaf <leaf-surface>; consumes and clears `args`.
	local leaf_surface=$1 rc
	if [[ ${#args[@]} -eq 0 ]]; then
		args=()
		return 0
	fi
	"$leaf" --surface "$leaf_surface" "${args[@]}"
	rc=$?
	args=()
	return $rc
}

current_ref() {
	git symbolic-ref --quiet --short HEAD 2>/dev/null || git rev-parse --short HEAD
}

range_messages() {
	local base=$1 head=$2 range
	[[ "$base" =~ ^[0-9a-fA-F]{7,64}$ && "$head" =~ ^[0-9a-fA-F]{7,64}$ ]] || {
		printf '[public-safety] blocked scan-error range input is not a commit ID\n' >&2
		exit 2
	}
	range="$base..$head"
	git log --format=%B "$range"
}

# ------------------------------------------------------------------------------
# Surfaces
# ------------------------------------------------------------------------------

case "$surface" in
commit-msg)
	[[ ${PUBLIC_SAFETY_MESSAGE+x} ]] || {
		printf '[public-safety] blocked scan-error commit-msg received no declared message\n' >&2
		exit 2
	}
	args+=(--text "$PUBLIC_SAFETY_MESSAGE" --text "$(current_ref)")
	run_leaf commit || exit $?
	;;

pre-commit)
	# The tracked tree first: a leak that is already committed does not become
	# safe because this particular change did not introduce it.
	add_tracked_tree
	run_leaf baseline || exit $?
	add_staged
	run_leaf diff || exit $?
	;;

pre-push)
	[[ -n "${PUBLIC_SAFETY_BASE:-}" && -n "${PUBLIC_SAFETY_HEAD:-}" ]] || {
		printf '[public-safety] blocked scan-error pre-push received no declared range\n' >&2
		exit 2
	}
	args+=(--text "$PUBLIC_SAFETY_BASE" --text "$PUBLIC_SAFETY_HEAD")
	run_leaf ref || exit $?
	args+=(--text "$(range_messages "$PUBLIC_SAFETY_BASE" "$PUBLIC_SAFETY_HEAD")")
	run_leaf commit || exit $?
	add_range "$PUBLIC_SAFETY_BASE" "$PUBLIC_SAFETY_HEAD"
	run_leaf diff || exit $?
	;;

pull-request)
	if [[ ${PUBLIC_SAFETY_MESSAGE+x} ]]; then
		args+=(--text "$PUBLIC_SAFETY_MESSAGE")
		run_leaf commit || exit $?
	elif [[ -n "${PUBLIC_SAFETY_BASE:-}" || -n "${PUBLIC_SAFETY_HEAD:-}" ]]; then
		[[ -n "${PUBLIC_SAFETY_BASE:-}" && -n "${PUBLIC_SAFETY_HEAD:-}" ]] || {
			printf '[public-safety] blocked scan-error pull-request range is incomplete\n' >&2
			exit 2
		}
		args+=(--text "$PUBLIC_SAFETY_BASE" --text "$PUBLIC_SAFETY_HEAD")
		run_leaf ref || exit $?
		args+=(--text "$(range_messages "$PUBLIC_SAFETY_BASE" "$PUBLIC_SAFETY_HEAD")")
		run_leaf commit || exit $?
		add_range "$PUBLIC_SAFETY_BASE" "$PUBLIC_SAFETY_HEAD"
		run_leaf diff || exit $?
	else
		args+=(--text "$(current_ref)" --text "$(git log -1 --format=%B HEAD)")
		run_leaf ref || exit $?
		add_tracked_tree
		run_leaf baseline || exit $?
	fi
	;;

main)
	args+=(--text "$(current_ref)" --text "$(git log -1 --format=%B HEAD)")
	run_leaf ref || exit $?
	add_tracked_tree
	run_leaf baseline || exit $?
	;;
esac

printf '[public-safety] %s: clean\n' "$surface"
exit 0
