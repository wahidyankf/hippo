#!/bin/sh
# Source from an admitted, finitely supervised gate before dependent Go tests.
# This routine owns only the directory it creates; it adds no process launcher.
set -eu

public_host_helper_owned_directory=

public_host_helper_hash() {
	if command -v sha256sum >/dev/null 2>&1; then
		if ! public_host_helper_hash_line=$(sha256sum "$1"); then return 1; fi
	else
		if ! public_host_helper_hash_line=$(shasum -a 256 "$1"); then return 1; fi
	fi
	printf '%s\n' "${public_host_helper_hash_line%% *}"
}

public_host_helper_validate() {
	case "${HIPPO_PUBLIC_HOST_HELPER:-}" in
	/*) ;;
	*)
		echo 'public host helper must be a clean absolute executable path' >&2
		return 1
		;;
	esac
	case "$HIPPO_PUBLIC_HOST_HELPER" in
	*/../* | */./* | *//*)
		echo 'public host helper path is not clean' >&2
		return 1
		;;
	esac
	if [ ! -f "$HIPPO_PUBLIC_HOST_HELPER" ] || [ -L "$HIPPO_PUBLIC_HOST_HELPER" ] || [ ! -x "$HIPPO_PUBLIC_HOST_HELPER" ]; then
		echo 'public host helper must be a regular executable, never a symlink' >&2
		return 1
	fi
	if [ "${#HIPPO_PUBLIC_HOST_HELPER_SHA256}" -ne 64 ]; then
		echo 'public host helper requires an exact SHA-256' >&2
		return 1
	fi
	case "$HIPPO_PUBLIC_HOST_HELPER_SHA256" in
	*[!0-9a-fA-F]*)
		echo 'public host helper SHA-256 is not hexadecimal' >&2
		return 1
		;;
	esac
	public_host_helper_actual=$(public_host_helper_hash "$HIPPO_PUBLIC_HOST_HELPER")
	public_host_helper_expected=$(printf '%s' "$HIPPO_PUBLIC_HOST_HELPER_SHA256" | tr 'A-F' 'a-f')
	if [ "$public_host_helper_actual" != "$public_host_helper_expected" ]; then
		echo 'public host helper SHA-256 does not match selected artifact' >&2
		return 1
	fi
}

public_host_helper_sources() {
	for public_host_helper_source in "$1/go.mod" "$1/go.sum" "$1"/tests/support/hostprobe/*.go; do
		public_host_helper_source_hash=$(public_host_helper_hash "$public_host_helper_source")
		printf '%s %s\n' "$public_host_helper_source_hash" "$public_host_helper_source"
	done
}

public_host_helper_prepare() {
	if [ "$#" -ne 1 ]; then
		echo 'helper preparation requires the owning checkout root' >&2
		return 1
	fi
	# Even a set-but-empty inherited input is a refusal, never a rebuild request.
	if [ "${HIPPO_PUBLIC_HOST_HELPER+set}${HIPPO_PUBLIC_HOST_HELPER_SHA256+set}" != '' ]; then
		if [ -z "${HIPPO_PUBLIC_HOST_HELPER:-}" ] || [ -z "${HIPPO_PUBLIC_HOST_HELPER_SHA256:-}" ]; then
			echo 'both inherited public host helper inputs must be nonempty' >&2
			return 1
		fi
		public_host_helper_validate
		return
	fi
	public_host_helper_go=${HIPPO_GO_BINARY:-go}
	public_host_helper_owned_directory=$(mktemp -d "${HIPPO_E2E_TEMP_PARENT:-${TMPDIR:-/tmp}}/hippo-host-helper.XXXXXX")
	public_host_helper_owned_directory=$(CDPATH='' cd -- "$public_host_helper_owned_directory" && pwd)
	public_host_helper_sources "$1" >"$public_host_helper_owned_directory/source-before.sha256"
	"$public_host_helper_go" version >"$public_host_helper_owned_directory/go-version.stdout"
	public_host_helper_tool=$(command -v "$public_host_helper_go")
	public_host_helper_hash "$public_host_helper_tool" >"$public_host_helper_owned_directory/go-tool.sha256"
	printf '%s\n' "$public_host_helper_go" test -c -race -p=1 -trimpath -buildvcs=false -o "$public_host_helper_owned_directory/hostprobe" ./tests/support/hostprobe >"$public_host_helper_owned_directory/build.argv"
	GOMAXPROCS=2 "$public_host_helper_go" test -c -race -p=1 -trimpath -buildvcs=false \
		-o "$public_host_helper_owned_directory/hostprobe" ./tests/support/hostprobe \
		>"$public_host_helper_owned_directory/build.stdout" 2>"$public_host_helper_owned_directory/build.stderr"
	printf '%s\n' "$public_host_helper_go" list -deps -test -json ./tests/support/hostprobe >"$public_host_helper_owned_directory/closure-json.argv"
	"$public_host_helper_go" list -deps -test -json ./tests/support/hostprobe \
		>"$public_host_helper_owned_directory/closure.json" 2>"$public_host_helper_owned_directory/closure.stderr"
	printf '%s\n' "$public_host_helper_go" list -deps -test -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./tests/support/hostprobe >"$public_host_helper_owned_directory/closure-inventory.argv"
	"$public_host_helper_go" list -deps -test -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./tests/support/hostprobe \
		>"$public_host_helper_owned_directory/nonstandard-packages.txt"
	while IFS= read -r public_host_helper_package; do
		case "$public_host_helper_package" in
		'' | github.com/wahidyankf/hippo/tests/support/hostprobe | github.com/wahidyankf/hippo/tests/support/hostprobe.test | 'github.com/wahidyankf/hippo/tests/support/hostprobe [github.com/wahidyankf/hippo/tests/support/hostprobe.test]' | 'github.com/wahidyankf/hippo/tests/support/hostprobe_test [github.com/wahidyankf/hippo/tests/support/hostprobe.test]') ;;
		*)
			echo "public host helper dependency escaped stdlib boundary: $public_host_helper_package" >&2
			return 1
			;;
		esac
	done <"$public_host_helper_owned_directory/nonstandard-packages.txt"
	public_host_helper_sources "$1" >"$public_host_helper_owned_directory/source-after.sha256"
	cmp "$public_host_helper_owned_directory/source-before.sha256" "$public_host_helper_owned_directory/source-after.sha256"
	HIPPO_PUBLIC_HOST_HELPER=$public_host_helper_owned_directory/hostprobe
	HIPPO_PUBLIC_HOST_HELPER_SHA256=$(public_host_helper_hash "$HIPPO_PUBLIC_HOST_HELPER")
	printf '%s\n' "$HIPPO_PUBLIC_HOST_HELPER_SHA256" >"$public_host_helper_owned_directory/helper.sha256"
	export HIPPO_PUBLIC_HOST_HELPER HIPPO_PUBLIC_HOST_HELPER_SHA256
	public_host_helper_validate
}

public_host_helper_cleanup() {
	if [ -n "$public_host_helper_owned_directory" ]; then
		rm -rf -- "$public_host_helper_owned_directory"
	fi
}
