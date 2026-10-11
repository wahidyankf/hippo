#!/bin/sh
# Source before the E2E gate acquires its temporary directory. The admitted
# gate caller supervises this preparation and all children with a finite bound.
set -eu

public_old_release_prepare() {
	if [ "$#" -ne 2 ]; then
		echo 'old release preparation requires checkout and owned E2E directory' >&2
		return 1
	fi
	public_old_release_root=$1
	public_old_release_directory=$2/old-release
	mkdir "$public_old_release_directory"
	public_old_release_directory=$(CDPATH='' cd -- "$public_old_release_directory" && pwd)
	case "$(uname -s):$(uname -m)" in
	Darwin:arm64) public_old_release_platform=darwin-arm64 ;;
	Darwin:x86_64) public_old_release_platform=darwin-amd64 ;;
	Linux:aarch64 | Linux:arm64) public_old_release_platform=linux-arm64 ;;
	Linux:x86_64) public_old_release_platform=linux-amd64 ;;
	*)
		echo 'old release does not support this host' >&2
		return 1
		;;
	esac
	public_old_release_lock=$public_old_release_root/tests/e2e/hippo-old.lock
	if [ "$(sed -n '1p' "$public_old_release_lock")" != version=v0.8.4 ] ||
		[ "$(sed -n '2p' "$public_old_release_lock")" != commit=abde2ecbeff7513549cfd1c0805fd20b5d232c01 ]; then
		echo 'old release lock identity is not immutable v0.8.4' >&2
		return 1
	fi
	public_old_release_archive=$public_old_release_directory/release.tar.gz
	public_old_release_digest=$(sed -n "s/^$public_old_release_platform=//p" "$public_old_release_lock")
	if [ "${#public_old_release_digest}" -ne 64 ]; then
		echo 'old release lock lacks exactly one host digest' >&2
		return 1
	fi
	case "$public_old_release_digest" in
	*[!0-9a-f]*)
		echo 'old release archive digest is malformed' >&2
		return 1
		;;
	esac
	if [ "${HIPPO_E2E_OLD_ARCHIVE+set}" = set ]; then
		case "$HIPPO_E2E_OLD_ARCHIVE" in
		/*) ;;
		*)
			echo 'old release archive requires an absolute path' >&2
			return 1
			;;
		esac
		if [ ! -f "$HIPPO_E2E_OLD_ARCHIVE" ] || [ -L "$HIPPO_E2E_OLD_ARCHIVE" ]; then
			echo 'old release archive must be a regular file, never a symlink' >&2
			return 1
		fi
		cp "$HIPPO_E2E_OLD_ARCHIVE" "$public_old_release_archive"
	else
		public_old_release_asset=hippo_v0.8.4_$(printf '%s' "$public_old_release_platform" | tr '-' '_').tar.gz
		curl --fail --location --proto '=https' --proto-redir '=https' --connect-timeout 15 --max-time 120 \
			--output "$public_old_release_archive" \
			"https://github.com/wahidyankf/hippo/releases/download/v0.8.4/$public_old_release_asset"
	fi
	public_old_release_actual=$(public_host_helper_hash "$public_old_release_archive")
	if [ "$public_old_release_actual" != "$public_old_release_digest" ]; then
		echo 'old release archive does not match immutable lock' >&2
		return 1
	fi
	# Extract only the single checksum-authenticated release member. No cache or
	# inherited executable/receipt is accepted as provenance for the old owner.
	public_old_release_members=$(tar -tzf "$public_old_release_archive")
	if [ "$public_old_release_members" != hippo ]; then
		echo 'old release archive must contain only hippo' >&2
		return 1
	fi
	tar -xzf "$public_old_release_archive" -C "$public_old_release_directory" hippo
	HIPPO_PUBLIC_OLD_BIN=$public_old_release_directory/hippo
	if [ ! -f "$HIPPO_PUBLIC_OLD_BIN" ] || [ -L "$HIPPO_PUBLIC_OLD_BIN" ]; then
		echo 'old release member must be a regular executable' >&2
		return 1
	fi
	chmod 500 "$HIPPO_PUBLIC_OLD_BIN"
	HIPPO_PUBLIC_OLD_BIN_SHA256=$(public_host_helper_hash "$HIPPO_PUBLIC_OLD_BIN")
	HIPPO_PUBLIC_OLD_VERSION_RECEIPT=$public_old_release_directory/version.stdout
	"$HIPPO_PUBLIC_OLD_BIN" version --json >"$HIPPO_PUBLIC_OLD_VERSION_RECEIPT" \
		2>"$public_old_release_directory/version.stderr"
	public_old_release_go=${HIPPO_GO_BINARY:-go}
	GOMAXPROCS=2 "$public_old_release_go" run -p=1 ./tests/support/oldidentity "$HIPPO_PUBLIC_OLD_VERSION_RECEIPT"
	printf '%s\n' '{"schemaVersion":1,"version":"v0.8.4","commit":"abde2ecbeff7513549cfd1c0805fd20b5d232c01"}' \
		>"$public_old_release_directory/version.expected"
	if [ -s "$public_old_release_directory/version.stderr" ] ||
		! cmp "$HIPPO_PUBLIC_OLD_VERSION_RECEIPT" "$public_old_release_directory/version.expected"; then
		echo 'old release executable identity does not match immutable version and commit' >&2
		return 1
	fi
	printf '%s %s\n' "$public_old_release_digest" "$public_old_release_platform" \
		>"$public_old_release_directory/archive.sha256"
	printf '%s\n' "$HIPPO_PUBLIC_OLD_BIN_SHA256" >"$public_old_release_directory/binary.sha256"
	export HIPPO_PUBLIC_OLD_BIN HIPPO_PUBLIC_OLD_BIN_SHA256 HIPPO_PUBLIC_OLD_VERSION_RECEIPT
}
