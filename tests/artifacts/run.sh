#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

for ignored in resource-guard.local.json .env .env.local .cache/example dist/example coverage/example local-tmp/example generated-output/example; do
  git check-ignore --quiet "$ignored"
done

if git ls-files --error-unmatch resource-guard.local.json >/dev/null 2>&1; then
  echo "resource-guard.local.json must not be tracked" >&2
  exit 1
fi

for tracked in go.mod go.sum resource-guard resource-guard.local.json.example specs/behaviours/README.md scripts/test.sh scripts/test-quick.sh scripts/build-release.sh; do
  git check-ignore --quiet "$tracked" && {
    echo "$tracked must remain committable" >&2
    exit 1
  }
  test -f "$tracked"
done

tracked_artifacts=$(git ls-files '.env' '.env.*' '.cache/**' 'dist/**' 'coverage/**' 'local-tmp/**' 'generated-output/**' '*.test')
if [ -n "$tracked_artifacts" ]; then
  echo "generated or private artifacts are tracked:" >&2
  echo "$tracked_artifacts" >&2
  exit 1
fi

test "$(sed -n '1p' resource-guard)" = '#!/bin/sh'
test ! -e project.json
test ! -e package.json
test "$(sed -n '1p' go.mod)" = 'module github.com/wahidyankf/resource-guard'
