#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

# Private configuration and generated state must be ignored at their exact
# repository-relative locations.
for ignored in hippo.local.json .env .env.local .cache/example dist/example coverage/example local-tmp/example generated-reports/example node_modules/example .husky/_/example; do
	git check-ignore --quiet "$ignored"
done

if git ls-files --error-unmatch hippo.local.json >/dev/null 2>&1; then
	echo "hippo.local.json must not be tracked" >&2
	exit 1
fi

# Conversely, source, specs, examples, and enforcement must remain publishable.
for tracked in go.mod go.sum package.json package-lock.json commitlint.config.cjs lint-staged.config.mjs hippo hippo.local.json.example .husky/commit-msg .husky/pre-commit .husky/pre-push specs/behaviours/README.md scripts/format-check.sh scripts/test.sh scripts/test-quick.sh scripts/build-release.sh; do
	git check-ignore --quiet "$tracked" && {
		echo "$tracked must remain committable" >&2
		exit 1
	}
	test -f "$tracked"
done

# Version-four coordination, terminal, conformance, and release-hardening
# surfaces are explicit repository artifacts rather than incidental files.
for tracked in conformance.manifest.json.example cmd/hippo-conformance/main.go internal/conformance/conformance.go internal/conformance/conformance_test.go internal/guard/lifetime.go internal/guard/reservation.go internal/guard/run_test.go internal/guard/terminal.go internal/policy/units.go specs/behaviours/conformance.feature specs/behaviours/reservations.feature specs/behaviours/terminal.feature tests/integration/conformance_test.go tests/integration/pty_test.go tests/integration/reservation_test.go tests/support/blockers_v04.go tests/support/pending_v04.go tests/support/release_v04.go tests/support/review_v04.go tests/unit/config_schema2_errors_test.go tests/unit/conformance_test.go tests/unit/reservation_test.go; do
	git check-ignore --quiet "$tracked" && {
		echo "$tracked must remain committable" >&2
		exit 1
	}
	test -f "$tracked"
done

tracked_artifacts=$(git ls-files '.env' '.env.*' '.cache/**' 'dist/**' 'coverage/**' 'local-tmp/**' 'generated-reports/**' 'node_modules/**' '.husky/_/**' '*.test')
if [ -n "$tracked_artifacts" ]; then
	echo "generated or private artifacts are tracked:" >&2
	echo "$tracked_artifacts" >&2
	exit 1
fi

# These layout assertions prevent the standalone module from drifting back
# into a workspace-specific package or tool tree.
test "$(sed -n '1p' hippo)" = '#!/bin/sh'
test ! -e project.json
test ! -e nx.json
grep -q '"private": true' package.json
if grep -Eq '"(nx|workspaces)"[[:space:]]*:' package.json; then
	echo "package.json must remain private contributor tooling without Nx workspace metadata" >&2
	exit 1
fi
test "$(sed -n '1p' go.mod)" = 'module github.com/wahidyankf/hippo'
