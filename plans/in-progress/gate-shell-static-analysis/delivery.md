# Delivery: Gate Shell Static Analysis

> **Legend** — `[AI]`: an agent performs the step. `[HUMAN]`: only a human can perform it because of unavailable
> credentials, physical action, external authority, or an unresolved decision. Split mixed work into separate items.

## Execution Checkout

Repository path: the primary checkout of this repository.

Worktree path: `worktrees/gate-shell-static-analysis/` below it.

Delivery mode: `worktree-to-pr`

Branch: `worktree/gate-shell-static-analysis`

```bash
test "$(git branch --show-current)" = main
test -z "$(git status --porcelain)"
git fetch origin --prune
git merge --ff-only origin/main
git worktree add worktrees/gate-shell-static-analysis \
  -b worktree/gate-shell-static-analysis origin/main
cd worktrees/gate-shell-static-analysis
npm ci
```

The repository gates run directly, never beneath `./hippo`.

## Delivery Unit

One branch and pull request deliver one outcome: shell static analysis is a gate. Clean and gate land together,
because a gate turned on before the clean-up fails every push, and a clean-up without the gate decays. Rollback reverts
the delivery commits together.

The archival commit rides in the same pull request as the delivery, by the owner's decision of 2026-09-26, so the
branch that merges carries the finished record. The worktree-to-PR workflow owns push, draft, exact-head quality, leak
review, rebase merge, main reconciliation, and the worktree and branch clean-up; their evidence is the pull request
itself — its `Quality gate` check, its leak-review record, and its merge commit — because every one of them post-dates
the archived copy of this checklist. Fix every gate failure at its cause; never bypass a hook.

## Phase 0: Environment Setup and Baseline

- [x] `[AI]` Provision the declared worktree with the commands above; acceptance: it is registered once at
      `origin/main` and `git status --porcelain` is empty. `[AC-05]`
  - Result (2026-09-26): registered once at `837ad5f` on `worktree/gate-shell-static-analysis`; `npm ci` installed
    the hooks and the status was empty.
- [x] `[AI]` Run `npm run test:quick`; acceptance: the baseline exits `0`. `[AC-02]`
  - Result: exit `0` at `837ad5f`, with the selected core coverage at 99.07%.
- [x] `[AI]` Move the plan to `plans/in-progress/gate-shell-static-analysis/` with `git mv` and update both stage
      indexes; acceptance: only the in-progress index links it. `[AC-05]`
  - Result: moved after the quality-gate repair commit; the backlog index no longer names it.
- [x] `[AI]` Confirm the current ShellCheck release, its per-platform asset names, and their SHA-256 digests from the
      upstream release page, and record them here; acceptance: four platforms are recorded, or the missing one is
      named and the pin refuses it. `[AC-01]`
  - Result: `v0.11.0` (published 2025-08-04) is still the latest release. The four `.tar.gz` assets and their SHA-256
    digests, each recomputed locally from a download and equal to the digest the release publishes:

    | Key              | Asset                                      | SHA-256                                                            |
    | ---------------- | ------------------------------------------ | ------------------------------------------------------------------ |
    | `darwin.aarch64` | `shellcheck-v0.11.0.darwin.aarch64.tar.gz` | `339b930feb1ea764467013cc1f72d09cd6b869ebf1013296ba9055ab2ffbd26f` |
    | `darwin.x86_64`  | `shellcheck-v0.11.0.darwin.x86_64.tar.gz`  | `c2c15e08df0e8fbc374c335b230a7ee958c313fa5714817a59aa59f1aa594f51` |
    | `linux.aarch64`  | `shellcheck-v0.11.0.linux.aarch64.tar.gz`  | `68a8133197a50beb8803f8d42f9908d1af1c5540d4bb05fdfca8c1fa47decefc` |
    | `linux.x86_64`   | `shellcheck-v0.11.0.linux.x86_64.tar.gz`   | `b7af85e41cc99489dcc21d66c6d5f3685138f06d34651e6d34b42ec6d54fe6f6` |

    Each archive carries the executable at `shellcheck-v0.11.0/shellcheck`. Running it at `--severity=warning` over the
    enumerated list at `837ad5f` reproduced the README's 14 findings exactly.

### Phase 0 Gate

- [x] `[AI]` Check the plan against every rule in `repo-governance/conventions/plans/006-structural-validation.md` and run `./rhino md internal-link validate`; acceptance: no rule fails and the link check exits `0`. `[AC-05]`
  - Result: no structural rule fails in the in-progress location; the link check and the directory-map check report no
    findings.

> **Pause Safety**: a clean worktree holds the active plan and recorded digests. Safe to stop. To resume:
> `./rhino md internal-link validate`.

## Phase 1: Pinned Analyser

- [x] `[AI]` **RED**: add `tests/artifacts/shellcheck-pin.sh` asserting that `scripts/shellcheck.sh` exits `125`
      against a pin whose digest does not match and against a platform the pin omits, and call it from
      `tests/artifacts/run.sh`; run `./tests/artifacts/run.sh`; acceptance: it fails because the wrapper does not
      exist. `[AC-01]`
  - Result: exit `1`, from the test's own setup:
    `cp: …/scripts/shellcheck.sh: No such file or directory`. The test copies the wrapper into a scratch tree, plants a
    stand-in analyser that records being executed, pins every supported platform to a digest the cached archive lacks,
    and then pins a version with no platform at all.
- [x] `[AI]` **GREEN**: add `shellcheck.lock` and `scripts/shellcheck.sh` as `tech-docs.md` specifies; run
      `./tests/artifacts/run.sh` and `scripts/shellcheck.sh --version`; acceptance: the refusal cases pass and the
      version matches the pin. `[AC-01]`
  - Result: `./tests/artifacts/run.sh` exits `0`, both refusals exiting `125` with the stand-in never run; the first
    `scripts/shellcheck.sh --version` fetched, verified, and reported `version: 0.11.0`, and a cached run takes about
    0.2 s.
- [x] `[AI]` **REFACTOR**: align the wrapper's comments and refusal messages with `ferret`; run
      `go tool shfmt -d scripts/shellcheck.sh tests/artifacts/shellcheck-pin.sh`; acceptance: no diff. `[AC-01]`
  - Result: the header, the `[shellcheck]` refusal prefix, and the lock parser already follow `ferret` and `rhino`, so
    no edit was needed; `shfmt -d` prints nothing, and the pinned ShellCheck reports nothing on either file at any
    severity.

### Phase 1 Gate

- [x] `[AI]` Run `./tests/artifacts/run.sh`; acceptance: exit `0`. `[AC-01]`
  - Result: exit `0`.

> **Pause Safety**: the pin verifies. Safe to stop. To resume: `./tests/artifacts/run.sh`.

## Phase 2: Clean, Then Gate

- [x] `[AI]` **RED**: add `scripts/shell-files.sh` and `scripts/shell-lint.sh`; run `scripts/shell-lint.sh`;
      acceptance: it exits non-zero with the 14 findings the README table records. `[AC-02]` `[AC-04]`
  - Result: exit `1` with exactly 14 lines — SC2148 on the three hooks, SC1007 at `hippo:4`,
    `scripts/build-release.sh:18,55`, and line 4 of `check-worktree-layout.sh`, `format-check.sh`, `test-loaded.sh`,
    `test-quick.sh`, `test.sh`, `tests/artifacts/run.sh`, and `tests/e2e/run.sh`, and
    `hippo:107:14: warning: Use "${var:?}" to ensure this never expands to / . [SC2115]`. The list names 39 files once
    the new scripts are tracked.
- [x] `[AI]` **GREEN**: apply the SC1007, SC2115, and SC2148 fixes from `tech-docs.md`; run `scripts/shell-lint.sh`;
      acceptance: exit `0`. `[AC-02]`
  - Result: exit `0` with no output. `CDPATH= cd` became `CDPATH='' cd` in ten places, `hippo:107` guards its prefix
    with `${platform_cache:?}`, and each hook starts with `# shellcheck shell=sh`.
- [x] `[AI]` **REFACTOR**: point `scripts/format-check.sh` at `scripts/shell-files.sh`; run
      `./scripts/format-check.sh`; acceptance: exit `0`, and `ferret` is now among the formatter's inputs. `[AC-04]`
  - Result: exit `0`. Both scripts read the list into positional parameters the same way, so neither word-splits
    it. A deliberate mis-indent in `ferret` then made `./scripts/format-check.sh` exit `1` with a `shfmt` diff for
    `ferret`, which the old directory arguments never covered; restoring it cleared the diff.
- [x] `[AI]` **RED** (mutation): revert `hippo:107` to the unguarded form, run `scripts/shell-lint.sh`, record the
      SC2115 output here, then restore the fix; acceptance: the mutation fails naming `hippo` and SC2115, and the
      restored tree exits `0`. `[AC-03]`
  - Result: the unguarded line made `scripts/shell-lint.sh` exit `1` with
    `hippo:107:14: warning: Use "${var:?}" to ensure this never expands to / . [SC2115]`; the restored line exits `0`.
- [x] `[AI]` Add the `shell-lint` entry to `repo-config.yml` on `pre-push`, `pull-request`, and `main`; run
      `./rhino repo-config validate` and `./rhino gate run --surface pre-push`; acceptance: both exit `0` and the
      pre-push run lists `shell-lint`. `[AC-05]`
  - Result: `repo-config validate` and `gate validate` report no findings; `gate list` places `shell-lint` on
    `pre-push`, `pull-request`, and `main`. The pre-push surface, fed one simulated push update on standard input as
    the hook does, passed all nine gates, `shell-lint` second.

### Phase 2 Gate

- [x] `[AI]` Run `npm test`; acceptance: exit `0`, proving every fixed script still behaves. `[AC-02]` `[AC-05]`
  - Result: the first run exited `1` in four end-to-end scenarios with `hippo.args.invalid` "schema 3 requires
    --resource-tier", because the executing shell exported a workstation `HIPPO_CONFIG` into the binary under test —
    the leak the [isolate test coordination state](../../backlog/isolate-test-coordination-state/README.md) plan owns,
    unrelated to this change. With that one variable unset, `npm test` exited `0`: quick, integration, end-to-end,
    race, and `govulncheck` (0 reachable vulnerabilities).

> **Pause Safety**: the repository is clean and the gate is registered. Safe to stop. To resume:
> `scripts/shell-lint.sh`.

## Phase 3: Rules, Documentation, and Delivery

- [ ] `[AI]` Apply rules propagation to the adapter change, then update `repository-adapter.md` (shell row, interpreter
      and static-analysis rows, pins) and `quality-gates.md`; record the propagation result here; acceptance: no
      document still describes static analysis as a gap, and the interpreter row names every Bash script. `[AC-05]`
- [ ] `[AI]` Run docs propagation and record whether `CHANGELOG.md` needs an entry for the `hippo` byte change;
      acceptance: the decision is recorded. `[AC-05]`
- [ ] `[AI]` Run `npm run test:quick`; acceptance: exit `0`. `[AC-02]` `[AC-05]`
- [ ] `[AI]` Inspect the diff and the proposed commit and PR text against data safety, then commit thematically;
      acceptance: hooks pass, including the new `shell-lint` on push. `[AC-05]`
- [ ] `[AI]` Replay the pull-request surface the `repository-contract` job runs, with
      `./rhino gate run --surface pull-request --base origin/main --head HEAD`; acceptance: exit `0`, and the run lists
      `shell-lint`. `[AC-05]`

### Phase 3 Gate

- [ ] `[AI]` Run `repo-governance/workflows/plan-execution-check.md` against AC-01 to AC-05 and record its verdict;
      acceptance: `PASS`. `[AC-05]`

> **Pause Safety**: the gate is committed on the branch. Safe to stop. To resume: `scripts/shell-lint.sh`.

## Phase 4: Knowledge Capture

- [ ] `[AI]` Route every `learnings.md` entry to one durable owner or discard it with a reason; acceptance: no
      unresolved entry remains. `[AC-05]`

### Phase 4 Gate

- [ ] `[AI]` Read `learnings.md` end to end; acceptance: every entry is terminal. `[AC-05]`

> **Pause Safety**: every learning is terminal. Safe to stop.

## Plan Archival

- [ ] `[AI]` Move the plan to `plans/done/YYYY-MM-DD__gate-shell-static-analysis/` with the completion date and update
      both stage indexes; acceptance: one done copy exists. `[AC-05]`
- [ ] `[AI]` Re-check the archived plan against `006-structural-validation.md` and run `npm run test:quick`; acceptance: no rule fails and the gate exits `0`. `[AC-05]`
- [ ] `[AI]` Commit the archival move onto the delivery branch; acceptance: the commit is on the head the pull request
      will merge, and publication, merge, and clean-up proceed under the Delivery Unit above. `[AC-05]`
