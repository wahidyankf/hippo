#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

# Private configuration and generated state must be ignored at their exact
# repository-relative locations.
for ignored in resource-guard.local.json .env .env.local .cache/example dist/example coverage/example local-tmp/example generated-output/example node_modules/example .husky/_/example; do
	git check-ignore --quiet "$ignored"
done

if git ls-files --error-unmatch resource-guard.local.json >/dev/null 2>&1; then
	echo "resource-guard.local.json must not be tracked" >&2
	exit 1
fi

# Conversely, source, specs, examples, and enforcement must remain publishable.
for tracked in go.mod go.sum package.json package-lock.json commitlint.config.cjs lint-staged.config.mjs resource-guard resource-guard.local.json.example .husky/commit-msg .husky/pre-commit .husky/pre-push specs/behaviours/README.md scripts/format-check.sh scripts/test.sh scripts/test-quick.sh scripts/build-release.sh; do
	git check-ignore --quiet "$tracked" && {
		echo "$tracked must remain committable" >&2
		exit 1
	}
	test -f "$tracked"
done

tracked_artifacts=$(git ls-files '.env' '.env.*' '.cache/**' 'dist/**' 'coverage/**' 'local-tmp/**' 'generated-output/**' 'node_modules/**' '.husky/_/**' '*.test')
if [ -n "$tracked_artifacts" ]; then
	echo "generated or private artifacts are tracked:" >&2
	echo "$tracked_artifacts" >&2
	exit 1
fi

# These layout assertions prevent the standalone module from drifting back
# into a workspace-specific package or tool tree.
test "$(sed -n '1p' resource-guard)" = '#!/bin/sh'
test ! -e project.json
test ! -e nx.json
grep -q '"private": true' package.json
if grep -Eq '"(nx|workspaces)"[[:space:]]*:' package.json; then
	echo "package.json must remain private contributor tooling without Nx workspace metadata" >&2
	exit 1
fi
test "$(sed -n '1p' go.mod)" = 'module github.com/wahidyankf/resource-guard'
