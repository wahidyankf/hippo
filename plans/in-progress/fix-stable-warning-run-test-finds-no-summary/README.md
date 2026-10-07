# Fix: Stable-Warning Run Test Finds No Summary

Status: In progress — authored 2026-10-07.

Since 2026-10-07T00:00Z the unit test `TestARunSparesAStableWarningByThePolicyItAdmitsAgainst` fails on every run with
"summary paths=[] error=<nil>, want one summary". Its collector dates every sample from a fixed instant on 2026-10-06,
the run's summary takes its day from those samples, and the run's own end-of-run evidence cleanup, which reads the wall
clock, moves any loose summary dated before today into the daily history archive. On any later UTC day, the summary the
test looks for is archived before the test looks. The defect is in the test alone; the cleanup behaves as specified.

This is a [bug-fix plan](../../../repo-governance/conventions/plans/010-bug-fix-plan.md). The owner directed on
2026-10-06 that every flake found be fixed. This defect is deterministic rather than flaky, and it fails the required
`Quality gate` of every pull request, so it blocks all work in this repository with no workaround: no retry passes, and
skipping the test would weaken a gate, which the
[upstream tool defects](../../../repo-governance/development/upstream-tool-defects.md) standard forbids. As for the
sibling plans
[`fix-corrupt-waiter-identity-test-flake`](../../done/2026-10-07__fix-corrupt-waiter-identity-test-flake/README.md) and
[`fix-distinct-root-lock-test-flake`](../../done/2026-10-07__fix-distinct-root-lock-test-flake/README.md), the owner's
direction is the standing request [plan lifecycle](../../../repo-governance/conventions/plan-lifecycle.md) accepts, and
it also directs this plan's quality gate and execution.

**Release.** The change is test-only, so it carries no release content: no `CHANGELOG.md` entry and no tag. `v0.8.5`,
published 2026-10-06T22:19:10Z, contains the test but its binary is unaffected; the next release cut from `origin/main`
carries the fix.

Line numbers in this plan are at `0761ebf`, the trunk commit it was written from, unless a sentence names another commit
or tag.

## Bug Report

**Description.** On any UTC day after 2026-10-06, `TestARunSparesAStableWarningByThePolicyItAdmitsAgainst` fails its
final assertion because the summary of the run it supervised has already been compacted into
`history/2026-10-06.jsonl.gz` by that run's own evidence cleanup.

**Steps to reproduce**, from a clean checkout of `wahidyankf/hippo` at `0761ebf` with `npm ci` run, on any UTC date
after 2026-10-06:

1. `go test -count=3 -v -run '^TestARunSparesAStableWarningByThePolicyItAdmitsAgainst$' ./internal/guard`.
2. To see where the summary went, add the _Probe copy_ under [Delivery](#delivery) through a build overlay and run the
   same command with `-overlay <file>`.

**Expected behaviour.** The test's own documentation says it holds the supervision loop's stable-warning exemption to
`RunConfig.Policy` (`internal/guard/run_test.go`, lines 230–234), and its last line requires the one summary the run
left in its root to record `passed` (lines 263 and 319–326). Nothing in the test depends on the calendar date.

**Actual behaviour.** Step 1, run 2026-10-07T00:22Z, failed 3 runs of 3 (0.04–0.05 s each):

```text
run_test.go:263: summary paths=[] error=<nil>, want one summary
--- FAIL: TestARunSparesAStableWarningByThePolicyItAdmitsAgainst (0.05s)
```

The same command with `-count=30`, run 2026-10-07T00:17Z, failed 30 of 30. Step 2 logged, in each failed run,
`archives=[…/history/2026-10-06.jsonl.gz]` beside the same failure. In CI, run
[37549931665](https://github.com/wahidyankf/hippo/actions/runs/37549931665) (pull request #151, a documentation-only
archival on head `2e227d0` over base `0761ebf`, started 2026-10-07T00:03:43Z) failed `Test (ubuntu-24.04)` and
`Test (macos-15)` on this test alone with the same line, and `Repository contract`, whose log reports only
`[gate] quick failed`; that gate runs `go test ./internal/...` (`scripts/test-quick.sh`, line 35). Run
[37547772023](https://github.com/wahidyankf/hippo/actions/runs/37547772023), on `fccc679` between 23:39Z and 23:58Z on
2026-10-06, passed every job; pull requests #148–#150 merged that day.

**Error output** is the `run_test.go:263` line quoted above; the failing check is at lines 324–325.

**Environment.** macOS 15.5 on Apple M2 Max with 12 cores, Go 1.27.1 (the local toolchain; `go.mod` requires 1.26.1),
Node 24.16.0, at `0761ebf` (`origin/main`); and GitHub's `ubuntu-24.04` and `macos-15` runners in the CI run above.

**Regression.** The test and its fixed instant arrived in `3eedac4`, "refactor(policy): decide the admission path once
for run and status" (pull request #140, merged 2026-10-06T16:13Z). It passed for the rest of that UTC day and has failed
since midnight. The tag `v0.8.5` (`456d24b`) carries the same lines (202, 218, and 263); its release gate ran on
2026-10-06, so the published binary, which no test file reaches, is unaffected.

## Duplicate Check

Run 2026-10-07 against `origin/main` at `0761ebf`:

- `gh issue list --state open` returned no issues; `gh issue list --state all` returned none either.
  `gh issue list --state all --search` for `TestARunSparesAStableWarningByThePolicyItAdmitsAgainst`, `StableWarning`,
  `stable warning`, `summary paths`, `want one summary`, `compactSummaries`, `stableWarningStart`, `compaction`,
  `evidence cleanup`, `midnight`, `UTC day`, `fixed date`, and `flake` each returned nothing.
- `gh pr list --state open` returned only #151, the archival of `fix-cancelled-waiter-cleanup-flake`, which this defect
  blocks. `gh pr list --state all --search` for the test's name, `StableWarning`, `compactSummaries`,
  `stableWarningStart`, `compaction`, `midnight`, and `UTC day` returned nothing. The other terms returned only merged
  or closed work that leaves this collector unchanged, apart from #140, which introduced it: for `stable warning`, #89,
  #90, #121, #122, #123, #126, and #140; for `evidence cleanup`, #3, #79, #80, #87, #89, #90, #94, #140, and #146; for
  `summary paths`, `want one summary`, and `fixed date`, #17, #20, #80, #81, #83, #90, #91, #92, #94, #95, and #98; for
  `flake`, the four sibling flake plans, their fixes, and their archivals (#136–#139, #141–#144, #148–#151), and #27,
  #80, #81, #83, #90, #93, #94, and #135.
- `plans/backlog/` holds no plan; `plans/in-progress/` holds the linting plan and `fix-cancelled-waiter-cleanup-flake`;
  `plans/ideas/` holds one brief, `q4-not-urgent-not-important/degraded-admission-release-audit-follow-ups.md`, about
  three documentation statements.
- `git grep -n` for `ARunSparesAStableWarning`, `stableWarningRunCollector`, and `stableWarningStart` outside
  `internal/guard/run_test.go` matched only the linting plan's `delivery.md`, line 1651, which records why the test was
  written. Over `plans/done`, `repo-governance`, `docs`, and `specs`, those names and `compactSummaries` and
  `want one summary` matched nothing.
- `git log -i -E --grep` for `stableWarning|summary paths|compact|midnight|UTC day` found only `b72e728`, "fix: read
  multiline compacted history", an unrelated reader fix.
- No web search tool was available to this session, so no search for outside reports was made; the defect is in HIPPO's
  own test, which the searches above cover.

No duplicate exists. Related, not a duplicate: #90 (`abc9094`), "test: make the wall-clock flaky tests deterministic",
repaired other tests whose verdict depended on the clock.

**Other fixed-date fixtures.** `grep -rn 'time\.Date(20'` over `internal/` and `tests/` test code finds this anchor and
31 others, in `internal/evidence/history_test.go`, `internal/cli/{development,history,interruption,refusal}_test.go`,
and `tests/unit/reservation_test.go`. Each passes its one fixed `now` both to the samples it writes and to the clock
that cleans up (`Cleanup(root, now)`, or `Now: func() time.Time { return now }`, with `stableDevelopmentSample(now)`),
so sample day and cleanup day never differ. The fixtures in `tests/support/` anchored at `time.Unix(0, 0)` are always in
the past, so their summaries are archived the same way every day; their outcome does not depend on the date. None shares
this defect: `go test -count=1 ./internal/... ./tests/unit ./tests/support`, run 2026-10-07T00:24Z, failed this one test
alone, and the CI run above failed it alone across the whole gate.

## Root Cause

**The samples carry a fixed date.** `stableWarningStart` is 2026-10-06T08:00:00Z (`run_test.go`, line 202), and the
collector stamps sample _n_ with `stableWarningStart + n s` (line 218). The test builds it with `finishAfter: 20` (line
245), so the child exits after the sample numbered 19. The run itself gets the wall clock, `Now: time.Now` (line 253).

**The summary takes its day from the samples.** `EvidenceWriter.Append` sets `summary.FinishedAt = sample.MeasuredAt`
for each sample (`internal/guard/evidence.go`, line 161), so the summary's `finishedAt` is the last sample's date,
2026-10-06T08:00:19Z or later. `Finalize` writes it as the root's loose `<id>.summary.json` (lines 232 and 285).

**The run's cleanup archives summaries dated before today.** Right after `Finalize`, the run's `finalize` closure calls
`evidence.Cleanup(config.EvidenceRoot, config.Now())` (`internal/guard/run.go`, lines 844–845). `Cleanup` takes the
writer lock and runs `cleanupLocked` (`internal/evidence/retention.go`, lines 199–214 and 94), whose `compactLocked`
(line 109; `internal/evidence/history.go`, lines 681–689) calls `compactSummaries` with that `now`. There, `today` is
the UTC day of `now` (line 356). Each loose summary not protected by an active writer (line 363) and not expired by its
file age (line 373) gets its day from `summaryArchiveDay` (lines 341–349), which reads `summaryTime` (lines 79–85), the
parsed `finishedAt`. A day before `today` (line 384) is merged into `history/<day>.jsonl.gz` and the loose file removed
(lines 388–440).

**So the test's glob finds nothing.** On 2026-10-06, the summary's day equalled `today` and the file stayed. From
2026-10-07 it is archived, and `requireSummaryOutcome` globs `root/*.summary.json` (line 323) after `Run` returns,
finding none (line 325).

**Evidence**, each through a scratch build overlay run 2026-10-07 at 00:23Z, 3 runs of 3:

- The _Probe copy_ logged `archives=[…/history/2026-10-06.jsonl.gz]` beside each failure.
- The _Back-day copy_, which only moves the run's clock to 2026-10-06T08:01Z (advancing in real time), passed: the date
  difference alone decides the verdict.
- The fixed collector of [Solution](#solution), with a probe that also read the loose summary, passed and logged one
  whose `finishedAt` was 2026-10-07T00:23:54.24Z while the wall clock read 00:23:35.28Z.

## Solution

**Date the samples from the wall clock.** The collector gains a `start time.Time` field, set to `time.Now().UTC()` where
the test builds it, and stamps sample _n_ with `start + n s`; the fixed `stableWarningStart` is removed. Samples stay
one second apart by the collector's own clock, so the one-second trend window is still full from the first warning
sample with no waiting, which is what the fixed instant was for (lines 200–207). The run's summary is then dated at
least 19 s after the wall clock at which the test started, and `compactSummaries` archives only a summary whose day is
before the cleanup's (`history.go`, line 384). The cause, a sample clock independent of the run's clock, is gone: both
now read the wall clock, as `controlledRunCollector` already does (line 39).

**Nothing compares the sample dates with the wall clock.** `policy` reads `MeasuredAt` only to check that it parses
(`internal/policy/policy.go`, line 156) and to measure windows between samples (lines 222–235 and 350–353). The
supervision loop times its warning grace and admission deadline with `config.Now()` alone (`run.go`, lines 852 and
1101–1114). So samples dated up to 19 s ahead of the wall clock change no decision, and no HIPPO behaviour rejects a
future-dated sample. `go doc time` confirms the comparison is on wall time: `UTC` strips the monotonic reading, and
`time.Parse` creates times without one, so `Before` falls back to the wall clock.

The test's lines 200–228 become:

```go
// stableWarningRunCollector samples a host that admits the run at once and then
// holds a macOS memory warning steady, one second apart by its own clock, so
// a one-second trend window is full from the first warning sample, with no
// waiting. Its child exits on the sample numbered finishAfter.
type stableWarningRunCollector struct {
	// start dates the first sample. It is the wall clock when the test starts,
	// never a fixed date: the run's evidence cleanup archives a summary dated
	// before its own day, and the test reads the loose summary the run left.
	start       time.Time
	calls       int
	finishAfter int
	exited      chan error
}

func (collector *stableWarningRunCollector) Collect(
	ctx context.Context, previous policy.CPUState, diskPath string,
) (policy.Reading, error) {
	reading, err := (&controlledRunCollector{}).Collect(ctx, previous, diskPath)
	reading.Sample.MeasuredAt = collector.start.Add(time.Duration(collector.calls) * time.Second).Format(time.RFC3339Nano)
	if collector.calls > 0 {
		reading.Sample.MemoryPressureLevel = new(2)
	}
	collector.calls++
	if collector.calls == collector.finishAfter {
		collector.exited <- nil
	}

	return reading, err
}
```

and line 245 becomes
`collector := &stableWarningRunCollector{start: time.Now().UTC(), finishAfter: 20, exited: make(chan error, 1)}`.
Nothing else in the file changes. A scratch module copy with this file printed `0 issues.` from
`go tool golangci-lint run ./internal/guard/...`, and `gofmt -l` and `go vet` were clean.

**Residual risk.** The summary is archived only if the cleanup's wall clock reads a later UTC day than the summary's
`finishedAt`, which is at least `start + 19 s`: the run would have to last more than 19 s and span 00:00Z, or the wall
clock step forward by more than that across midnight mid-run. The test took 0.02–0.05 s in every run above, locally and
in CI. Such a failure would print this plan's message; it is not addressed further.

**Conditions beyond the reported one.** A test starting just before midnight now dates its summary on the next day,
which `compactSummaries` keeps (only a day before `today` is archived). The fixture keeps its power to fail: with the
run's exemption removed, or judged by `config.Resolution.Policy`, the fixed test fails (see the break tests in
[Delivery](#delivery)); the second is the mutation the linting plan recorded when it wrote the test.

**Alternatives rejected.**

- _Freeze the run's clock at the fixed date_ (`Now: func() time.Time { return stableWarningStart }`). The cleanup then
  keeps the summary, but the warning grace is timed by `config.Now().Sub(*warningSince)` (`run.go`, line 1114), which a
  frozen clock holds at zero, so nothing is ever shed. A scratch overlay passed 3 of 3 with the exemption removed:
  assertion theatre, per
  [Gherkin implementation review](../../../repo-governance/workflows/quality/gherkin-implementation-review.md).
- _Run the whole test on 2026-10-06_, an advancing clock offset to the fixed date (the _Back-day copy_). It passes, but
  every receipt and lease the run writes is then dated before the files' real modification times, so age rules that
  compare the two, such as the negative-age branch at `retention.go`, lines 133–134, see files from the future, which
  the test does not mean to exercise; the gap grows daily.
- _Anchor a day ahead_ (`time.Now().UTC().AddDate(0, 0, 1)`). It removes even the residual, but keeps the sample clock
  apart from the run's clock and dates the summary a day after the run's other records, which a reader would take as
  meaningful.
- _Read the summary from the history archive, or skip or retry the test._ Each changes what the test asserts, or weakens
  a gate, which the upstream tool defects standard forbids. `requireSummaryOutcome` is also shared with
  `TestARunWhoseResolutionAlreadyStopsNeverLaunches` (line 314).

**Public contract.** No exit status, `hippo.*` code, JSON document, configuration key, evidence shape, or production
code moves; the change is confined to `internal/guard/run_test.go`.

**Release content.** None. `CHANGELOG.md` must be true to the shipped binary
([documentation architecture](../../../repo-governance/conventions/documentation-architecture.md), line 19), and the
binary is unchanged; the sibling test-only fixes carried no entry either.

**References**, read 2026-10-07 from the Go 1.27.1 distribution's `go doc` and `go help`:

- [Package time: Monotonic Clocks][go-time] — "t.In, t.Local, and t.UTC … strip any monotonic clock reading", and
  "time.Parse … always create times with no monotonic clock reading"; "If either t or u contains no monotonic clock
  reading, these operations fall back to using the wall clock readings."
- [Package time: Now][go-now] — "Now returns the current local time"; [Time.UTC][go-utc] — "UTC returns t with the
  location set to UTC".
- [Build flags: `-overlay`][go-build] — "a build will run as if the disk file path exists with the contents given by the
  backing file paths", so every proof below leaves the worktree untouched.
- [Testing flags][go-testflag] — `-count`, `-race`, `-run`, and `-v`.
- #90 (`abc9094`), "test: make the wall-clock flaky tests deterministic" — the same class of defect, a verdict that
  depends on the clock, in other tests.

### Specification Changes

None. No scenario binds this test: `git grep -n ARunSparesAStableWarning -- specs tests` prints nothing, so the
[Gherkin implementation review](../../../repo-governance/workflows/quality/gherkin-implementation-review.md) has no row
to judge.

### File Impact

```text
internal/guard/run_test.go                                          stableWarningRunCollector and its construction
plans/in-progress/README.md                                         Directory Map entry
plans/in-progress/fix-stable-warning-run-test-finds-no-summary/README.md  this plan and its execution record
```

### Acceptance Criteria

- **AC-01** Given `0761ebf`'s test on a UTC day after 2026-10-06, when it runs, then it fails with
  `summary paths=[] error=<nil>, want one summary`, and the _Probe copy_ logs `history/2026-10-06.jsonl.gz`.
- **AC-02** Given the fixed test, when it runs, then it passes 30 runs of 30, and 10 of 10 under `-race`.
- **AC-03** Given the fixed test with its collector start and its run clock both advanced 48 h, when it runs, then it
  passes.
- **AC-04** Given the fixed test with its collector start restored to 2026-10-06T08:00:00Z, when it runs, then it fails
  with `summary paths=[] error=<nil>, want one summary`.
- **AC-05** Given a run whose stable-warning exemption is removed, when the fixed test runs, then it fails with
  `a stable warning shed the child it spares:`.
- **AC-06** Given a run that judges the exemption by `config.Resolution.Policy`, when the fixed test runs, then it fails
  with `a stable warning shed the child it spares:`.
- **AC-07** Given the fix branch, then `git diff --name-only origin/main...HEAD` lists only the File Impact paths,
  `git grep -n stableWarningStart -- '*.go'` prints nothing, and nothing under `cmd/`, `specs/`, `docs/`, `tests/`,
  `README.md`, or `CHANGELOG.md`, nor any non-test file under `internal/`, changes.
- **AC-08** Given the fix branch's head, then the full gate exits `0` locally and the pull request's `Quality gate`
  reads `success` on that head.
- **AC-09** Given the fix merges, then `origin/main` contains its merge commit and no commit of the pull request changes
  `CHANGELOG.md`, so the next release cut from `origin/main` carries the fix with no entry and no tag of its own.
- **AC-10** Given execution finishes, then this plan is gated, reconciled, and archived under `plans/done/`, and its
  worktrees and branches are gone.

## Delivery

**Execution checkout.** `worktrees/fix-stable-warning-run-test-finds-no-summary` under the HIPPO repository location,
created from `origin/main` at `0761ebf` on branch `worktree/fix-stable-warning-run-test-finds-no-summary`, with
`npm ci`, per [worktree to pull request](../../../repo-governance/workflows/maintenance/worktree-to-pull-request.md).
HIPPO cannot guard its own gates, so every test and gate command below runs directly, never under `./hippo`, per
[resource-aware development](../../../repo-governance/development/resource-aware-development.md); `npm ci` may be
wrapped.

**Deviation: the plan does not land alone first.** The bug-fix plan convention lands the plan alone before any fix is
committed. Here every pull request's required `Quality gate` fails on this defect, so a plan-only pull request cannot
meet its [merge preconditions](../../../repo-governance/conventions/pull-request-merge.md). The plan and the fix
therefore travel in one pull request whose first commit is this plan, carrying the quality gate's verdict; the later
commits are the fix and the execution record. Rebase-merging puts the plan commit on trunk first, and while the pull
request is open, `gh pr list` shows it to a parallel finder's duplicate check.

**Deviation: a second worktree.** The owner directed on 2026-10-06 that a fix's worktree and branch be removed
immediately after it merges, while [integration path](../../../repo-governance/conventions/integration-path.md)
provisions at most one worktree per plan. As in the sibling plans, unit 2 provisions
`worktrees/fix-stable-warning-run-test-finds-no-summary-record` and removes it after its own merge.

**Dependent, not scope.** Pull request #151 is blocked by this defect; once unit 1 merges, it is rebased onto
`origin/main` and its gate rerun under its own plan.

**Commands** the items below name, run from the worktree root. Proofs count Go's raw `-v` lines (`--- PASS`,
`--- FAIL`); where a shell hook rewrites `go test` output, bypass it (`rtk proxy go test …`).

- _Stable-warning test_: `go test -count=3 -v -run '^TestARunSparesAStableWarningByThePolicyItAdmitsAgainst$'`
  `./internal/guard`. "With an overlay" adds `-overlay <scratch>/<name>.json` before `-count`.
- _Repeated test_: the _Stable-warning test_ with `-count=30`, then with `-race -count=10`.
- _Scratch_: a directory made with `mktemp -d` at the start of the item that uses it and removed with `rm -r` at its
  end; proof of removal: `test ! -e <scratch>` exits `0`. Nothing in it is ever copied into the worktree.
- _Overlay_: `<scratch>/<name>.json` holding `{"Replace":{"<worktree>/<file>":"<scratch>/<copy>"}}`, with the worktree's
  absolute path; an overlay may list two files. The copies, each of the worktree's current file, located by their text:
  - _Probe copy_ of `internal/guard/run_test.go`: immediately before `requireSummaryOutcome(t, root,`
    `evidence.OutcomePassed)` in the test, `archives, _ := filepath.Glob(filepath.Join(root, "history",`
    `"*.jsonl.gz"))` then `t.Logf("probe: archives=%v", archives)`.
  - _Back-day copy_ of the unfixed `internal/guard/run_test.go`, before Phase 2's GREEN: after the `collector := …` line
    of the test, `offset := stableWarningStart.Sub(time.Now()) + time.Minute`, and that test's `Now: time.Now` becomes
    `Now: func() time.Time { return time.Now().Add(offset) }`.
  - _Later-day copy_ of `internal/guard/run_test.go`, after Phase 2: `start: time.Now().UTC()` becomes
    `start: time.Now().UTC().Add(48 * time.Hour)`, and the test's `Now: time.Now` becomes
    `Now: func() time.Time { return time.Now().Add(48 * time.Hour) }`.
  - _Fixed-date copy_ of `internal/guard/run_test.go`, after Phase 2: `start: time.Now().UTC()` becomes
    `start: time.Date(2026, 10, 6, 8, 0, 0, 0, time.UTC)`.
  - _Unspared copy_ of `internal/guard/run.go`: `stableWarning := policy.SparesStableWarning(` (line 1097) becomes
    `stableWarning := false && policy.SparesStableWarning(`.
  - _Resolution-policy copy_ of `internal/guard/run.go`: the last argument of that call, `config.Policy`, becomes
    `config.Resolution.Policy`.
- _Full gate_: `GOFLAGS=-timeout=30m npm test`, the release gate with its per-package timeout raised for the host's
  load, as `scripts/test-loaded.sh` raises it.
- _Reconcile_: `git -C ../.. fetch origin`, then `git -C ../.. merge --ff-only origin/main`, proved by
  `git -C ../.. rev-list --left-right --count HEAD...origin/main` reading `0 0`.
- _Land_: inspect the branch's diff against
  [data safety](../../../repo-governance/conventions/public-repository-data-safety.md); run the
  [push review](../../../repo-governance/workflows/quality/pr-leak-review/002-push-review.md) and push; screen the title
  and body with `scripts/public-safety/outbound-preflight.sh --surface pull-request` and open a draft pull request; mark
  it ready; wait for `Quality gate` on the head, polling no faster than every three minutes; post the
  [leak review](../../../repo-governance/workflows/quality/pr-leak-review.md) for that exact head; rebase-merge once
  every merge precondition holds; then _Reconcile_. If `origin/main` moves after the push, the rebase and its push
  follow [no destructive Git operations](../../../repo-governance/conventions/no-destructive-git-operations.md).

**Delivery units**, landed serially:

1. _Fix_ — this plan and its index entry (first commit), the rewritten collector, and the execution record, in one pull
   request. Rollback: revert its commits; no release depends on them.
2. _Record_ — the landing record, the execution check, and the move to `plans/done/`, from the record worktree.
   Rollback: revert its merge.

**Pause safety.** Each item records its result when ticked. To resume, read the last ticked item, then
`git -C <worktree> log --oneline origin/main..HEAD` and `gh pr list --head <branch>`. A _Scratch_ directory left by an
interrupted item is removed and the item rerun. Between unit 1's merge and unit 2, the record is this file on
`origin/main`, with Phases 1–4 ticked and Phase 5 open.

### Phase 1: Plan

- [x] `[AI]` Write this file and its `plans/in-progress/README.md` entry, then run the entry checks; proof: each of
      `./node_modules/.bin/prettier --check`, `scripts/check-markdown-line-length.sh`, and
      `./node_modules/.bin/markdownlint-cli2` on the two files, `./rhino md internal-link validate`,
      `./rhino governance directory-map validate`, and `./rhino governance word-budget validate` exits `0`. `[AC-10]`
  - Result: (2026-10-07) at 00:35Z, on the uncommitted files over `0761ebf`: `prettier --check`,
    `check-markdown-line-length.sh`, and `markdownlint-cli2` (0 issues) each exited `0`; `internal-link validate`
    checked 1328 links and `directory-map validate` 56 directories, each with no findings; `word-budget validate` exited
    `0`.
- [x] `[AI]` Run the [plan quality gate](../../../repo-governance/workflows/quality/plan-quality-gate.md) on this folder
      in mode `normal`, at most three cycles, with its ledger under `local-tmp/quality/plan/`; proof: one terminal
      `plan-quality-gate:` verdict line recorded here. `[AC-10]`
  - Result: (2026-10-07) `plan-quality-gate: PASS_WITH_FINDINGS (2 cycles, 7 rows fixed; 1 MEDIUM open)`, run by an
    independent `plan-checker` with `plan-fixer` propagation (ledgers `__20261007T0042Z` and `__20261007T0054Z`). Cycle
    1 found PQC-01 (HIGH: the `stableWarningStart` grep proofs matched this plan's own text) and six MEDIUM or LOW rows,
    all repaired; cycle 2 held every repair and left PQC-08 (MEDIUM: the _Full gate_ item's fallback for an unrelated
    failure assumes another fix can merge first, which this defect prevents) open. If that fallback triggers, the
    executor stops for the owner.
- [x] `[AI]` Commit this file and the index entry alone as the branch's first commit,
      `docs(plans): plan the fix for the stable-warning run test's missing summary`; proof:
      `git log --oneline origin/main..HEAD` lists exactly that commit, `git show --name-only --format= HEAD` prints only
      the two paths, and `git status --porcelain` prints nothing. `[AC-07]` `[AC-10]`
  - Result: (2026-10-07) committed as `9055566`, the branch's first commit, on `origin/main` at `0761ebf`. Checked at
    the start of execution, before any other commit: `git log --oneline origin/main..HEAD` listed exactly that commit,
    `git show --name-only --format= HEAD` printed `plans/in-progress/README.md` and this README and nothing else, and
    `git status --porcelain` printed nothing.

### Phase 2: The Collector Dates Samples From the Wall Clock

- [x] `[AI]` RED: run the _Stable-warning test_ without an overlay; then, in a _Scratch_, write the _Probe copy_ and the
      _Back-day copy_ with an overlay each and run the _Stable-warning test_ with each; proof: without an overlay, 3 of
      3 fail at `run_test.go:263` with `summary paths=[] error=<nil>, want one summary`; with the probe, each failure
      also logs `history/2026-10-06.jsonl.gz`; with the back-day overlay, 3 of 3 pass; `git status --porcelain` prints
      nothing; the _Scratch_ is removed. `[AC-01]`
  - Result: (2026-10-07, 00:57Z, load average 9.03 at the start) without an overlay, the _Stable-warning test_ exited
    `1` with 3 of 3 `--- FAIL` (0.03–0.04 s), each at `run_test.go:263` with
    `summary paths=[] error=<nil>, want one summary`. With the _Probe copy_'s overlay, 3 of 3 `--- FAIL` (0.04–0.05 s),
    each logging `probe: archives=[…/history/2026-10-06.jsonl.gz]` beside the same message (the probe's two inserted
    lines move it to line 265). With the _Back-day copy_'s overlay, 3 of 3 `--- PASS` (0.04 s). Every run used raw `-v`
    output through `rtk proxy go test`. `git status --porcelain` printed nothing; the _Scratch_ was removed (`test ! -e`
    exits `0`).
- [x] `[AI]` GREEN: in `internal/guard/run_test.go`, replace lines 200–228 with the block under [Solution](#solution)
      and line 245 with the construction given there, changing nothing else; proof: the _Stable-warning test_ passes 3
      of 3 with no `--- FAIL`, and `git grep -n stableWarningStart -- '*.go'` prints nothing (this plan's own text still
      names it, and is not searched). `[AC-02]` `[AC-07]`
  - Result: (2026-10-07, 00:57Z) the block under [Solution](#solution) now stands from the comment
    `// stableWarningRunCollector samples` to the closing brace of `Collect`, and `diff` of those lines against the
    plan's block printed nothing; line 245 holds the construction given there; nothing else changed (`git diff --stat`:
    6 insertions, 6 deletions, one file). The _Stable-warning test_ passed 3 of 3 (0.03–0.04 s) with no `--- FAIL`, and
    `git grep -n stableWarningStart -- '*.go'` printed nothing (exit `1`).
- [x] `[AI]` REFACTOR: confirm the comment on `start` states why it is the wall clock; proof:
      `go tool golangci-lint run ./internal/guard/...` prints `0 issues.`, `gofmt -l internal/guard` prints nothing,
      `go vet ./internal/guard` exits `0`, and the _Stable-warning test_ still passes 3 of 3. `[AC-07]`
  - Result: (2026-10-07, 00:57Z) the comment on `start` says it is the wall clock when the test starts, never a fixed
    date, because the run's evidence cleanup archives a summary dated before its own day and the test reads the loose
    summary the run left. `go tool golangci-lint run ./internal/guard/...` exited `0` and printed `0 issues.` (with its
    usual warning that `nilaway` is an unknown `//nolint` linter, which predates this change); `gofmt -l internal/guard`
    printed nothing; `go vet ./internal/guard` exited `0`; the _Stable-warning test_ passed 3 of 3.
- [x] `[AI]` Commit `internal/guard/run_test.go` alone as
      `test(guard): date the stable-warning samples from the wall clock`; proof: `git log --oneline origin/main..HEAD`
      lists it after the plan commit, and `git status --porcelain` prints nothing. `[AC-07]`
  - Result: (2026-10-07) `a4cf8c8` `test(guard): date the stable-warning samples from the wall clock`, one file changed
    (6 insertions, 6 deletions), through the pre-commit and `commit-msg` gates (`public-safety-tree`, `format-staged`,
    `public-safety-message`, `commit-message`) with no bypass. `git log --oneline origin/main..HEAD` listed it above the
    plan commit `9055566`, and `git status --porcelain` printed nothing. Hashes in this record are as committed; a
    rebase in the item that follows the record would rewrite them, and the last record commit would then restate them.

### Phase 3: Review and Documentation

- [x] `[AI]` Break tests: in a _Scratch_, write the _Later-day_, _Fixed-date_, _Unspared_, and _Resolution-policy_
      copies with an overlay each (the last two list `run.go` only), and run the _Stable-warning test_ with each; proof,
      each recorded here: later-day passes 3 of 3; fixed-date fails 3 of 3 with
      `summary paths=[] error=<nil>, want one summary`; unspared and resolution-policy each fail 3 of 3 with
      `a stable warning shed the child it spares:`; `git status --porcelain` prints nothing; the _Scratch_ is removed.
      `[AC-03]` `[AC-04]` `[AC-05]` `[AC-06]`
  - Result: (2026-10-07, 00:58Z) each overlay was built from the committed files and run as the _Stable-warning test_
    with `-overlay`, 3 runs. _Later-day copy_: 3 of 3 `--- PASS` (0.03–0.04 s). _Fixed-date copy_: 3 of 3 `--- FAIL`
    (0.03–0.04 s), each at `run_test.go:263` with `summary paths=[] error=<nil>, want one summary`. _Unspared copy_ and
    _Resolution-policy copy_ (each `run.go` only, line 1097): each 3 of 3 `--- FAIL` (0.01–0.02 s), each at
    `run_test.go:261` with
    `a stable warning shed the child it spares: code=0 error=stopped: pressure shed after 2 samples`.
    `git status --porcelain` printed nothing; the _Scratch_ was removed (`test ! -e` exits `0`).
- [x] `[AI]` Run [docs propagation](../../../repo-governance/workflows/quality/docs-propagation.md) and record that no
      page describes this test and that `CHANGELOG.md` gets no entry; proof:
      `git grep -n -e ARunSparesAStableWarning -e stableWarningRunCollector -- README.md docs specs CHANGELOG.md`
      `repo-governance` prints nothing, and `git diff --name-only origin/main...HEAD -- README.md docs specs tests`
      `CHANGELOG.md` prints nothing. `[AC-07]` `[AC-09]`
  - Result: (2026-10-07, 00:59Z) no page describes this test: the `git grep -n` of the test's name and the collector's
    name over `README.md docs specs CHANGELOG.md repo-governance` printed nothing (exit `1`), and
    `git diff --name-only origin/main...HEAD -- README.md docs specs tests CHANGELOG.md` printed nothing. The removed
    name `stableWarningStart` appears in the tracked tree only in this plan's own text, and the test's name appears
    elsewhere only in the linting plan's `delivery.md`, line 1651, which stays true because the test keeps its name. Per
    [Release content](#solution) the test-only change is true to the shipped binary, so `CHANGELOG.md` gets no entry.
    Status `no-change`: nothing stale, nothing removed, no command in an affected document to run.

### Phase 4: Verification

- [x] `[AI]` Run both forms of the _Repeated test_, recording `uptime` before each; proof: each exits `0`, with 30 and
      10 `--- PASS` lines and no `--- FAIL`. Fallback, decided now: one failure stops the plan before landing; its
      output is recorded here, the cause it shows replaces the matching part of [Root Cause](#root-cause), and nothing
      lands until a new RED proves that cause. `[AC-02]`
  - Result: (2026-10-07, at `a4cf8c8`, run directly) `-count=30` at 00:59Z, `uptime` before: load averages 4.67 5.38
    5.64: exit `0`, 30 `--- PASS` lines, 0 `--- FAIL`, `ok` in 1.092 s. `-race -count=10`, `uptime` before: load
    averages 4.46 5.32 5.62: exit `0`, 10 `--- PASS` lines, 0 `--- FAIL`, `ok` in 1.685 s. The fallback did not trigger.
- [x] `[AI]` Run the _Full gate_ on the branch head; proof: exit `0`, ending with `No vulnerabilities found.` A failure
      in another test is recorded here and filed as its own bug-fix plan under the owner's standing direction; this plan
      lands only after that fix merges and a rebase onto it reruns the _Full gate_ clean. `[AC-08]`
  - Result: (2026-10-07) `GOFLAGS=-timeout=30m npm test` at `a4cf8c8`, run directly and never under `./hippo`, from
    00:59:30Z to 01:13:47Z (about 14 minutes; load averages 4.08 5.21 5.57 at the start), exit `0`: selected production
    line coverage 99.38% (955/961 statements), `golangci-lint` `0 issues.`, every package `ok`, ending with
    `No vulnerabilities found.`; `git status --porcelain` printed nothing afterwards. No other test failed, so the
    item's unrelated-failure fallback did not trigger.
- [ ] `[AI]` Commit the execution record so far as a `docs(plans)` commit, then `git fetch origin` and
      `git rebase origin/main`, reading the whole incoming diff, and rerun the _Stable-warning test_ and the _Full gate_
      if the rebase brought commits; a conflict stops the work for the owner, per integration path; proof:
      `git status --porcelain` prints nothing before the rebase, and the reruns exit `0`. `[AC-08]`
- [ ] `[AI]` Confirm the change stays inside its boundary; proof: `git diff --name-only origin/main...HEAD` prints only
      the paths in [File Impact](#file-impact). `[AC-07]`
- [ ] `[AI]` Commit the rest of the execution record as the branch's last commit, a `docs(plans)` commit; proof:
      `git log --format=%s origin/main..HEAD` lists the plan commit last (oldest) and `git status --porcelain` prints
      nothing. `[AC-10]`

### Phase 5: Land and Record

- [ ] `[AI]` Land unit 1 with _Land_, its pull-request body carrying the RED and GREEN captures per
      [red-green-refactor](../../../repo-governance/workflows/quality/red-green-refactor.md) and the plan-first
      deviation above; proof: the merge commit on `origin/main` and _Reconcile_ reading `0 0`, both recorded here by
      unit 2, since the merged copy cannot hold its own merge. `[AC-08]`
- [ ] `[AI]` Immediately after that merge, run
      [dev artifact clean-up](../../../repo-governance/workflows/maintenance/dev-artifact-clean-up.md) for this
      worktree: confirm nothing is unpushed or running, remove it with `git worktree remove` without `--force`, and
      delete `worktree/fix-stable-warning-run-test-finds-no-summary` locally and on `origin`; proof: `git worktree list`
      omits it, and `git branch --list` and `git ls-remote origin` for that branch print nothing. `[AC-10]`
- [ ] `[AI]` Provision the record worktree from the HIPPO repository location, since unit 1's worktree is gone:
      `git fetch origin`, then `git worktree add worktrees/fix-stable-warning-run-test-finds-no-summary-record`
      `-b worktree/fix-stable-warning-run-test-finds-no-summary-record origin/main`, then `npm ci` inside the new
      worktree; proof: `git branch --show-current`, run there, prints the branch. `[AC-10]`
- [ ] `[AI]` Record the landing in the record worktree's copy of this plan: the pull request, its merge commit, and the
      `Quality gate` run on the merged head, then commit it as
      `docs(plans): record fix-stable-warning-run-test-finds-no-summary's landing`; proof:
      `git merge-base --is-ancestor <merge commit> origin/main` exits `0`, and
      `git log --oneline <merge commit>~<k>..<merge commit> -- CHANGELOG.md` prints nothing, where `<k>` is the pull
      request's commit count, `gh pr view <number> --json commits --jq '.commits | length'`, since a rebase-merge leaves
      those commits last on `origin/main`, each recorded here, and `git status --porcelain` prints nothing after the
      commit. `[AC-09]`

### Phase 6: Close

- [ ] `[AI]` Route each learning below to its durable owner, or discard it with a reason; proof: each entry names its
      owner or its reason. `[AC-10]`
- [ ] `[AI]` Run the [execution check](../../../repo-governance/workflows/plan/plan-execution-check.md), then commit
      Phase 6's record as `docs(plans): record fix-stable-warning-run-test-finds-no-summary's execution check`; proof:
      its verdict line recorded here, and `git status --porcelain` prints nothing after the commit. `[AC-10]`

### Archival

- [ ] `[AI]` Move this folder with `git mv` to `plans/done/<completion date>__<this slug>/`, update
      `plans/in-progress/README.md` and `plans/done/README.md` in the same change, commit it as
      `docs(plans): archive the stable-warning run test fix plan`, and land it with _Land_; proof:
      `git status --porcelain` prints nothing before _Land_, then the merge commit, posted on the archival pull request,
      and no copy left under `plans/in-progress/`. `[AC-10]`
- [ ] `[AI]` Run [dev artifact clean-up](../../../repo-governance/workflows/maintenance/dev-artifact-clean-up.md) for
      the record worktree right after that merge; proof, posted on the archival pull request: `git worktree list` omits
      it, no local or remote `worktree/fix-stable-warning-*` branch remains, and _Reconcile_ reads `0 0`. `[AC-10]`

## Learnings

None yet.

## Directory Map

This plan is one document, so this README has no siblings to map.

[go-time]: https://pkg.go.dev/time#hdr-Monotonic_Clocks
[go-now]: https://pkg.go.dev/time#Now
[go-utc]: https://pkg.go.dev/time#Time.UTC
[go-build]: https://pkg.go.dev/cmd/go#hdr-Compile_packages_and_dependencies
[go-testflag]: https://pkg.go.dev/cmd/go#hdr-Testing_flags
