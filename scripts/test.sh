#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

# A Git hook running inside a linked worktree exports GIT_DIR, and Git prefers
# it over both the working directory and -C. Everything below creates or
# inspects repositories by path; without this, a fixture's `git init` would
# re-initialize the repository under test and its commits would land on the
# branch being pushed.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY \
	GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_COMMON_DIR GIT_NAMESPACE GIT_PREFIX

# External helper compilation runs once before any dependent test process.
# Admission and finite process-group supervision belong to the gate caller.
. ./tests/support/hostprobe/prepare.sh
trap public_host_helper_cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
public_host_helper_prepare "$root"

# Extend the quick gate with process boundaries, race detection, and a current
# vulnerability scan. This is the release gate used by CI on every platform.
./scripts/test-quick.sh
go test -count=1 ./tests/integration \
	-skip '^(TestUnitBehaviours|TestIntegrationBehaviours|TestE2EBehaviours|TestFeatureCompliance)$'
HIPPO_BDD_SUITE=integration go test -count=1 -run '^TestIntegrationBehaviours$' ./tests/bdd
./tests/e2e/run.sh
go test -race -count=1 ./cmd/... ./internal/... ./tests/support ./tests/support/hostprobe ./tests/support/oldidentity ./tests/unit ./tests/bdd ./tests/integration \
	-skip '^(TestUnitBehaviours|TestIntegrationBehaviours|TestE2EBehaviours|TestFeatureCompliance)$'
go test -race -count=1 ./tests/contract
HIPPO_BDD_SUITE=unit go test -race -count=1 -run '^TestUnitBehaviours$' ./tests/bdd
HIPPO_BDD_SUITE=integration go test -race -count=1 -run '^TestIntegrationBehaviours$' ./tests/bdd
go tool govulncheck ./...
