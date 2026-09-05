#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
	echo "usage: $0 <directory> <version> <commit>" >&2
	exit 1
fi

directory=$1
version=$2
commit=$3

if ! printf '%s\n' "$version" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'; then
	echo "release asset version is invalid" >&2
	exit 1
fi
if [ "${#commit}" -ne 40 ] || printf '%s\n' "$commit" | grep -Eq '[^0-9a-f]'; then
	echo "release asset commit is invalid" >&2
	exit 1
fi

work=$(mktemp -d "${TMPDIR:-/tmp}/hippo-release-assets.XXXXXX")
trap 'rm -rf -- "$work"' EXIT HUP INT TERM
expected_names="$work/expected-names"
actual_names="$work/actual-names"
: >"$expected_names"

# A release is complete only when all supported platform archives and one
# four-entry checksum inventory are present and internally consistent.
for target in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do
	archive="hippo_${version}_${target}.tar.gz"
	test -f "$directory/$archive"
	printf '%s\n' "$archive" >>"$expected_names"
done
test -f "$directory/checksums.txt"
printf '%s\n' checksums.txt >>"$expected_names"
find "$directory" -mindepth 1 -maxdepth 1 -print | sed 's#^.*/##' | LC_ALL=C sort >"$actual_names"
LC_ALL=C sort -o "$expected_names" "$expected_names"
cmp -s "$expected_names" "$actual_names" || {
	echo "release directory must contain exactly the supported archives and checksums" >&2
	exit 1
}
test "$(wc -l <"$directory/checksums.txt" | tr -d ' ')" -eq 4

checksum_names="$work/checksum-names"
awk 'NF != 2 || length($1) != 64 || $1 !~ /^[0-9a-f]+$/ { exit 1 } { name=$2; sub(/^\*/, "", name); print name }' "$directory/checksums.txt" | LC_ALL=C sort >"$checksum_names"
sed '/checksums.txt/d' "$expected_names" >"$work/archive-names"
cmp -s "$work/archive-names" "$checksum_names" || {
	echo "checksum inventory must name every supported archive exactly once" >&2
	exit 1
}

if command -v sha256sum >/dev/null 2>&1; then
	(cd "$directory" && sha256sum --check checksums.txt)
else
	(cd "$directory" && shasum -a 256 --check checksums.txt)
fi

native_target="$(go env GOOS)_$(go env GOARCH)"
native_binary=
archive_inspector="$work/archive-inspector.go"
cat >"$archive_inspector" <<'EOF'
package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
)

func inspect(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer compressed.Close()
	archive := tar.NewReader(compressed)
	header, err := archive.Next()
	if err != nil {
		return err
	}
	if header.Name != "hippo" || header.Typeflag != tar.TypeReg || header.Mode != 0755 {
		return errors.New("archive must contain exactly one regular mode-755 hippo member")
	}
	if header.Uid != 0 || header.Gid != 0 || header.Uname != "root" || header.Gname != "root" {
		return errors.New("archive ownership metadata must be normalized")
	}
	if _, err = archive.Next(); !errors.Is(err, io.EOF) {
		return errors.New("archive must contain exactly one member")
	}
	return nil
}

func main() {
	for _, path := range os.Args[1:] {
		if err := inspect(path); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
EOF
for target in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do
	archive="hippo_${version}_${target}.tar.gz"
	go run "$archive_inspector" "$directory/$archive"
	members="$work/${target}.members"
	tar -tzf "$directory/$archive" >"$members"
	if [ "$(wc -l <"$members" | tr -d ' ')" -ne 1 ] || [ "$(sed -n '1p' "$members")" != hippo ]; then
		echo "each release archive must contain only hippo" >&2
		exit 1
	fi
	member_metadata="$work/${target}.member-metadata"
	tar -tvzf "$directory/$archive" >"$member_metadata"
	if [ "$(wc -l <"$member_metadata" | tr -d ' ')" -ne 1 ] || [ "$(awk 'NR == 1 { print $1 }' "$member_metadata")" != '-rwxr-xr-x' ]; then
		echo "each release archive member must be one regular mode-755 file" >&2
		exit 1
	fi
	extract="$work/$target"
	mkdir "$extract"
	tar -xzf "$directory/$archive" -C "$extract"
	binary="$extract/hippo"
	test -f "$binary" && test -x "$binary"
	if mode=$(stat -f '%Lp' "$binary" 2>/dev/null); then
		:
	else
		mode=$(stat -c '%a' "$binary")
	fi
	if [ "$mode" != 755 ]; then
		echo "release binary mode must be 755" >&2
		exit 1
	fi
	metadata="$work/${target}.metadata"
	go version -m "$binary" >"$metadata"
	grep -Fq "vcs.revision=$commit" "$metadata" || {
		actual_revision=$(sed -n 's/^.*vcs.revision=//p' "$metadata")
		echo "release binary revision does not match: expected $commit actual ${actual_revision:-missing}" >&2
		exit 1
	}
	grep -Fq 'vcs.modified=false' "$metadata" || {
		echo "release binary source state is not clean" >&2
		exit 1
	}
	if [ "$target" = "$native_target" ]; then
		native_binary=$binary
	fi
done

if [ -z "$native_binary" ]; then
	echo "native release target is unsupported" >&2
	exit 1
fi
expected_json=$(printf '{"schemaVersion":1,"version":"%s","commit":"%s"}' "$version" "$commit")
actual_json=$("$native_binary" version --json)
if [ "$actual_json" != "$expected_json" ]; then
	echo "native release version identity does not match" >&2
	exit 1
fi
