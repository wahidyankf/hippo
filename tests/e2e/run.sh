#!/bin/sh
set -eu

tool_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
temporary_parent=${RESOURCE_GUARD_E2E_TEMP_PARENT:-${TMPDIR:-/tmp}}
go_binary=${RESOURCE_GUARD_GO_BINARY:-go}
temporary_dir=$(mktemp -d "$temporary_parent/resource-guard-e2e.XXXXXX")

cleanup() {
  rm -rf -- "$temporary_dir"
}

trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

cd "$tool_dir"
GOMAXPROCS=2 "$go_binary" build -p=1 -trimpath \
  -ldflags "-X github.com/wahidyankf/resource-guard/internal/cli.Version=v0.0.0-test -X github.com/wahidyankf/resource-guard/internal/cli.Commit=0000000000000000000000000000000000000000" \
  -o "$temporary_dir/resource-guard" ./cmd/resource-guard
RESOURCE_GUARD_BIN="$temporary_dir/resource-guard" "$go_binary" test -count=1 ./tests/e2e
