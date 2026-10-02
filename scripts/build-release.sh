#!/bin/sh
set -eu

# This script clones the checkout and then checks out a commit inside the clone.
# A Git hook in a linked worktree exports GIT_DIR, which Git prefers over -C, so
# without this the detach would move the real repository's HEAD instead.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY \
	GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_COMMON_DIR GIT_NAMESPACE GIT_PREFIX

if [ "$#" -ne 3 ]; then
	echo "usage: $0 <version> <commit> <output-dir>" >&2
	exit 1
fi

version=$1
commit=$2
output_dir=$3
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)

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
output_dir=$(CDPATH='' cd -- "$output_dir" && pwd)
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

# The Go toolchain is an input to the binary, so a newer local Go would build
# different bytes from the same commit. Build with the release go.mod names --
# its toolchain line, else its go line -- the same choice setup-go makes for
# release.yml, and let the go command fetch that release when it is not local.
toolchain=$(sed -n 's/^toolchain[[:space:]][[:space:]]*\(go[^[:space:]]*\)[[:space:]]*$/\1/p' "$source_root/go.mod")
if [ -z "$toolchain" ]; then
	toolchain=go$(sed -n 's/^go[[:space:]][[:space:]]*\([^[:space:]]*\)[[:space:]]*$/\1/p' "$source_root/go.mod")
fi
if ! printf '%s\n' "$toolchain" | grep -Eq '^go[0-9]+\.[0-9]+\.[0-9]+$'; then
	echo "go.mod must name an exact Go release to build with" >&2
	exit 1
fi
GOTOOLCHAIN=$toolchain
# Host Go settings would change the binary too: the user's go env file, extra
# build flags, experiments, and the architecture levels a host may raise.
GOENV=off
export GOTOOLCHAIN GOENV
unset GOFLAGS GOEXPERIMENT GOAMD64 GOARM64 GOFIPS140

# Every member carries the release commit's time rather than the build's, and
# the archiver is built from the release commit itself, so the archive bytes
# depend on the commit alone.
modified=$(git -C "$source_root" show -s --format=%ct "$commit")
archiver="$work/release-archive"
(
	cd "$source_root"
	go build -o "$archiver" ./scripts/release-archive
)

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
	# The archiver writes the one member as root-owned mode 755 whatever the
	# build user or host tar, and never the build time.
	"$archiver" -binary "$binary" -mtime "$modified" -output "$output_dir/$archive"
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
