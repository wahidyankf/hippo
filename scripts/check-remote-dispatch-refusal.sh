#!/usr/bin/env bash
# Verify the remote-dispatch placement and refusal contract (RD-A2).
set -euo pipefail

repo_root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)

entrypoint="$repo_root/repo-governance/development/resource-aware-development.md"
map="$repo_root/repo-governance/development/README.md"
config="$repo_root/repo-config.yml"

[[ -r "$entrypoint" ]] || {
	printf 'FAIL: remote-dispatch entrypoint is missing or unreadable\n' >&2
	exit 1
}

grep -Fq 'REMOTE_DISPATCH_HOME' "$entrypoint"
grep -Fq 'dispatch must not happen' "$entrypoint"
grep -Fq 'recorded' "$entrypoint"
grep -Fq 'refusal' "$entrypoint"
grep -Fq '429a891' "$entrypoint"
grep -Fq 'measured' "$entrypoint"
grep -Fq 'pressure' "$entrypoint"
grep -Fq 'HIPPO boundary' "$entrypoint"
grep -Fq 'placed on' "$entrypoint"
grep -Fq 'target host' "$entrypoint"
grep -Fq 'before remote work' "$entrypoint"
grep -Fq 'deleted last' "$entrypoint"
grep -Fq 'public-repository-data-safety.md' "$entrypoint"
grep -Fq 'Remote dispatch' "$entrypoint"
grep -Fq 'remote dispatch' "$map"
grep -Fq 'remote-dispatch-refusal' "$config"

printf '%s\n' 'Remote-dispatch refusal contract: PASS'
