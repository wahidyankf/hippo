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

- _Quick gate_: `npm run test:quick`. _Full gate_: `npm test`, or `GOFLAGS=-timeout=30m npm test` on a loaded host
  (Phase 4a). Both run directly: HIPPO cannot guard HIPPO, so never beneath `./hippo`, per
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
- [x] `[AI]` Replace every `policy.BuiltinCatalog()` call in `tests/support` with a helper resolving the scenario's
      configuration through `config.Load` (`swe-developer`); run the unit adapter and
      `git grep -n 'BuiltinCatalog()' -- tests/support`; acceptance: the adapter exits `0` and the grep prints nothing.
      `[AC-17]` `[AC-18]` `[AC-19]`
  - Result: (2026-10-06) `tests/support` no longer calls `policy.BuiltinCatalog()`: `Driver.resolveProfile` in
    `degraded_lineage.go` resolves the requested profile, or the default, through `config.Load`, which returns the
    built-in catalog when the scenario names no configuration and the scenario's own file when it does.
    `assessAdmission`, `assessPressure`, `assessRelease`, `balancedEphemeralChild`, and `childOutsideExemption` call it,
    and `resolveConfigured` is `resolveProfile` with no request. `git grep -n 'BuiltinCatalog()' -- tests/support`
    prints nothing (exit `1`), and `go test -count=1 -timeout 45m -run TestUnitBehaviours ./tests/unit` exits `0` in 199
    s.
- [x] `[AI]` **RED** (`swe-developer`): replace "Minimal work still runs on a tiny machine" with the outline in
      [the specification changes](tech-docs/003-specification-changes.md), bind it, and update its exemption in
      `tests/contract/contract.go`; run the unit adapter; acceptance: the extends-minimal row fails with exit `125`
      naming `hippo.policy.replan-required`. `[AC-17]`
  - Result: (2026-10-06) `specs/behaviours/admission.feature` replaces "Minimal work still runs on a tiny machine" with
    the Scenario Outline "The last-resort floor follows the minimal lineage", the plan's three rows and its
    `@e2e-exempt` tag, and `tests/contract/contract.go` carries the exemption under the new name with the same
    `hostEvidenceBoundary`. The steps bind in `tests/support/steps.go`: `tinyMachineUnder` writes the row's
    configuration (`derivedProfileConfiguration` for the minimal row, and `derivedProfileWithoutFallback`, which sets
    `"fallback":""`, for the constrained row), `requireConfiguredMinimal` holds the second row to `local-minimal` at
    concurrency one, and `requireNoUsableFallbackReplan` holds the third row to the replan and then runs the scenario's
    own configuration through the public command line for exit `125` naming `hippo.policy.replan-required`. Resolution
    goes through `config.Load`. RED observed with
    `go test -count=1 -v -run 'TestUnitBehaviours/The_last-resort' ./tests/unit`: the built-in and no-fallback rows pass
    and the extends-minimal row fails with "the configured profile was refused with exit 125 naming
    hippo.policy.replan-required: resource profile has no usable fallback"
    (`340 scenarios (2 passed, 1 failed, 337 undefined)`, the rest unselected).
- [x] `[AI]` **RED** (`swe-developer`): add "Automatic owner shares follow the profile lineage" to
      `specs/behaviours/reservations.feature`, bound with an empty share map, plus its exemption; run the unit adapter;
      acceptance: the extends-balanced row fails with one share instead of four. `[AC-18]`
  - Result: (2026-10-06) `specs/behaviours/reservations.feature` gains the `@e2e-exempt` Scenario Outline "Automatic
    owner shares follow the profile lineage" with the plan's four rows, after the scenario it extends, and
    `tests/contract/contract.go` exempts it with `reservationCapacityBoundary` (the binary always plans with the
    configuration's filled share map). `hostWithProfileOfNoOwnerShare` resolves the row's profile through `config.Load`
    (a derived configuration for the three configured rows), `planAutomaticReservation` calls `guard.PlanReservation`
    with `OwnerShares: map[string]int{}`, and `requireOwnerShares` recomputes the vector for the stated number of shares
    and, on a mismatch, names the number the capacity was divided into. RED observed with
    `go test -count=1 -v -run 'TestUnitBehaviours/Automatic_owner_shares' ./tests/unit`: the built-in and
    extends-minimal rows pass, and the extends-balanced row fails with
    `profile "local-balanced" divided capacity into 1 owner shares instead of 4` (the extends-constrained row fails the
    same way, `1 ... instead of 2`).
- [x] `[AI]` **RED** (`swe-developer`): convert "Stable warning spares a balanced ephemeral child admitted under normal
      pressure" to the outline, bind the configured row, and rename its exemption; run the unit adapter; acceptance: the
      steps bind and both rows pass, the configured row already carrying lineage through v0.8.4's attribute; the
      mutation below proves the row can fail. `[AC-19]`
  - Result: (2026-10-06) `specs/behaviours/execution.feature` converts "Stable warning spares a balanced ephemeral child
    admitted under normal pressure" into the `@e2e-exempt` Scenario Outline "Stable warning spares an ephemeral child of
    the balanced lineage" with the plan's two rows, and `tests/contract/contract.go` renames the exemption, keeping
    `processControlBoundary`. The built-in row's text was the replaced scenario's, so "Unsafe pressure still sheds a
    balanced ephemeral child admitted under normal pressure" and "Stable warning still sheds work outside the exemption"
    keep their own steps. `balancedLineageChild` resolves the row's profile through `config.Load` (a derived
    configuration for the configured row, and it fails if the configuration resolved some other profile), and
    `holdStableWarning` binds both the old "class grace" and the new "ephemeral grace" wording.
    `go test -count=1 -v -run 'TestUnitBehaviours/Stable_warning_spares' ./tests/unit` exits `0` with both rows passing
    (`6 steps (6 passed)`), the configured row carrying lineage through v0.8.4's `Profile.DegradedAdmission`; the
    mutation item below proves the rows can fail.
- [x] `[AI]` **RED** (`swe-developer`): add to `tests/unit/policy_test.go` tests of each lineage answer and of the floor
      for a catalog whose profile extends `minimal`, and to `tests/unit/reservation_test.go` the owner-share default per
      lineage with an empty share map; run `go test -count=1 ./tests/unit`; acceptance: compilation fails on
      `policy.Lineage`. `[AC-17]` `[AC-18]`
  - Result: (2026-10-06) `tests/unit/policy_test.go` gains `TestEachLineageAnswersItsThreeQuestions`
    (`DegradedAdmission`, `LastResortFloor`, and `DefaultOwnerShares` for balanced `true`/`false`/`4`, constrained
    `false`/`false`/`2`, minimal `false`/`true`/`1`, and an unset lineage `false`/`false`/`1`),
    `TestBuiltinProfilesCarryTheirLineage`, `TestTheLastResortFloorFollowsTheMinimalLineage` (a catalog whose profiles
    extend `minimal`, directly and through a child, resolve to themselves on a tiny machine at concurrency one with the
    relaxed memory and disk thresholds and `LineageMinimal` on the resolution; strict work on such a profile still
    replans; a constrained-lineage profile with no fallback still has none; a balanced request still ends on built-in
    `minimal`), and `TestResolveRefusesAProfileWithNoLineage`. `tests/unit/reservation_test.go` gains
    `TestAutomaticOwnerSharesDefaultByLineage` (an empty share map, a name the old `switch` never knew and `balanced`
    itself, each lineage to its `4`, `2`, `1`, `1`) and `TestAConfiguredOwnerShareOutranksTheLineageDefault`. RED
    observed: `go test -count=1 ./tests/unit` fails `[build failed]` with `undefined: policy.Lineage` (and
    `LineageBalanced`, `LineageConstrained`, `LineageMinimal`, `LineageUnset`, `policy.ProfileName`).
- [x] `[AI]` **GREEN** (`swe-developer`): add `ProfileName` and `Lineage` to `internal/policy/profiles.go`, set lineage
      on the built-ins, key the floor on `LastResortFloor()` and `internal/guard/reservation.go`'s default on
      `DefaultOwnerShares()`; in this same item delete the Unit 4 entries for the owner-share `switch` in
      `internal/guard/reservation.go` from `tests/support/domain_literals_allowlist.go`, because the analysis fails on
      an entry whose violation is gone; run `go test -count=1 ./tests/unit` and the unit adapter; acceptance: both pass,
      including the three outlines. `[AC-17]` `[AC-18]` `[AC-19]` `[AC-08]`
  - Result: (2026-10-06) `internal/policy/profiles.go` adds `ProfileName` (a defined string, the same JSON) and
    `Lineage` (`LineageUnset`, `LineageBalanced`, `LineageConstrained`, `LineageMinimal`) with three exhaustive `switch`
    answers: `DegradedAdmission()` (balanced), `LastResortFloor()` (minimal), and `DefaultOwnerShares()` (`4`, `2`, `1`,
    and `1` for an unset lineage, the share v0.8.4 gave a name it did not know). `Profile.Lineage` (JSON `-`) replaces
    `Profile.DegradedAdmission`, the built-ins carry theirs, `Resolution.Lineage` (JSON `-`) is copied by `Resolve`,
    which sets `Resolution.DegradedAdmission` from `Lineage.DegradedAdmission()`, takes the floor from
    `profile.Lineage.LastResortFloor()` instead of `current == profileMinimal`, and refuses a profile whose lineage is
    unset. `internal/guard/reservation.go`'s default is `resolution.Lineage.DefaultOwnerShares()`, which removes the
    owner-share `switch`. Behaviour change (D5): a configured profile that extends `minimal` and does not fit now
    resolves to itself on the relaxed floor where it was refused with "resource profile has no usable fallback" (exit
    `125` naming `hippo.policy.replan-required`); configuration keys and JSON are unchanged. Deviation: the typed
    `Resolution` fields the RED tests read forced every profile name the file impact lists to be typed in this item, so
    `ProfileName` already types `Catalog`, `Profile`, `Resolution`, `EvidenceSummary`, `ReservationOwner`,
    `reservationWaiter`, `ReservationEntry`, `ReservationPolicy.OwnerShares`, `Coordination`, the configuration file's
    names, the two `AcquireReservation` parameters, and the monitor line, `configOptions.requestedProfile` became
    `requestedProfileFlag` with a `requestedProfile()` reader, and all 14 Unit 4 entries went from
    `tests/support/domain_literals_allowlist.go` here (the analysis reported each as stale, and the owner-share entries
    are the two `PlanReservation` ones), so the REFACTOR item below has no entry or constant left to delete.
    `go test -count=1 ./tests/unit` exits `0` in 203 s (the unit adapter included), and
    `-run 'TestUnitBehaviours/(The_last-resort|Automatic_owner_shares|Stable_warning_spares|Production_code_compares)'`
    passes all ten rows: `345 scenarios (10 passed, 335 undefined)` with the rest unselected. Extra test beyond the
    plan: `TestConfiguredProfilesInheritTheirLineageAndOwnerShares` (lineage and share inheritance through extends, and
    a `minimal` that extends `balanced` refused at load by the fallback cycle).
- [x] `[AI]` **REFACTOR** (`swe-developer`): delete `Profile.DegradedAdmission` for `Lineage.DegradedAdmission()`, build
      `reservationCoordination`'s shares from the built-ins' lineage, and type every profile name the file impact lists;
      in this same item delete the remaining Unit 4 entries from `tests/support/domain_literals_allowlist.go`; run
      `go test -count=1 ./internal/... ./tests/unit`, the unit adapter, and _Lint_; acceptance: all exit `0`. `[AC-17]`
      `[AC-18]` `[AC-08]`
  - Result: (2026-10-06) The REFACTOR's three edits landed in the GREEN above, because that item could not compile
    without them: `Profile.DegradedAdmission` is gone for `Lineage.DegradedAdmission()` (nothing reads the field;
    `git grep -n 'profile.DegradedAdmission' -- '*.go'` prints nothing), `reservationCoordination` builds its share
    table from each built-in profile's `Lineage.DefaultOwnerShares()` (`internal/config/config.go`), and every profile
    name the file impact lists is a `policy.ProfileName`, with the only text conversions at the process boundary
    (`HIPPO_PROFILE` in `internal/guard/run.go`, the monitor's state line, and `--profile` read by
    `configOptions.requestedProfile()`). No quoted `"balanced"`, `"constrained"`, or `"minimal"` is left in production
    code outside the three `ProfileName` constants, and no Unit 4 allowlist entry or constant is left to delete.
    Acceptance after a review of the diff found nothing further to extract: `go test -count=1 ./internal/...` exits `0`
    (10 packages), `go test -count=1 ./tests/unit` exits `0` in 203 s with the unit adapter inside it (the run of the
    GREEN above, with no Go change since), `go tool golangci-lint run` exits `0` with `0 issues` (it first reported a
    `gosec` G703 taint on the scenario configuration read back from disk, which `Driver.configDocument` now holds
    instead, and an unused `nolint:cyclop` on the floor test, removed), and NilAway exits `0`.
- [x] `[AI]` Mutation: make `LineageBalanced.DegradedAdmission()` return `false`, run the unit adapter, record the
      output, restore it; acceptance: both rows of the stable-warning outline fail, and pass once restored. `[AC-19]`
  - Result: (2026-10-06) Mutation: `LineageBalanced.DegradedAdmission()` in `internal/policy/profiles.go` returned
    `false` instead of `true`, and `go test -count=1 -v ./tests/unit` with
    `-run 'TestUnitBehaviours/(Stable_warning_spares|A_configured_profile_derived|Stable_macOS_warning_admits)'` failed
    `345 scenarios (4 failed, 341 undefined)`: both rows of "Stable warning spares an ephemeral child of the balanced
    lineage" fail (`exit=0 completed=false stderr="HIPPO shedding ephemeral child after memory-warning."`, the child
    shed instead of spared), and so do "Stable macOS warning admits degraded work"
    (`got admitted=false ... DegradedAdmission:false Lineage:1`) and "A configured profile derived from balanced admits
    degraded work" (`profile "local-balanced": exit=0 stderr="HIPPO deferred task: safe admission was not reached."`);
    `TestEachLineageAnswersItsThreeQuestions` fails too (`lineage 1 DegradedAdmission() = false, want true`). Restored
    byte for byte (`cmp` against the copy taken before the edit), and the same selection passes:
    `345 scenarios (4 passed, 341 undefined)`, `13 steps (13 passed)`.
- [x] `[AI]` Confirm Unit 4's entries are gone from `tests/support/domain_literals_allowlist.go`, each deleted in the
      item that removed its violation; run the unit adapter; acceptance: the allowlist holds no entry tagged Unit 4 and
      the adapter exits `0`. `[AC-08]`
  - Result: (2026-10-06) `git grep -n 'Unit 4\|removedByLineage' -- tests/support/domain_literals_allowlist.go` prints
    nothing (exit `1`): the allowlist holds only the six Unit 6 entries, and the `removedByLineage` constant and the
    `guardReservation` file constant went with the entries that named them. Each of the 14 entries was deleted in the
    GREEN item above, the item that removed its violation: the analysis reported all 14 as stale the moment the profile
    types landed (`domain literal analysis found 14 problems`, each `holds no finding`) and "Production code compares no
    domain value with a literal" passes once they are gone.
    `go test -count=1 -timeout 45m -run TestUnitBehaviours ./tests/unit` (the _Unit adapter_) exits `0` in 353 s, and
    `go test -count=1 -timeout 45m ./tests/integration` exits `0` in 367 s; both finished inside Go's 10-minute default,
    so the longer `-timeout` was only a precaution at host load 7 to 16.
- [x] `[AI]` Synchronize `specs/architecture.md`'s policy-engine element with the lineage clause; proof:
      `./rhino md internal-link validate` exits `0`. `[AC-17]`
  - Result: (2026-10-06) `specs/architecture.md`'s "Policy engine and profiles" element now reads "classify evidence,
    choose an adaptive development profile, key every profile rule on the built-in lineage a profile inherits through
    `extends`, and preserve strict transaction and release envelopes", which is the plan's text without Unit 5's
    admission clause; the ledger bullet's "internal shedding cause: storage (73) or other pressure (75)" stays true.
    `./rhino md internal-link validate` exits `0` (`checked 1278 links, no findings`) and
    `npx prettier --check specs/architecture.md` passes.

### Unit 4 close

- [x] `[AI]` Run docs propagation: `docs/reference/resource-policy.md` (the floor follows the `minimal` lineage) and
      `docs/reference/configuration.md` (the lineage note covers the floor and default owner shares); proof: status
      recorded and `npm run format:check` exits `0`. `[AC-17]` `[AC-18]`
  - Result: (2026-10-06) status `landed`. Updated: `docs/reference/resource-policy.md` (Derived thresholds: when no
    profile fits, ordinary work resolves to the first profile of the `minimal` lineage its fallback chain reaches, built
    in or configured, at relaxed memory and disk; a chain that reaches none, such as a `constrained`-lineage profile
    with no `fallback`, fails `125` naming `hippo.policy.replan-required`), `docs/reference/configuration.md` (the
    lineage note now covers the last-resort floor, and says a profile `automaticOwnerShares` does not name takes the
    share of the profile it extends, 4, 2, or 1 by lineage), and `tech-docs/004-file-impact.md` (Unit 4 gains the five
    files the file-impact learning names, now routed). Checked and unchanged: the built-in share tables in
    `resource-policy.md` and `docs/how-to/enable-reservation-coordination.md`, the `balanced` → `constrained` →
    `minimal` sentences there and in `README.md`, and every `hippo.policy.replan-required` page (`exit-codes.md`,
    `respond-to-exit-codes.md`, the concurrency pages, the two-repository tutorial). Each describes built-ins or another
    replan cause and still reads true. `specs/architecture.md` was synchronized in the item above. `CHANGELOG.md`: no
    entry, since the `Fixed` entry is Unit 7's. Removed: none. Not run: none; no changed page shows a command.
    `npm run format:check` exits `0`.
- [x] `[AI]` Run the Gherkin implementation review over the three outlines; proof: statuses recorded. `[AC-17]`
      `[AC-18]` `[AC-19]`
  - Result: (2026-10-06) all three `implemented`; each row passes on this rerun in `TestUnitBehaviours` and
    `TestIntegrationBehaviours` (9 rows, `--- PASS`), all `@e2e-exempt` with recorded exemptions. "The last-resort floor
    follows the minimal lineage" (`admission.feature`): implementation `Resolve` in `internal/policy/profiles.go`
    (`profile.Lineage.LastResortFloor()`); bound in `tests/support/driver.go` (`tinyMachineUnder`,
    `requireConfiguredMinimal`, `requireNoUsableFallbackReplan`, which also checks exit `125` at the command-line
    boundary); tests `TestTheLastResortFloorFollowsTheMinimalLineage` in `tests/unit/policy_test.go`. The
    extends-minimal row failed before the GREEN. "Automatic owner shares follow the profile lineage"
    (`reservations.feature`): implementation `PlanReservation` in `internal/guard/reservation.go`
    (`resolution.Lineage.DefaultOwnerShares()`); bound in `tests/support/degraded_lineage.go`
    (`hostWithProfileOfNoOwnerShare`, `planAutomaticReservation` with an empty share map, `requireOwnerShares`, which
    names the share count it found); tests `TestAutomaticOwnerSharesDefaultByLineage` in
    `tests/unit/reservation_test.go`. The extends-balanced and extends-constrained rows failed before the GREEN (1 share
    instead of 4 or 2). "Stable warning spares an ephemeral child of the balanced lineage" (`execution.feature`,
    built-in and configured rows): implementation `Lineage.DegradedAdmission()` copied into
    `Resolution.DegradedAdmission` and read at `internal/guard/run.go` lines 910 and 1080; bound in
    `tests/support/degraded_lineage.go` (`balancedLineageChild`, `holdStableWarning`); tests
    `TestEachLineageAnswersItsThreeQuestions`. The mutation item above failed both rows.
- [x] `[AI]` Dispatch `swe-reviewer` over the unit's diff; proof: findings recorded, none blocking open. `[AC-17]`
  - Result: (2026-10-06) no blocking finding. MEDIUM (reproduced): the floor ignored a profile's own `fallback`, so a
    configured `minimal`-lineage profile with a fallback was admitted at the relaxed floor where `v0.8.4` fell back to
    `minimal` at normal thresholds; fixed test-first — the floor applies only at the end of the chain
    (`LastResortFloor() && Fallback == "" && !Strict`), pinned by a unit test with the reviewer's configuration and
    outline row `#02`. LOW: the built-in-name overrides `minimal` extends `constrained` (no floor, `125`) and `balanced`
    extends `minimal` (floor) pinned by tests as deliberate D5 consequences; no consumer configuration on this
    workstation overrides a built-in name. LOW: the guard reads degraded admission from `Resolution.Lineage`, the
    boolean kept only as the published status field. LOW accepted: the guard-level owner-share fallback is unreachable
    from the binary, since configuration fills every share.
- [x] `[AI]` Run the _Full gate_; proof: exit `0` with the coverage figure recorded. `[AC-17]` `[AC-18]` `[AC-19]`
  - Result: (2026-10-06) not exit `0` in one pass; deviation recorded. Quick gate passed, coverage 99.35% (911/917). At
    load averages 34–47, `npm test`'s non-race integration step hit Go's 10-minute default; rerun with `-timeout 30m`:
    integration `ok` (578 s), e2e `ok` (52 s), `govulncheck` found no vulnerabilities. Race pass 1 failed only
    "Cancelled FIFO waiters use a fresh cleanup deadline", a pre-existing flake (failure rate equal on `892c462`; see
    learnings), which passed 5/5 alone under `-race`. Race pass 2 (`-timeout 30m`) passed it and failed only "Release
    versions use exact semantic syntax", whose build lost files from the Go build cache trimmed mid-run; that scenario
    passed under `-race` in both adapters on rerun. Every scenario passed under `-race` in at least one pass on this
    tree. The FIFO flake is now its own bug-fix plan, `fix-cancelled-waiter-cleanup-flake`.

### Unit 4 landing

- [x] `[AI]` Inspect the diff against data safety and commit thematically; proof: hooks pass, range recorded. `[AC-17]`
  - Result: (2026-10-06) data-safety scan of the diff found no candidate; thematic commits, every hook passed:
    `2f498bd`..`6d5faa6`, five commits.
- [x] `[AI]` Push review, push, screen title and body, open a draft pull request; proof: screen exit `0`, number
      recorded. `[AC-17]`
  - Result: push review clean, every `pre-push` gate passed; title and body screened (exit `0`); draft pull request
    #135.
- [x] `[AI]` Mark ready and wait for `Quality gate` on the head; proof: `success` on the recorded head. `[AC-17]`
  - Result: `Quality gate` `success` on head `6d5faa6` (run 37455536577).
- [x] `[AI]` Post the leak review for that head; proof: `leak-review` reads `success`. `[AC-17]`
  - Result: `pass` review posted on `6d5faa6`; `leak-review` reads `success`.
- [x] `[AI]` Rebase-merge when every precondition holds, then _Reconcile_; proof: merge commit and `0 0` recorded.
      `[AC-17]`
  - Result: merged as `c109c4d`; reconcile count `0 0`.

> **Pause Safety**: Unit 4 is on `main`. Safe to stop. To resume: the starting commands for Unit 5.

## Phase 4a: Discovered — Explicit Gate Timeouts (Withdrawn)

Added on 2026-10-06 as discovered work to put an explicit `-timeout` on every `go test` step of `scripts/test-quick.sh`
and `scripts/test.sh`, after the full gate hit Go's 10-minute package default under host load in Units 2, 3, and 4 (see
[learnings](learnings.md)). Withdrawn the same day before any change: `scripts/test-loaded.sh` already raises the
timeout through `GOFLAGS` (`-timeout=${HIPPO_LOAD_TEST_TIMEOUT:-60m}`) precisely so that `scripts/test.sh` stays exactly
as CI runs it, and an explicit timeout in the shared scripts would contradict that decision and delay CI's detection of
a hung package. On a loaded host the _Full gate_ runs as `GOFLAGS=-timeout=30m npm test`, which changes no assertion.

- [x] `[AI]` Decide the discovered work against the repository's existing loaded-gate mechanism; proof: the reason
      recorded. `[AC-24]`
  - Result: (2026-10-06) withdrawn, as above; the branch created for it, `worktree/gate-test-timeouts` at `c109c4d`, was
    renamed to Unit 5's `worktree/single-admission-decision` with no commit of its own work.

## Phase 5: Unit 5 — Admission Path and Decision

Branch `worktree/single-admission-decision`.

- [x] `[AI]` Create the branch with the starting commands; proof: `git branch --show-current` prints it. `[AC-20]`
  - Result: `worktree/single-admission-decision` from `origin/main` at `c109c4d` (renamed from the withdrawn Phase 4a
    branch).
- [x] `[AI]` **RED** (`swe-developer`): add `tests/unit/admission_decision_test.go` covering every path under both
      windows, `WindowUnset` refused, `SparesStableWarning`, and an input whose `Policy` differs from its resolution's
      `Policy`, where the input's `Policy` alone decides; run `go test -count=1 ./tests/unit`; acceptance: compilation
      fails on `policy.DecideAdmission`. `[AC-20]` `[AC-19]`
  - Result: (2026-10-06) `tests/unit/admission_decision_test.go` holds ten tests: every path under both windows (21
    rows: snapshot normal, one sample, warning, critical, no samples, blocked disk, and a stable warning that still
    waits; sampling normal, short of the consecutive samples, busy CPU, warning, critical, blocked disk, the stable
    warning of the balanced lineage as `AdmissionDegraded` and of the constrained, minimal, unset, service,
    transactional, release, and one-sample-short cases as `AdmissionWait`), a resolution at replan or cleanup deciding
    its own path under both windows and outranking a blocked disk, `WindowUnset` and an unknown window refused with
    `AdmissionUnset`, five rows where the input's `Policy` and a different `Resolution.Policy` disagree (strict versus
    lenient, both ways, under both windows, and a zero resolution policy), the same for the stable-warning floor through
    both `DecideAdmission` and `SparesStableWarning`, `SparesStableWarning`'s nine rows, and a table proving
    `AdmissionDegraded` is exactly "a stable warning that is spared" over 4 lineages, 4 classes, and 4 sample windows.
    Every `AdmissionInput` literal comes from one helper that names all five fields. RED observed with
    `go test -count=1 -timeout 30m ./tests/unit`: `unit [build failed]` with `undefined: policy.EvidenceWindow`,
    `policy.AdmissionInput`, and the rest; `-gcflags=-e` lists all 99 undefined references, 9 of them
    `policy.DecideAdmission` and 4 `policy.SparesStableWarning`.
- [x] `[AI]` **GREEN** (`swe-developer`): add `internal/policy/admission.go` as
      [the design](tech-docs/001-domain-types.md#admission-path-and-the-single-decision-unit-5) specifies; run the same
      command, then _Policy coverage_; acceptance: the tests pass, and the coverage tool exits `0` printing a figure at
      or above 99%. `[AC-20]`
  - Result: (2026-10-06) `internal/policy/admission.go` adds `AdmissionPath` (`AdmissionUnset` zero, then `Normal`,
    `Degraded`, `Wait`, `Cleanup`, `Replan`), `EvidenceWindow` (`WindowUnset` zero, `WindowSnapshot`, `WindowSampling`),
    `AdmissionInput` (the five fields), `DecideAdmission`, and `SparesStableWarning`, in the design's order: a
    resolution at `replan` or `cleanup` decides its own path, a storage-blocked assessment is `AdmissionCleanup`, a
    snapshot is `Normal` for a normal state and `Wait` otherwise, a sampling window is `Normal` when `AdmissionReady`,
    `Degraded` for an ephemeral task whose lineage allows it with `WarningAdmissionReady`, and `Wait` otherwise, and an
    unset or unknown window returns `AdmissionUnset` with an error. Both functions read the input's `Policy` alone. The
    eligibility condition is written out in both `DecideAdmission` and `SparesStableWarning` here, for the REFACTOR to
    share. `go test -count=1 -timeout 30m ./tests/unit` exits `0` (`ok ... 382.010s`, host load averages 33 to 45);
    under `-coverpkg=./internal/policy -coverprofile=coverage/unit.out` it passed 480 tests, and
    `go run ./tests/coverage --profile coverage/unit.out --directories internal/policy --minimum 99` printed
    `selected production line coverage: 100.00% (404/404 statements)` and exited `0`, every block of `admission.go`
    covered.
- [x] `[AI]` **REFACTOR** (`swe-developer`): share the eligibility rule between `DecideAdmission` and
      `SparesStableWarning`; run the same command and _Lint_; acceptance: both exit `0`. `[AC-20]`
  - Result: (2026-10-06) `DecideAdmission`'s degraded step now calls
    `SparesStableWarning(input.TaskClass, input.Resolution, input.Samples, input.Policy)`, so the eligibility condition
    (an ephemeral task, a lineage that allows degraded admission, and `WarningAdmissionReady`) is written once.
    `go test -count=1 -timeout 30m ./tests/unit` exits `0` (`ok ... 254.959s`) and `go tool golangci-lint run` exits `0`
    with `0 issues` (after `gofumpt -extra -w` fixed one wrapped row in the new test file). Mutation check: replacing
    the two reads of `input.Policy` in `DecideAdmission` by `input.Resolution.Policy` fails 6 of the 36 admission tests
    (`path = 3 (<nil>), want 1` and the reverse), so the input's `Policy` alone decides.
- [x] `[AI]` **RED** (`swe-developer`): add to `internal/cli/development_test.go` a table from each path to status's
      decision, `profile.exitCode`, and retryability, matching AC-14's rows; run `go test -count=1 ./internal/cli`;
      acceptance: compilation fails because `withAssessmentDecision` takes no path. `[AC-14]`
  - Result: (2026-10-06) `TestStatusDecisionFollowsTheAdmissionPath` holds six rows from a path to the decision, exit
    code, and retryability that the resolution publishes through its JSON codec: normal to `run`/0, wait and degraded to
    a retryable `wait`/75, a blocked disk and a resolution already at cleanup to `cleanup`/73, and a replan resolution
    to `replan`/78. `TestStatusRefusesAnUnsetAdmissionPathInsteadOfDefaulting` pins that an unset path is an error that
    leaves the resolution as it was. Both call `withAssessmentDecision(resolution, path)` and read its error. The status
    table `TestStatusJSONKeepsEachResolutionsV084DecisionAndExitCode` also gains AC-14's fourth row, a configured strict
    profile that does not fit, through `status --json --config`: it passes on the unchanged code
    (`ok ... internal/cli`), so it is a regression row, not part of the RED. RED observed with
    `go test -count=1 ./internal/cli`: `FAIL ... [build failed]` with
    `cannot use test.path (variable of uint8 type policy.AdmissionPath) as policy.Assessment value` in argument to
    `withAssessmentDecision`, and `assignment mismatch: 2 variables but withAssessmentDecision returns 1 value`.
- [x] `[AI]` **GREEN** (`swe-developer`): make `withAssessmentDecision` an exhaustive `switch` over the path, fed by
      `DecideAdmission` with `WindowSnapshot` and `resolution.Policy`, the policy `status` assesses with today; run the
      same command; acceptance: it passes. `[AC-14]`
  - Result: (2026-10-06) `withAssessmentDecision(resolution, path)` in `internal/cli/development.go` is one `switch`
    over all six paths with no `default`: normal and replan keep the resolution, wait and degraded set `wait`,
    `ReasonCapacityDeferred`, and retryable, cleanup sets `cleanup` and `ReasonStorageBlocked` (what a resolution
    already at cleanup says), and an unset path returns an error. `status` builds an `AdmissionInput` from its two
    samples, `TaskEphemeral`, `resolution.Policy`, and `WindowSnapshot`, and a refused decision is
    `hippo.supervision.failed`. The superseded assessment-fed test became
    `TestAResolutionPublishesTheIntegerV084CarriedForItsReason`, its decision half now held by the path table.
    `go test -count=1 ./internal/cli` exits `0` (`ok ... 4.624s`), the six path rows and the four status JSON rows (the
    strict row included) passing.
- [x] `[AI]` **REFACTOR** (`swe-developer`): route `run.go`'s sampling loop and supervision exemption through
      `DecideAdmission` and `SparesStableWarning`, passing `config.Policy`, the policy `run` admits against today and
      which callers set apart from `config.Resolution`, and deleting `admitted`; run `go test -count=1 ./internal/guard`
      and the unit and integration adapters; acceptance: all exit `0`, the admission and execution scenarios included.
      `[AC-20]` `[AC-19]`
  - Result: (2026-10-06) `Run`'s admission loop calls `policy.DecideAdmission` with `config.Resolution`,
    `config.TaskClass`, the samples, `config.Policy` (the policy `run` admits against, after the default it substitutes
    when unset), and `WindowSampling`, and switches over all six paths with no `default`: cleanup is the storage-blocked
    stop (same stderr line, outcome, and reason), normal breaks the labelled loop, degraded forces concurrency one
    without reservation and breaks, wait defers through `deferAtTheDeadline` once the deadline passes and otherwise
    sleeps one interval, replan stops with `ReasonReplanRequired` and outcome `admission-failed`, and an unset path is
    `hippo.supervision.failed`. The `admitted` boolean is deleted, and the supervision loop's stable-warning exemption
    calls `policy.SparesStableWarning` with `config.Policy`. New RED first:
    `TestARunWhoseResolutionAlreadyStopsNeverLaunches` (a resolution at replan or cleanup over healthy samples) failed
    on the old loop with `result code=1 error=payload must not start, want status 0 and a stop for reason 5` (and
    `... reason 1`), the old loop ignoring the resolution, and passes after. `go tool golangci-lint run` caught `status`
    growing to 127 lines (`funlen`, limit 120), so the status decision moved to `decideStatus`; lint then exits `0`.
    `go test -count=1 -timeout 30m ./internal/guard ./internal/cli` exits `0` (15.585 s, 4.918 s); the unit adapter
    (`-run TestUnitBehaviours ./tests/unit`) exits `0` in 216.970 s, and
    `HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd` exits `0`; the integration adapter
    (`-run TestIntegrationBehaviours ./tests/integration`) exits `0` in 214.619 s, and
    `HIPPO_BDD_ADAPTER=integration go test -count=1 ./tests/bdd` exits `0`. Host load averages 13 to 45. The first run
    of these adapters lost its log to another session's scratchpad cleanup and was rerun whole.
- [x] `[AI]` **REFACTOR** (`swe-developer`): make the driver's `assessAdmission` call `DecideAdmission` with
      `WindowSampling` and `resolution.Policy`; run the unit adapter and
      `git grep -nE "AdmissionReady\(" -- internal/guard internal/cli tests/support`; acceptance: the adapter exits `0`
      and the grep prints nothing. `[AC-20]`
  - Result: (2026-10-06) `Driver.assessAdmission` (`tests/support/driver.go`) resolves the profile through `config.Load`
    as before, then calls `policy.DecideAdmission` with the resolution, `driver.taskClass`, its samples,
    `resolution.Policy`, and `WindowSampling`, and keeps no rule of its own: a `switch` over all six paths admits for
    `AdmissionNormal`, admits at concurrency one for `AdmissionDegraded`, and does not admit for the other four. The
    seven admission scenarios whose `When` is "development admission is assessed" change binding only. Every
    `AdmissionInput` literal outside `internal/policy` names all five fields.
    `go test -count=1 -timeout 30m -run TestUnitBehaviours ./tests/unit` exits `0` in 258.494 s,
    `HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd` exits `0`, `go tool golangci-lint run` exits `0`, and
    `git grep -nE "AdmissionReady\(" -- internal/guard internal/cli tests/support` prints nothing (exit `1`).
- [x] `[AI]` Add `policy.AdmissionInput` to `exhaustruct_v5`'s enforce patterns, plant a literal omitting `Policy` and
      `Window`, run _Lint_, record the output, and remove it; acceptance: the literal fails naming `exhaustruct`, and
      lint exits `0` once removed. `[AC-20]`
  - Result: (2026-10-06) `.golangci.yml` gains `^github\.com/wahidyankf/hippo/internal/policy\.AdmissionInput$` beside
    the two `Recorded*` patterns (explicit mode stays on); lint with the pattern and every literal complete exits `0`
    (`0 issues`). Planting the literal in `decideStatus` with `Policy` and `Window` removed, lint exits `1` printing
    `internal/cli/development.go:101:48: policy.AdmissionInput is missing fields Policy, Window (exhaustruct_v5)` and
    `* exhaustruct_v5: 1`. With the file restored byte for byte (`cmp`), lint exits `0` with `0 issues`.
- [x] `[AI]` Synchronize `specs/architecture.md`'s policy-engine element with the admission clause; proof:
      `./rhino md internal-link validate` exits `0`. `[AC-20]`
  - Result: (2026-10-06) the **Policy engine and profiles** element reads "... key every profile rule on the built-in
    lineage a profile inherits through `extends`, decide the admission path once for `run`, `status`, and the behaviour
    driver, and preserve strict transaction and release envelopes", the clause the specification changes plan names.
    `specs/behaviours/admission.feature` changes bindings only, so it has no text change.
    `./rhino md internal-link validate` prints `checked 1280 links, no findings` and exits `0`. With every item above
    ticked, the finishing gates pass on the tree: `npm run test:quick` exits `0` (selected production line coverage
    99.36%, 928/934 statements; its first run failed one scenario, "Release builds use only exact committed source", on
    a toolchain fault recorded in [learnings](learnings.md), and passed alone and on the whole rerun),
    `go tool nilaway -include-pkgs=github.com/wahidyankf/hippo -pretty-print=false ./...` exits `0`,
    `go tool golangci-lint run` exits `0`, and the integration adapter
    (`-run TestIntegrationBehaviours ./tests/integration`, 237.292 s) and
    `HIPPO_BDD_ADAPTER=integration go test -count=1 ./tests/bdd` exit `0` after the driver change.

### Unit 5 close

- [x] `[AI]` Apply rules propagation to the `exhaustruct_v5` scope change in the adapter's [Go analysis gates][go-gates]
      module; proof: status recorded. `[AC-20]`
  - Result: (2026-10-06) verified no-op: the [Go analysis gates][go-gates] module states the rule ("one
    `enforce-patterns` regex per closed domain struct", explicit mode) without enumerating targets, so adding
    `^github\.com/wahidyankf/hippo/internal/policy\.AdmissionInput$` to `.golangci.yml` stays inside it;
    `gochecksumtype` keeps no annotated target, since `AdmissionPath` and `EvidenceWindow` are scalar enums. No higher
    or lower rule names the targets (`git grep -n "AdmissionInput\|RecordedOutcome" -- repo-governance AGENTS.md` prints
    nothing).
- [x] `[AI]` Run docs propagation; proof: status recorded. `[AC-14]`
  - Result: (2026-10-06) verified no-op for `docs/`: the unit changes no exit status, `hippo.*` code, message, or
    `status --json` value (D7), and no page names `AdmissionReady`, `withAssessmentDecision`, `DecideAdmission`, or
    `SparesStableWarning` (`git grep` over `docs/` and `README.md` prints nothing); `specs/architecture.md` line 116
    already carries the single admission decision from the sync item.
- [x] `[AI]` Run the Gherkin implementation review over the rebound admission scenarios; proof: statuses recorded.
      `[AC-20]`
  - Result: (2026-10-06) 24 frozen rows, every one passing in `TestUnitBehaviours` and `TestIntegrationBehaviours`
    before and after the breaks; each break was reverted byte for byte (`cmp`). Paths: `DecideAdmission` and
    `SparesStableWarning` (`internal/policy/admission.go`), reached through `assessAdmission`
    (`tests/support/driver.go`), `Run`'s sampling loop and supervision exemption (`internal/guard/run.go`), and
    `decideStatus` (`internal/cli/development.go`). Breaks: (a) never `AdmissionDegraded`; (b) lineage gate dropped in
    `DecideAdmission`, (b2) in `SparesStableWarning`; (c) status and run windows swapped, (c1) status alone; (d)
    `SparesStableWarning` false, (j) true; (e) no storage check, (f) no resolution switch; (g) sampling never normal;
    (h) `WarningAdmissionReady` dropped; (i) class gate dropped. **implemented** (`admission.feature` unless noted):
    `:5` Healthy consecutive samples (g: `work was not admitted`); `:11` Stable macOS warning admits degraded (a, d:
    `got admitted=false`); `:17` Growing pressure defers (h: `unsafe degraded work was admitted`); `:23` Strict work
    never degraded (i: same); `:29` derived from balanced (a, c, d:
    `HIPPO deferred task: safe admission was not reached`); `:36` outside balanced's lineage (b, b2:
    `admitting ephemeral child under stable macOS warning`); `execution.feature:124` Warning outlasts the grace (j:
    `got reason 0 with exit 0`); `:130` Worsening warning (h: `reason=0`; under a it does not fail but spins the
    one-hour admission window with a no-op `Sleep`, killed after 11 min, so c and d skipped it); `:136` both rows (d:
    `HIPPO shedding ephemeral child after memory-warning`); `:147` compressor, swap, and disk rows (h, j:
    `child finished instead of being shed`); `:160` both rows (b2 or i, and j: same). **untested** by any break,
    decision open: `admission.feature:43` and `:49` (4 rows), which assert `Resolve`'s profile; `:62` Exhausted storage
    and `:80` strict transaction (e, f, e+f all pass: the driver reads the resolution, not the path; only
    `tests/unit/admission_decision_test.go` and `TestARunWhoseResolutionAlreadyStopsNeverLaunches` fail);
    `execution.feature:118` Critical pressure and `:147`'s critical row, which shed without the exemption; and every
    status scenario (`public-cli.feature:9`–`:106`, `evidence.feature:39`): under c1 all 346 scenarios pass at both
    adapters, and only `internal/cli`'s `TestStatusJSONKeepsEachResolutionsV084DecisionAndExitCode` fails
    (`Decision:wait ExitCode:75`). None unimplemented or drifted. Decisions on the open rows (2026-10-06):
    `admission.feature:62` and `:80` now read the decided path (`Driver.admission`, set by `assessAdmission`;
    `requireStorageBlocked` wants `AdmissionCleanup` and `requireReplan` wants `AdmissionReplan`; the feature text is
    unchanged), so they are **implemented**: with the resolution switch removed `:80` fails (unit adapter,
    `admission path 3`), and with the storage check and the resolution switch both removed both fail at both adapters,
    while the storage check removed alone leaves both passing and the switch removed alone leaves `:62` passing (a disk
    below the floor reaches cleanup by either route, so only the joint break is observable there); without the new
    assertion every one of those breaks passes. `admission.feature:43` and `:49` test profile selection, not the
    admission decision, so they are outside this review's subject. `execution.feature:118` and the critical row of
    `:147` shed without the exemption, also outside it. The status scenarios are window-insensitive on fixture samples,
    and the status window and policy are pinned by the Go tests
    `TestStatusJSONKeepsEachResolutionsV084DecisionAndExitCode` and `TestStatusDecisionFollowsTheAdmissionPath` instead.
- [x] `[AI]` Dispatch `swe-reviewer` over the unit's diff; proof: findings recorded, none blocking open. `[AC-20]`
  - Result: (2026-10-06) no CRITICAL or HIGH finding, and equivalence of the three callers verified; no blocking finding
    remains. The MEDIUM, status's unpinned `policy` argument (`decideStatus`), is fixed test-first: a row in
    `TestStatusJSONKeepsEachResolutionsV084DecisionAndExitCode` with 10 GiB free disk (below `DefaultPolicy`'s 30 GiB
    reserve, above the resolved profile's) expects `run` with exit 0; it passes on the code and fails under the mutation
    `Policy: policy.DefaultPolicy()` (`Decision:cleanup ExitCode:73`), then the mutation was reverted. LOW: the unpinned
    stable-warning exemption policy in `Run` is fixed by `TestARunSparesAStableWarningByThePolicyItAdmitsAgainst`
    (`RunConfig.Policy` spares, `Resolution.Policy` would not), which fails alone under `config.Resolution.Policy`
    (`pressure shed after 2 samples`), then reverted. LOW: `Run` now checks `decisionError` before the switch, with the
    unset path in the same guard (a `(Normal, err)` is refused); it keeps no test of its own, because `DecideAdmission`
    is a pure function whose error `Run` cannot be made to receive with a path beside it without a seam added only for
    the test, and the guard adds no uncovered statement. LOW: the unreachable cleanup path prints the resolution's own
    reason (`HIPPO blocked task: storage blocked; ...`, not `normal`), RED first in
    `TestARunWhoseResolutionAlreadyStopsNeverLaunches`; the exit status, stop reason, and outcome are unchanged. LOW:
    the driver's refusal of a replan or cleanup resolution it used to admit degraded is recorded in
    [learnings](learnings.md). LOW accepted as designed, with no change: the window is validated after the replan,
    cleanup, and storage steps (learning (a)). The Gherkin review's two `untested` rows, `admission.feature:62` and
    `:80`, are fixed in the item above; its other open rows are decided there, and the spin of `execution.feature:130`
    under a break that never admits is recorded in learnings.
- [x] `[AI]` Run the _Full gate_; proof: exit `0` with the coverage figure recorded. `[AC-14]` `[AC-20]`
  - Result: (2026-10-06) `GOFLAGS=-timeout=30m npm test` exit `0` at host load 16–32: selected production line coverage
    99.36% (928/934), race detector clean, `govulncheck` "No vulnerabilities found."

### Unit 5 landing

- [x] `[AI]` Inspect the diff against data safety and commit thematically; proof: hooks pass, range recorded. `[AC-20]`
  - Result: (2026-10-06) data-safety scan of the diff found no candidate; thematic commits, every hook passed:
    `6adb665`..`c5bc6b6` (refactor, test, docs(plans)); a line-length push refusal was fixed by amending the record
    commit, and the branch was rebased twice over plans-only commits.
- [x] `[AI]` Push review, push, screen title and body, open a draft pull request; proof: screen exit `0`, number
      recorded. `[AC-20]`
  - Result: push review clean, every `pre-push` gate passed; title and body screened (exit `0`); draft pull request
    #140.
- [x] `[AI]` Mark ready and wait for `Quality gate` on the head; proof: `success` on the recorded head. `[AC-20]`
  - Result: `Quality gate` `success` on head `c5bc6b6` (run 37491455117).
- [x] `[AI]` Post the leak review for that head; proof: `leak-review` reads `success`. `[AC-20]`
  - Result: `pass` review posted on `c5bc6b6`; `leak-review` reads `success`.
- [x] `[AI]` Rebase-merge when every precondition holds, then _Reconcile_; proof: merge commit and `0 0` recorded.
      `[AC-20]`
  - Result: merged as `e261965`; reconcile count `0 0`.

> **Pause Safety**: Unit 5 is on `main`. Safe to stop. To resume: the starting commands for Unit 6.

## Phase 6: Unit 6 — Strict Decoding and the End of the Ratchet

Branch `worktree/strict-enum-decoding`.

- [x] `[AI]` Create the branch with the starting commands; proof: `git branch --show-current` prints it. `[AC-21]`
  - Result: `worktree/strict-enum-decoding` from `origin/main` at `e261965`.
- [x] `[AI]` **RED** (`swe-developer`): add "History lists a task class this version does not know as recorded" to
      `specs/behaviours/public-cli.feature`; run the unit adapter; acceptance: undefined, then bound and passing as the
      outcome scenario did, the mutation below proving it can fail. `[AC-22]`
  - Result: (2026-10-06) the scenario is added after the outcome one, with the specification-changes wording ("a current
    summary whose task class is batch", "that row's task class reads batch"). RED observed with
    `HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd`: exit `1`,
    `undefined behavior step "a current summary whose task class is batch"` and
    `undefined behavior step "it exits 0 and that row's task class reads batch"`. Bound in `tests/support/steps.go` and
    `tests/support/history_v05.go` (`summaryRecordingTaskClass` writes the summary as plain JSON through a helper shared
    with the outcome step, so the typed summary never has to hold `batch`; `requireHistoryRowTaskClass` reads the one
    listed row through a helper shared with the outcome check). The structural form exits `0`, and
    `go test -count=1 -run 'TestUnitBehaviours/History_lists_a_task_class' ./tests/unit` passes (1 of 347 scenarios
    selected, 3 steps), as does the same scenario at the compiled end-to-end adapter
    (`HIPPO_BIN=<built binary> go test -count=1 -run 'TestE2EBehaviours/History_lists_a_task_class...' ./tests/e2e`), on
    code that still holds the class as a string; the mutation item below proves it can fail.
- [x] `[AI]` **RED** (`swe-developer`): add to `tests/unit/reservation_test.go` a ledger owner with class `batch`
      asserting the error comes from decoding, not validation; run `go test -count=1 ./tests/unit`; acceptance: it fails
      with the validation error `reservation ledger owner class is invalid`. `[AC-21]`
  - Result: (2026-10-06) `TestReservationLedgerClassIsRefusedWhereItIsDecoded` holds four rows through
    `guard.ReservationStatus`: an owner and a waiter whose class is `batch`, which must fail with an error that contains
    `decode reservation ledger` and `"batch"` and not `class is invalid`, and an owner and a waiter whose class is
    `release`, a known member no ledger holds, which must still fail with `owner|waiter class is invalid` and not at
    decode. Every row also checks the ledger bytes stay unchanged. RED observed with
    `go test -count=1 -run TestReservationLedgerClassIsRefusedWhereItIsDecoded ./tests/unit` (the focused form of the
    plan's command; the whole package runs in the next item): the two `batch` rows fail with
    `error "reservation ledger owner class is invalid" does not contain "decode reservation ledger"` (and the waiter's
    twin), and the two `release` rows pass before and after.
- [x] `[AI]` **GREEN** (`swe-developer`): add `TaskClass.UnmarshalText` to `internal/policy/profiles.go`; run the same
      command and the unit adapter; acceptance: both pass, "Reservation ledger classes are validated before mutation"
      included. `[AC-21]`
  - Result: (2026-10-06) `TaskClass.UnmarshalText` accepts `ephemeral`, `service`, `transactional`, and `release` in one
    exhaustive `switch` and refuses any other text, the empty one included, with `unknown task class "<text>"`; the
    ledger's owner and waiter classes decode through it with no change to `reservation.go`. The four new rows pass
    (`go test -count=1 -run TestReservationLedgerClassIsRefusedWhereItIsDecoded ./tests/unit`: `ok`).
    `go test -count=1 -timeout 30m -coverpkg=./internal/policy ./tests/unit` exits `0` (`ok ... 246.279s`,
    `coverage: 100.0% of statements in ./internal/policy`), which runs the unit adapter's 347 scenarios, "Reservation
    ledger classes are validated before mutation" among them (also run alone, `PASS`), and
    `HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd` exits `0`. Host load averages 7 to 13.
- [x] `[AI]` **REFACTOR** (`swe-developer`): derive the accepted set from one member list shared with
      `validReservationClass`; run the same command and _Lint_; acceptance: both exit `0`. `[AC-21]`
  - Result: (2026-10-06) `policy.TaskClasses()` is the one member list (`ephemeral`, `service`, `transactional`,
    `release`, the order the history filter lists them); `TaskClass.UnmarshalText` accepts exactly its members, and
    `validReservationClass` is "any class but release" over the same list. A defined string has no `iota` to derive a
    list from, so the new `tests/unit/task_class_test.go` holds the list to the constants (it parses
    `internal/policy/profiles.go` and compares every `TaskClass` constant with `TaskClasses()`) and pins the strict
    decode over seven refused texts; planting `TaskBatch TaskClass = "batch"` failed it with
    `TaskClasses lists [ephemeral service transactional release]` and
    `internal/policy declares [ephemeral service transactional release batch]`, then the file was restored byte for byte
    (`cmp`). `go test -count=1 -timeout 30m -coverpkg=./internal/policy ./tests/unit` exits `0` (`ok ... 251.345s`,
    `coverage: 100.0% of statements in ./internal/policy`) and `go tool golangci-lint run` exits `0` with `0 issues`.
- [x] `[AI]` **RED** (`swe-developer`): add to `internal/evidence/history_test.go` a `batch` class row listed as
      recorded, and to `tests/integration/lease_evidence_test.go` a lease record with class `batch` that stays an
      invalid session record; run `go test -count=1 ./internal/evidence ./tests/integration`; acceptance: compilation
      fails on `policy.RecordedTaskClass`. `[AC-22]`
  - Result: (2026-10-06) `internal/evidence/history_test.go` gains
    `TestHistoryListsAClassThisVersionHasNoMemberForAsRecorded` (three plain-JSON summaries with the classes `batch`,
    `ephemeral`, and none: all three listed, `batch` reading as the recorded text with no member and marshalling back as
    `"taskClass":"batch"`, an unrecorded class omitted, and the class filter selecting only a known class) and an
    aggregation test that keeps `batch` apart from `ephemeral` as recorded. `tests/integration/lease_evidence_test.go`
    gains `TestExclusiveStatusKeepsRefusingASessionRecordWhoseClassHasNoMember` (a service session record rewritten with
    class `batch`, empty, or absent stays `exclusive compatibility session record is invalid`, bytes unchanged) and
    `TestHeavyLeaseDescriptionKeepsTheClassTheOwnerRecorded` (`(class batch)`, `(class unknown)` for an empty or absent
    class), and `internal/cli/history_test.go` gains a table pinning the text, `--json`, `--jsonl`, and `--class`
    outputs for an unknown class; these three regression tests pass on the unchanged code, run alone, because only the
    evidence package can name the new type. RED observed with
    `go test -count=1 -timeout 30m ./internal/evidence ./tests/integration`: exit `1`,
    `internal/evidence/history_test.go:256:58: undefined: policy.RecordedTaskClass` and
    `batch.String undefined (type string has no field or method String)` (`FAIL ... internal/evidence [build failed]`),
    while `ok ... tests/integration 413.743s` (host load averages 11 to 22).
- [x] `[AI]` **GREEN** (`swe-developer`): add `RecordedTaskClass` and type `evidence.Summary.TaskClass`, `Query.Class`,
      `leaseOwner.Class`, and `EvidenceSummary.TaskClass`, renaming the CLI fields to `classFlag`; in this same item
      delete the Unit 6 entries, the last the allowlist holds, from `tests/support/domain_literals_allowlist.go`; run
      the same command and the unit adapter; acceptance: both pass. `[AC-22]` `[AC-08]`
  - Result: (2026-10-06) `policy.RecordedTaskClass` (`internal/policy/profiles.go`) holds the member the text names and
    the text: `RecordedClass` builds one (a value that is no member keeps its text), `TaskClass()` returns the member
    and whether there is one, `String`, `IsZero`, and `MarshalText` give the text back, and `UnmarshalText` never fails.
    `evidence.Summary.TaskClass` is a `RecordedTaskClass` (`omitzero`, so an unrecorded class stays omitted),
    `evidence.Query.Class` a `policy.TaskClass` (empty for no filter; a class with no member matches no filter),
    `leaseOwner.Class` a `RecordedTaskClass` (the lease message still reads `unknown` for an empty class, and
    `validSessionRecord` still refuses a class with no member), and `EvidenceSummary.TaskClass` a `policy.TaskClass`.
    The CLI fields are `historyOptions.classFlag` and `runOptions.classFlag`; `runClass` reads `run`'s flag once at the
    boundary into `runOptions.class` (an empty flag meaning `ephemeral`, as the guard's default always did, and
    `release` refused with the same message), and `readHistoryFilters` reads `--class` and `--outcome` in the order the
    checks always ran (source, class, resource tier, outcome). The six Unit 6 entries are deleted from
    `tests/support/domain_literals_allowlist.go` in this item, which now holds `[]domainAllowance{}`; the scenario
    "Production code compares no domain value with a literal" passes with the list empty
    (`go test -count=1 -run 'TestUnitBehaviours/Production_code_compares' ./tests/unit`: 1 passed, 2 steps).
    `go test -count=1 -timeout 30m ./internal/evidence ./tests/integration` exits `0` (`ok ... 0.193s`,
    `ok ... 389.142s`), `go test -count=1 -timeout 30m -coverpkg=./internal/policy ./tests/unit` exits `0`
    (`ok ... 314.472s`, `coverage: 100.0% of statements in ./internal/policy`), which runs all 347 unit scenarios, the
    new history one and "Reservation ledger classes are validated before mutation" included,
    `HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd` exits `0`, `go test -count=1 ./internal/...` exits `0`, and
    `go tool golangci-lint run` exits `0` with `0 issues` (after splitting the new evidence test, which `cyclop`,
    `exhaustive`, and `prealloc` flagged). Host load averages 7 to 22.
- [x] `[AI]` **REFACTOR** (`swe-developer`): share one parsing path between the strict and tolerant forms; run
      `go test -count=1 ./internal/... ./tests/unit`; acceptance: exit `0`. `[AC-22]`
  - Result: (2026-10-06) `policy.ParseTaskClass(text)` is the one place text becomes a class: `TaskClass.UnmarshalText`,
    `RecordedClass` (so the tolerant `RecordedTaskClass.UnmarshalText`), `runClass`, and the history `--class` filter
    (`classFilter`) all ask it. Its tests came first and failed to compile on `policy.ParseTaskClass`
    (`tests/unit/task_class_test.go:98:32: undefined: policy.ParseTaskClass`): over nine texts, members and not, the
    strict parse accepts exactly the texts the tolerant reader reads with a member, the tolerant reader never refuses
    and keeps every text, and the JSON round trip keeps the text and omits an empty one.
    `go test -count=1 -timeout 30m ./internal/... ./tests/unit` exits `0` (`ok ... internal/guard 16.270s`,
    `ok ... tests/unit 214.586s`) and `go tool golangci-lint run` exits `0` with `0 issues`.
- [x] `[AI]` Mutation: make `RecordedTaskClass` refuse unknown text, run the unit adapter, record the output, restore
      it; acceptance: the unknown-class history scenario fails, and passes once restored. `[AC-22]`
  - Result: (2026-10-06) with `RecordedTaskClass.UnmarshalText` returning `ParseTaskClass`'s refusal for any non-empty
    text that is no member, `go test -count=1 -timeout 30m -run TestUnitBehaviours ./tests/unit` exits `1`
    (`347 scenarios (345 passed, 2 failed)`, 354.862 s) with exactly two failures: "History lists a task class this
    version does not know as recorded" at `When JSON history is requested for thirty days`, with
    `hippo: [hippo.evidence.unreadable] reading run history: unknown task class "batch"`, and "Legacy schema-one
    PID-only ownership remains conservative" (heavy row), whose seeded legacy heavy lock records the class `heavy` and
    now fails with `exclusive compatibility heavy owner cannot be decoded`, so that scenario also pins the lease
    record's tolerant read. The file was restored byte for byte (`cmp` against the pre-mutation copy), and both
    scenarios pass again
    (`go test -count=1 -run 'TestUnitBehaviours/(History_lists_a_task_class...|Legacy_schema-one...)' ./tests/unit`: 3
    scenarios, 9 steps, `PASS`).
- [x] `[AI]` **RED** (`swe-developer`): add to `tests/unit/config_schema2_errors_test.go` a configuration whose
      coordination mode is `exclusive`, asserting the error comes from decoding, and a configuration with `"mode": ""`,
      asserting it still loads with the default mode, as today's decoder accepts it; run
      `go test -count=1 ./tests/unit`; acceptance: the `exclusive` case fails, because today's refusal comes after
      decoding, and the empty-mode case passes before and after. `[AC-23]`
  - Result: (2026-10-07) `TestCoordinationModeIsRefusedWhereTheDocumentIsDecoded` loads five documents naming the mode
    `exclusive`, `batch`, `Reservation`, ` reservation`, and `reservation `, each beside a profile that extends a
    missing one: the catalog build refuses that after decoding, so the error names the mode only when the mode is
    refused first, at decode. `TestAnEmptyOrAbsentCoordinationModeLoadsAsTheDefault` loads `"mode": ""`, `"mode": null`,
    no mode, no `coordination` object, and `"mode": "reservation"`, each as schema 2 with reservation coordination.
    `internal/cli/development_test.go` gains `TestStatusRefusesAnUnknownCoordinationModeAsAnUnreadableConfiguration`,
    which runs `status --json --config` over the adaptive fixture with each mode: three refused with exit `125` naming
    `hippo.config.unreadable` and the mode, and the empty and `reservation` modes loading with exit `0`. RED observed
    with `go test -count=1 -run 'TestCoordinationModeIsRefused|TestAnEmptyOrAbsentCoordinationMode' ./tests/unit` (the
    focused form of the plan's command): exit `1`, the five mode rows fail with
    `a document naming the coordination mode "exclusive" was refused as unknown profile "missing"`, want a refusal that
    names the mode (and the four others alike), and the empty-mode test passes; the CLI test passes before and after
    (`ok ... internal/cli`), which is the "still exits `125`" the next item keeps.
- [x] `[AI]` **GREEN** (`swe-developer`): close `coordination.mode` in `internal/config/config.go`; run the same
      command; acceptance: it passes, and `status --config` on that file still exits `125` naming
      `hippo.config.unreadable`. `[AC-23]`
  - Result: (2026-10-07) `internal/config/config.go` declares the unexported `coordinationMode` (`uint8`:
    `coordinationModeDefault`, the zero value, and `coordinationModeReservation`) with an `UnmarshalText` that accepts
    the empty string and `reservation` and refuses every other text with `unsupported coordination mode "<text>"`;
    `coordinationFile.Mode` has that type, and the reported mode keeps its string through one constant,
    `reservationModeName`. The comparison after decoding stays for the next item, rewritten for the type so the package
    compiles. The five RED rows pass (`go test -count=1 -run '...' ./tests/unit`: `ok`), and
    `go test -count=1 -timeout 30m ./tests/unit ./internal/cli ./internal/guard` exits `0` (`ok ... 202.497s`, `4.702s`,
    `15.672s`). Through a binary built from the tree, `status --json --config` over
    `{"schemaVersion":2,"coordination": {"mode":"exclusive"}}` exits `125` printing
    `hippo: [hippo.config.unreadable] resource configuration: unsupported coordination mode "exclusive"`, and over
    `"mode": ""` exits `0`.
- [x] `[AI]` **REFACTOR** (`swe-developer`): delete the post-decode mode comparison the type now makes redundant; run
      the same command and _Lint_; acceptance: both exit `0`. `[AC-23]`
  - Result: (2026-10-07) the comparison in `buildCoordination` (`configured.Mode != ...`, with its
    `unsupported schema %d coordination mode` error) is deleted: the type admits nothing it refused, so no value of
    `coordinationFile.Mode` reaches it that it would refuse. `go test -count=1 -timeout 30m ./tests/unit ./internal/cli`
    exits `0` (`ok ... 199.971s`, `4.488s`; the five decode rows, the default-mode rows, and the status rows included)
    and `go tool golangci-lint run` exits `0` with `0 issues`.
- [x] `[AI]` Add `policy.RecordedTaskClass` to `exhaustruct_v5`'s enforce patterns and run _Lint_; proof: exit `0`.
      `[AC-22]`
  - Result: (2026-10-07) `.golangci.yml` now lists `^github\.com/wahidyankf/hippo/internal/policy\.RecordedTaskClass$`
    after `AdmissionInput`, so the four patterns are `RecordedOutcome`, `RecordedBudgetOutcome`, `AdmissionInput`, and
    `RecordedTaskClass`. `go tool golangci-lint run` exits `0` with `0 issues`. To show the pattern bites, an incomplete
    literal (`RecordedTaskClass{class: member}`, the text dropped) planted in `internal/policy/profiles.go` exits `1`
    with `policy.RecordedTaskClass is missing field text (exhaustruct_v5)`; the file was restored byte-for-byte (`cmp`)
    and lint exits `0` again.
- [x] `[AI]` Confirm the allowlist is empty: `tests/support/domain_literals_allowlist.go` still declares
      `domainLiteralAllowlist` but holds no entry, every entry having been deleted in the item that removed its
      violation; run the unit adapter; acceptance: exit `0`. `[AC-08]`
  - Result: (2026-10-07) the file declares `var domainLiteralAllowlist = []domainAllowance{}`: the six Unit 6 entries
    were deleted in the items that removed their violations (the `removedByDecoding` group and its constant went with
    the last of them), so the 31 at introduction are all gone.
    `go test -count=1 -timeout 30m -run TestUnitBehaviours ./tests/unit` exits `0` (`ok ... 200.691s`), the analysis
    scenario passing with nothing on the allowlist.
- [x] `[AI]` **RED** (`swe-developer`): change the analysis scenario's `Then` to "Then it reports no finding" and rebind
      its step in `tests/support/steps.go`; run the unit adapter, then
      `git grep -n domainLiteralAllowlist -- tests internal`; acceptance: the adapter exits `0`, because the allowlist
      is already empty, and the grep prints the declaration and the code that reads it, which is the failing state of
      AC-08. `[AC-08]`
  - Result: (2026-10-07) `specs/behaviours/quality-gates.feature` reads `Then it reports no finding` under "Production
    code compares no domain value with a literal", and `tests/support/steps.go` binds `^it reports no finding$` to the
    new `requireNoDomainLiteralFinding`, which fails on any finding and names each.
    `go test -count=1 -timeout 30m -run TestUnitBehaviours ./tests/unit` exits `0` (`ok ... 219.678s`), as the plan
    expects: the allowlist is empty, so the scenario passes either way.
    `git grep -n domainLiteralAllowlist -- tests internal` prints `tests/support/domain_literals.go:597` (the reading
    code in `requireDomainLiteralRatchet`) and `tests/support/domain_literals_allowlist.go:3` and `:7` (the
    declaration), the failing state of AC-08.
- [x] `[AI]` **GREEN** (`swe-developer`): delete `tests/support/domain_literals_allowlist.go` and the code that reads
      it; run the unit and integration adapters; acceptance: both exit `0`. `[AC-08]`
  - Result: (2026-10-07) `tests/support/domain_literals_allowlist.go` is deleted, with `domainAllowance`,
    `domainAllowanceKey`, `reconcileDomainFindings`, and `requireDomainLiteralRatchet` from
    `tests/support/domain_literals.go`. `go test -count=1 -timeout 30m -run TestUnitBehaviours ./tests/unit` exits `0`
    (`ok ... 284.975s`), `-run TestIntegrationBehaviours ./tests/integration` exits `0` (`ok ... 350.537s`), and
    `HIPPO_BDD_ADAPTER=unit|integration go test ./tests/bdd` exits `0` for both (`0.323s`, `0.389s`). A first unit run
    with the file deleted on disk and still tracked failed 24 release scenarios with
    `copy fixture entry: lstat .../tests/support/domain_literals_allowlist.go: no such file or directory`, because the
    release fixtures copy `git ls-files --cached --others`, which lists a tracked file that is gone. The deletion is
    therefore staged in the index (`git rm --cached`, nothing committed), after which the adapter passes; the landing
    commit carries it. `tests/support`'s own test package does not compile until the next item, which removes the
    fixture that names the deleted types, as the plan orders.
- [x] `[AI]` **REFACTOR** (`swe-developer`): remove the stale-entry fixture from
      `tests/support/domain_literals_internal_test.go`; run _Analysis fixtures_ and
      `git grep -n domainLiteralAllowlist -- tests internal`; acceptance: the fixtures pass and the grep prints nothing.
      `[AC-08]`
  - Result: (2026-10-07) `TestDomainLiteralReportsFindingsOffTheAllowlistAndStaleEntries` is removed, and a smaller
    `TestDomainLiteralStepReportsEveryFindingAndNothingElse` takes its place, holding the new step to accepting no
    finding and refusing any with the count and each finding named. With the allowlist gone, the `Symbol` an entry was
    keyed on served nothing, so the `Symbol` field, `symbol()`, `functionSymbol`, the walker's node stack, and the
    `owner` and `symbol` parameters of `checkRawNames` and `add` are deleted as well (a deviation from the plan's
    wording, in the item's spirit). `go test -count=1 -run DomainLiteral ./tests/support` exits `0` (`ok ... 0.360s`),
    the whole `./tests/support` package exits `0` (`ok ... 1.578s`),
    `git grep -n domainLiteralAllowlist -- tests internal` prints nothing (exit `1`), neither does a grep for `ratchet`
    or `allowlist` in `tests`, `internal`, `cmd`, or `specs`, and `go tool golangci-lint run` exits `0` with `0 issues`.
    AC-07 (an allowlist entry whose violation is gone fails the analysis) was proven in Unit 1, by the stale-entry
    fixture's RED and GREEN items and again when the analysis reported Unit 4's 14 entries as stale the moment their
    violations went, and it retires with the mechanism: with no allowlist there is no entry to go stale, so removing the
    fixture leaves nothing for AC-07 to test, and AC-08's grep and scenario hold the closed state instead.

### Unit 6 close

- [x] `[AI]` Apply rules propagation to the closed ratchet and the final `exhaustruct_v5` scope in the adapter's [Go
      analysis gates][go-gates] module; proof: status recorded. `[AC-08]`
  - Result: (2026-10-07) status `landed`, two rows, both `resolved`. R1, the closed ratchet: the module's "ratchet" and
    "counting" bullets (31 entries by unit, per-symbol counting, "the last removal deletes the file") described a file
    that no longer exists, so they are replaced by one `findings` bullet (none are allowed, the scenario fails on any, a
    violation is fixed where it stands and never listed, and no waiver or exemption mechanism replaces the list); the
    module's `description` and `when_to_use` and the `repository-adapter/README.md` entry drop the "ratchet" and
    "allowlist" wording. The rule keeps one canonical home, with no copy elsewhere
    (`git grep -n -i "ratchet\|domain_literals_allowlist"` outside `plans/` and `CHANGELOG.md` prints only that new
    bullet). R2, the final `exhaustruct_v5` scope: verified no-op, as in Unit 5, since the module states the rule ("one
    `enforce-patterns` regex per closed domain struct", explicit mode) without enumerating targets and the final four
    patterns, `RecordedOutcome`, `RecordedBudgetOutcome`, `AdmissionInput`, and `RecordedTaskClass`, sit inside it.
    Disposition for both: covered, by the analysis scenario failing on any finding and by _Lint_ failing on an
    incomplete `RecordedTaskClass` literal (shown in the pattern item above). The module is 675 words (budget 750);
    `./rhino governance word-budget validate` and `directory-map validate` exit `0`.
- [x] `[AI]` Run docs propagation: `docs/reference/json-schemas.md` says history lists an unknown class as recorded;
      proof: status recorded. `[AC-22]`
  - Result: (2026-10-07) status `landed`. Updated: `docs/reference/json-schemas.md` (`history --json`: a row whose
    `taskClass` this version does not know is listed as recorded, and `--class` cannot select it, because the filter
    takes only the four classes `hippo history` names; the paragraph on an unknown outcome and the outcome list that
    `tests/unit/outcome_vocabulary_test.go` reads are unchanged), `docs/reference/cli.md` (the `history` filter note
    says an outcome or class a later version recorded is listed as recorded and selected by no `--outcome` or `--class`
    value), and `specs/architecture.md` (the `history` sentence lists an outcome or task class this version does not
    know as recorded, and an unknown outcome is never counted toward promotion). The class list in `cli.md`'s filter
    sentence and its `hippo run` usage-mistake sentence are unchanged: the strict decode changes no value either accepts
    (D7). Unchanged, checked: `README.md`, `docs/how-to/inspect-evidence-and-abandoned-groups.md`, and `CHANGELOG.md`
    (the `v0.8.5` entry is Unit 7's). Removed: none. Not run: none. `npm run format:check`,
    `./rhino md internal-link validate`, and `./rhino governance word-budget validate` exit `0`.
- [x] `[AI]` Run the Gherkin implementation review over the new and changed scenarios; proof: statuses recorded.
      `[AC-08]` `[AC-22]`
  - Result: (2026-10-07) frozen before the breaks: the two scenarios this unit adds or changes, `public-cli.feature:46`
    "History lists a task class this version does not know as recorded" (new) and `quality-gates.feature:56` "Production
    code compares no domain value with a literal" (its `Then` changed), and the six unchanged scenarios whose paths the
    decode changes reach: `reservations.feature:463` "Reservation ledger classes are validated before mutation", `:399`
    "Legacy schema-one PID-only ownership remains conservative" (2 rows), `public-cli.feature:182` "A flag value a
    command cannot accept is a usage mistake", `public-cli.feature:14` and `:19` the exclusive status scenarios (3
    rows), and `execution.feature:11` "Reservation coordination rejects every compatibility class as a protocol
    mismatch". Runs select the frozen rows with `-run` at `TestUnitBehaviours`, `TestIntegrationBehaviours` (nine rows
    each) and `TestE2EBehaviours` (five rows, a binary built from the tree); all three exit `0` before the breaks and
    after, and each break below was reverted byte for byte (`cmp`). **Breaks** (the first record numbered them B1 and B3
    to B7, with no B2 anywhere in the plan or its learnings, so they are renumbered here in order, and the release break
    the reviewer found missing is B7): (B1) `RecordedTaskClass.UnmarshalText` refuses an unknown non-empty class; (B2)
    `TaskClass.UnmarshalText` accepts any text; (B3) `validReservationClass` drops its member check; (B2+B3) both; (B4)
    `validSessionRecord` accepts any class; (B5a) `runClass` accepts any text but `release`; (B5b) `classFilter` takes
    any text; (B6a) a planted `class == "service"` in `internal/policy`; (B6b) a planted `Outcome string` field there;
    (B7) `runClass` drops its `release` refusal, run after the review (see its item below). **implemented**:
    `public-cli.feature:46` (`RecordedTaskClass.UnmarshalText` and `internal/policy/profiles.go`, read by `ReadHistory`
    in `internal/evidence/history.go` and printed by `internal/cli/history.go`; steps in `tests/support/steps.go` and
    `history_v05.go`; under B1 it fails at all three adapters with
    `hippo: [hippo.evidence.unreadable] reading run history: unknown task class "batch"`; Go tests
    `TestHistoryListsAClassThisVersionHasNoMemberForAsRecorded...` in `internal/evidence` and `internal/cli`);
    `quality-gates.feature:56` (`analyzeDomainLiterals` and the new `requireNoDomainLiteralFinding`; B6a fails it at the
    unit and integration adapters with `internal/policy/profiles.go:460: literal-comparison: class`, B6b with
    `raw-field: Outcome`; it is `@e2e-exempt`, so the e2e adapter passes under both; with no allowlist there is no entry
    that can excuse a finding); `reservations.feature:399` (the tolerant lease read: under B1 the `heavy` row fails at
    the unit and integration adapters, because the seeded lock records the class `heavy`, which has no member, and the
    `service` row passes, its class being one); `public-cli.feature:182` (B5a fails its `run --class` row and B5b its
    `history --class` row, the D7 regression of AC-12, and B5a fails at the unit and end-to-end adapters alike, so the
    scenario is not end-to-end only, as this record first said; B7, which removes the `release` refusal, passed every
    test here and at `HEAD` until the review added its `run --class release` row, below); and
    `reservations.feature:463`, which has a decision: B2 alone and B3 alone each leave it passing at both adapters,
    because decoding and validation both refuse a class with no member, and only B2+B3 fails it, at both adapters. Each
    layer is pinned by a Go test instead: B2 fails `TestReservationLedgerClassIsRefusedWhereItIsDecoded` (the `batch`
    rows, owner and waiter) and `TestTaskClassDecodesOnlyItsMembers`, while B3 first failed nothing. This record first
    explained that as decoding always supplying a member, which is wrong: an owner or waiter whose `class` key is absent
    or null never reaches `UnmarshalText`, so it arrives at validation as the empty class, and only the member half
    refuses it (exit `125`, `reservation ledger owner class is invalid`). The member half was **untested**, not
    unreachable, and the four absent-class and null-class rows (owner and waiter) the review added to
    `TestReservationLedgerClassIsRefusedWhereItIsDecoded` now fail under B3, the ledger being accepted. Decision: pin
    it, not the scenario, whose `Then` states the fail-closed contract and not which layer refuses;
    `TestAReservationHoldsExactlyTheClassesThatReserve` (`internal/guard/reservation_class_test.go`) holds the set to
    ephemeral, service, and transactional, and fails under B3 on every class with no member, the empty one included (it
    replaced the first version of this test when the review made the check an exhaustive switch). **Examined, not
    reached by any break**: `public-cli.feature:14` and `:19` (their documents are malformed or from the future, not
    classed) and `execution.feature:11` (the marker, not a class) pass under B1 to B6b, so they are outside this
    review's subject; the class half of session validity, which B4 removes, is pinned by
    `TestExclusiveStatusKeepsRefusingASessionRecordWhoseClassHasNoMember` (the `batch`, empty, and absent rows fail
    under B4). AC-23 has no scenario, as `tech-docs/003-specification-changes.md` records; its three Go tests and the
    RED above pin it. None unimplemented or drifted.
- [x] `[AI]` Dispatch `swe-reviewer` over the unit's diff; proof: findings recorded, none blocking open. `[AC-21]`
  - Result: (2026-10-07) no CRITICAL or HIGH finding; no blocking finding remains. The three MEDIUM and three LOW
    findings were each re-validated against the code and applied by `swe-developer` at the coordinator's direction,
    test-first, every mutation reverted byte for byte (`cmp`):
    - MEDIUM-1, `runClass`'s `release` refusal pinned by no test: confirmed. With the clause deleted,
      `hippo run --class release` exits `125` naming `hippo.supervision.failed` (the guard refuses the class in the same
      words) instead of exit `2` naming `hippo.args.invalid`, and every test passed, here and at `HEAD`. Fixed: the row
      `run --class release` joins `invalidFlagValues` (`tests/support/usage_v082.go`, with `taskClassRelease` in
      `driver.go`) and `TestRunClassAcceptsOnlyTheClassesRunMayGuard` (`internal/cli/development_test.go`) holds
      `runClass` to an empty flag meaning ephemeral, the three classes it guards accepted, and `release`, `batch`,
      `Ephemeral`, and ` service` refused as `hippo.args.invalid` with no class. Both pass on the code. Under the
      mutation (B7, the `|| class == policy.TaskRelease` clause deleted) the Go test fails
      (`runClass("release") = "release" (<nil>)`) and the scenario "A flag value a command cannot accept is a usage
      mistake" fails at the unit and end-to-end adapters
      (`"run --class release -- ..." was not refused as a usage mistake: exit=125`, `[hippo.supervision.failed]`);
      restored, all three pass. The Gherkin review's break list is renumbered (the first record skipped B2) and B7
      recorded there, and its "e2e only" remark for `public-cli.feature:182` is corrected: B5a fails the scenario at the
      unit adapter as well as the end-to-end one.
    - MEDIUM-2, the member half of `validReservationClass` is not unreachable through bytes: confirmed. An owner or
      waiter whose `class` key is absent or null never reaches `UnmarshalText`, arrives as the empty class, and only the
      member half refuses it (exit `125`, `reservation ledger owner class is invalid`). Fixed: the guard test's comment,
      the learnings bullet "two refusing layers", and the Gherkin Result for `reservations.feature:463` are corrected,
      and four rows (owner and waiter, class absent and null) join
      `TestReservationLedgerClassIsRefusedWhereItIsDecoded`, expecting `owner|waiter class is invalid`, no decode
      failure, and unchanged ledger bytes. They pass on the code and fail under B3 (the member half dropped):
      `a ledger holding a class no reservation may hold was accepted`, four rows.
    - MEDIUM-3, `validReservationClass` was "any member but release", fail-open for a future class: confirmed. RED: with
      `TaskBatch` planted in the constants and in `TaskClasses()`, the old code returned `true` for it and _Lint_
      reported nothing at that function (only the other switches and the test's map). Fixed: an exhaustive `switch` over
      `TaskClass` (ephemeral, service, and transactional true; release false; default false), so the `exhaustive` linter
      forces a decision for any new member. `TaskClasses()` stays the one list for the decoder and the history filter,
      as the REFACTOR item prescribed, so nothing conflicts there; only the ledger check stopped reading it, a deviation
      from that item's wording recorded in learnings, with `TaskClasses`'s doc comment corrected. The test became
      `TestAReservationHoldsExactlyTheClassesThatReserve` (an explicit set over every member, itself checked by
      `exhaustive`, and the no-member rows). Mutation: with the same plant, _Lint_ exits `1` with
      `reservation.go:626: missing cases in switch of type policy.TaskClass: policy.TaskBatch (exhaustive)` and the test
      reports the class undecided and refused; with the member check dropped (B3) the test fails on all five classes
      with no member. Restored, `0 issues`.
    - LOW-1, the diagnostic text of already-refused inputs changed: recorded in `learnings.md` (2026-10-07) with the
      before and after of each, measured by running a binary built from `HEAD` and one from the tree over the same
      documents, and marked as a candidate for the `v0.8.5` `CHANGELOG.md` wording in Unit 7. Exit statuses and
      `hippo.*` codes are unchanged.
    - LOW-2, the module's history-narrating sentence: replaced by a rule, "findings: none — the scenario fails on any; a
      violation is fixed where it stands, and no allowlist, waiver, or exemption exists", and "its empty findings" in
      the module's `description` and the adapter `README.md` entry now reads "its no-finding rule".
    - LOW-3, AC-07: the stale-entry fixture's item above now states that AC-07 was proven in Unit 1 and retires with the
      mechanism. The routing of Unit 6 learnings into `tech-docs/` is left for the final knowledge-capture item; each
      candidate, the new ones included, is listed in `learnings.md` with its routing line.
- [x] `[AI]` Run the _Full gate_; proof: exit `0` with the coverage figure recorded. `[AC-08]` `[AC-21]` `[AC-22]`
      `[AC-23]`
  - Result: (2026-10-07) `GOFLAGS=-timeout=30m npm test` exit `0` at host load 8–19, after the reviewer's findings were
    fixed: selected production line coverage 99.38% (955/961), race detector clean, `govulncheck` "No vulnerabilities
    found."

### Unit 6 landing

- [x] `[AI]` Inspect the diff against data safety and commit thematically; proof: hooks pass, range recorded. `[AC-21]`
  - Result: (2026-10-06) data-safety scan of the diff found no candidate; thematic commits, every hook passed:
    `81e123d`..`2c4e288` (refactor(config), refactor(policy), docs, docs(governance), docs(plans)); the first grouping
    put the staged allowlist deletion in the config commit, so the unpushed commits were rebuilt and the config commit
    verified to build and test alone; rebased once over the four bug-fix landings.
- [x] `[AI]` Push review, push, screen title and body, open a draft pull request; proof: screen exit `0`, number
      recorded. `[AC-21]`
  - Result: push review clean, every `pre-push` gate passed; title and body screened (exit `0`); draft pull request
    #145.
- [x] `[AI]` Mark ready and wait for `Quality gate` on the head; proof: `success` on the recorded head. `[AC-21]`
  - Result: `Quality gate` `success` on head `2c4e288` (run 37520536730).
- [x] `[AI]` Post the leak review for that head; proof: `leak-review` reads `success`. `[AC-21]`
  - Result: `pass` review posted on `2c4e288`; `leak-review` reads `success`.
- [x] `[AI]` Rebase-merge when every precondition holds, then _Reconcile_; proof: merge commit and `0 0` recorded.
      `[AC-21]`
  - Result: merged as `2555fc1`; reconcile count `0 0`.

> **Pause Safety**: every code unit is on `main`. Safe to stop. To resume: the starting commands for Unit 7.

## Phase 7: Unit 7 — Release v0.8.5

Branch `worktree/release-v0.8.5`, under [release cut](../../../repo-governance/workflows/maintenance/release-cut.md),
dispatched to `swe-releaser`. The owner authorized this release on 2026-10-06 (D2, D12).

- [x] `[AI]` Create the branch with the starting commands; proof: `git branch --show-current` prints it. `[AC-24]`
  - Result: `worktree/release-v0.8.5` from `origin/main` at `2555fc1`.
- [x] `[AI]` Before cutting, confirm every bug-fix plan this release carries has merged:
      `fix-cancelled-waiter-cleanup-flake`, `fix-degraded-lineage-scenario-flake`, and
      `fix-distinct-root-lock-test-flake`, plus any later one under `plans/in-progress/fix-*` (added 2026-10-06 at the
      owner's direction that every flake found is fixed); proof: each fix pull request reads `MERGED` and its merge
      commit is an ancestor of this branch's head. If one has not merged, wait for it rather than cut without it.
      `[AC-24]`
  - Result: (2026-10-07) #141 `aa274ee` (`fix-cancelled-waiter-cleanup-flake`), #142 `c2a08ca`
    (`fix-degraded-lineage-scenario-flake`), #143 `4e1e7d5` (`fix-distinct-root-lock-test-flake`), and #144 `6b28306`
    (`fix-corrupt-waiter-identity-test-flake`, the one later fix plan) read `MERGED`, and `git merge-base --is-ancestor`
    exits `0` for each against this branch's head and against the tag `v0.8.5`.
- [x] `[AI]` Add the `v0.8.5` entry to `CHANGELOG.md`, or complete and date the `## [v0.8.5] — Unreleased` entry the
      cancelled-waiter fix added — `Fixed`: the `minimal`-lineage floor, naming the configured profiles whose exit `125`
      becomes an admission — and name `v0.8.5` as the current release in the five pages the file impact lists; proof:
      `git grep -n 'v0\.8\.4' -- ':!plans' ':!CHANGELOG.md'` prints nothing. `[AC-24]`
  - Result: (2026-10-07) `## [v0.8.5] — 2026-10-07`, verified against `git diff v0.8.4..HEAD` and by running a binary
    built from `git archive v0.8.4` and one built from `HEAD` over the same crafted configurations, evidence, and
    ledgers in a scratch `HIPPO_ROOT`. `Fixed` keeps the cancelled-waiter bullet and adds the floor: a configured
    profile extending `minimal` (directly, through another configured profile, or reached by another profile's
    `fallback`) with no `fallback` and a disk reserve no host meets exited `125` naming `hippo.policy.replan-required`
    on `v0.8.4` and runs on `HEAD`; a `constrained`-lineage profile with no `fallback` and transactional work exit `125`
    on both. The bullet also names two narrowings the binaries show where name and lineage disagree: a configured
    `minimal` with `extends: constrained, fallback: ""`, and a configured `minimal` that names a `fallback`, took the
    floor on `v0.8.4` and now do not (the second falls back; with a `constrained`-lineage fallback it exits `125`).
    `Changed` lists the diagnostic text of already-refused inputs, each with the same exit status and code on both
    binaries: the coordination mode wording, schema 1 with an unknown mode, the mode error preempting an unknown
    `extends`, a ledger owner with class `batch`, a `sheddingExitCode` of `99`, and the Go type names for non-string
    values. Left out, because the binaries agree: automatic owner shares (a configured profile that extends
    `constrained`, at depth one or two, is allocated 6 of 12 CPUs on both, since `buildCoordination` already inherits
    shares through `extends` for every catalog profile, so the guard's lineage default is unreachable from the command
    line), and `history` over an unknown `outcome`, `budgetOutcome`, or `taskClass` (listed as recorded, selected by no
    `--outcome` or `--class`, on both). The five pages now name `v0.8.5`, and
    `git grep -n 'v0\.8\.4' -- ':!plans' ':!CHANGELOG.md' ':!*.go'` prints nothing. Deviation from the item's proof
    grep: 18 comment lines in 13 Go files under `internal/` and `tests/` still cite `v0.8.4` as the historical baseline
    the typed codecs preserve, and stay unchanged.
- [x] `[AI]` Run docs propagation and the _Full gate_; proof: status recorded and exit `0`. `[AC-24]`
  - Result: (2026-10-07) docs propagation status `landed`, committed as `e0c9648` and `0f7be31`; the _Full gate_
    (`GOFLAGS=-timeout=30m npm test`) exited `0` at load 3.9–7.7: selected production line coverage 99.38% (955/961),
    race detector clean, `govulncheck` "No vulnerabilities found." Updated: `docs/reference/resource-policy.md` (Derived
    thresholds: the floor applies to the profile that ends the fallback chain when its lineage reaches `minimal`, and a
    `minimal`-lineage profile with a `fallback`, set or inherited, falls back instead; the Unit 4 wording, "the first
    profile of the `minimal` lineage its fallback chain reaches", was wrong for such a profile) and
    `docs/reference/configuration.md` (the lineage note: only a `minimal`-lineage profile with no `fallback` is a floor,
    and a profile inherits its parent's configured share, so it gets its lineage's built-in share only when no profile
    in its lineage is named). Verified unchanged: the owner-share tables in `configuration.md`, `resource-policy.md`,
    and `enable-reservation-coordination.md`; the recorded unknown outcome and class in `cli.md`, `json-schemas.md`, and
    `specs/architecture.md`; and no page or spec quotes a diagnostic whose text changed. `npm run format:check`,
    `markdownlint-cli2` over the changed pages, `scripts/check-markdown-line-length.sh`, and
    `./rhino md internal-link validate` (1375 links, no findings) exit `0`.
- [x] `[AI]` Run the [docs quality gate](../../../repo-governance/workflows/quality/docs-quality-gate.md) on subject
      `all` on this branch's head, before landing, so any repair it makes is committed here and reaches the commit being
      tagged (PQG-14); proof: its verdict line recorded here and `git status --porcelain` printing nothing after the
      repairs are committed. `[AC-24]`
  - Result: (2026-10-07) `docs-quality-gate: PASS_WITH_FINDINGS (2 cycles, 1 MEDIUM and 2 LOW open)` on `0f7be31`, then
    `91b3784`. Cycle 1 (`all__20261006T2032Z`) found D-001 (HIGH, pre-existing since before `v0.8.4`: a release check
    whose first reading does not fit exits `125` naming `hippo.policy.replan-required`, where `cli.md` and
    `monitor-a-release.md` said `124`) and six MEDIUM or LOW rows; the documents, not the code, were fixed, because the
    binary agrees with `release.feature` and the explanation page, and the fixer added the one first-reading `124` (free
    disk below the 256 MiB floor). Cycle 2 (`all__20261006T2055Z`) held all seven and left D-008 (MEDIUM: the first
    reading has no CPU figure on Linux), D-009, and D-010 (LOW: a "v1 client" label; the `HIPPO_` prefix rule) open and
    non-blocking; all three were then fixed here, verified against `internal/cli/release.go` and
    `internal/host/collector.go`. Repairs committed as `91b3784` and the commit after it; formatter, linter, line
    length, word budget, and internal links pass, and `git status --porcelain` printed only this plan's record.

- [x] `[AI]` Land it: data-safety inspection and commit, push review and push, screened draft pull request, ready,
      `Quality gate` on the head, the leak review for that head, rebase merge, and _Reconcile_; proof: the merge commit
      and `0 0` recorded. `[AC-24]`
  - Result: (2026-10-07) two pull requests. #146 (head `206f427`, `Quality gate` `success` in run 37530951853, `pass`
    leak review on that head) merged as `02caddb`; reconcile `0 0`. The manual test below found F1, which blocked the
    tag, so `worktree/release-v0.8.5-changelog` carried a `CHANGELOG.md`-only fix as #147 (head `ecf5591`,
    `Quality gate` `success` in run 37535560635, `pass` leak review) merged as `456d24b`; reconcile `0 0`. Each branch
    was deleted locally after its merge; `origin` had already removed it.
- [x] `[AI]` Move this worktree onto the merge commit, because the rebase merge leaves it on `worktree/release-v0.8.5`
      at the pre-rebase commit and `scripts/build-release.sh` refuses unless its commit equals the checkout's `HEAD` and
      the checkout is clean, untracked files included: `git switch --detach <merge commit>`; proof: `git rev-parse HEAD`
      prints the merge commit recorded above and `git status --porcelain --untracked-files=all` prints nothing.
      `[AC-24]`
  - Result: (2026-10-07) `git switch --detach 02caddb`, then `git switch --detach 456d24b` after #147;
    `git rev-parse HEAD` printed `456d24bc2a20ee23a7b81746c1789133cd5c3a97` and
    `git status --porcelain --untracked-files=all` printed nothing.
- [x] `[AI]` Run `scripts/test.sh` in this worktree, now a clean detached checkout of the merge commit; proof: exit `0`
      and `git status --porcelain --untracked-files=all` still printing nothing. `[AC-24]`
  - Result: (2026-10-07) at `456d24b`: exit `0`, selected production line coverage 99.38% (955/961), and
    `git status --porcelain --untracked-files=all` still printed nothing. A first run at `02caddb` was stopped once F1
    superseded that commit.
- [x] `[AI]` Manual exploratory test of the release candidate, added at the owner's direction on 2026-10-06 because
      every repository on the workstation runs HIPPO: build `hippo` from this clean detached checkout into
      `local-tmp/manual-v0.8.5/`, fetch the published `v0.8.4` binary beside it as the baseline, and run both under an
      isolated `HIPPO_ROOT` in `local-tmp/` through these charters: `run` of a passing, a failing, and a signalled child
      (exit statuses and summary outcomes); `status --json` with the built-in and a configured profile (`profile` fields
      unchanged); `history` and `history --outcome` over evidence holding an outcome word this build does not know
      (listed as recorded, not selectable); a configured profile that extends `minimal` and does not fit (the floor
      applies where `v0.8.4` exited `125`); automatic owner shares of a profile that extends `balanced`; a configuration
      whose `coordination.mode` is unknown and one whose mode is empty; and an invalid invocation (`2`,
      `hippo.args.invalid`). Proof: a session log under `local-tmp/manual-v0.8.5/` and a table recorded here naming each
      charter with the observed status, code, and output against the baseline; every difference from `v0.8.4` is the
      intended floor and owner-share fix or a finding, and any finding blocks the tag until a new pull request fixes it.
      `[AC-24]`
  - Result: (2026-10-07) session log `local-tmp/manual-v0.8.5/session.md`: the candidate built from `02caddb` against
    the published `v0.8.4` binary, each charter in its own isolated `HIPPO_ROOT`, plus a go1.26.1 build of the candidate
    to cross-check the toolchain and charter h for the two narrowings the `CHANGELOG.md` entry names. `456d24b` differs
    from `02caddb` only in `CHANGELOG.md`, so the results hold for the tagged commit.

    | Charter                                  | `v0.8.4`                      | `v0.8.5` candidate    | Verdict       |
    | ---------------------------------------- | ----------------------------- | --------------------- | ------------- |
    | a. `run` pass, fail, `exit 3`, SIGTERM   | `0`, `1`, `3`, `143`          | identical             | same          |
    | b. `status --json`, six profiles         | `profile` objects             | byte-identical        | same          |
    | c. `history` over unknown words          | listed; unknown filter `2`    | byte-identical        | same          |
    | c. non-string history `outcome`          | `125`, `of type string`       | `125`, names the type | intended text |
    | d. `minimal`-lineage misfit, four shapes | `125`, replan-required        | `0`, the floor        | intended      |
    | d. with a `fallback`; transactional      | `0`; `125`                    | identical             | same          |
    | e. owner shares, `balanced` lineage      | 3 CPU and 7 GiB, and variants | identical             | same          |
    | f. unknown `coordination.mode`           | `125`, schema wording         | `125`, mode wording   | intended text |
    | f. empty or omitted mode                 | `0`; schema 1 `125`           | identical             | same          |
    | f. non-string `mode` or `extends`        | `125`, `of type string`       | `125`, names the type | intended text |
    | g. nine invalid invocations              | `2`, `hippo.args.invalid`     | byte-identical        | same          |
    | h. the two name-lineage narrowings       | `0`, floored as `minimal`     | `125` or falls back   | intended (D5) |
    | h. `minimal`, no `extends` or `fallback` | `0`, the floor                | identical             | same          |

    Every difference is the intended floor or diagnostic text; the owner-share charter shows none, as the `CHANGELOG.md`
    entry records. Finding F1 (low, documentation): the entry quoted a `JSON value must be string type` suffix that only
    a go1.27.1 build prints, while the release toolchain is go1.26.1 from `go.mod`. It blocked the tag until #147
    corrected the entry.
- [x] `[AI]` Screen the tag name and the notes
      `gh api repos/wahidyankf/hippo/releases/generate-notes -f tag_name=v0.8.5 --jq .body` returns with
      `scripts/public-safety/outbound-preflight.sh --surface release`; proof: exit `0`. `[AC-24]`
  - Result: (2026-10-07) the notes generated for `v0.8.5` at `456d24b` and the tag name:
    `scripts/public-safety/outbound-preflight.sh --surface release` exits `0`, and so does the workstation's outbound
    preflight.
- [x] `[AI]` In this detached worktree, run `./scripts/build-release.sh v0.8.5 <merge commit> <output-dir>` and
      `./tests/artifacts/release-assets.sh <output-dir> v0.8.5 <merge commit>`, with `<output-dir>` the ignored
      `local-tmp/release-v0.8.5` so the checkout stays clean; proof: both exit `0`. `[AC-24]`
  - Result: (2026-10-07) at `456d24b`: `./scripts/build-release.sh v0.8.5 456d24b... local-tmp/release-v0.8.5` and
    `./tests/artifacts/release-assets.sh local-tmp/release-v0.8.5 v0.8.5 456d24b...` both exit `0`.
- [x] `[AI]` Create the annotated tag `v0.8.5` on the merge commit and push it; proof: `release.yml`'s run and the
      published release's `checksums.txt` recorded here. `[AC-24]`
  - Result: (2026-10-07) annotated tag `v0.8.5` (`f24bba1`) on `456d24b`, pushed; `release.yml` run 37539705716
    `success` on `456d24b`; published at <https://github.com/wahidyankf/hippo/releases/tag/v0.8.5>. `checksums.txt`:

    ```text
    70f6ea5475adf5b818ed1be46e18ce481493cfecae635b1fb0ca5f806c6aefba  hippo_v0.8.5_darwin_amd64.tar.gz
    498beba385fc887ac75f41e92ae091854a92354570b2c2c48109877b5ad34862  hippo_v0.8.5_darwin_arm64.tar.gz
    13aac6777ea838a80eb699bc00816ca7f59ae6e1be4bbefd007fa15666cb7c4c  hippo_v0.8.5_linux_amd64.tar.gz
    1b3e60aa5f491228487f8994efe237d5c2cbbf42b32d69c46cdb5295b683b6f2  hippo_v0.8.5_linux_arm64.tar.gz
    ```

    The local build's digests differ because it used go1.27.1; `release.yml` builds with go1.26.1 from `go.mod`, and the
    published binary reports go1.26.1.
- [x] `[AI]` Run the commands in `docs/how-to/install-a-pinned-release.md` for this platform in an empty directory;
      proof: the checksum line reads `OK` and `version --json` names `v0.8.5`. `[AC-24]`
  - Result: (2026-10-07) in an empty directory: `hippo_v0.8.5_darwin_arm64.tar.gz: OK`, and `version --json` names
    `v0.8.5` with commit `456d24bc2a20ee23a7b81746c1789133cd5c3a97`.

> **Pause Safety**: `v0.8.5` is published and never replaced. Consumer repins are coordinated outside this repository.

## Phase 8: Knowledge Capture and Execution Check

- [x] `[AI]` Route every `learnings.md` entry to one durable owner or discard it with a reason; proof: no unresolved
      entry. `[AC-25]`
  - Result: (2026-10-07) all 50 entries in `learnings.md` end with a dated **Resolution**: most are already owned
    (governance pages, doc comments, tests, `specs/`, `CHANGELOG.md`, the four bug-fix plans), some are promoted
    (comments in `tests/support/domain_literals.go`, `blockers_v04.go`, `release_v04.go`, and `pending_v04.go`; one
    sentence each in `release-cut.md` Preconditions and `gherkin-implementation-review.md` Assertion Theater, through
    Rules Propagation with no conflict), and the rest discarded with a reason. The Unit 6 diagnostic entry is corrected:
    its suffix was a go1.27.1 measurement (F1, #147). The plan's `tech-docs/` are archived with it and so own nothing,
    which supersedes the Unit 6 close note that left routing into `tech-docs/` to this item; the file impact additions
    are discarded as recorded in each unit's results.
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
