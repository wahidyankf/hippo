# CI Placement

The pull-request gate mirrors the hooks and nothing heavier. Heavy gates run after merge on `main`, so a regression can
reach `main` before it is detected. That cost is accepted: each merge would otherwise send every other pull request back
to rebase and wait for macOS.

## Pull Requests

[`pr-quality-gate.yml`](../../../.github/workflows/pr-quality-gate.yml) runs these jobs:

- `Formatting`: `scripts/format-check.sh`.
- `Repository contract`: `./rhino gate run --surface pull-request`. That surface runs the `quick` gate (typecheck, lint,
  unit, and behaviour adapters) with the documentation and configuration gates.
- `Pinned RHINO bootstrap`: only when the consumer boundary changes.
- `Mixed-version compatibility`: `tests/e2e/mixed-version.sh`, the tagged leaves that compare the released v0.8.5 and
  v0.10.0 binaries with the current build on a shared ledger. Its result depends on the host's CPU count, so the
  runner's own count is the evidence. It runs on the same `ubuntu-24.04` runner as the other jobs and nothing else from
  the full suite.
- `Release build`: every release asset, and the Linux checksum inventory.

The aggregate `Quality gate` is the required check. It fails skipped or cancelled jobs, because a skipped required check
never reports. The [leak review](../../workflows/quality/pr-leak-review/003-enforcement.md) is a local harness verdict,
not a required check. Keep storage within the [GitHub Actions storage](../github-actions-storage.md) allowance.

## After Merge

[`main-test.yml`](../../../.github/workflows/main-test.yml) runs on each push to `main` and on manual dispatch. Each
`main` commit keeps its own result, because the run is never cancelled by a later commit. It runs:

- the full gate, `./scripts/test.sh`, on `ubuntu-24.04` and `macos-15`;
- the macOS release reproducibility check: the same commit must rebuild the Linux checksums byte for byte.

## A Failure on Main

A failed run on `main` is an immediate-priority defect. Fix it forward, or revert it, before any release is cut.

[`release.yml`](../../../.github/workflows/release.yml) refuses to publish a tag whose commit has no passing main-test
run. A run still in progress is fixed by re-running the release once it passes. A failed run is not: the commit is not
releasable, and the next version is the fix.
