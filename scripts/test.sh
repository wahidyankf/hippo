#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

./scripts/test-quick.sh
go test -count=1 ./tests/integration
./tests/e2e/run.sh
go test -race -count=1 ./internal/release ./tests/unit ./tests/integration
go tool govulncheck ./...
