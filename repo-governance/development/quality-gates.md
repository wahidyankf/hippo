# Quality Gates

The gates follow the [Test and Gate Contract](quality/checks/test-and-gate-contract.md). `repo-config.yml` declares
every gate under `gates.entries`. Hooks and workflows only call a surface, `./rhino gate run --surface <surface>`, and
`./rhino gate list` prints the declaration. The [gate inventory](quality-gates/002-gate-inventory.md) lists every entry.

## Surfaces

- **`pre-commit`**: `format-staged` formats the staged files and restages them.
- **`commit-msg`**: `commit-message` checks the Conventional Commits format.
- **`pre-push`**: `public-safety-range` scans the pushed commits for leaks. No test, lint, or document check runs here.
- **`pull-request`**: every other check, in parallel jobs of
  [the pull-request workflow](../../.github/workflows/pr-quality-gate.yml).
- **`main`**: `quick`, `test-integration`, `test-e2e`, and `vuln-scan`, run after each merge by
  [main-test](../../.github/workflows/main-test.yml).
- **`scheduled`** and **`manual`**: no registry gates. The nightly loaded-host run is a workflow.

`npm ci` installs the hooks through `prepare`. Unrun hooks mean unverified pushes:
[integration path](../conventions/integration-path.md).

## Targets

- `npm run test:quick`: `go vet`, a compile of every package, strict lint, NilAway, unit tests with `-race`, the three
  static behaviour adapters in turn, the 99% coverage check, and `test:behaviour:coverage` last.
- `npm run test:integration`: the integration suites with `-race`, and the integration behaviour suite.
- `npm run test:e2e`: the compiled end-to-end suites, run by `tests/e2e/run.sh`.
- `npm run test:behaviour:coverage`: the static behaviour-coverage check. It runs no test.

Every package with tests runs in one target or registry entry; a compile-only `-run '^$'` does not count. Cheap failures
run before coverage. The BDD suite host is described in
[behaviour-driven development](behaviour-driven-development.md#suite-host). Under load, raise Go's ten-minute
per-package timeout locally with `GOFLAGS=-timeout=30m`.

## In CI

The pull-request gate runs `quick` and `test-integration` in their own jobs, the other pull-request entries in one job,
and the release build. The pinned RHINO bootstrap runs only when the consumer boundary changes. The `main` surface runs
after merge on Linux and macOS. A failure on `main` is fixed forward or reverted before any release cut. Job detail:
[CI placement](quality-gates/001-ci-placement.md).

## The Test and Gate Contract

Adopter status for each module, as [Conformance Record](quality/checks/test-and-gate-contract/007-conformance-record.md)
requires. "Open" names what the repository does not meet yet.

- **001 Registry and Dispatch (C1 to C5): adapted, open.** `repo-config.yml` is the one registry, and
  `gate wiring validate` (W01 to W07) checks it. Open: no check fails when `npm ci` has not installed the hooks (C3),
  and no run is made with the network denied (C5).
- **002 Surface Placement (C6, C8 to C11): adapted.** The local lists are `pre-commit: format-staged`,
  `commit-msg: commit-message`, and `pre-push: public-safety-range`. The pull-request gate replays those checks and
  holds the rest.
- **003 Pull-Request Scope (C7): adapted.** The repository is one Go module, so every check runs at global scope. A slow
  check is split into parallel jobs, never narrowed.
- **004 Test Levels and Targets (C12, C14, C17, C18, C21): adapted, open.** The four targets exist. Open: unit tests are
  not yet mechanically hermetic (C21).
- **005 Behaviour Corpus (C13): adapted, open.** The corpus is `specs/behaviours/`, and `test:behaviour:coverage` checks
  that each scenario is bound or exempt. Target: every scenario bound at the unit, integration, and e2e levels.
- **006 Coverage Floor (C15): adapted, open.** Target: 99% over `cmd`, `internal`, and `scripts/release-archive`, read
  from the unit run, with no exclusion outside a named category. Today `test:quick` enforces 99% over a named list of
  packages. Closed by this unit's coverage items.
- **007 Conformance Record (C16, C19, C20, C22 to C24): adapted.** The shell-lint gate runs ShellCheck at warning
  severity over the list in `scripts/shell-files.sh`, and the POSIX `sh` deviation is recorded in the
  [repository adapter](quality/stacks/repository-adapter.md). The [gate inventory](quality-gates/002-gate-inventory.md)
  is the C19 record. The required check and the ruleset (C22) are targets, below.

## Landing and Ruleset

- **Landing gate (C9).** Before a pull request opens, the pull-request surface runs over the final head:
  `./rhino gate run --surface pull-request --base <sha> --head <sha>`. The result is recorded with the task.
- **Ruleset (C22), a target not yet applied.** The `main` ruleset requires the `Quality gate` check and refuses
  non-fast-forward updates.

## Never Nx

Never adopt Nx. The targets invoke shell scripts and the registry directly. Repair failure causes:
[push hook verification](../conventions/push-hook-verification.md).
