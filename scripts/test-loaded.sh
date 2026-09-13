#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

# HIPPO exists for hosts under contention, so its own gate has to run on one.
# Every flake this suite has produced was a constant chosen against an idle
# machine and then used as if it were a property: a termination grace below the
# host's scheduling jitter, a one-second bound on cancel-to-reap, a fifteen
# second budget for a run that spends most of it clearing admission. None of
# them reproduced idle and all of them reproduced under load, so an idle-only
# gate cannot see the class at all.
#
# A failure here is a finding, not necessarily a defect in the product: the
# usual cause is a fixture bound that is tighter than the host's jitter.

multiplier=${HIPPO_LOAD_MULTIPLIER:-2}
if command -v nproc >/dev/null 2>&1; then
	cores=$(nproc)
else
	cores=$(sysctl -n hw.ncpu)
fi
workers=$((cores * multiplier))

load_pids=""

# Reclaim the load on every exit path. A leaked busy loop outlives the job and
# silently poisons every later measurement taken on the same host.
cleanup() {
	if [ -n "$load_pids" ]; then
		# shellcheck disable=SC2086 # The list is shell-generated pids, split on purpose.
		kill $load_pids 2>/dev/null || true
	fi
}

trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

worker=0
while [ "$worker" -lt "$workers" ]; do
	(while :; do :; done) &
	load_pids="$load_pids $!"
	worker=$((worker + 1))
done

# Go's default ten minute per-package test timeout is sized for an idle machine.
# Under this much contention the unit and integration suites legitimately need
# longer, and a timeout panic says nothing about the code. Raise it through
# GOFLAGS so the shared release gate stays exactly as CI runs it elsewhere.
GOFLAGS="${GOFLAGS:+$GOFLAGS }-timeout=${HIPPO_LOAD_TEST_TIMEOUT:-60m}"
export GOFLAGS

# With at least one busy worker per core, CPU never falls under any profile
# ceiling, so a guarded child can never clear admission on this host. The
# end-to-end fixtures that start one read this flag and accept only HIPPO's
# documented deferral, which is the product working rather than a finding.
if [ "$workers" -ge "$cores" ]; then
	HIPPO_LOAD_SATURATED=1
	export HIPPO_LOAD_SATURATED
fi

echo "running the release gate against $workers busy workers on $cores cores"
./scripts/test.sh
