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
- _Unit adapter_: `HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd`. _Integration adapter_: the same with
  `HIPPO_BDD_ADAPTER=integration`.
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

- [ ] `[AI]` Confirm the checkout once the plan pull request has merged: `git -C ../.. worktree list` lists this
      worktree once, and `git status --porcelain` prints nothing; proof: both outputs recorded here. `[AC-25]`
- [ ] `[AI]` Confirm every post-write question has an answer. From the worktree root, with
      `plan=plans/in-progress/strict-go-linting-and-domain-modeling/delivery.md`, run
      `grep -cE '^- \*\*PW-[0-9]+\*\*' "$plan"` and
      `tr '\n' ' ' < "$plan" | grep -oE 'Answer \(owner, +[0-9-]+\)' | wc -l`; proof: both print `7`, one answer for
      each of PW-1 to PW-7, and the counts are recorded here. `[AC-25]`
- [ ] `[AI]` Run the quick gate on `origin/main` as the baseline; proof: exit `0` and the core coverage figure recorded
      here. `[AC-01]`

### Phase 0 Gate

- [ ] `[AI]` Run `./rhino md internal-link validate` and `./rhino governance directory-map validate`; proof: both exit
      `0`. `[AC-25]`

> **Pause Safety**: nothing has changed. To resume: re-run the Phase 0 Gate.

## Phase 1: Unit 1 — Gates

Branch `worktree/strict-go-linting-gates`.

### Lint settings

- [ ] `[AI]` Create the branch with the starting commands; proof: `git branch --show-current` prints it. `[AC-03]`
- [ ] `[AI]` **RED** (`swe-developer`): add the step "govet runs nilness and exhaustive checks switch statements and map
      literals" to "Lint gate wiring is exhaustive and module scoped" in `specs/behaviours/quality-gates.feature`, bound
      in `tests/support/steps.go` to read `.golangci.yml`; run the unit adapter; acceptance: it fails at that step
      because `.golangci.yml` names neither setting. `[AC-03]` `[AC-04]`
- [ ] `[AI]` **GREEN** (`swe-developer`): add `govet: enable: [nilness]` and `exhaustive: check: [switch, map]` to
      `.golangci.yml`; run the unit adapter, then _Lint_; acceptance: the adapter passes, and lint reports exactly one
      finding, `exhaustive` at `internal/status/status.go:179`. `[AC-03]` `[AC-04]`
- [ ] `[AI]` **GREEN** (`swe-developer`): list all 18 codes in `retryable` (`internal/status/status.go`), `true` only
      for `CodeLimitCapacityDeferred` and `CodeLimitPressureShed`; run _Lint_ and `go test -count=1 ./tests/unit`;
      acceptance: both exit `0`. `[AC-04]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): rewrite the `retryable` comment to say every code is listed so a new code
      needs a decision; run _Lint_; acceptance: exit `0`. `[AC-04]`
- [ ] `[AI]` Mutation: add `internal/policy/nilness_mutation.go` dereferencing a pointer inside its own `== nil` branch,
      run _Lint_, record the output, delete the file; acceptance: lint fails naming `govet` and `nilness`, and passes
      once the file is gone. `[AC-03]`
- [ ] `[AI]` Mutation: delete the `CodeArgsInvalid` key from `retryable`, run _Lint_, record the output, restore it;
      acceptance: lint fails naming `exhaustive` and `status.CodeArgsInvalid`, and passes once restored. `[AC-04]`

### NilAway

- [ ] `[AI]` Pin it: `go get -tool go.uber.org/nilaway/cmd/nilaway@v0.0.0-20260918162853-acb8859b9031`, then
      `go mod tidy`; proof: `go.mod`'s `tool` block names `go.uber.org/nilaway/cmd/nilaway` and `go tool nilaway -h`
      exits `0`. `[AC-01]`
- [ ] `[AI]` Run _NilAway_ and record every diagnostic here; acceptance: it exits `3`, and the list is compared with the
      eight diagnostics [the gates](tech-docs/002-gates-and-analysis.md#nilaway) record, any difference named. `[AC-01]`
- [ ] `[AI]` Bounded checkpoint, one attempt: put `//nolint:nilaway // <reason>` on one finding classified as a false
      positive (or, if none is, on a scratch copy of one finding), run _Lint_; acceptance: exit `0` makes inline
      directives the mechanism; a `nolintlint` finding makes `-exclude-errors-in-files` on the `scripts/test-quick.sh`
      line the mechanism, with nothing retried. Record which. `[AC-01]`
- [ ] `[AI]` **RED** (`swe-developer`): for the `internal/guard/exclusive_status.go:60` finding, add a test in
      `internal/guard/exclusive_status_test.go` where `liveExclusiveHeavyOwner` returns `nil` and the caller proceeds;
      run `go test -count=1 -run Exclusive ./internal/guard`; acceptance: it panics with a nil dereference. At most two
      attempts; if neither reaches the dereference, the finding is a false positive and the next GREEN and REFACTOR
      record `Not applicable` while the exclusion item covers it. `[AC-01]`
- [ ] `[AI]` **GREEN** (`swe-developer`): guard the nil result in `internal/guard/exclusive_status.go`; run the same
      command; acceptance: it passes. `[AC-01]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): keep the guard in the shape the surrounding error handling uses; run
      `go test -count=1 ./internal/guard` and _NilAway_; acceptance: the tests pass and NilAway no longer reports the
      file. `[AC-01]`
- [ ] `[AI]` **RED** (`swe-developer`): for the `internal/guard/lease.go:315` finding, add a test in
      `tests/integration/lease_evidence_test.go` where `readLeaseOwner` returns a `nil` owner without an error; run
      `go test -count=1 ./tests/integration`; acceptance: it panics with a nil dereference, under the same two-attempt
      rule. `[AC-01]`
- [ ] `[AI]` **GREEN** (`swe-developer`): guard the nil owner in `internal/guard/lease.go`; run the same command;
      acceptance: it passes. `[AC-01]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): fold the guard into the existing owner checks at lines 315–327; run the
      same command and _NilAway_; acceptance: both clean for the file. `[AC-01]`
- [ ] `[AI]` Fix the six test-code findings by checking each value before use (`t.Fatal` on `nil`) in
      `tests/support/release_v04.go`, `internal/guard/run_test.go`, `tests/integration/lease_evidence_test.go`, and
      `tests/support/isolation_test.go` (`swe-developer`); run _NilAway_; acceptance: no diagnostic names those files.
      `[AC-01]`
- [ ] `[AI]` Exclude each remaining false positive with the mechanism the checkpoint chose, each with its reason; run
      _NilAway_; acceptance: exit `0`. `[AC-01]`
- [ ] `[AI]` **RED** (`swe-developer`): add the step "the quick gate invokes the pinned NilAway over the module" to the
      lint-wiring scenario, bound to read `scripts/test-quick.sh`; run the unit adapter; acceptance: it fails at that
      step. `[AC-01]`
- [ ] `[AI]` **GREEN** (`swe-developer`): add the _NilAway_ command to `scripts/test-quick.sh` directly after
      `go tool golangci-lint run`; run the unit adapter and the quick gate; acceptance: both exit `0`. `[AC-01]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): give the new line a comment stating why NilAway runs beside golangci-lint
      and why `-json` is not used; run `./scripts/format-check.sh`; acceptance: exit `0`. `[AC-01]`
- [ ] `[AI]` Mutation: add a production line reading a field of `liveExclusiveHeavyOwner`'s result unguarded, run the
      quick gate, record the output, remove it; acceptance: the gate fails at the NilAway step naming that file and
      line, and passes once removed. `[AC-02]`

### Domain literal analysis

- [ ] `[AI]` **RED** (`swe-developer`): add `tests/support/domain_literals_internal_test.go` with fixtures for each rule
      in [the gates](tech-docs/002-gates-and-analysis.md#domain-literal-analysis): a `ResolvedProfile == "balanced"`
      comparison, a `case "constrained":` over a profile, a raw `Outcome string` field, a raw `class string` parameter,
      a defined-type value compared with a literal, and the negatives (`""`, `0`, test files, unlisted names), plus a
      stale allowlist entry; run _Analysis fixtures_; acceptance: it fails to compile because the analysis does not
      exist. `[AC-05]` `[AC-06]` `[AC-07]`
- [ ] `[AI]` Bounded checkpoint, one attempt: load every production package with `go list -export -json` and
      `go/importer` in `gc` mode; acceptance: all load, or the syntax-only fallback is adopted and recorded here.
      `[AC-05]`
- [ ] `[AI]` **GREEN** (`swe-developer`): implement `tests/support/domain_literals.go`; run _Analysis fixtures_;
      acceptance: every fixture passes. `[AC-05]` `[AC-06]` `[AC-07]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): sort findings and print `<path>:<line>: <rule>: <identifier>`; run
      _Analysis fixtures_ and _Lint_; acceptance: both exit `0`. `[AC-05]`
- [ ] `[AI]` **RED** (`swe-developer`): add "Production code compares no domain value with a literal" to
      `specs/behaviours/quality-gates.feature` with its end-to-end exemption in `tests/contract/contract.go`; run the
      unit adapter, then bind the steps with an empty allowlist and run it again; acceptance: first undefined, then
      failing with the current findings, whose count and list are recorded here. `[AC-05]`
- [ ] `[AI]` **GREEN** (`swe-developer`): add `tests/support/domain_literals_allowlist.go` with one entry per finding,
      each tagged with the unit that removes it; run the unit and integration adapters; acceptance: both exit `0`.
      `[AC-05]` `[AC-07]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): group the entries by removing unit with a comment per group; run the unit
      adapter; acceptance: exit `0`. `[AC-07]`
- [ ] `[AI]` Mutation: add `if resolution.ResolvedProfile == "balanced" {}` to `internal/guard/reservation.go`, run the
      unit adapter, record the output, remove it; acceptance: it fails naming that file, that line, and the
      literal-comparison rule, and passes once removed. `[AC-05]`

### Unit 1 close

- [ ] `[AI]` Apply [rules propagation](../../../repo-governance/workflows/quality/rules-propagation.md) to the gate
      changes: the repository adapter's `golang-standards.md` gates entry (nilness, exhaustive maps, NilAway and its
      exclusions, the analysis, `gochecksumtype`'s empty target), its Version Sources, and `quality-gates.md`'s quick
      gate order; proof: the run's status and ledger recorded here, and `npm run test:quick` exits `0`. `[AC-01]`
- [ ] `[AI]` Run [docs propagation](../../../repo-governance/workflows/quality/docs-propagation.md) for the unit; proof:
      its status and updated files recorded here. `[AC-01]`
- [ ] `[AI]` Run the
      [Gherkin implementation review](../../../repo-governance/workflows/quality/gherkin-implementation-review.md) over
      the two changed scenarios; proof: each status with its implementation and test path recorded here, none
      `untested`, `unimplemented`, or `drifted`. `[AC-05]`
- [ ] `[AI]` Dispatch `swe-reviewer` over the unit's diff; proof: its findings recorded here with none blocking open.
      `[AC-01]`
- [ ] `[AI]` Run the _Full gate_; proof: exit `0` with the coverage figure recorded. `[AC-01]` `[AC-02]`

### Unit 1 landing

- [ ] `[AI]` Inspect the diff against
      [data safety](../../../repo-governance/conventions/public-repository-data-safety.md) and commit thematically;
      proof: every hook passes and `git log --oneline origin/main..HEAD` is recorded. `[AC-01]`
- [ ] `[AI]` Run the [push review](../../../repo-governance/workflows/quality/pr-leak-review/002-push-review.md), push,
      screen the title and body with `scripts/public-safety/outbound-preflight.sh --surface pull-request`, and open a
      draft pull request; proof: the screen exits `0` and the pull request number is recorded. `[AC-01]`
- [ ] `[AI]` Mark it ready and wait, polling no faster than every three minutes, for `Quality gate` on the head; proof:
      `success` on the recorded head. `[AC-01]`
- [ ] `[AI]` Post the [leak review](../../../repo-governance/workflows/quality/pr-leak-review.md) for that exact head;
      proof: the `leak-review` status on the head reads `success`. `[AC-01]`
- [ ] `[AI]` Rebase-merge once every [merge precondition](../../../repo-governance/conventions/pull-request-merge.md)
      holds, then _Reconcile_; proof: the merge commit recorded and the reconcile count `0 0`. `[AC-01]`

> **Pause Safety**: Unit 1 is on `main`. Safe to stop. To resume: the starting commands for Unit 2.

## Phase 2: Unit 2 — Outcome

Branch `worktree/typed-run-outcome`.

- [ ] `[AI]` Create the branch with the starting commands; proof: `git branch --show-current` prints it. `[AC-09]`
- [ ] `[AI]` **RED** (`swe-developer`): add "History lists an outcome this version does not know as recorded" to
      `specs/behaviours/public-cli.feature`; run the unit adapter; acceptance: its steps are undefined. Bind them in
      `tests/support/steps.go` and `tests/support/history_v05.go`; acceptance: it passes, because `v0.8.4` reads the
      outcome as a plain string — this scenario pins tolerance through the change, and the mutation below proves it can
      fail. `[AC-11]`
- [ ] `[AI]` Prove PW-4's condition, that no released HIPPO version ever wrote `pressure-shed` or `storage-shed` into
      `budgetOutcome`, before `BudgetOutcome` is typed: `git tag --list 'v*'` lists `v0.8.4`, then run the PW-4 proof
      from _Commands the items name_; proof: the output recorded here. Empty output means every release tag writes the
      field only through `SetReservationContext`, called only with `"admitted"`: the condition holds, `BudgetOutcome`
      keeps the members `Unset` and `Admitted`, and the history GREEN below deletes the two comparisons. Any line
      printed names a tag and another writer: the condition fails, the comparisons stay, typed, `BudgetOutcome` gains a
      member for each value printed, and the next RED, the next GREEN, and the history GREEN follow that branch. Record
      which branch applies. `[AC-11]`
- [ ] `[AI]` **RED** (`swe-developer`): add `internal/evidence/outcome_test.go` (each member's wire string, `Unset` and
      `Unknown` refused by the encoder, strict `ParseOutcome`, tolerant `RecordedOutcome` round trip) and
      `tests/unit/outcome_vocabulary_test.go` (the list in `docs/reference/json-schemas.md` equals
      `evidence.Outcomes()`); run `go test -count=1 ./internal/evidence ./tests/unit`; acceptance: compilation fails on
      the missing type. `[AC-09]` `[AC-11]`
- [ ] `[AI]` **GREEN** (`swe-developer`): add `internal/evidence/outcome.go` as
      [the design](tech-docs/001-domain-types.md#run-outcome-unit-2) specifies; run the same command; acceptance: it
      passes. `[AC-09]` `[AC-11]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): keep one wire table in `MarshalText` and derive `ParseOutcome` from
      `Outcomes()`; run the same command and _Lint_; acceptance: both exit `0`. `[AC-09]`
- [ ] `[AI]` **RED** (`swe-developer`): add to `internal/guard/run_test.go` a `finalOutcome` test (unset gives
      `supervision-failed` and `hippo.supervision.failed`) and a test that a deadline deferral's summary records
      `capacity-deferred`; run `go test -count=1 -run 'FinalOutcome|DeadlineDeferral' ./internal/guard`; acceptance:
      compilation fails on `finalOutcome`. `[AC-10]` `[AC-09]`
- [ ] `[AI]` **GREEN** (`swe-developer`): type the writer in `internal/guard/run.go` and `internal/guard/evidence.go`,
      start the outcome unset, assign `OutcomeCapacityDeferred` at the deadline deferral, and add `finalOutcome`; in
      this same item delete the Unit 2 entries for `internal/guard/evidence.go` from
      `tests/support/domain_literals_allowlist.go`, because the analysis fails on an entry whose violation is gone; run
      the same command and the unit adapter; acceptance: both pass. `[AC-09]` `[AC-10]` `[AC-08]`
- [ ] `[AI]` **RED** (`swe-developer`): add to `internal/guard/run_test.go` a table test of
      `promoteFinalize(exitCode, returnError, finalizeError, launched)`: no finalize error leaves the exit code and
      error as they are; an error the run already returned stands over a finalize error; with no run error, a finalize
      error becomes the run's error with exit code `1`, naming a refused write when nothing launched; and the failure
      `finalOutcome` returns for an unset outcome comes back with code `hippo.supervision.failed`, which `status.Status`
      maps to `125`; run `go test -count=1 -run PromoteFinalize ./internal/guard`; acceptance: compilation fails on
      `promoteFinalize`. `[AC-10]`
- [ ] `[AI]` **GREEN** (`swe-developer`): extract the deferred promotion in `internal/guard/run.go` into the pure
      function `promoteFinalize`, with no hook added to `RunConfig`, and have the deferred closure call it; run the same
      command and `go test -count=1 ./internal/guard`; acceptance: both pass. `[AC-10]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): leave the deferred closure holding only the call, and give
      `promoteFinalize` a comment stating that the unset outcome's supervision failure reaches the caller through it;
      run `go test -count=1 ./internal/guard` and _Lint_; acceptance: both exit `0`. `[AC-10]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): delete `guard.RunOutcomes` and have `internal/cli/history.go` list
      `evidence.Outcomes()`; run `go test -count=1 ./internal/... ./tests/unit`; acceptance: exit `0`. `[AC-12]`
- [ ] `[AI]` **RED** (`swe-developer`): add to `internal/evidence/history_test.go` a row with outcome `future-outcome`
      and `budgetOutcome` `admitted`, asserting it is listed as recorded and never counts toward owner promotion; run
      `go test -count=1 ./internal/evidence`; acceptance: compilation fails on the typed fields. `[AC-11]`
- [ ] `[AI]` **GREEN** (`swe-developer`): type `Summary`, `Query`, and `healthyPromotionSummary` in
      `internal/evidence/history.go` and rename the history flag field to `outcomeFlag` in `internal/cli/commands.go`,
      parsed with `ParseOutcome`. Typing `Summary.BudgetOutcome` makes its two string comparisons stop compiling, so
      this same item applies the branch the PW-4 proof item recorded: when the condition holds, delete the two
      `budgetOutcome` comparisons; otherwise replace both with comparisons against the typed `BudgetOutcome` members. In
      this same item delete the Unit 2 entries for `internal/evidence/history.go` and `internal/cli/commands.go` from
      `tests/support/domain_literals_allowlist.go`; run the same command and the unit adapter; acceptance: both pass.
      `[AC-11]` `[AC-12]` `[AC-08]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): simplify `healthyPromotionSummary` now that it reads only typed members;
      run `git grep -nE '"(pressure|storage)-shed"' -- internal/evidence/history.go` and
      `go test -count=1 ./internal/evidence ./tests/unit`; acceptance: the grep prints nothing and the tests exit `0`.
      `[AC-11]`
- [ ] `[AI]` Mutation: make `RecordedOutcome.UnmarshalText` refuse unknown text, run the unit adapter, record the
      output, restore it; acceptance: the unknown-outcome history scenario fails, and passes once restored. `[AC-11]`
- [ ] `[AI]` Enable `exhaustruct_v5` in `.golangci.yml` with an enforce pattern for `evidence.RecordedOutcome`, then
      plant a literal omitting a field, run _Lint_, record the output, and remove it; acceptance: the planted literal
      fails naming `exhaustruct`, which confirms the pattern syntax, and lint exits `0` once removed. `[AC-09]`
- [ ] `[AI]` Confirm Unit 2's entries are gone from `tests/support/domain_literals_allowlist.go`, each deleted in the
      item that removed its violation; run the unit adapter; acceptance: the allowlist holds no entry tagged Unit 2 and
      the adapter exits `0`. `[AC-08]`

### Unit 2 close

- [ ] `[AI]` Apply rules propagation to the `exhaustruct_v5` change (`.golangci.yml` reason and the adapter); proof: its
      status recorded here. `[AC-09]`
- [ ] `[AI]` Run docs propagation: `docs/reference/json-schemas.md` says history lists an unknown outcome as recorded;
      proof: its status recorded and `npm run format:check` exits `0`. `[AC-11]`
- [ ] `[AI]` Run the Gherkin implementation review over the new history scenario; proof: its status recorded. `[AC-11]`
- [ ] `[AI]` Dispatch `swe-reviewer` over the unit's diff; proof: findings recorded, none blocking open. `[AC-09]`
- [ ] `[AI]` Run the _Full gate_; proof: exit `0` with the coverage figure recorded. `[AC-09]` `[AC-10]` `[AC-11]`
      `[AC-12]`

### Unit 2 landing

- [ ] `[AI]` Inspect the diff against data safety and commit thematically; proof: hooks pass, range recorded. `[AC-09]`
- [ ] `[AI]` Push review, push, screen title and body, open a draft pull request; proof: screen exit `0`, number
      recorded. `[AC-09]`
- [ ] `[AI]` Mark ready and wait for `Quality gate` on the head; proof: `success` on the recorded head. `[AC-09]`
- [ ] `[AI]` Post the leak review for that head; proof: `leak-review` reads `success`. `[AC-09]`
- [ ] `[AI]` Rebase-merge when every precondition holds, then _Reconcile_; proof: merge commit and `0 0` recorded.
      `[AC-09]`

> **Pause Safety**: Unit 2 is on `main`. Safe to stop. To resume: the starting commands for Unit 3.

## Phase 3: Unit 3 — Internal Reasons

Branch `worktree/typed-internal-reasons`. No specification changes: the contract scenarios already pin every status and
code, and run unchanged as regressions.

- [ ] `[AI]` Create the branch with the starting commands; proof: `git branch --show-current` prints it. `[AC-13]`
- [ ] `[AI]` **RED** (`swe-developer`): add `tests/unit/reason_test.go` pinning each member's `v0.8.4` integer (`0`,
      `73`, `74`, `75`, `76`, `78`) and `Stopped`'s unwrapping; run `go test -count=1 ./tests/unit`; acceptance:
      compilation fails on `policy.Reason`. `[AC-13]` `[AC-14]`
- [ ] `[AI]` **GREEN** (`swe-developer`): add `internal/policy/reason.go`; run the same command; acceptance: it passes.
      `[AC-13]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): keep the integer table in one `switch`; run the same command and _Lint_;
      acceptance: both exit `0`. `[AC-13]`
- [ ] `[AI]` **RED** (`swe-developer`): add `internal/cli/status_test.go` asserting each reason's code, exit status, and
      message, and that `ReasonNone` inside a stop reports `hippo.supervision.failed`; run
      `go test -count=1 ./internal/cli`; acceptance: compilation fails on `reasonCode`. `[AC-13]`
- [ ] `[AI]` **GREEN** (`swe-developer`): replace `internalReasons` with `reasonCode` and switch `reasonMessage` over
      `policy.Reason` without a `default` in `internal/cli/status.go`; run the same command; acceptance: it passes.
      `[AC-13]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): delete the `//nolint:exhaustive` above `reasonMessage`; run _Lint_ and
      `git grep -n 'nolint:exhaustive' -- internal`; acceptance: lint exits `0` and the grep prints nothing. `[AC-13]`
- [ ] `[AI]` **RED** (`swe-developer`): add to `internal/guard/run_test.go` a test that a deferral whose summary write
      fails still reports the refused write, and switch the guard tests' deferral expectations to `*policy.Stop`; run
      `go test -count=1 ./internal/guard`; acceptance: the new expectations fail against integer returns. `[AC-13]`
- [ ] `[AI]` **GREEN** (`swe-developer`): return `policy.Stopped(...)` at every site in `internal/guard/run.go`,
      `internal/guard/reservation.go`, `internal/policy/profiles.go`, `internal/cli/development.go`, and
      `internal/cli/release.go`, and treat a stop like `nil` in `promoteFinalize`; run
      `go test -count=1 ./internal/... ./tests/unit`; acceptance: exit `0`. `[AC-13]`
- [ ] `[AI]` **GREEN** (`swe-developer`): update the test files
      [the file impact](tech-docs/004-file-impact.md#unit-3--internal-reasons) lists to read reasons from the stop; run
      the unit and integration adapters and `go test -count=1 ./tests/integration`; acceptance: all exit `0`. `[AC-13]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): delete the five constants and the bare `73`; run
      `git grep -nE '(StorageBlocked|PressureShed|CapacityDeferred|ReplanRequired|ProtocolMismatch)ExitCode' -- '*.go'`
      and _Lint_; acceptance: the grep prints nothing, because the pathspec leaves out the Markdown that names the
      constants, and lint exits `0`. `[AC-13]`
- [ ] `[AI]` **RED** (`swe-developer`): add to `tests/unit/reservation_test.go` cases that a ledger owner with
      `sheddingExitCode` `74` fails at decode and that storage and pressure sheds write `73` and `75`; run
      `go test -count=1 ./tests/unit`; acceptance: compilation fails on `guard.ShedCause`. `[AC-15]` `[AC-16]`
- [ ] `[AI]` **GREEN** (`swe-developer`): add `ShedCause` and its codec to `internal/guard/reservation.go` and type
      `ReservationOwner`'s field; run the same command; acceptance: it passes. `[AC-15]` `[AC-16]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): delete `validSheddingExitCode` and `callerShedCode` for
      `ShedCause.Reason()`; run `go test -count=1 ./internal/guard ./tests/unit` and the unit adapter; acceptance: exit
      `0`, with "Reservation ledger structure is validated before mutation" passing. `[AC-15]`
- [ ] `[AI]` **RED** (`swe-developer`): add to `internal/cli/development_test.go` an assertion that status JSON's
      `profile.exitCode` encodes `73`, `75`, and `78` for cleanup, wait, and replan resolutions; run
      `go test -count=1 ./internal/cli`; acceptance: compilation fails on `Resolution.Reason`. `[AC-14]`
- [ ] `[AI]` **GREEN** (`swe-developer`): replace `Resolution.ExitCode` with `Reason` encoded by `Reason.MarshalJSON`;
      run the same command; acceptance: it passes. `[AC-14]`
- [ ] `[AI]` **REFACTOR** (`swe-developer`): make `withAssessmentDecision` set reasons, not integers; run the unit
      adapter; acceptance: exit `0`. `[AC-14]`

### Unit 3 close

- [ ] `[AI]` Assess `specs/` and run docs propagation; record the verified no-op for the behaviour corpus and whether
      `specs/architecture.md`'s shedding-cause bullet still reads true; proof: the status recorded. `[AC-13]`
- [ ] `[AI]` Dispatch `swe-reviewer` over the unit's diff; proof: findings recorded, none blocking open. `[AC-13]`
- [ ] `[AI]` Run the _Full gate_; proof: exit `0` with the coverage figure recorded. `[AC-13]` `[AC-14]` `[AC-15]`
      `[AC-16]`

### Unit 3 landing

- [ ] `[AI]` Inspect the diff against data safety and commit thematically; proof: hooks pass, range recorded. `[AC-13]`
- [ ] `[AI]` Push review, push, screen title and body, open a draft pull request; proof: screen exit `0`, number
      recorded. `[AC-13]`
- [ ] `[AI]` Mark ready and wait for `Quality gate` on the head; proof: `success` on the recorded head. `[AC-13]`
- [ ] `[AI]` Post the leak review for that head; proof: `leak-review` reads `success`. `[AC-13]`
- [ ] `[AI]` Rebase-merge when every precondition holds, then _Reconcile_; proof: merge commit and `0 0` recorded.
      `[AC-13]`

> **Pause Safety**: Unit 3 is on `main`. Safe to stop. To resume: the starting commands for Unit 4.

## Phase 4: Unit 4 — Profile Lineage

Branch `worktree/profile-lineage-rules`.

- [ ] `[AI]` Create the branch with the starting commands; proof: `git branch --show-current` prints it. `[AC-17]`
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

- [ ] `[AI]` Apply rules propagation to the `exhaustruct_v5` scope change; proof: status recorded. `[AC-20]`
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

- [ ] `[AI]` Apply rules propagation to the closed ratchet and the final `exhaustruct_v5` scope in the adapter; proof:
      status recorded. `[AC-08]`
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
