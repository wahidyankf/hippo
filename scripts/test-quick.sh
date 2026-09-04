#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

go test -run '^$' ./...
go tool golangci-lint run
go test -count=1 ./internal/release ./tests/unit
mkdir -p coverage
go test -count=1 -coverpkg=./internal/policy,./internal/config,./internal/host -coverprofile=coverage/unit.out ./tests/unit
go run ./tests/coverage --profile coverage/unit.out --directories internal/policy,internal/config --files internal/host/collector.go,internal/host/linux_parsers.go --minimum 99
RESOURCE_GUARD_BDD_ADAPTER=unit go test -count=1 ./tests/bdd
RESOURCE_GUARD_BDD_ADAPTER=integration go test -count=1 ./tests/bdd
RESOURCE_GUARD_BDD_ADAPTER=e2e go test -count=1 ./tests/bdd
./tests/artifacts/run.sh
