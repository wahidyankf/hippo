#!/bin/sh
set -eu

tool_dir=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
temporary_parent=${HIPPO_E2E_TEMP_PARENT:-${TMPDIR:-/tmp}}
go_binary=${HIPPO_GO_BINARY:-go}
# Passive helper sourcing precedes every owned acquisition and trap. A source
# refusal therefore retains its genuine status and owns no E2E temporary yet.
. "$tool_dir/tests/support/hostprobe/prepare.sh"
. "$tool_dir/tests/e2e/old-release.sh"
. "$tool_dir/tests/e2e/v085-release.sh"
temporary_dir=$(mktemp -d "$temporary_parent/hippo-e2e.XXXXXX")

# Every outcome removes the compiled fixture; signal-specific exits make an
# interrupted harness visible without leaking temporary binaries.
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
# Exercise the public process boundary with the same embedded identity used by
# tagged binaries, while keeping compilation concurrency bounded.
GOMAXPROCS=2 "$go_binary" build -p=1 -trimpath -buildvcs=false \
	-ldflags "-X github.com/wahidyankf/hippo/internal/adapters/cli.Version=v0.0.0-test -X github.com/wahidyankf/hippo/internal/adapters/cli.Commit=0000000000000000000000000000000000000000" \
	-o "$temporary_dir/hippo" ./cmd/hippo
public_old_release_prepare "$tool_dir" "$temporary_dir"
public_v085_release_prepare "$tool_dir" "$temporary_dir"
# Both immutable releases are prepared independently; this tagged leaf uses
# verified v0.8.5 and the same current candidate as the retained v0.8.4 corpus.
HIPPO_BIN="$temporary_dir/hippo" "$go_binary" test -tags=mixedversion -count=1 -timeout=120s -p=2 \
	-run '^TestMixedVersionV085DisplayInteroperability$' ./tests/integration
HIPPO_BIN="$temporary_dir/hippo" "$go_binary" test -count=1 ./tests/e2e \
	-skip '^(TestUnitBehaviours|TestIntegrationBehaviours|TestE2EBehaviours|TestFeatureCompliance)$'
HIPPO_BDD_SUITE=e2e HIPPO_BIN="$temporary_dir/hippo" "$go_binary" test -count=1 \
	-run '^TestE2EBehaviours$' ./internal/adapters/tui
# These public entries retain their own canonical execution and native receipts.
# All use the same stamped candidate and prepared helper, and run serially.
HIPPO_BDD_SUITE=e2e HIPPO_BDD_ADAPTER=e2e HIPPO_BIN="$temporary_dir/hippo" "$go_binary" test -count=1 \
	-run '^TestTUIBDDPublicCanonicalSuite$' ./internal/adapters/tui
HIPPO_BDD_SUITE=e2e HIPPO_BDD_ADAPTER=e2e HIPPO_BIN="$temporary_dir/hippo" "$go_binary" test -count=1 \
	-run '^TestTUIBDDPublicObservationCanonical$' ./internal/adapters/tui
HIPPO_BDD_SUITE=e2e HIPPO_BDD_ADAPTER=e2e HIPPO_BIN="$temporary_dir/hippo" "$go_binary" test -count=1 \
	-run '^TestTUIBDDPublicSessionHolderSignals$' ./internal/adapters/tui
