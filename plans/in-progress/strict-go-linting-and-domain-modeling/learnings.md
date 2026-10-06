# Learnings: strict-go-linting-and-domain-modeling

<!-- Append observations during execution, dated, as they happen. Resolve every entry before archival: promote it to
one durable owner or discard it with a reason. -->

- (2026-10-06, Unit 1) **The plan's _Unit adapter_ command only verifies bindings.**
  `HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd` calls `contract.Verify`, which parses the corpus and checks that
  every step resolves to exactly one binding; it executes no scenario. After the RED step was bound it exited `0` (0.4
  s) although the new check fails. The scenarios execute in `tests/unit`
  (`go test -count=1 -run TestUnitBehaviours ./tests/unit`, about 190 s for all 336 scenarios), `tests/integration`
  (`-run TestIntegrationBehaviours`), and the compiled end-to-end adapter. Items that say "run the unit adapter"
  therefore run the executing form, and the structural `./tests/bdd` form beside it. A single scenario runs in about 3 s
  with `-run 'TestUnitBehaviours/<Scenario_name_with_underscores>'`. Later units inherit this: their RED and GREEN items
  name "the unit adapter" and must be read the same way. Routing candidate: correct _Unit adapter_ and _Integration
  adapter_ in `delivery.md`'s "Commands the items name" through plan propagation.

- (2026-10-06, Unit 1) **NilAway reports one conflict per nil source, so an exclusion masks every later read of that
  source.** `//nolint:nilaway` on the line-60 dereference of `liveExclusiveHeavyOwner`'s result also hides any new
  unguarded read of the same result: NilAway reported only the original site with and without the directive, and the
  plan's Unit 1 mutation (a new read of that result) could not fail the gate. The mutation was proved on an unexcluded
  source instead (`AcquireSession`, `internal/guard/run.go`). Consequence for the adapter record: an exclusion's reason
  must say it covers the whole source, and the next change to a source with an exclusion needs review, because NilAway
  will stay quiet about it. Routing candidate: the repository adapter's NilAway exclusions entry. **Routed**
  (2026-10-06, Unit 1 close) to
  `repo-governance/development/quality/stacks/repository-adapter/001-go-analysis-gates.md`, NilAway exclusions.
- (2026-10-06, Unit 1) **`//nolint:nilaway` scope and noise.** NilAway scopes a directive to the AST node
  `ast.NewCommentMap` attaches it to: a trailing comment after the `{` of an `if` line did not cover the finding on that
  line, a comment line directly above the `if` covers the statement and the whole group of sites sharing the nil source.
  golangci-lint accepts the directive (no `nolintlint` finding, so inline directives are the mechanism) but prints
  `level=warning msg="[runner/nolint_filter] Found unknown linters in //nolint directives: nilaway"` on every run; the
  exit status stays `0`. Routing candidate: the repository adapter. **Routed** (2026-10-06, Unit 1 close) to
  `repo-governance/development/quality/stacks/repository-adapter/001-go-analysis-gates.md`, NilAway exclusions.
- (2026-10-06, Unit 1) **The NilAway pin raises shared modules.** `go get -tool` moved `golang.org/x/sys` (a direct,
  production dependency) from v0.47.0 to v0.48.0 and `golang.org/x/tools`, `x/mod`, `x/sync`, `x/exp/typeparams`, and
  `x/telemetry` with it, by minimal version selection; the plan's selection argument says the tool pulls in tool-only
  modules and does not mention the `x/sys` bump. Build, lint, and the quick gate pass under the new versions. Routing
  candidate: the dependency selection argument in `tech-docs/002-gates-and-analysis.md` and the Version Sources entry.
  **Routed** (2026-10-06, Unit 1 close): the versions to `tech-docs/002-gates-and-analysis.md`'s selection argument; the
  review obligation to `repo-governance/development/quality/stacks/repository-adapter/001-go-analysis-gates.md`, pin
  changes. Version Sources stays `go.mod`, because the repository adapter convention allows only manifest paths there
  and never repeats a version.
- (2026-10-06, Unit 1) **File impact additions.** NilAway's test-code findings reach one file the file impact does not
  list, `tests/integration/run_test.go` (line 182, the same `AcquireSession` source as `lease_evidence_test.go:41`). The
  two attempt tests for the false-positive classification went into `internal/guard/exclusive_status_test.go` and
  `tests/integration/lease_evidence_test.go`, as planned. Routing candidate: `tech-docs/004-file-impact.md`, Unit 1.
  **Routed** (2026-10-06, Unit 1 close) to `tech-docs/004-file-impact.md`, Unit 1, with `tests/support/driver.go`, which
  holds the new wiring checks and was unlisted too.

- (2026-10-06, Unit 1) **The analysis loader needs `go list -deps`, and reads only the running platform's files.**
  `tech-docs/002-gates-and-analysis.md` names `go list -export -json ./cmd/... ./internal/...`; the `gc` importer
  resolves every import, standard library included, through an export file, and `-export` without `-deps` lists none for
  them. The loader runs `go list -export -deps -json`. A pattern whose directory is missing (`./cmd/...` in a module
  without `cmd/`) is an error, not a warning. Files a build constraint excludes on the running platform are not
  analysed, so a violation added only to a Linux-only or Darwin-only file is seen on one platform; today only
  `internal/host` has such files and it carries no domain name. Routing candidate: the Domain Literal Analysis section
  of `tech-docs/002-gates-and-analysis.md` and the repository adapter's entry for the analysis.
- (2026-10-06, Unit 1) **An allowlist counted per symbol cannot name the new finding, so the report names the group.**
  Entries are keyed by file, enclosing symbol, and rule, with a count. The mutation that planted a comparison in
  `PlanReservation`, which already holds two allowlisted comparisons, was reported at an old line, not the planted one.
  `reconcileDomainFindings` now reports every finding of a symbol and rule whose findings outnumber its entries. Units 2
  to 6 delete entries by symbol and rule, so a unit that removes only some of a symbol's comparisons deletes exactly
  that many entries.
- (2026-10-06, Unit 1 close) **A lint-wiring step matched its own setting's comment.** `requireNilnessAndExhaustiveMaps`
  (`tests/support/driver.go`) looks for the substrings `switch` and `map` anywhere in the `exhaustive:` block of
  `.golangci.yml`, and that block's comment, "A map keyed by an enumerated type must name every member, as a switch over
  it must.", holds both words. A scratch copy of the check run on a copy of `.golangci.yml` with `- switch` and `- map`
  deleted still passed both, so the step's map and switch clause is assertion theatre; the `nilness` clause is not,
  because the `govet:` comment does not say `nilness`. The lint behaviour itself is proved by the Unit 1 `retryable`
  mutation. Routing candidate: a `swe-developer` fix that reads the `check:` list items, test-first, then the Gherkin
  implementation review item rerun. **Resolved** (2026-10-06, Unit 1 close): review finding F1, fixed test-first in
  `tests/support/driver.go` with `tests/support/lint_wiring_internal_test.go`; the review rerun records both scenarios
  `implemented`.
- (2026-10-06, Unit 1 close) **The domain literal analysis has known gaps, outside the agreed rules.** Review finding
  F6: it does not refuse a comparison with `-1` (a unary expression, not a literal) or a rune literal, a comparison
  through a `string(x)` conversion, or a domain name declared as a named result, and an allowlist entry's key (file,
  symbol, rule) omits the identifier. Accepted for now as outside the PW-1 and PW-2 rules. Routing candidate: the Unit 6
  item that removes the allowlist, which can drop the key gap, and an idea brief if a later defect shows one of the
  other gaps matters.
- (2026-10-06, Unit 2) **The unit adapter shells out to `go test ./internal/guard`, so a RED in that package fails the
  adapter.** About a dozen unit-adapter scenarios ("Schema-one ownership survives supervisor-only death", "Transactional
  owners are the final emergency victim", and others) call `runInternalGuardRegression` and run a named `internal/guard`
  test; when `internal/guard`'s tests do not compile, each reports `FAIL ... [build failed]` and the suite exits `1`
  (`go test -count=1 ./tests/unit` showed 156 `FAIL` lines for two undefined symbols). A RED item that leaves
  `internal/guard` uncompilable therefore cannot be followed by a unit-adapter run until its GREEN, and an adapter run
  that overlaps edits to `internal/guard` is contaminated. Unit 2's items already order the runs this way; later units
  that add RED tests to `internal/guard` or `internal/policy` inherit it. Routing candidate: the Unit 3 to 5 items that
  name a unit-adapter run, and the plan-propagation note on _Unit adapter_.
- (2026-10-06, Unit 2) **`exhaustruct_v5` checks every literal unless `explicit-mode` is on, and `enforce-patterns` are
  full-path regexes.** The plan scopes the linter with `enforce-patterns` alone; in the default implicit mode that would
  require every struct literal in the module to be complete and bring back the brittleness that disabled `exhaustruct`.
  `dev.gaijin.team/go/exhaustruct/v5@v5.0.3` reads each pattern as a regex over `import/path.TypeName`, so Unit 2 sets
  `explicit-mode: true` with `^github\.com/wahidyankf/hippo/internal/evidence\.RecordedOutcome$` (and the same for
  `RecordedBudgetOutcome`); a planted literal missing its unexported `text` field reported
  `evidence.RecordedOutcome is missing field text (exhaustruct_v5)`, and the rest of the module (incomplete literals
  everywhere) still passes. Units 5 and 6 add their patterns the same way. Routing candidate:
  `tech-docs/002-gates-and-analysis.md`'s `exhaustruct_v5` scope paragraph, and the adapter's gates entry (the Unit 2
  close item). **Routed** (2026-10-06, Unit 2 close) to both: the as-built paragraph in `tech-docs/002`, and the
  `exhaustruct_v5` entry in `repo-governance/development/quality/stacks/repository-adapter/001-go-analysis-gates.md`.
- (2026-10-06, Unit 2) **File impact additions.** Beyond `tech-docs/004-file-impact.md`'s Unit 2 list, the unit also
  touched `internal/guard/reservation.go` (the cancelled-receipt reason used a deleted constant),
  `internal/cli/interruption_test.go`, `tests/support/interruption_v082.go`, `tests/support/driver.go`,
  `tests/support/pending_v04.go`, and `tests/integration/lease_evidence_test.go` (typed reads and `Finalize` calls), and
  it added `internal/evidence.RecordedBudgetOutcome` and `BudgetOutcomeUnknown`, which the design implied (D6) but did
  not list. Routing candidate: `tech-docs/004-file-impact.md`, Unit 2. **Routed** (2026-10-06, Unit 2 close) to
  `tech-docs/004-file-impact.md`, Unit 2, with the docs propagation's `docs/reference/cli.md` and
  `specs/architecture.md`.
- (2026-10-06, Unit 2) **The full gate's race step sits near Go's default 10-minute package timeout under host load.**
  At load averages of 15–18 (an indexing workstation), `go test -race ./tests/integration` passed 600 s and failed the
  gate; the same step alone took 359 s, and a rerun at lower load took 534 s, against 275 s during Unit 1. The non-race
  integration run moved from 213 s to 220 s at comparable load, so the code did not slow it. Routing candidate:
  `scripts/test.sh` (an explicit `-timeout` on the race step) or the quality-gates page, as a separate change.
- (2026-10-06, Unit 3) **The plan's items could not run in their written order.** The delivery items put the
  `Resolution.Reason` and constant-deletion REFACTOR (item 10) before the `ShedCause` and `Resolution` items it depends
  on: the constants `run.go`, `reservation.go`, `development.go`, and the `tests/support` readers still use are deleted
  only once `ShedCause` and `Resolution.Reason` exist. Unit 3 ran the items in dependency order, with item 10 last, and
  ticked each when its own acceptance held. Routing candidate: `delivery.md`, Unit 3, as a note on the item order for a
  later unit that follows the same RED, GREEN, and REFACTOR pattern over a shared constant.
- (2026-10-06, Unit 3) **`tests/unit` cannot compile between a RED that adds a test for a missing symbol and its GREEN,
  so the plan's RED acceptance, "compilation fails", is also the state in which the unit adapter cannot run.** The
  `legacyReasons` bridge in `internal/cli/status.go` followed from it: the guard, the policy, and the handlers return
  `policy.Stop` in separate items, and each item has to leave the module compiling and its tests passing, so `classify`
  read both a stop and the old integers until the last item deleted the integers. The bridge was a map of five entries,
  deleted with the constants. Routing candidate: none; it is the transitional shape the plan's item split implies.
- (2026-10-06, Unit 3) **A stop that carries no error needs a name of its own, `policy.BareStop`.** The plan says every
  stop is built by `policy.Stopped`. v0.8.4 returned a status beside a `nil` error for a deferral or a shed, and callers
  across the guard, the application, and the test driver read `err == nil` as "nothing went wrong beyond this status". A
  stop whose cause is `nil` must keep that reading: `Application.Run` still returns a `nil` error beside it,
  `promoteFinalize` and the release defers let a finalize or release error replace it, and `endedByInterruption` treats
  it as no error. A stop with a cause (a shed whose child could not be confirmed stopped) stands against a finalize
  error, as the `(74, stopError)` it replaces did. `policy.BareStop` and `carriesNoError` name the rule once; the plan's
  wording "every stop" would have lost the shed's unconfirmed-retirement error. Routing candidate: `tech-docs/001`'s
  PW-3 paragraph. **Routed** (2026-10-06, Unit 3 close) to `tech-docs/001-domain-types.md`, Finalize precedence.
- (2026-10-06, Unit 3) **`Reason.UnmarshalJSON` and `ShedCause.UnmarshalJSON` are strict and read every `uint8` for its
  integer.** The plan names the legacy codecs; it does not say how a decode refuses what is not a member. Both decode an
  integer only if some member's legacy integer equals it, so `74` in a ledger's `sheddingExitCode`, `1`, `-73`, `73.5`,
  and a string all fail at decode and the ledger fails closed with its bytes preserved. The loop over `uint8` keeps the
  integer table in one `switch` each, so a member added later is read back without a second table. Routing candidate:
  `tech-docs/001`'s legacy-codec paragraph. **Routed** (2026-10-06, Unit 3 close) to `tech-docs/001-domain-types.md`,
  Codecs that keep integers.
- (2026-10-06, Unit 3) **File impact additions.** Beyond `tech-docs/004-file-impact.md`'s Unit 3 list, the unit also
  touched `internal/cli/application.go` (its final branch reads `noErrorBeyondItsReason`, so a bare stop still returns a
  `nil` error). The list's `internal/guard/run.go` entry names `callerShedCode`; the function was `callerShedReason`.
  `tests/support/loaded_gate.go` needed a test-local constant for the near-miss deferral status `75`, which was never an
  exit status. Routing candidate: `tech-docs/004-file-impact.md`, Unit 3. **Routed** (2026-10-06, Unit 3 close) to
  `tech-docs/004-file-impact.md`, Unit 3, and the function's name to `tech-docs/001-domain-types.md`.
- (2026-10-06, Unit 3) **A fixed `go test` default timeout of 10 minutes fails the integration adapter under host
  load.** At load averages of 27 to 44 the integration package exceeded Go's default 10 minutes on the first Unit 3 run
  and passed with `-timeout 45m`; the unit adapter took 524 s. The failure was the timeout, not a test. Routing
  candidate: the Unit 2 learning on the race step's timeout; `scripts/test.sh`.
- (2026-10-06, Unit 4) **A type that flows through a struct cannot be introduced in two steps, so the GREEN and REFACTOR
  of the profile items collapse.** The RED tests read `Resolution.ResolvedProfile` as a `ProfileName`; that forces
  `Resolution`, `Catalog`, and `Profile` to carry it, and from there every guard type and parameter a resolution's names
  are copied into (`EvidenceSummary`, `ReservationOwner`, `reservationWaiter`, `ReservationEntry`, both
  `AcquireReservation` parameters), the configuration file's names, and the monitor's JSON. The module compiles only
  when all of them are typed, so the item that adds `ProfileName` also removed all 14 Unit 4 allowlist entries (the
  analysis reported each as stale), `Profile.DegradedAdmission`, and the hand-written share table, and the REFACTOR item
  had nothing left but its acceptance runs. Routing candidate: `delivery.md`, Unit 4, as a note that its GREEN carries
  the REFACTOR's edits; the same holds for any later unit that types a field the tests read.
- (2026-10-06, Unit 4) **`go test -run DomainLiteral ./tests/support` does not run the ratchet; the unit adapter does.**
  Those tests exercise the analysis against fixtures and pass whatever the allowlist holds. The scenario "Production
  code compares no domain value with a literal" runs the analysis over the module and is what reported
  `domain literal analysis found 14 problems` after the profile types landed. A unit that deletes allowlist entries, or
  adds a violation, needs `go test -count=1 -run 'TestUnitBehaviours/Production_code_compares' ./tests/unit` (about
  three seconds) beside the finishing gates the coordinator lists. Routing candidate:
  `tech-docs/002-gates-and-analysis.md` and the finishing-gate list in each unit's task.
- (2026-10-06, Unit 4) **Decisions the plan left open.** An unset lineage answers `false`, `false`, and `1` (no degraded
  admission, no floor, and the one share v0.8.4 gave a name it did not know), and `Resolve` refuses a profile that
  carries it, so only a hand-built `Resolution` in a test can reach the `1`. The third row of the floor outline needs a
  profile with no fallback, which the configuration already allows as `"fallback":""`; the driver's
  `derivedProfileWithoutFallback` writes it in this unit, ahead of Unit 5's note that the configuration writer gains the
  `fallback` override. The built-in row of "Stable warning spares an ephemeral child of the balanced lineage" has its
  own step text ("admitted on healthy Darwin samples"), so the unchanged "Unsafe pressure still sheds..." outline keeps
  its step. Routing candidate: `tech-docs/001-domain-types.md`, Profile Identity and Lineage, and
  `tech-docs/003-specification-changes.md`.
- (2026-10-06, Unit 4) **File impact additions.** Beyond `tech-docs/004-file-impact.md`'s Unit 4 list, the unit also
  touched `tests/integration/lease_evidence_test.go` and `tests/integration/run_test.go` (their `FallbackChain` literals
  are `[]policy.ProfileName`), `tests/support/blockers_v04.go` and `tests/support/pending_v04.go` (share maps keyed by
  `ProfileName`), and `tests/support/review_v04.go`, whose corrupt-ledger scenario kept its corruption kind in the
  driver's `requestedProfile` and now has a `corruptionKind` field. The no-fallback row hands the public command line
  the configuration text the driver wrote (`Driver.configDocument`) instead of reading the file back, because `gosec`
  G703 flags the write in `runGuardedAtBoundary` once its content comes from a file read. Routing candidate:
  `tech-docs/004-file-impact.md`, Unit 4. **Routed** (2026-10-06, Unit 4 close) to `tech-docs/004-file-impact.md`,
  Unit 4.
- (2026-10-06, Unit 4) **The degraded-admission scenarios use a 100 ms real-time window and fail under heavy host
  load.** At load averages near 45 they failed twice and passed on rerun; the fixture predates this plan. Routing
  candidate: an idea brief to make the fixture's clock injectable, since a flaky gate on a tool every repository on the
  workstation runs is a release risk.
- (2026-10-06, Unit 4) **"Cancelled FIFO waiters use a fresh cleanup deadline" flakes under race and load; the cause
  predates this plan.** The step frees the coordination lock 20 ms after cancelling, and the production cleanup gives
  itself a fresh 100 ms (`coordinationLifecycleWait`) to take that lock. When the scheduler delays either side, the
  cleanup falls back to removing the waiter in the background after the step has already read the ledger. A repro of the
  step under `-race` with CPU burners failed 17 and 21 of 300 trials on both `892c462` (pre-plan) and this unit's tree,
  and `coordination.go` is byte-identical to `892c462`. The lock wait also has a hazard of its own: after a stall,
  `select` may pick `ctx.Done()` over a ready poll and refuse a free lock, which the process gate at `coordination.go`
  already guards against. Not fixed here because it is outside the plan's contract. Routing candidate: an idea brief for
  an injectable cleanup wait on `ReservationAdmissionOptions` (default unchanged; the scenario sets about 2 s and keeps
  its strict assertion) and a lock-wait check of the lock before honouring the deadline. **Routed** (2026-10-06, at the
  owner's direction) to the bug-fix plan `fix-cancelled-waiter-cleanup-flake`; the degraded-admission flake gets its own
  bug-fix plan.
