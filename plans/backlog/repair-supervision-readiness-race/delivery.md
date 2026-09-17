# Delivery: Repair Supervision Readiness Race

> **Legend** — `[AI]`: an agent performs the step. `[HUMAN]`: only a human can perform it because of unavailable
> credentials, physical action, external authority, or an unresolved decision. Split mixed work into separate items.

## Execution Checkout

Repository path: `~/ose-projects/hippo/`

Worktree path: `~/ose-projects/hippo/worktrees/repair-supervision-readiness-race/`

Delivery mode: `worktree-to-pr`

Branch: `worktree/repair-supervision-readiness-race`

Verify and provision from the primary checkout:

```bash
cd ~/ose-projects/hippo
test "$(git branch --show-current)" = main
test -z "$(git status --porcelain)"
git fetch origin --prune
git merge --ff-only origin/main
git worktree add worktrees/repair-supervision-readiness-race \
  -b worktree/repair-supervision-readiness-race origin/main
cd worktrees/repair-supervision-readiness-race
npm ci
```

HIPPO cannot guard its own repository commands. Run one heavy command at a time directly; do not wrap them in `./hippo`.

## Delivery Unit

One branch and pull request deliver one test-only outcome: deterministic child readiness precedes the injected
collector failure. Rollback reverts `tests/support/driver.go`, `tests/support/driver_test.go`, and the plan record
together.

The worktree-to-PR workflow owns commit, push, draft PR, exact-head quality, leak review, rebase merge, main
reconciliation, and cleanup. Fix every gate failure at its cause, including a pre-existing failure encountered in
scope; never bypass a hook.

## Phase 0: Environment Setup and Baseline

- [ ] `[AI]` Provision the declared worktree with the exact `git worktree add` command above; acceptance: it is
      registered once on `worktree/repair-supervision-readiness-race` at `origin/main`. `[AC-03]`
- [ ] `[AI]` In the worktree, run `npm ci`; acceptance: dependencies install, hooks activate, and
      `git status --porcelain` is empty. `[AC-03]`
- [ ] `[AI]` Run `npm run test:quick`; acceptance: the baseline pre-push gate exits `0` without a retry. `[AC-03]`
- [ ] `[AI]` Move `plans/backlog/repair-supervision-readiness-race/` to
      `plans/in-progress/repair-supervision-readiness-race/` with `git mv`; acceptance: one in-progress copy exists and
      no backlog copy exists. `[AC-03]`
- [ ] `[AI]` Update `plans/backlog/README.md` and `plans/in-progress/README.md`; acceptance: only the in-progress index
      links the active plan. `[AC-03]`
- [ ] `[AI]` Run `./rhino plan validate`; acceptance: plan validation exits `0`. `[AC-03]`

### Phase 0 Gate

- [ ] `[AI]` Run `git status --short` and record the baseline plus plan activation paths in this file; acceptance: no
      unowned path is present. `[AC-03]`

> **Pause Safety**: one clean worktree exists and the active plan validates. Safe to stop. To resume:
> `./rhino plan validate`.

## Phase 1: Deterministic Readiness Cycle

- [ ] `[AI]` **RED**: edit only the child command in `tests/support/driver.go::loseHostEvidence` so it delays briefly
      by using `trap '' TERM; sleep 0.05; printf '%s' "$$" > "$GUARD_CHILD_PID"; while :; do sleep 1; done` before
      writing `child.pid`; run `go test -count=1 -run '^TestUnitBehaviours$' ./tests/unit`; acceptance: the existing
      supervision-failure scenario fails with `read guarded child PID` before its cleanup assertion. `[AC-01]`
- [ ] `[AI]` Record the exact failing command and diagnostic under this item; acceptance: the failure proves the
      readiness race without changing production code or the Gherkin corpus. `[AC-01]` `[AC-03]`
- [ ] `[AI]` **RED**: add `tests/support/driver_test.go` in package `support` to require that a failing
      `beforeFailure` hook waits on a missing marker through `awaitMarkerFile` with a short test bound, then returns a
      readiness-specific error that wins over the injected collector error; name the tests
      `TestSequenceCollectorRunsBeforeFailureHook` and `TestSequenceCollectorReturnsReadinessErrorWithinBound`, then
      run `go test -count=1 ./tests/support`; acceptance: both tests fail before the optional hook seam exists.
      `[AC-04]`
- [ ] `[AI]` **GREEN**: add the optional `beforeFailure func() error` seam to `sequenceCollector`, call it immediately
      before the injected error, and configure `loseHostEvidence` to use
      `awaitMarkerFile(pidPath, interruptReadinessWait)` with the exact timeout error from `tech-docs.md`; run
      `go test -count=1 -run '^TestUnitBehaviours$' ./tests/unit`, then `go test -count=1 ./tests/support`; acceptance:
      AC-01, AC-02, and AC-04 pass with the readiness error taking precedence on timeout. `[AC-01]` `[AC-02]` `[AC-04]`
- [ ] `[AI]` **REFACTOR**: keep the hook private and reuse `awaitMarkerFile` without editing
      `tests/support/review_v04.go`; run `gofmt -w tests/support/driver.go tests/support/driver_test.go`, then
      `git diff --check`; acceptance: one bounded polling implementation owns this path and the diff check exits `0`.
      `[AC-01]` `[AC-02]` `[AC-04]`
- [ ] `[AI]` Run `go test -count=20 -run '^TestUnitBehaviours$' ./tests/unit`; acceptance: all twenty executions pass
      without retry and the delayed PID publication remains enabled. `[AC-01]` `[AC-02]` `[AC-03]`

### Phase 1 Gate

- [ ] `[AI]` Run `git diff --name-only` and `git diff --check`; acceptance: the product and specification trees are
      unchanged, only declared plan/index plus `tests/support/driver.go` and `tests/support/driver_test.go` differ, the
      focused commands pass, and the diff check exits `0`. `[AC-03]` `[AC-04]`

> **Pause Safety**: the deterministic race passes twenty times and the diff is test-only. Safe to stop. To resume:
> `go test -count=20 -run '^TestUnitBehaviours$' ./tests/unit`.

## Phase 2: Repository Gates and Delivery

- [ ] `[AI]` Run `npm run test:quick`; acceptance: formatting, compilation, lint, unit tests, 99% deterministic-core
      coverage, three BDD compliance adapters, and artifact checks pass. `[AC-03]`
- [ ] `[AI]` Run `npm test`; acceptance: the quick gate, integration tests, compiled end-to-end behavior, race detector,
      and vulnerability scan all exit `0`. `[AC-02]` `[AC-03]`
- [ ] `[AI]` Inspect the complete diff and proposed commit/PR text against public-repository data safety; acceptance:
      no credential, private identifier, machine path, or private infrastructure value is present. `[AC-03]`
- [ ] `[AI]` Commit the declared paths with a Conventional Commit; acceptance: commit hooks pass without bypass and the
      commit contains only plan/index plus `tests/support/driver.go` and `tests/support/driver_test.go`. `[AC-03]`
      `[AC-04]`
- [ ] `[AI]` Push `worktree/repair-supervision-readiness-race`; acceptance: the pre-push hook passes and the remote head
      equals the local commit. `[AC-03]`
- [ ] `[AI]` Open a draft PR with a public-safety-screened title and body; acceptance: it targets `main` and records the
      RED, twenty-run stress, and gate evidence. `[AC-03]`
- [ ] `[AI]` Mark the PR ready only after the diff is final; acceptance: the ready-for-review quality run starts on the
      exact head. `[AC-03]`
- [ ] `[AI]` Post one exact-head leak review after quality passes; acceptance: the review names the unchanged head and
      reports no finding. `[AC-03]`
- [ ] `[AI]` Rebase-merge after all five merge preconditions pass; acceptance: GitHub reports the PR merged. `[AC-03]`
- [ ] `[AI]` Reconcile primary `main` with `origin/main`; acceptance:
      `git rev-list --left-right --count main...origin/main` prints `0 0`. `[AC-03]`

### Phase 2 Gate

- [ ] `[AI]` Run `npm test` on reconciled main; acceptance: exit `0` without retry and the merged diff remains
      test-only. `[AC-01]` `[AC-02]` `[AC-03]`

> **Pause Safety**: the repair is merged, main is green, and no delivery branch carries unique work. Safe to stop. To
> resume: `npm test`.

## Phase 3: Execution Review

- [ ] `[AI]` Verify AC-01 through AC-04 against the recorded RED, twenty-run stress result, bounded-error unit proof,
      complete gate, and merged
      tree; acceptance: every criterion has terminal evidence. `[AC-03]`
- [ ] `[AI]` Run `repo-governance/workflows/plan-execution-check.md` against Phases 0–3 and record its verdict here;
      acceptance: it reports `PASS` before knowledge capture begins. `[AC-03]`

### Phase 3 Gate

- [ ] `[AI]` Confirm no production, specification, public-documentation, dependency, or configuration path changed;
      acceptance: the execution review permits knowledge capture. `[AC-03]`

> **Pause Safety**: substantive work is terminal and reviewed. Safe to stop. To resume: `git show --stat --oneline
origin/main`.

## Phase 4: Knowledge Capture

- [ ] `[AI]` Apply the generalizability, secret/sensitivity, and repository-relevance gates to every `learnings.md`
      entry; acceptance: each entry has one safe owner or a discard reason. `[AC-03]`
- [ ] `[AI]` Route each surviving entry to exactly one durable owner, filing code/test follow-up work as a separate
      backlog plan rather than editing it inline; acceptance: `learnings.md` records every terminal destination.
      `[AC-03]`
- [ ] `[AI]` If execution produced no generalizable entry, write
      `No generalizable learnings — the repaired race was fully captured by the permanent fixture.`; acceptance: the
      running log has an explicit terminal state. `[AC-03]`

### Phase 4 Gate

- [ ] `[AI]` Read every `learnings.md` entry and verify it is promoted, filed, or discarded with reason; acceptance: no
      unresolved entry remains. `[AC-03]`

> **Pause Safety**: every learning is terminal and main remains green. Safe to stop. To resume:
> `rg -n '^## Learning|Status|No generalizable' plans/in-progress/repair-supervision-readiness-race/learnings.md`.

## Plan Archival

- [ ] `[AI]` Verify every substantive item is complete and the execution-check verdict is `PASS`. `[AC-03]`
- [ ] `[AI]` Move the plan to `plans/done/YYYY-MM-DD__repair-supervision-readiness-race/` with the actual completion
      date; acceptance: exactly one done copy exists and no in-progress copy remains. `[AC-03]`
- [ ] `[AI]` Update `plans/in-progress/README.md`, `plans/done/README.md`, and every live reference; acceptance: no live
      link names the former in-progress path. `[AC-03]`
- [ ] `[AI]` Run `npm run test:quick`; acceptance: exit `0` from the archived state. `[AC-03]`
- [ ] `[AI]` Run `./rhino plan validate`; acceptance: exit `0` from the archived state. `[AC-03]`
- [ ] `[AI]` Run `git diff --check`; acceptance: exit `0` from the archived state. `[AC-03]`
- [ ] `[AI]` Commit the archival transaction using a Conventional Commit; acceptance: hooks pass and only archive/index
      paths are present. `[AC-03]`
- [ ] `[AI]` Push the archival branch; acceptance: the pre-push hook exits `0` and the remote head equals the local
      commit. `[AC-03]`
- [ ] `[AI]` Open the archival PR as a draft; acceptance: it targets `main` and records archived-state proof. `[AC-03]`
- [ ] `[AI]` Mark the archival PR ready only after the diff is final; acceptance: exact-head quality starts. `[AC-03]`
- [ ] `[AI]` Post one exact-head leak review for the archival PR; acceptance: it names the unchanged head and reports no
      finding. `[AC-03]`
- [ ] `[AI]` Rebase-merge the archival PR after all five preconditions pass; acceptance: GitHub reports it merged.
      `[AC-03]`
- [ ] `[AI]` Run dev artifact clean-up from the primary checkout; acceptance: the plan worktree and task branch are
      absent, `main` equals `origin/main`, and no untracked secret or unrelated artifact was removed. `[AC-03]`
