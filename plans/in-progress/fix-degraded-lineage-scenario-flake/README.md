# Fix: Degraded-Lineage Admission Scenario Flakes Under Load

Status: In progress (2026-10-06)

On a heavily loaded host, the behaviour scenario "A configured profile derived from balanced admits degraded work" fails
intermittently: the guard defers the work with "safe admission was not reached" instead of admitting it at concurrency
one. The scenario's fixture times the guard's admission window on the wall clock while its host readings advance on a
synthetic one, so the scenario passes only when the runner takes sixteen readings inside 100 real milliseconds. The
defect is in the test fixture alone: production HIPPO defers when its readings come slowly, which is the safe outcome.

This is a [bug-fix plan](../../../repo-governance/conventions/plans/010-bug-fix-plan.md). The owner requested it on
2026-10-06 under the Upstream Tool Defects standard recorded by a consumer workstation, because the defect blocks the
full gate of the `v0.8.5` release, which runs on a host where every workstation repository is guarded through HIPPO.
Under this repository's [upstream tool defects](../../../repo-governance/development/upstream-tool-defects.md) standard,
that request also directs this plan's quality gate, its execution, and its release.

**Workaround, recorded.** Re-running the gate sometimes passes, and the convention counts "a retry that reliably
succeeds" as a workaround. Here the retry is not reliable: the host's sustained load is the condition that produces the
failure. The owner's request is what the convention accepts in place of a blocking defect, as in
[the v0.8.4 fix](../../done/2026-10-03__fix-configured-profiles-starve-under-macos-warning/README.md) and the sibling
plan `fix-cancelled-waiter-cleanup-flake`.

**Release.** The change is test-only, so it carries no release content of its own: no `CHANGELOG.md` entry and no tag.
It must merge before the in-flight plan
[strict Go linting and domain modeling](../strict-go-linting-and-domain-modeling/README.md) cuts `v0.8.5` in its Unit 7,
so that cut's full gate runs with it (see [Phase 5](#phase-5-release-through-v085)).

Line numbers in this plan are at `4cd530a`, the trunk commit that landed it and the one it executes from, unless a
sentence names another commit or tag. The files they cite are identical at `c109c4d`, where the defect was diagnosed.

## Bug Report

**Description.** Under host load, the guard in "A configured profile derived from balanced admits degraded work" defers
ephemeral work under a full stable macOS warning window instead of admitting it at concurrency one, and the scenario
fails at its `Then` step.

**Steps to reproduce**, from a clean checkout of `wahidyankf/hippo` at `c109c4d` with `npm ci` run:

1. _As observed:_ run `npm test` on a host whose load average stays well above its core count. The scenario fails in
   some runs of the race-enabled step,
   `go test -race -count=1 ./cmd/... ./internal/... ./tests/support ./tests/unit ./tests/integration`
   (`scripts/test.sh`, line 20), and passes on rerun.
2. _Under in-process contention:_ add a scratch test to package `support` that starts eight goroutines at once, each
   running the scenario's bindings on a fresh `Driver` — `stableDarwinWarning`,
   `derivedProfileConfiguration("balanced")`, `guardUnderConfiguration` — and counts runs whose `errorOutput` lacks the
   degraded-admission line; repeat it 25 times. Run it with
   `GOMAXPROCS=1 go test -race -count=1 -timeout 30m -run <name> ./tests/support`.
3. _Deterministically:_ in `tests/support/degraded_lineage.go`, add `time.Sleep(10 * time.Millisecond)` to
   `advancingCollector.Collect` after its context check (line 66), standing in for a runner the scheduler starves. Then
   run `go test -count=2 -v -run 'TestUnitBehaviours/^A_configured_profile_' ./tests/unit`.

**Expected behaviour.** The scenario at `specs/behaviours/admission.feature`, lines 29–33, says that with a
configuration whose default profile extends `balanced` and "a full stable Darwin warning window with safe headroom",
"the guard admits the child at concurrency one and says so". The window is a property of the readings, which the fixture
supplies, so the outcome should not depend on how fast the runner takes them.

**Actual behaviour.** Step 3 failed 2 runs of 2 on 2026-10-06; "A configured profile outside balanced's lineage never
uses degraded admission" passed both:

```text
Scenario: A configured profile derived from balanced admits degraded work
  # ../../specs/behaviours/admission.feature:29
  Then the guard admits the child at concurrency one and says so
  # ../../specs/behaviours/admission.feature:33
    Error: profile "local-balanced": exit=0 stderr="HIPPO deferred task: safe admission was not reached.\n"
--- FAIL: TestUnitBehaviours/A_configured_profile_derived_from_balanced_admits_degraded_work (0.13s)
```

Godog prints each location comment at the end of its line; it is wrapped here to fit. Step 2 deferred 53 of 200 runs on
2026-10-06 at load averages of 6–29; the diagnosing session's earlier copy of it deferred 156 of 200. Run alone, the
scenario passed 10 of 10 race-enabled passes with `GOMAXPROCS=1` at load averages of 25–40, so a serial rerun is not a
reproduction. The consumer's gate saw it fail twice at load averages near 45 and pass on rerun (the linting plan's
`learnings.md`, lines 184–187).

**Error output** is the `Error:` line quoted above, from the step binding `requireConfiguredDegradedAdmission`,
`tests/support/degraded_lineage.go`, lines 185–191.

**Environment.** macOS 15.5 on Apple M2 Max with 12 cores, Go 1.27.1 (the local toolchain; `go.mod` requires 1.26.1),
Node 24.16.0, at `c109c4d` (`origin/main`).

**Not a regression.** The fixture's timing dates from `9a3c142` (pull request #126), which added the scenario in the
`v0.8.4` fix; the `v0.8.4` tag's `guardUnderConfiguration` sets the same 100 ms window, 1 ms interval, `time.Sleep`, and
`time.Now` (its lines 118–136). Later commits moved the run into `guardUnder` without changing its timing.

## Duplicate Check

Run 2026-10-06 against `origin/main` at `c109c4d`:

- `gh issue list --state all` returned no issues. `gh issue list --state all --search` for `flake`, `flaky`,
  `safe admission`, `degraded admission`, `admission window`, `lineage`, `derived from balanced`, and `clock` each
  returned nothing.
- `gh pr list --state open` returned no pull requests. `gh pr list --state all --search` for the same terms returned
  only merged or closed work, none touching this fixture's clock: #90 and #94 repaired other wall-clock tests, #103
  released force-stop fixtures on a clock, #126 added this scenario, and #135 is the linting plan's lineage work.
- `plans/backlog/` holds no plan, `plans/in-progress/` only the linting plan, and `plans/ideas/` one brief, the
  degraded-admission release-audit follow-ups, which does not mention the fixture.
- `grep -rliE` over `plans/` outside `done/` and over `repo-governance/`, for one alternation of `flak`,
  `safe admission was not reached`, `derived from balanced`, `guardUnder`, `logical clock`, `injectable clock`,
  `clock injectable`, and `admission window`, found the idea brief above and the linting plan, whose `learnings.md`
  (lines 184–187) records this flake with the routing candidate "an idea brief to make the fixture's clock injectable",
  never filed. Inside `plans/done/` it found the supervision readiness race repair, a different flake, and the `v0.8.4`
  fix plan that wrote this fixture.
- `git log -i -F --grep` for `derived from balanced`, `guardUnder`, `logical clock`, and `degraded-lineage` found
  nothing; for `admission window` and `safe admission was not reached` it found `4f7b15c`, `5c30714`, `5997710`, and
  `68cae05`, which widened or documented other fixtures' windows and left this one alone.
- No web search tool was available to this session, so no search for outside reports was made; the defect is in HIPPO's
  own tests, which the searches above cover.

No duplicate exists.

## Root Cause

**Two clocks.** The step "the guard runs ephemeral work under that configuration" (`tests/support/steps.go`, line 513)
calls `guardUnderConfiguration`, which resolves the profile and calls `guardUnder` (`tests/support/degraded_lineage.go`,
lines 134–183). `guardUnder` sets a 1 ms `SampleInterval` and a 100 ms `AdmissionWindow` (lines 157–158) and runs
`guard.Run` with `Sleep: time.Sleep` and `Now: time.Now` (lines 174–175). Its collector, an `advancingCollector` (lines
55–76, built at line 170), stamps reading _n_ at `n × 1 s` on a synthetic clock. The readings decide readiness; the wall
clock decides the deadline.

**What admission needs.** The scenario's window is the 16 stable warning readings of `stableWarningSamples`
(`tests/support/driver.go`, lines 402–416). `WarningAdmissionReady` (`internal/policy/policy.go`, line 357) first
requires `warningWindowReady` (lines 345–354): the first and last readings at least `TrendWindow` apart, which is 15 s,
the default (line 117) that no profile overrides. On the synthetic clock that is the sixteenth reading.

**What the guard allows.** `Run` sets `deadline := config.Now().Add(config.Policy.AdmissionWindow)`
(`internal/guard/run.go`, line 852). Each pass of its admission loop (lines 873–930) collects a reading (line 878),
appends it to the evidence file (lines 889–894), checks both admissions (lines 904–921), stops when the wall clock has
reached the deadline (lines 923–925), and otherwise pauses through `config.Sleep` (line 927, via `waitForContext`, lines
367–371). So the scenario passes only if 16 collections, 16 evidence writes, and 15 real 1 ms sleeps finish within 100
real milliseconds — under 6.25 ms a reading. `time.Sleep` "pauses the current goroutine for at least the duration d"
([package time][go-sleep]); scheduler delay, the race detector, and file writes add to every pass. A loaded runner
exceeds the budget, the loop leaves at line 924 with `admitted` false, and line 934 prints the reported message.

**Why the stall reproduction is exact.** A 10 ms stall per reading puts the wall clock at or past the deadline by the
tenth reading, six before admission is possible, whatever the host's speed. `4f7b15c` proved the same defect in three
other fixtures the same way ("a collector slowed by 50 ms per sample failed all three scenarios 3/3").

**Why this fixture still has it.** `4f7b15c` gave fixtures whose evidence always admits an admission window no runner
exhausts, `evidenceDecidesAdmission` (`tests/support/pending_v04.go`, lines 55–61), whose comment says "a fixture that
asserts a deferral sets its own tight window instead". `guardUnder` serves both: the sibling scenario "A configured
profile outside balanced's lineage never uses degraded admission" (`admission.feature`, lines 35–40) asserts the
deferral, and so does the constrained row of `TestTheGuardReadsDegradedAdmissionFromTheLineageAlone`
(`tests/support/degraded_lineage_internal_test.go`, lines 14–46). An hour-long window would make each of those wait an
hour, so the shared fixture kept 100 ms. The `v0.8.4` plan that wrote it made supervision independent of host speed with
the advancing collector, but its admission deadline stayed on the wall clock.

**Only the admitting side flakes.** A slow runner can only reach the deadline sooner, so the deferral scenario and the
constrained row cannot fail this way. The balanced row of `TestTheGuardReadsDegradedAdmissionFromTheLineageAlone` calls
`guardUnder` too and can fail the same way.

**Production is fail-safe.** In production the readings and the deadline share the wall clock, and a guard that cannot
complete a full warning window before its deadline defers with `124` naming `hippo.limit.capacity-deferred`; it never
admits on a partial window. The defect is the fixture's mix of two clocks.

## Solution

**One logical clock for the fixture's guard.** `guardUnder` gives `guard.Run` a clock whose time passes only when the
guard pauses: a `logicalClock` holding a `time.Time`, whose `Now` returns it and whose `Sleep` adds the duration to it,
started at `time.Now()` so evidence timestamps stay current. `RunConfig` already accepts both seams
(`internal/guard/run.go`, lines 44 and 56), and dozens of fixtures already pass a no-op `Sleep`. The 100 ms window
stays.

Why it removes the cause: the deadline and the pauses now share one clock, which only the guard's own pauses advance, so
the window counts readings. The guard admits at the sixteenth reading, after 15 logical milliseconds, and the deferral
paths end after the 101st reading, at 100 logical milliseconds, with no wall-clock wait. However slowly a runner takes
its readings, the outcome is the one the readings decide. On the paths this fixture takes, `Run` calls `Now` and `Sleep`
only on its own goroutine (lines 604, 799, 845, 852, 864, 923, 927, 937, 1085, 1098, and 1164), so the clock needs no
lock; the race-enabled repeats below confirm that. The other calls (lines 434, 612–615, 631, and 1021) sit behind
`ReservationPolicy.Enabled`, which `guardUnder` never sets.

**A stall seam, for the regression test.** `Driver` gains `readingStall time.Duration`, zero in every scenario, and
`advancingCollector` gains `stall`, which `Collect` sleeps in real time before each reading; `guardUnder` passes the
driver's value. The test drives both scenarios' own step bindings with a 10 ms stall, so it fails for the reported
reason before the fix and cannot pass by luck after it.

**Conditions beyond the reported one.**

- The balanced and constrained rows of `TestTheGuardReadsDegradedAdmissionFromTheLineageAlone` use `guardUnder`, so the
  same change covers them.
- `superviseLineageChild` (`degraded_lineage.go`, lines 416–451) keeps `time.Sleep` and `time.Now`: it admits on three
  healthy readings under the hour-long `evidenceDecidesAdmission` window (`fastBehaviourPolicy`, `driver.go`, lines
  546–554), and its 3 ms shed graces are measured on the real clock by design, per the `v0.8.4` plan.
- The bounded-cancellation fixture in `tests/support/blockers_v04.go`, lines 620–623, also sets a 100 ms window, but it
  admits on three readings with two 2 ms pauses and no failure of it has been observed; it is left as it is.

**Alternatives rejected.**

- _Use `evidenceDecidesAdmission`._ The deferral scenario and the constrained row would wait out an hour of real 1 ms
  pauses before deferring.
- _Widen the window to a second or more._ The diagnosing session's copy saw no failure in 100 runs with a 1 s window at
  `GOMAXPROCS=2`, but a wider margin is still a constant chosen against one host's load, the class
  `scripts/test-loaded.sh` describes, and every deferral run would wait the whole window in real time.
- _A no-op `Sleep` with the real `Now`._ Admission would still race the wall clock, and the deferral paths would spin
  through readings and evidence writes for 100 ms.
- _Put the production deadline on the readings' timestamps._ Production is correct, and that changes behaviour the
  public contract fixes.
- _Retry or quarantine the scenario._ It weakens a gate, which the upstream tool defects standard forbids.

**Public contract.** No exit status, `hippo.*` code, JSON document, configuration key, evidence shape, or production
code moves; the change is confined to `tests/support`.

**Release content.** None. `CHANGELOG.md` records "all notable changes to HIPPO" and must be true to the shipped binary
([documentation architecture](../../../repo-governance/conventions/documentation-architecture.md), line 19), and the
binary is unchanged. Earlier fixture repairs of the same class — `4f7b15c`, `5c30714`, `abc9094`, `46aee64`, and
`272dd2e` — carried no entry.

**References**, read 2026-10-06 from the Go 1.27.1 distribution's `go doc` and `go help testflag`:

- [Package time: Sleep][go-sleep] — "pauses the current goroutine for at least the duration d", so a stall never ends
  early and the reproduction's lower bound holds.
- [Package runtime: GOMAXPROCS][go-runtime] — limits the operating system threads executing Go code at once, the
  in-process contention the reproduction and the repeats use.
- [Testing flags][go-testflag] — `-count n` runs each test _n_ times; `-timeout` defaults to 10 minutes.
- `4f7b15c`, "test(support): let controlled evidence alone decide fixture admission" — the same defect in three other
  fixtures, its stall-based RED, and the convention this fixture could not adopt.
- `9a3c142` (#126), "fix(guard): key degraded admission and its shed exemption on lineage", and its plan,
  [the v0.8.4 fix](../../done/2026-10-03__fix-configured-profiles-starve-under-macos-warning/README.md) — where the
  fixture and its advancing collector came from.

### Specification Changes

None. `specs/behaviours/admission.feature` is preserved word for word; both scenarios stay `@e2e-exempt` with their
`tests/contract/contract.go` entries (lines 87–88) unchanged, and run at the unit and integration adapters. No page
under `specs/` describes the fixture's clock.

### File Impact

```text
tests/support/degraded_lineage.go                logicalClock; guardUnder runs on it; advancingCollector stall
tests/support/driver.go                          Driver.readingStall
tests/support/degraded_lineage_internal_test.go  TestDegradedAdmissionIgnoresARunnerStall
plans/in-progress/fix-degraded-lineage-scenario-flake/README.md   execution record
```

### Acceptance Criteria

- **AC-01** Given a configuration whose default profile extends `balanced`, a full stable Darwin warning window, and a
  runner that takes 10 ms over each reading, when the guard runs ephemeral work under it, then it admits the child at
  concurrency one and prints the degraded-admission line.
- **AC-02** Given the same window and stall with a profile that extends `constrained`, then the guard defers naming
  `hippo.limit.capacity-deferred` and prints no degraded-admission line.
- **AC-03** Given the fix, then both scenarios' text and contract entries are unchanged, both pass at the unit and
  integration adapters, and granting or denying a lineage degraded admission still fails the scenario that covers it.
- **AC-04** Given the fix branch, then `git diff --name-only origin/main...HEAD` lists only the File Impact paths, and
  nothing under `cmd/`, `internal/`, `specs/`, `docs/`, `README.md`, or `CHANGELOG.md` changes.
- **AC-05** Given the host's load at execution, recorded with `uptime`, then the regression and lineage tests pass 50
  consecutive race-enabled runs at `GOMAXPROCS=1`, the two scenarios pass 10 at the integration adapter, the contention
  reproduction defers none of 200 runs, and the full gate passes on the fix branch's head.
- **AC-06** Given the fix merges, then either the published `v0.8.5` contains its merge commit, or, when `v0.8.5`
  shipped without it, the record shows that `origin/main` contains the merge commit and `v0.8.5` does not, and that the
  fix is test-only with no release content, so no patch release follows.
- **AC-07** Given execution finishes, then this plan is gated, reconciled, and archived under `plans/done/`, and its
  worktrees and branches are gone.

## Delivery

**Execution checkout.** `worktrees/fix-degraded-lineage-scenario-flake` under the HIPPO repository location, created
from `origin/main` at `c109c4d` on branch `worktree/fix-degraded-lineage-scenario-flake`, per
[worktree to pull request](../../../repo-governance/workflows/maintenance/worktree-to-pull-request.md). Unit 1 lands
from that branch. Unit 2 branches from `origin/main` in the same directory as
`worktree/fix-degraded-lineage-scenario-flake-fix`. HIPPO cannot guard its own gates, so every command below runs
directly, never under `./hippo`, per
[resource-aware development](../../../repo-governance/development/resource-aware-development.md). No item starts CPU
load of its own; `scripts/test-loaded.sh` is not run.

**Deviation, recorded.** The owner directed on 2026-10-06 that this worktree and its branches be removed immediately
after the fix merges. [Integration path](../../../repo-governance/conventions/integration-path.md) provisions at most
one worktree per plan and removes it once every unit has landed, and Unit 4 cannot land before `v0.8.5` exists. So Unit
4 provisions a second worktree, `worktrees/fix-degraded-lineage-scenario-flake-record`, once `v0.8.5` is published, and
removes it after its own merge.

**Commands** the items below name, run from the worktree root:

- _Regression test_: `go test -count=3 -v -run` with the pattern
  `'TestDegradedAdmissionIgnoresARunnerStall|TestTheGuardReadsDegradedAdmissionFromTheLineageAlone'` and
  `./tests/support`.
- _Focused scenarios_: `go test -count=1 -v -run 'TestUnitBehaviours/^A_configured_profile_' ./tests/unit` for the unit
  adapter, and the same with `TestIntegrationBehaviours` and `./tests/integration` for the integration adapter. Godog
  runs each scenario as a subtest named after it, and only the two scenarios of this plan carry that prefix: the unit
  run printed `346 scenarios (2 passed, 344 undefined)` on 2026-10-06.
- _Repeated tests_: `GOMAXPROCS=1` before the _Regression test_, with `-race -count=50 -timeout 30m` in place of
  `-count=3`.
- _Repeated scenarios_: `GOMAXPROCS=1` before the integration form of _Focused scenarios_, with
  `-race -count=10 -timeout 60m` in place of `-count=1`. One race-enabled unit pass took 40–116 s at `GOMAXPROCS=1` and
  load averages of 25–40 on 2026-10-06, so Go's default 10-minute timeout would stop it.
- _Contention check_: write `tests/support/zz_contention_scratch_test.go` with step 2 of the [Bug Report](#bug-report)
  as one test, `TestContentionScratch`, that logs `deferred <failures> of <runs>`; run
  `GOMAXPROCS=1 go test -race -count=1 -timeout 30m -v -run TestContentionScratch ./tests/support`; then delete the
  file.
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
2. _Fix_ — the logical clock, the stall seam, the regression test, and this plan's execution record. Rollback: revert
   its merge; no release depends on it.
3. _Release_ — `v0.8.5`, cut by the linting plan's Unit 7 after unit 2 merges; this plan only records it. That plan's
   Unit 7 holds the cut until this fix has merged, in its item "Before cutting, confirm both bug-fix plans this release
   carries have merged: `fix-cancelled-waiter-cleanup-flake` and `fix-degraded-lineage-scenario-flake` … wait for it
   rather than cut without it", which lands with that plan's Unit 5. If it does not land, or the tag is cut anyway,
   Phase 4's tag check and Phase 5's Recovery item cover it. A published tag is never replaced.
4. _Record_ — the release record, the execution check, and the move to `plans/done/`. Rollback: revert its merge.

**Out of scope: repinning consumers.** The binary is unchanged, so no consumer repins for this plan; the linting plan
leaves repinning `v0.8.5` to each consumer's own repository
([plan lifecycle](../../../repo-governance/conventions/plan-lifecycle.md)).

**Pause safety.** Each item records its result when ticked. To resume, read the last ticked item, then
`git -C <worktree> log --oneline origin/main..HEAD`, `gh pr list --head <branch>`, and
`git ls-remote --tags origin v0.8.5`. Between unit 2's merge and unit 4, the record is this file on `origin/main`, with
Phases 1–4 ticked and Phase 5 open.

### Phase 1: Plan

- [x] `[AI]` Land this file and its `plans/in-progress/README.md` entry alone with _Land_, from
      `worktree/fix-degraded-lineage-scenario-flake`; proof: the merge commit on `origin/main` and _Reconcile_ reading
      `0 0`. `[AC-07]`
  - Result: (2026-10-06) pull request #137, rebased once onto `db9632a` (index conflict with the sibling plan, both
    entries kept), merged as `4cd530a`; _Reconcile_ `0 0`.
- [x] `[AI]` Create the fix branch in the same directory: `git fetch origin --prune`, then
      `git switch -c worktree/fix-degraded-lineage-scenario-flake-fix origin/main`, then `npm ci`; proof:
      `git branch --show-current` prints the branch and `git status --porcelain` prints nothing. `[AC-04]`
  - Result: `worktree/fix-degraded-lineage-scenario-flake-fix` from `origin/main` at `4cd530a`, `node_modules` already
    installed; the merged plan branch was deleted at once; tree clean.
- [x] `[AI]` Run the [plan quality gate](../../../repo-governance/workflows/quality/plan-quality-gate.md) on this folder
      in mode `normal`, at most three cycles, and commit its repairs and its verdict line as the fix branch's first
      commit, a `docs(plans)` commit; proof: one terminal `plan-quality-gate:` verdict line recorded here and
      `git status --porcelain` printing nothing. `[AC-07]`
  - Result: `plan-quality-gate: PASS (2 cycles, 8 rows fixed: 1 HIGH, 3 MEDIUM, 4 LOW; 0 open)`. Entry checks: prettier,
    `markdownlint-cli2` 0 issues, `./rhino md internal-link validate` and `./rhino governance directory-map validate` no
    findings, line-length check clean. Cycle 1: PQG-01 (HIGH, record-worktree provisioning after the items writing into
    it) and PQG-02..07 fixed. Cycle 2: all seven held; PQG-08 (LOW, a line listed on the fixture's path) found and
    fixed.

### Phase 2: The Admission Window Counts Readings

- [x] `[AI]` RED: in `tests/support/driver.go`, add `readingStall time.Duration` after `Driver.lineage`, with a comment
      saying it is real time the degraded-lineage admission collector takes over each reading, standing in for a runner
      the scheduler starves, zero in every scenario. In `tests/support/degraded_lineage.go`, add `stall time.Duration`
      to `advancingCollector`, call `time.Sleep(collector.stall)` in `Collect` after its context check, and set
      `stall: driver.readingStall` in `guardUnder`'s collector. Add `TestDegradedAdmissionIgnoresARunnerStall` to
      `tests/support/degraded_lineage_internal_test.go` with rows `profileBalanced` requiring
      `(*Driver).requireConfiguredDegradedAdmission` and `profileConstrained` requiring
      `(*Driver).requireConfiguredDeferral`; each row builds `&Driver{readingStall: 10 * time.Millisecond}`, defers
      `Close`, and calls `stableDarwinWarning`, `derivedProfileConfiguration(row.base)`, `guardUnderConfiguration`, and
      the row's requirement. Run the _Regression test_; proof: the `balanced` row fails 3 runs of 3 with
      `profile "local-balanced": exit=0 stderr="HIPPO deferred task: safe admission was not reached.\n"`, the
      `constrained` row and `TestTheGuardReadsDegradedAdmissionFromTheLineageAlone` pass, and _Focused scenarios_ pass
      at both adapters. `[AC-01]` `[AC-02]`
  - Result: (2026-10-06) _Regression test_ at load averages 23–27: `TestDegradedAdmissionIgnoresARunnerStall/balanced`
    failed 3 runs of 3, each at `degraded_lineage_internal_test.go:72` with
    `profile "local-balanced": exit=0 stderr="HIPPO deferred task: safe admission was not reached.\n"`; `/constrained`
    passed 3 of 3 and `TestTheGuardReadsDegradedAdmissionFromTheLineageAlone` (both rows) passed 3 of 3 (18 test results
    in all: 12 passed, 6 failed, the six being the `balanced` rows and their three parents). _Focused scenarios_: unit
    and integration adapters each printed `346 scenarios (2 passed, 344 undefined)`, `8 steps (8 passed)`, and `PASS`
    for both scenarios.
- [x] `[AI]` GREEN: in `tests/support/degraded_lineage.go`, add `type logicalClock struct{ now time.Time }` with
      `Now() time.Time` returning `now` and `Sleep(duration time.Duration)` adding `duration` to it; in `guardUnder`,
      create `clock := &logicalClock{now: time.Now()}` and pass `clock.Sleep` and `clock.Now` in place of `time.Sleep`
      and `time.Now`, keeping the 100 ms `AdmissionWindow`; proof: the _Regression test_ passes 3 runs of 3, and
      _Focused scenarios_ pass at both adapters. `[AC-01]` `[AC-02]` `[AC-03]`
  - Result: (2026-10-06) `logicalClock` added and `guardUnder` runs on `clock.Sleep` and `clock.Now`, window still 100
    ms. _Regression test_ at load averages 31–44 passed 3 runs of 3: `balanced` 0.27–0.38 s, `constrained` 1.40–1.48 s
    (the 101 stalled readings of 10 ms each, with no wall-clock deadline to end them early), and both
    `TestTheGuardReadsDegradedAdmissionFromTheLineageAlone` rows. _Focused scenarios_: both adapters printed
    `346 scenarios (2 passed, 344 undefined)`, `8 steps (8 passed)`, and `PASS`; the admitting scenario took 0.08 s
    (unit) and 0.06 s (integration), the deferral scenario 0.03 s and 0.02 s, against 0.12–0.17 s and 0.13 s before.
- [x] `[AI]` REFACTOR: give `logicalClock` a comment saying time passes only when the guard pauses, so the window counts
      the guard's own readings, and amend `guardUnder`'s comment to say its window is logical; leave
      `superviseLineageChild` on the real clock; proof:
      `git grep -c -E 'time\.(Sleep|Now),' -- tests/support/degraded_lineage.go` prints
      `tests/support/degraded_lineage.go:2`, `go tool golangci-lint run ./tests/support/...` prints `0 issues.`, and the
      _Regression test_ and _Focused scenarios_ still pass. `[AC-03]` `[AC-04]`
  - Result: (2026-10-06) `logicalClock` carries the comment that time passes only when the guard pauses and the window
    counts the guard's own readings, and `guardUnder`'s comment says its window is logical.
    `git grep -c -E 'time\.(Sleep|Now),' -- tests/support/degraded_lineage.go` printed
    `tests/support/degraded_lineage.go:2` (`superviseLineageChild` alone);
    `go tool golangci-lint run ./tests/support/...` printed `0 issues.`; `gofmt -l tests/support` printed nothing.
    _Regression test_ passed 3 runs of 3 (18 `--- PASS` lines) and _Focused scenarios_ passed at both adapters
    (`346 scenarios (2 passed, 344 undefined)`).
- [x] `[AI]` Commit this phase's three files as one `test(support)` commit; proof: `git log --oneline origin/main..HEAD`
      lists it and `git diff --name-only HEAD -- tests` prints nothing. `[AC-04]`
  - Result: (2026-10-06) `c764eae` `test(support): count the degraded-lineage admission window in readings`, three
    files, 56 insertions and 4 deletions, through the commit hooks (`public-safety-tree`, `format-staged`,
    `public-safety-message`, and `commit-message` passed, none bypassed). `git log --oneline origin/main..HEAD` lists
    `c764eae` and `260fec2`; `git diff --name-only HEAD -- tests` printed nothing.

### Phase 3: Review and Documentation

- [x] `[AI]` Run the
      [Gherkin implementation review](../../../repo-governance/workflows/quality/gherkin-implementation-review.md) on
      the two scenarios, with these break tests, each reverted after it fails and checked with
      `git diff --quiet -- internal tests`: restoring `time.Sleep` and `time.Now` in `guardUnder` fails the `balanced`
      row of the _Regression test_ with the reported message; making `LineageBalanced.DegradedAdmission()`
      (`internal/policy/profiles.go`) return `false` fails "A configured profile derived from balanced admits degraded
      work" at the unit adapter and the `balanced` row with the reported message; moving `LineageConstrained` into the
      `true` case fails "A configured profile outside balanced's lineage never uses degraded admission" and the
      `constrained` row with `profile "local-constrained": reason=0 exit=0` followed by the degraded-admission line.
      Proof: each scenario's status and each break test's failure recorded here, none `untested`, `unimplemented`, or
      `drifted`. `[AC-03]`
  - Result: (2026-10-06) Frozen list: the two scenarios of `specs/behaviours/admission.feature`, lines 29–33 and 35–40;
    their text and `tests/contract/contract.go` entries are unchanged. Both **implemented**. Implementation:
    `Lineage.DegradedAdmission` (`internal/policy/profiles.go`, line 50), read by the admission loop
    (`internal/guard/run.go`, line 910) and driven by `guardUnder`. Tests: the step bindings
    `requireConfiguredDegradedAdmission` and `requireConfiguredDeferral` at the unit and integration adapters,
    `TestDegradedAdmissionIgnoresARunnerStall` (both rows), and `TestTheGuardReadsDegradedAdmissionFromTheLineageAlone`.
    Break tests, each reverted from a scratch copy and checked with `git diff --quiet -- internal tests` (exit `0`) and
    `cmp` (byte-identical; `shasum -a 256` of `profiles.go` `e1824cf6…` and `degraded_lineage.go` `a0ddb438…` as
    before): (1) `time.Sleep` and `time.Now` restored in `guardUnder` (with `_ = clock` to compile): the `balanced` row
    failed 3 runs of 3 with
    `profile "local-balanced": exit=0 stderr="HIPPO deferred task: safe admission was not reached.\n"`, the
    `constrained` row and the lineage test passed, and both scenarios still passed at the unit adapter (3 runs of 3), as
    the plan expects, because only the stall test slows the runner. (2) `LineageBalanced.DegradedAdmission()` returning
    `false`: the unit adapter failed "A configured profile derived from balanced admits degraded work" at its `Then`
    step (`admission.feature:33`) with
    `Error: profile "local-balanced": exit=0 stderr="HIPPO deferred task: safe admission was not reached.\n"`, the
    `balanced` row failed with that same message, the lineage test's balanced row failed with
    `admitted degraded = false, want true`, and the outside-lineage scenario passed. (3) `LineageConstrained` moved into
    the `true` case: the unit adapter failed "A configured profile outside balanced's lineage never uses degraded
    admission" (`admission.feature:40`) with `Error: profile "local-constrained": reason=0 exit=0` followed by
    `stderr="HIPPO admitting ephemeral child under stable macOS warning pressure with concurrency 1.\n"`, the
    `constrained` row failed with the same `profile "local-constrained": reason=0 exit=0` line, the lineage test's
    constrained row failed, and the admitting scenario passed. Statuses: none `untested`, `unimplemented`, or `drifted`.
- [x] `[AI]` Run [docs propagation](../../../repo-governance/workflows/quality/docs-propagation.md) and record that no
      page describes the fixture's clock and that, per [Release content](#solution), `CHANGELOG.md` gets no entry;
      proof: `git diff --name-only origin/main...HEAD -- README.md docs specs CHANGELOG.md` prints nothing. `[AC-04]`
  - Result: (2026-10-06) No page describes the fixture's clock. A search of `README.md`, `docs/`, `specs/`,
    `CHANGELOG.md`, and `repo-governance/` for `advancingCollector`, `guardUnder`, `logicalClock`, `readingStall`,
    `evidenceDecidesAdmission`, `wall clock`, `logical clock`, `injectable clock`, `admission window`, and
    `safe admission was not reached` found only production wording (`docs/how-to/respond-to-exit-codes.md` lines 70–72,
    `docs/reference/resource-policy.md` line 103, `CHANGELOG.md` line 16), which the fix leaves true, and plan records
    that still name `advancingCollector` correctly. Per [Release content](#solution) `CHANGELOG.md` gets no entry.
    `git diff --name-only origin/main...HEAD -- README.md docs specs CHANGELOG.md` printed nothing. Status `no-change`.

### Phase 4: Verification

- [x] `[AI]` Bounded checkpoint: run the _Repeated tests_, then the _Repeated scenarios_, recording `uptime` before and
      after each; proof: both exit `0`, with 50 `--- PASS` lines for each of the four support subtests and 10 for each
      scenario. Fallback, decided now: one failure stops the plan before landing; its output is recorded here, the cause
      it shows replaces the matching part of [Root Cause](#root-cause), and nothing lands until a new RED proves that
      cause. `[AC-05]`
  - Result: (2026-10-06) _Repeated tests_ exit `0`: 50 `--- PASS` for each of `TestDegradedAdmissionIgnoresARunnerStall`
    (`balanced`, `constrained`) and `TestTheGuardReadsDegradedAdmissionFromTheLineageAlone` (both rows), no `--- FAIL`;
    _Repeated scenarios_ exit `0`: 10 `--- PASS` for each of the two scenarios, no `--- FAIL`. `uptime` load averages
    15.02 / 24.06 / 23.63 before, 23.10 / 20.34 / 21.92 between, 26.56 / 31.07 / 30.03 after.
- [x] `[AI]` Run the _Contention check_ on the fix branch's head, recording `uptime` before and after; proof: it logs
      `deferred 0 of 200`, and `test ! -e tests/support/zz_contention_scratch_test.go` exits `0` once it is deleted.
      Fallback, decided now: as for the checkpoint above. `[AC-05]`
  - Result: (2026-10-06) `TestContentionScratch` logged `deferred 0 of 200` (8 concurrent runs x 25, `GOMAXPROCS=1`,
    `-race`; `ok` in 77 s), against 53 of 200 before the fix; load averages 21.35 / 29.31 / 29.43 before and 20.81 /
    26.60 / 28.34 after; the scratch file was deleted and `git status --porcelain` lists only this README.
- [x] `[AI]` Run the _Full gate_ on the branch head; proof: exit `0`, ending with `No vulnerabilities found.` `[AC-05]`
  - Result: (2026-10-06) `GOFLAGS=-timeout=30m npm test` at `c764eae`, the form the repository's loaded-host runner
    uses, exit `0` at load 22–32 (`uptime` 31.77 before, 24.71 after): selected production line coverage 99.35%
    (911/917), race detector clean, ending with "No vulnerabilities found."
- [x] `[AI]` Before landing, commit the execution record so far as a `docs(plans)` commit on the fix branch, because
      `git rebase` refuses a dirty tree and the rebase never auto-stashes; then `git fetch origin --tags` and confirm
      `v0.8.5` does not yet exist; then rebase onto `origin/main`, reading the whole incoming diff (the linting plan's
      units edit `tests/support/degraded_lineage.go` and `tests/support/driver.go`), and rerun the _Regression test_,
      _Focused scenarios_ at both adapters, and the _Full gate_ if the rebase brought commits; proof:
      `git status --porcelain` prints nothing before the rebase, `git ls-remote --tags origin v0.8.5` prints nothing,
      and the reruns exit `0`. If the tag already exists, land anyway and the Recovery item in Phase 5 fires. `[AC-05]`
      `[AC-06]` `[AC-07]`
  - Result: (2026-10-07) the record so far was committed as `3fff2ac` on a clean tree, and
    `git ls-remote --tags origin v0.8.5` printed nothing before each rebase. Main gained the linting plan's Unit 5
    (`e261965`, which edits `tests/support/driver.go`), so the branch was rebased without conflict; the _Regression
    test_ passed 3 of 3 and the _Focused scenarios_ passed at both adapters. Main then gained the cancelled-waiter fix
    (`aa274ee`), so the branch was rebased again without conflict to `86b06f4`: the _Regression test_ passed 3 of 3, the
    _Focused scenarios_ passed 2 of 2 at each adapter, and the _Full gate_ exited `0` at load 5.5–12.7 with selected
    production line coverage 99.36% (928/934), race detector clean, ending with "No vulnerabilities found."
- [x] `[AI]` Confirm the change stays inside its boundary; proof: `git diff --name-only origin/main...HEAD` prints only
      the paths in [File Impact](#file-impact). `[AC-04]`
  - Result: (2026-10-07) `git diff --name-only origin/main...HEAD` at `aa274ee` prints this README,
    `tests/support/degraded_lineage.go`, `tests/support/degraded_lineage_internal_test.go`, and
    `tests/support/driver.go`: exactly the File Impact paths.
- [x] `[AI]` Commit the rest of this plan's execution record (Phases 1–4 ticked with the rebase, rerun, and boundary
      results) as a `docs(plans)` commit on the fix branch, as the last commit before landing; proof:
      `git status --porcelain` prints nothing. `[AC-07]`

### Phase 5: Release Through v0.8.5

- [ ] `[AI]` Land unit 2 with _Land_; proof: the merge commit on `origin/main` and _Reconcile_ reading `0 0`, both
      recorded here by unit 4, since the merged copy cannot hold its own merge. The fix must merge before the linting
      plan's Unit 7 tags `v0.8.5`. `[AC-01]` `[AC-02]` `[AC-03]` `[AC-04]` `[AC-05]`
- [ ] `[AI]` Immediately after that merge, run
      [dev artifact clean-up](../../../repo-governance/workflows/maintenance/dev-artifact-clean-up.md) for this
      worktree, at the owner's direction: confirm nothing is unpushed or running, then remove the worktree with
      `git worktree remove` without `--force`, and delete `worktree/fix-degraded-lineage-scenario-flake` and `...-fix`
      locally and on `origin`; proof: `git worktree list` omits it,
      `git branch --list 'worktree/fix-degraded-lineage-*'` and
      `git ls-remote origin 'refs/heads/worktree/fix-degraded-lineage-*'` print nothing. `[AC-07]`
- [ ] `[AI]` Provision `worktrees/fix-degraded-lineage-scenario-flake-record` from `origin/main` on branch
      `worktree/fix-degraded-lineage-scenario-flake-record`, with `npm ci`, once `v0.8.5` is published; proof:
      `git branch --show-current` prints the branch. `[AC-07]`
- [ ] `[AI]` Once `v0.8.5` is published, record the release in the record worktree's copy of this plan; proof:
      `git merge-base --is-ancestor <unit 2 merge commit> v0.8.5` exits `0`, and the release URL and the tag's peeled
      commit are recorded here. `[AC-06]`
- [ ] `[AI]` Recovery, dormant until triggered. Trigger: `v0.8.5` is published without unit 2's merge commit (the
      ancestry check above exits `1`). Then cut no patch release, because the binary is unchanged: record in the record
      worktree's copy of this plan that the fix is test-only with no release content and that `v0.8.5`'s full gate ran
      without it, with the result the linting plan's Unit 7 recorded for it; proof:
      `git merge-base --is-ancestor <unit 2 merge commit> origin/main` exits `0` and the `v0.8.5` check exits `1`, both
      recorded there, so the next release cut from `origin/main` carries the fix. Otherwise: a dated, evidenced
      `Not triggered`. `[AC-06]`

### Phase 6: Close

- [ ] `[AI]` Route each learning below to its durable owner, or discard it with a reason; proof: each entry names its
      owner or its reason. `[AC-07]`
- [ ] `[AI]` Run the [execution check](../../../repo-governance/workflows/plan/plan-execution-check.md); proof: its
      verdict line recorded here. `[AC-07]`

### Archival

- [ ] `[AI]` Move this folder with `git mv` to `plans/done/<completion date>__fix-degraded-lineage-scenario-flake/`,
      update `plans/in-progress/README.md` and `plans/done/README.md` in the same change, and land it with _Land_;
      proof: the merge commit, posted on the archival pull request, and no copy left under `plans/in-progress/`.
      `[AC-07]`
- [ ] `[AI]` Run [dev artifact clean-up](../../../repo-governance/workflows/maintenance/dev-artifact-clean-up.md) for
      the record worktree right after that merge; proof, posted on the archival pull request: `git worktree list` omits
      it, no local or remote `worktree/fix-degraded-lineage-*` branch remains, and _Reconcile_ reads `0 0`. `[AC-07]`

## Learnings

Entries are added as execution teaches something, and each is routed to a durable owner or discarded with a reason
before archival.

- (2026-10-06) _The stall test alone guards the clock._ Restoring the wall clock in `guardUnder` fails only the
  `balanced` row of `TestDegradedAdmissionIgnoresARunnerStall`; both scenarios and the lineage test keep passing on an
  unloaded runner, since only that test slows the readings. That is the design (the scenarios stay load-independent
  specifications), but it makes the stall test the sole mutation guard of the clock. Route: none needed; discard, the
  plan's break test 1 already pins it.
- (2026-10-06) _A hoisted collector, not an inline one._ Setting `stall` inside the multi-line `advancingCollector`
  literal in `guard.RunConfig` made `gofmt` realign every neighbouring field of the struct literal. `guardUnder` builds
  the collector in a local `collector` instead, so the diff stays minimal; the plan's wording ("set `stall` in
  `guardUnder`'s collector") holds.
- (2026-10-06) _`rtk` rewrites `go test` and hides `--- PASS` lines._ The pre-tool hook turns `go test -v` into a JSON
  summary, which cannot show the 50 `--- PASS` lines Phase 4's checkpoint counts. `rtk proxy go test …` prints Go's raw
  output. Route: Phase 4 executor; no repository change.
- (2026-10-06) _The deferral rows now cost 1.4 s._ With the window logical, `constrained` takes 101 stalled readings of
  10 ms each (1.40–1.48 s per run) where the old fixture ended at 100 ms of wall clock; the scenarios themselves fell
  from 0.12–0.17 s to 0.02–0.08 s. 50 repeats of the _Regression test_ add about a minute. Route: none; discard, the
  cost is the stall the test asks for.

## Directory Map

This plan is one document, so this README has no siblings to map.

[go-sleep]: https://pkg.go.dev/time#Sleep
[go-runtime]: https://pkg.go.dev/runtime
[go-testflag]: https://pkg.go.dev/cmd/go#hdr-Testing_flags
