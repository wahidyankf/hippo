#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

# The quick contract covers formatting, compilation, strict lint, unit tests,
# 99% deterministic-core coverage, behavior adapters, and repository policy
# before a push is allowed.
./scripts/format-check.sh
go test -run '^$' ./...
go tool golangci-lint run
go test -count=1 ./internal/evidence ./internal/release ./tests/unit
mkdir -p coverage
go test -count=1 -coverpkg=./internal/policy,./internal/config,./internal/host,./internal/evidence -coverprofile=coverage/unit.out ./tests/unit
go run ./tests/coverage --profile coverage/unit.out --directories internal/policy,internal/config --files internal/host/collector.go,internal/host/linux_parsers.go,internal/evidence/histogram.go --minimum 99
RESOURCE_GUARD_BDD_ADAPTER=unit go test -count=1 ./tests/bdd
RESOURCE_GUARD_BDD_ADAPTER=integration go test -count=1 ./tests/bdd
RESOURCE_GUARD_BDD_ADAPTER=e2e go test -count=1 ./tests/bdd
./tests/artifacts/run.sh
