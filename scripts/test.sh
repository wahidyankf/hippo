#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

# Extend the quick gate with process boundaries, race detection, and a current
# vulnerability scan. This is the release gate used by CI on every platform.
./scripts/test-quick.sh
go test -count=1 ./tests/integration
./tests/e2e/run.sh
go test -race -count=1 ./internal/evidence ./internal/release ./tests/unit ./tests/integration
go tool govulncheck ./...
