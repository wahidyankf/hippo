#!/bin/sh
set -eu

# The integration gate: the integration package with the race detector, and the
# integration behaviour suite, as the test:integration target (C14). The
# package's OS-specific files are selected by the build constraints of the host
# running it, exactly as an untagged run always selected them. The gate runs
# unwrapped, because HIPPO cannot guard HIPPO: its caller owns admission and
# supervision. Network denial is not enforced here yet.

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

# A Git hook running inside a linked worktree exports GIT_DIR. The fixtures below
# create repositories by path, so the inherited variables must not reach them.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY \
	GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_COMMON_DIR GIT_NAMESPACE GIT_PREFIX

. ./tests/support/hostprobe/prepare.sh
trap public_host_helper_cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
public_host_helper_prepare "$root"

go test -race -count=1 ./tests/integration \
	-skip '^(TestUnitBehaviours|TestIntegrationBehaviours|TestE2EBehaviours|TestFeatureCompliance)$'
HIPPO_BDD_SUITE=integration go test -race -count=1 -run '^TestIntegrationBehaviours$' ./tests/bdd
