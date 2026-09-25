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

## Phase 0: Environment Setup and Baseline

- [ ] `[AI]` Provision the declared worktree with the commands above; acceptance: it is registered once at
      `origin/main` and `git status --porcelain` is empty. `[AC-03]`
- [ ] `[AI]` Move the plan to `plans/in-progress/neutralize-test-fixture-identifiers/` with `git mv` and update both
      stage indexes; acceptance: only the in-progress index links it. `[AC-03]`
- [ ] `[AI]` Check the plan against every rule in `repo-governance/conventions/plans/006-structural-validation.md` and
      run `./rhino md internal-link validate`; acceptance: no rule fails and the link check exits `0`. `[AC-03]`

### Phase 0 Gate

- [ ] `[AI]` Read line 42 of `internal/identity/identity_test.go` and run `git grep -c` for its source string, keeping
      the string out of every recorded note; acceptance: exactly one file with two matches is reported. `[AC-02]`

> **Pause Safety**: the active plan is in place. Safe to stop. To resume: `./rhino md internal-link validate`.

## Phase 1: Synthetic Fixture Cycle

- [ ] `[AI]` **RED** (mutation): change only line 46's expected value to `fixture-source`, then run
      `go test -count=1 -run '^TestOverrideSourceWithoutFile$' ./internal/identity`; acceptance: it fails with
      `unexpected identity`, proving the assertion reads the value. `[AC-01]`
- [ ] `[AI]` **GREEN**: change line 42's override to `fixture-source` as well, then re-run the same command; acceptance:
      it passes. `[AC-01]`
- [ ] `[AI]` **REFACTOR**: run `go tool golangci-lint fmt --diff`, then `git diff --check`; acceptance: both exit `0`
      and only the two lines changed. `[AC-01]` `[AC-03]`
- [ ] `[AI]` Re-run the Phase 0 `git grep -c`; acceptance: no match in any tracked file. `[AC-02]`

### Phase 1 Gate

- [ ] `[AI]` Run `npm run test:quick`; acceptance: exit `0`. `[AC-03]`

> **Pause Safety**: the fixture is synthetic and the quick gate passes. Safe to stop. To resume:
> `go test -count=1 ./internal/identity`.

## Phase 2: Delivery and Review

- [ ] `[AI]` Run `npm test`; acceptance: exit `0`. `[AC-03]`
- [ ] `[AI]` Commit with a Conventional Commit whose message does not name the removed string, after inspecting the
      diff against data safety; acceptance: hooks pass and only declared paths are committed. `[AC-02]` `[AC-03]`
- [ ] `[AI]` Push, open the pull request as a draft with a screened body, mark it ready, post one exact-head leak
      review, and rebase-merge once every precondition holds; acceptance: GitHub reports it merged. `[AC-03]`
- [ ] `[AI]` Reconcile primary `main`; acceptance: `git rev-list --left-right --count main...origin/main` prints
      `0 0`. `[AC-03]`

### Phase 2 Gate

- [ ] `[AI]` Run `repo-governance/workflows/plan-execution-check.md` against AC-01 to AC-03 and route every
      `learnings.md` entry; acceptance: `PASS` and no unresolved entry. `[AC-03]`

> **Pause Safety**: the change is merged. Safe to stop. To resume: `git show --stat --oneline origin/main`.

## Plan Archival

- [ ] `[AI]` Move the plan to `plans/done/YYYY-MM-DD__neutralize-test-fixture-identifiers/` with the completion date
      and update both stage indexes; acceptance: one done copy exists. `[AC-03]`
- [ ] `[AI]` Re-check the archived plan against `006-structural-validation.md` and run `npm run test:quick`;
      acceptance: no rule fails and the gate exits `0`. `[AC-03]`
- [ ] `[AI]` Deliver the archival commit through its own pull request under the same merge preconditions; acceptance:
      GitHub reports it merged. `[AC-03]`
- [ ] `[AI]` Run dev artifact clean-up; acceptance: the worktree and both branch copies are absent and `main` equals
      `origin/main`. `[AC-03]`
