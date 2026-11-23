# CI Placement

The pull-request gate mirrors the hooks and adds the rest. Heavy gates run after merge on `main`, so a regression can
reach `main` before it is detected. That cost is accepted: each merge would otherwise send every other pull request back
to rebase and wait for macOS.

## Pull Requests

[`pr-quality-gate.yml`](../../../.github/workflows/pr-quality-gate.yml) runs these jobs:

- `Test quick`: `./rhino gate run --surface pull-request --entry quick`. Typecheck, strict lint, NilAway, unit tests,
  and the behaviour checks.
- `Test integration`: the `test-integration` entry, with the integration suites.
- `Pull-request checks`: every other pull-request entry, named one by one with `--entry`, so each runs in exactly one
  job.
- `Pinned RHINO bootstrap`: `./rhino version`, only when the consumer boundary or the workflow changes.
- `Release build`: every release asset, its inventory, and the Linux checksums.
- `Quality gate`: the aggregate. It fails when any job is not an explicit success, so a skipped or cancelled job cannot
  pass.

The mixed-version compatibility leaf is an end-to-end layer. It runs in the e2e suite, not per pull request. The full
list of entries and their surfaces is in the [gate inventory](002-gate-inventory.md).

## After Merge

[`main-test.yml`](../../../.github/workflows/main-test.yml) runs on each push to `main` and on manual dispatch. Each
`main` commit keeps its own result, because a later commit never cancels an earlier run. It runs:

- `./rhino gate run --surface main` on `ubuntu-24.04` and `macos-15`: `quick`, `test-integration`, `test-e2e`, and
  `vuln-scan`. The macOS leg is the only proof of the darwin-tagged host code, because Linux cannot compile it.
- the release reproducibility check: a macOS rebuild must produce the Linux checksums byte for byte.

## Scheduled

[`loaded-host.yml`](../../../.github/workflows/loaded-host.yml) runs nightly on both systems. It runs
`scripts/test-loaded.sh`: the quick, integration, and end-to-end targets under synthetic CPU load. A failure is a
finding to triage, usually a fixture bound tighter than the host's jitter. The job is not required.

## A Failure on Main

A failed run on `main` is an immediate-priority defect. Fix it forward, or revert it, before any release is cut.

[`release.yml`](../../../.github/workflows/release.yml) refuses to publish a tag whose commit has no passing main-test
run. A run still in progress is fixed by re-running the release once it passes. A failed run is not: the commit is not
releasable, and the next version is the fix.
