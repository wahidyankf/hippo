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
  **Discarded** (2026-10-06): `scripts/test-loaded.sh` already raises the timeout through `GOFLAGS`, keeping the shared
  gate as CI runs it; see Phase 4a.
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
  candidate: the Unit 2 learning on the race step's timeout; `scripts/test.sh`. **Discarded** (2026-10-06):
  `scripts/test-loaded.sh` already raises the timeout through `GOFLAGS`, keeping the shared gate as CI runs it; see
  Phase 4a.
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
- (2026-10-06, Unit 5) **Decisions the plan left open in the decision function and its callers.** (a) The window is
  checked after steps 1 and 2, as the design lists them, so a resolution already at replan or cleanup decides its own
  path even under `WindowUnset`; an unset or unknown window is an error only once the samples have to be read. (b) A
  resolution at `wait`, which `Resolve` never returns, is decided by its samples, like one at `run`. (c) `Run` has no
  outcome for a replan, which the command line never hands it (`cli.run` stops on any resolution with a reason before
  `guard.Run`); the loop records `admission-failed`, "a run HIPPO itself stopped after host sampling began and before
  its child started", and stops with `ReasonReplanRequired`. The branch is pinned by a new guard test, written RED
  first, `TestARunWhoseResolutionAlreadyStopsNeverLaunches`. (d) A path left unset, which only an error from
  `DecideAdmission` produces, is `hippo.supervision.failed` in both `run` and `status`, and neither can reach it: both
  pass a fixed window. (e) `withAssessmentDecision` returns `(policy.Resolution, error)`, with an unset path an error,
  so no caller silently defaults; the decision for `status` moved into a helper, `decideStatus`, because `status` grew
  to 127 lines under `funlen`'s 120. Routing candidate: `tech-docs/001-domain-types.md`, the Unit 5 section.
- (2026-10-06, Unit 5) **Unit 5 removes no allowlist entry and adds none.** After Unit 4 the allowlist holds only Unit 6
  entries, so the plan's note "remove Unit 5's entries if any become unnecessary" had nothing to remove; the new types
  (`AdmissionPath`, `EvidenceWindow`) are introduced typed, and `AdmissionPath` is on the analysis's name list, so a raw
  field of that name would have failed. The ratchet scenario still passes (it runs in the unit adapter). Routing
  candidate: none; `tech-docs/002-gates-and-analysis.md`'s removal table already lists no Unit 5 row.
- (2026-10-06, Unit 5) **A RED in `internal/cli` breaks the unit adapter the way one in `internal/guard` does.** The
  Unit 2 learning names `internal/guard` and `internal/policy`; `tests/support/blockers_v04.go` also shells out to
  `go test` for `./internal/cli` (three regressions), `./internal/conformance`, `./tests/integration`, and
  `./tests/unit` (`runGoRegressionV10`). A CLI RED that does not compile, and any edit to those packages while an
  adapter run is in flight, contaminates the run. Unit 5 ran each adapter only after the GREEN or REFACTOR that restored
  compilation, and edited Go files only between runs. Routing candidate: the same note as the Unit 2 learning, in
  `delivery.md`'s "Commands the items name".
- (2026-10-06, Unit 5) **The first ten compile errors of a RED do not name the function the acceptance names.**
  `go test ./tests/unit` stops after ten errors, which are the new types and constants (`policy.EvidenceWindow`,
  `policy.AdmissionInput`, ...); `policy.DecideAdmission` shows only with `-gcflags=-e` (99 undefined references, 9 of
  them `DecideAdmission`). The RED still fails for the stated reason. Routing candidate: none.
- (2026-10-06, Unit 5) **AC-14's fourth row had no test at the command boundary.** The status table
  `TestStatusJSONKeepsEachResolutionsV084DecisionAndExitCode` held three of AC-14's four rows (normal, a stable warning,
  a blocked disk); "a strict profile that does not fit" was reached only through `withAssessmentDecision` in the plan's
  new table. Unit 5 adds the row to the status table through `status --json --config`, with a configured strict profile
  that does not fit; it passes on the unchanged `origin/main` code, which is the point of a regression row. Routing
  candidate: none.
- (2026-10-06, Unit 5) **File impact additions.** Beyond `tech-docs/004-file-impact.md`'s Unit 5 list, the unit also
  touched `internal/guard/run_test.go` (the already-stopped resolution test above). The list's
  `specs/behaviours/admission.feature` needs no edit (bindings only, as listed), and the adapter module's
  `exhaustruct_v5` entry belongs to the Unit 5 close. Routing candidate: `tech-docs/004-file-impact.md`, Unit 5.
- (2026-10-06, Unit 5) **Logs kept in the shared scratchpad root are not safe across sessions.** Another session's
  cleanup deleted the first adapter run's log while it ran, so its result was unknown and the run was repeated whole
  (about 8 minutes at a load of 13 to 45). Later logs went under a subdirectory of the scratchpad unique to this task.
  Routing candidate: the coordinator's task template (a per-task scratch subdirectory), not this repository.
- (2026-10-06, Unit 5) **A toolchain fault failed one unit scenario once.** In the first `npm run test:quick` of the
  unit, "Release builds use only exact committed source" failed with
  `package runtime is not in std (/opt/homebrew/Cellar/go/1.27.1/libexec/src/runtime)` while the other 345 scenarios
  passed; the same scenario passed alone on rerun, and the directory is present. The cause is outside the repository
  (the Go installation, on a host other sessions share), and the scenario has no connection to admission. The gate was
  rerun whole. Routing candidate: none.
- (2026-10-06, Unit 5) **The driver now refuses a replan or cleanup resolution it used to admit degraded.** The old
  `assessAdmission` admitted an ephemeral task of the balanced lineage degraded under a stable warning without reading
  the resolution's decision, so a resolution already at replan or cleanup (a reason other than none) was admitted at
  concurrency one; `DecideAdmission` lets such a resolution decide its own path first, so the driver refuses it, as
  `cli.run` already does before `guard.Run`. No scenario relied on it: the 24 frozen rows pass at both adapters before
  and after. The change is observable only to a future scenario that pairs an unfitting or storage-blocked resolution
  with a stable warning. Routing candidate: `tech-docs/001-domain-types.md`, the Unit 5 driver paragraph (one sentence
  saying the driver now agrees with the command line on a resolution that already stops).
- (2026-10-06, Unit 5) **A fixture that never admits spins at full CPU through its one-hour window.** Under a break that
  never admits, `execution.feature:130` ("Worsening warning") runs `Run`'s admission loop with a no-op `Sleep` and an
  admission window of one hour (`evidenceDecidesAdmission`), so the loop samples as fast as the CPU allows until the
  window closes or the run is killed (it was killed after 11 minutes in the Gherkin review). A break that admits at
  once, or the correct code, never notices; only a mutation that withholds admission does, and it also hides every
  scenario behind it. Routing candidate: an idea brief to bound such fixtures (a sample budget or a test sleep that
  advances the injected clock, so a window the evidence never decides closes in a few iterations).

- (2026-10-07, Unit 6) **A defined string has no member list, so `policy.TaskClasses()` is hand-kept and a test holds
  it.** `Outcome` derives its list from an `iota` range (Unit 2); `TaskClass` is `type TaskClass string`, so a class
  declared and not listed would be refused at decode and absent from the `history --class` message with nothing failing.
  `tests/unit/task_class_test.go` parses `internal/policy/profiles.go` and compares every `TaskClass` constant with
  `TaskClasses()` (planting `TaskBatch TaskClass = "batch"` failed it naming both lists, then was reverted). Routing
  candidate: `tech-docs/001-domain-types.md`, the Unit 6 strict paragraph (one sentence: the list is hand-kept and
  pinned by the AST test).
- (2026-10-07, Unit 6) **As built, `RecordedTaskClass` reports an unknown class as "no member", not as a fifth value.**
  A struct of the member and the recorded text, as the plan says: `RecordedClass(class)`,
  `TaskClass() (TaskClass, bool)`, `String()`, `IsZero()` (with `omitzero`, so an unset class is omitted as before),
  `MarshalText`, and an `UnmarshalText` that never fails and keeps the text of any class, member or not. The strict and
  tolerant readers share `ParseTaskClass`, and `TestStrictAndTolerantReadersAgreeOnWhatIsAMember` holds them to one
  membership rule. `Query.Class` is a plain `policy.TaskClass` parsed strictly from `--class`, and `matchesClass` never
  selects a row whose class has no member. Routing candidate: `tech-docs/001-domain-types.md`, the Unit 6 tolerant
  paragraph (the as-built API in one sentence).
- (2026-10-07, Unit 6) **The strict `TaskClass.UnmarshalText` guards every decoded `TaskClass` field, not only the
  ledger's.** `ReservationEntry`, `SafetyReceipt`, and the writer's `EvidenceSummary` also decode through it. None is
  read from a file a later version wrote (the summary and receipt are decoded only in tests, with known classes, and the
  full suites pass), but a future reader of `EvidenceSummary` from disk must use `RecordedTaskClass` or it will refuse a
  later version's evidence. Routing candidate: `tech-docs/001-domain-types.md`, the D6 note ("recorded evidence is read
  by the `Recorded*` types"), as the Unit 2 reviewer's F4 already asked for the outcome codec.
- (2026-10-07, Unit 6) **Release scenarios copy `git ls-files --cached --others`, so a deleted tracked file fails 24
  scenarios until the deletion is staged.** With `domain_literals_allowlist.go` removed on disk and still in the index,
  the unit adapter failed 24 release scenarios with
  `copy fixture entry: lstat .../domain_literals_allowlist.go: no such file or directory`; after `git rm --cached` it
  passed. Any unit that deletes a tracked file and runs an adapter before committing has the same failure. Routing
  candidate: the starting commands or the GREEN item of a deleting unit (stage the deletion before the adapter runs),
  and the coordinator's task template.
- (2026-10-07, Unit 6) **A scenario with two refusing layers passes under either break alone.** "Reservation ledger
  classes are validated before mutation" passes with decoding broken alone (validation still refuses `batch`) and with
  validation broken alone (decoding still refuses it), and fails only with both. The member half of
  `validReservationClass` is reachable through bytes, as the first form of this entry wrongly said it was not: an owner
  or waiter whose `class` key is absent or null never reaches `UnmarshalText`, arrives at validation as the empty class,
  and only that half refuses it (exit `125`, `reservation ledger owner class is invalid`). The review (MEDIUM-2) added
  the absent-class and null-class rows, owner and waiter, to `TestReservationLedgerClassIsRefusedWhereItIsDecoded`; they
  fail when the member half is dropped (the ledger is accepted). Routing candidate: none; the Gherkin review records the
  decision (the scenario states the contract, the Go tests pin each layer).
- (2026-10-07, Unit 6) **`validReservationClass` names every member in a switch, so the member list is not the whole of
  the plan's REFACTOR.** The REFACTOR item derived the accepted set from one list shared with `validReservationClass`,
  which made the check "any member but release": a class added to the constants and the list would have been admitted to
  every ledger silently. The review (MEDIUM-3) replaced it with an exhaustive `switch` over `TaskClass` (ephemeral,
  service, and transactional reserve; release and any other class do not), so the `exhaustive` linter reports a class
  added without a decision here (planting `TaskBatch` failed `exhaustive` at the switch, and the test, which names every
  member in a map the linter also checks, failed with the class refused). The decoder and the history filter keep
  `TaskClasses()`, so the shared list is unchanged for them and only the ledger check stopped reading it, a deliberate
  deviation from the item's wording. `TestAReservationHoldsExactlyTheClassesThatReserve` replaced
  `TestReservationClassIsEveryMemberButReleaseAndNothingElse`. Routing candidate: `tech-docs/001-domain-types.md`, the
  Unit 6 strict paragraph (one sentence: the ledger check is a closed switch, not a derivation from the list).
- (2026-10-07, Unit 6) **A refusal clause pinned by no test can hide behind a second layer that says the same words.**
  `runClass`'s `release` refusal was removed in the review's mutation and every test still passed, because the guard
  refuses the class with the same message (`class must be ephemeral, service, or transactional`) under a different code,
  `hippo.supervision.failed` and exit `125`, where the caller's mistake is `hippo.args.invalid` and exit `2`; only a
  test that reads the code and status sees the difference, and none did, at `HEAD` either. The review (MEDIUM-1) added
  the `run --class release` row to the usage-mistake scenario's `invalidFlagValues` (which fails at the unit and
  end-to-end adapters without the clause) and `TestRunClassAcceptsOnlyTheClassesRunMayGuard`. Routing candidate: none.
- (2026-10-07, Unit 6) **The strict decodes changed the text of diagnostics for inputs that were already refused.** No
  exit status and no `hippo.*` code changed, and the messages are not a contract, but a reader comparing v0.8.4 with
  v0.8.5 output sees these (measured by running a binary built from `HEAD` and one from the tree over the same
  documents): a schema 2 document with `"mode": "exclusive"` read `unsupported schema 2 coordination mode "exclusive"`
  and reads `unsupported coordination mode "exclusive"` (`internal/config/config.go:85`); a schema 1 document with an
  unknown mode read `schema 1 does not support reservation coordination` and now reports the mode error (with
  `"mode": "reservation"` it reads as before); a document with an unknown mode and another error, such as a profile that
  extends a missing one, read the other error and now reads the mode error, because decoding runs first; and a
  non-string value names the Go type, `of type config.coordinationMode` where it read `of type string`, and likewise
  `Summary.taskClass of type policy.RecordedTaskClass` in `history`, each now followed by
  `JSON value must be string type` (`hippo.config.unreadable` and `hippo.evidence.unreadable`, both exit `125`, as
  before). Routing candidate: a candidate for the `v0.8.5` `CHANGELOG.md` wording in Unit 7, as one line under `Changed`
  or `Fixed` (the text of an unsupported coordination mode and of a non-string `taskClass` changed; exit statuses and
  codes did not).
- (2026-10-07, Unit 6) **"Legacy schema-one PID-only ownership remains conservative" is the one scenario that pins the
  tolerant lease read.** Its seeded lock records the class `heavy`, which `TaskClass` has no member for, so a lease
  owner that refused unknown classes fails its `heavy` row at both executing adapters (the `service` row passes). The
  same break is what the mutation item observed. Routing candidate: none.
- (2026-10-07, Unit 6) **A decode-time proof needs a second, later failure.** The coordination-mode RED pairs a bad mode
  with a profile that extends a missing one: only a refusal at decode can name the mode, because the profile error would
  fire first after decoding. Without the pairing, the old post-decode comparison also passes the test. Routing
  candidate: `tech-docs/001-domain-types.md`, the Unit 6 tests paragraph.
- (2026-10-07, Unit 6) **The plan's whole-package RED commands were run in focused form.** Each RED item names a package
  (`./tests/unit`, `./internal/evidence`), but the failing test is one function and a whole `./tests/unit` run is about
  4 to 6 minutes at a load of 15 to 30; the Results record the focused `-run` form, and the whole package ran at each
  GREEN and REFACTOR. The ratchet-end RED (change the `Then`, rebind) exits `0` by design: the allowlist is already
  empty, and the grep that prints the declaration and the reading code is the failing state. Routing candidate: none.
- (2026-10-07, Unit 6) **The allowlist's `Symbol` keying served only the allowlist.** With the list gone, the `Symbol`
  field, `symbol()`, `functionSymbol`, the walker's node stack, and the `owner` and `symbol` parameters are dead, so
  they are deleted with the stale-entry fixture, which the plan's REFACTOR item does not name; a smaller
  `TestDomainLiteralStepReportsEveryFindingAndNothingElse` replaces the fixture. Routing candidate:
  `tech-docs/002-gates-and-analysis.md`, the ratchet section (no allowlist and no symbol at the end).
- (2026-10-07, Unit 6) **The GREEN of the strict-class item could not stop at one line.** Typing `leaseOwner.Class`,
  `Summary.TaskClass`, and the two flags changed every comparison on them, so the item that adds the type also rewrote
  the call sites that compared a class with a string (`runOptions.class`, `runClass`, `classFilter`); the Results of
  those items record it, and the REFACTOR items that follow are the cleanups. The `...Flag` suffix on
  `historyOptions.classFlag` and `runOptions.classFlag` is what keeps raw CLI text outside the domain name list, as the
  analysis's header says. Routing candidate: none.
- (2026-10-07, Unit 6) **File impact additions.** Beyond `tech-docs/004-file-impact.md`'s Unit 6 list the unit also
  touched `internal/cli/development.go` (`runClass`), `internal/guard/exclusive_status.go` (publishes a legacy owner's
  class as recorded), `internal/guard/run_test.go`, `internal/cli/history_test.go`, `internal/cli/development_test.go`
  (the unreadable coordination mode through the command), `tests/integration/lease_evidence_test.go` (the lease owner's
  class), `tests/support/domain_literals_internal_test.go`, `docs/reference/cli.md`, `specs/architecture.md`, and
  `repository-adapter/README.md`, and added `tests/unit/task_class_test.go` and
  `internal/guard/reservation_class_test.go`. Routing candidate: `tech-docs/004-file-impact.md`, Unit 6.
