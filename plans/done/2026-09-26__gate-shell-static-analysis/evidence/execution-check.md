# Execution Check: Gate Shell Static Analysis

Recorded under the [execution check workflow](../../../../repo-governance/workflows/plan-execution-check.md).

- Command: the six steps of that workflow, in order, read against the branch rather than the checklist.
- Commit: `92d00ea` plus the uncommitted Phase 3 and Phase 4 records that accompany this file.
- Time: 2026-09-26T01:45Z.

## 1. Scope

Matches. The diff from `837ad5f` touches the pin, its wrapper and refusal test, the shared list, the gate script, the
format check, the registry entry, the fourteen fixes, the adapter and quality-gate documents, and this plan. It leaves
the adopted `scripts/public-safety/` copies, the `shfmt` settings, and every note below the threshold alone, as the
plan excluded.

## 2. Requirements

| Criterion | Verdict | Evidence                                                                                                                                                              |
| --------- | ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| AC-01     | met     | `tests/artifacts/shellcheck-pin.sh` failed before the wrapper existed and passes now: a mismatched cached archive and an unpinned platform both exit `125` unexecuted |
| AC-02     | met     | `scripts/shell-lint.sh` exits `0` at warning severity over all 39 listed files                                                                                        |
| AC-03     | met     | the `hippo:107` mutation failed the gate naming `hippo` and SC2115; the restored line passes                                                                          |
| AC-04     | met     | `scripts/format-check.sh` and `scripts/shell-lint.sh` both read `scripts/shell-files.sh`; the `ferret` mis-indent now fails the format check                          |
| AC-05     | met     | the registry places `shell-lint` on all three surfaces, the local pull-request replay ran it, and the adapter records a gate                                          |

The CI half of AC-05 — the `repository-contract` job running `shell-lint` on the exact head — post-dates this record by
construction. The Delivery Unit assigns that evidence to the pull request's `Quality gate` check.

## 3. Checklist Evidence

Every ticked item carries a result, and each result matches the repository: the files exist with executable modes
where they run, `shellcheck.lock` holds the four digests Phase 0 recorded, and the backlog holds no copy of the plan.

## 4. Gates

Every declared gate ran to a terminal result: the Phase 0 baseline, `./tests/artifacts/run.sh`, the RED and GREEN
`shell-lint` runs, both mutations, the pre-push and pull-request surfaces, `npm test` (a first run failed on an
inherited workstation variable outside this change, recorded in `delivery.md`; the rerun passed), and
`npm run test:quick`.

## 5. Cleanup

No task-owned scratch remains in the tree: both mutations were restored and `git status` shows only these records,
nothing was written to `local-tmp/`, and the downloaded analyser sits in the ignored `.cache/`, which the worktree's
removal takes with it. The worktree and branches are removed after merge under the Delivery Unit.

## 6. Knowledge Capture

The one learning is terminal, discarded with its reason.

## Verdict

`PASS`. Archival may proceed.
