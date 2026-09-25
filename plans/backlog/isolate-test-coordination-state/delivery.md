# Delivery: Isolate Test Coordination State

> **Legend** — `[AI]`: an agent performs the step. `[HUMAN]`: only a human can perform it because of unavailable
> credentials, physical action, external authority, or an unresolved decision. Split mixed work into separate items.

## Execution Checkout

Repository path: the primary checkout of this repository.

Worktree path: `worktrees/isolate-test-coordination-state/` below it.

Delivery mode: `worktree-to-pr`

Branch: `worktree/isolate-test-coordination-state`

```bash
test "$(git branch --show-current)" = main
test -z "$(git status --porcelain)"
git fetch origin --prune
git merge --ff-only origin/main
git worktree add worktrees/isolate-test-coordination-state \
  -b worktree/isolate-test-coordination-state origin/main
cd worktrees/isolate-test-coordination-state
npm ci
```

The repository gates run directly, never beneath `./hippo`. Only the AC-04 reproduction runs beneath it, because the
defect is what that wrapper exports.

## Delivery Unit

One branch and pull request deliver one test-only outcome: every test package that starts the product owns its
coordination state. Rollback reverts `tests/support/isolation.go`, `tests/support/isolation_test.go`, the four
`main_test.go` files, and the plan record together.

The worktree-to-PR workflow owns commit, push, draft, exact-head quality, leak review, rebase merge, main
reconciliation, and cleanup. Fix every gate failure at its cause; never bypass a hook.

## Phase 0: Environment Setup and Baseline

- [ ] `[AI]` Provision the declared worktree with the commands above; acceptance: it is registered once at
      `origin/main` and `git status --porcelain` is empty. `[AC-05]`
- [ ] `[AI]` Run `npm run test:quick`; acceptance: the baseline exits `0` without retry. `[AC-05]`
- [ ] `[AI]` Move the plan to `plans/in-progress/isolate-test-coordination-state/` with `git mv` and update both stage
      indexes; acceptance: one in-progress copy exists and only the in-progress index links it. `[AC-05]`
- [ ] `[AI]` Check the plan against every rule in `repo-governance/conventions/plans/006-structural-validation.md` and run `./rhino md internal-link validate`; acceptance: no rule fails and the link check exits `0`. `[AC-05]`

### Phase 0 Gate

- [ ] `[AI]` Run `git status --short` and record the result here; acceptance: only plan-activation paths differ.
      `[AC-05]`

> **Pause Safety**: a clean worktree holds the active plan. Safe to stop. To resume: `./rhino md internal-link validate`.

## Phase 1: Isolation Cycle

- [ ] `[AI]` **RED**: run `./hippo run --class ephemeral --resource-tier standard --disk-path . -- ./tests/e2e/run.sh`;
      acceptance: it fails as the README table records, and the failing scenarios and assertion identifiers are
      recorded under this item. `[AC-04]`
- [ ] `[AI]` **RED**: add `tests/support/isolation_test.go` with `TestRunIsolatedRemovesInheritedCoordination`,
      `TestRunIsolatedKeepsHarnessInputs`, and `TestRunIsolatedLeavesMarkedHelpersUnchanged`, driving the helper
      through a re-executed test binary; run `go test -count=1 ./tests/support`; acceptance: the package fails to
      compile or the three tests fail because `RunIsolated` does not exist. `[AC-01]` `[AC-02]` `[AC-03]`
- [ ] `[AI]` **GREEN**: add `tests/support/isolation.go` as `tech-docs.md` specifies; run
      `go test -count=1 ./tests/support`; acceptance: the three tests pass. `[AC-01]` `[AC-02]` `[AC-03]`
- [ ] `[AI]` **GREEN**: add `main_test.go` with `os.Exit(support.RunIsolated(m))` to `tests/unit`, `tests/integration`,
      `tests/bdd`, and `tests/e2e`; re-run the Phase 1 RED reproduction; acceptance: it exits `0` with every `HIPPO_*`
      variable still exported by the wrapper. `[AC-04]`
- [ ] `[AI]` **REFACTOR**: keep the allowlist and marker in one place, run
      `go tool golangci-lint fmt --diff` and `git diff --check`; acceptance: both exit `0` and the four `TestMain`
      bodies are one line each. `[AC-01]` `[AC-05]`

### Phase 1 Gate

- [ ] `[AI]` Run `git diff --name-only origin/main`; acceptance: only the paths in the file-impact analysis differ.
      `[AC-05]`

> **Pause Safety**: the guarded reproduction passes. Safe to stop. To resume: `go test -count=1 ./tests/support`.

## Phase 2: Repository Gates and Delivery

- [ ] `[AI]` Run docs propagation for the change and record whether `end-to-end-testing.md` gains its sentence;
      acceptance: the decision and any edit are recorded here. `[AC-05]`
- [ ] `[AI]` Run `npm run test:quick`; acceptance: exit `0`. `[AC-05]`
- [ ] `[AI]` Run `npm test`; acceptance: exit `0` without retry. `[AC-04]` `[AC-05]`
- [ ] `[AI]` Inspect the diff and the proposed commit and PR text against data safety, then commit with a Conventional
      Commit; acceptance: hooks pass and the commit holds only declared paths. `[AC-05]`
- [ ] `[AI]` Push, open the pull request as a draft with a screened body recording both REDs and both GREENs, then mark
      it ready; acceptance: the ready-for-review run starts on the exact head. `[AC-05]`
- [ ] `[AI]` Post one exact-head leak review and rebase-merge once every merge precondition holds; acceptance: GitHub
      reports the pull request merged. `[AC-05]`
- [ ] `[AI]` Reconcile primary `main`; acceptance: `git rev-list --left-right --count main...origin/main` prints
      `0 0`. `[AC-05]`

### Phase 2 Gate

- [ ] `[AI]` Run `npm test` on reconciled `main`; acceptance: exit `0`. `[AC-04]` `[AC-05]`

> **Pause Safety**: the change is merged and `main` is green. Safe to stop. To resume: `npm test`.

## Phase 3: Execution Review and Knowledge Capture

- [ ] `[AI]` Run `repo-governance/workflows/plan-execution-check.md` against AC-01 to AC-05 and record its verdict;
      acceptance: `PASS`. `[AC-05]`
- [ ] `[AI]` Route every `learnings.md` entry to one durable owner or discard it with a reason, filing code follow-ups
      as backlog plans; acceptance: no unresolved entry remains. `[AC-05]`

### Phase 3 Gate

- [ ] `[AI]` Confirm no product, specification, or public-contract path changed; acceptance: the review permits
      archival. `[AC-05]`

> **Pause Safety**: substantive work is terminal. Safe to stop. To resume: `git show --stat --oneline origin/main`.

## Plan Archival

- [ ] `[AI]` Move the plan to `plans/done/YYYY-MM-DD__isolate-test-coordination-state/` with the completion date and
      update both stage indexes; acceptance: one done copy exists. `[AC-05]`
- [ ] `[AI]` Re-check the archived plan against `006-structural-validation.md` and run `npm run test:quick`; acceptance: no rule fails and the gate exits `0`. `[AC-05]`
- [ ] `[AI]` Deliver the archival commit through its own pull request under the same merge preconditions; acceptance:
      GitHub reports it merged. `[AC-05]`
- [ ] `[AI]` Run dev artifact clean-up; acceptance: the worktree and both branch copies are absent and `main` equals
      `origin/main`. `[AC-05]`
