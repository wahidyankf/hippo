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

### Execution Record

Executed on 2026-09-26 in the combined v0.8.2 flake-repair worktree `worktrees/deterministic-flaky-tests` (branch
`worktree/deterministic-flaky-tests`, from `main` at `acf6577`), with the owner's approval, rather than in the declared
worktree. That branch carries this repair beside other test-only flake repairs, each in its own commit, and one pull
request delivers them all. Its delivery run was told to run Go and gate commands through `./hippo`, which departs from
the rule above; see `learnings.md`. Push, pull-request, merge, reconciliation, and clean-up items belong to that
delivery run and are marked delegated below.

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
      **Not performed (2026-09-26):** executed in the combined worktree named in the execution record.
- [x] `[AI]` In the worktree, run `npm ci`; acceptance: dependencies install, hooks activate, and
      `git status --porcelain` is empty. `[AC-03]`
      **Result:** `npm ci` installed with 0 vulnerabilities; `git status --porcelain` was empty.
- [ ] `[AI]` Run `npm run test:quick`; acceptance: the baseline quick gate exits `0` without a retry. `[AC-03]`
      **Not performed (2026-09-26):** no separate baseline; the quick gate runs on the final branch head in Phase 2.
- [x] `[AI]` Move `plans/backlog/repair-supervision-readiness-race/` to
      `plans/in-progress/repair-supervision-readiness-race/` with `git mv`; acceptance: one in-progress copy exists and
      no backlog copy exists. `[AC-03]`
      **Result:** `git mv` moved the folder; one in-progress copy exists and no backlog copy.
- [x] `[AI]` Update `plans/backlog/README.md` and `plans/in-progress/README.md`; acceptance: only the in-progress index
      links the active plan. `[AC-03]`
      **Result:** only the in-progress index links the plan; the backlog index states it holds no plan.
- [x] `[AI]` Check the plan against every rule in `repo-governance/conventions/plans/006-structural-validation.md` and
      run `./rhino md internal-link validate`; acceptance: no rule fails and the link check exits `0`. `[AC-03]`
      **Result:** read against every rule, none fails; `./rhino md internal-link validate` reported `checked 770 links, no findings`.

### Phase 0 Gate

- [x] `[AI]` Run `git status --short` and record the baseline plus plan activation paths in this file; acceptance: no
      unowned path is present. `[AC-03]`
      **Result:** the plan rename and the two stage indexes, beside the branch's committed repairs; no unowned path.

> **Pause Safety**: one clean worktree exists and the active plan passes the structural rules. Safe to stop. To
> resume: `./rhino md internal-link validate`.

## Phase 1: Deterministic Readiness Cycle

- [x] `[AI]` **RED**: edit only the child command in `tests/support/driver.go::loseHostEvidence` so it delays briefly
      by using `trap '' TERM; sleep 0.05; printf '%s' "$$" > "$GUARD_CHILD_PID"; while :; do sleep 1; done` before
      writing `child.pid`; run `go test -count=1 -run '^TestUnitBehaviours$' ./tests/unit`; acceptance: the existing
      supervision-failure scenario fails with `read guarded child PID` before its cleanup assertion. `[AC-01]`
      **Result:** the planned `sleep 0.05` did not fail (5 of 5 passed): the child still wrote its PID inside the 50 ms termination grace after the first 20 ms supervision sample. `sleep 0.2` outlasts both and failed 5 of 5.
- [x] `[AI]` Record the exact failing command and diagnostic under this item; acceptance: the failure proves the
      readiness race without changing production code or the Gherkin corpus. `[AC-01]` `[AC-03]`
      **Result:** `go test -count=5 -v -run '^TestUnitBehaviours$/^A_supervision_failure_reaps_the_guarded_child_before_releasing_ownership$' ./tests/unit` failed 5 of 5 with `read guarded child PID: open <temp>/hippo-lease-<n>/child.pid: no such file or directory`; product code and the Gherkin corpus were unchanged.
- [x] `[AI]` **RED**: add `tests/support/driver_test.go` in package `support` to require that a failing
      `beforeFailure` hook waits on a missing marker through `awaitMarkerFile` with a short test bound, then returns a
      readiness-specific error that wins over the injected collector error; name the tests
      `TestSequenceCollectorRunsBeforeFailureHook` and `TestSequenceCollectorReturnsReadinessErrorWithinBound`, then
      run `go test -count=1 ./tests/support`; acceptance: both tests fail before the optional hook seam exists.
      `[AC-04]`
      **Result:** `go test -count=1 -run TestSequenceCollector ./tests/support` did not build: `unknown field beforeFailure in struct literal of type sequenceCollector`, `undefined: childPIDReadiness`, `undefined: errChildPIDNotReady`.
- [x] `[AI]` **GREEN**: add the optional `beforeFailure func() error` seam to `sequenceCollector`, call it immediately
      before the injected error, and configure `loseHostEvidence` to use
      `awaitMarkerFile(pidPath, interruptReadinessWait)` with the exact timeout error from `tech-docs.md`; run
      `go test -count=1 -run '^TestUnitBehaviours$' ./tests/unit`, then `go test -count=1 ./tests/support`; acceptance:
      AC-01, AC-02, and AC-04 pass with the readiness error taking precedence on timeout. `[AC-01]` `[AC-02]` `[AC-04]`
      **Result:** the private `beforeFailure` seam runs before the injected error, and `childPIDReadiness(pidPath, interruptReadinessWait)` wraps `awaitMarkerFile` and returns `errChildPIDNotReady` with the exact message from `tech-docs.md`. The focused scenario passed 5 of 5 and both support tests passed.
- [x] `[AI]` **REFACTOR**: keep the hook private and reuse `awaitMarkerFile` without editing
      `tests/support/review_v04.go`; run `gofmt -w tests/support/driver.go tests/support/driver_test.go`, then
      `git diff --check`; acceptance: one bounded polling implementation owns this path and the diff check exits `0`.
      `[AC-01]` `[AC-02]` `[AC-04]`
      **Result:** `gofmt` reported nothing, `git diff --check` exited `0`, and `review_v04.go` is unchanged. `driver_test.go` carries `//nolint:testpackage` because the seam is package-private.
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
- [ ] `[AI]` Re-check the archived plan against `repo-governance/conventions/plans/006-structural-validation.md`;
      acceptance: no rule fails in the archived state. `[AC-03]`
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
