# Architecture Checks

`go test -count=1 ./tests/architecture` verifies production imports across all platform files and exercises rejected
dependency fixtures. The checker reads Go syntax without executing any fixture.
