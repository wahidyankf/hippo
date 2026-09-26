#!/usr/bin/env bash
# The pinned ShellCheck this repository's shell-lint gate runs.
#
# Every analysis dispatches through here rather than through whatever
# `shellcheck` happens to be on PATH or in a runner image. An analyser that
# changes its rules between two runs of the same gate turns a clean tree red,
# or a defect green, without anyone editing a script, and a floating version
# is exactly that.
#
# Verification is unconditional and happens before execution. The cache keeps
# the verified release archive, re-digests it every run, and re-derives the
# executable from it, because a cache is a place other processes can write. A
# cached archive that no longer matches the pin is refused rather than fetched
# again: something changed it, and a fresh download would hide that.
#
# Exit 125 is this wrapper refusing to run: a malformed pin, a platform the pin
# does not name, a digest that does not match, or a version that is not the
# one the pin names. Every other exit code is ShellCheck's own.
set -euo pipefail
unset CDPATH

tool_dir=$(cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(cd -- "$tool_dir/.." && pwd)
lock_file="$repo_root/shellcheck.lock"

refuse() {
	printf '[shellcheck] %s\n' "$1" >&2
	exit 125
}

[[ -r "$lock_file" ]] || refuse "there is no pin at shellcheck.lock, and an unpinned analyser is not a gate"

# Every key must resolve exactly once. A duplicate is ambiguous, and ambiguity
# in a pin is a configuration error rather than something to resolve by
# preferring the first or the last.
lock_value() {
	local key=$1 value
	value=$(awk -F= -v key="$key" '$0 !~ /^#/ && $1 == key { print substr($0, index($0, "=") + 1) }' "$lock_file")
	[[ -n "$value" ]] || refuse "the pin declares no $key"
	[[ "$value" != *$'\n'* ]] || refuse "the pin declares $key more than once"
	printf '%s' "$value"
}

version=$(lock_value version)
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || refuse "the pin declares an invalid stable version"

# Only the platforms the pin can name. Anything else is refused by name rather
# than approximated by a neighbouring build.
case "$(uname -s)" in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) refuse "there is no pinned ShellCheck for this operating system" ;;
esac
case "$(uname -m)" in
x86_64 | amd64) arch=x86_64 ;;
arm64 | aarch64) arch=aarch64 ;;
*) refuse "there is no pinned ShellCheck for this architecture" ;;
esac
target="$os.$arch"

digest=$(lock_value "$target")
[[ "$digest" =~ ^[0-9a-f]{64}$ ]] || refuse "the pin declares an invalid digest for $target"

# The cache lives in the ignored .cache directory beside the other build
# inputs. Tests may redirect it without weakening any verification.
cache_root=${SHELLCHECK_INSTALL_CACHE:-"$repo_root/.cache/shellcheck"}
install_dir="$cache_root/$version/$target"
archive_name="shellcheck-$version.$target.tar.gz"
archive="$install_dir/$archive_name"
member="shellcheck-$version/shellcheck"
binary="$install_dir/shellcheck"

digest_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{ print $1 }'
	else
		shasum -a 256 "$1" | awk '{ print $1 }'
	fi
}

digest_of_member() {
	if command -v sha256sum >/dev/null 2>&1; then
		tar -xzOf "$archive" "$member" | sha256sum | awk '{ print $1 }'
	else
		tar -xzOf "$archive" "$member" | shasum -a 256 | awk '{ print $1 }'
	fi
}

if [[ -e "$archive" ]]; then
	[[ "$(digest_of "$archive")" == "$digest" ]] ||
		refuse "the cached archive for $target does not match the pinned digest; remove $install_dir to fetch it again"
else
	# A hook may run with no network. Say so plainly rather than failing with a
	# transport error that reads like an outage.
	command -v curl >/dev/null 2>&1 || refuse "curl is required to install the pinned ShellCheck"

	staging=$(mktemp -d "${TMPDIR:-/tmp}/shellcheck-install.XXXXXX")
	trap 'rm -rf -- "$staging"' EXIT

	url="https://github.com/koalaman/shellcheck/releases/download/$version/$archive_name"
	curl -fsSL --retry 2 -o "$staging/$archive_name" "$url" ||
		refuse "the pinned ShellCheck could not be fetched, and this wrapper will not fall back to an unpinned one"

	# Verify before publishing. Verifying afterwards tells you what you already
	# trusted.
	[[ "$(digest_of "$staging/$archive_name")" == "$digest" ]] ||
		refuse "the fetched archive does not match the pinned digest for $target"

	# Publish atomically, so a concurrent gate either sees no archive or sees a
	# complete one, and never a half-written download.
	mkdir -p "$install_dir"
	published=$(mktemp "$install_dir/.archive.XXXXXX")
	cp "$staging/$archive_name" "$published"
	mv -f "$published" "$archive"
	[[ "$(digest_of "$archive")" == "$digest" ]] ||
		refuse "the published archive for $target did not verify immediately after installation"
fi

# The executable is derived from the verified archive, so it is compared with
# the archive's own member on every run rather than with a recorded digest a
# writer could have replaced alongside it.
expected=$(digest_of_member) || refuse "the pinned archive does not carry $member"
if [[ ! -x "$binary" || "$(digest_of "$binary")" != "$expected" ]]; then
	extracted=$(mktemp "$install_dir/.shellcheck.XXXXXX")
	tar -xzOf "$archive" "$member" >"$extracted" ||
		refuse "the pinned archive could not be extracted"
	chmod 0755 "$extracted"
	mv -f "$extracted" "$binary"
	[[ "$(digest_of "$binary")" == "$expected" ]] ||
		refuse "the extracted ShellCheck did not verify immediately after extraction"
fi

reported=$("$binary" --version 2>/dev/null | awk '$1 == "version:" { print $2 }') || reported=
[[ "$reported" == "${version#v}" ]] ||
	refuse "the pinned executable reports a version the pin does not name"

exec "$binary" "$@"
