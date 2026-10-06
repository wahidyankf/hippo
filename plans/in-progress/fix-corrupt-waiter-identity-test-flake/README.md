# Fix: Corrupt-Waiter Identity Test Flakes Under Load

Status: In progress (2026-10-06)

On a heavily loaded host, the waiter rows of the unit test `TestReservationIdentityPathCorruptionFailClosed` fail
intermittently with "competing admission bypassed corrupt live identity" although nothing was bypassed: the competing
admission never reached the ledger. The test makes it with a zero wait while its own waiter goroutine keeps retrying,
and when that goroutine holds the shared root's coordination lock at that instant, the admission is refused at the lock
with a coordination deferral, which the test does not accept. The defect is in the test alone; both refusals are
specified deferrals that callers see identically.

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
plans [`fix-cancelled-waiter-cleanup-flake`](../fix-cancelled-waiter-cleanup-flake/README.md),
[`fix-degraded-lineage-scenario-flake`](../fix-degraded-lineage-scenario-flake/README.md), and
`fix-distinct-root-lock-test-flake` (pull request #138).

**Release.** The change is test-only, so it carries no release content of its own: no `CHANGELOG.md` entry and no tag.
It must merge before the in-flight plan
[strict Go linting and domain modeling](../strict-go-linting-and-domain-modeling/README.md) cuts `v0.8.5` in its Unit 7,
so that cut's full gate runs with it (see [Phase 5](#phase-5-release-through-v085)).

Line numbers in this plan are at `4cd530a`, the trunk commit it was written from, unless a sentence names another commit
or tag.

## Bug Report

**Description.** Under host load, the `waiter/missing` and `waiter/replaced` subtests of
`TestReservationIdentityPathCorruptionFailClosed` fail their competing-admission assertion because the zero-wait
admission is refused at the coordination lock, which the test's own queued waiter holds, instead of being deferred by
the ledger.

**Steps to reproduce**, from a clean checkout of `wahidyankf/hippo` at `4cd530a` with `npm ci` run:

1. _As observed:_ run `GOFLAGS=-timeout=30m npm test` on a host whose load average stays well above its core count. The
   test fails in some runs of the race-enabled step (`scripts/test.sh`, line 20), and passes on rerun. The same test
   also runs, as a `go test` subprocess, under the waiter rows of the scenario outline "Reservation identity path
   corruption remains fail closed" (`specs/behaviours/reservations.feature`, lines 356–367).
2. _Under in-process contention:_ `GOMAXPROCS=2 go test -race -count=150 -timeout 30m -run`, the pattern
   `'TestReservationIdentityPathCorruptionFailClosed/waiter'`, and `./internal/guard`.
3. _Deterministically:_ through a Go build overlay (the _Stall copy_ under [Delivery](#delivery)), add
   `time.Sleep(50 * time.Millisecond)` right after the coordination lock is taken in the admission loop of
   `AcquireReservationWithOptions` (after `internal/guard/reservation.go`, line 1212) and in `ReservationStatus` (after
   line 1575). It stands in for a runner the scheduler starves while holding the lock and serializes nothing new. Then
   run `go test -overlay <file> -count=3 -v -run 'TestReservationIdentityPathCorruptionFailClosed' ./internal/guard`.

**Expected behaviour.** The scenario outline's `When` step says "status and competing admission reconcile that ledger"
(`reservations.feature`, line 359), so the competing admission is expected to read the ledger and be deferred by it with
`ErrReservationDeferred`, as the test asserts (`internal/guard/run_test.go`, lines 2143–2145). The coordination lock is
a short critical section around each ledger transaction, not the property under test.

**Actual behaviour.** The full gate on branch `worktree/fix-cancelled-waiter-cleanup-flake-fix`, at a load average near
27, failed in its race-enabled step with the output below, reported by the session executing that plan on 2026-10-06.
That branch leaves this test's body unchanged (it sits 37 lines lower there, at line 2181 of `69e196b`), and its cleanup
change runs only after cancellation, which this assertion precedes (line 2146).

```text
run_test.go:2181: competing admission bypassed corrupt live identity: session=false error=
  shared coordination deferred admission: another admission is updating the shared root
FAIL github.com/wahidyankf/hippo/internal/guard 24.378s
```

Each quoted failure is one line in the output, wrapped here after `error=` to fit. Step 2, run the same day by the
sessions executing the sibling plans, failed 4 runs of 150 on `4cd530a` with only a plan added (the
`fix-distinct-root-lock-test-flake` worktree) and 2 of 150 on the cancelled-waiter fix branch. Run by this plan's author
on 2026-10-06 at load averages of 9–24, it passed 300 subtests of 300 at `GOMAXPROCS=2` and 300 of 300 at
`GOMAXPROCS=1`: the failure needs more load than in-process contention alone supplies. Step 3 failed every waiter
subtest, 6 of 6 at `-count=3` and 40 of 40 at `-count=20`, each with the reported message at `run_test.go:2144`, and
passed every owner subtest:

```text
run_test.go:2144: competing admission bypassed corrupt live identity: session=false error=
  shared coordination deferred admission: another admission is updating the shared root
--- FAIL: TestReservationIdentityPathCorruptionFailClosed/waiter/missing (0.19s)
```

**Error output** is the `run_test.go` line quoted above, from the assertion at lines 2143–2145.

**Environment.** macOS 15.5 on Apple M2 Max with 12 cores, Go 1.27.1 (the local toolchain; `go.mod` requires 1.26.1),
Node 24.16.0, at `4cd530a` (`origin/main`).

**Not a regression.** The test dates from `ac76575` (2026-09-05, pull request #3), which introduced reservation
coordination, and `git log -L2086,2174:internal/guard/run_test.go` shows no later change. The `v0.8.4` tag carries the
same body (its assertion at line 1716); `git diff v0.8.4 4cd530a` leaves `internal/guard/coordination.go` unchanged and
touches none of the admission loop's lock, write, release, or pause lines in `internal/guard/reservation.go`.

## Duplicate Check

Run 2026-10-06 against `origin/main` at `4cd530a`:

- `gh issue list --state all` returned no issues. `gh issue list --state all --search` for `flake`, `flaky`, `corrupt`,
  `corrupt live identity`, `IdentityPathCorruption`, `competing admission`, `waiter`, and `coordination deferred` each
  returned nothing.
- `gh pr list --state open` returned only #138, the `fix-distinct-root-lock-test-flake` plan.
  `gh pr list --state all --search` for `IdentityPathCorruption`, `competing admission`, and `errCoordinationDeferred`
  returned nothing; for `corrupt live identity` only #3, which introduced the test; for `flake`, `flaky`, `corrupt`, and
  `coordination deferred` only merged or closed work that leaves this test unchanged: #90 and #94 repaired other
  wall-clock tests, #74, #76, and #78 are the coordination-gate fix `d9ab21a`, #18 hardened contended fixtures under
  `tests/support/`, #136 and #137 are two sibling plans, and the rest are features, releases, and other fixes.
- `plans/backlog/` holds no plan; `plans/in-progress/` holds the linting plan and two sibling bug-fix plans; and
  `plans/ideas/` holds one brief, the degraded-admission release-audit follow-ups.
- `grep -rliE` over `plans/` outside `done/`, for one alternation of `IdentityPathCorruption`, `corrupt live identity`,
  `competing admission`, `errCoordinationDeferred`, and `corrupt`, matched only the linting plan, on `corrupt` alone
  (ledger and class corruption, a different subject). Inside `plans/done/` the same terms found nothing. Over
  `repo-governance/`, `specs/`, and `docs/`, the first four matched only `specs/behaviours/reservations.feature`, the
  outline this test binds.
- `git log -i -F --grep` for `corrupt live identity`, `IdentityPathCorruption`, `competing admission`, and
  `errCoordinationDeferred` found nothing.
- No web search tool was available to this session, so no search for outside reports was made; the defect is in HIPPO's
  own test, which the searches above cover.

No duplicate exists. Related, not duplicates: the sibling plans fix the same class of defect — a fixture whose verdict
depends on the scheduler, the class `scripts/test-loaded.sh` describes (lines 7–16) — in other fixtures, as #90
(`abc9094`) and #94 (`68cae05`) did before them.

## Root Cause

**What the test does.** Each subtest fills the one-CPU capacity with an owner (`run_test.go`, lines 2095–2100). In the
waiter rows it starts a goroutine that calls `AcquireReservation` for a service-class waiter with a 3 s wait (lines
2105–2113), then polls the ledger until the waiter appears, for up to 1 s on the wall clock (lines 2114–2125). It
corrupts the waiter's identity path (line 2127), reads the ledger bytes (line 2129), calls `ReservationStatus` (line
2133), and makes a competing ephemeral admission with a zero wait (lines 2137–2139), requiring it to fail with
`ErrReservationDeferred` (lines 2143–2145). Only then does it cancel the waiter (lines 2146–2151).

**The waiter keeps taking the lock.** A queued waiter that is not admitted repeats one transaction: it takes the
coordination lock (`internal/guard/reservation.go`, line 1209), reads and reconciles the ledger, writes it, releases the
lock (lines 1301–1302), and pauses `coordinationPollInterval`, 10 ms (`internal/guard/coordination.go`, line 23), in
`waitForReservationRetry` (`reservation.go`, lines 1094–1115, called at line 1338). So for the whole of the test's
corrupt window the waiter is in the same process, taking and releasing the root's lock about every 10 ms.

**A zero wait is refused at a held lock.** `acquireCoordinationLock` first takes the in-process gate for the lock file
(`coordination.go`, lines 102–174). A free gate is taken at once (lines 145–151); with a zero wait a held one is refused
at once with `errCoordinationDeferred` (lines 152–156). The gate is held from before the `flock` until after its unlock
(lines 208 and 194–196), so within one process it is the gate, not the `flock` (line 228), that refuses. The admission
loop returns that refusal before reading the ledger (`reservation.go`, lines 1209–1212). A scratch overlay that tagged
the three `errCoordinationDeferred` returns, run with the stall of bug-report step 3, failed 40 of 40 waiter subtests,
each with the tag of line 155.

**Why the test does not accept it.** `errCoordinationDeferred` (`coordination.go`, line 36) and `ErrReservationDeferred`
(`reservation.go`, line 38) are distinct errors, and `errors.Is(blockedError, ErrReservationDeferred)` (`run_test.go`,
line 2143) holds only for the second. The owner rows never fail this way: they start no waiter, so nothing else in the
process holds the lock.

**When the waiter holds it.** `ReservationStatus` waits up to 2 s for the lock (`reservation.go`, line 1572;
`coordination.go`, line 33). If it holds the lock past the waiter's 10 ms pause — as the race detector and a loaded host
make likely — the waiter is waiting at the gate when status releases it, takes it, and holds it through its own
transaction while the test goroutine moves on to the competing admission. Any scheduler delay of the test goroutine
between line 2133 and line 2137 widens the same window. Step 3 of the bug report makes both certain: a 50 ms hold in
status queues the waiter at the gate, and a 50 ms hold in its own transaction keeps the gate taken.

**Not a product defect.** Both refusals are specified deferrals and reach a caller identically. `Run` maps either to
`policy.ReasonCapacityDeferred` (`internal/guard/run.go`, lines 660–669); the boundary reports that reason as
`hippo.limit.capacity-deferred` with status `124` and one message (`internal/cli/status.go`, lines 23–28 and 116–121);
and both write the same never-started receipt with reason `admission-deadline` (`reservation.go`, lines 1316–1323 for
the ledger, 1355–1380 for the lock). `specs/behaviours/reservations.feature` specifies the lock refusal itself: "with
the coordination lock held by another admission, the same wait is deferred for coordination" (line 73), and "A
coordination lock held through the whole wait still leaves a never-started receipt" (lines 75–79). Only the
informational stderr note differs, naming its cause (`run.go`, lines 661 and 666); notes are not in the
[public contract](../../../repo-governance/development/public-contract.md).

**The defect.** The test's verdict depends on whether its own waiter goroutine is inside a lock transaction at the
instant of the zero-wait admission, which the scheduler decides. A lock refusal there says nothing about the corrupt
identity, because the admission never read the ledger; the test is right to reject it, and wrong to let it happen.

## Solution

**Park the waiter outside the lock.** The waiter is started with `AcquireReservationWithOptions` and a `Pause` seam
(`ReservationAdmissionOptions`, `reservation.go`, lines 47–53) that closes a `parked` channel and blocks until the
waiter's context is cancelled, returning `ctx.Err()`. The test waits for `parked` instead of polling the ledger, then
reads the waiter's token from the ledger once. The pause runs only from line 1338, after the transaction's write and
`releaseCoordinationLock` (lines 1301–1302), so a parked waiter holds neither gate nor `flock`. Nothing else in the
process touches the `t.TempDir()` root, so the competing admission takes a free gate at once (`coordination.go`, lines
145–151) and a free `flock` on its first attempt, reads and reconciles the ledger, and is deferred by it — however
slowly the runner goes. The `ErrReservationDeferred` expectation is unchanged, so a lock refusal still fails the test.

Cancellation is unchanged in effect: the pause returns `context.Canceled`, the loop writes its never-started receipt and
returns it (lines 1338–1345), and the deferred cleanup removes the waiter (lines 1190–1202), as line 2148 requires. The
`Pause` seam is how production waits too (`run.go`, lines 611–618), and `tests/unit/reservation_test.go`, lines 362–378,
already drives the loop through it.

The new body of the `if record == "waiter"` block, replacing lines 2105–2125:

```go
waiterContext, cancel := context.WithCancel(context.Background())
t.Cleanup(cancel)
waiterCancel = cancel
waiterDone = make(chan error, 1)
// The waiter parks in its retry pause, which it reaches only after
// releasing the coordination lock, so the competing admission below
// meets a free lock and is decided by the ledger alone.
parked := make(chan struct{})
options := ReservationAdmissionOptions{Pause: func(pauseContext context.Context, _ time.Duration) error {
	close(parked)
	<-pauseContext.Done()

	return pauseContext.Err()
}}
go func() {
	_, waiterError := AcquireReservationWithOptions(
		waiterContext, root, "", policy.TaskService, "minimal", "waiter", plan, 1, guardLivenessLimit, options,
	)
	waiterDone <- waiterError
}()
select {
case <-parked:
case waiterError := <-waiterDone:
	t.Fatalf("waiter returned before parking: %v", waiterError)
case <-time.After(guardLivenessLimit):
	t.Fatal("waiter did not park")
}
ledger, readError := readReservationLedger(root)
if readError != nil || len(ledger.Waiters) != 1 {
	t.Fatalf("waiter did not enqueue: waiters=%d error=%v", len(ledger.Waiters), readError)
}
value = ledger.Waiters[0].Token
```

`close(parked)` runs once: the pause returns only an error, which ends the loop. A scratch copy of the test written this
way, run on 2026-10-06 through build overlays: passed 3 runs of 3 alone and 3 of 3 with the stall of bug-report step 3;
passed 20 of 20 race-enabled waiter runs with the stall, and 150 of 150 at `GOMAXPROCS=2` under `-race`; passed the
scenario outline's four rows at both adapters with the stall; `go tool golangci-lint run ./internal/guard/...` printed
`0 issues.` on a scratch copy of the module. Under the stall the unfixed test failed every waiter row at both adapters.

**Conditions beyond the reported one.**

- _The enqueue poll's 1 s wall-clock bound_ (lines 2114–2125) failed with "waiter did not enqueue" whenever the waiter's
  first transaction took longer. The park wait replaces it, bounded by `guardLivenessLimit` (lines 2176–2180), "a
  liveness maximum, not the property under test".
- _The waiter's 3 s wait_ (line 2110) let the waiter defer itself if its first transaction outlasted it. It becomes
  `guardLivenessLimit`, so it too bounds only liveness; a waiter that returns before parking fails at once with its
  error.
- _A parked waiter on failure._ Today's waiter ends by itself after 3 s; a parked one would wait for a cancellation that
  a failed assertion skips. `t.Cleanup(cancel)` ends it with the subtest.
- _The ledger-bytes check_ (lines 2129–2135 and 2165–2167) now sees only `ReservationStatus`'s reconciliation, since the
  parked waiter writes nothing between the two reads; before, the waiter's own writes (line 1301) could interleave.
- _The waiter's own retries during the corrupt window_ are no longer exercised. The outline names only "status and
  competing admission" (`reservations.feature`, line 359), and the waiter's cancellation and cleanup with a corrupt
  identity still run (lines 2146–2151).
- _Two other zero-wait admissions expect `ErrReservationDeferred` beside a running guard_:
  `TestRunFailedActivationReportRetainsLauncherIdentity` (lines 1939–1951) and
  `TestRunUnconfirmedRetirementPreservesOwnershipAndExit` (lines 2589–2599). Neither has been seen failing, and whether
  anything holds their root's lock at that instant is not established; they are left as they are, and a failure of
  either is a new defect, filed as its own bug-fix plan under the owner's standing request.

**Alternatives rejected.**

- _Accept either deferral_ (`ErrReservationDeferred` or `IsCoordinationDeferred`). It would pass whenever the admission
  never read the ledger, so a run could pass without testing the corrupt identity at all: assertion theater, per
  [Gherkin implementation review](../../../repo-governance/workflows/quality/gherkin-implementation-review.md). The
  held-lock break test below would pass.
- _Give the competing admission a nonzero wait._ It still passes only if the waiter releases inside that wait, a
  constant chosen against one host's load, and a deferred admission waits its whole wait before returning, lengthening
  every subtest; of retries and longer sleeps, #90 said "They lower the chance of a failure but do not remove it."
- _Map a lock refusal to `ErrReservationDeferred` in production._ Callers already see one outcome (see
  [Root Cause](#root-cause)), and line 73 of `reservations.feature` distinguishes "deferred for coordination"; a product
  change for a test-only defect.
- _Retry or quarantine the test._ It weakens a gate, which the upstream tool defects standard forbids.

**Public contract.** No exit status, `hippo.*` code, JSON document, configuration key, evidence shape, or production
code moves; the change is confined to `internal/guard/run_test.go`.

**Release content.** None. `CHANGELOG.md` must be true to the shipped binary
([documentation architecture](../../../repo-governance/conventions/documentation-architecture.md), line 19), and the
binary is unchanged. Earlier test-only repairs of the same class, #90 and #94 among them, carried no entry, nor do the
sibling degraded-lineage and distinct-root fixes.

**References**, read 2026-10-06 from the Go 1.27.1 distribution's `go doc` and `go help`:

- [The Go Programming Language Specification: Receive operator][go-receive] — "A receive operation on a closed channel
  can always proceed immediately", so `close(parked)` releases the test's wait.
- [Package context][go-context] — `Done` is closed when the context is cancelled, and `Err` then returns `Canceled`, the
  value the pause returns.
- [Package testing: T.Cleanup][go-cleanup] — cleanup functions run when the test and its subtests complete.
- [Build flags: `-overlay`][go-build] — "a build will run as if the disk file path exists with the contents given by the
  backing file paths", so every mutation below leaves the worktree untouched; set through `GOFLAGS`, it also reaches the
  `go test ./internal/guard` the scenario binding runs (`tests/support/blockers_v04.go`, line 1046).
- [Testing flags][go-testflag] and [package runtime: GOMAXPROCS][go-runtime] — `-count`, `-race`, `-timeout`, and the
  in-process contention the repeats use.
- `d9ab21a`, "fix(guard): take a free coordination gate before any bounded wait" (#78) — why a free gate is taken at
  once even with a zero wait.
- #90 (`abc9094`), "test: make the wall-clock flaky tests deterministic", and #94 (`68cae05`), "test: close the
  remaining slow-runner flake risks" — the same class of defect in other tests, and their rejected alternatives.

### Specification Changes

None. "Reservation identity path corruption remains fail closed" (`specs/behaviours/reservations.feature`, lines
356–367) stays word for word, its step binding (`tests/support/steps.go`, lines 213–219, and
`tests/support/blockers_v04.go`, lines 1109–1111) and its `tests/contract/contract.go` entry (line 290) are unchanged,
and lines 73 and 75–79 stay true.

### File Impact

```text
internal/guard/run_test.go                                          TestReservationIdentityPathCorruptionFailClosed
plans/in-progress/fix-corrupt-waiter-identity-test-flake/README.md  execution record
```

### Acceptance Criteria

- **AC-01** Given a runner that holds the coordination lock 50 ms in every admission transaction and every status read,
  when the test's waiter rows run, then they pass, and at both adapters so do the outline's waiter rows.
- **AC-02** Given the coordination lock held by another holder at the competing admission, when the test runs, then each
  row fails with `competing admission bypassed corrupt live identity: session=false error=shared coordination deferred`.
- **AC-03** Given a reconciliation that treats a missing or replaced identity path as stale, when the test runs, then
  the owner rows fail with `competing admission bypassed corrupt live identity: session=true error=<nil>`.
- **AC-04** Given the same stale reconciliation, when the test runs, then the waiter rows fail with
  `corrupt live identity was treated stale:`.
- **AC-05** Given an admission loop that ignores its `Pause` seam, when the test runs, then the waiter rows fail at the
  park wait with `waiter did not park` or `waiter returned before parking:`.
- **AC-06** Given the fix branch, then `git diff --name-only origin/main...HEAD` lists only the File Impact paths, the
  waiter block polls no ledger on the wall clock, and nothing under `cmd/`, `specs/`, `docs/`, `tests/`, `README.md`, or
  `CHANGELOG.md`, nor any non-test file under `internal/`, changes.
- **AC-07** Given the host's load at execution, recorded with `uptime`, then the test passes 500 consecutive
  race-enabled runs at `GOMAXPROCS=2` and 500 at the default, the outline passes at both adapters, and the full gate
  passes on the fix branch's head.
- **AC-08** Given the fix merges, then either the published `v0.8.5` contains its merge commit, or, when `v0.8.5`
  shipped without it, the record shows that `origin/main` contains the merge commit and `v0.8.5` does not, and that the
  fix is test-only with no release content, so no patch release follows.
- **AC-09** Given execution finishes, then this plan is gated, reconciled, and archived under `plans/done/`, and its
  worktrees and branches are gone.

## Delivery

**Execution checkout.** `worktrees/fix-corrupt-waiter-identity-test-flake` under the HIPPO repository location, created
from `origin/main` at `4cd530a` on branch `worktree/fix-corrupt-waiter-identity-test-flake`, per
[worktree to pull request](../../../repo-governance/workflows/maintenance/worktree-to-pull-request.md). Unit 1 lands
from that branch. Unit 2 branches from `origin/main` in the same directory as
`worktree/fix-corrupt-waiter-identity-test-flake-fix`. HIPPO cannot guard its own gates, so every command below runs
directly, never under `./hippo`, per
[resource-aware development](../../../repo-governance/development/resource-aware-development.md). No item starts CPU
load of its own; `scripts/test-loaded.sh` is not run, and contention is in-process only.

**Deviation, recorded.** The owner directed on 2026-10-06 that this worktree and its branches be removed immediately
after the fix merges. [Integration path](../../../repo-governance/conventions/integration-path.md) provisions at most
one worktree per plan and removes it once every unit has landed, and Unit 4 cannot land before `v0.8.5` exists. So Unit
4 provisions a second worktree, `worktrees/fix-corrupt-waiter-identity-test-flake-record`, once `v0.8.5` is published,
and removes it after its own merge.

**Commands** the items below name, run from the worktree root. Proofs count Go's raw `-v` lines (`--- PASS`,
`--- FAIL`); where a shell hook rewrites `go test` output, bypass it, as the degraded-lineage plan learned
(`rtk proxy go test …`).

- _Corruption test_: `go test -count=3 -v -run '^TestReservationIdentityPathCorruptionFailClosed$' ./internal/guard`.
  "With an overlay" adds `-overlay <scratch>/<name>.json` before `-count`.
- _Focused scenarios_: `go test -count=1 -v -run 'TestUnitBehaviours/^Reservation_identity_path_corruption'`
  `./tests/unit` for the unit adapter, and the same with `TestIntegrationBehaviours` and `./tests/integration` for the
  integration adapter. Godog names the outline's rows `Reservation_identity_path_corruption_remains_fail_closed`, then
  `#01`, `#02`, and `#03` suffixed, in the order owner/missing, owner/replaced, waiter/missing, waiter/replaced. "With
  an overlay" prefixes `GOFLAGS=-overlay=<scratch>/<name>.json`, which also reaches the binding's `go test` subprocess.
- _Repeated test_: the _Corruption test_ with `-race -count=500 -timeout 30m` in place of `-count=3`, run once with
  `GOMAXPROCS=2` before it and once without.
- _Scratch_: a directory made with `mktemp -d` at the start of the item that uses it and removed with `rm -r` at its
  end; proof of removal: `test ! -e <scratch>` exits `0`. Nothing in it is ever copied into the worktree.
- _Overlay_: `<scratch>/<name>.json` holding `{"Replace":{"<worktree>/<file>":"<scratch>/<copy>"}}`, with the worktree's
  absolute path; an overlay may list several files. The copies, each of the worktree's current file, located by their
  text (line numbers are at `4cd530a`):
  - _Stall copy_ of `internal/guard/reservation.go`: `time.Sleep(50 * time.Millisecond)` as a new statement right after
    the `if lockError != nil { … neverStartedAtCoordinationLock … }` block in the admission loop (after line 1212), and
    right after the `if err != nil { return ReservationTotals{}, err }` that follows `ReservationStatus`'s
    `acquireCoordinationLock` (after line 1575).
  - _Held-lock copy_ of `internal/guard/run_test.go`, after Phase 2: immediately before
    `blocked, blockedError := AcquireReservation(` in `TestReservationIdentityPathCorruptionFailClosed`,
    `held, holdError := acquireCoordinationLock(context.Background(), root, time.Second)` followed by
    `if holdError != nil { t.Fatal(holdError) }`, and `_ = releaseCoordinationLock(held)` immediately after that call's
    closing parenthesis.
  - _Stale copy_ of `internal/guard/reservation.go`: in `reservationIdentityAlive`, the candidate list
    `[]string{path, anchor}` (line 487) becomes `[]string{path, anchor}[:1]`, and the
    `return false, errors.New("reservation identity state is unverifiable")` under `if identity == nil` with an expected
    identity (line 527) becomes `return false, nil`, so a missing or replaced identity path reads as stale.
  - _No-pause copy_ of `internal/guard/reservation.go`: the three lines `if pause != nil {`, `return pause(ctx, delay)`,
    and `}` in `waitForReservationRetry` (lines 1104–1106) are deleted.
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
2. _Fix_ — the rewritten waiter block and this plan's execution record. Rollback: revert its merge; no release depends
   on it.
3. _Release_ — `v0.8.5`, cut by the linting plan's Unit 7 after unit 2 merges; this plan only records it. That plan's
   Unit 7 item "Before cutting, confirm every bug-fix plan this release carries has merged", in its worktree at
   `0150a49` on 2026-10-06, covers "any later one under `plans/in-progress/fix-*`", which includes this plan once unit 1
   lands. If the tag is cut anyway, Phase 4's tag check and Phase 5's Recovery item cover it. A published tag is never
   replaced.
4. _Record_ — the release record, the execution check, and the move to `plans/done/`. Rollback: revert its merge.

**Out of scope: repinning consumers.** The binary is unchanged, so no consumer repins for this plan; the linting plan
leaves repinning `v0.8.5` to each consumer's own repository
([plan lifecycle](../../../repo-governance/conventions/plan-lifecycle.md)).

**Pause safety.** Each item records its result when ticked. To resume, read the last ticked item, then
`git -C <worktree> log --oneline origin/main..HEAD`, `gh pr list --head <branch>`, and
`git ls-remote --tags origin v0.8.5`. A _Scratch_ directory left by an interrupted item is removed and the item rerun.
Between unit 2's merge and unit 4, the record is this file on `origin/main`, with Phases 1–4 ticked and Phase 5 open.

### Phase 1: Plan

- [ ] `[AI]` Land this file and its `plans/in-progress/README.md` entry alone with _Land_, from
      `worktree/fix-corrupt-waiter-identity-test-flake`; proof: the merge commit on `origin/main` and _Reconcile_
      reading `0 0`. `[AC-09]`
- [ ] `[AI]` Create the fix branch in the same directory: `git fetch origin --prune`, then
      `git switch -c worktree/fix-corrupt-waiter-identity-test-flake-fix origin/main`, then `npm ci`; proof:
      `git branch --show-current` prints the branch and `git status --porcelain` prints nothing. `[AC-06]`
- [ ] `[AI]` Run the [plan quality gate](../../../repo-governance/workflows/quality/plan-quality-gate.md) on this folder
      in mode `normal`, at most three cycles, and commit its repairs and its verdict line as the fix branch's first
      commit, a `docs(plans)` commit; proof: one terminal `plan-quality-gate:` verdict line recorded here and
      `git status --porcelain` printing nothing. `[AC-09]`

### Phase 2: The Waiter Parks Outside the Lock

- [ ] `[AI]` RED: in a _Scratch_, write the _Stall copy_ and its overlay as `stall.json`, and run the _Corruption test_
      and the _Focused scenarios_ with it, recording the output; proof: the _Corruption test_ fails `waiter/missing` and
      `waiter/replaced` 3 runs of 3 each with `competing admission bypassed corrupt live identity: session=false error=`
      followed by `shared coordination deferred admission: another admission is updating the shared root`, and passes
      both owner rows 3 of 3; at each adapter rows `#02` and `#03` fail and the first two pass; `git status --porcelain`
      prints nothing; the _Scratch_ is removed. `[AC-01]`
- [ ] `[AI]` GREEN: in `internal/guard/run_test.go`, `TestReservationIdentityPathCorruptionFailClosed`, replace the body
      of the `if record == "waiter"` block (lines 2105–2125) with the block under [Solution](#solution), changing
      nothing else. In a _Scratch_, write the _Stall copy_ and `stall.json`; proof: the _Corruption test_ passes every
      row 3 runs of 3 without an overlay and 3 of 3 with `stall.json`, the _Focused scenarios_ pass all four rows at
      both adapters with `stall.json`, and the _Scratch_ is removed. `[AC-01]`
- [ ] `[AI]` REFACTOR: confirm the waiter block reads no clock but its liveness bound and states why it parks; proof:
      `awk '/^func TestReservationIdentityPathCorruptionFailClosed/,/^}/' internal/guard/run_test.go` piped to
      `grep -cE 'time\.(Now|Sleep)|time\.Second'` prints `2` (the owner's and the replacement's 1 s admission waits),
      `go tool golangci-lint run ./internal/guard/...` prints `0 issues.`, `gofmt -l internal/guard` prints nothing, and
      the _Corruption test_ still passes. If lint reports a length or complexity finding on the test, move the waiter's
      start and park into a helper `startParkedReservationWaiter(t, root, plan)` returning the token, the cancel
      function, and the done channel, and rerun this proof. `[AC-06]`
- [ ] `[AI]` Commit `internal/guard/run_test.go` alone as
      `test(guard): park the corrupt-identity waiter outside the coordination lock`; proof:
      `git log --oneline origin/main..HEAD` lists it after the gate commit, and `git diff --name-only HEAD -- internal`
      prints nothing. `[AC-06]`

### Phase 3: Review and Documentation

- [ ] `[AI]` Break tests: in a _Scratch_, write the _Held-lock copy_, the _Stale copy_, and the _No-pause copy_ with an
      overlay for each (the _Stale_ and _No-pause_ overlays list only `reservation.go`), and run the _Corruption test_
      with each, using `-count=1` for the no-pause overlay, whose waiter rows each take the 30 s liveness limit; proof,
      each recorded here: the held-lock overlay fails all four rows 3 of 3 with
      `competing admission bypassed corrupt live identity: session=false error=shared coordination deferred`; the stale
      overlay fails both owner rows 3 of 3 with
      `competing admission bypassed corrupt live identity: session=true error=<nil>` and both waiter rows 3 of 3 with
      `corrupt live identity was treated stale:`; the no-pause overlay fails both waiter rows with `waiter did not park`
      or `waiter returned before parking:` and passes both owner rows; `git diff --quiet -- internal` exits `0`; the
      _Scratch_ is removed. `[AC-02]` `[AC-03]` `[AC-04]` `[AC-05]`
- [ ] `[AI]` Run the
      [Gherkin implementation review](../../../repo-governance/workflows/quality/gherkin-implementation-review.md) on
      "Reservation identity path corruption remains fail closed", whose binding runs this test: with the _Stale copy_
      overlay through `GOFLAGS`, run the _Focused scenarios_; proof: all four rows fail at both adapters, the review
      records each row's status, none `untested`, `unimplemented`, or `drifted`, and the _Scratch_ is removed. `[AC-03]`
      `[AC-04]`
- [ ] `[AI]` Run [docs propagation](../../../repo-governance/workflows/quality/docs-propagation.md) and record that no
      page describes this test and that, per [Release content](#solution), `CHANGELOG.md` gets no entry; proof:
      `git grep -n 'IdentityPathCorruption' -- README.md docs specs CHANGELOG.md repo-governance` prints nothing, and
      `git diff --name-only origin/main...HEAD -- README.md docs specs tests CHANGELOG.md` prints nothing. `[AC-06]`

### Phase 4: Verification

- [ ] `[AI]` Bounded checkpoint: run both forms of the _Repeated test_, then the _Focused scenarios_, recording `uptime`
      before and after each; proof: each exits `0`, with 500 `--- PASS` lines for every row in each _Repeated test_
      form, four passing rows at each adapter, and no `--- FAIL`. Fallback, decided now: one failure stops the plan
      before landing; its output is recorded here, the cause it shows replaces the matching part of
      [Root Cause](#root-cause), and nothing lands until a new RED proves that cause. `[AC-07]`
- [ ] `[AI]` Run the _Full gate_ on the branch head; proof: exit `0`, ending with `No vulnerabilities found.` A failure
      in another test is recorded here and filed as its own bug-fix plan under the owner's standing request; this plan
      lands only after that fix merges and a rebase onto it reruns the _Full gate_ clean. `[AC-07]`
- [ ] `[AI]` Before landing, commit the execution record so far as a `docs(plans)` commit on the fix branch, because
      `git rebase` refuses a dirty tree and the rebase never auto-stashes; then `git fetch origin --tags` and confirm
      `v0.8.5` does not yet exist; then rebase onto `origin/main`, reading the whole incoming diff (the linting plan's
      in-flight units and the sibling fixes edit `internal/guard/run_test.go` and `internal/guard/reservation.go`), and
      rerun the _Corruption test_, the _Focused scenarios_, and the _Full gate_ if the rebase brought commits; proof:
      `git status --porcelain` prints nothing before the rebase, `git ls-remote --tags origin v0.8.5` prints nothing,
      and the reruns exit `0`. If the tag already exists, land anyway and the Recovery item in Phase 5 fires. `[AC-07]`
      `[AC-08]` `[AC-09]`
- [ ] `[AI]` Confirm the change stays inside its boundary; proof: `git diff --name-only origin/main...HEAD` prints only
      the paths in [File Impact](#file-impact). `[AC-06]`
- [ ] `[AI]` Commit the rest of this plan's execution record (Phases 1–4 ticked with the rebase, rerun, and boundary
      results) as a `docs(plans)` commit on the fix branch, as the last commit before landing; proof:
      `git status --porcelain` prints nothing. `[AC-09]`

### Phase 5: Release Through v0.8.5

- [ ] `[AI]` Land unit 2 with _Land_, its pull-request body carrying the RED and GREEN captures per
      [red-green-refactor](../../../repo-governance/workflows/quality/red-green-refactor.md); proof: the merge commit on
      `origin/main` and _Reconcile_ reading `0 0`, both recorded here by unit 4, since the merged copy cannot hold its
      own merge. The fix must merge before the linting plan's Unit 7 tags `v0.8.5`. `[AC-01]` `[AC-02]` `[AC-03]`
      `[AC-04]` `[AC-05]` `[AC-06]` `[AC-07]`
- [ ] `[AI]` Immediately after that merge, run
      [dev artifact clean-up](../../../repo-governance/workflows/maintenance/dev-artifact-clean-up.md) for this
      worktree, at the owner's direction: confirm nothing is unpushed or running, then remove the worktree with
      `git worktree remove` without `--force`, and delete `worktree/fix-corrupt-waiter-identity-test-flake` and
      `...-fix` locally and on `origin`; proof: `git worktree list` omits it,
      `git branch --list 'worktree/fix-corrupt-waiter-*'` and
      `git ls-remote origin 'refs/heads/worktree/fix-corrupt-waiter-*'` print nothing. `[AC-09]`
- [ ] `[AI]` Provision `worktrees/fix-corrupt-waiter-identity-test-flake-record` from `origin/main` on branch
      `worktree/fix-corrupt-waiter-identity-test-flake-record`, with `npm ci`, once `v0.8.5` is published; proof:
      `git branch --show-current` prints the branch. `[AC-09]`
- [ ] `[AI]` Once `v0.8.5` is published, record the release in the record worktree's copy of this plan; proof:
      `git merge-base --is-ancestor <unit 2 merge commit> v0.8.5` exits `0`, and the release URL and the tag's peeled
      commit are recorded here. `[AC-08]`
- [ ] `[AI]` Recovery, dormant until triggered. Trigger: `v0.8.5` is published without unit 2's merge commit (the
      ancestry check above exits `1`). Then cut no patch release, because the binary is unchanged: record in the record
      worktree's copy of this plan that the fix is test-only with no release content and that `v0.8.5`'s full gate ran
      without it, with the result the linting plan's Unit 7 recorded for it; proof:
      `git merge-base --is-ancestor <unit 2 merge commit> origin/main` exits `0` and the `v0.8.5` check exits `1`, both
      recorded there, so the next release cut from `origin/main` carries the fix. Otherwise: a dated, evidenced
      `Not triggered`. `[AC-08]`

### Phase 6: Close

- [ ] `[AI]` Route each learning below to its durable owner, or discard it with a reason; proof: each entry names its
      owner or its reason. `[AC-09]`
- [ ] `[AI]` Run the [execution check](../../../repo-governance/workflows/plan/plan-execution-check.md); proof: its
      verdict line recorded here. `[AC-09]`

### Archival

- [ ] `[AI]` Move this folder with `git mv` to `plans/done/<completion date>__fix-corrupt-waiter-identity-test-flake/`,
      update `plans/in-progress/README.md` and `plans/done/README.md` in the same change, and land it with _Land_;
      proof: the merge commit, posted on the archival pull request, and no copy left under `plans/in-progress/`.
      `[AC-09]`
- [ ] `[AI]` Run [dev artifact clean-up](../../../repo-governance/workflows/maintenance/dev-artifact-clean-up.md) for
      the record worktree right after that merge; proof, posted on the archival pull request: `git worktree list` omits
      it, no local or remote `worktree/fix-corrupt-waiter-*` branch remains, and _Reconcile_ reads `0 0`. `[AC-09]`

## Learnings

None yet. Entries are added as execution teaches something, and each is routed to a durable owner or discarded with a
reason before archival.

## Directory Map

This plan is one document, so this README has no siblings to map.

[go-receive]: https://go.dev/ref/spec#Receive_operator
[go-context]: https://pkg.go.dev/context
[go-cleanup]: https://pkg.go.dev/testing#T.Cleanup
[go-build]: https://pkg.go.dev/cmd/go#hdr-Compile_packages_and_dependencies
[go-testflag]: https://pkg.go.dev/cmd/go#hdr-Testing_flags
[go-runtime]: https://pkg.go.dev/runtime
