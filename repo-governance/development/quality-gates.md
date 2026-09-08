# Quality Gates

What runs, where, and in what order.

## Locally

- **`commit-msg`** — commitlint. Conventional Commits, on every commit.
- **`pre-commit`** — `lint-staged`: `goimports` and `gofumpt` over staged Go, `shfmt` over staged shell, Prettier over staged JSON, Markdown, and YAML.
- **`pre-push`** — `npm run test:quick`, the whole quick gate, unguarded.

Install them with `npm ci`. A worktree whose hooks never ran pushes unverified work — see [integration path](../conventions/integration-path.md).

## The Quick Gate

`scripts/test-quick.sh`, in order: formatting, whole-module compilation, strict lint, unit tests, deterministic core coverage at 99%, the three behaviour adapters serially, artifact policy, and documentation hygiene under the pinned RHINO.

Order is deliberate. The cheapest failure to read comes first, so a formatting mistake does not cost a coverage run to discover.

## The Full Gate

`scripts/test.sh` adds the integration adapter, compiled end-to-end behaviour, a race-detected pass, and `govulncheck`. It is the release gate, and it is what CI runs on every supported platform.

## In CI

[`pr-quality-gate.yml`](../../.github/workflows/pr-quality-gate.yml) mirrors every hook contract and absorbs everything the retired `ci.yml` ran. One aggregate check, `Quality gate`, is the sole required status check; it treats a skipped or cancelled job as failure, because a skipped required check never reports at all.

## Never Nx

This repository does not use Nx and must not acquire it. `npm run test:quick` and `npm test` are the two entry points, and both call a shell script directly.

Failures are repaired at the cause. See [push hook verification](../conventions/push-hook-verification.md).
