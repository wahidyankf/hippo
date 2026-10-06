# Fix: Cancelled Waiter Cleanup Flakes Under Load

Status: In progress (2026-10-06)

On a heavily loaded host, the behaviour scenario "Cancelled FIFO waiters use a fresh cleanup deadline" fails
intermittently: a cancelled reservation waiter is still in the shared FIFO ledger when its acquisition returns. Two
mechanisms produce it. The scenario's own lock holder releases late under scheduler starvation, past the cleanup's 100
ms budget. And the cleanup itself can refuse a coordination lock that is free, because its budget is enforced by a
context that can lapse before the cleanup tries the lock — a production hazard, not only a test one.

This is a [bug-fix plan](../../../repo-governance/conventions/plans/010-bug-fix-plan.md). The owner requested it on
2026-10-06 under the Upstream Tool Defects standard recorded by a consumer workstation, because the defect blocks the
full gate of the `v0.8.5` release, which runs on a host where every workstation repository is guarded through HIPPO.
Under this repository's [upstream tool defects](../../../repo-governance/development/upstream-tool-defects.md) standard,
that request also directs this plan's quality gate, its execution, and its release.

**Workaround, recorded.** Re-running the gate sometimes passes, and the convention counts "a retry that reliably
succeeds" as a workaround. Here the retry is not reliable: the host's sustained load averages of 34–47 are the condition
that produces the failure. The owner's request is what the convention accepts in place of a blocking defect, as in
[the v0.8.4 fix](../../done/2026-10-03__fix-configured-profiles-starve-under-macos-warning/README.md).

**Release.** This fix ships in `v0.8.5`, which the in-flight plan
[strict Go linting and domain modeling](../strict-go-linting-and-domain-modeling/README.md) cuts in its Unit 7. This
plan releases nothing of its own unless that cut happens without the fix (see [Phase 5](#phase-5-release-through-v085)).

Line numbers in this plan are at `db9632a`, the trunk commit that landed it and the one it executes from.

## Bug Report

**Description.** After cancelling a queued reservation waiter whose coordination lock is briefly held,
`AcquireReservation` returns `context.Canceled` but the waiter is still in `reservations.json`; a background retry
removes it later. The scenario that asserts the removal fails at its `Then` step.

**Steps to reproduce**, from a clean checkout of `wahidyankf/hippo` at `a0e7819` with `npm ci` run:

1. _As observed:_ run `npm test` on a host whose load average stays well above its core count. The scenario fails in
   some runs of the race-enabled step,
   `go test -race -count=1 ./cmd/... ./internal/... ./tests/support ./tests/unit ./tests/integration`.
2. _Deterministically:_ in `tests/support/blockers_v04.go`, line 779, change `time.Sleep(20 * time.Millisecond)` to
   `time.Sleep(500 * time.Millisecond)`. This stands in for a release that scheduler starvation delays past the
   cleanup's budget. Then run `go test -count=3 ./tests/integration -run` with the pattern
   `'TestIntegrationBehaviours/^Cancelled_FIFO_waiters_use_a_fresh_cleanup_deadline$'`.
3. _The free-lock refusal, deterministically:_ give `removeReservationWaiterAfterCancellation` a `wait` parameter that
   replaces `coordinationLifecycleWait` in both its `context.WithTimeout` and its lock wait, changing nothing else, and
   call it from a test in package `guard` with a zero wait while nothing holds the lock, as Phase 3's RED item does.

**Expected behaviour.** The scenario at `specs/behaviours/reservations.feature`, lines 243–246, says "bounded cleanup
removes the waiter without blocking the FIFO queue", and [`specs/architecture.md`](../../../specs/architecture.md), line
248, says "cancelled waiter cleanup receives a fresh bounded context". A cleanup whose lock is released within that
bound, or is free, removes the waiter before the acquisition returns.

**Actual behaviour.** In the race-enabled integration run of `npm test`, at load averages of 34–47, the `Then` step
failed with `cancelled waiter remained in FIFO accounting after fresh cleanup deadline`; the scenario passed 5 runs of 5
when run alone. Step 2 failed 3 runs of 3 on 2026-10-06, and the same edit failed the unit adapter the same way:

```text
Scenario: Cancelled FIFO waiters use a fresh cleanup deadline
  # ../../specs/behaviours/reservations.feature:230
  Then bounded cleanup removes the waiter without blocking the FIFO queue
  # ../../specs/behaviours/reservations.feature:233
    Error: cancelled waiter remained in FIFO accounting after fresh cleanup deadline
--- FAIL: TestUnitBehaviours/Cancelled_FIFO_waiters_use_a_fresh_cleanup_deadline (0.21s)
    suite.go:640: cancelled waiter remained in FIFO accounting after fresh cleanup deadline
```

Godog prints each location comment at the end of its line; it is wrapped here to fit. The transcript is verbatim from
the runs at `a0e7819`, where the scenario began at line 230; it now begins at line 243. Step 3 failed 5 runs of 5 with
`context deadline exceeded` while no holder had the lock.

**Error output** is the `Error:` line quoted above, from the step binding `requireV04CancelledWaiterCleanup`,
`tests/support/blockers_v04.go`, line 796.

**Environment.** macOS 15.5 on Apple silicon with 12 cores, Go 1.27.1 (the local toolchain; `go.mod` requires 1.26.1),
Node 24.16.0. Reproduced on the `v0.8.4` tag's code and on trunk at `a0e7819`; the cleanup path and the step binding are
the same in both (`git diff v0.8.4 a0e7819` touches neither `removeReservationWaiterAfterCancellation` nor
`requireV04CancelledWaiterCleanup`, and leaves `internal/guard/coordination.go` unchanged).

**Not a regression.** An instrumented reproduction (a scratch test, since deleted) ran the binding 300 times under
`-race` beside 24 CPU-bound workers: 17 of 300 failed on `892c462`, before the in-flight linting plan began, and 21 of
300 on the current tree. `892c462` is five commits after `v0.8.4`, with identical `internal/` and `tests/` trees. The
constants involved date from `ac76575`, which introduced reservation coordination.

## Duplicate Check

Run 2026-10-06 against `origin/main` at `a0e7819`:

- `gh issue list --state all` returned no issues. `gh issue list --state all --search` for `flake`, `flaky`, `cleanup`,
  `cleanup deadline`, `waiter`, and `cancelled waiter` each returned nothing.
- `gh pr list --state open` returned no pull requests. `gh pr list --state all --search` for the same terms returned
  only merged or closed work, none touching this cleanup: #90 and #94 repaired other timing-sensitive tests, and #74,
  #76, and #78 are the coordination-gate fix `d9ab21a` described under [Root Cause](#root-cause).
- `plans/backlog/` holds no plan, `plans/in-progress/` only the linting plan, and `plans/ideas/` one brief, the
  degraded-admission release-audit follow-ups.
- `grep -rliE` over `plans/` outside `done/` and over `repo-governance/`, for one alternation of `flak`,
  `cleanup deadline`, `coordinationLifecycleWait`, `fresh cleanup`, `cancelled waiter`, `cancelled FIFO`, and
  `removeReservationWaiter`, found nothing; inside `plans/done/` it found only the supervision readiness race repair, a
  different flake.
- `git log -i --grep` for `cancelled waiter`, `cleanup deadline`, `waiter cleanup`, `coordinationLifecycleWait`, and
  `fresh cleanup` found nothing. `git log -L737,800:tests/support/blockers_v04.go` shows the binding unchanged since
  `ac76575` apart from `067fd2a`, which hardened other fixtures for loaded hosts.
- No web search tool was available to this session, so no search for outside reports was made; the defect is in HIPPO's
  own code and tests, which the searches above cover.

No duplicate exists.

## Root Cause

**The cleanup.** When a queued acquisition returns for any reason, a deferred function in
`AcquireReservationWithOptions` (`internal/guard/reservation.go`, lines 1190–1202) calls
`removeReservationWaiterAfterCancellation` (lines 1405–1410). That function makes a fresh
`context.WithTimeout(context.Background(), coordinationLifecycleWait)` and passes it, with the same 100 ms as `wait`
(`internal/guard/coordination.go`, line 24), to `acquireCoordinationLock` through `removeReservationWaiter` (lines
1412–1426). One budget is enforced by two clocks. When the cleanup fails, `retainReservationWaiterUntilCleanup` (lines
1382–1403) keeps the waiter's identity locked and retries every 10 ms in the background, so the waiter leaves the ledger
only later.

**The step.** `requireV04CancelledWaiterCleanup` (`tests/support/blockers_v04.go`, lines 737–800) fills capacity with
one owner, queues a waiter with a 1 s wait, takes `coordination.lock` itself with a blocking `flock` (lines 771 and
522–534), cancels (line 777), and starts a goroutine that sleeps 20 ms and unlocks (lines 778–781). It then waits for
the acquisition to return (line 782) and reads the ledger at once (lines 785–797). Any cleanup that fails, however
briefly, is seen as a waiter that remained.

**Mode 1: the test's release lands after the budget** (29 of the 34 failures the instrumented reproduction captured).
Under starvation the 20 ms sleep returned 98–293 ms late, past the 100 ms the cleanup waits. The cleanup is right to
give up on a lock that is genuinely held; the defect is the fixture's 20 ms margin, a constant chosen on an idle host.
Unloaded, a release delay of 90 ms failed 1 run in 10 and of 110 ms or more failed 10 in 10. Step 2 of the bug report
makes it deterministic.

**Mode 2: the cleanup refuses a free lock** (the other 5). The release was on time, but the cleanup's goroutine stalled
past its context's deadline, and the context path refuses without trying the lock:

- At entry, `acquireCoordinationProcessGate` returns `ctx.Err()` before looking at the gate (`coordination.go`, lines
  103–105), so a context that lapsed before the first attempt refuses a free lock outright. Step 3 of the bug report
  reaches this path deterministically: `context.WithTimeout` with a zero duration is cancelled before it returns
  (`if dur <= 0 { c.cancel(true, DeadlineExceeded, cause) }` in the standard library's `context.WithDeadlineCause`).
- In the poll loop (`coordination.go`, lines 213–245), a failed `flock` is followed by a `select` on `ctx.Done()` and
  the poll timer (lines 234–244). When both are ready, the [Go specification][go-select] says "a single one that can
  proceed is chosen via a uniform pseudo-random selection", and the `ctx.Done()` case returns without another `flock`,
  so a lock freed during the sleep is refused about half the time.

The `wait` budget alone never does this. The loop tries `flock` (line 214) before it checks the deadline (line 224), so
every refusal on `wait` follows a fresh failed attempt. The process gate takes a free gate before it starts its timer
(lines 142–151), the guard `d9ab21a` added after the same random choice refused a free gate at a nearly spent budget.
Only the context path lacks that ordering, and the cleanup is the one production caller whose context exists only to
repeat its `wait`: `lockCoordinationForRelease` (lines 248–250) passes `context.Background()` with the same 100 ms.

The production consequence of mode 2: a cancelled run's waiter can stay in the FIFO ledger with a free lock until a
background retry removes it, and while it is at the head, no waiter behind it is admitted (`reservation.go`, lines
1284–1285 admit only the head).

## Solution

Two changes, one per mode.

**1. The cleanup budget is one clock.** `removeReservationWaiterAfterCancellation` takes a `wait` and acquires the lock
with `acquireCoordinationLock(context.Background(), root, wait)`; `removeReservationWaiter` and its context parameter
are folded into it. `context.Background()` "is never canceled, has no values, and has no deadline" ([context
package][go-context]), so its `Done` channel never becomes ready; in Go 1.27.1 it is `nil`, and "receiving from a nil
channel blocks forever" ([Go specification][go-receive]). Either way the loop's `select` can only take the timer. The
cleanup then refuses only after an attempt has found the lock held: [`flock`][flock-darwin] with `LOCK_NB` fails with
`EWOULDBLOCK` exactly when "the file is locked". This mirrors `lockCoordinationForRelease` rather than inventing a new
rule.

Why it removes the cause: the refusal paths of mode 2 belong to the context, and the cleanup no longer has one. A stall
before the first attempt, a stall in the loop, and every background retry by `retainReservationWaiterUntilCleanup`
(which calls the same function) all end in an attempt on the lock. The budget stays bounded: a held lock is still
refused after `wait`, so the scenario "Failed cancelled-waiter cleanup retains verifiable FIFO ownership" (lines
262–266) keeps its 500 ms bound.

**2. A cleanup-wait seam, for the scenario.** `ReservationAdmissionOptions` (`reservation.go`, lines 47–53), which
already carries the `Now`, `Pause`, and `Heartbeat` seams, gains `CleanupWait time.Duration`. Zero, the default, means
`coordinationLifecycleWait`. `AcquireReservationWithOptions` passes it to the deferred cleanup; the background retry
keeps `coordinationLifecycleWait` per attempt. `Run` (`internal/guard/run.go`, lines 629–645) does not set it, so
production behaviour is unchanged. The step binding sets `CleanupWait: 2 * time.Second` and keeps its strict immediate
read. It also holds the lock for 500 ms instead of 20 ms: above the 100 ms default, so the scenario fails if the
configured wait is ever ignored, and 1.5 s below the configured wait — five times the worst lateness measured (293 ms).

**Conditions beyond the reported one.**

- Callers whose context carries a caller's cancellation keep "cancellation wins": the admission wait (`reservation.go`,
  line 1209), the lease wait (`internal/guard/lease.go`, line 460), and status observation. The test
  `TestCoordinationLockRejectsPreCanceledContext` (`internal/guard/run_test.go`, line 694) pins that, and this plan does
  not touch it.
- `waitReservationVictimRelease` (`run.go`, lines 299–312) is the other production caller with a timeout context. There
  the context bounds a whole observation loop of many attempts, and lapsing ends the observation; no symptom traces to
  it, so it is left as it is.

**Alternatives rejected.**

- _A final non-blocking `flock` in `acquireCoordinationLock`'s `ctx.Done()` case_, the first fix considered. It leaves
  the gate's entry check (`coordination.go`, lines 103–105) refusing a lapsed context, so a cleanup stalled before its
  first attempt still refuses a free lock, and Phase 3's RED still fails. Extending it to the entry check would let a
  signal-cancelled admission take a lock freed at that moment and admit work after cancellation, contradicting
  `TestCoordinationLockRejectsPreCanceledContext` unless deadline and cancellation were told apart in a function every
  coordination path shares. That is more surface than the one caller that needs it.
- _Raise `coordinationLifecycleWait`._ It bounds release, peak observation, and victim presence too, and the sister
  scenario's 500 ms bound; it changes production behaviour to widen a test margin.
- _Poll the ledger in the step until the waiter leaves._ The background retry removes it eventually whether or not the
  fresh cleanup worked, so the step would pass with the behaviour broken: assertion theater, per
  [Gherkin implementation review](../../../repo-governance/workflows/quality/gherkin-implementation-review.md).
- _Retry or quarantine the scenario._ It weakens a gate, which the upstream tool defects standard forbids.

**Public contract.** No exit status, `hippo.*` code, JSON document, configuration key, evidence shape, or ledger field
moves. `ReservationAdmissionOptions` is in `internal/guard`, which no consumer can import. The release is a patch, part
of `v0.8.5`.

**References**, read 2026-10-06 from the Go 1.27.1 distribution's copies and the macOS 15.5 manual:

- [The Go Programming Language Specification: Select statements][go-select] — the uniform pseudo-random choice among
  ready cases. Primary source for mode 2's loop path.
- [The Go Programming Language Specification: Receive operator][go-receive] — a receive from a `nil` channel blocks
  forever.
- [Package context][go-context] — `Background` is never cancelled and has no deadline, and "Done may return nil if this
  context can never be canceled".
- [The context package's source][go-context-src], `WithDeadlineCause` — a deadline already passed cancels the context
  before it is returned, which makes Phase 3's RED deterministic; `emptyCtx.Done` returns `nil`.
- [`flock(2)`, macOS][flock-darwin], and [`flock(2)`, Linux][flock-linux] — `LOCK_NB` fails with `EWOULDBLOCK` instead
  of blocking, and locks belong to the open file, so the step's separate descriptor contends with HIPPO's.
- `d9ab21a`, "fix(guard): take a free coordination gate before any bounded wait" — the same random choice, fixed for the
  process gate's timer; this plan's fix applies its rule to the cleanup's lock wait.

### Specification Changes

- `specs/architecture.md` `[E]`, the reservation-lifecycle constraint at line 248:

  ```diff
  - cancelled waiter cleanup receives a fresh bounded context
  + cancelled waiter cleanup receives a fresh bounded lock wait and takes a free lock however late it starts
  ```

- `specs/behaviours/reservations.feature`: no change. "Cancelled FIFO waiters use a fresh cleanup deadline" and "Failed
  cancelled-waiter cleanup retains verifiable FIFO ownership" are preserved word for word; only the first one's binding
  changes, in `tests/support/blockers_v04.go`. Both stay `@e2e-exempt` with their `tests/contract/contract.go` entries
  unchanged, and run at the unit and integration adapters.

### File Impact

```text
internal/guard/reservation.go      CleanupWait seam; cleanup acquires with context.Background() and its wait
internal/guard/run_test.go         TestCancelledWaiterCleanupTakesAFreeLockWithItsBudgetSpent
tests/support/blockers_v04.go      requireV04CancelledWaiterCleanup: 500 ms hold, CleanupWait 2 s
specs/architecture.md              line 248, as above
CHANGELOG.md                       the v0.8.5 Fixed bullet
plans/in-progress/fix-cancelled-waiter-cleanup-flake/README.md   execution record
```

### Acceptance Criteria

- **AC-01** Given a ledger holding one waiter and a coordination lock nobody holds, when its cleanup runs with no budget
  left, then it takes the lock and removes the waiter.
- **AC-02** Given a queued waiter whose coordination lock is held for 500 ms after its acquisition is cancelled, with a
  2 s cleanup wait, when the acquisition returns, then it returns `context.Canceled` and the ledger read at once holds
  no waiter.
- **AC-03** Given the default cleanup wait and a lock held past it, then the cancelled acquisition still returns within
  500 ms with the FIFO bytes and the locked identity retained, and a pre-cancelled context still refuses the lock.
- **AC-04** Given the fix branch, then `git diff --name-only origin/main...HEAD` lists only the File Impact paths, and
  no exit status, code, document, configuration key, or ledger field changes.
- **AC-05** Given the host's load at execution, recorded with `uptime`, then the two cleanup scenarios pass 20
  consecutive race-enabled integration runs, and the full gate passes on the fix branch's head.
- **AC-06** Given the fix merges, then a published `v0.8.5` (or, if that cut missed it, `v0.8.6`) contains its merge
  commit, and its `CHANGELOG.md` entry names the fix under `Fixed`.
- **AC-07** Given execution finishes, then this plan is gated, reconciled, and archived under `plans/done/`, and its
  worktrees and branches are gone.

## Delivery

**Execution checkout.** `worktrees/fix-cancelled-waiter-cleanup-flake` under the HIPPO repository location, created from
`origin/main` at `a0e7819` on branch `worktree/fix-cancelled-waiter-cleanup-flake`, per
[worktree to pull request](../../../repo-governance/workflows/maintenance/worktree-to-pull-request.md). Unit 1 lands
from that branch. Unit 2 branches from `origin/main` in the same directory as
`worktree/fix-cancelled-waiter-cleanup-flake-fix`. HIPPO cannot guard its own gates, so every command below runs
directly, never under `./hippo`, per
[resource-aware development](../../../repo-governance/development/resource-aware-development.md).

**Deviation, recorded.** The owner directed on 2026-10-06 that this worktree and its branches be removed immediately
after the fix merges. [Integration path](../../../repo-governance/conventions/integration-path.md) provisions at most
one worktree per plan and removes it once every unit has landed, and Unit 4 cannot land before `v0.8.5` exists. So Unit
4 provisions a second worktree, `worktrees/fix-cancelled-waiter-cleanup-flake-record`, once `v0.8.5` is published, and
removes it after its own merge.

**Commands** the items below name, run from the worktree root:

- _Focused scenarios_: `go test -count=1 -run 'TestUnitBehaviours/^(Cancelled_FIFO_waiters|Failed_cancelled-waiter)_'`
  `./tests/unit` for the unit adapter, and the same with `TestIntegrationBehaviours` and `./tests/integration` for the
  integration adapter. Godog runs each scenario as a subtest named after it, and only "Cancelled FIFO waiters use a
  fresh cleanup deadline" and "Failed cancelled-waiter cleanup retains verifiable FIFO ownership" carry those prefixes.
- _Repeated scenarios_: the integration form of _Focused scenarios_ with `-race -count=20 -timeout 60m -v` in place of
  `-count=1`. A race-enabled pass took about 60 s at load averages of 14–35 on 2026-10-06, so Go's default 10-minute
  timeout stopped an earlier attempt after 10 passes.
- _Cleanup test_: `go test -count=20 ./internal/guard -run` with the pattern
  `'TestCancelledWaiterCleanupTakesAFreeLockWithItsBudgetSpent|TestCoordinationLock'`.
- _Full gate_: `GOFLAGS=-timeout=30m npm test`, the release gate with its per-package timeout raised for the host's
  load, as `scripts/test-loaded.sh` raises it.
- _Reconcile_: `git -C ../.. fetch origin`, then `git -C ../.. merge --ff-only origin/main`, proved by
  `git -C ../.. rev-list --left-right --count HEAD...origin/main` reading `0 0`.
- _Land_: inspect the diff against [data safety](../../../repo-governance/conventions/public-repository-data-safety.md)
  and commit; run the [push review](../../../repo-governance/workflows/quality/pr-leak-review/002-push-review.md) and
  push; screen the title and body with `scripts/public-safety/outbound-preflight.sh --surface pull-request` and open a
  draft pull request; mark it ready; wait for `Quality gate` on the head, polling no faster than every three minutes;
  post the [leak review](../../../repo-governance/workflows/quality/pr-leak-review.md) for that exact head; rebase-merge
  once every [merge precondition](../../../repo-governance/conventions/pull-request-merge.md) holds; then _Reconcile_.

**Delivery units**, landed serially:

1. _Plan_ — this file and the in-progress index entry alone. Rollback: revert its merge.
2. _Fix_ — the two code changes, their tests, the specification line, the `CHANGELOG.md` bullet, and this plan's
   execution record. Rollback: revert its merge before `v0.8.5` is cut; after it, publish the next patch.
3. _Release_ — `v0.8.5`, cut by the linting plan's Unit 7 after unit 2 merges; this plan only records it. That plan's
   Unit 7 holds the cut until this fix has merged, in its item "Before cutting, confirm both bug-fix plans this release
   carries have merged", and completes the `## [v0.8.5] — Unreleased` entry that Phase 4 adds, in its item "Add the
   `v0.8.5` entry to `CHANGELOG.md`, or complete and date the `## [v0.8.5] — Unreleased` entry the cancelled-waiter fix
   added". A published tag is never replaced.
4. _Record_ — the release record, the execution check, and the move to `plans/done/`. Rollback: revert its merge.

**Out of scope: repinning consumers.** The linting plan leaves repinning `v0.8.5` to each consumer's own repository,
coordinated outside this one ([plan lifecycle](../../../repo-governance/conventions/plan-lifecycle.md)); this plan
records the release values a repin uses.

**Pause safety.** Each item records its result when ticked. To resume, read the last ticked item, then
`git -C <worktree> log --oneline origin/main..HEAD`, `gh pr list --head <branch>`, and
`git ls-remote --tags origin v0.8.5`. Between unit 2's merge and unit 4, the record is this file on `origin/main`, with
Phases 1–4 ticked and Phase 5 open.

### Phase 1: Plan

- [x] `[AI]` Land this file and its `plans/in-progress/README.md` entry alone with _Land_, from
      `worktree/fix-cancelled-waiter-cleanup-flake`; proof: the merge commit on `origin/main` and _Reconcile_ reading
      `0 0`. `[AC-07]`
  - Result: (2026-10-06) pull request #136, rebased once onto `c109c4d`, merged as `db9632a`; _Reconcile_ `0 0`.
- [x] `[AI]` Create the fix branch in the same directory: `git fetch origin --prune`, then
      `git switch -c worktree/fix-cancelled-waiter-cleanup-flake-fix origin/main`, then `npm ci`; proof:
      `git branch --show-current` prints the branch and `git status --porcelain` prints nothing. `[AC-04]`
  - Result: `worktree/fix-cancelled-waiter-cleanup-flake-fix` from `origin/main` at `db9632a`, `node_modules` already
    installed by the plan unit's `npm ci`; the merged plan branch was deleted at once; tree clean.
- [x] `[AI]` Run the [plan quality gate](../../../repo-governance/workflows/quality/plan-quality-gate.md) on this folder
      in mode `normal`, at most three cycles, and commit its repairs and its verdict line as the fix branch's first
      commit, a `docs(plans)` commit; proof: one terminal `plan-quality-gate:` verdict line recorded here and
      `git status --porcelain` printing nothing. `[AC-07]`
  - Result: `plan-quality-gate: PASS (2 cycles, 6 rows fixed: 1 HIGH, 3 MEDIUM, 2 LOW; 0 open)`. Entry checks: prettier,
    `markdownlint-cli2` 0 issues, `./rhino md internal-link validate` and `./rhino governance directory-map validate` no
    findings. Cycle 1: PQG-01 (HIGH, record-worktree provisioning after the items writing into it), PQG-02, PQG-03,
    PQG-04, PQG-05 fixed. Cycle 2: all five held; PQG-06 (MEDIUM, rebase on a dirty tree) found and fixed.

### Phase 2: Mode 1 — The Scenario Honours a Configured Cleanup Wait

- [ ] `[AI]` RED: in `tests/support/blockers_v04.go`, `requireV04CancelledWaiterCleanup`, change the release delay from
      `20 * time.Millisecond` to `500 * time.Millisecond`; run _Focused scenarios_ at both adapters; proof: both fail
      "Cancelled FIFO waiters use a fresh cleanup deadline" with
      `cancelled waiter remained in FIFO accounting after fresh cleanup deadline`, and both pass "Failed
      cancelled-waiter cleanup retains verifiable FIFO ownership". `[AC-02]`
- [ ] `[AI]` GREEN: add `CleanupWait` to `ReservationAdmissionOptions` in `internal/guard/reservation.go`, default zero
      to `coordinationLifecycleWait` in `AcquireReservationWithOptions`, and give
      `removeReservationWaiterAfterCancellation` a `wait` parameter that replaces `coordinationLifecycleWait` in both
      its `context.WithTimeout` and its lock wait, leaving its context in place; the deferred cleanup passes the option
      and `retainReservationWaiterUntilCleanup` passes `coordinationLifecycleWait`. In the binding, call
      `guard.AcquireReservationWithOptions` with `guard.ReservationAdmissionOptions{CleanupWait: 2 * time.Second}`;
      proof: _Focused scenarios_ pass at both adapters. `[AC-02]` `[AC-03]`
- [ ] `[AI]` REFACTOR: document the field in one comment naming its zero value, and keep `Run` (`internal/guard/run.go`)
      without it; proof: `git grep -n 'CleanupWait' -- internal cmd` prints only `internal/guard/reservation.go` lines,
      `go tool golangci-lint run ./internal/guard/... ./tests/support/...` prints `0 issues.`, and _Focused scenarios_
      still pass at both adapters. `[AC-02]` `[AC-04]`

### Phase 3: Mode 2 — The Cleanup Never Refuses a Free Lock

- [ ] `[AI]` RED: add `TestCancelledWaiterCleanupTakesAFreeLockWithItsBudgetSpent` to `internal/guard/run_test.go`
      beside `TestCoordinationLockRejectsPreCanceledContext`. It calls `ensureReservationCoordination`, writes with
      `writeReservationLedger` a ledger that passes `validateReservationLedger`, so the test fails at the lock alone:
      `SchemaVersion` `reservationLedgerSchemaVersion`, `NextSequence` 1, `Capacity` one CPU and 256 MiB
      (`ReservationVector{CPU: MinimumReservationCPU, MemoryBytes: MinimumReservationMemoryBytes}`), and one waiter with
      `Token` from `token()`, `PID` this process's, `Class` `policy.TaskEphemeral`, `Profile` `minimal`, `Requested` the
      same one CPU and 256 MiB, `Sequence` 1, and `MaxOwners` 20, then calls
      `removeReservationWaiterAfterCancellation(root, value, 0)` with no holder of the lock and requires a nil error and
      an empty `Waiters` from `readReservationLedger`; run the _Cleanup test_; proof: it fails all 20 passes with
      `context deadline exceeded`, and every `TestCoordinationLock` test passes. `[AC-01]`
- [ ] `[AI]` GREEN: in `removeReservationWaiterAfterCancellation`, drop the context and acquire with
      `acquireCoordinationLock(context.Background(), root, wait)`, folding `removeReservationWaiter` into it; proof: the
      _Cleanup test_ passes all 20 passes, and _Focused scenarios_ pass at both adapters. `[AC-01]` `[AC-03]`
- [ ] `[AI]` REFACTOR: update the deferred function's `//nolint:contextcheck` explanation from "its own bounded context"
      to "its own bounded wait" (the linter still requires the directive), and confirm the cleanup was the only context
      that merely repeated its wait; proof: `git grep -n 'context.WithTimeout' -- 'internal/guard/*.go' ':!*_test.go'`
      prints only `internal/guard/run.go` (`waitReservationVictimRelease`), and `npm run test:quick` exits `0`.
      `[AC-01]` `[AC-04]`

### Phase 4: Specification, Documentation, and Verification

- [ ] `[AI]` Synchronize `specs/architecture.md`, line 248, as-built per [Specification Changes](#specification-changes)
      under [specification maintenance](../../../repo-governance/development/specification-maintenance.md); proof:
      `grep -c 'fresh bounded lock wait' specs/architecture.md` prints `1`, and
      `HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd` and
      `HIPPO_BDD_ADAPTER=integration go test -count=1 ./tests/bdd` exit `0`. `[AC-03]`
- [ ] `[AI]` Run the
      [Gherkin implementation review](../../../repo-governance/workflows/quality/gherkin-implementation-review.md) on
      the two cleanup scenarios, with these break tests, each reverted after it fails: always passing
      `coordinationLifecycleWait` to the deferred cleanup fails "Cancelled FIFO waiters use a fresh cleanup deadline"
      with the reported message; defaulting `CleanupWait` to 2 s fails "Failed cancelled-waiter cleanup retains
      verifiable FIFO ownership" with `failed-cleanup cancellation was not bounded`; restoring the context in the
      cleanup fails the _Cleanup test_. Proof: each scenario's status and each break test's failure recorded here, none
      `untested`, `unimplemented`, or `drifted`. `[AC-01]` `[AC-02]` `[AC-03]`
- [ ] `[AI]` Run [docs propagation](../../../repo-governance/workflows/quality/docs-propagation.md): add to
      `CHANGELOG.md` a `Fixed` bullet saying that cancelling a queued run could leave its waiter in the shared FIFO
      queue, holding up the waiters behind it, when its cleanup started more than 100 ms after cancellation, even with
      the coordination lock free, and that the cleanup now takes a free lock however late it starts. Place it under the
      `## [v0.8.5]` entry if one exists on `origin/main`; otherwise add `## [v0.8.5] — Unreleased` with a `### Fixed`
      subsection above `## [v0.8.4]`, which the linting plan's Unit 7 completes when it adds its own bullet and dates
      the heading. `docs/explanation/failing-closed.md`, lines 65–66 ("a fresh bounded cleanup attempt") stays true and
      is not changed. Proof: `npm run format:check` exits `0` and `git grep -n 'however late it starts' CHANGELOG.md`
      prints one line. `[AC-06]`
- [ ] `[AI]` Bounded checkpoint: run _Repeated scenarios_, recording `uptime` before and after; proof: exit `0` with 20
      `--- PASS` lines for each scenario. Fallback, decided now: one failure stops the plan before landing; its output
      is recorded here, the cause it shows replaces the matching hypothesis in [Root Cause](#root-cause), and no release
      carries this fix until a new RED proves that cause. `[AC-05]`
- [ ] `[AI]` Run the _Full gate_ on the branch head; proof: exit `0`, ending with `No vulnerabilities found.` `[AC-05]`
- [ ] `[AI]` Before landing, commit the execution record so far as a `docs(plans)` commit on the fix branch, because
      `git rebase` refuses a dirty tree and the rebase never auto-stashes; then `git fetch origin --tags` and confirm
      `v0.8.5` does not yet exist; then rebase onto `origin/main`, reading the whole incoming diff (the linting plan's
      units edit `internal/guard/reservation.go`, `tests/support/blockers_v04.go`, and `specs/architecture.md`), and
      rerun _Focused scenarios_ at both adapters and the _Full gate_ if the rebase brought commits; proof:
      `git status --porcelain` prints nothing before the rebase, `git ls-remote --tags origin v0.8.5` prints nothing,
      and the reruns exit `0`. If the tag already exists, land anyway and the recovery item in Phase 5 fires. `[AC-05]`
      `[AC-06]` `[AC-07]`
- [ ] `[AI]` Confirm the change stays inside its boundary; proof: `git diff --name-only origin/main...HEAD` prints only
      the paths in [File Impact](#file-impact). `[AC-04]`
- [ ] `[AI]` Commit the rest of this plan's execution record (Phases 1–4 ticked with the rebase, rerun, and boundary
      results) as a `docs(plans)` commit on the fix branch, as the last commit before landing; proof:
      `git status --porcelain` prints nothing. `[AC-07]`

### Phase 5: Release Through v0.8.5

- [ ] `[AI]` Land unit 2 with _Land_; proof: the merge commit on `origin/main` and _Reconcile_ reading `0 0`, both
      recorded here by unit 4, since the merged copy cannot hold its own merge. The fix must merge before the linting
      plan's Unit 7 tags `v0.8.5`. `[AC-01]` `[AC-02]` `[AC-03]` `[AC-04]` `[AC-05]`
- [ ] `[AI]` Immediately after that merge, run
      [dev artifact clean-up](../../../repo-governance/workflows/maintenance/dev-artifact-clean-up.md) for this
      worktree, at the owner's direction: confirm nothing is unpushed or running, then remove the worktree with
      `git worktree remove` without `--force`, and delete `worktree/fix-cancelled-waiter-cleanup-flake` and `...-fix`
      locally and on `origin`; proof: `git worktree list` omits it,
      `git branch --list 'worktree/fix-cancelled-waiter-*'` and
      `git ls-remote origin 'refs/heads/worktree/fix-cancelled-waiter-*'` print nothing. `[AC-07]`
- [ ] `[AI]` Provision `worktrees/fix-cancelled-waiter-cleanup-flake-record` from `origin/main` on branch
      `worktree/fix-cancelled-waiter-cleanup-flake-record`, with `npm ci`, once `v0.8.5` is published; proof:
      `git branch --show-current` prints the branch. `[AC-07]`
- [ ] `[AI]` Once `v0.8.5` is published, record the release in the record worktree's copy of this plan; proof:
      `git merge-base --is-ancestor <unit 2 merge commit> v0.8.5` exits `0`, `git show v0.8.5:CHANGELOG.md` holds the
      `Fixed` bullet, and the release URL, the tag's peeled commit, and `checksums.txt` are recorded here. `[AC-06]`
- [ ] `[AI]` Recovery, dormant until triggered. Trigger: `v0.8.5` is published without unit 2's merge commit (the
      ancestry check above exits `1`). Then cut `v0.8.6` on `origin/main` through
      [release cut](../../../repo-governance/workflows/maintenance/release-cut.md), moving the bullet to a dated
      `## [v0.8.6]` entry and naming `v0.8.6` in every page that names the current release; proof: the same three
      records for `v0.8.6`. Otherwise: a dated, evidenced `Not triggered`. `[AC-06]`

### Phase 6: Close

- [ ] `[AI]` Route each learning below to its durable owner, or discard it with a reason; proof: each entry names its
      owner or its reason. `[AC-07]`
- [ ] `[AI]` Run the [execution check](../../../repo-governance/workflows/plan/plan-execution-check.md); proof: its
      verdict line recorded here. `[AC-07]`

### Archival

- [ ] `[AI]` Move this folder with `git mv` to `plans/done/<completion date>__fix-cancelled-waiter-cleanup-flake/`,
      update `plans/in-progress/README.md` and `plans/done/README.md` in the same change, and land it with _Land_;
      proof: the merge commit, posted on the archival pull request, and no copy left under `plans/in-progress/`.
      `[AC-07]`
- [ ] `[AI]` Run [dev artifact clean-up](../../../repo-governance/workflows/maintenance/dev-artifact-clean-up.md) for
      the record worktree right after that merge; proof, posted on the archival pull request: `git worktree list` omits
      it, no local or remote `worktree/fix-cancelled-waiter-*` branch remains, and _Reconcile_ reads `0 0`. `[AC-07]`

## Learnings

None yet. Entries are added as execution teaches something, and each is routed to a durable owner or discarded with a
reason before archival.

## Directory Map

This plan is one document, so this README has no siblings to map.

[go-select]: https://go.dev/ref/spec#Select_statements
[go-receive]: https://go.dev/ref/spec#Receive_operator
[go-context]: https://pkg.go.dev/context
[go-context-src]: https://go.dev/src/context/context.go
[flock-darwin]:
  https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/flock.2.html
[flock-linux]: https://man7.org/linux/man-pages/man2/flock.2.html
