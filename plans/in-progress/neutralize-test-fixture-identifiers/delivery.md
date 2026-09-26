# Delivery: Neutralize Test Fixture Identifiers

> **Legend** — `[AI]`: an agent performs the step. `[HUMAN]`: only a human can perform it because of unavailable
> credentials, physical action, external authority, or an unresolved decision. Split mixed work into separate items.

## Execution Checkout

Repository path: the primary checkout of this repository.

Worktree path: `worktrees/neutralize-test-fixture-identifiers/` below it.

Delivery mode: `worktree-to-pr`

Branch: `worktree/neutralize-test-fixture-identifiers`

```bash
test "$(git branch --show-current)" = main
test -z "$(git status --porcelain)"
git fetch origin --prune
git merge --ff-only origin/main
git worktree add worktrees/neutralize-test-fixture-identifiers \
  -b worktree/neutralize-test-fixture-identifiers origin/main
cd worktrees/neutralize-test-fixture-identifiers
npm ci
```

## Delivery Unit

One branch and pull request deliver one test-only outcome: the source-override fixture is synthetic. Rollback reverts
the test file and the plan record together. The worktree-to-PR workflow owns commit, push, draft, exact-head quality,
leak review, rebase merge, main reconciliation, and cleanup.

Owner decision, 2026-09-26: the plan is archived inside this delivery pull request rather than through a second one.
The merge, the primary reconciliation, and the clean-up follow the archival commit, so they cannot be ticked here; the
worktree-to-PR workflow performs them and the pull request records their proof. See the
[quality gate](evidence/quality-gate.md).

## Phase 0: Environment Setup and Baseline

- [x] `[AI]` Provision the declared worktree with the commands above; acceptance: it is registered once at
      `origin/main` and `git status --porcelain` is empty. `[AC-03]`
  - Result: registered once at `837ad5f`; `npm ci` ran beneath the worktree-local guard; the porcelain status was empty.
- [x] `[AI]` Move the plan to `plans/in-progress/neutralize-test-fixture-identifiers/` with `git mv` and update both
      stage indexes; acceptance: only the in-progress index links it. `[AC-03]`
  - Result: moved with `git mv`; the backlog index lost its entry and the in-progress index gained it.
- [x] `[AI]` Check the plan against every rule in `repo-governance/conventions/plans/006-structural-validation.md` and
      run `./rhino md internal-link validate`; acceptance: no rule fails and the link check exits `0`. `[AC-03]`
  - Result: 0 structural findings over the whole `plans/` tree; the link check exited `0`. The quality gate returned
    `PASS_WITH_FINDINGS`; see [the record](evidence/quality-gate.md).

### Phase 0 Gate

- [x] `[AI]` Read line 42 of `internal/identity/identity_test.go` and run `git grep -c` for its source string, keeping
      the string out of every recorded note; acceptance: exactly one file with two matches is reported. `[AC-02]`
  - Result: `internal/identity/identity_test.go:2`, case-sensitive and case-insensitive alike.

> **Pause Safety**: the active plan is in place. Safe to stop. To resume: `./rhino md internal-link validate`.

## Phase 1: Synthetic Fixture Cycle

- [x] `[AI]` **RED** (mutation): change only line 46's expected value to `fixture-source`, then run
      `go test -count=1 -run '^TestOverrideSourceWithoutFile$' ./internal/identity`; acceptance: it fails with
      `unexpected identity`, proving the assertion reads the value. `[AC-01]`
  - Result: exit `1` at `identity_test.go:47` with `unexpected identity`; the reported value carried the old source
    string and group `local`. Run beneath the worktree-local guard at the `light` tier.
- [x] `[AI]` **GREEN**: change line 42's override to `fixture-source` as well, then re-run the same command; acceptance:
      it passes. `[AC-01]`
  - Result: `ok github.com/wahidyankf/hippo/internal/identity`, exit `0`.
- [x] `[AI]` **REFACTOR**: run `go tool golangci-lint fmt --diff`, then `git diff --check`; acceptance: both exit `0`
      and only the two lines changed. `[AC-01]` `[AC-03]`
  - Result: both exited `0` with no output; `git diff --numstat` reports `2 2` for the test file alone.
- [x] `[AI]` Re-run the Phase 0 `git grep -c`; acceptance: no match in any tracked file. `[AC-02]`
  - Result: no output, exit `1`, case-insensitive included.

### Phase 1 Gate

- [x] `[AI]` Run `npm run test:quick`; acceptance: exit `0`. `[AC-03]`
  - Result: exit `0`, run directly; selected production line coverage 99.07%.

> **Pause Safety**: the fixture is synthetic and the quick gate passes. Safe to stop. To resume:
> `go test -count=1 ./internal/identity`.

## Phase 2: Delivery and Review

- [x] `[AI]` Run `npm test`; acceptance: exit `0`. `[AC-03]`
  - Result: exit `0`, run directly with `HIPPO_CONFIG` unset and `HIPPO_ROOT` an empty temporary directory;
    `govulncheck` reported no called vulnerability. Two earlier runs failed for reasons outside this diff, recorded as
    [L1 and L2](learnings.md).
- [x] `[AI]` Commit with a Conventional Commit whose message does not name the removed string, after inspecting the
      diff against data safety; acceptance: hooks pass and only declared paths are committed. `[AC-02]` `[AC-03]`
  - Result: `test(identity): prove the source override with a synthetic value` holds only the test file; the
    `public-safety-tree`, `format-staged`, `public-safety-message`, and `commit-message` hooks passed.
- [ ] `[AI]` Push and open the pull request as a draft with a screened body; acceptance: the draft exists at the
      pushed head. `[AC-03]`

### Phase 2 Gate

- [ ] `[AI]` Run `repo-governance/workflows/plan-execution-check.md` against AC-01 to AC-03 and route every
      `learnings.md` entry; acceptance: `PASS` and no unresolved entry. `[AC-03]`

> **Pause Safety**: the change is committed and the draft is open. Safe to stop. To resume:
> `git log --oneline origin/main..HEAD`.

## Plan Archival

- [ ] `[AI]` Move the plan to `plans/done/YYYY-MM-DD__neutralize-test-fixture-identifiers/` with the completion date
      and update both stage indexes; acceptance: one done copy exists. `[AC-03]`
- [ ] `[AI]` Re-check the archived plan against `006-structural-validation.md` and run `npm run test:quick`;
      acceptance: no rule fails and the gate exits `0`. `[AC-03]`
