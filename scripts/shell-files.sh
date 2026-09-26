#!/bin/sh
# Print every shell file this repository owns, one path per line, sorted:
# every tracked `*.sh` file plus the extensionless entrypoints. The formatter
# and the analyser both read this one list, so neither can cover a file the
# other misses. Husky's generated shims under `.husky/_/` are not listed; the
# install creates them, and this repository does not own them.
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

tracked=$(git ls-files -- '*.sh')
printf '%s\n' "$tracked" hippo rhino ferret \
	.husky/commit-msg .husky/pre-commit .husky/pre-push | LC_ALL=C sort -u
