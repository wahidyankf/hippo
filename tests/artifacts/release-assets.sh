#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
  echo "usage: $0 <directory> <version>" >&2
  exit 1
fi

directory=$1
version=$2

# A release is complete only when all supported platform archives and one
# four-entry checksum inventory are present and internally consistent.
for target in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do
  test -f "$directory/resource-guard_${version}_${target}.tar.gz"
done
test -f "$directory/checksums.txt"
test "$(wc -l < "$directory/checksums.txt" | tr -d ' ')" -eq 4

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$directory" && sha256sum --check checksums.txt)
else
  (cd "$directory" && shasum -a 256 --check checksums.txt)
fi
