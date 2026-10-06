# Fix: Distinct-Root Coordination Lock Test Flakes Under Load

Status: In progress (2026-10-06)

On a heavily loaded host, the unit test `TestCoordinationLockAllowsDistinctRootsInParallel` fails intermittently with
"distinct-root coordination was serialized" although nothing was serialized: the second root's lock was taken without
waiting, and only the runner was slow. The test times a lock acquisition on the wall clock and calls a slow acquisition
serialization, but a serialized acquisition cannot succeed at all here — it is refused, and the test already fails on
that refusal. The defect is in the test alone; the coordination lock behaves as specified.

This is a [bug-fix plan](../../../repo-governance/conventions/plans/010-bug-fix-plan.md). The owner requested it on
2026-10-06, directing that every flake found be fixed, under the Upstream Tool Defects standard recorded by a consumer
workstation, because the defect fails the full gate of the `v0.8.5` release, which runs on a host where every
workstation repository is guarded through HIPPO. Under this repository's
[upstream tool defects](../../../repo-governance/development/upstream-tool-defects.md) standard, that request also
directs this plan's quality gate, its execution, and its release.

**Workaround, recorded.** Re-running the gate sometimes passes, and the convention counts "a retry that reliably
succeeds" as a workaround. Here the retry is not reliable: the host's sustained load is the condition that produces the
failure. The owner's request is what the convention accepts in place of a blocking defect, as in
[the v0.8.4 fix](../../done/2026-10-03__fix-configured-profiles-starve-under-macos-warning/README.md) and the sibling
plans [`fix-cancelled-waiter-cleanup-flake`](../fix-cancelled-waiter-cleanup-flake/README.md) and
[`fix-degraded-lineage-scenario-flake`](../../done/2026-10-07__fix-degraded-lineage-scenario-flake/README.md).

**Release.** The change is test-only, so it carries no release content of its own: no `CHANGELOG.md` entry and no tag.
It must merge before the in-flight plan
[strict Go linting and domain modeling](../strict-go-linting-and-domain-modeling/README.md) cuts `v0.8.5` in its Unit 7,
so that cut's full gate runs with it (see [Phase 5](#phase-5-release-through-v085)).

Line numbers in this plan are at `4cd530a`, the trunk commit it was written from, unless a sentence names another commit
or tag.

## Bug Report

**Description.** Under host load, `TestCoordinationLockAllowsDistinctRootsInParallel` fails its elapsed-time assertion
after acquiring a second, distinct root's coordination lock successfully, reporting a correct acquisition as
serialization.

**Steps to reproduce**, from a clean checkout of `wahidyankf/hippo` at `4cd530a` with `npm ci` run:

1. _As observed:_ run `GOFLAGS=-timeout=30m npm test` on a host whose load average stays well above its core count. The
   test fails in some runs of one of the two steps that test `./internal/...` (`scripts/test-quick.sh`, line 35, or
   `scripts/test.sh`, line 20), and passes on rerun.
2. _Under in-process contention:_ add, through a Go build overlay, a scratch test file to package `guard` (the
   _Contention harness_ under [Delivery](#delivery)) whose eight goroutines each run 25 rounds of the test's own body —
   hold a fresh root with a 1 s wait, then time the acquisition and release of a second fresh root with a 50 ms wait —
   and count the rounds that succeed but take 40 ms or more. Run it with
   `GOMAXPROCS=1 go test -overlay <file> -race -count=5 -timeout 30m -v -run`, the pattern
   `'^TestContentionScratchToday$'`, and `./internal/guard`.
3. _Deterministically:_ through a Go build overlay, add `time.Sleep(50 * time.Millisecond)` as the first statement of
   `openCoordinationLock` (`internal/guard/coordination.go`, line 94), standing in for a runner the scheduler starves
   inside the timed window; it slows every acquisition and serializes nothing. Then run
   `go test -overlay <file> -count=3 -v -run '^TestCoordinationLockAllowsDistinctRootsInParallel$' ./internal/guard`.

**Expected behaviour.** [`specs/architecture.md`](../../../specs/architecture.md), lines 78–79, says HIPPO instances
"coordinate through that same root", and line 237 that mutations are "serialized by the shared `coordination.lock`" — a
lock per root. Acquiring one root's lock while another root's is held should succeed whatever the runner's speed, and
the test should fail only when the two roots contend.

**Actual behaviour.** The full gate on branch `worktree/fix-cancelled-waiter-cleanup-flake-fix`, at a load average near
28, failed with the output below; that branch leaves `internal/guard/coordination.go` and lines 674–692 of
`internal/guard/run_test.go` unchanged. Run alone afterwards, the test passed 5 runs of 5 in about 0.2 s, and 20 of 20
on 2026-10-06 at `4cd530a`.

```text
run_test.go:690: distinct-root coordination was serialized: 96.982459ms
FAIL github.com/wahidyankf/hippo/internal/guard 18.179s
```

Step 3 failed 3 runs of 3 on 2026-10-06, each after the acquisition succeeded:

```text
=== RUN   TestCoordinationLockAllowsDistinctRootsInParallel
    run_test.go:690: distinct-root coordination was serialized: 53.270125ms
--- FAIL: TestCoordinationLockAllowsDistinctRootsInParallel (0.11s)
```

The other two runs reported 50.607459 ms and 52.195875 ms. Step 2, run six times (`-count=1` once, then `-count=5`) at
load averages of 18–27, counted 8 of 1,200 rounds that succeeded in 40 ms or more, the slowest in 114.646292 ms, and no
refusal: a round more than twice as long as its own 50 ms wait still took the lock.

**Error output** is the `run_test.go:690` line quoted above, from the test's elapsed assertion (lines 689–691).

**Environment.** macOS 15.5 on Apple M2 Max with 12 cores, Go 1.27.1 (the local toolchain; `go.mod` requires 1.26.1),
Node 24.16.0, at `4cd530a` (`origin/main`).

**Not a regression.** The test dates from `ac76575` (2026-09-05), which introduced reservation coordination, and
`git log -L674,692:internal/guard/run_test.go` shows no later change. The `v0.8.4` tag carries the same test body (its
line 246) and the same `internal/guard/coordination.go` (`git diff v0.8.4 4cd530a -- internal/guard/coordination.go`
prints nothing).

## Duplicate Check

Run 2026-10-06 against `origin/main` at `4cd530a`:

- `gh issue list --state all` returned no issues. `gh issue list --state all --search` for `flake`, `flaky`,
  `distinct-root`, `distinct roots`, `serialized`, `coordination lock`, and `AllowsDistinctRoots` each returned nothing.
- `gh pr list --state open` returned no pull requests. `gh pr list --state all --search` for the same terms returned
  only merged or closed work, none touching this test: #90 and #94 repaired other wall-clock tests, #74, #76, and #78
  are the coordination-gate fix `d9ab21a` described under [Root Cause](#root-cause), #136 and #137 are the two sibling
  bug-fix plans, and the rest (#1, #3, #18, #27, #66, #70, #79, #80, #81, #83, #89, #91, #92, #93, #95, #96, #97, and
  #135) are features, other fixes, releases, and documentation that leave this test unchanged.
- `plans/backlog/` holds no plan; `plans/in-progress/` holds the linting plan and the two sibling bug-fix plans; and
  `plans/ideas/` holds one brief, the degraded-admission release-audit follow-ups.
- `grep -rliE` over `plans/` outside `done/` and over `repo-governance/`, for one alternation of `AllowsDistinctRoots`,
  `distinct-root`, `distinct roots`, `serialized`, and `flak`, matched only on `flak`: the two sibling plans, the
  in-progress index, and the linting plan's `delivery.md` and `learnings.md`, each about the sibling flakes. Inside
  `plans/done/`, `AllowsDistinctRoots`, `distinct-root`, `distinct roots`, and `flak` found only the supervision
  readiness race repair, a different flake.
- `git log -i -F --grep` for `distinct-root`, `distinct root`, `AllowsDistinctRoots`, `serialized`, and `wall-clock`
  found nothing.
- No web search tool was available to this session, so no search for outside reports was made; the defect is in HIPPO's
  own test, which the searches above cover.

No duplicate exists. Related, not duplicates: the sibling plans fix the same class of defect — a timing constant chosen
on an idle host and failing under load, the class `scripts/test-loaded.sh` describes (lines 7–16) — in other fixtures,
as #90 (`abc9094`) and #94 (`68cae05`) did before them.

## Root Cause

**What the test does.** It holds the coordination lock of one `t.TempDir()` root with a 1 s wait and releases it only
when the test returns (`internal/guard/run_test.go`, lines 675–679). It starts a clock (line 681), then acquires a
second `t.TempDir()` root's lock with a 50 ms wait (line 682), fails on any error (lines 683–685), releases it (lines
686–688), and fails if the elapsed time reached 40 ms (lines 689–691). The timed window covers creating the second
temporary directory, which is evaluated after the clock starts, opening or creating its `coordination.lock`
(`coordination.go`, line 95), and the gate and lock calls themselves.

**What "serialized" could mean, and where it would show.** Two acquisitions in one process can contend in two places
only:

- the in-process gate, `acquireCoordinationProcessGate` (`coordination.go`, lines 102–174), keyed by the device and
  inode of the lock file (lines 106–118); and
- the file lock, `flock` with `LOCK_NB` (line 214), which [`flock(2)`][flock-darwin] places on the file ("Locks are on
  files, not file descriptors").

Distinct `t.TempDir()` roots give distinct `coordination.lock` files ("Each subsequent call to TempDir returns a unique
directory", [package testing][go-tempdir]), so distinct gates and distinct locks. The one state they share is the
`coordinationProcessGates` mutex (lines 58–66), held only for map bookkeeping (lines 119–127, 130–135, 138–140, and
177–187) and never across a wait.

If the roots did contend, the first lock is held for the whole timed window, so nothing could free it: the gate's timer
(lines 158–173) or the lock loop's deadline (lines 224–229) refuses with `errCoordinationDeferred` after 50 ms, and line
684 fails the test before the elapsed check is reached. A scratch overlay that keys every root to one gate showed it:
the test failed 3 runs of 3 at `run_test.go:684` with
`shared coordination deferred admission: another admission is updating the shared root`, never at line 690.

**Why the success path does not wait.** When the roots do not contend, the gate is free and is taken by a non-blocking
`select` before any timer exists (lines 142–151, the ordering `d9ab21a` introduced), and `flock` succeeds on its first
attempt (lines 214–216). The success path waits on nothing, so its elapsed time is the runner's speed alone: directory
creation, file creation, `fstat`, `flock`, `close`, and scheduler delay, which the race detector and a loaded host
lengthen without bound. The reported 96.98 ms is past the 50 ms wait itself, which a contended acquisition could not
have survived.

**The defect.** The elapsed assertion (lines 689–691) adds nothing to serialization detection — line 684 already fails
on it — and adds a failure mode that measures the host: a constant chosen on an idle machine, the class
`scripts/test-loaded.sh` describes. Step 3 of the bug report makes it deterministic; step 2 reproduces it by in-process
contention alone.

## Solution

**Judge serialization by the refusal, with a zero wait.** The test acquires the second root with a wait of `0` and fails
on any error with `distinct-root coordination was serialized: <error>`; the clock and the elapsed assertion are removed.
A zero wait takes a free gate at once (lines 145–151) and refuses a held one at once (lines 152–156), and the lock loop
takes a free `flock` on its first attempt and refuses a held one at once (`wait == 0`, line 224). So the outcome is the
same however slowly the runner goes: the test passes if and only if the roots share neither gate nor lock, and a
serialization fails at once rather than after a 50 ms wait. Zero is a value production passes too, when a bounded wait
is spent (`internal/guard/reservation.go`, line 1208, and `internal/guard/lease.go`, line 459).

**A control in the same test.** After releasing the second root, the test acquires the held root with a zero wait and
requires a nil lock and `IsCoordinationDeferred(err)`, failing with
`a zero wait did not refuse the held root: lock=<lock> error=<error>`. The first assertion means "no contention" only
while a zero wait refuses a held lock; without the control, a change that made a zero wait return neither a lock nor an
error would pass the test vacuously, because `releaseCoordinationLock(nil)` returns `nil` (`coordination.go`, lines
191–193). The first root keeps its 1 s wait; a fresh root is always free, so that wait never runs.

A scratch copy of the test written this way, run on 2026-10-06 through build overlays: passed 3 of 3 alone and 3 of 3
with the 50 ms stall of bug-report step 3; failed 3 of 3 with `distinct-root coordination was serialized:` followed by
the coordination deferral when every root shared one gate, and the same when its second acquisition named the held root;
and failed 3 of 3 at the control with `lock=<nil> error=<nil>` when a zero wait returned nothing. The contention harness
of step 2, with a zero wait, refused none of 1,200 acquisitions.

**Conditions beyond the reported one.**

- `TestCoordinationLockUsesOneBoundedWaitBudget` (`run_test.go`, lines 706–725) fails if a refused 30 ms wait took more
  than 100 ms (line 722). There the elapsed time is the behaviour under test — one budget, not one per layer — so the
  argument above does not remove it, and its 70 ms margin is below the 96.98 ms of runner delay reported. It has not
  been seen failing (50 of 50 race-enabled runs at `GOMAXPROCS=1` on 2026-10-06, the slowest run 0.07 s) and is left as
  it is; a failure of it is a new defect.
- `TestCoordinationLockSerializesSameProcessByRoot` (lines 611–655) waits 1 s for a released lock to pass to its waiter
  (line 652); a late waiter only makes its 25 ms observation (line 639) pass. Not seen failing; left as it is.
- `TestCoordinationLockGrantsFreeRootWhenBudgetIsNearlySpent` (lines 661–672) already pins that a nearly spent budget
  takes a free root, which this fix relies on.

**Alternatives rejected.**

- _Widen the bound._ A wider constant is still chosen against one host's load; of retries and longer sleeps, #90 said
  "They lower the chance of a failure but do not remove it."
- _Drop only the elapsed assertion and keep the 50 ms wait._ Correct, but a serialization would then take 50 ms to
  report, and the verdict would still pass through a timer the test does not need.
- _Run the acquisition in a goroutine and fail if it does not return in time._ That reintroduces the wall clock.
- _Inject a clock into `acquireCoordinationLock`._ A production seam for a test-only defect, the ground on which #90
  rejected a product-side host-evidence seam.
- _Retry or quarantine the test._ It weakens a gate, which the upstream tool defects standard forbids.

**Public contract.** No exit status, `hippo.*` code, JSON document, configuration key, evidence shape, or production
code moves; the change is confined to `internal/guard/run_test.go`.

**Release content.** None. `CHANGELOG.md` must be true to the shipped binary
([documentation architecture](../../../repo-governance/conventions/documentation-architecture.md), line 19), and the
binary is unchanged. Earlier test-only repairs of the same class, #90 and #94 among them, carried no entry, nor does the
sibling degraded-lineage fix.

**References**, read 2026-10-06 from the Go 1.27.1 distribution's `go doc` and `go help`, and the macOS 15.5 manual:

- [Package testing: T.TempDir][go-tempdir] — each call returns a unique directory, so the two roots never share a lock
  file.
- [`flock(2)`, macOS][flock-darwin], and [`flock(2)`, Linux][flock-linux] — locks are on files, and `LOCK_NB` fails with
  `EWOULDBLOCK` instead of blocking, which is the refusal a contended zero-wait acquisition returns.
- [The Go Programming Language Specification: Select statements][go-select] — the uniform pseudo-random choice among
  ready cases, why `d9ab21a` takes a free gate before any timer exists.
- [Build flags: `-overlay`][go-build] — "a build will run as if the disk file path exists with the contents given by the
  backing file paths", so every mutation below leaves the worktree untouched.
- [Testing flags][go-testflag] and [package runtime: GOMAXPROCS][go-runtime] — `-count`, `-race`, `-timeout`, and the
  in-process contention the repeats use.
- `d9ab21a`, "fix(guard): take a free coordination gate before any bounded wait" (#78) — the ordering that makes the
  success path wait-free.
- #90 (`abc9094`), "test: make the wall-clock flaky tests deterministic", and #94 (`68cae05`), "test: close the
  remaining slow-runner flake risks" — the same class of defect in other tests, and their rejected alternatives.

### Specification Changes

None. No scenario or specification names this test, and `specs/architecture.md` lines 78–79 and 237 stay true.

### File Impact

```text
internal/guard/run_test.go                                        TestCoordinationLockAllowsDistinctRootsInParallel
plans/in-progress/fix-distinct-root-lock-test-flake/README.md     execution record
```

### Acceptance Criteria

- **AC-01** Given one root's lock held and a runner that stalls 50 ms in every lock acquisition, when
  `TestCoordinationLockAllowsDistinctRootsInParallel` acquires a second, distinct root, then it passes.
- **AC-02** Given every root sharing one in-process gate, when the test runs, then it fails at once with
  `distinct-root coordination was serialized:` followed by the coordination deferral.
- **AC-03** Given the test's second acquisition aimed at the held root, when it runs, then it fails with the same
  message.
- **AC-04** Given a zero wait that returns neither a lock nor an error, when the test runs, then it fails at its control
  with `a zero wait did not refuse the held root: lock=<nil> error=<nil>`.
- **AC-05** Given the fix branch, then `git diff --name-only origin/main...HEAD` lists only the File Impact paths, the
  test reads no clock, and nothing under `cmd/`, `specs/`, `docs/`, `README.md`, or `CHANGELOG.md`, nor any non-test
  file under `internal/`, changes.
- **AC-06** Given the host's load at execution, recorded with `uptime`, then the test passes 500 consecutive
  race-enabled runs at `GOMAXPROCS=1` and 500 at the default, the other `TestCoordinationLock` tests pass, the
  contention check refuses none of 1,000 zero-wait acquisitions, and the full gate passes on the fix branch's head.
- **AC-07** Given the fix merges, then either the published `v0.8.5` contains its merge commit, or, when `v0.8.5`
  shipped without it, the record shows that `origin/main` contains the merge commit and `v0.8.5` does not, and that the
  fix is test-only with no release content, so no patch release follows.
- **AC-08** Given execution finishes, then this plan is gated, reconciled, and archived under `plans/done/`, and its
  worktrees and branches are gone.

## Delivery

**Execution checkout.** `worktrees/fix-distinct-root-lock-test-flake` under the HIPPO repository location, created from
`origin/main` at `4cd530a` on branch `worktree/fix-distinct-root-lock-test-flake`, per
[worktree to pull request](../../../repo-governance/workflows/maintenance/worktree-to-pull-request.md). Unit 1 lands
from that branch. Unit 2 branches from `origin/main` in the same directory as
`worktree/fix-distinct-root-lock-test-flake-fix`. HIPPO cannot guard its own gates, so every command below runs
directly, never under `./hippo`, per
[resource-aware development](../../../repo-governance/development/resource-aware-development.md). No item starts CPU
load of its own; `scripts/test-loaded.sh` is not run, and contention is in-process only.

**Deviation, recorded.** The owner directed on 2026-10-06 that this worktree and its branches be removed immediately
after the fix merges. [Integration path](../../../repo-governance/conventions/integration-path.md) provisions at most
one worktree per plan and removes it once every unit has landed, and Unit 4 cannot land before `v0.8.5` exists. So Unit
4 provisions a second worktree, `worktrees/fix-distinct-root-lock-test-flake-record`, once `v0.8.5` is published, and
removes it after its own merge.

**Commands** the items below name, run from the worktree root. Proofs count Go's raw `-v` lines (`--- PASS`,
`--- FAIL`); where a shell hook rewrites `go test` output, bypass it, as the degraded-lineage plan learned
(`rtk proxy go test …`).

- _Lock test_: `go test -count=3 -v -run '^TestCoordinationLockAllowsDistinctRootsInParallel$' ./internal/guard`. "With
  an overlay" adds `-overlay <scratch>/<name>.json` before `-count`.
- _Neighbour tests_: `go test -race -count=3 -v -run '^TestCoordinationLock' ./internal/guard`.
- _Repeated test_: the _Lock test_ with `-race -count=500 -timeout 30m` in place of `-count=3`, run once with
  `GOMAXPROCS=1` before it and once without.
- _Scratch_: a directory made with `mktemp -d` at the start of the item that uses it and removed with `rm -r` at its
  end; proof of removal: `test ! -e <scratch>` exits `0`. Nothing in it is ever copied into the worktree.
- _Overlay_: `<scratch>/<name>.json` holding `{"Replace":{"<worktree>/<file>":"<scratch>/<copy>"}}`, with the worktree's
  absolute path; a `<file>` absent from disk is added to the build. The copies, each of the worktree's current file:
  - _Stall copy_ of `internal/guard/coordination.go`: `time.Sleep(50 * time.Millisecond)` as the first statement of
    `openCoordinationLock` (line 94).
  - _Shared-gate copy_ of `internal/guard/coordination.go`: line 118 becomes `identity := coordinationProcessIdentity{}`
    followed by `_, _ = device, inode`, so every root shares one gate.
  - _Zero-wait copy_ of `internal/guard/coordination.go`: `if wait == 0 { return nil, nil }` as the first statement of
    `acquireCoordinationLock` (line 201).
  - _Same-root copy_ of `internal/guard/run_test.go`, after Phase 2: the distinct-root acquisition in
    `TestCoordinationLockAllowsDistinctRootsInParallel` passes `held` in place of `t.TempDir()`.
  - _Contention harness_, added as `internal/guard/zz_distinct_root_contention_scratch_test.go` in package `guard`: a
    helper taking a wait `w` starts eight goroutines, each running 25 rounds that acquire a fresh `t.TempDir()` root
    with a 1 s wait, record `time.Now()`, acquire and release a second fresh `t.TempDir()` root with wait `w`, record
    the elapsed time, and release the first; under a mutex it counts rounds whose second acquisition or release returned
    an error (`refused`), and of the rest those of 40 ms or more (`over 40ms`), then logs
    `wait <w>: over 40ms <n> of 200, refused <m>, worst <elapsed>`. `TestContentionScratchToday` calls it with 50 ms and
    `TestContentionScratchZeroWait` with `0`.
- _Contention check_: with the _Contention harness_ overlay, run
  `GOMAXPROCS=1 go test -overlay <scratch>/contention.json -race -count=5 -timeout 30m -v -run`, the pattern
  `'^TestContentionScratchZeroWait$'`, and `./internal/guard`.
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
2. _Fix_ — the rewritten test and this plan's execution record. Rollback: revert its merge; no release depends on it.
3. _Release_ — `v0.8.5`, cut by the linting plan's Unit 7 after unit 2 merges; this plan only records it. That plan's
   Unit 7 holds the cut until this fix has merged, in its item "Before cutting, confirm every bug-fix plan this release
   carries has merged: `fix-cancelled-waiter-cleanup-flake`, `fix-degraded-lineage-scenario-flake`, and
   `fix-distinct-root-lock-test-flake` … wait for it rather than cut without it", which lands with that plan's Unit 5.
   If it does not land, or the tag is cut anyway, Phase 4's tag check and Phase 5's Recovery item cover it. A published
   tag is never replaced.
4. _Record_ — the release record, the execution check, and the move to `plans/done/`. Rollback: revert its merge.

**Out of scope: repinning consumers.** The binary is unchanged, so no consumer repins for this plan; the linting plan
leaves repinning `v0.8.5` to each consumer's own repository
([plan lifecycle](../../../repo-governance/conventions/plan-lifecycle.md)).

**Pause safety.** Each item records its result when ticked. To resume, read the last ticked item, then
`git -C <worktree> log --oneline origin/main..HEAD`, `gh pr list --head <branch>`, and
`git ls-remote --tags origin v0.8.5`. A _Scratch_ directory left by an interrupted item is removed and the item rerun.
Between unit 2's merge and unit 4, the record is this file on `origin/main`, with Phases 1–4 ticked and Phase 5 open.

### Phase 1: Plan

- [x] `[AI]` Land this file and its `plans/in-progress/README.md` entry alone with _Land_, from
      `worktree/fix-distinct-root-lock-test-flake`; proof: the merge commit on `origin/main` and _Reconcile_ reading
      `0 0`. `[AC-08]`
  - Result: (2026-10-06) pull request #138 rebase-merged as `33ca271` after `Quality gate` `success` on head `8e6ba28`
    and `leak-review` `success`; _Reconcile_ read `0 0`. The plan branch was then deleted locally; GitHub had already
    deleted it remotely.
- [x] `[AI]` Create the fix branch in the same directory: `git fetch origin --prune`, then
      `git switch -c worktree/fix-distinct-root-lock-test-flake-fix origin/main`, then `npm ci`; proof:
      `git branch --show-current` prints the branch and `git status --porcelain` prints nothing. `[AC-05]`
  - Result: (2026-10-07, recorded at the execution check) `worktree/fix-distinct-root-lock-test-flake-fix` from
    `origin/main` at `33ca271`, with `npm ci`: the gate commit `736c2f1`, the branch's first, has `33ca271` as its
    parent and was made on a clean tree.
- [x] `[AI]` Run the [plan quality gate](../../../repo-governance/workflows/quality/plan-quality-gate.md) on this folder
      in mode `normal`, at most three cycles, and commit its repairs and its verdict line as the fix branch's first
      commit, a `docs(plans)` commit; proof: one terminal `plan-quality-gate:` verdict line recorded here and
      `git status --porcelain` printing nothing. `[AC-08]`
  - Result: `plan-quality-gate: PASS_WITH_FINDINGS (1 cycle, 0 rows fixed; 1 LOW open)`. Entry checks: prettier,
    `markdownlint-cli2` 0 issues, `./rhino md internal-link validate` and `./rhino governance directory-map validate` no
    findings. RED, GREEN, and break overlays each reproduced 3 runs of 3. The open row, PQG-01 (delivery unit 3 said the
    linting plan's Unit 7 item omits this plan), was outdated by that item's later wording and is restated below in the
    sibling plans' form. Fix branch created at `33ca271` with `npm ci` under HIPPO.

### Phase 2: The Test Reads Serialization From the Refusal

- [x] `[AI]` RED: in a _Scratch_, write the _Stall copy_ and the _Shared-gate copy_ with an overlay for each, and run
      the _Lock test_ with each overlay, recording the output; proof: with the stall overlay it fails 3 runs of 3 at
      `run_test.go:690` with `distinct-root coordination was serialized:` and an elapsed time of at least 50 ms, past
      the error check at line 684, so the lock was taken; with the shared-gate overlay it fails 3 runs of 3 at
      `run_test.go:684` with `shared coordination deferred admission: another admission is updating the shared root` and
      never at line 690; `git status --porcelain` prints nothing; the _Scratch_ is removed. `[AC-01]` `[AC-02]`
  - Result: (2026-10-06) the _Lock test_ (`go test -overlay <scratch>/<name>.json -count=3 -timeout 30m -v -run`, the
    plan's pattern, `./internal/guard`) exited `1` with each overlay. Stall overlay: 3 of 3 `--- FAIL` (0.10–0.11 s),
    each at `run_test.go:690` with `distinct-root coordination was serialized:` and elapsed times of 52.184583 ms,
    57.12425 ms, and 51.710542 ms, all past the line-684 error check, so the lock was taken. Shared-gate overlay: 3 of 3
    `--- FAIL` (0.05 s), each at `run_test.go:684` with
    `shared coordination deferred admission: another admission is updating the shared root`, none at line 690.
    `git status --porcelain` printed nothing before this record; the _Scratch_ was removed (`test ! -e` exits `0`). Load
    average 13.50 at the start.
- [x] `[AI]` GREEN: in `internal/guard/run_test.go`, `TestCoordinationLockAllowsDistinctRootsInParallel`, name the first
      root `held := t.TempDir()`; delete `start` (line 681) and the elapsed assertion (lines 689–691); acquire the
      second root with `acquireCoordinationLock(context.Background(), t.TempDir(), 0)`, failing on an error with
      `t.Fatalf("distinct-root coordination was serialized: %v", err)`, and keep its release; then acquire `held` with a
      zero wait as `contended` and, unless `contended` is nil and `IsCoordinationDeferred(err)` holds, release any lock
      and fail with `t.Fatalf("a zero wait did not refuse the held root: lock=%v error=%v", contended, err)`. In a
      _Scratch_, write the _Stall copy_ and its overlay; proof: the _Lock test_ passes 3 runs of 3 without an overlay
      and 3 of 3 with the stall overlay, the _Neighbour tests_ pass, and the _Scratch_ is removed. `[AC-01]`
  - Result: (2026-10-06) the test body is rewritten as the item says (`held`, a zero-wait second root failing with
    `distinct-root coordination was serialized: %v`, then the zero-wait `contended` control). The _Lock test_ passed 3
    of 3 without an overlay (0.00 s each, exit `0`) and 3 of 3 with the stall overlay (0.16 s each, exit `0`, the three
    50 ms stalls inside the test and no failure). The _Neighbour tests_ (`go test -race -count=3 -timeout 30m -v -run`
    `'^TestCoordinationLock'`) exited `0` with 15 `--- PASS` lines, each of the 5 tests 3 of 3, and no `--- FAIL`. The
    _Scratch_ was removed (`test ! -e` exits `0`). Load average 10.55 at the start.
- [x] `[AI]` REFACTOR: give the test a comment saying distinct roots must not contend, that a zero wait takes a free
      lock at once and refuses a held one at once, and that the verdict therefore comes from the refusal and never from
      the runner's speed; proof:
      `awk '/^func TestCoordinationLockAllowsDistinctRootsInParallel/,/^}/' internal/guard/run_test.go` piped to
      `grep -cE 'time\.(Now|Since)'` prints `0`, `go tool golangci-lint run ./internal/guard/...` prints `0 issues.`,
      `gofmt -l internal/guard` prints nothing, and the _Lock test_ and _Neighbour tests_ still pass. `[AC-05]`
  - Result: (2026-10-06) a four-line comment above the test says distinct roots must not contend, that a zero wait takes
    a free lock and refuses a held one at once so the verdict comes from the refusal and never from the runner's speed,
    and that the control keeps the first check honest. The `awk` and `grep -cE` pipeline prints `0`;
    `go tool golangci-lint run ./internal/guard/...` prints `0 issues.` (with its usual warning that `nilaway` is an
    unknown `//nolint` linter, which predates this change); `gofmt -l internal/guard` prints nothing; the _Lock test_
    passed 3 of 3 and the _Neighbour tests_ 15 of 15 (5 tests, 3 passes each, `-race`), both exit `0`. Load average
    10.50 at the start.
- [x] `[AI]` Commit `internal/guard/run_test.go` alone as
      `test(guard): judge distinct-root coordination by refusal, not the clock`; proof:
      `git log --oneline origin/main..HEAD` lists it after the gate commit, and `git diff --name-only HEAD -- internal`
      prints nothing. `[AC-05]`
  - Result: (2026-10-06) `946dd7f` `test(guard): judge distinct-root coordination by refusal, not the clock`, one file
    changed (15 insertions, 6 deletions), through the pre-commit and `commit-msg` gates with no bypass.
    `git log --oneline origin/main..HEAD` lists `946dd7f` above the gate commit `736c2f1`, and
    `git diff --name-only HEAD -- internal` prints nothing.

### Phase 3: Review and Documentation

- [x] `[AI]` Break tests: in a _Scratch_, write the _Shared-gate copy_, the _Same-root copy_, and the _Zero-wait copy_
      with an overlay for each, and run the _Lock test_ with each; proof, each recorded here: the shared-gate and
      same-root overlays each fail 3 runs of 3 with `distinct-root coordination was serialized:` followed by
      `shared coordination deferred admission: another admission is updating the shared root`; the zero-wait overlay
      fails 3 of 3 with `a zero wait did not refuse the held root: lock=<nil> error=<nil>`;
      `git diff --quiet -- internal` exits `0`; the _Scratch_ is removed. `[AC-02]` `[AC-03]` `[AC-04]`
  - Result: (2026-10-06) each overlay was built from the committed files, and each run was the _Lock test_ with
    `-overlay <scratch>/<name>.json`, `-timeout 30m`, and exit `1`. Shared-gate overlay (`coordination.go` line 118
    keying every root to one gate): 3 of 3 `--- FAIL` (0.00 s) at `run_test.go:688` with
    `distinct-root coordination was serialized:` followed by
    `shared coordination deferred admission: another admission is updating the shared root`. Same-root overlay
    (`run_test.go` with `held` in place of the second `t.TempDir()`): 3 of 3 `--- FAIL` (0.00 s), the same line and
    message. Zero-wait overlay (`if wait == 0 { return nil, nil }` first in `acquireCoordinationLock`): 3 of 3
    `--- FAIL` (0.00 s) at `run_test.go:699` with `a zero wait did not refuse the held root: lock=<nil> error=<nil>`.
    `git diff --quiet -- internal` exits `0`; the _Scratch_ was removed (`test ! -e` exits `0`). Load average 9.51 at
    the start.
- [x] `[AI]` Run [docs propagation](../../../repo-governance/workflows/quality/docs-propagation.md) and record that no
      page describes this test and that, per [Release content](#solution), `CHANGELOG.md` gets no entry; proof:
      `git grep -n 'AllowsDistinctRoots' -- README.md docs specs CHANGELOG.md repo-governance` prints nothing, and
      `git diff --name-only origin/main...HEAD -- README.md docs specs CHANGELOG.md` prints nothing. `[AC-05]`
  - Result: (2026-10-06) no page describes this test: `git grep -n 'AllowsDistinctRoots' -- README.md docs specs`
    `CHANGELOG.md repo-governance` prints nothing (exit `1`), and neither does a wider
    `git grep -n -iE 'distinct[- ]roots?|elapsed.*serializ'` over the same paths. `git diff --name-only`
    `origin/main...HEAD -- README.md docs specs CHANGELOG.md` prints nothing. Per [Release content](#solution) the
    test-only change is true to the shipped binary, so `CHANGELOG.md` gets no entry. Status `no-change`: nothing stale,
    nothing removed, no command in an affected document to run.

### Phase 4: Verification

- [x] `[AI]` Bounded checkpoint: run both forms of the _Repeated test_, then the _Neighbour tests_, recording `uptime`
      before and after each; proof: each exits `0`, with 500 `--- PASS` lines for the test in each _Repeated test_ form
      and no `--- FAIL`. Fallback, decided now: one failure stops the plan before landing; its output is recorded here,
      the cause it shows replaces the matching part of [Root Cause](#root-cause), and nothing lands until a new RED
      proves that cause. A failure of a neighbour test alone is recorded here and filed as its own bug-fix plan under
      the owner's standing request; this plan lands only after that fix merges and a rebase onto it reruns the
      _Neighbour tests_ clean. `[AC-06]`
  - Result: (2026-10-06) at `946dd7f`, run directly, each exit `0`. _Repeated test_ with `GOMAXPROCS=1`
    (`go test -race -count=500 -timeout 30m -v -run '^TestCoordinationLockAllowsDistinctRootsInParallel$'`
    `./internal/guard`): 500 `--- PASS` lines, 0 `--- FAIL`, the slowest pass 0.01 s, `ok` in 1.696 s; `uptime` before
    and after: load averages 11.36 15.43 19.25 and 14.45 16.00 19.43. _Repeated test_ at the default `GOMAXPROCS`: 500
    `--- PASS`, 0 `--- FAIL`, the slowest pass 0.03 s, `ok` in 2.464 s; load averages 14.45 16.00 19.43 before and 14.49
    15.98 19.40 after. _Neighbour tests_ (`-race -count=3`): 15 `--- PASS`, 3 of 3 for each of the 5
    `TestCoordinationLock` tests, 0 `--- FAIL`, `ok` in 1.475 s; load averages 14.49 15.98 19.40 before and after (the
    kernel's average had not moved within the run). The fallback did not fire.
- [x] `[AI]` In a _Scratch_, write the _Contention harness_ and its overlay as `contention.json`, and run the
      _Contention check_, recording `uptime` before and after; proof: exit `0` and five log lines each reading
      `refused 0`, and the _Scratch_ is removed. Fallback, decided now: as for the checkpoint above. `[AC-06]`
  - Result: (2026-10-06) the _Contention harness_ (eight goroutines, 25 rounds each, a 1 s first root, a timed second
    root with wait `w`) was added to package `guard` by overlay, never to the worktree. The _Contention check_
    (`GOMAXPROCS=1 go test -overlay <scratch>/contention.json -race -count=5 -timeout 30m -v -run`
    `'^TestContentionScratchZeroWait$' ./internal/guard`) exited `0` with five lines, each
    `wait 0s: over 40ms 0 of 200, refused 0`, with worst rounds of 1.482208 ms, 22.651042 ms, 235.917 µs, 183.125 µs,
    and 37.660541 ms: 1,000 zero-wait acquisitions, none refused. `uptime` before: load averages 17.03 16.42 19.42;
    after: 15.98 16.21 19.33. Controls, not required by the item: the same harness with the _Shared-gate copy_ counted
    `refused 200` of 200 in one run, so the counter sees a refusal; `TestContentionScratchToday` (50 ms wait, bug-report
    step 2, `-count=5`) counted `over 40ms` 0, 0, 1, 0, and 0 of 200 with no refusal, the one at 40.154583 ms. The
    _Scratch_ was removed (`test ! -e` exits `0`) and `git status --porcelain` shows only this README.
- [x] `[AI]` Run the _Full gate_ on the branch head; proof: exit `0`, ending with `No vulnerabilities found.` `[AC-06]`
  - Result: (2026-10-06) `GOFLAGS=-timeout=30m npm test` at `946dd7f`, the form the repository's loaded-host runner
    uses, exit `0` (`uptime` load 13.07 before, 7.41 after): selected production line coverage 99.35% (911/917), race
    detector clean, ending with "No vulnerabilities found."

- [x] `[AI]` Before landing, commit the execution record so far as a `docs(plans)` commit on the fix branch, because
      `git rebase` refuses a dirty tree and the rebase never auto-stashes; then `git fetch origin --tags` and confirm
      `v0.8.5` does not yet exist; then rebase onto `origin/main`, reading the whole incoming diff (the linting plan's
      in-flight Unit 5 adds tests to `internal/guard/run_test.go`, and the cancelled-waiter fix adds one beside this
      test, at line 706), and rerun the _Lock test_, the _Neighbour tests_, and the _Full gate_ if the rebase brought
      commits; proof: `git status --porcelain` prints nothing before the rebase, `git ls-remote --tags origin v0.8.5`
      prints nothing, and the reruns exit `0`. If the tag already exists, land anyway and the Recovery item in Phase 5
      fires. `[AC-06]` `[AC-07]` `[AC-08]`
  - Result: (2026-10-07) the record so far was committed as `7a262ef` on a clean tree, and
    `git ls-remote --tags origin v0.8.5` printed nothing. Main had gained the linting plan's Unit 5 (`e261965`), the
    cancelled-waiter fix (`aa274ee`, adding its test beside this one), and the degraded-lineage fix (`c2a08ca`), so the
    branch was rebased without conflict to `1221319`: the _Lock test_ passed 3 of 3, the _Neighbour tests_ passed under
    `-race`, and the _Full gate_ exited `0` at load 6.5–14.3 with selected production line coverage 99.36% (928/934),
    race detector clean, ending with "No vulnerabilities found."
- [x] `[AI]` Confirm the change stays inside its boundary; proof: `git diff --name-only origin/main...HEAD` prints only
      the paths in [File Impact](#file-impact). `[AC-05]`
  - Result: (2026-10-07) `git diff --name-only origin/main...HEAD`, against `origin/main` at `c2a08ca` with head
    `1221319`, prints `internal/guard/run_test.go` and this README: exactly the File Impact paths.
- [x] `[AI]` Commit the rest of this plan's execution record (Phases 1–4 ticked with the rebase, rerun, and boundary
      results) as a `docs(plans)` commit on the fix branch, as the last commit before landing; proof:
      `git status --porcelain` prints nothing. `[AC-08]`
  - Result: (2026-10-07, recorded at the execution check) committed as `0876fa6`, #143's head and last commit, on a
    clean tree; it landed as `4e1e7d5`. Recorded here because the commit cannot hold its own hash.

### Phase 5: Release Through v0.8.5

- [x] `[AI]` Land unit 2 with _Land_, its pull-request body carrying the RED and GREEN captures per
      [red-green-refactor](../../../repo-governance/workflows/quality/red-green-refactor.md); proof: the merge commit on
      `origin/main` and _Reconcile_ reading `0 0`, both recorded here by unit 4, since the merged copy cannot hold its
      own merge. The fix must merge before the linting plan's Unit 7 tags `v0.8.5`. `[AC-01]` `[AC-02]` `[AC-03]`
      `[AC-04]` `[AC-05]` `[AC-06]`
  - Result: (2026-10-07) pull request #143, its body carrying the RED and GREEN captures under "How it was proved",
    rebase-merged as `4e1e7d5` (2026-10-06T18:46:09Z), the fix commit landing as `23b21ed`, after `Quality gate`
    `success` on head `0876fa6` in `PR Quality Gate` run 37511082174 (an earlier run on the same head, 37511069618, was
    cancelled and superseded) and the leak review's pass for that exact head, with `leak-review` `success`. The fix
    merged before `v0.8.5` was tagged. _Reconcile_ read `0 0`.
- [x] `[AI]` Immediately after that merge, run
      [dev artifact clean-up](../../../repo-governance/workflows/maintenance/dev-artifact-clean-up.md) for this
      worktree, at the owner's direction: confirm nothing is unpushed or running, then remove the worktree with
      `git worktree remove` without `--force`, and delete `worktree/fix-distinct-root-lock-test-flake` and `...-fix`
      locally and on `origin`; proof: `git worktree list` omits it, `git branch --list 'worktree/fix-distinct-root-*'`
      and `git ls-remote origin 'refs/heads/worktree/fix-distinct-root-*'` print nothing. `[AC-08]`
  - Result: (2026-10-07) right after the merge, with nothing unpushed and nothing running in it, the worktree was
    removed with `git worktree remove` without `--force`, and both branches were deleted locally; GitHub had already
    deleted them on `origin`. Rerun from the record worktree: `git worktree list` omits it,
    `git ls-remote origin 'refs/heads/worktree/fix-distinct-root-*'` prints nothing, and
    `git branch --list 'worktree/fix-distinct-root-*'` lists only `worktree/fix-distinct-root-lock-test-flake-record`,
    which the next item provisioned afterwards; `git branch --list` naming the two deleted branches prints nothing.
- [x] `[AI]` Provision `worktrees/fix-distinct-root-lock-test-flake-record` from `origin/main` on branch
      `worktree/fix-distinct-root-lock-test-flake-record`, with `npm ci`, once `v0.8.5` is published; proof:
      `git branch --show-current` prints the branch. `[AC-08]`
  - Result: (2026-10-07) after `git fetch origin --prune`, `git worktree add -b` created the branch and worktree from
    `origin/main` at `456d24b`, and `npm ci` ran under `./hippo run --class ephemeral --resource-tier light`, exit `0`.
    `git branch --show-current` prints `worktree/fix-distinct-root-lock-test-flake-record`.
- [x] `[AI]` Once `v0.8.5` is published, record the release in the record worktree's copy of this plan; proof:
      `git merge-base --is-ancestor <unit 2 merge commit> v0.8.5` exits `0`, and the release URL and the tag's peeled
      commit are recorded here. `[AC-07]`
  - Result: (2026-10-07) `v0.8.5` is published at <https://github.com/wahidyankf/hippo/releases/tag/v0.8.5>
    (2026-10-06T22:19:10Z, neither draft nor prerelease, four archives and `checksums.txt`), built by `release.yml` run
    37539705716, `success`. The annotated tag object `f24bba1` peels (`git rev-parse v0.8.5^{commit}`) to
    `456d24bc2a20ee23a7b81746c1789133cd5c3a97`, the merge of #147. `git merge-base --is-ancestor 4e1e7d5 v0.8.5` exits
    `0`, so the release's full gate ran with the fix. Per [Release content](#solution), it carries no `CHANGELOG.md`
    entry.
- [ ] `[AI]` Recovery, dormant until triggered. Trigger: `v0.8.5` is published without unit 2's merge commit (the
      ancestry check above exits `1`). Then cut no patch release, because the binary is unchanged: record in the record
      worktree's copy of this plan that the fix is test-only with no release content and that `v0.8.5`'s full gate ran
      without it, with the result the linting plan's Unit 7 recorded for it; proof:
      `git merge-base --is-ancestor <unit 2 merge commit> origin/main` exits `0` and the `v0.8.5` check exits `1`, both
      recorded there, so the next release cut from `origin/main` carries the fix. Otherwise: a dated, evidenced
      `Not triggered`. `[AC-07]`
  - Not triggered (2026-10-07): `git merge-base --is-ancestor 4e1e7d5 v0.8.5` exits `0`, as does the same check against
    `origin/main`, so `v0.8.5` carries unit 2 and there is nothing to recover.

### Phase 6: Close

- [x] `[AI]` Route each learning below to its durable owner, or discard it with a reason; proof: each entry names its
      owner or its reason. `[AC-08]`
  - Result: (2026-10-07) [Learnings](#learnings) held no entry when first routed; the execution check's F2 then added
    one, the `npm ci` deviation, discarded with its reason.
- [x] `[AI]` Run the [execution check](../../../repo-governance/workflows/plan/plan-execution-check.md); proof: its
      verdict line recorded here. `[AC-08]`
  - Result: (2026-10-07) `plan-execution-check: PASS_WITH_FINDINGS (1 run, 5 LOW, 3 repaired, 1 closed by archival)`, at
    `589a190`. F1 (LOW: Phase 1's fix-branch item and the last Phase 4 item had no result line), F2 (LOW: `npm ci` under
    `./hippo` unrecorded as a deviation), and F5 (LOW: the boundary result's commit read ambiguously) are repaired in
    this commit; F3 (LOW: the leak review was posted before `Quality gate` finished, with every merge precondition
    holding at the merge) needs no repair; F4 closes with the archival items.

### Archival

- [ ] `[AI]` Move this folder with `git mv` to `plans/done/<completion date>__fix-distinct-root-lock-test-flake/`,
      update `plans/in-progress/README.md` and `plans/done/README.md` in the same change, and land it with _Land_;
      proof: the merge commit, posted on the archival pull request, and no copy left under `plans/in-progress/`.
      `[AC-08]`
- [ ] `[AI]` Run [dev artifact clean-up](../../../repo-governance/workflows/maintenance/dev-artifact-clean-up.md) for
      the record worktree right after that merge; proof, posted on the archival pull request: `git worktree list` omits
      it, no local or remote `worktree/fix-distinct-root-*` branch remains, and _Reconcile_ reads `0 0`. `[AC-08]`

## Learnings

- (2026-10-07) Every RED, GREEN, break, repeated, and contention proof came out as [Root Cause](#root-cause) predicted.
  One deviation: `npm ci`, a dependency install rather than a HIPPO gate, ran under `./hippo` twice (the Phase 1 gate
  result and the Phase 5 provision result), against the Delivery rule that every command runs directly, because the
  workstation's outer guard wraps dependency installs. **Resolved (2026-10-07):** discarded; the rule concerns HIPPO's
  own gates, which all ran directly, and the same observation was discarded in
  [the configured-profiles fix](../../done/2026-10-03__fix-configured-profiles-starve-under-macos-warning/README.md).

## Directory Map

This plan is one document, so this README has no siblings to map.

[go-tempdir]: https://pkg.go.dev/testing#T.TempDir
[go-select]: https://go.dev/ref/spec#Select_statements
[go-build]: https://pkg.go.dev/cmd/go#hdr-Compile_packages_and_dependencies
[go-testflag]: https://pkg.go.dev/cmd/go#hdr-Testing_flags
[go-runtime]: https://pkg.go.dev/runtime
[flock-darwin]:
  https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/flock.2.html
[flock-linux]: https://man7.org/linux/man-pages/man2/flock.2.html
