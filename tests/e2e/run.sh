#!/bin/sh
set -eu

tool_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
temporary_parent=${HIPPO_E2E_TEMP_PARENT:-${TMPDIR:-/tmp}}
go_binary=${HIPPO_GO_BINARY:-go}
temporary_dir=$(mktemp -d "$temporary_parent/hippo-e2e.XXXXXX")

# Every outcome removes the compiled fixture; signal-specific exits make an
# interrupted harness visible without leaking temporary binaries.
cleanup() {
	rm -rf -- "$temporary_dir"
}

trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

cd "$tool_dir"
# Exercise the public process boundary with the same embedded identity used by
# tagged binaries, while keeping compilation concurrency bounded.
GOMAXPROCS=2 "$go_binary" build -p=1 -trimpath \
	-ldflags "-X github.com/wahidyankf/hippo/internal/cli.Version=v0.0.0-test -X github.com/wahidyankf/hippo/internal/cli.Commit=0000000000000000000000000000000000000000" \
	-o "$temporary_dir/hippo" ./cmd/hippo
HIPPO_BIN="$temporary_dir/hippo" "$go_binary" test -count=1 ./tests/e2e
