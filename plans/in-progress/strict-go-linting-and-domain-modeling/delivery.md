# Delivery: Strict Go Linting and Domain Modeling

> **Legend** — `[AI]`: an agent performs the step. `[HUMAN]`: only a human can perform it because of unavailable
> credentials, physical action, external authority, or an unresolved decision. Every item here is `[AI]`. Items that
> write or change code name the agent they are dispatched to, per
> [SWE delegation](../../../repo-governance/conventions/swe-delegation.md): `swe-developer` for RED, GREEN, and
> REFACTOR; `swe-debugger` for any gate that fails unexpectedly; `swe-reviewer` for each unit's static review;
> `swe-releaser` for the release.

## Execution Checkout

Repository path: the HIPPO repository location, whose primary checkout stays on `main`.

Worktree path: `worktrees/strict-go-linting-and-domain-modeling/` below it, the worktree that authored this plan, reused
for every unit per [integration path](../../../repo-governance/conventions/integration-path.md).

Delivery mode: `worktree-to-pr`, one pull request per unit, landed serially.

Branches, each created from `origin/main` in that one worktree once the previous unit has merged:

| Unit | Branch                                                  |
| ---- | ------------------------------------------------------- |
| Plan | `worktree/strict-go-linting-and-domain-modeling`        |
| 1    | `worktree/strict-go-linting-gates`                      |
| 2    | `worktree/typed-run-outcome`                            |
| 3    | `worktree/typed-internal-reasons`                       |
| 4    | `worktree/profile-lineage-rules`                        |
| 5    | `worktree/single-admission-decision`                    |
| 6    | `worktree/strict-enum-decoding`                         |
| 7    | `worktree/release-v0.8.5`                               |
| 8    | `worktree/strict-go-linting-and-domain-modeling-record` |

Starting a unit, from the worktree root:

```bash
test -z "$(git status --porcelain)"
git fetch origin --prune
git switch -c <unit branch> origin/main
npm ci
```

Commands the items name:

- _Quick gate_: `npm run test:quick`. _Full gate_: `npm test`. Both run directly: HIPPO cannot guard HIPPO, so never
  beneath `./hippo`, per
  [resource-aware development](../../../repo-governance/development/resource-aware-development.md).
- _Lint_: `go tool golangci-lint run`.
- _NilAway_: `go tool nilaway -include-pkgs=github.com/wahidyankf/hippo -pretty-print=false ./...`.
- _Unit adapter_: `go test -count=1 -run TestUnitBehaviours ./tests/unit`, which executes the scenarios (one scenario:
  `-run 'TestUnitBehaviours/<Scenario_name_with_underscores>'`), beside the structural binding check
  `HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd`. _Integration adapter_: the same with
  `-run TestIntegrationBehaviours ./tests/integration` and `HIPPO_BDD_ADAPTER=integration`. Corrected on 2026-10-06
  during Unit 1: the structural form alone executes no scenario (see [learnings](learnings.md)).
- _Analysis fixtures_: `go test -count=1 -run DomainLiteral ./tests/support`.
- _Policy coverage_: `mkdir -p coverage`, then
  `go test -count=1 -coverpkg=./internal/policy -coverprofile=coverage/unit.out ./tests/unit`, then
  `go run ./tests/coverage --profile coverage/unit.out --directories internal/policy --minimum 99`: the same coverage
  commands as `scripts/test-quick.sh` lines 31–32, narrowed to `internal/policy`. Both exit `0` when the figure the tool
  prints is at or above 99%.
- _Reconcile_: `git -C ../.. fetch origin`, then `git -C ../.. merge --ff-only origin/main`, proved by
  `git -C ../.. rev-list --left-right --count HEAD...origin/main` reading `0 0`.

The PW-4 proof, run from the worktree root in Unit 2. It reads the release tags, not the working tree, and prints every
line that writes `budgetOutcome` in some other way than `SetReservationContext(..., "admitted")`:

```bash
for tag in $(git tag --list 'v*'); do
  git grep -nE 'SetReservationContext\(|BudgetOutcome *[:=]' "$tag" -- '*.go'
done | grep -vE -e 'SetReservationContext\(.*, "admitted"\)' \
  -e 'func \(writer \*EvidenceWriter\) SetReservationContext' \
  -e 'summary\.BudgetOutcome = outcome$'
```

The plan pull request, from `worktree/strict-go-linting-and-domain-modeling`, carries this plan, the brief's removal,
the plan quality-gate repairs, and the move to `plans/in-progress/`. Execution begins after it merges; the move is
therefore not an item below.

## Delivery Units

| Unit | Outcome                                                          | Rollback                       |
| ---- | ---------------------------------------------------------------- | ------------------------------ |
| 1    | `nilness`, `exhaustive` maps, NilAway, and the analysis gate     | revert its merge               |
| 2    | the run outcome is a closed type; history reads it tolerantly    | revert its merge               |
| 3    | the five internal reasons are one closed type, carried as a stop | revert its merge               |
| 4    | profile rules key on lineage; the `minimal` floor fix            | revert its merge               |
| 5    | one admission decision for `run`, `status`, and the driver       | revert its merge               |
| 6    | strict and tolerant decoding; the allowlist is gone              | revert its merge               |
| 7    | `v0.8.5` published                                               | none: publish `v0.8.6` instead |
| 8    | this plan archived                                               | revert its merge               |

Every unit's owner is this repository. Each unit is testable alone, because each lands with its own tests and leaves the
gates green; Unit 1 lands first because every later unit is proved by its gates. Units 3 and 4 precede Unit 5, whose
decision returns a reason and reads a lineage.

## Pause Safety

Each item records its result when it is ticked, in the same edit. At any pause, the plan carries the checkout (the table
above), the last ticked item, and the next unticked one. To resume:

```bash
git branch --show-current
git log --oneline origin/main..HEAD
gh pr list --head "$(git branch --show-current)"
```

A bounded checkpoint's attempts already spent are recorded in its result, and a resumed session continues that count.

## Post-Write Gate

Questions the complete draft revealed, returned to the owner on 2026-10-06. The draft follows each recommendation; an
answer that differs is applied through
[plan propagation](../../../repo-governance/workflows/quality/plan-propagation.md) before Phase 1 starts.

- **PW-1** — the analysis name list. Recommended: the plan's concepts only (see
  [the gates](tech-docs/002-gates-and-analysis.md#domain-literal-analysis)). Answer (owner, 2026-10-06): the
  recommendation.
- **PW-2** — comparisons with `""` and `0`. Recommended: not refused, as presence tests. Answer (owner, 2026-10-06): the
  recommendation.
- **PW-3** — how an internal reason travels. Recommended: as a `*policy.Stop` error. Answer (owner, 2026-10-06): the
  recommendation.
- **PW-4** — `budgetOutcome`. Recommended: type it and delete the two comparisons no writer can satisfy. Answer (owner,
  2026-10-06): the recommendation, provided no released HIPPO version ever wrote either value; if one did, the
  comparisons stay, typed.
- **PW-5** — the configuration value D6 names. Recommended: close `coordination.mode`. Answer (owner, 2026-10-06): the
  recommendation.

The plan quality gate's first cycle raised two more, answered the same day:

- **PW-6** — how AC-10's exit-`125` clause is proven, since every path in `Run` assigns an outcome. Choices: extract the
  deferred promotion into a pure function and test it directly; add a test hook to `RunConfig`; narrow AC-10.
  Recommended: the extraction. Answer (owner, 2026-10-06): the recommendation, a pure `promoteFinalize` tested directly
  in Unit 2, no hook in `RunConfig`, and AC-10 as written.
- **PW-7** — the two production NilAway findings the brief records as unreachable by construction. Choices: classify
  both as false positives now, or keep the predeclared two-attempt fallback. Recommended: keep it. Answer (owner,
  2026-10-06): the recommendation; attempt a code fix up to twice, and if the hazard is still unprovable, a reasoned
  `//nolint:nilaway`, or the file exclusion the Unit 1 checkpoint selects, recorded in the repository adapter per D11.

plan-quality-gate: PASS_WITH_FINDINGS (2 cycles, 1 MEDIUM open: PQG-14, owned by the Unit 7 docs quality gate item,
which now runs before landing).

## Phase 0: Readiness

- [x] `[AI]` Confirm the checkout once the plan pull request has merged: `git -C ../.. worktree list` lists this
      worktree once, and `git status --porcelain` prints nothing; proof: both outputs recorded here. `[AC-25]`
  - Result: (2026-10-06) the plan PR #131 merged as `a3c6b82`; `git -C ../.. worktree list` lists this worktree once, on
    `worktree/strict-go-linting-gates` at `a3c6b82`, and `git status --porcelain` printed nothing.
- [x] `[AI]` Confirm every post-write question has an answer. From the worktree root, with
      `plan=plans/in-progress/strict-go-linting-and-domain-modeling/delivery.md`, run
      `grep -cE '^- \*\*PW-[0-9]+\*\*' "$plan"` and
      `tr '\n' ' ' < "$plan" | grep -oE 'Answer \(owner, +[0-9-]+\)' | wc -l`; proof: both print `7`, one answer for
      each of PW-1 to PW-7, and the counts are recorded here. `[AC-25]`
  - Result: both counts print `7`.
- [x] `[AI]` Run the quick gate on `origin/main` as the baseline; proof: exit `0` and the core coverage figure recorded
      here. `[AC-01]`
  - Result: exit `0` at `a3c6b82`; selected production line coverage 99.30% (846/852 statements).

### Phase 0 Gate

- [x] `[AI]` Run `./rhino md internal-link validate` and `./rhino governance directory-map validate`; proof: both exit
      `0`. `[AC-25]`
  - Result: both exit `0` (1257 links; 50 directories; no findings).

> **Pause Safety**: nothing has changed. To resume: re-run the Phase 0 Gate.

## Phase 1: Unit 1 — Gates

Branch `worktree/strict-go-linting-gates`.

### Lint settings

- [x] `[AI]` Create the branch with the starting commands; proof: `git branch --show-current` prints it. `[AC-03]`
  - Result: (2026-10-06) the branch already existed when this executor started, created from `origin/main` at `a3c6b82`
    with `npm ci` done; `git branch --show-current` prints `worktree/strict-go-linting-gates`.
- [x] `[AI]` **RED** (`swe-developer`): add the step "govet runs nilness and exhaustive checks switch statements and map
      literals" to "Lint gate wiring is exhaustive and module scoped" in `specs/behaviours/quality-gates.feature`, bound
      in `tests/support/steps.go` to read `.golangci.yml`; run the unit adapter; acceptance: it fails at that step
      because `.golangci.yml` names neither setting. `[AC-03]` `[AC-04]`
  - Result: (2026-10-06) the step is added to the scenario, bound in `tests/support/steps.go` to
    `requireNilnessAndExhaustiveMaps` (new, in `tests/support/driver.go`, beside the other lint checks, reading the
    `govet:` and `exhaustive:` blocks of `.golangci.yml`). **Deviation:** the _Unit adapter_ command as written,
    `HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd`, only verifies that every step resolves to a binding
    (`contract.Verify`), executes no scenario, and exits `0` with the new step bound. The executing adapter is
    `go test -count=1 -run TestUnitBehaviours ./tests/unit`; it exited `1`: `336 scenarios (335 passed, 1 failed)`,
    `Scenario: Lint gate wiring is exhaustive and module scoped`,
    `And govet runs nilness and exhaustive checks switch statements and map literals`,
    `Error: govet does not enable nilness`. This reading of "the unit adapter" applies to every item below; see
    [learnings](learnings.md).
- [x] `[AI]` **GREEN** (`swe-developer`): add `govet: enable: [nilness]` and `exhaustive: check: [switch, map]` to
      `.golangci.yml`; run the unit adapter, then _Lint_; acceptance: the adapter passes, and lint reports exactly one
      finding, `exhaustive` at `internal/status/status.go:179`. `[AC-03]` `[AC-04]`
  - Result: (2026-10-06) `.golangci.yml` gains `exhaustive: check: [switch, map]` and `govet: enable: [nilness]` under
    `linters.settings`, each with a comment. The executing unit adapter
    (`go test -count=1 -run TestUnitBehaviours ./tests/unit`) exits `0` in 187 s, and the structural form
    (`HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd`) exits `0`. _Lint_ then reported `2 issues`: the expected
    one, `exhaustive` at `internal/status/status.go:179:17` (`missing keys in map of key type status.Code`, 16 codes
    named), and a `gofumpt` finding in the new `settingsBlock` helper (`group-params`: `configuration, key string`),
    fixed in the same item; the rerun reports exactly `1 issues: * exhaustive: 1`.
- [x] `[AI]` **GREEN** (`swe-developer`): list all 18 codes in `retryable` (`internal/status/status.go`), `true` only
      for `CodeLimitCapacityDeferred` and `CodeLimitPressureShed`; run _Lint_ and `go test -count=1 ./tests/unit`;
      acceptance: both exit `0`. `[AC-04]`
  - Result: (2026-10-06) `retryable` now names all 18 codes in `status.All` order, `true` for the two named. _Lint_
    exits `0` (`0 issues.`); `go test -count=1 ./tests/unit` exits `0` (`ok ... 191.899s`).
- [x] `[AI]` **REFACTOR** (`swe-developer`): rewrite the `retryable` comment to say every code is listed so a new code
      needs a decision; run _Lint_; acceptance: exit `0`. `[AC-04]`
  - Result: (2026-10-06) the comment now says every code is listed, the false ones too, so a new code needs a decision
    on whether waiting helps, and that `exhaustive` refuses an omission. _Lint_ exits `0` (`0 issues.`).
- [x] `[AI]` Mutation: add `internal/policy/nilness_mutation.go` dereferencing a pointer inside its own `== nil` branch,
      run _Lint_, record the output, delete the file; acceptance: lint fails naming `govet` and `nilness`, and passes
      once the file is gone. `[AC-03]`
  - Result: (2026-10-06) the planted file read `if box == nil { return box.value }`. _Lint_ exited `1`:
    `internal/policy/nilness_mutation.go:7:14: nilness: nil dereference in field selection (govet)`, plus two `unused`
    findings for the file's own scaffolding (`3 issues: govet: 1, unused: 2`). The file was deleted; _Lint_ then exited
    `0` (`0 issues.`) and `git status --short` shows no trace of it.
- [x] `[AI]` Mutation: delete the `CodeArgsInvalid` key from `retryable`, run _Lint_, record the output, restore it;
      acceptance: lint fails naming `exhaustive` and `status.CodeArgsInvalid`, and passes once restored. `[AC-04]`
  - Result: (2026-10-06) with the `CodeArgsInvalid` key removed, _Lint_ exited `1`:
    `internal/status/status.go:181:17: missing keys in map of key type status.Code: status.CodeArgsInvalid (exhaustive)`
    (`1 issues: exhaustive: 1`). The file was restored byte for byte (`diff` against the saved copy is empty); _Lint_
    then exited `0` (`0 issues.`).

### NilAway

- [x] `[AI]` Pin it: `go get -tool go.uber.org/nilaway/cmd/nilaway@v0.0.0-20260918162853-acb8859b9031`, then
      `go mod tidy`; proof: `go.mod`'s `tool` block names `go.uber.org/nilaway/cmd/nilaway` and `go tool nilaway -h`
      exits `0`. `[AC-01]`
  - Result: (2026-10-06) `go get -tool go.uber.org/nilaway/cmd/nilaway@v0.0.0-20260918162853-acb8859b9031` exited `0`,
    as did `go mod tidy`. `go.mod`'s `tool` block now lists `go.uber.org/nilaway/cmd/nilaway`, and `go tool nilaway -h`
    exits `0`. Surprising: the pin raises shared dependencies through minimal version selection: `golang.org/x/sys`
    v0.47.0 to v0.48.0 (a direct, production dependency), `golang.org/x/tools` v0.49.0 to v0.50.0, `golang.org/x/mod`
    v0.40.0 to v0.41.0, `golang.org/x/sync` v0.22.0 to v0.23.0, `golang.org/x/exp/typeparams` and
    `golang.org/x/telemetry` to September 2026 pseudo-versions, and it adds `github.com/klauspost/compress` v1.20.0 and
    `go.uber.org/nilaway` itself as `// indirect` requirements (`go.mod` +9/-6, `go.sum` +18/-14). `go build ./...` and
    _Lint_ still exit `0` (`0 issues.`) under the raised versions.
- [x] `[AI]` Run _NilAway_ and record every diagnostic here; acceptance: it exits `3`, and the list is compared with the
      eight diagnostics [the gates](tech-docs/002-gates-and-analysis.md#nilaway) record, any difference named. `[AC-01]`
  - Result: (2026-10-06) exit `3`, 8 diagnostics (about 1 s), the same eight the gates document records, with one
    difference named below. Each is "Potential nil panic detected": (1) `internal/guard/exclusive_status.go:60:40`, a
    literal `nil` returned from `liveExclusiveHeavyOwner()` at line 125, position 0, dereferenced through `heavy` (line
    42); (2) `internal/guard/lease.go:315:19`, result 0 of `readLeaseOwner()` unguarded, field `SchemaVersion`, via
    `owner` (line 307), same source also at lines 322:11 and 327:74; (3) `tests/support/release_v04.go:1416:29`,
    unassigned variable `outputs` sliced into, also at 1432:49 and 1436:50; (4) `internal/guard/run_test.go:1909:15`,
    unassigned `holder` accessed field `Process`; (5) `tests/integration/lease_evidence_test.go:41:20`, a literal `nil`
    returned from `AcquireSession()` (`internal/guard/lease.go:472:11`), field `Inherited` read through `inherited`
    (line 40), same source also at `tests/integration/run_test.go:182:46`; (6)
    `tests/integration/lease_evidence_test.go:281:2`, unassigned `owner` written at an index, also at 288:2; (7) the
    same file at `324:2`, also at 331:2; (8) `tests/support/isolation_test.go:119:75`, result 0 of `os.Stat()`
    unguarded, `IsDir()` via `info` (line 118). **Difference:** diagnostic (5) also reaches
    `tests/integration/run_test.go:182:46`, a file the file-impact table does not list; the six-test-code-findings item
    below fixes it with the others and the file impact gains that path. The two production sources stand as the plan
    recorded them: four production sites in all (one in `exclusive_status.go`, three in `lease.go`).
- [x] `[AI]` Bounded checkpoint, one attempt: put `//nolint:nilaway // <reason>` on one finding classified as a false
      positive (or, if none is, on a scratch copy of one finding), run _Lint_; acceptance: exit `0` makes inline
      directives the mechanism; a `nolintlint` finding makes `-exclude-errors-in-files` on the `scripts/test-quick.sh`
      line the mechanism, with nothing retried. Record which. `[AC-01]`
  - Result: (2026-10-06) no finding was classified as a false positive yet, so the one attempt used a scratch copy of
    the `exclusive_status.go:60` shape (`internal/guard/nilaway_scratch.go`, an exported function dereferencing the
    first result of a `(*T, bool)` source with `//nolint:nilaway // <reason>` on the dereferencing line; deleted
    afterwards). _Lint_ exited `0` (`0 issues.`, no `nolintlint` finding), so **inline `//nolint:nilaway // <reason>`
    directives are the mechanism**; `-exclude-errors-in-files` is not used and nothing was retried. Verification beyond
    the attempt, not a second attempt: the scratch copy did not itself reproduce a NilAway diagnostic (NilAway honours
    the `(*T, bool)` return contract), so the same directive was placed on the real
    `internal/guard/exclusive_status.go:60` line and reverted after: NilAway then reported 7 diagnostics, none in that
    file (8 without the directive), and _Lint_ again exited `0` with `0 issues.` but printed
    `level=warning msg="[runner/nolint_filter] Found unknown linters in //nolint directives: nilaway"`, a warning that
    does not fail the gate. The file is byte for byte as before.
- [x] `[AI]` **RED** (`swe-developer`): for the `internal/guard/exclusive_status.go:60` finding, add a test in
      `internal/guard/exclusive_status_test.go` where `liveExclusiveHeavyOwner` returns `nil` and the caller proceeds;
      run `go test -count=1 -run Exclusive ./internal/guard`; acceptance: it panics with a nil dereference. At most two
      attempts; if neither reaches the dereference, the finding is a false positive and the next GREEN and REFACTOR
      record `Not applicable` while the exclusion item covers it. `[AC-01]`
  - Result: (2026-10-06) two attempts, both spent, neither panicked, so the finding is a false positive. Attempt 1,
    `TestExclusiveStatusWithoutHeavyOwnerCountsSessionsOnly`: an exclusive-mode root with a live service session and no
    `heavy.lock`, so `liveExclusiveHeavyOwner` returns `nil` with `live` false; `ExclusiveStatus` returns the session
    totals before line 60. Attempt 2, `TestExclusiveStatusRefusesNullHeavyOwnerDocument`: `heavy.lock/owner.json`
    holding `null`, the one decoder shape that could yield no owner; `readLeaseOwner` still returns a non-nil
    zero-valued owner, schema 0, and `ExclusiveStatus` returns an unreadable-state error that is not a protocol
    mismatch, leaving the document unchanged. `go test -count=1 -v -run Exclusive ./internal/guard` exits `0` with all
    four `Exclusive` tests passing (`ok ... 0.402s`) and _Lint_ exits `0`. The two tests are kept: they pin the
    invariant that the exclusion reason below rests on. Reachability finding: `liveExclusiveHeavyOwner` returns a `nil`
    owner only together with `live` false or a non-nil error, and the caller stops on both, but NilAway does not
    correlate the three results.
- [x] `[AI]` **GREEN** (`swe-developer`): guard the nil result in `internal/guard/exclusive_status.go`; run the same
      command; acceptance: it passes. `[AC-01]`
  - Result: Not applicable (2026-10-06): the RED item found no reachable dereference, so the finding is a false positive
    and the exclusion item below covers it.
- [x] `[AI]` **REFACTOR** (`swe-developer`): keep the guard in the shape the surrounding error handling uses; run
      `go test -count=1 ./internal/guard` and _NilAway_; acceptance: the tests pass and NilAway no longer reports the
      file. `[AC-01]`
  - Result: Not applicable (2026-10-06): no guard was added; the exclusion item below covers
    `internal/guard/exclusive_status.go:60`.
- [x] `[AI]` **RED** (`swe-developer`): for the `internal/guard/lease.go:315` finding, add a test in
      `tests/integration/lease_evidence_test.go` where `readLeaseOwner` returns a `nil` owner without an error; run
      `go test -count=1 ./tests/integration`; acceptance: it panics with a nil dereference, under the same two-attempt
      rule. `[AC-01]`
  - Result: (2026-10-06) `readLeaseOwner` is unexported, so both attempts reach lines 315 to 327 through the exported
    `DescribeHeavyLease`, and neither panicked: the finding is a false positive. Attempt 1,
    `TestDescribeHeavyLeaseReportsNullOwnerDocumentAsUnverifiable`: `heavy.lock/owner.json` holding `null`; the decoder
    leaves a non-nil zero-valued owner (schema 0) and the description reads "cannot be verified".
    `go test -count=1 ./tests/integration` exits `0` (`ok ... 213.180s`). Attempt 2,
    `TestDescribeHeavyLeaseReportsEmptyOwnerDocumentAsUnverifiable`: an empty `owner.json`; the decode error returns a
    `nil` owner with `err` set, and line 315 short-circuits on `err != nil` before reading `SchemaVersion`. Run narrowed
    to save 3.5 minutes, `go test -count=1 -v -run DescribeHeavyLease ./tests/integration`: exits `0`, both tests pass;
    _Lint_ exits `0`. Both tests are kept as the evidence for the exclusion reason. Reachability finding:
    `readLeaseOwner` returns `&owner` with a `nil` error and `nil` with a non-nil error, and every caller tests `err`
    first; NilAway appears to lose that contract at the join after line 310 reassigns `err`, so line 315 reads as a path
    with a nil owner and a nil `err`, which the code cannot produce.
- [x] `[AI]` **GREEN** (`swe-developer`): guard the nil owner in `internal/guard/lease.go`; run the same command;
      acceptance: it passes. `[AC-01]`
  - Result: Not applicable (2026-10-06): no reachable dereference, so no guard; the exclusion item below covers
    `internal/guard/lease.go:315`, 322, and 327.
- [x] `[AI]` **REFACTOR** (`swe-developer`): fold the guard into the existing owner checks at lines 315–327; run the
      same command and _NilAway_; acceptance: both clean for the file. `[AC-01]`
  - Result: Not applicable (2026-10-06): no guard was added; the exclusion item below covers the file.
- [x] `[AI]` Fix the six test-code findings by checking each value before use (`t.Fatal` on `nil`) in
      `tests/support/release_v04.go`, `internal/guard/run_test.go`, `tests/integration/lease_evidence_test.go`, and
      `tests/support/isolation_test.go` (`swe-developer`); run _NilAway_; acceptance: no diagnostic names those files.
      `[AC-01]`
  - Result: (2026-10-06) each value is checked before use, failing the test on nil:
    `tests/integration/lease_evidence_test.go` (the inherited session at line 41 gains `inherited == nil`, and a new
    helper `readOwnerDocument` replaces the two unchecked `json.Unmarshal` into a nil map at lines 281 and 324, failing
    on an unreadable or null document), `tests/integration/run_test.go` (the unlisted site at line 182:
    `err != nil || session == nil`), `internal/guard/run_test.go` (`holder == nil || holder.Process == nil` before
    `holder.Process.Kill()`), `tests/support/isolation_test.go` (`info != nil` in the probe's root test), and
    `tests/support/release_v04.go` (`len(outputs) != 2` is refused before `outputs[0]`). NilAway exits `3` with exactly
    two diagnostics left, `internal/guard/exclusive_status.go:60:40` and `internal/guard/lease.go:315:19`; none names a
    test file. Regression: _Lint_ exits `0`; `go test -count=1 ./internal/guard ./tests/support` exits `0`; the scenario
    "Rebuilding a release commit reproduces its archives" passes in the unit adapter
    (`-run TestUnitBehaviours/<scenario>`); the touched integration tests pass
    (`-run 'TestHeavyLease|TestPortLease|TestDescribeHeavyLease|TestInheritedGuardRunsDirectly' ./tests/integration`,
    `ok`). The file impact gains `tests/integration/run_test.go`.
- [x] `[AI]` Exclude each remaining false positive with the mechanism the checkpoint chose, each with its reason; run
      _NilAway_; acceptance: exit `0`. `[AC-01]`
  - Result: (2026-10-06) two inline exclusions, the only two false positives left, each a
    `//nolint:nilaway // <reason>`: (1) `internal/guard/exclusive_status.go:60`, trailing the
    `appendExclusiveOwner(&totals, runID, *heavy)` line: "liveExclusiveHeavyOwner returns a nil owner only with live
    false or an error, and both return above; NilAway does not correlate the three results"; (2)
    `internal/guard/lease.go:315`, on its own line directly above `if err != nil || owner.SchemaVersion != 1 {` in
    `DescribeHeavyLease`: "readLeaseOwner returns a nil owner only with an error, which this condition tests first;
    NilAway loses that once err is reassigned above". NilAway exits `0`. Surprising: NilAway scopes a directive to the
    AST node `ast.NewCommentMap` attaches it to (`diagnostic/nolint.go`), so a comment trailing the `{` of an `if` line
    did not cover the finding on that line (still reported at `lease.go:315:19`), while a comment line directly above
    the `if` covers the whole statement and, because NilAway groups the three sites by one nil source, also suppresses
    the grouped sites at lines 322 and 327. _Lint_ exits `0` (`0 issues.`) but now prints
    `level=warning msg="[runner/nolint_filter] Found unknown linters in //nolint directives: nilaway"` on every run; it
    does not fail the gate. The reasons go into the repository adapter in the Unit 1 close.
- [x] `[AI]` **RED** (`swe-developer`): add the step "the quick gate invokes the pinned NilAway over the module" to the
      lint-wiring scenario, bound to read `scripts/test-quick.sh`; run the unit adapter; acceptance: it fails at that
      step. `[AC-01]`
  - Result: (2026-10-06) the step is added after "the quick gate invokes module-local lint" in "Lint gate wiring is
    exhaustive and module scoped", bound in `tests/support/steps.go` to `requirePinnedNilAway` (new, in
    `tests/support/driver.go`): it reads `scripts/test-quick.sh` and `go.mod`, and requires a `go tool nilaway` line
    with `-include-pkgs=github.com/wahidyankf/hippo`, `-pretty-print=false`, and `./...`, no `-json`, running after
    `go tool golangci-lint run`, plus the tool directive in `go.mod`. Run narrowed to the scenario
    (`go test -count=1 -run 'TestUnitBehaviours/Lint_gate_wiring' ./tests/unit`, the full adapter having run in the
    lint-settings items): exit `1`, `And the quick gate invokes the pinned NilAway over the module`,
    `Error: the quick gate does not invoke the pinned NilAway`, `6 steps (5 passed, 1 failed)`.
- [x] `[AI]` **GREEN** (`swe-developer`): add the _NilAway_ command to `scripts/test-quick.sh` directly after
      `go tool golangci-lint run`; run the unit adapter and the quick gate; acceptance: both exit `0`. `[AC-01]`
  - Result: (2026-10-06) `go tool nilaway -include-pkgs=github.com/wahidyankf/hippo -pretty-print=false ./...` is now
    the line directly after `go tool golangci-lint run` in `scripts/test-quick.sh`. The executing unit adapter
    (`go test -count=1 -run TestUnitBehaviours ./tests/unit`) exits `0` (`ok ... 193.116s`) and the structural form
    exits `0`. `npm run test:quick` exits `0` in 3 min 41 s (the first attempt stopped at the format check, because the
    plan file was not yet Prettier-formatted; after `npx prettier --write` the rerun passed every step): lint
    `0 issues.` with the unknown-linter warning, the NilAway step silent,
    `selected production line coverage: 99.30% (846/852 statements)`, and the three `tests/bdd` adapters `ok`.
- [x] `[AI]` **REFACTOR** (`swe-developer`): give the new line a comment stating why NilAway runs beside golangci-lint
      and why `-json` is not used; run `./scripts/format-check.sh`; acceptance: exit `0`. `[AC-01]`
  - Result: (2026-10-06) a six-line comment above the line says NilAway follows nil flow across functions and packages,
    which golangci-lint's per-function `nilness` cannot, so it runs beside the linter; that it exits `3` on a diagnostic
    and `1` when loading fails, either of which stops the script; that `-json` is not used because it always exits `0`,
    so a finding would pass; and that a false positive is excluded where it stands with a reasoned `//nolint:nilaway`
    directive. `./scripts/format-check.sh` exits `0` (gofumpt/goimports diff clean, `shfmt -d` clean, Prettier clean).
- [x] `[AI]` Mutation: add a production line reading a field of `liveExclusiveHeavyOwner`'s result unguarded, run the
      quick gate, record the output, remove it; acceptance: the gate fails at the NilAway step naming that file and
      line, and passes once removed. `[AC-02]`
  - Result: (2026-10-06) **Deviation:** the mutation as written cannot fail the gate, so it was run as written and then
    on a substitute source. As written, `_ = heavy.PID` after the error check in `ExclusiveStatus` (also
    `totals.ActiveOwners = heavy.PID`, `_ = *heavy`, and an index into `heavy.Token`, each tried): `go tool nilaway`
    exits `0` with the line-60 exclusion in place, and with the exclusion removed it still reports only the original
    `*heavy` site, never the new line. Cause, from `diagnostic/conflict.go` and the observed output: NilAway reports one
    conflict per nil source (here result 0 of `liveExclusiveHeavyOwner`), so the exclusion on that source also hides
    every later unguarded read of it. The full quick gate then ran past NilAway and failed only at `internal/guard` unit
    tests, where my own `TestExclusiveStatusWithoutHeavyOwnerCountsSessionsOnly` panicked on the real nil dereference
    (`exclusive_status.go:46`), exit `1`. Substitute: a source with no exclusion, `AcquireSession`, which returns
    `nil, nil` for a deferral (`internal/guard/lease.go:473`). The planted line
    `config.noteDeferralf("HIPPO session %s.\n", session.Token)` in `internal/guard/run.go`, before the `session == nil`
    check, made `npm run test:quick` pass format, build, and _Lint_ and then fail at the NilAway step, exit `3`:
    `internal/guard/run.go:658:46: Potential nil panic detected`, with the flow
    `literal nil returned from AcquireSession() in position 0` to `result 0 of AcquireSession() accessed field Token`.
    `run.go` was restored (`git diff` empty) and `npm run test:quick` exits `0` again (`ok ... 193.300s`,
    `selected production line coverage: 99.30% (846/852 statements)`). The `exclusive_status.go` mutation was reverted
    to the excluded state, and `go tool nilaway` exits `0`.

### Domain literal analysis

- [x] `[AI]` **RED** (`swe-developer`): add `tests/support/domain_literals_internal_test.go` with fixtures for each rule
      in [the gates](tech-docs/002-gates-and-analysis.md#domain-literal-analysis): a `ResolvedProfile == "balanced"`
      comparison, a `case "constrained":` over a profile, a raw `Outcome string` field, a raw `class string` parameter,
      a defined-type value compared with a literal, and the negatives (`""`, `0`, test files, unlisted names), plus a
      stale allowlist entry; run _Analysis fixtures_; acceptance: it fails to compile because the analysis does not
      exist. `[AC-05]` `[AC-06]` `[AC-07]`
  - Result: (2026-10-06) the file holds eleven `TestDomainLiteral...` tests: the `ResolvedProfile == "balanced"`
    comparison, a `case "constrained":` over a profile, raw `Outcome string`/`Decision bool`/`Lineage int` fields, raw
    `class string`, interface-method, and function-literal parameters, a defined-type `Mode`/`Level` compared with a
    literal in `==`, `!=` (literal on the left), and `case`, and the negatives (`""` and `0` comparisons, `_test.go`
    files, a path outside `cmd/` and `internal/`, the unlisted names `Path`, `State`, `Reason`, `outcomeFlag`, an
    interface-typed `Outcome`, and a typed constant), a package that does not type-check, the reconcile function (a
    finding off the allowlist, a stale entry, a matched pair), and the module loader against a temporary module
    importing `strings` (plus a directory with no module). `go test -count=1 -run DomainLiteral ./tests/support` exits
    `1` at build time: `undefined: analyzeDomainPackage`, `undefined: domainPackage`, `undefined: domainFinding`,
    `undefined: ruleRawField`, `undefined: ruleLiteralComparison` (ten errors listed, then `too many errors`;
    `FAIL ... [build failed]`).
- [x] `[AI]` Bounded checkpoint, one attempt: load every production package with `go list -export -json` and
      `go/importer` in `gc` mode; acceptance: all load, or the syntax-only fallback is adopted and recorded here.
      `[AC-05]`
  - Result: (2026-10-06) one attempt, all load, so **the typed analysis is adopted** and the syntax-only fallback is not
    needed. A scratch program outside the repository (in the session scratchpad) ran
    `go list -export -deps -json ./cmd/... ./internal/...` in the module root, collected each package's `Export` file,
    and type-checked the non-test files of every package with `go/types` and `importer.ForCompiler(fset, "gc", lookup)`:
    147 packages with export data, 12 production packages, `typechecked 12 of 12`, in 0.7 s warm. One difference from
    the command as written: `-deps` is required, because the `gc` importer resolves each import, the standard library
    included, through its own export file, and `go list -export` without `-deps` lists none for them. Nothing was
    retried.
- [x] `[AI]` **GREEN** (`swe-developer`): implement `tests/support/domain_literals.go`; run _Analysis fixtures_;
      acceptance: every fixture passes. `[AC-05]` `[AC-06]` `[AC-07]`
  - Result: (2026-10-06) `tests/support/domain_literals.go` implements the analysis as designed, on the standard library
    only: `analyzeDomainLiterals(root)` lists packages with `go list -export -deps -json ./cmd/... ./internal/...`,
    type-checks each production package with `go/types` through `importer.ForCompiler(..., "gc", lookup)`, and
    `analyzeDomainPackage` walks the syntax for the two rules (`literal-comparison` by name list or by a module-declared
    defined string or integer type; `raw-field` for fields and parameters); `reconcileDomainFindings` counts findings
    against allowlist entries. Fixture adjustments made while greening, both mistakes in the fixtures and not in the
    rules: a `case ModeFast` constant that duplicated `case "fast"` is now `"turbo"`, and the temporary module for the
    loader test gained a `cmd/tool` package, because `go list ./cmd/...` exits `1`
    (`lstat ./cmd/: no such file or directory`) when the directory is missing; it now also proves the `cmd/` tree is
    read. `go test -count=1 -v -run DomainLiteral ./tests/support` exits `0`: 11 tests, 11 `--- PASS`, 0 `--- FAIL`
    (`ok ... 0.393s`). The first _Lint_ run reported `musttag` (the `go list` struct lacked `json` tags) and
    `varnamelen` (`ok`), both fixed; _Lint_ then exits `0` (`0 issues.`). Known limit, recorded in the file's header
    comment: files a build constraint excludes on the running platform are not analysed; only `internal/host` has such
    files and it carries no domain name.
- [x] `[AI]` **REFACTOR** (`swe-developer`): sort findings and print `<path>:<line>: <rule>: <identifier>`; run
      _Analysis fixtures_ and _Lint_; acceptance: both exit `0`. `[AC-05]`
  - Result: (2026-10-06) the sorting (`compareDomainFindings`: path, line, rule, identifier, applied in
    `analyzeDomainPackage` and again across packages in `analyzeDomainLiterals`) and the `domainFinding.String()` form
    `<path>:<line>: <rule>: <identifier>` were already in place from the GREEN item, because the fixtures assert that
    form; the refactor that remained was in the fixture file, where two copies of the finding printing became one
    helper, `findingLines`. _Analysis fixtures_ exit `0` (`ok ... 0.400s`) and _Lint_ exits `0` (`0 issues.`).
- [x] `[AI]` **RED** (`swe-developer`): add "Production code compares no domain value with a literal" to
      `specs/behaviours/quality-gates.feature` with its end-to-end exemption in `tests/contract/contract.go`; run the
      unit adapter, then bind the steps with an empty allowlist and run it again; acceptance: first undefined, then
      failing with the current findings, whose count and list are recorded here. `[AC-05]`
  - Result: (2026-10-06) the scenario ("When the domain literal analysis runs over production code", "Then every finding
    is on the ratchet allowlist and every allowlist entry holds a finding") is added to
    `specs/behaviours/quality-gates.feature` tagged `@e2e-exempt`, with its exemption in `tests/contract/contract.go`
    (boundary `repositoryConfigBoundary`: the analysis reads production source, outside the compiled binary). First run,
    steps unbound: both the structural form (`HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd`) and the executing
    form (`go test -count=1 -run TestUnitBehaviours ./tests/unit`) exit `1`:
    `undefined behavior step "the domain literal analysis runs over production code"` and
    `undefined behavior step "every finding is on the ratchet allowlist and every allowlist entry holds a finding"`.
    Second run, steps bound in `tests/support/steps.go` to `runDomainLiteralAnalysis` and `requireDomainLiteralRatchet`
    with an empty `domainLiteralAllowlist` (new `tests/support/domain_literals_allowlist.go`), narrowed to the scenario
    (`-run 'TestUnitBehaviours/Production_code_compares_no_domain_value_with_a_literal'`): exit `1`, the `When` step
    passes and the `Then` step fails with `Error: domain literal analysis found 31 problems`. **31 findings: 26
    `raw-field` and 5 `literal-comparison`**, the same total as the scratch syntax scan the gates document records (21
    fields and 5 parameters, and 5 comparisons): internal/cli/commands.go:15: raw-field: requestedProfile;
    internal/cli/commands.go:43: raw-field: taskClass; internal/cli/commands.go:45: raw-field: outcome;
    internal/cli/commands.go:64: raw-field: class; internal/cli/development.go:345: raw-field: Profile;
    internal/evidence/history.go:38: raw-field: TaskClass; internal/evidence/history.go:39: raw-field: Outcome;
    internal/evidence/history.go:45: raw-field: BudgetOutcome; internal/evidence/history.go:55: raw-field: Class;
    internal/evidence/history.go:57: raw-field: Outcome; internal/evidence/history.go:212: literal-comparison: Outcome;
    internal/evidence/history.go:217: literal-comparison: BudgetOutcome; internal/evidence/history.go:217:
    literal-comparison: BudgetOutcome; internal/guard/evidence.go:59: raw-field: outcome;
    internal/guard/evidence.go:194: raw-field: TaskClass; internal/guard/evidence.go:195: raw-field: Outcome;
    internal/guard/evidence.go:209: raw-field: RequestedProfile; internal/guard/evidence.go:210: raw-field:
    ResolvedProfile; internal/guard/evidence.go:220: raw-field: BudgetOutcome; internal/guard/evidence.go:232:
    raw-field: outcome; internal/guard/lease.go:27: raw-field: Class; internal/guard/owner_metadata.go:44: raw-field:
    Profile; internal/guard/owner_metadata.go:192: raw-field: profile; internal/guard/reservation.go:113: raw-field:
    Profile; internal/guard/reservation.go:131: raw-field: Profile; internal/guard/reservation.go:266:
    literal-comparison: ResolvedProfile; internal/guard/reservation.go:268: literal-comparison: ResolvedProfile;
    internal/guard/reservation.go:1046: raw-field: profile; internal/guard/reservation.go:1062: raw-field: profile;
    internal/policy/profiles.go:82: raw-field: RequestedProfile; internal/policy/profiles.go:83: raw-field:
    ResolvedProfile.
- [x] `[AI]` **GREEN** (`swe-developer`): add `tests/support/domain_literals_allowlist.go` with one entry per finding,
      each tagged with the unit that removes it; run the unit and integration adapters; acceptance: both exit `0`.
      `[AC-05]` `[AC-07]`
  - Result: (2026-10-06) `tests/support/domain_literals_allowlist.go` holds 31 `domainAllowance` entries, one per
    finding in path order, each with the enclosing symbol the analysis printed (for example `Summary.Outcome`,
    `PlanReservation`, `EvidenceWriter.Finalize.outcome`) and the unit that removes it, by the plan's removal table: 11
    for Unit 2 (outcomes and budget outcomes: the `internal/guard/evidence.go` writer and summary,
    `internal/evidence/history.go`, and `outcome` in the CLI history options), 14 for Unit 4 (profile names in
    `internal/policy`, `internal/guard`, `internal/cli`, and the two owner-share `switch` comparisons in
    `PlanReservation`), and 6 for Unit 6 (task classes and the CLI class flags). Both executing adapters exit `0`
    (`go test -count=1 -run TestUnitBehaviours ./tests/unit`: `ok ... 195.113s`;
    `go test -count=1 -run TestIntegrationBehaviours ./tests/integration`: `ok ... 194.983s`), and so do the structural
    forms (`HIPPO_BDD_ADAPTER=unit` and `=integration go test -count=1 ./tests/bdd`).
- [x] `[AI]` **REFACTOR** (`swe-developer`): group the entries by removing unit with a comment per group; run the unit
      adapter; acceptance: exit `0`. `[AC-07]`
  - Result: (2026-10-06) the 31 entries now sit in three groups, Unit 2 (11), Unit 4 (14), and Unit 6 (6), each opened
    by a comment saying what that unit removes. The first regrouped file failed _Lint_ with six `goconst` findings,
    because the same path literal now repeats across the entries and `tests/support/release_v04.go`; four path constants
    (`cliCommands`, `evidenceHistory`, `guardEvidence`, `guardReservation`) and one symbol constant (`promotionSummary`)
    in the allowlist file fixed them, and _Lint_ exits `0` (`0 issues.`). The executing unit adapter exits `0`
    (`ok ... 193.492s`) and so does the structural form.
- [x] `[AI]` Mutation: add `if resolution.ResolvedProfile == "balanced" {}` to `internal/guard/reservation.go`, run the
      unit adapter, record the output, remove it; acceptance: it fails naming that file, that line, and the
      literal-comparison rule, and passes once removed. `[AC-05]`
  - Result: (2026-10-06) the line was planted before `shares := settings.OwnerShares[...]` in `PlanReservation` (line
    263). **Deviation, fixed test-first:** the first run, narrowed to the scenario
    (`go test -count=1 -run 'TestUnitBehaviours/Production_code_compares_no_domain_value_with_a_literal' ./tests/unit`),
    exited `1` with
    `domain literal analysis found 1 problems: internal/guard/reservation.go:270: literal-comparison: ResolvedProfile`,
    naming the file and the rule but line 270, the old `case "constrained":` line shifted down, not the planted line
    263: with entries counted per symbol, the list cannot say which of three findings in `PlanReservation` is new, so
    the reconcile reported the last in line order. RED: `TestDomainLiteralReportsFindingsOffTheAllowlistAndStaleEntries`
    now expects every finding of an over-budget symbol, and failed with only `a.go:31` reported. GREEN:
    `reconcileDomainFindings` reports all findings of a symbol and rule whose findings outnumber its entries; the
    fixtures and _Lint_ exit `0`. Rerun with the line planted: exit `1`, `domain literal analysis found 3 problems:`
    `internal/guard/reservation.go:263: literal-comparison: ResolvedProfile`, `:268`, and `:270`, so the planted line is
    named with the file and the rule. After removal (`git diff --stat internal/guard/reservation.go` empty) the same
    scenario passes (`ok ... 2.876s`). The scenario was run narrowed (about 3 s) in both mutation runs; with the
    mutation removed and every item of this unit's gates in place, `npm run test:quick` exits `0` in 3 min 44 s, which
    runs the whole unit and integration adapters (the new scenario included), the analysis fixtures, lint, and NilAway:
    `selected production line coverage: 99.30% (846/852 statements)`.

### Unit 1 close

- [x] `[AI]` Apply [rules propagation](../../../repo-governance/workflows/quality/rules-propagation.md) to the gate
      changes: the repository adapter's `golang-standards.md` gates entry (nilness, exhaustive maps, NilAway and its
      exclusions, the analysis, `gochecksumtype`'s empty target), its Version Sources, and `quality-gates.md`'s quick
      gate order; proof: the run's status and ledger recorded here, and `npm run test:quick` exits `0`. `[AC-01]`
  - Result: (2026-10-06) status `landed`, eight rows. The adapter entrypoint sat at its 750-word ceiling (763 with the
    first draft), so, as the stack-packs adapter convention's Size section allows, the record went to a new companion
    module, `repo-governance/development/quality/stacks/repository-adapter/001-go-analysis-gates.md` (indexed by a new
    `repository-adapter/README.md` and by `stacks/README.md`), and the adapter's `golang-standards.md` gates entry now
    links it, its formatter and disabled-linter text relocated there. Ledger: R1 `nilness`, R2 `exhaustive`
    `switch`/`map`, R3 NilAway runner, plain output not `-json`, exit codes, `-include-pkgs`, and R4 its two inline
    exclusions with reasons, scoping, masking, and the unknown-linters warning, R6 the analysis and its 31-entry ratchet
    (Unit 2: 11, Unit 4: 14, Unit 6: 6), R7 `gochecksumtype`'s empty target: each `resolved`, placed in the module; R5
    the tool pin raising shared modules: `resolved`, a review obligation in the module, with Dependency Selection's
    "What does it pull in?" as the rule that already suffices; R8 Version Sources: `no-change`, still `go.mod`, because
    the stack-packs adapter convention, a higher level, allows only manifest paths there and never a repeated version,
    so the NilAway pseudo-version and the `x/sys` v0.47.0 to v0.48.0 move are recorded in
    `tech-docs/002-gates-and-analysis.md` instead. `quality-gates.md`'s quick-gate order names NilAway directly after
    strict lint and the analysis as a corpus scenario. Dispositions: covered by the lint-wiring and analysis scenarios,
    except the masking review, unenforced by decision because NilAway stays silent by construction. Placement record:
    `local-tmp/rules-propagation-2026-10-06-unit1-gates.md`. `./rhino governance word-budget validate`,
    `directory-map validate`, and `./rhino md internal-link validate` exit `0`; `npm run test:quick` exits `0`
    (`selected production line coverage: 99.30% (846/852 statements)`).
- [x] `[AI]` Run [docs propagation](../../../repo-governance/workflows/quality/docs-propagation.md) for the unit; proof:
      its status and updated files recorded here. `[AC-01]`
  - Result: (2026-10-06) status `landed`. Updated: `specs/behaviours/README.md` (the feature index now names NilAway and
    the domain literal analysis), and the plan documents that describe the repository:
    `tech-docs/003-specification-changes.md` (the scenario diff now carries the as-built step order and wording, and
    four new steps, not three), `tech-docs/002-gates-and-analysis.md` (the `x/sys` move in the selection argument, and
    where the adapter record now lives), and `tech-docs/004-file-impact.md` (Unit 1 gains
    `tests/integration/run_test.go`, `tests/support/driver.go`, the adapter module and its index, and
    `specs/behaviours/README.md`). Unchanged, checked: `README.md` (its `npm run test:quick` line already says "lint"
    and lists no linter), `docs/` (no page describes the lint or analysis gates), `CHANGELOG.md` (no `Unreleased`
    section exists or has existed; the `v0.8.5` entry is Unit 7's), and `specs/architecture.md` (describes no gate).
    Removed: none. Not run: none; no affected document shows a command.
- [x] `[AI]` Run the
      [Gherkin implementation review](../../../repo-governance/workflows/quality/gherkin-implementation-review.md) over
      the two changed scenarios; proof: each status with its implementation and test path recorded here, none
      `untested`, `unimplemented`, or `drifted`. `[AC-05]`
  - Result: (2026-10-06) both `implemented`, after one rerun. "Production code compares no domain value with a literal":
    implementation `tests/support/domain_literals.go` (`analyzeDomainLiterals`, `reconcileDomainFindings`) over
    `tests/support/domain_literals_allowlist.go`, bound in `tests/support/steps.go`; tests
    `tests/support/domain_literals_internal_test.go` and the scenario in the unit and integration adapters, which failed
    with 31 findings on an empty allowlist and named the planted line in the Unit 1 mutation. "Lint gate wiring is
    exhaustive and module scoped": implementation `.golangci.yml`, `scripts/test-quick.sh`, and `go.mod`, checked by
    `requireNilnessAndExhaustiveMaps` and `requirePinnedNilAway` in `tests/support/driver.go`; tests
    `tests/support/lint_wiring_internal_test.go` and the scenario in both adapters. The first pass found the exhaustive
    `switch`/`map` clause `untested`: the check matched those words in the block's own comment, so a scratch copy passed
    with both list items deleted (see [learnings](learnings.md)). Review finding F1 below fixed it test-first: comment
    and blank lines are dropped and the exact list items are required. The new mutation test covers `- map` and
    `- switch` deleted, `check:` removed, `nilness` commented out, moved to `disable:`, or enabled and disabled, and the
    NilAway line followed by `|| true`, `&`, `; true`, or `| cat`, each failing its step. Rerun:
    `go test -count=1 -v -run 'LintWiring|DomainLiteral' ./tests/support` exits `0` (23 `--- PASS`), and both scenarios
    pass in `TestUnitBehaviours` and `TestIntegrationBehaviours` (`ok`).
- [x] `[AI]` Dispatch `swe-reviewer` over the unit's diff; proof: its findings recorded here with none blocking open.
      `[AC-01]`
  - Result: `swe-reviewer` (2026-10-06): F1 HIGH, the lint-wiring step could not catch its own mutation: fixed
    test-first as above. F2 MEDIUM, no per-code `retryable` test: fixed by
    `TestRetryableMarksOnlyTheCodesWhoseConditionLiftsOnItsOwn` in `tests/unit/vocabulary_test.go`, proved by two flips.
    F3 MEDIUM, the `x/sys` bump unrecorded: recorded in the `tech-docs/002-gates-and-analysis.md` selection argument;
    `govulncheck` runs in the full gate. F4 LOW, the NilAway exclusions are accurate but no gate detects a stale
    directive: accepted, recorded in the adapter's Go analysis gates module. F5 LOW, a dead guard in
    `tests/support/release_v04.go`: fixed with a fixed-size array. F6 LOW, analyzer gaps (`-1` and rune literals,
    `string(x)` conversions, named results, an allowlist key without the identifier): accepted as outside the PW-1 and
    PW-2 rules, recorded in [learnings](learnings.md). None blocking open.
- [x] `[AI]` Run the _Full gate_; proof: exit `0` with the coverage figure recorded. `[AC-01]` `[AC-02]`
  - Result: (2026-10-06) `npm test` exit `0`: selected production line coverage 99.30% (846/852), race detector clean,
    `govulncheck` "No vulnerabilities found."

### Unit 1 landing

- [x] `[AI]` Inspect the diff against
      [data safety](../../../repo-governance/conventions/public-repository-data-safety.md) and commit thematically;
      proof: every hook passes and `git log --oneline origin/main..HEAD` is recorded. `[AC-01]`
  - Result: (2026-10-06) data-safety scan of the diff found no candidate; six thematic commits, every hook passed:
    `7e03023`, `8539e1b`, `6a4ec32`, `44e9424`, `737fff8`, `083d691`.
- [x] `[AI]` Run the [push review](../../../repo-governance/workflows/quality/pr-leak-review/002-push-review.md), push,
      screen the title and body with `scripts/public-safety/outbound-preflight.sh --surface pull-request`, and open a
      draft pull request; proof: the screen exits `0` and the pull request number is recorded. `[AC-01]`
  - Result: push review clean, every `pre-push` gate passed; title and body screened (exit `0`); draft pull request
    #132.
- [x] `[AI]` Mark it ready and wait, polling no faster than every three minutes, for `Quality gate` on the head; proof:
      `success` on the recorded head. `[AC-01]`
  - Result: `Quality gate` `success` on head `083d691` (run 37399366467).
- [x] `[AI]` Post the [leak review](../../../repo-governance/workflows/quality/pr-leak-review.md) for that exact head;
      proof: the `leak-review` status on the head reads `success`. `[AC-01]`
  - Result: `pass` review posted on `083d691`; `leak-review` reads `success`.
- [x] `[AI]` Rebase-merge once every [merge precondition](../../../repo-governance/conventions/pull-request-merge.md)
      holds, then _Reconcile_; proof: the merge commit recorded and the reconcile count `0 0`. `[AC-01]`
  - Result: merged as `341cb75`; reconcile count `0 0`.

> **Pause Safety**: Unit 1 is on `main`. Safe to stop. To resume: the starting commands for Unit 2.

## Phase 2: Unit 2 — Outcome

Branch `worktree/typed-run-outcome`.

- [x] `[AI]` Create the branch with the starting commands; proof: `git branch --show-current` prints it. `[AC-09]`
  - Result: `worktree/typed-run-outcome` from `origin/main` at `341cb75`.
- [x] `[AI]` **RED** (`swe-developer`): add "History lists an outcome this version does not know as recorded" to
      `specs/behaviours/public-cli.feature`; run the unit adapter; acceptance: its steps are undefined. Bind them in
      `tests/support/steps.go` and `tests/support/history_v05.go`; acceptance: it passes, because `v0.8.4` reads the
      outcome as a plain string — this scenario pins tolerance through the change, and the mutation below proves it can
      fail. `[AC-11]`
  - Result: (2026-10-06) The scenario is added to `specs/behaviours/public-cli.feature` (after "History filters current
    labeled summaries", no `@e2e-exempt` tag, so it runs at all three boundaries). RED:
    `HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd` and
    `go test -count=1 -run 'TestUnitBehaviours/<scenario>' ./tests/unit`, scenario
    `History_lists_an_outcome_this_version_does_not_know_as_recorded`, both exited `1` with
    `undefined behavior step "a current summary whose outcome is future-outcome"`,
    `"JSON history is requested for thirty days"`, and `"it exits 0 and that row's outcome reads future-outcome"`. Bound
    in `tests/support/steps.go` (three steps, the outcome word captured) and `tests/support/history_v05.go`
    (`summaryRecordingOutcome` writes the bytes as plain JSON, not an `evidence.Summary`, because the typed summary
    cannot hold a word this version does not know; `requestJSONHistory` runs in process at the unit and integration
    boundaries and through `runBinaryInRoot` against the compiled binary at the end-to-end one;
    `requireHistoryRowOutcome`). GREEN as the item predicts, since `v0.8.4` reads the outcome as a plain string: the
    unit adapter exits `0` (`3 steps (3 passed)`), the structural `./tests/bdd` form exits `0` for the unit,
    integration, and e2e adapters, and the integration and end-to-end adapters
    (`-run 'TestIntegrationBehaviours/...' ./tests/integration`, `-run 'TestE2EBehaviours/...' ./tests/e2e`) exit `0`
    for the scenario. The mutation item below proves it can fail.
- [x] `[AI]` Prove PW-4's condition, that no released HIPPO version ever wrote `pressure-shed` or `storage-shed` into
      `budgetOutcome`, before `BudgetOutcome` is typed: `git tag --list 'v*'` lists `v0.8.4`, then run the PW-4 proof
      from _Commands the items name_; proof: the output recorded here. Empty output means every release tag writes the
      field only through `SetReservationContext`, called only with `"admitted"`: the condition holds, `BudgetOutcome`
      keeps the members `Unset` and `Admitted`, and the history GREEN below deletes the two comparisons. Any line
      printed names a tag and another writer: the condition fails, the comparisons stay, typed, `BudgetOutcome` gains a
      member for each value printed, and the next RED, the next GREEN, and the history GREEN follow that branch. Record
      which branch applies. `[AC-11]`
  - Result: (2026-10-06) `git tag --list 'v*'` lists 19 tags, `v0.1.0` to `v0.8.4`, and `v0.8.4` is among them. The PW-4
    proof from _Commands the items name_ printed **nothing** (empty output after the three filters). Checked that the
    emptiness is not vacuous: before the filters the loop printed 60 lines, exactly four per tag for the 15 tags
    `v0.4.0` to `v0.8.4` (the `SetReservationContext` declaration, the one `summary.BudgetOutcome = outcome` assignment,
    and two calls, `internal/guard/run.go` and `tests/support/pending_v04.go`, each passing the literal `"admitted"`),
    and none for the four tags `v0.1.0` to `v0.3.1`, which predate the field. The only places the words `pressure-shed`
    and `storage-shed` meet `BudgetOutcome` in any tag are the reader's two comparisons in
    `internal/evidence/history.go` (from `v0.6.0`), never a write. **Branch applied: the condition holds.** No released
    version wrote either value, so `BudgetOutcome` keeps the members `Unset` and `Admitted` only, the history GREEN
    deletes the two `budgetOutcome` comparisons, and the RED, GREEN, and REFACTOR items follow that branch.
- [x] `[AI]` **RED** (`swe-developer`): add `internal/evidence/outcome_test.go` (each member's wire string, `Unset` and
      `Unknown` refused by the encoder, strict `ParseOutcome`, tolerant `RecordedOutcome` round trip) and
      `tests/unit/outcome_vocabulary_test.go` (the list in `docs/reference/json-schemas.md` equals
      `evidence.Outcomes()`); run `go test -count=1 ./internal/evidence ./tests/unit`; acceptance: compilation fails on
      the missing type. `[AC-09]` `[AC-11]`
  - Result: (2026-10-06) Added `internal/evidence/outcome_test.go` (external `evidence_test` package: each member's wire
    string and `String`, `Outcomes()` listing the ten in order, the encoder refusing `OutcomeUnset`, `OutcomeUnknown`,
    and a non-member, strict `ParseOutcome` and strict `Outcome` decoding, the tolerant `RecordedOutcome` round trip
    with `omitzero`, and the same for `BudgetOutcome` and its reader form `RecordedBudgetOutcome`) and
    `tests/unit/outcome_vocabulary_test.go` (the bullets under "`outcome` is one of these values, and no other" in
    `docs/reference/json-schemas.md` equal `evidence.Outcomes()` as sets, in both directions). RED:
    `go test -count=1 ./internal/evidence ./tests/unit` exited `1`, both packages `[build failed]`:
    `internal/evidence/outcome_test.go:15:19: undefined: evidence.Outcome` (and every `evidence.Outcome*` member) and
    `tests/unit/outcome_vocabulary_test.go:35:35: undefined: evidence.Outcomes`. **Design choices the plan left open,
    recorded for the GREEN:** the tolerant types keep unexported fields behind `Recorded(Outcome)` and
    `RecordedBudget(BudgetOutcome)` constructors and `Outcome()`/`String()` accessors; `Outcome` and `BudgetOutcome`
    also decode strictly through `UnmarshalText`, because the typed writer summary (`guard.EvidenceSummary`) is decoded
    by tests and a `uint8` cannot read a string without one; `Outcome.String` derives from `MarshalText`, so the wire
    table stays in one place.
- [x] `[AI]` **GREEN** (`swe-developer`): add `internal/evidence/outcome.go` as
      [the design](tech-docs/001-domain-types.md#run-outcome-unit-2) specifies; run the same command; acceptance: it
      passes. `[AC-09]` `[AC-11]`
  - Result: (2026-10-06) Added `internal/evidence/outcome.go`: `Outcome` (`uint8`, `OutcomeUnset` zero through
    `OutcomeAdmissionFailed`, then the reader-only `OutcomeUnknown`), `Outcomes()`, `MarshalText` as one exhaustive
    `switch` (`OutcomeUnset`, `OutcomeUnknown`, and a non-member are refused), `String`, strict `ParseOutcome` and
    `UnmarshalText`, and `RecordedOutcome` (unexported member and text, `Recorded`, `Outcome()`, `String`, `IsZero`,
    `MarshalText`, never-failing `UnmarshalText`). The budget side follows the PW-4 branch: `BudgetOutcome` with
    `BudgetOutcomeUnset` and `BudgetOutcomeAdmitted` and, **beyond the two members the design names**, the reader-only
    `BudgetOutcomeUnknown`, because D6 asks the tolerant reader for an explicit unknown member carrying the raw text;
    `RecordedBudgetOutcome` mirrors `RecordedOutcome`, and one private `parseBudgetOutcome` derives the word from
    `MarshalText`. `go test -count=1 ./internal/evidence ./tests/unit` exits `0`: `ok internal/evidence 0.402s`,
    `ok tests/unit 195.264s`.
- [x] `[AI]` **REFACTOR** (`swe-developer`): keep one wire table in `MarshalText` and derive `ParseOutcome` from
      `Outcomes()`; run the same command and _Lint_; acceptance: both exit `0`. `[AC-09]`
  - Result: (2026-10-06) `ParseOutcome` now asks each outcome in `Outcomes()` for its `String()`, so `MarshalText` is
    the one table of wire words. `go test -count=1 ./internal/evidence ./tests/unit` exits `0`
    (`ok internal/evidence 0.176s`, `ok tests/unit 192.641s`) and _Lint_ exits `0` (`0 issues.`). **Surprises:** (1) the
    first _Lint_ run of this unit failed `6 issues` on code written in the earlier items, because those items do not run
    it: `goconst` (`"30d"` now three times in `tests/support/history_v05.go`, fixed with a `historyWindow` constant),
    `musttag` (three anonymous structs in `outcome_test.go` without `json` tags), and `prealloc` (two slices in
    `tests/unit/outcome_vocabulary_test.go`); fixed here, tests only. (2) The unit adapter's scenarios "internal guard
    regression ..." run `go test` over `internal/guard`, so a RED that stops that package compiling fails about a dozen
    unit-adapter scenarios; the unit adapter can be run only while `internal/guard` compiles, which is why this item's
    final run was made with the next item's RED test file held aside. See [learnings](learnings.md).
- [x] `[AI]` **RED** (`swe-developer`): add to `internal/guard/run_test.go` a `finalOutcome` test (unset gives
      `supervision-failed` and `hippo.supervision.failed`) and a test that a deadline deferral's summary records
      `capacity-deferred`; run `go test -count=1 -run 'FinalOutcome|DeadlineDeferral' ./internal/guard`; acceptance:
      compilation fails on `finalOutcome`. `[AC-10]` `[AC-09]`
  - Result: (2026-10-06) `internal/guard/run_test.go` gains `TestFinalOutcomeFailsAnUnsetOutcomeAsASupervisionFailure`
    (every member of `evidence.Outcomes()` passes through `finalOutcome` unchanged; `OutcomeUnset` gives
    `OutcomeSupervisionFailed` and a `status.Failure` naming `hippo.supervision.failed`, which `status.Status` maps to
    `125`) and `TestDeadlineDeferralSummaryRecordsCapacityDeferred` (the summary a deadline deferral writes carries
    `"outcome": "capacity-deferred"`, decoded as a plain string so the test compiles before and after the type). The
    existing `TestRunHostAdmissionDeferralWritesNeverStartedReceipt` fixture is extracted into `deferAtTheDeadline` and
    shared by it and the new deferral test, which keeps its assertions. RED:
    `go test -count=1 -run 'FinalOutcome|DeadlineDeferral' ./internal/guard` exited `1`:
    `internal/guard/run_test.go:163:20: undefined: finalOutcome` and `:167:16: undefined: finalOutcome`,
    `FAIL ... [build failed]`.
- [x] `[AI]` **GREEN** (`swe-developer`): type the writer in `internal/guard/run.go` and `internal/guard/evidence.go`,
      start the outcome unset, assign `OutcomeCapacityDeferred` at the deadline deferral, and add `finalOutcome`; in
      this same item delete the Unit 2 entries for `internal/guard/evidence.go` from
      `tests/support/domain_literals_allowlist.go`, because the analysis fails on an entry whose violation is gone; run
      the same command and the unit adapter; acceptance: both pass. `[AC-09]` `[AC-10]` `[AC-08]`
  - Result: (2026-10-06) `internal/guard/run.go`: the ten `outcome...` string constants are gone;
    `var outcome evidence.Outcome` starts `OutcomeUnset`; every assignment names a member (the 22 existing ones, and the
    deadline deferral now assigns `OutcomeCapacityDeferred` before it writes its receipt); the `finalize` closure
    records `finalOutcome(outcome)` and joins the failure it returns with the write and cleanup errors; `finalOutcome`
    is a pure function returning `OutcomeSupervisionFailed` and a `status.Failure` naming `hippo.supervision.failed` for
    `OutcomeUnset`. `RunOutcomes()` stays for now, derived from `evidence.Outcomes()`, until its REFACTOR item deletes
    it. `internal/guard/evidence.go`: `EvidenceSummary.Outcome` is `evidence.Outcome`, `EvidenceSummary.BudgetOutcome`
    is `evidence.BudgetOutcome` (`omitempty` still omits the unset value), `Finalize` and `SetReservationContext` take
    the types. **File impact additions:** `internal/guard/reservation.go` (the cancelled-receipt reason used the deleted
    constant; now `evidence.OutcomeAdmissionCancelled.String()`), `tests/support/driver.go`,
    `tests/support/pending_v04.go`, and `tests/integration/lease_evidence_test.go` (their `Finalize` and
    `SetReservationContext` calls pass members). The four Unit 2 allowlist entries for `internal/guard/evidence.go`
    (`SetReservationContext.outcome`, `EvidenceSummary.Outcome`, `EvidenceSummary.BudgetOutcome`, `Finalize.outcome`)
    are deleted in this item. Proof: `go test -count=1 -run 'FinalOutcome|DeadlineDeferral' ./internal/guard` exits `0`;
    the executing unit adapter (`go test -count=1 -run TestUnitBehaviours ./tests/unit`) exits `0` (`ok ... 239.158s`),
    including "Production code compares no domain value with a literal", and
    `HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd` exits `0`. Check that the deferral test can fail: with the new
    assignment removed it fails with `code=1 error=hippo: [hippo.supervision.failed]` and the message
    `the run ended without deciding its outcome; its summary records supervision-failed`, the unset default doing what
    D14 says; restored, it passes.
- [x] `[AI]` **RED** (`swe-developer`): add to `internal/guard/run_test.go` a table test of
      `promoteFinalize(exitCode, returnError, finalizeError, launched)`: no finalize error leaves the exit code and
      error as they are; an error the run already returned stands over a finalize error; with no run error, a finalize
      error becomes the run's error with exit code `1`, naming a refused write when nothing launched; and the failure
      `finalOutcome` returns for an unset outcome comes back with code `hippo.supervision.failed`, which `status.Status`
      maps to `125`; run `go test -count=1 -run PromoteFinalize ./internal/guard`; acceptance: compilation fails on
      `promoteFinalize`. `[AC-10]`
  - Result: (2026-10-06) `internal/guard/run_test.go` gains `TestPromoteFinalizeDecidesWhatTheCallerSees`, a 12-row
    table over `promoteFinalize(exitCode, returnError, finalizeError, launched)`: no finalize error leaves a clean
    result, a deferral status, and the run's own failure as they are; an error the run returned stands over a finalize
    error, after launch and over a refused write before it; with no run error a finalize error becomes the run's error
    with exit code `1`, over exit `0` and over a deferral status, keeping its own shape before launch unless the root
    refused a write, and named as `hippo.evidence.unwritable` ("recording the lifetime summary") when one did before
    launch; and the failure `finalOutcome` returns for an unset outcome comes back, before and after launch, with code
    `hippo.supervision.failed`, which `status.Status` maps to `125`. RED:
    `go test -count=1 -run PromoteFinalize ./internal/guard` exited `1`:
    `internal/guard/run_test.go:262:17: undefined: promoteFinalize`, `FAIL ... [build failed]`.
- [x] `[AI]` **GREEN** (`swe-developer`): extract the deferred promotion in `internal/guard/run.go` into the pure
      function `promoteFinalize`, with no hook added to `RunConfig`, and have the deferred closure call it; run the same
      command and `go test -count=1 ./internal/guard`; acceptance: both pass. `[AC-10]`
  - Result: (2026-10-06) `internal/guard/run.go` gains the pure function
    `promoteFinalize(exitCode, returnError, finalizeError, launched)`; the deferred closure now calls `finalize()` and
    hands its result to it, with no hook added to `RunConfig`. `go test -count=1 -run PromoteFinalize ./internal/guard`
    exits `0` (12 subtests pass) and `go test -count=1 ./internal/guard` exits `0` (`ok ... 15.098s`). Check that the
    table can fail: with the `returnError != nil` clause dropped from the first condition, the rows "an error the run
    returned stands over a finalize error" and "... over a refused write before launch" fail; restored, the table
    passes.
- [x] `[AI]` **REFACTOR** (`swe-developer`): leave the deferred closure holding only the call, and give
      `promoteFinalize` a comment stating that the unset outcome's supervision failure reaches the caller through it;
      run `go test -count=1 ./internal/guard` and _Lint_; acceptance: both exit `0`. `[AC-10]`
  - Result: (2026-10-06) The deferred closure in `Run` is now the one line
    `defer func() { exitCode, returnError = promoteFinalize(exitCode, returnError, finalize(), launched) }()`, and
    `promoteFinalize`'s comment states that the supervision failure `finalOutcome` returns for an unset outcome reaches
    the caller as its finalize error, so the boundary reports `hippo.supervision.failed` and exit `125`.
    `go test -count=1 ./internal/guard` exits `0` (`ok ... 15.403s`) and _Lint_ exits `0` (`0 issues.`). Surprise: the
    first _Lint_ run reported 4 `errorlint` findings in the item-8 table's helpers (`fmt.Errorf` formatting an error
    with `%v`); the `check` functions now return a message string, the empty string meaning nothing is wrong, in the
    same item. NilAway also exits `0` at this point.
- [x] `[AI]` **REFACTOR** (`swe-developer`): delete `guard.RunOutcomes` and have `internal/cli/history.go` list
      `evidence.Outcomes()`; run `go test -count=1 ./internal/... ./tests/unit`; acceptance: exit `0`. `[AC-12]`
  - Result: (2026-10-06) `guard.RunOutcomes` is deleted from `internal/guard/run.go` (the `git grep -n RunOutcomes` over
    `*.go` and the non-plan Markdown prints nothing), and `internal/cli/history.go` lists the outcomes from
    `evidence.Outcomes()` through a new private `outcomeNames`, which builds the same ten words in the same order, so
    the `--outcome must be one of ...` message is byte for byte what `v0.8.4` prints.
    `go test -count=1 ./internal/... ./tests/unit` exits `0` (`internal/cli 4.539s`, `internal/evidence 1.281s`,
    `internal/guard 16.403s`, `tests/unit 203.204s`).
- [x] `[AI]` **RED** (`swe-developer`): add to `internal/evidence/history_test.go` a row with outcome `future-outcome`
      and `budgetOutcome` `admitted`, asserting it is listed as recorded and never counts toward owner promotion; run
      `go test -count=1 ./internal/evidence`; acceptance: compilation fails on the typed fields. `[AC-11]`
  - Result: (2026-10-06) `internal/evidence/history_test.go` gains
    `TestHistoryListsAnUnknownOutcomeAsRecordedAndNeverPromotesOnIt`: a control run recorded as `passed` with budget
    outcome `admitted` is eligible for owner promotion; the same marshalled bytes with `"outcome":"future-outcome"`
    swapped in (the word a later version might write) is listed by `ReadHistory` as `future-outcome` (member
    `OutcomeUnknown`) with `admitted` (member `BudgetOutcomeAdmitted`), is re-encoded with both words intact, is not
    matched by an `OutcomePassed` filter, and makes `EvaluatePromotion` answer `recent-overlap-unhealthy`, never
    eligible. RED: `go test -count=1 ./internal/evidence` exited `1`, `[build failed]`:
    `history_test.go:168:12: cannot use Recorded(OutcomePassed) (value of struct type RecordedOutcome)`
    `as string value in struct literal`,
    `:168:52: cannot use RecordedBudget(BudgetOutcomeAdmitted) ... as string value`,
    `:207:21: rows[0].Outcome.String undefined (type string has no field or method String)`, and
    `:217:85: cannot use OutcomePassed (constant 1 of uint8 type Outcome) as string value in struct literal`.
- [x] `[AI]` **GREEN** (`swe-developer`): type `Summary`, `Query`, and `healthyPromotionSummary` in
      `internal/evidence/history.go` and rename the history flag field to `outcomeFlag` in `internal/cli/commands.go`,
      parsed with `ParseOutcome`. Typing `Summary.BudgetOutcome` makes its two string comparisons stop compiling, so
      this same item applies the branch the PW-4 proof item recorded: when the condition holds, delete the two
      `budgetOutcome` comparisons; otherwise replace both with comparisons against the typed `BudgetOutcome` members. In
      this same item delete the Unit 2 entries for `internal/evidence/history.go` and `internal/cli/commands.go` from
      `tests/support/domain_literals_allowlist.go`; run the same command and the unit adapter; acceptance: both pass.
      `[AC-11]` `[AC-12]` `[AC-08]`
  - Result: (2026-10-06) `internal/evidence/history.go`: `Summary.Outcome` is a `RecordedOutcome` and
    `Summary.BudgetOutcome` a `RecordedBudgetOutcome`, both with `omitzero` (a summary with neither still omits the
    keys), `Query.Outcome` is an `Outcome` with `OutcomeUnset` meaning no filter, `matchesQuery` compares members, and
    `healthyPromotionSummary` asks for `OutcomePassed`, which an unknown recording never is. **PW-4 branch applied (the
    condition holds): the two `budgetOutcome` comparisons are deleted**, not retyped. `aggregateHistoryRows` keys on the
    recorded text. `internal/cli/commands.go`: the history flag field is `outcomeFlag`; `internal/cli/history.go` parses
    it through a new `outcomeFilter` with `evidence.ParseOutcome`, so `--outcome future-outcome` still exits `2` naming
    `hippo.args.invalid` and the ten outcomes (the error check order is unchanged: source, class, tier, outcome). The
    seven Unit 2 allowlist entries that remained after the writer item (`historyOptions.outcome`, `Summary.Outcome`,
    `Summary.BudgetOutcome`, `Query.Outcome`, and three `healthyPromotionSummary` comparisons) are deleted, and with
    them the now-unused `promotionSummary` and `removedByOutcome` constants and the empty Unit 2 group comment, which
    `unused` would otherwise fail on. **File impact additions:** `internal/cli/interruption_test.go`,
    `tests/support/interruption_v082.go` (typed reads), and the existing fixtures in
    `internal/evidence/history_test.go`, `internal/cli/history_test.go`, `tests/support/history_v05.go`, and
    `internal/guard/run_test.go` now use `Recorded(...)`. Proof: `go test -count=1 ./internal/evidence` exits `0` (the
    item-12 test passes); `go test -count=1 -run DomainLiteral ./tests/support` exits `0`; the executing unit adapter
    exits `0` (`ok ... 199.450s`, including the analysis scenario with the seven entries gone) and
    `HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd` exits `0`.
- [x] `[AI]` **REFACTOR** (`swe-developer`): simplify `healthyPromotionSummary` now that it reads only typed members;
      run `git grep -nE '"(pressure|storage)-shed"' -- internal/evidence/history.go` and
      `go test -count=1 ./internal/evidence ./tests/unit`; acceptance: the grep prints nothing and the tests exit `0`.
      `[AC-11]`
  - Result: (2026-10-06) `healthyPromotionSummary` now returns early for anything but a recorded `OutcomePassed` with
    `AggregateCount` `0`, names the minimum-memory pointer once, and carries a comment saying why the budget outcome is
    not consulted (only `passed` counts, so a shed, deferred, or unknown run never does).
    `git grep -nE '"(pressure|storage)-shed"' -- internal/evidence/history.go` prints nothing (exit `1`), and
    `go test -count=1 ./internal/evidence ./tests/unit` exits `0` (`ok internal/evidence 0.418s`,
    `ok tests/unit 202.821s`).
- [x] `[AI]` Mutation: make `RecordedOutcome.UnmarshalText` refuse unknown text, run the unit adapter, record the
      output, restore it; acceptance: the unknown-outcome history scenario fails, and passes once restored. `[AC-11]`
  - Result: (2026-10-06) Mutation: `RecordedOutcome.UnmarshalText` returned the `ParseOutcome` error for a word it does
    not know instead of keeping it. The full executing unit adapter
    (`go test -count=1 -run TestUnitBehaviours ./tests/unit`) exited `1` with exactly one failure,
    `338 scenarios (337 passed, 1 failed)`, `1042 steps (1040 passed, 1 failed, 1 skipped)`:
    `Scenario: History lists an outcome this version does not know as recorded`,
    `When JSON history is requested for thirty days`,
    `Error: hippo: [hippo.evidence.unreadable] reading run history: unknown outcome "future-outcome"`. The same scenario
    also failed at the integration and end-to-end adapters (`-run 'TestIntegrationBehaviours/...'` and
    `-run 'TestE2EBehaviours/...'`, both exit `1`), so the tolerance is pinned at all three boundaries. The file was
    restored byte for byte (`diff` against the saved copy is empty) and the scenario passes again at all three (exit
    `0`).
- [x] `[AI]` Enable `exhaustruct_v5` in `.golangci.yml` with an enforce pattern for `evidence.RecordedOutcome`, then
      plant a literal omitting a field, run _Lint_, record the output, and remove it; acceptance: the planted literal
      fails naming `exhaustruct`, which confirms the pattern syntax, and lint exits `0` once removed. `[AC-09]`
  - Result: (2026-10-06) `.golangci.yml`: `exhaustruct_v5` leaves the `disable` list (its reason comment goes with it;
    `exhaustruct` stays disabled with its reason, now noting that the v5 successor is enabled below, scoped) and gains
    settings: `explicit-mode: true`, so only a type a pattern names is checked, and `enforce-patterns` with the regexes
    `^github\.com/wahidyankf/hippo/internal/evidence\.RecordedOutcome$` and
    `^github\.com/wahidyankf/hippo/internal/evidence\.RecordedBudgetOutcome$` (the second added beyond the plan's single
    pattern, because `RecordedBudgetOutcome` is the other struct this unit creates), each comment-explained. **Pattern
    syntax confirmed from `dev.gaijin.team/go/exhaustruct/v5@v5.0.3`:** a regex matched against the full path
    `package/import/path.TypeName`, not the package-qualified short name; in the default implicit mode every literal is
    checked, so `explicit-mode` is what makes the scope narrow (the plan's text names only `enforce-patterns`). With the
    setting on and nothing planted, _Lint_ exits `0` (`0 issues.`), so no existing literal, and no fixture elsewhere in
    the module, is caught. Planted `internal/evidence/exhaustruct_mutation.go` with
    `var _ = RecordedOutcome{outcome: OutcomePassed}` and `var _ = RecordedBudgetOutcome{text: "admitted"}`: _Lint_
    exited `1`:
    `internal/evidence/exhaustruct_mutation.go:4:9: evidence.RecordedOutcome is missing field text (exhaustruct_v5)` and
    `:7:9: evidence.RecordedBudgetOutcome is missing field budget (exhaustruct_v5)` (`2 issues: exhaustruct_v5: 2`). The
    file was deleted (`git status --short` shows no trace) and _Lint_ exits `0` again (`0 issues.`).
- [x] `[AI]` Confirm Unit 2's entries are gone from `tests/support/domain_literals_allowlist.go`, each deleted in the
      item that removed its violation; run the unit adapter; acceptance: the allowlist holds no entry tagged Unit 2 and
      the adapter exits `0`. `[AC-08]`
  - Result: (2026-10-06) `grep -c "Unit 2\|removedByOutcome" tests/support/domain_literals_allowlist.go` prints `0`: no
    entry, no unit constant, and no group comment is tagged Unit 2 any more. The 11 Unit 2 entries were deleted in the
    items that removed their violations (4 for `internal/guard/evidence.go` in the writer GREEN, 7 for
    `internal/evidence/history.go` and `internal/cli/commands.go` in the history GREEN). Remaining: 20, Unit 4: 14
    (`removedByLineage`), Unit 6: 6 (`removedByDecoding`), the 31 at introduction less 11. The executing unit adapter
    (`go test -count=1 -run TestUnitBehaviours ./tests/unit`) exits `0` (`ok ... 215.629s`), including "Production code
    compares no domain value with a literal" (so no stale entry and no new finding) and "Lint gate wiring is exhaustive
    and module scoped" (so the `.golangci.yml` edit left that wiring intact), and
    `HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd` exits `0`.

### Unit 2 close

- [x] `[AI]` Apply rules propagation to the `exhaustruct_v5` change (`.golangci.yml` reason and the adapter's [Go
      analysis gates][go-gates] module); proof: its status recorded here. `[AC-09]`
  - Result: (2026-10-06) status `landed`, two rows, both `resolved`. R1, the `.golangci.yml` reason: the
    `exhaustruct_v5` settings comment already says complete literals are required only for the closed domain types,
    never globally, that explicit mode checks only what a pattern names, and that patterns match the full import path
    and type name; the `exhaustruct` disable reason notes its scoped successor. Checked and left as written. R2, the
    adapter: the [Go analysis gates][go-gates] module gains an `exhaustruct_v5` entry (`explicit-mode: true`, one
    full-path regex per closed domain struct, and why: without explicit mode every literal is checked and the fixture
    brittleness returns), and its description and index entry name it. Disposition: covered by _Lint_, shown failing on
    the planted literals in the item above. The as-built pattern syntax and `explicit-mode` also go into
    `tech-docs/002-gates-and-analysis.md`'s scope paragraph, routing the second Unit 2 learning.
    `./rhino governance word-budget validate` and `directory-map validate` exit `0`.
- [x] `[AI]` Run docs propagation: `docs/reference/json-schemas.md` says history lists an unknown outcome as recorded;
      proof: its status recorded and `npm run format:check` exits `0`. `[AC-11]`
  - Result: (2026-10-06) status `landed`. Updated: `docs/reference/json-schemas.md` (`history --json`: a row whose
    `outcome` or `budgetOutcome` this version does not know is listed as recorded, never refused or defaulted; an
    unknown `outcome` never qualifies for owner promotion and no `--outcome` value selects it; the outcome list that
    `tests/unit/outcome_vocabulary_test.go` reads is unchanged), `docs/reference/cli.md` (the `--outcome` filter note
    says the same), `specs/architecture.md` (the `history` sentence lists an unknown outcome as recorded, never counted
    toward promotion), and `tech-docs/004-file-impact.md` (Unit 2 gains the two pages above and the six files the third
    Unit 2 learning names, now routed). Unchanged, checked: `README.md` (names no outcome),
    `docs/how-to/inspect-evidence-and-abandoned-groups.md` (links the outcome list), and `CHANGELOG.md` (no `Unreleased`
    section; the `v0.8.5` entry is Unit 7's). Removed: none. Not run: none; the pages' `hippo history` examples are
    unchanged. `npm run format:check` exits `0`.
- [x] `[AI]` Run the Gherkin implementation review over the new history scenario; proof: its status recorded. `[AC-11]`
  - Result: (2026-10-06) "History lists an outcome this version does not know as recorded": `implemented`.
    Implementation: `RecordedOutcome.UnmarshalText` in `internal/evidence/outcome.go`, which keeps an unknown word as
    `OutcomeUnknown` with its text, read by `ReadHistory` in `internal/evidence/history.go` and printed by
    `internal/cli/history.go`. Bound in `tests/support/steps.go` and `tests/support/history_v05.go`: the summary is
    written as plain JSON, and the `Then` step requires exit `0` and exactly one row with that run ID whose outcome
    reads `future-outcome`. Tests: the scenario at all three boundaries (`TestUnitBehaviours`,
    `TestIntegrationBehaviours`, `TestE2EBehaviours`, each `--- PASS` on this rerun) and
    `TestHistoryListsAnUnknownOutcomeAsRecordedAndNeverPromotesOnIt` in `internal/evidence/history_test.go`. The
    mutation item above made the decoder refuse unknown text, and the scenario failed at all three boundaries.
- [x] `[AI]` Dispatch `swe-reviewer` over the unit's diff; proof: findings recorded, none blocking open. `[AC-09]`
  - Result: (2026-10-06) no blocking finding. F1 MEDIUM (five outcome words unasserted at the summary) fixed:
    `requireSummaryOutcome` pins all six shed/stop/block/supervision words in `internal/guard/run_test.go` and
    `tests/integration/run_test.go`, plus a new `TestRunLosingSupervisionAfterLaunchRecordsSupervisionFailed`; each of
    six member swaps in `run.go` failed its test. F2 MEDIUM (`Outcomes()` hand-kept) fixed: derived from the member
    range, pinned by `TestOutcomesListsEveryMemberBetweenUnsetAndUnknown` under three mutations. F3 LOW:
    `reservation.go` derives the retry-wait reason from `OutcomeAdmissionCancelled`. F4 LOW: the strict `UnmarshalText`
    methods document that recorded-evidence readers use the `Recorded*` types (D6). F5 LOW accepted under PW-6.
- [x] `[AI]` Run the _Full gate_; proof: exit `0` with the coverage figure recorded. `[AC-09]` `[AC-10]` `[AC-11]`
      `[AC-12]`
  - Result: (2026-10-06) `npm test` exit `0`: coverage 99.30% (846/852), race clean, `govulncheck` "No vulnerabilities
    found." The first run timed out in the race step (`tests/integration` past Go's 10-minute package default) at host
    load 15–18; alone it passed in 359 s, and the rerun passed with the race step at 534 s.

### Unit 2 landing

- [x] `[AI]` Inspect the diff against data safety and commit thematically; proof: hooks pass, range recorded. `[AC-09]`
  - Result: (2026-10-06) data-safety scan of the diff found no candidate; thematic commits, every hook passed:
    `3dc1572`, `efb02ee`, `8d27255`, `84db85b`, `009f976`.
- [x] `[AI]` Push review, push, screen title and body, open a draft pull request; proof: screen exit `0`, number
      recorded. `[AC-09]`
  - Result: push review clean, every `pre-push` gate passed; title and body screened (exit `0`); draft pull request
    #133.
- [x] `[AI]` Mark ready and wait for `Quality gate` on the head; proof: `success` on the recorded head. `[AC-09]`
  - Result: `Quality gate` `success` on head `009f976` (run 37411579434).
- [x] `[AI]` Post the leak review for that head; proof: `leak-review` reads `success`. `[AC-09]`
  - Result: `pass` review posted on `009f976`; `leak-review` reads `success`.
- [x] `[AI]` Rebase-merge when every precondition holds, then _Reconcile_; proof: merge commit and `0 0` recorded.
      `[AC-09]`
  - Result: merged as `a7b3700`; reconcile count `0 0`.

> **Pause Safety**: Unit 2 is on `main`. Safe to stop. To resume: the starting commands for Unit 3.

## Phase 3: Unit 3 — Internal Reasons

Branch `worktree/typed-internal-reasons`. No specification changes: the contract scenarios already pin every status and
code, and run unchanged as regressions.

- [x] `[AI]` Create the branch with the starting commands; proof: `git branch --show-current` prints it. `[AC-13]`
  - Result: `worktree/typed-internal-reasons` from `origin/main` at `a7b3700`.
- [x] `[AI]` **RED** (`swe-developer`): add `tests/unit/reason_test.go` pinning each member's `v0.8.4` integer (`0`,
      `73`, `74`, `75`, `76`, `78`) and `Stopped`'s unwrapping; run `go test -count=1 ./tests/unit`; acceptance:
      compilation fails on `policy.Reason`. `[AC-13]` `[AC-14]`
  - Result: (2026-10-06) `tests/unit/reason_test.go` pins each member's `v0.8.4` integer (`0`, `73`, `74`, `75`, `76`,
    `78`) through `json.Marshal`, refuses a non-member, reads back only those six integers (`1`, `2`, `72`, `77`, `79`,
    `-73`, `73.5`, `"73"`, `true`, and `[]` all refused), and pins `Stopped`'s unwrapping, `errors.AsType` through a
    wrapping error, a cause-less stop's text, and `policy.BareStop`. RED observed: `go test -count=1 ./tests/unit`
    reports `tests/unit/reason_test.go:18:16: undefined: policy.Reason` (and the five members) with
    `FAIL ... [build failed]`.
- [x] `[AI]` **GREEN** (`swe-developer`): add `internal/policy/reason.go`; run the same command; acceptance: it passes.
      `[AC-13]`
  - Result: (2026-10-06) `internal/policy/reason.go` adds `Reason` (`ReasonNone`, `ReasonStorageBlocked`,
    `ReasonCapacityDeferred`, `ReasonPressureShed`, `ReasonProtocolMismatch`, `ReasonReplanRequired`), its
    legacy-integer codec (`MarshalJSON` and a strict `UnmarshalJSON`), `Stop`, `Stopped`, and `BareStop`. The new tests
    pass (7 passed) and `go test -count=1 ./tests/unit` exits `0` (253 s). Two additions beyond the plan's list, both
    needed by the design: `UnmarshalJSON`, because `tests/support/driver.go` decodes status JSON into
    `policy.Resolution` and a bare `uint8` would silently read `75` as no member; and `BareStop`, the cause-less stop
    that `promoteFinalize` and the ownership-release path treat as they treated a `nil` error. `Stop` carries
    `//nolint:errname` with its reason, as `status.Failure` and `status.Interruption` do. The unit run overlapped one
    compile-neutral edit to `reason.go`, so the REFACTOR run below repeats it clean.
- [x] `[AI]` **REFACTOR** (`swe-developer`): keep the integer table in one `switch`; run the same command and _Lint_;
      acceptance: both exit `0`. `[AC-13]`
  - Result: (2026-10-06) The integer table was already one `switch` (`Reason.legacyExitCode`), read in both directions:
    `MarshalJSON` calls it and `UnmarshalJSON` asks every `uint8` value for its integer, so a new member needs no second
    table. Nothing to move. `go test -count=1 ./tests/unit` exits `0` (351 s, run clean after the last edit) and
    `go tool golangci-lint run` exits `0` (the first lint run found `errname` on `Stop`, `errorlint` in the new test,
    and `nilerr` in `BareStop`; the first and third were fixed in the code, the second in the test, and the `errname`
    finding carries the justified `//nolint` described above).
- [x] `[AI]` **RED** (`swe-developer`): add `internal/cli/status_test.go` asserting each reason's code, exit status, and
      message, and that `ReasonNone` inside a stop reports `hippo.supervision.failed`; run
      `go test -count=1 ./internal/cli`; acceptance: compilation fails on `reasonCode`. `[AC-13]`
  - Result: (2026-10-06) `internal/cli/status_test.go` asserts each of the five reasons' code, exit status (`124` or
    `125`), and message through `reasonCode`, `reasonMessage`, and `classify`; that a stop with a cause reports the
    cause's text and one inside a wrapping error keeps its reason; that `ReasonNone` and a value that is no member
    inside a stop report `hippo.supervision.failed` (`125`); that a classified failure inside a stop outranks it; and
    that a started child's status is never explained. RED observed: `go test -count=1 ./internal/cli` fails
    `[build failed]` with `undefined: reasonCode`.
- [x] `[AI]` **GREEN** (`swe-developer`): replace `internalReasons` with `reasonCode` and switch `reasonMessage` over
      `policy.Reason` without a `default` in `internal/cli/status.go`; run the same command; acceptance: it passes.
      `[AC-13]`
  - Result: (2026-10-06) `internal/cli/status.go` gains `reasonCode(policy.Reason) status.Code` and switches
    `reasonMessage` over `policy.Reason`, neither with a `default`; `classify` reads a `*policy.Stop` first (a bare stop
    says the reason's sentence, a stop with a cause says the cause), and `go test -count=1 ./internal/cli` exits `0`.
    Deviation, recorded in `learnings.md`: the plan orders this item before the guard and handlers return stops (items 8
    to 10), and the `internal/cli` tests drive the real guard (`TestPressureShedNamesItsOwnReason`, the
    protocol-mismatch tests), which still return the integers here. So `internalReasons` is kept for now as
    `legacyReasons`, retyped to map each integer to its `policy.Reason`, read after the stop; the later REFACTOR that
    deletes the five constants deletes it too.
- [x] `[AI]` **REFACTOR** (`swe-developer`): delete the `//nolint:exhaustive` above `reasonMessage`; run _Lint_ and
      `git grep -n 'nolint:exhaustive' -- internal`; acceptance: lint exits `0` and the grep prints nothing. `[AC-13]`
  - Result: (2026-10-06) The `//nolint:exhaustive` above `reasonMessage` is gone: both switches over `policy.Reason`
    name `ReasonNone` and fall to one trailing return. `go tool golangci-lint run` exits `0` and
    `git grep -n 'nolint:exhaustive' -- internal` prints nothing (exit `1`).
- [x] `[AI]` **RED** (`swe-developer`): add to `internal/guard/run_test.go` a test that a deferral whose summary write
      fails still reports the refused write, and switch the guard tests' deferral expectations to `*policy.Stop`; run
      `go test -count=1 ./internal/guard`; acceptance: the new expectations fail against integer returns. `[AC-13]`
  - Result: (2026-10-06) `internal/guard/run_test.go` now expects `*policy.Stop` where the deferral and shed tests
    expected integers: a new `requireStop` (status `0` beside a stop that carries only its reason) serves the
    host-admission and deadline deferrals, the emergency stop, and the held-service-port deferral; the
    unconfirmed-retirement table carries the stop it expects (`ReasonStorageBlocked`, `ReasonPressureShed`, and none for
    the cancellation); and the `promoteFinalize` table's deferral rows return a stop instead of exit `75`. Two
    additions: a stop that carries an error stands over a finalize error, and the new
    `TestADeferralWhoseSummaryCannotBeWrittenReportsTheFailedWrite`, which makes a real deadline deferral's summary link
    fail (a directory at the summary's name, derived from a fixed clock) and requires status `1` with the failed write
    and the deferral's one receipt. RED observed: `go test -count=1 ./internal/guard` reports 12 failed (for example
    `result code=75 error=<nil>, want status 0 and a stop for reason 2`,
    `result code=74 error=<nil>, want status 0 and a stop for reason 3`, and three `promoteFinalize` rows
    `exit code 0, want 1`). The new summary-write test passes against integer returns, since integers and `nil` already
    gave that precedence; it guards the stop-for-`nil` change and is proved by mutation at the GREEN.
- [x] `[AI]` **GREEN** (`swe-developer`): return `policy.Stopped(...)` at every site in `internal/guard/run.go`,
      `internal/guard/reservation.go`, `internal/policy/profiles.go`, `internal/cli/development.go`, and
      `internal/cli/release.go`, and treat a stop like `nil` in `promoteFinalize`; run
      `go test -count=1 ./internal/... ./tests/unit`; acceptance: exit `0`. `[AC-13]`
  - Result: (2026-10-06) Every site now returns `policy.Stopped(...)` beside status `0`: `internal/guard/run.go` (the
    deadline, reservation, lease, storage, protocol-mismatch, replan, and shed returns), `internal/cli/development.go`
    (replan, the schema-3 and status coordination mismatches through one `coordinationFailure`, and a non-fitting
    resolution), and `internal/cli/release.go`. `profiles.go` has no return site, only the two `Resolution.ExitCode`
    assignments, which move with the `Resolution.Reason` items below; until then the two resolution returns read the
    reason through `legacyReasons`. `promoteFinalize` treats a stop that carries no error (`policy.BareStop`) as it
    treated `nil`, through `carriesNoError`, and a stop that carries an error stands, as the `(74, stopError)` it
    replaces did: the plan says every stop, which would have let a finalize error replace a shed whose child could not
    be confirmed stopped. The ownership-release and port-release defers use the same rule, and keep a storage or
    capacity deferral's reason beside a release failure as the old exit status survived it. Two things the plan did not
    name, both needed to keep the in-process contract: `Application.Run` still returns a `nil` error beside a bare stop
    (`TestPressureShedNamesItsOwnReason` pins it), and `endedByInterruption` treats a bare stop as no error. Mutation:
    `carriesNoError` reduced to `err == nil` failed `TestADeferralWhoseSummaryCannotBeWrittenReportsTheFailedWrite` and
    three `promoteFinalize` rows, and passed again once restored. `go test -count=1 ./internal/... ./tests/unit` exits
    `0` (the unit adapter in 524 s at host load 27 to 44), after the `tests/support` updates the next item lists, since
    the adapter drives them.
- [x] `[AI]` **GREEN** (`swe-developer`): update the test files
      [the file impact](tech-docs/004-file-impact.md#unit-3--internal-reasons) lists to read reasons from the stop; run
      the unit and integration adapters and `go test -count=1 ./tests/integration`; acceptance: all exit `0`. `[AC-13]`
  - Result: (2026-10-06) Test files now read the reason from the stop: `internal/guard/run_test.go` and
    `tests/integration/run_test.go` use `requireStop` (status `0` beside a bare `policy.Stop` naming the reason),
    `internal/cli/development_test.go` and `tests/unit/adaptive_test.go` read `Resolution.Reason`, and
    `tests/support/driver.go` records the reason beside the exit status in `recordGuardResult`, so `requireDeferred`,
    `requireShed`, `requireDegradedShed`, the lineage guard, and the protocol-mismatch sentinels in
    `degraded_lineage.go`, `blockers_v04.go`, `pending_v04.go`, `review_v04.go`, and `history_v05.go` compare
    `driver.reason`. `tests/support/loaded_gate.go` holds a test-local `internalDeferralStatus` (`75`) for its near-miss
    deferral, a status v0.8.4 never exited with. Results: `go test -count=1 -timeout 45m ./tests/unit` (the unit adapter
    and every package test there) exits `0` in 732 s, and `go test -count=1 -timeout 45m ./tests/integration` (the
    integration adapter) exits `0` in 392 s, both at host load 28 to 73 with the whole unit in place. An earlier
    integration run at the default 10-minute timeout failed on the timeout, not on a test, at load 27 to 44.
- [x] `[AI]` **REFACTOR** (`swe-developer`): delete the five constants and the bare `73`; run
      `git grep -nE '(StorageBlocked|PressureShed|CapacityDeferred|ReplanRequired|ProtocolMismatch)ExitCode' -- '*.go'`
      and _Lint_; acceptance: the grep prints nothing, because the pathspec leaves out the Markdown that names the
      constants, and lint exits `0`. `[AC-13]`
  - Result: (2026-10-06) Deleted `StorageBlockedExitCode`, `CapacityDeferredExitCode`, and `PressureShedExitCode` from
    `internal/guard/run.go`, `ReplanRequiredExitCode` and `ProtocolMismatchExitCode` from `internal/policy/profiles.go`
    (the bare `73` went with the `Resolution.Reason` GREEN), and the `legacyReasons` bridge from
    `internal/cli/status.go`, since no layer returns an integer reason any more and `classify` reads a `policy.Stop`
    alone after a classified failure.
    `git grep -nE '(StorageBlocked|PressureShed|CapacityDeferred|ReplanRequired|ProtocolMismatch)ExitCode' -- '*.go'`
    prints nothing (exit `1`), `git grep -nE 'nolint:exhaustive' -- '*.go'` prints nothing, and
    `go tool golangci-lint run` exits `0` with `0 issues`. Lint first reported three findings after the deletions, none
    a weakened check: an `exhaustive` map over `policy.Reason` in `internal/cli/development_test.go` (now a table), a
    `maintidx` finding on `TestRunUnconfirmedRetirementPreservesOwnershipAndExit` (the stop check moved into
    `requireUnconfirmedRetirementResult`), and a `nolintlint` finding that `gocyclo` no longer fires on
    `Application.run` (removed from that directive). NilAway
    (`go tool nilaway -include-pkgs=github.com/wahidyankf/hippo -pretty-print=false ./...`) exits `0` and
    `go test -count=1 -run DomainLiteral ./tests/support` exits `0`.
- [x] `[AI]` **RED** (`swe-developer`): add to `tests/unit/reservation_test.go` cases that a ledger owner with
      `sheddingExitCode` `74` fails at decode and that storage and pressure sheds write `73` and `75`; run
      `go test -count=1 ./tests/unit`; acceptance: compilation fails on `guard.ShedCause`. `[AC-15]` `[AC-16]`
  - Result: (2026-10-06) `tests/unit/reservation_test.go` gains the ledger cases: `sheddingExitCode` `74`, a shedding
    owner with no cause, and a cause with no shedding each fail closed with the ledger bytes preserved (the corruption
    table); `TestALedgerShedCauseOutsideStorageAndPressureFailsAdmissionClosed` holds `AcquireReservation` over a ledger
    recording `74` to an error and unchanged bytes; `TestShedCauseKeepsItsV084LedgerCode` pins `0`, `73`, and `75` both
    ways and refuses `1`, `72`, `74`, `76`, `78`, `-73`, `73.5`, `"73"`, `true`, and `[]`;
    `TestShedCauseNamesTheReasonItsOwnerStopsFor` pins storage to `ReasonStorageBlocked` and pressure to
    `ReasonPressureShed`; and `TestEachShedWritesItsV084LedgerCode` reads the ledger after a storage and a pressure shed
    and requires `73` and `75`. `SelectPressureVictim` and `SelectEmergencyPressureVictim` callers take the typed
    causes. RED observed: `go test -count=1 ./tests/unit` fails `[build failed]` with `undefined: guard.ShedCause` (and
    `ShedCauseNone`, `ShedCauseStorage`, `ShedCausePressure`).
- [x] `[AI]` **GREEN** (`swe-developer`): add `ShedCause` and its codec to `internal/guard/reservation.go` and type
      `ReservationOwner`'s field; run the same command; acceptance: it passes. `[AC-15]` `[AC-16]`
  - Result: (2026-10-06) `internal/guard/reservation.go` adds `ShedCause` (`ShedCauseNone`, `ShedCauseStorage`,
    `ShedCausePressure`), `ShedCause.Reason()` (one exhaustive `switch`), and a ledger codec: `ledgerCode` is the one
    table (`0`, `73`, `75`), `MarshalJSON` refuses a non-member, and `UnmarshalJSON` asks every `uint8` value for its
    integer and refuses every other one, so a ledger recording `74`, `1`, or `"73"` fails closed at decode.
    `ReservationOwner.SheddingExit int` is now `SheddingCause ShedCause` (JSON name `sheddingExitCode`, omitted when
    none); the ledger validation, `ReservationSheddingSelection`, `SelectPressureVictim`,
    `SelectEmergencyPressureVictim`, and `Run`'s two shed sites take the type, and every test caller passes
    `guard.ShedCauseStorage` or `guard.ShedCausePressure`. The new tests pass and `go test -count=1 ./tests/unit` exits
    `0` (the same 524 s run as the stops item above). `validSheddingExitCode` and `callerShedReason` are now unused and
    go in the next item.
- [x] `[AI]` **REFACTOR** (`swe-developer`): delete `validSheddingExitCode` and `callerShedCode` for
      `ShedCause.Reason()`; run `go test -count=1 ./internal/guard ./tests/unit` and the unit adapter; acceptance: exit
      `0`, with "Reservation ledger structure is validated before mutation" passing. `[AC-15]`
  - Result: (2026-10-06) `validSheddingExitCode` and the guard's `callerShedReason` (the plan's `callerShedCode`) are
    deleted and nothing reads either:
    `git grep -nE 'validSheddingExitCode|callerShedCode|callerShedReason|SheddingExit\b' -- '*.go'` prints nothing. The
    ledger's shed-cause check is `ShedCause`'s strict decoder plus the `ShedCauseNone` comparisons in the ledger
    validation, and `ShedCause.Reason()` answers the reason a shed's caller is told. `go test -count=1 ./internal/...`
    exits `0`, and the full unit adapter run exits `0`. "Reservation ledger structure is validated before mutation"
    passes (`go test -count=1 -v -run 'TestUnitBehaviours/.*Reservation_ledger_structure' ./tests/unit`: `PASS`).
- [x] `[AI]` **RED** (`swe-developer`): add to `internal/cli/development_test.go` an assertion that status JSON's
      `profile.exitCode` encodes `73`, `75`, and `78` for cleanup, wait, and replan resolutions; run
      `go test -count=1 ./internal/cli`; acceptance: compilation fails on `Resolution.Reason`. `[AC-14]`
  - Result: (2026-10-06) `internal/cli/development_test.go` gains
    `TestStatusJSONKeepsEachResolutionsV084DecisionAndExitCode`, which runs `status --json` over normal pressure, a
    warning, and a disk below the floor and requires `profile.decision` and `profile.exitCode` of `run`/`0`, `wait`/`75`
    (retryable), and `cleanup`/`73`; and `TestAResolutionCarriesTheReasonItsDecisionStopsFor`, which requires
    `withAssessmentDecision` to set `ReasonStorageBlocked` or `ReasonCapacityDeferred` (and none for normal pressure), a
    resolution that already stops to keep its own reason, and `Resolution.Reason` to publish `73`, `75`, and `78` as
    `exitCode`. The `replan`/`78` row is the `ReasonReplanRequired` case of the last assertion here, and
    `tests/unit/adaptive_test.go` holds the strict profile that resolves to it. RED observed:
    `go test -count=1 ./internal/cli` fails `[build failed]` with
    `cleanup.Reason undefined (type policy.Resolution has no field or method Reason)`.
- [x] `[AI]` **GREEN** (`swe-developer`): replace `Resolution.ExitCode` with `Reason` encoded by `Reason.MarshalJSON`;
      run the same command; acceptance: it passes. `[AC-14]`
  - Result: (2026-10-06) `internal/policy/profiles.go`: `Resolution.ExitCode int` is now `Resolution.Reason Reason` with
    the JSON name `exitCode`, so `Reason.MarshalJSON` publishes the v0.8.4 integer; `Resolve` sets
    `ReasonStorageBlocked` for a cleanup and `ReasonReplanRequired` for a replan, which removes the bare `73` and
    `ReplanRequiredExitCode`. `internal/cli/development.go` and `release.go` read `resolution.Reason`
    (`return 0, policy.Stopped(resolution.Reason, nil)` when it is not `ReasonNone`), and `withAssessmentDecision` sets
    the reasons the next item asks for, because it was the last reader of the guard constants.
    `tests/unit/adaptive_test.go` and `tests/support/driver.go` read `Reason`.
    `go test -count=1 ./internal/cli ./internal/policy` passes, including
    `TestStatusJSONKeepsEachResolutionsV084DecisionAndExitCode` (`run`/`0`, `wait`/`75`, `cleanup`/`73`) and
    `TestAResolutionCarriesTheReasonItsDecisionStopsFor` (`73`, `75`, `78`).
- [x] `[AI]` **REFACTOR** (`swe-developer`): make `withAssessmentDecision` set reasons, not integers; run the unit
      adapter; acceptance: exit `0`. `[AC-14]`
  - Result: (2026-10-06) `withAssessmentDecision` sets `ReasonStorageBlocked` with decision `cleanup` for a blocked disk
    and `ReasonCapacityDeferred` with decision `wait` and `Retryable` for any other state but normal, and keeps the
    reason a resolution already carries; it names no integer. The edit landed with the `Resolution.Reason` GREEN above,
    since the function read the deleted guard constants, so the item's own evidence is the unit adapter after it: the
    full `go test -count=1 -timeout 45m ./tests/unit` (which runs `TestUnitBehaviours`, the _Unit adapter_) exits `0` in
    732 s at host load 52 to 73, with every Go change of the unit in place.

### Unit 3 close

- [x] `[AI]` Assess `specs/` and run docs propagation; record the verified no-op for the behaviour corpus and whether
      `specs/architecture.md`'s shedding-cause bullet still reads true; proof: the status recorded. `[AC-13]`
  - Result: (2026-10-06) status `landed`, plan documents only. Behaviour corpus: a verified no-op.
    `git diff --stat -- specs docs README.md CHANGELOG.md` prints nothing, no feature file names an internal integer or
    a deleted constant (`grep -rnE 'ExitCode|\b7[3-8]\b' specs/behaviours` prints nothing), and the unchanged scenarios
    pass as regressions in the unit and integration adapters (the item results above). `specs/architecture.md`: the
    shedding-cause bullet ("the internal shedding cause: storage (73) or other pressure (75), which callers see as
    `124`") still reads true, because `ShedCause` writes `73` and `75` to `sheddingExitCode`. The decode refusal of any
    other value falls under the next bullet's "shedding-state invariants ... fail closed". The shedding paragraph's
    "internal shed reason (storage or other pressure); both reach the caller as `124`" also holds; the C4 section names
    no constant. `docs/`: no page names `73` to `78` as an internal reason or any deleted constant. Status JSON's
    `profile.exitCode` is shown only as `0` in `json-schemas.md`, unchanged and still true. `README.md`: nothing.
    `CHANGELOG.md`: no entry, the `v0.8.5` one being Unit 7's. Updated: `tech-docs/001-domain-types.md` (as-built notes:
    only `policy.BareStop` is treated as `nil`; both legacy decoders are strict over every `uint8`; `callerShedReason`),
    and `tech-docs/004-file-impact.md` (Unit 3: `internal/cli/application.go`, the `loaded_gate.go` constant, the
    function's name, and the architecture row resolved), routing three Unit 3 learnings. Removed: none. Not run: none.
- [x] `[AI]` Dispatch `swe-reviewer` over the unit's diff; proof: findings recorded, none blocking open. `[AC-13]`
  - Result: (2026-10-06) no blocking finding; all 24 integer sites, the boundary classification, `callerShedReason`,
    finalize precedence, the ledger codecs (write `73`/`75`, accept `0`/`73`/`75`), and `profile.exitCode` verified
    equivalent to `origin/main`. F1 MEDIUM (session-release precedence untested) fixed: `promoteRelease` extracted and
    pinned by `TestPromoteReleaseDecidesWhatTheCallerSeesOfAFailedRelease`, two mutations failing it. F2 LOW: comment
    corrected. F3 LOW: the abandon and port-release defers use the shared helper. F4 LOW: one `policy.CarriesNoError`,
    counting only a top-level bare stop, tested with wrapped and joined stops. F5 LOW: a bare stop's `Error()` names its
    reason in words, tested to carry no digits.
- [x] `[AI]` Run the _Full gate_; proof: exit `0` with the coverage figure recorded. `[AC-13]` `[AC-14]` `[AC-15]`
      `[AC-16]`
  - Result: (2026-10-06) `npm test` passed the quick gate (99.33%, 893/899), the integration suite, and the end-to-end
    suite, then its race step timed out at Go's 10-minute package default in `tests/unit` and `tests/integration` at
    host load 34–47 (Spotlight indexing). The race step rerun with `-timeout 30m` and nothing else changed exited `0`
    (`tests/unit` 553 s, `tests/integration` 599 s), and `govulncheck` reported "No vulnerabilities found." The
    unmodified gate runs in CI on both platforms; see the learning on the race-step timeout.

### Unit 3 landing

- [x] `[AI]` Inspect the diff against data safety and commit thematically; proof: hooks pass, range recorded. `[AC-13]`
  - Result: (2026-10-06) data-safety scan of the diff found no candidate; thematic commits, every hook passed:
    `3d2b4f3`, `849ed20`, `699be68`.
- [x] `[AI]` Push review, push, screen title and body, open a draft pull request; proof: screen exit `0`, number
      recorded. `[AC-13]`
  - Result: push review clean, every `pre-push` gate passed; title and body screened (exit `0`); draft pull request
    #134.
- [x] `[AI]` Mark ready and wait for `Quality gate` on the head; proof: `success` on the recorded head. `[AC-13]`
  - Result: `Quality gate` `success` on head `699be68` (run 37425821386).
- [x] `[AI]` Post the leak review for that head; proof: `leak-review` reads `success`. `[AC-13]`
  - Result: `pass` review posted on `699be68`; `leak-review` reads `success`.
- [x] `[AI]` Rebase-merge when every precondition holds, then _Reconcile_; proof: merge commit and `0 0` recorded.
      `[AC-13]`
  - Result: merged as `a0e7819`; reconcile count `0 0`.

> **Pause Safety**: Unit 3 is on `main`. Safe to stop. To resume: the starting commands for Unit 4.

## Phase 4: Unit 4 — Profile Lineage

Branch `worktree/profile-lineage-rules`.

- [x] `[AI]` Create the branch with the starting commands; proof: `git branch --show-current` prints it. `[AC-17]`
  - Result: `worktree/profile-lineage-rules` from `origin/main` at `a0e7819`.
- [ ] `[AI]` Replace every `policy.BuiltinCatalog()` call in `tests/support` with a helper resolving the scenario's
      configuration through `config.Load` (`swe-developer`); run the unit adapter and
      `git grep -n 'BuiltinCatalog()' -- tests/support`; acceptance: the adapter exits `0` and the grep prints nothing.
      `[AC-17]` `[AC-18]` `[AC-19]`
- [ ] `[AI]` **RED** (`swe-developer`): replace "Minimal work still runs on a tiny machine" with the outline in
      [the specification changes](tech-docs/003-specification-changes.md), bind it, and update its exemption in
      `tests/contract/contract.go`; run the unit adapter; acceptance: the extends-minimal row fails with exit `125`
      naming `hippo.policy.replan-required`. `[AC-17]`
- [ ] `[AI]` **RED** (`swe-developer`): add "Automatic owner shares follow the profile lineage" to
      `specs/behaviours/reservations.feature`, bound with an empty share map, plus its exemption; run the unit adapter;
      acceptance: the extends-balanced row fails with one share instead of four. `[AC-18]`
- [ ] `[AI]` **RED** (`swe-developer`): convert "Stable warning spares a balanced ephemeral child admitted under normal
      pressure" to the outline, bind the configured row, and rename its exemption; run the unit adapter; acceptance: the
      steps bind and both rows pass, the configured row already carrying lineage through v0.8.4's attribute; the
      mutation below proves the row can fail. `[AC-19]`
- [ ] `[AI]` **RED** (`swe-developer`): add to `tests/unit/policy_test.go` tests of each lineage answer and of the floor
      for a catalog whose profile extends `minimal`, and to `tests/unit/reservation_test.go` the owner-share default per
      lineage with an empty share map; run `go test -count=1 ./tests/unit`; acceptance: compilation fails on
      `policy.Lineage`. `[AC-17]` `[AC-18]`
- [ ] `[AI]` **GREEN** (`swe-developer`): add `ProfileName` and `Lineage` to `internal/policy/profiles.go`, set lineage
      on the built-ins, key the floor on `LastResortFloor()` and `internal/guard/reservation.go`'s default on
      `DefaultOwnerShares()`; in this same item delete the Unit 4 entries for the owner-share `switch` in
      `internal/guard/reservation.go` from `tests/support/domain_literals_allowlist.go`, because the analysis fails on
      an entry whose violation is gone; run `go test -count=1 ./tests/unit` and the unit adapter; acceptance: both pass,
      including the three outlines. `[AC-17]` `[AC-18]` `[AC-19]` `[AC-08]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): delete `Profile.DegradedAdmission` for `Lineage.DegradedAdmission()`, build
      `reservationCoordination`'s shares from the built-ins' lineage, and type every profile name the file impact lists;
      in this same item delete the remaining Unit 4 entries from `tests/support/domain_literals_allowlist.go`; run
      `go test -count=1 ./internal/... ./tests/unit`, the unit adapter, and _Lint_; acceptance: all exit `0`. `[AC-17]`
      `[AC-18]` `[AC-08]`
- [ ] `[AI]` Mutation: make `LineageBalanced.DegradedAdmission()` return `false`, run the unit adapter, record the
      output, restore it; acceptance: both rows of the stable-warning outline fail, and pass once restored. `[AC-19]`
- [ ] `[AI]` Confirm Unit 4's entries are gone from `tests/support/domain_literals_allowlist.go`, each deleted in the
      item that removed its violation; run the unit adapter; acceptance: the allowlist holds no entry tagged Unit 4 and
      the adapter exits `0`. `[AC-08]`
- [ ] `[AI]` Synchronize `specs/architecture.md`'s policy-engine element with the lineage clause; proof:
      `./rhino md internal-link validate` exits `0`. `[AC-17]`

### Unit 4 close

- [ ] `[AI]` Run docs propagation: `docs/reference/resource-policy.md` (the floor follows the `minimal` lineage) and
      `docs/reference/configuration.md` (the lineage note covers the floor and default owner shares); proof: status
      recorded and `npm run format:check` exits `0`. `[AC-17]` `[AC-18]`
- [ ] `[AI]` Run the Gherkin implementation review over the three outlines; proof: statuses recorded. `[AC-17]`
      `[AC-18]` `[AC-19]`
- [ ] `[AI]` Dispatch `swe-reviewer` over the unit's diff; proof: findings recorded, none blocking open. `[AC-17]`
- [ ] `[AI]` Run the _Full gate_; proof: exit `0` with the coverage figure recorded. `[AC-17]` `[AC-18]` `[AC-19]`

### Unit 4 landing

- [ ] `[AI]` Inspect the diff against data safety and commit thematically; proof: hooks pass, range recorded. `[AC-17]`
- [ ] `[AI]` Push review, push, screen title and body, open a draft pull request; proof: screen exit `0`, number
      recorded. `[AC-17]`
- [ ] `[AI]` Mark ready and wait for `Quality gate` on the head; proof: `success` on the recorded head. `[AC-17]`
- [ ] `[AI]` Post the leak review for that head; proof: `leak-review` reads `success`. `[AC-17]`
- [ ] `[AI]` Rebase-merge when every precondition holds, then _Reconcile_; proof: merge commit and `0 0` recorded.
      `[AC-17]`

> **Pause Safety**: Unit 4 is on `main`. Safe to stop. To resume: the starting commands for Unit 5.

## Phase 5: Unit 5 — Admission Path and Decision

Branch `worktree/single-admission-decision`.

- [ ] `[AI]` Create the branch with the starting commands; proof: `git branch --show-current` prints it. `[AC-20]`
- [ ] `[AI]` **RED** (`swe-developer`): add `tests/unit/admission_decision_test.go` covering every path under both
      windows, `WindowUnset` refused, `SparesStableWarning`, and an input whose `Policy` differs from its resolution's
      `Policy`, where the input's `Policy` alone decides; run `go test -count=1 ./tests/unit`; acceptance: compilation
      fails on `policy.DecideAdmission`. `[AC-20]` `[AC-19]`
- [ ] `[AI]` **GREEN** (`swe-developer`): add `internal/policy/admission.go` as
      [the design](tech-docs/001-domain-types.md#admission-path-and-the-single-decision-unit-5) specifies; run the same
      command, then _Policy coverage_; acceptance: the tests pass, and the coverage tool exits `0` printing a figure at
      or above 99%. `[AC-20]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): share the eligibility rule between `DecideAdmission` and
      `SparesStableWarning`; run the same command and _Lint_; acceptance: both exit `0`. `[AC-20]`
- [ ] `[AI]` **RED** (`swe-developer`): add to `internal/cli/development_test.go` a table from each path to status's
      decision, `profile.exitCode`, and retryability, matching AC-14's rows; run `go test -count=1 ./internal/cli`;
      acceptance: compilation fails because `withAssessmentDecision` takes no path. `[AC-14]`
- [ ] `[AI]` **GREEN** (`swe-developer`): make `withAssessmentDecision` an exhaustive `switch` over the path, fed by
      `DecideAdmission` with `WindowSnapshot` and `resolution.Policy`, the policy `status` assesses with today; run the
      same command; acceptance: it passes. `[AC-14]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): route `run.go`'s sampling loop and supervision exemption through
      `DecideAdmission` and `SparesStableWarning`, passing `config.Policy`, the policy `run` admits against today and
      which callers set apart from `config.Resolution`, and deleting `admitted`; run `go test -count=1 ./internal/guard`
      and the unit and integration adapters; acceptance: all exit `0`, the admission and execution scenarios included.
      `[AC-20]` `[AC-19]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): make the driver's `assessAdmission` call `DecideAdmission` with
      `WindowSampling` and `resolution.Policy`; run the unit adapter and
      `git grep -nE "AdmissionReady\(" -- internal/guard internal/cli tests/support`; acceptance: the adapter exits `0`
      and the grep prints nothing. `[AC-20]`
- [ ] `[AI]` Add `policy.AdmissionInput` to `exhaustruct_v5`'s enforce patterns, plant a literal omitting `Policy` and
      `Window`, run _Lint_, record the output, and remove it; acceptance: the literal fails naming `exhaustruct`, and
      lint exits `0` once removed. `[AC-20]`
- [ ] `[AI]` Synchronize `specs/architecture.md`'s policy-engine element with the admission clause; proof:
      `./rhino md internal-link validate` exits `0`. `[AC-20]`

### Unit 5 close

- [ ] `[AI]` Apply rules propagation to the `exhaustruct_v5` scope change in the adapter's [Go analysis gates][go-gates]
      module; proof: status recorded. `[AC-20]`
- [ ] `[AI]` Run docs propagation; proof: status recorded. `[AC-14]`
- [ ] `[AI]` Run the Gherkin implementation review over the rebound admission scenarios; proof: statuses recorded.
      `[AC-20]`
- [ ] `[AI]` Dispatch `swe-reviewer` over the unit's diff; proof: findings recorded, none blocking open. `[AC-20]`
- [ ] `[AI]` Run the _Full gate_; proof: exit `0` with the coverage figure recorded. `[AC-14]` `[AC-20]`

### Unit 5 landing

- [ ] `[AI]` Inspect the diff against data safety and commit thematically; proof: hooks pass, range recorded. `[AC-20]`
- [ ] `[AI]` Push review, push, screen title and body, open a draft pull request; proof: screen exit `0`, number
      recorded. `[AC-20]`
- [ ] `[AI]` Mark ready and wait for `Quality gate` on the head; proof: `success` on the recorded head. `[AC-20]`
- [ ] `[AI]` Post the leak review for that head; proof: `leak-review` reads `success`. `[AC-20]`
- [ ] `[AI]` Rebase-merge when every precondition holds, then _Reconcile_; proof: merge commit and `0 0` recorded.
      `[AC-20]`

> **Pause Safety**: Unit 5 is on `main`. Safe to stop. To resume: the starting commands for Unit 6.

## Phase 6: Unit 6 — Strict Decoding and the End of the Ratchet

Branch `worktree/strict-enum-decoding`.

- [ ] `[AI]` Create the branch with the starting commands; proof: `git branch --show-current` prints it. `[AC-21]`
- [ ] `[AI]` **RED** (`swe-developer`): add "History lists a task class this version does not know as recorded" to
      `specs/behaviours/public-cli.feature`; run the unit adapter; acceptance: undefined, then bound and passing as the
      outcome scenario did, the mutation below proving it can fail. `[AC-22]`
- [ ] `[AI]` **RED** (`swe-developer`): add to `tests/unit/reservation_test.go` a ledger owner with class `batch`
      asserting the error comes from decoding, not validation; run `go test -count=1 ./tests/unit`; acceptance: it fails
      with the validation error `reservation ledger owner class is invalid`. `[AC-21]`
- [ ] `[AI]` **GREEN** (`swe-developer`): add `TaskClass.UnmarshalText` to `internal/policy/profiles.go`; run the same
      command and the unit adapter; acceptance: both pass, "Reservation ledger classes are validated before mutation"
      included. `[AC-21]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): derive the accepted set from one member list shared with
      `validReservationClass`; run the same command and _Lint_; acceptance: both exit `0`. `[AC-21]`
- [ ] `[AI]` **RED** (`swe-developer`): add to `internal/evidence/history_test.go` a `batch` class row listed as
      recorded, and to `tests/integration/lease_evidence_test.go` a lease record with class `batch` that stays an
      invalid session record; run `go test -count=1 ./internal/evidence ./tests/integration`; acceptance: compilation
      fails on `policy.RecordedTaskClass`. `[AC-22]`
- [ ] `[AI]` **GREEN** (`swe-developer`): add `RecordedTaskClass` and type `evidence.Summary.TaskClass`, `Query.Class`,
      `leaseOwner.Class`, and `EvidenceSummary.TaskClass`, renaming the CLI fields to `classFlag`; in this same item
      delete the Unit 6 entries, the last the allowlist holds, from `tests/support/domain_literals_allowlist.go`; run
      the same command and the unit adapter; acceptance: both pass. `[AC-22]` `[AC-08]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): share one parsing path between the strict and tolerant forms; run
      `go test -count=1 ./internal/... ./tests/unit`; acceptance: exit `0`. `[AC-22]`
- [ ] `[AI]` Mutation: make `RecordedTaskClass` refuse unknown text, run the unit adapter, record the output, restore
      it; acceptance: the unknown-class history scenario fails, and passes once restored. `[AC-22]`
- [ ] `[AI]` **RED** (`swe-developer`): add to `tests/unit/config_schema2_errors_test.go` a configuration whose
      coordination mode is `exclusive`, asserting the error comes from decoding, and a configuration with `"mode": ""`,
      asserting it still loads with the default mode, as today's decoder accepts it; run
      `go test -count=1 ./tests/unit`; acceptance: the `exclusive` case fails, because today's refusal comes after
      decoding, and the empty-mode case passes before and after. `[AC-23]`
- [ ] `[AI]` **GREEN** (`swe-developer`): close `coordination.mode` in `internal/config/config.go`; run the same
      command; acceptance: it passes, and `status --config` on that file still exits `125` naming
      `hippo.config.unreadable`. `[AC-23]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): delete the post-decode mode comparison the type now makes redundant; run
      the same command and _Lint_; acceptance: both exit `0`. `[AC-23]`
- [ ] `[AI]` Add `policy.RecordedTaskClass` to `exhaustruct_v5`'s enforce patterns and run _Lint_; proof: exit `0`.
      `[AC-22]`
- [ ] `[AI]` Confirm the allowlist is empty: `tests/support/domain_literals_allowlist.go` still declares
      `domainLiteralAllowlist` but holds no entry, every entry having been deleted in the item that removed its
      violation; run the unit adapter; acceptance: exit `0`. `[AC-08]`
- [ ] `[AI]` **RED** (`swe-developer`): change the analysis scenario's `Then` to "Then it reports no finding" and rebind
      its step in `tests/support/steps.go`; run the unit adapter, then
      `git grep -n domainLiteralAllowlist -- tests internal`; acceptance: the adapter exits `0`, because the allowlist
      is already empty, and the grep prints the declaration and the code that reads it, which is the failing state of
      AC-08. `[AC-08]`
- [ ] `[AI]` **GREEN** (`swe-developer`): delete `tests/support/domain_literals_allowlist.go` and the code that reads
      it; run the unit and integration adapters; acceptance: both exit `0`. `[AC-08]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): remove the stale-entry fixture from
      `tests/support/domain_literals_internal_test.go`; run _Analysis fixtures_ and
      `git grep -n domainLiteralAllowlist -- tests internal`; acceptance: the fixtures pass and the grep prints nothing.
      `[AC-08]`

### Unit 6 close

- [ ] `[AI]` Apply rules propagation to the closed ratchet and the final `exhaustruct_v5` scope in the adapter's [Go
      analysis gates][go-gates] module; proof: status recorded. `[AC-08]`
- [ ] `[AI]` Run docs propagation: `docs/reference/json-schemas.md` says history lists an unknown class as recorded;
      proof: status recorded. `[AC-22]`
- [ ] `[AI]` Run the Gherkin implementation review over the new and changed scenarios; proof: statuses recorded.
      `[AC-08]` `[AC-22]`
- [ ] `[AI]` Dispatch `swe-reviewer` over the unit's diff; proof: findings recorded, none blocking open. `[AC-21]`
- [ ] `[AI]` Run the _Full gate_; proof: exit `0` with the coverage figure recorded. `[AC-08]` `[AC-21]` `[AC-22]`
      `[AC-23]`

### Unit 6 landing

- [ ] `[AI]` Inspect the diff against data safety and commit thematically; proof: hooks pass, range recorded. `[AC-21]`
- [ ] `[AI]` Push review, push, screen title and body, open a draft pull request; proof: screen exit `0`, number
      recorded. `[AC-21]`
- [ ] `[AI]` Mark ready and wait for `Quality gate` on the head; proof: `success` on the recorded head. `[AC-21]`
- [ ] `[AI]` Post the leak review for that head; proof: `leak-review` reads `success`. `[AC-21]`
- [ ] `[AI]` Rebase-merge when every precondition holds, then _Reconcile_; proof: merge commit and `0 0` recorded.
      `[AC-21]`

> **Pause Safety**: every code unit is on `main`. Safe to stop. To resume: the starting commands for Unit 7.

## Phase 7: Unit 7 — Release v0.8.5

Branch `worktree/release-v0.8.5`, under [release cut](../../../repo-governance/workflows/maintenance/release-cut.md),
dispatched to `swe-releaser`. The owner authorized this release on 2026-10-06 (D2, D12).

- [ ] `[AI]` Create the branch with the starting commands; proof: `git branch --show-current` prints it. `[AC-24]`
- [ ] `[AI]` Add the `v0.8.5` entry to `CHANGELOG.md` — `Fixed`: the `minimal`-lineage floor, naming the configured
      profiles whose exit `125` becomes an admission — and name `v0.8.5` as the current release in the five pages the
      file impact lists; proof: `git grep -n 'v0\.8\.4' -- ':!plans' ':!CHANGELOG.md'` prints nothing. `[AC-24]`
- [ ] `[AI]` Run docs propagation and the _Full gate_; proof: status recorded and exit `0`. `[AC-24]`
- [ ] `[AI]` Run the [docs quality gate](../../../repo-governance/workflows/quality/docs-quality-gate.md) on subject
      `all` on this branch's head, before landing, so any repair it makes is committed here and reaches the commit being
      tagged (PQG-14); proof: its verdict line recorded here and `git status --porcelain` printing nothing after the
      repairs are committed. `[AC-24]`
- [ ] `[AI]` Land it: data-safety inspection and commit, push review and push, screened draft pull request, ready,
      `Quality gate` on the head, the leak review for that head, rebase merge, and _Reconcile_; proof: the merge commit
      and `0 0` recorded. `[AC-24]`
- [ ] `[AI]` Move this worktree onto the merge commit, because the rebase merge leaves it on `worktree/release-v0.8.5`
      at the pre-rebase commit and `scripts/build-release.sh` refuses unless its commit equals the checkout's `HEAD` and
      the checkout is clean, untracked files included: `git switch --detach <merge commit>`; proof: `git rev-parse HEAD`
      prints the merge commit recorded above and `git status --porcelain --untracked-files=all` prints nothing.
      `[AC-24]`
- [ ] `[AI]` Run `scripts/test.sh` in this worktree, now a clean detached checkout of the merge commit; proof: exit `0`
      and `git status --porcelain --untracked-files=all` still printing nothing. `[AC-24]`
- [ ] `[AI]` Screen the tag name and the notes
      `gh api repos/wahidyankf/hippo/releases/generate-notes -f tag_name=v0.8.5 --jq .body` returns with
      `scripts/public-safety/outbound-preflight.sh --surface release`; proof: exit `0`. `[AC-24]`
- [ ] `[AI]` In this detached worktree, run `./scripts/build-release.sh v0.8.5 <merge commit> <output-dir>` and
      `./tests/artifacts/release-assets.sh <output-dir> v0.8.5 <merge commit>`, with `<output-dir>` the ignored
      `local-tmp/release-v0.8.5` so the checkout stays clean; proof: both exit `0`. `[AC-24]`
- [ ] `[AI]` Create the annotated tag `v0.8.5` on the merge commit and push it; proof: `release.yml`'s run and the
      published release's `checksums.txt` recorded here. `[AC-24]`
- [ ] `[AI]` Run the commands in `docs/how-to/install-a-pinned-release.md` for this platform in an empty directory;
      proof: the checksum line reads `OK` and `version --json` names `v0.8.5`. `[AC-24]`

> **Pause Safety**: `v0.8.5` is published and never replaced. Consumer repins are coordinated outside this repository.

## Phase 8: Knowledge Capture and Execution Check

- [ ] `[AI]` Route every `learnings.md` entry to one durable owner or discard it with a reason; proof: no unresolved
      entry. `[AC-25]`
- [ ] `[AI]` Run the [execution check](../../../repo-governance/workflows/plan/plan-execution-check.md) against AC-01 to
      AC-25; proof: its verdict line recorded here, permitting archival. `[AC-25]`

> **Pause Safety**: the verdict is recorded. To resume: the Plan Archival section.

## Plan Archival

Branch `worktree/strict-go-linting-and-domain-modeling-record`, a docs-only pull request (D15).

- [ ] `[AI]` Create the branch with the starting commands; proof: `git branch --show-current` prints it. `[AC-25]`
- [ ] `[AI]` Move the plan with `git mv` to `plans/done/YYYY-MM-DD__strict-go-linting-and-domain-modeling/`, using the
      completion date, and update `plans/in-progress/README.md` and `plans/done/README.md`; proof: one copy exists and
      `git grep -n 'in-progress/strict-go-linting'` prints nothing outside `plans/done/`. `[AC-25]`
- [ ] `[AI]` Run `npm run test:quick`, `./rhino md internal-link validate`, and
      `./rhino governance directory-map validate` from the archived state; proof: all exit `0`. `[AC-25]`
- [ ] `[AI]` Land it through the same landing steps as each unit; proof: the merge commit and `0 0` recorded in the pull
      request body, since they post-date the archived copy. `[AC-25]`
- [ ] `[AI]` Run [dev artifact clean-up](../../../repo-governance/workflows/maintenance/dev-artifact-clean-up.md): the
      worktree, every unit branch locally and on `origin`, and the release output directory; proof:
      `git -C ../.. worktree list` and `git -C ../.. branch -a` show neither the worktree nor any branch in the
      Execution Checkout table, recorded in the pull request. `[AC-25]`

[go-gates]: ../../../repo-governance/development/quality/stacks/repository-adapter/001-go-analysis-gates.md
