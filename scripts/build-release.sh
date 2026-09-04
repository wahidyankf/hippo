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
case "$version" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "version must be a v-prefixed semantic version" >&2; exit 1 ;;
esac
if [ "${#commit}" -ne 40 ]; then
  echo "commit must be a 40-character lowercase Git object ID" >&2
  exit 1
fi
case "$commit" in
  *[!0-9a-f]*) echo "commit must be a 40-character lowercase Git object ID" >&2; exit 1 ;;
esac

mkdir -p "$output_dir"
output_dir=$(CDPATH= cd -- "$output_dir" && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/resource-guard-release.XXXXXX")
trap 'rm -rf -- "$work"' EXIT HUP INT TERM

# Build the complete supported matrix from one commit with CGO disabled, which
# keeps the archives independent from runner-local system libraries.
for target in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do
  goos=${target%_*}
  goarch=${target#*_}
  binary="$work/resource-guard"
  archive="resource-guard_${version}_${goos}_${goarch}.tar.gz"
  (
    cd "$root"
    CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -p=1 -trimpath \
      -ldflags "-s -w -X github.com/wahidyankf/resource-guard/internal/cli.Version=$version -X github.com/wahidyankf/resource-guard/internal/cli.Commit=$commit" \
      -o "$binary" ./cmd/resource-guard
  )
  chmod 755 "$binary"
  tar -C "$work" -czf "$output_dir/$archive" resource-guard
done

# Publish one checksum inventory covering exactly the archives above. The
# companion artifact test verifies both the names and these digests.
(
  cd "$output_dir"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum resource-guard_*.tar.gz > checksums.txt
  else
    shasum -a 256 resource-guard_*.tar.gz > checksums.txt
  fi
)
