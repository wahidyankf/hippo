#!/bin/sh
# Prove the pinned ShellCheck wrapper refuses, with exit 125 and before any
# analyser runs, the two pins it cannot verify: a cached archive whose digest
# is not the pinned one, and a pin that names no archive for this platform.
# Both cases run offline against a copy of the wrapper in a scratch tree, so
# the repository's own pin and cache are never read or written.
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)

work=$(mktemp -d "${TMPDIR:-/tmp}/hippo-shellcheck-pin.XXXXXX")
trap 'rm -rf -- "$work"' EXIT HUP INT TERM

mkdir -p "$work/tree/scripts" "$work/fake/shellcheck-v0.11.0"
cp "$repo_root/scripts/shellcheck.sh" "$work/tree/scripts/shellcheck.sh"
chmod 0755 "$work/tree/scripts/shellcheck.sh"

# A stand-in analyser that records being executed. If the wrapper ever runs it,
# the marker exists and the refusal came too late.
marker="$work/executed"
printf '#!/bin/sh\ntouch "%s"\n' "$marker" >"$work/fake/shellcheck-v0.11.0/shellcheck"
chmod 0755 "$work/fake/shellcheck-v0.11.0/shellcheck"
(cd "$work/fake" && tar -czf "$work/fake.tar.gz" shellcheck-v0.11.0)

# expect_refusal <case> <expected stderr fragment>
expect_refusal() {
	status=0
	SHELLCHECK_INSTALL_CACHE="$work/cache" "$work/tree/scripts/shellcheck.sh" --version \
		>"$work/stdout" 2>"$work/stderr" || status=$?
	if [ "$status" -ne 125 ]; then
		echo "shellcheck pin, $1: expected exit 125, got $status" >&2
		cat "$work/stderr" >&2
		exit 1
	fi
	if ! grep -q -- "$2" "$work/stderr"; then
		echo "shellcheck pin, $1: expected a refusal naming \"$2\"" >&2
		cat "$work/stderr" >&2
		exit 1
	fi
	if [ -e "$marker" ]; then
		echo "shellcheck pin, $1: the analyser ran before the refusal" >&2
		exit 1
	fi
}

# Case 1: every supported platform is pinned to a digest the cached archive
# does not have, and that archive is already in the cache.
pinned=0000000000000000000000000000000000000000000000000000000000000000
{
	printf 'version=v0.11.0\n'
	for target in darwin.aarch64 darwin.x86_64 linux.aarch64 linux.x86_64; do
		printf '%s=%s\n' "$target" "$pinned"
		mkdir -p "$work/cache/v0.11.0/$target"
		cp "$work/fake.tar.gz" "$work/cache/v0.11.0/$target/shellcheck-v0.11.0.$target.tar.gz"
	done
} >"$work/tree/shellcheck.lock"
expect_refusal "cached digest mismatch" "does not match the pinned digest"

# Case 2: the pin names a version and no platform archive at all, so whatever
# platform runs this has none.
printf 'version=v0.11.0\n' >"$work/tree/shellcheck.lock"
expect_refusal "unpinned platform" "the pin declares no"
