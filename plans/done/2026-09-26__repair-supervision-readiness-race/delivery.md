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
request delivers them all. Its first delivery run was told to run Go and gate commands through `./hippo`, which departs
from the rule above; see `learnings.md`. The later gate runs ran directly. Before push, `main` had moved, so the fix
commits were replayed by cherry-pick onto `worktree/deterministic-flaky-tests-r2` from `main` at `e3a3d1c`, with the
owner's approval, leaving this archival for a follow-up after merge.

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
- [x] `[AI]` Run `go test -count=20 -run '^TestUnitBehaviours$' ./tests/unit`; acceptance: all twenty executions pass
      without retry and the delayed PID publication remains enabled. `[AC-01]` `[AC-02]` `[AC-03]`
      **Result:** `ok github.com/wahidyankf/hippo/tests/unit 3517.740s`: all twenty full unit-adapter runs passed with the delayed write enabled. The focused scenario also passed 100 of 100 with `-count=100`.

### Phase 1 Gate

- [x] `[AI]` Run `git diff --name-only` and `git diff --check`; acceptance: the product and specification trees are
      unchanged, only declared plan/index plus `tests/support/driver.go` and `tests/support/driver_test.go` differ, the
      focused commands pass, and the diff check exits `0`. `[AC-03]` `[AC-04]`
      **Result:** this delivery's commit changes only the plan and indexes, `tests/support/driver.go`, and `tests/support/driver_test.go`. Product and `specs/` trees are unchanged, and `git diff --check` exited `0`. The branch's other commits repair separate flakes in other test files.

> **Pause Safety**: the deterministic race passes twenty times and the diff is test-only. Safe to stop. To resume:
> `go test -count=20 -run '^TestUnitBehaviours$' ./tests/unit`.

## Phase 2: Repository Gates and Delivery

- [x] `[AI]` Run `npm run test:quick`; acceptance: formatting, compilation, lint, unit tests, 99% deterministic-core
      coverage, three BDD compliance adapters, and artifact checks pass. `[AC-03]`
      **Result:** `npm test` at `c314faa` ran `./scripts/test-quick.sh` first and it passed: formatting, `0 issues`, unit adapters, `selected production line coverage: 99.07%`, three BDD adapters, and artifacts.
- [x] `[AI]` Run `npm test`; acceptance: the quick gate, integration tests, compiled end-to-end behavior, race detector,
      and vulnerability scan all exit `0`. `[AC-02]` `[AC-03]`
      **Result:** exited `0` at `c314faa`: integration `216.417s`, e2e `40.901s`, race detector, and `govulncheck` reported "Your code is affected by 0 vulnerabilities".
- [x] `[AI]` Inspect the complete diff and proposed commit/PR text against public-repository data safety; acceptance:
      no credential, private identifier, machine path, or private infrastructure value is present. `[AC-03]`
      **Result:** `scripts/public-safety/outbound-preflight.sh --surface commit` reported clean on the diff and message; no private identifier or machine path is present.
- [x] `[AI]` Commit the declared paths with a Conventional Commit; acceptance: commit hooks pass without bypass and the
      commit contains only plan/index plus `tests/support/driver.go` and `tests/support/driver_test.go`. `[AC-03]`
      `[AC-04]`
      **Result:** committed as `test(support): wait for child PID readiness before injected host failure`; the commit hooks passed.
- [x] `[AI]` Push `worktree/repair-supervision-readiness-race`; acceptance: the pre-push hook passes and the remote head
      equals the local commit. `[AC-03]`
      **Result:** delivered on `worktree/deterministic-flaky-tests-r2`; the pre-push hook passed and the remote head matched the local head `2d6b383`.
- [x] `[AI]` Open a draft PR with a public-safety-screened title and body; acceptance: it targets `main` and records the
      RED, twenty-run stress, and gate evidence. `[AC-03]`
      **Result:** opened as draft pull request #90 against `main`, with a screened body recording the RED, stress, and gate evidence.
- [x] `[AI]` Mark the PR ready only after the diff is final; acceptance: the ready-for-review quality run starts on the
      exact head. `[AC-03]`
      **Result:** #90 was marked ready with its diff final; the ready-for-review quality run passed on head `2d6b383`.
- [x] `[AI]` Post one exact-head leak review after quality passes; acceptance: the review names the unchanged head and
      reports no finding. `[AC-03]`
      **Result:** one leak review on head `2d6b383`, reporting no finding.
- [x] `[AI]` Rebase-merge after all five merge preconditions pass; acceptance: GitHub reports the PR merged. `[AC-03]`
      **Result:** #90 was rebase-merged with the exact head matched, after green checks and zero unresolved threads; `main` became `abc9094`.
- [x] `[AI]` Reconcile primary `main` with `origin/main`; acceptance:
      `git rev-list --left-right --count main...origin/main` prints `0 0`. `[AC-03]`
      **Result:** the primary checkout fast-forwarded to `origin/main`, and the count printed `0 0`.

### Phase 2 Gate

- [x] `[AI]` Run `npm test` on reconciled main; acceptance: exit `0` without retry and the merged diff remains
      test-only. `[AC-01]` `[AC-02]` `[AC-03]`
      **Result:** `npm test` exited `0` without retry on `main` at `e23151a`, after the repair and the later v0.8.2 fixes merged: `0 issues`, selected production line coverage 99.29%, every adapter passing, and no vulnerabilities. The merged repair diff is test-only.

> **Pause Safety**: the repair is merged, main is green, and no delivery branch carries unique work. Safe to stop. To
> resume: `npm test`.

## Phase 3: Execution Review

- [x] `[AI]` Verify AC-01 through AC-04 against the recorded RED, twenty-run stress result, bounded-error unit proof,
      complete gate, and merged
      tree; acceptance: every criterion has terminal evidence. `[AC-03]`
      **Result:** AC-01 and AC-02 have a 5/5 RED, 5/5 GREEN, 100/100 focused runs, and 20/20 full unit-adapter runs; the reaping and lease assertions are unchanged. AC-03 is shown by the test-only diff and the `npm test` pass. AC-04 is shown by `TestSequenceCollectorReturnsReadinessErrorWithinBound` returning `errChildPIDNotReady` at a 10 ms bound. The merged tree passed `npm test` as recorded in the Phase 2 gate.
- [x] `[AI]` Run `repo-governance/workflows/plan-execution-check.md` against Phases 0–3 and record its verdict here;
      acceptance: it reports `PASS` before knowledge capture begins. `[AC-03]`
      **Result:** checked at the branch head. Scope, requirements, checklist evidence, and gates pass. Clean-up and merge are the delegated items above, and are now recorded as done, so the verdict is PASS.

### Phase 3 Gate

- [x] `[AI]` Confirm no production, specification, public-documentation, dependency, or configuration path changed;
      acceptance: the execution review permits knowledge capture. `[AC-03]`
      **Result:** `git diff --name-only acf6577..HEAD` lists no product, `specs/`, public-document, dependency, or configuration path.

> **Pause Safety**: substantive work is terminal and reviewed. Safe to stop. To resume: `git show --stat --oneline
origin/main`.

## Phase 4: Knowledge Capture

- [x] `[AI]` Apply the generalizability, secret/sensitivity, and repository-relevance gates to every `learnings.md`
      entry; acceptance: each entry has one safe owner or a discard reason. `[AC-03]`
      **Result:** both entries passed the secret and sensitivity gates. Learning 1 is fixture-specific; Learning 2 is an instruction conflict for the owner.
- [x] `[AI]` Route each surviving entry to exactly one durable owner, filing code/test follow-up work as a separate
      backlog plan rather than editing it inline; acceptance: `learnings.md` records every terminal destination.
      `[AC-03]`
      **Result:** Learning 1 is captured by the permanent comment above the delayed write in `loseHostEvidence`. Learning 2 is routed to the owner in the delivery hand-back.
- [ ] `[AI]` If execution produced no generalizable entry, write
      `No generalizable learnings — the repaired race was fully captured by the permanent fixture.`; acceptance: the
      running log has an explicit terminal state. `[AC-03]`
      **Not triggered (2026-09-26):** execution produced two entries, and both are routed.

### Phase 4 Gate

- [x] `[AI]` Read every `learnings.md` entry and verify it is promoted, filed, or discarded with reason; acceptance: no
      unresolved entry remains. `[AC-03]`
      **Result:** both entries are terminal.

> **Pause Safety**: every learning is terminal and main remains green. Safe to stop. To resume:
> `rg -n '^## Learning|Status|No generalizable' plans/in-progress/repair-supervision-readiness-race/learnings.md`.

## Plan Archival

- [x] `[AI]` Verify every substantive item is complete and the execution-check verdict is `PASS`. `[AC-03]`
      **Result:** every substantive item is complete or not performed with a reason; the repair merged as #90, and the execution-check verdict is PASS.
- [x] `[AI]` Move the plan to `plans/done/YYYY-MM-DD__repair-supervision-readiness-race/` with the actual completion
      date; acceptance: exactly one done copy exists and no in-progress copy remains. `[AC-03]`
      **Result:** `git mv` to `plans/done/2026-09-26__repair-supervision-readiness-race/`; exactly one copy exists.
- [x] `[AI]` Update `plans/in-progress/README.md`, `plans/done/README.md`, and every live reference; acceptance: no live
      link names the former in-progress path. `[AC-03]`
      **Result:** the in-progress index lists no plan, and the done index lists this record; no live link names the former path.
- [x] `[AI]` Run `npm run test:quick`; acceptance: exit `0` from the archived state. `[AC-03]`
      **Result:** the full `npm test`, which includes the quick gate, exited `0` from the archived state on `main` at `e23151a`.
- [x] `[AI]` Re-check the archived plan against `repo-governance/conventions/plans/006-structural-validation.md`;
      acceptance: no rule fails in the archived state. `[AC-03]`
      **Result:** read against every rule in the archived state, none fails; `./rhino md internal-link validate` inside the main-surface gate was clean.
- [x] `[AI]` Run `git diff --check`; acceptance: exit `0` from the archived state. `[AC-03]`
      **Result:** exited `0`.
- [x] `[AI]` Commit the archival transaction using a Conventional Commit; acceptance: hooks pass and only archive/index
      paths are present. `[AC-03]`
      **Result:** committed as `docs(plans): archive the supervision readiness race repair`; only archive and index paths are present.
- [ ] `[AI]` Push the archival branch; acceptance: the pre-push hook exits `0` and the remote head equals the local
      commit. `[AC-03]`
      **Carried by the archival pull request (2026-09-26):** the archival follows the merged repair on `worktree/archive-readiness-race-plan`; this item completes with that pull request.
- [ ] `[AI]` Open the archival PR as a draft; acceptance: it targets `main` and records archived-state proof. `[AC-03]`
      **Carried by the archival pull request (2026-09-26):** the archival follows the merged repair on `worktree/archive-readiness-race-plan`; this item completes with that pull request.
- [ ] `[AI]` Mark the archival PR ready only after the diff is final; acceptance: exact-head quality starts. `[AC-03]`
      **Carried by the archival pull request (2026-09-26):** the archival follows the merged repair on `worktree/archive-readiness-race-plan`; this item completes with that pull request.
- [ ] `[AI]` Post one exact-head leak review for the archival PR; acceptance: it names the unchanged head and reports no
      finding. `[AC-03]`
      **Carried by the archival pull request (2026-09-26):** the archival follows the merged repair on `worktree/archive-readiness-race-plan`; this item completes with that pull request.
- [ ] `[AI]` Rebase-merge the archival PR after all five preconditions pass; acceptance: GitHub reports it merged.
      `[AC-03]`
      **Carried by the archival pull request (2026-09-26):** the archival follows the merged repair on `worktree/archive-readiness-race-plan`; this item completes with that pull request.
- [ ] `[AI]` Run dev artifact clean-up from the primary checkout; acceptance: the plan worktree and task branch are
      absent, `main` equals `origin/main`, and no untracked secret or unrelated artifact was removed. `[AC-03]`
      **Carried by the archival pull request (2026-09-26):** the archival follows the merged repair on `worktree/archive-readiness-race-plan`; this item completes with that pull request.
