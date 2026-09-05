#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
	echo "usage: $0 <version> <commit> <output-dir>" >&2
	exit 1
fi

version=$1
commit=$2
output_dir=$3
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

# Release identity is embedded in every binary and must be unambiguous before
# any output directory is touched.
if ! printf '%s\n' "$version" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'; then
	echo "version must be a v-prefixed semantic version" >&2
	exit 1
fi
if [ "${#commit}" -ne 40 ]; then
	echo "commit must be a 40-character lowercase Git object ID" >&2
	exit 1
fi
case "$commit" in
*[!0-9a-f]*)
	echo "commit must be a 40-character lowercase Git object ID" >&2
	exit 1
	;;
esac
if ! git -C "$root" cat-file -e "$commit^{commit}" 2>/dev/null; then
	echo "commit must identify a real Git commit" >&2
	exit 1
fi
head_commit=$(git -C "$root" rev-parse HEAD)
if [ "$commit" != "$head_commit" ]; then
	echo "commit must equal checkout HEAD" >&2
	exit 1
fi
if ! checkout_status=$(git -C "$root" status --porcelain --untracked-files=all); then
	echo "release checkout cleanliness could not be verified" >&2
	exit 1
fi
if [ -n "$checkout_status" ]; then
	echo "release checkout must be clean, including untracked files" >&2
	exit 1
fi

mkdir -p "$output_dir"
output_dir=$(CDPATH= cd -- "$output_dir" && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/hippo-release.XXXXXX")
materialization=
cleanup() {
	rm -rf -- "$work" "$materialization"
}
trap 'cleanup' EXIT HUP INT TERM
materialization=$(mktemp -d "${TMPDIR:-/tmp}/hippo-release-source.XXXXXX")
source_root="$materialization/source"
git clone --quiet --no-checkout --no-local "$root" "$source_root"
git -C "$source_root" checkout --detach --quiet "$commit"

# Build the complete supported matrix from one commit with CGO disabled, which
# keeps the archives independent from runner-local system libraries.
for target in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do
	goos=${target%_*}
	goarch=${target#*_}
	binary="$work/hippo"
	archive="hippo_${version}_${goos}_${goarch}.tar.gz"
	(
		cd "$source_root"
		CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -p=1 -trimpath -buildvcs=true \
			-ldflags "-s -w -X github.com/wahidyankf/hippo/internal/cli.Version=$version -X github.com/wahidyankf/hippo/internal/cli.Commit=$commit" \
			-o "$binary" ./cmd/hippo
	)
	chmod 755 "$binary"
	# Ownership metadata is pinned so an archive never carries the build user.
	# The name:id spelling is the only one both implementations accept: GNU tar
	# rejects bsdtar's --uid/--gid/--uname/--gname, and the release runner is
	# Linux.
	tar --format ustar --owner=root:0 --group=root:0 -C "$work" -czf "$output_dir/$archive" hippo
done

# Publish one checksum inventory covering exactly the archives above. The
# companion artifact test verifies both the names and these digests.
(
	cd "$output_dir"
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum hippo_*.tar.gz >checksums.txt
	else
		shasum -a 256 hippo_*.tar.gz >checksums.txt
	fi
)

trap - EXIT HUP INT TERM
cleanup
