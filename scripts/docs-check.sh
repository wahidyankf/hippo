#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

# A Git hook running inside a linked worktree exports GIT_DIR, and Git prefers
# it over both the working directory and -C. Nothing here runs Git today, but
# the pinned wrapper below resolves a release into a shared cache and this file
# is one of three entry points a hook invokes directly.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY \
	GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_COMMON_DIR GIT_NAMESPACE GIT_PREFIX

# Documentation hygiene, against the pinned RHINO release in rhino.lock. Six
# independent questions with no ordering between them, so they run together and
# the gate waits for the slowest rather than the sum; one resolution serves all
# six, because the wrapper verifies and installs before handing over.
#
# Extracted from the quick gate so the pull-request gate can name this check as
# a job of its own without holding a second copy of the invocation.
#
# shellcheck disable=SC2016 # $RHINO_BIN belongs to the shell ./rhino execs.
./rhino --bootstrap-exec sh -c '
	set -u
	"$RHINO_BIN" repo-config validate & a=$!
	"$RHINO_BIN" governance word-budget validate & b=$!
	"$RHINO_BIN" governance directory-map validate & c=$!
	"$RHINO_BIN" harness parity validate & d=$!
	"$RHINO_BIN" md internal-link validate & e=$!
	"$RHINO_BIN" md mermaid validate & f=$!
	documentation=0
	for check in $a $b $c $d $e $f; do
		wait "$check" || documentation=1
	done
	exit "$documentation"
'
