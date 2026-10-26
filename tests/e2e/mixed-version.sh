#!/bin/sh
set -eu

# The pull-request gate's bounded share of tests/e2e/run.sh. It runs only the
# tagged compatibility leaf: v0.10.0, which pins the very-heavy tier's hazard
# and owner row. It builds the same stamped candidate and prepares the same
# verified release as run.sh. The full suite still runs after merge in
# .github/workflows/main-test.yml.

tool_dir=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
temporary_parent=${HIPPO_E2E_TEMP_PARENT:-${TMPDIR:-/tmp}}
go_binary=${HIPPO_GO_BINARY:-go}
# Passive helper sourcing precedes every owned acquisition and trap.
. "$tool_dir/tests/support/hostprobe/prepare.sh"
. "$tool_dir/tests/e2e/v0100-release.sh"
temporary_dir=$(mktemp -d "$temporary_parent/hippo-mixed.XXXXXX")

# Every outcome removes the compiled candidate and the verified release copy.
cleanup() {
	public_host_helper_cleanup
	rm -rf -- "$temporary_dir"
}

trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

# Keep the acquired cleanup path until physical-path resolution succeeds.
canonical_temporary_dir=$(CDPATH='' cd -- "$temporary_dir" && pwd -P)
temporary_dir=$canonical_temporary_dir

cd "$tool_dir"
public_host_helper_prepare "$tool_dir"
GOMAXPROCS=2 "$go_binary" build -p=1 -trimpath -buildvcs=false \
	-ldflags "-X github.com/wahidyankf/hippo/internal/adapters/cli.Version=v0.0.0-test -X github.com/wahidyankf/hippo/internal/adapters/cli.Commit=0000000000000000000000000000000000000000" \
	-o "$temporary_dir/hippo" ./cmd/hippo
public_v0100_release_prepare "$tool_dir" "$temporary_dir"
HIPPO_BIN="$temporary_dir/hippo" "$go_binary" test -tags=mixedversion -count=1 -timeout=120s -p=2 \
	-run '^TestMixedVersionV0100VeryHeavyCompatibility$' ./tests/integration
