# Quality Gates

What runs, where, and in what order.

## Locally

Each hook runs `./rhino gate run --surface <hook>`; [`repo-config.yml`](../../repo-config.yml) declares the gates each surface runs, and `./rhino gate list` prints them.

- **`commit-msg`** — the public-safety message screen, then commitlint through `scripts/check-commit-message.sh`: Conventional Commits, on every commit.
- **`pre-commit`** — the public-safety tree screen, then `scripts/format-staged.sh`: `goimports` and `gofumpt` over staged Go, `shfmt` over staged shell, Prettier over staged JSON, Markdown, and YAML.
- **`pre-push`** — the public-safety range screen, `shell-lint`, the quick gate, repository configuration, and the documentation gates, unguarded.
- **`shell-lint`** — on `pre-push` and in the pull-request replay: the checksum-pinned ShellCheck at `--severity=warning` over every shell file `scripts/shell-files.sh` lists, the same list the format check hands `shfmt`.

Install them with `npm ci`. A worktree whose hooks never ran pushes unverified work — see [integration path](../conventions/integration-path.md).

## The Quick Gate

`scripts/test-quick.sh`, in order: the worktree layout check, formatting, whole-module compilation, strict lint, the tests of every package under `./cmd/...` and `./internal/...` plus `tests/support`, the `tests/unit` corpus once under deterministic core coverage at 99%, the three behaviour adapters serially, and artifact policy. Documentation hygiene is not in it; it runs as its own gates on the same surfaces.

Every package holding a `_test.go` file must be run by a `go test` pattern in `scripts/test-quick.sh`, `scripts/test.sh`, or `tests/e2e/run.sh`; the compile-only `-run '^$'` line does not count. The scenario "Every package with tests runs in a gate" holds that list complete, so a new package cannot land with tests that never execute.

Order is deliberate. The cheapest failure to read comes first, so a formatting mistake does not cost a coverage run to discover.

## The Full Gate

`scripts/test.sh` adds the integration adapter, compiled end-to-end behaviour, a race-detected pass over every `./cmd/...` and `./internal/...` package, `tests/support`, and the unit and integration corpora, and `govulncheck`. It is the release gate, and CI runs it on `ubuntu-24.04` and `macos-15`.

## In CI

[`pr-quality-gate.yml`](../../.github/workflows/pr-quality-gate.yml) mirrors every hook contract and absorbs everything the retired `ci.yml` ran. One aggregate check, `Quality gate`, is the sole required status check; it treats a skipped or cancelled job as failure, because a skipped required check never reports at all.

## Never Nx

This repository does not use Nx and must not acquire it. `npm run test:quick` and `npm test` are the two entry points, and both call a shell script directly.

Failures are repaired at the cause. See [push hook verification](../conventions/push-hook-verification.md).
