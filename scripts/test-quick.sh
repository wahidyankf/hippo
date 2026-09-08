#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

# The quick contract covers formatting, compilation, strict lint, unit tests,
# 99% deterministic-core coverage, behavior adapters, repository policy, and
# documentation hygiene before a push is allowed.
./scripts/format-check.sh
go test -run '^$' ./...
go tool golangci-lint run
go test -count=1 ./internal/evidence ./internal/release ./tests/unit
mkdir -p coverage
go test -count=1 -coverpkg=./internal/policy,./internal/config,./internal/host,./internal/evidence -coverprofile=coverage/unit.out ./tests/unit
go run ./tests/coverage --profile coverage/unit.out --directories internal/policy,internal/config --files internal/host/collector.go,internal/host/linux_parsers.go,internal/evidence/histogram.go --minimum 99
HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd
HIPPO_BDD_ADAPTER=integration go test -count=1 ./tests/bdd
HIPPO_BDD_ADAPTER=e2e go test -count=1 ./tests/bdd
./tests/artifacts/run.sh

# Documentation hygiene, against the pinned RHINO release in rhino.lock. Six
# independent questions with no ordering between them, so they run together and
# the gate waits for the slowest rather than the sum; one resolution serves all
# six, because the wrapper verifies and installs before handing over.
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
