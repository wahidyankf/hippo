# Domain Types

One section per delivery unit that introduces a type. Each names the members, the codec, the call sites it replaces, and
the tests that pin it. Line numbers are `892c462`'s; re-read each file before editing it.

## Run Outcome (Unit 2)

**Type.** `evidence.Outcome`, a `uint8` in `internal/evidence/outcome.go`, because `internal/guard` writes it,
`internal/evidence` reads it, and `internal/evidence` imports no other HIPPO package. Members, in this order:
`OutcomeUnset` (zero), `OutcomePassed`, `OutcomeTaskFailed`, `OutcomeSupervisionFailed`, `OutcomePressureShed`,
`OutcomeStorageShed`, `OutcomeEmergencySafetyStop`, `OutcomeCapacityDeferred`, `OutcomeStorageBlocked`,
`OutcomeAdmissionCancelled`, `OutcomeAdmissionFailed`, and `OutcomeUnknown`, which only a reader produces.

**Codec.** One exhaustive `switch` in `Outcome.MarshalText` returns the ten `v0.8.4` wire strings (`passed`,
`task-failed`, `supervision-failed`, `pressure-shed`, `storage-shed`, `emergency-safety-stop`, `capacity-deferred`,
`storage-blocked`, `admission-cancelled`, `admission-failed`) and an error for `OutcomeUnset` and `OutcomeUnknown`.
`Outcomes()` returns the ten writable members in that order; `guard.RunOutcomes` is deleted and
`internal/cli/history.go` lists `evidence.Outcomes()` instead. `ParseOutcome` is strict and serves `history --outcome`.

**Tolerant reader.** `evidence.RecordedOutcome` is a struct of the parsed `Outcome` and the recorded text. Its
`UnmarshalText` never fails: a known string yields its member, any other yields `OutcomeUnknown` with the text kept.
`MarshalText` writes the recorded text back, so `history --json`, `--jsonl`, and the text listing show exactly what was
recorded, and `IsZero` reports an empty recording so the reader's `omitzero` tag omits it as `omitempty` did.
`evidence.Summary.Outcome` becomes a `RecordedOutcome`; `Query.Outcome` becomes an `Outcome`, with `OutcomeUnset`
meaning no filter. `healthyPromotionSummary` (`internal/evidence/history.go`, line 211) asks for `OutcomePassed`, which
an unknown recording never is.

**Budget outcome.** `evidence.BudgetOutcome`, a `uint8` with `BudgetOutcomeUnset` (zero) and `BudgetOutcomeAdmitted`,
whose `MarshalText` writes `admitted`; its reader form keeps unknown text the same way. The two comparisons with
`"pressure-shed"` and `"storage-shed"` (`history.go`, line 217) are deleted: no writer has ever produced either, and a
shed run already fails the `OutcomePassed` check beside them, so owner promotion decides as before. This is the recorded
answer to PW-4 ([delivery](../delivery.md#post-write-gate)): the deletion holds only if no released HIPPO version ever
wrote either value, which the Unit 2 proof item checks against every release tag before `BudgetOutcome` is typed. If it
finds one, the comparisons stay, typed, and `BudgetOutcome` gains a member for each value found.

**Writer.** `run.go` declares `var outcome evidence.Outcome` at line 810, so it starts `OutcomeUnset`. The deadline
deferral that relied on the old default (line 934) assigns `OutcomeCapacityDeferred` explicitly, and the 22 other
assignments name members. `EvidenceWriter.Finalize` and `SetReservationContext` (`internal/guard/evidence.go`, lines 59
and 232) take the types, and `EvidenceSummary.Outcome` and `BudgetOutcome` are typed.

**Unset at finalize.** A pure function `finalOutcome(evidence.Outcome) (evidence.Outcome, error)` in `run.go` returns
`OutcomeSupervisionFailed` and a `status.Failure` naming `hippo.supervision.failed` for `OutcomeUnset`, and the member
unchanged otherwise. The `finalize` closure records its result and returns the failure with its other errors.

**Promotion.** The deferred promotion (`run.go`, lines 836–842) becomes a pure function,
`promoteFinalize(exitCode int, returnError, finalizeError error, launched bool) (int, error)`, which the deferred
closure only calls. No finalize error, or an error the run already returned, leaves the exit code and error as they are.
Otherwise the finalize error becomes the run's error with exit code `1`, as a refused evidence write named by
`refusedEvidenceWrite` when nothing launched, as today. The failure `finalOutcome` returns for an unset outcome
therefore reaches the caller carrying `hippo.supervision.failed`, which `status.Status` maps to `125`. Tests call
`promoteFinalize` directly because every path in `Run` assigns an outcome, so an unset one cannot be reached through
`Run`; no hook is added to `RunConfig`. This is the recorded answer to PW-6
([delivery](../delivery.md#post-write-gate)).

**Tests.** `internal/evidence/outcome_test.go` (codec table, parse, tolerant round trip), `internal/guard/run_test.go`
(`finalOutcome`, `promoteFinalize`, and the deadline deferral still summarized `capacity-deferred`), and
`tests/unit/outcome_vocabulary_test.go`, which holds the outcome list in `docs/reference/json-schemas.md` (line 260) to
`evidence.Outcomes()` the way `tests/unit/vocabulary_test.go` holds the code table.

## Internal Reason (Unit 3)

**Type.** `policy.Reason`, a `uint8` in `internal/policy/reason.go`: `ReasonNone` (zero, no stop),
`ReasonStorageBlocked`, `ReasonCapacityDeferred`, `ReasonPressureShed`, `ReasonProtocolMismatch`, and
`ReasonReplanRequired`. It replaces `StorageBlockedExitCode`, `CapacityDeferredExitCode`, `PressureShedExitCode`
(`run.go`, lines 22–33), `ReplanRequiredExitCode`, and `ProtocolMismatchExitCode` (`profiles.go`, lines 13 and 15),
which are deleted with the bare `73` at `profiles.go`, line 260.

**Carriage.** A stop travels as an error, `*policy.Stop{Reason, Cause}`, built by `policy.Stopped`, never as an integer:
`return CapacityDeferredExitCode, nil` becomes `return 0, policy.Stopped(policy.ReasonCapacityDeferred, nil)`, and
`return policy.ReplanRequiredExitCode, resolveError` becomes
`return 0, policy.Stopped(policy.ReasonReplanRequired, resolveError)`. The integer a handler returns then carries only
`0`, `1`, `2`, or a child's status. This is the recorded answer to PW-3 ([delivery](../delivery.md#post-write-gate)).

**One boundary switch.** In `internal/cli/status.go`, `classify` finds a `*policy.Stop` with `errors.AsType` and maps
its reason through `reasonCode(policy.Reason) status.Code`, one exhaustive `switch` with no `default`; `status.Status`
already turns each code into `124` or `125`. `internalReasons` is deleted. `reasonMessage` switches over `policy.Reason`
instead of `status.Code`, also without a `default`, so its `//nolint:exhaustive` is deleted. `ReasonNone` inside a
`Stop` is a fault and reports `hippo.supervision.failed`.

**Finalize precedence.** `promoteFinalize` replaces a nil error with a refused evidence write today, and a deferral
returned `nil`. A deferral is now an error, so the promotion treats a `*policy.Stop` as it treated `nil`: a deferral
whose summary write fails still reports the refused write, as today. A unit test pins it.

As built in Unit 3 (2026-10-06): only a stop that carries no error, `policy.BareStop`, is treated as `nil`, through
`carriesNoError`; this holds in `promoteFinalize`, the ownership- and port-release defers, `endedByInterruption`, and
`Application.Run`, which still returns a `nil` error beside it. A stop with a cause, such as a shed whose child could
not be confirmed stopped, stands against a finalize error, as the `(74, stopError)` it replaces did. "Every stop" would
have lost that error.

**Codecs that keep integers.** Two wire values are integers and stay so:

- `policy.Resolution.ExitCode int` becomes `Reason policy.Reason` with the JSON name `exitCode`, encoded by
  `Reason.MarshalJSON` through one exhaustive `switch` that names each member's `v0.8.4` integer: `0` for none, `73`
  storage blocked, `74` pressure shed, `75` capacity deferred, `76` protocol mismatch, `78` replan required. Status JSON
  therefore keeps publishing `profile.exitCode` (`0`, `73`, `75`, or `78`) as `v0.8.4` does.
- `ReservationOwner.SheddingExit int` (`reservation.go`, line 120) becomes `SheddingCause guard.ShedCause` with the JSON
  name `sheddingExitCode`: `ShedCauseNone` (zero, omitted), `ShedCauseStorage` (`73`), and `ShedCausePressure` (`75`).
  Its `UnmarshalJSON` accepts `0`, `73`, and `75` and refuses anything else, the set `validSheddingExitCode` (line 1618)
  accepts today; that function and `callerShedCode` (`run.go`, line 1168) are deleted, and `ShedCause.Reason()` maps
  storage to `ReasonStorageBlocked` and pressure to `ReasonPressureShed` in one exhaustive `switch`.

As built in Unit 3 (2026-10-06), both decoders are strict the same way: `Reason.UnmarshalJSON` and
`ShedCause.UnmarshalJSON` accept an integer only when some member's legacy integer equals it, by asking every `uint8`
value, so each integer table stays one `switch`. `74` in a ledger, `1`, `-73`, `73.5`, and a string fail at decode, and
the ledger fails closed with its bytes kept. The guard function was named `callerShedReason`, not `callerShedCode`.

**Tests.** `tests/unit/reason_test.go` (legacy integers; `tests/unit` is the run that measures `internal/policy`'s 99%
coverage), `tests/unit/reservation_test.go` (ledger codec), and a new `internal/cli/status_test.go` (reason to code to
status, every member), plus the existing contract scenarios at all three boundaries.

## Profile Identity and Lineage (Unit 4)

**Types.** `policy.ProfileName`, a defined `string` for the open set of profile names, replaces `string` wherever a
profile name is held: `Catalog.DefaultProfile` and its map keys, `Profile.Name` and `Fallback`, `Resolution`'s
requested, resolved, and fallback-chain names, `ReservationOwner.Profile` and `reservationWaiter.Profile`,
`ReservationEntry.Profile`, the evidence summaries' profile fields, and the guard and command-line parameters that pass
one. Its JSON is the same string.

`policy.Lineage`, a `uint8`: `LineageUnset` (zero), `LineageBalanced`, `LineageConstrained`, `LineageMinimal`. Each
answers three questions in one exhaustive `switch` apiece: `DegradedAdmission()` (balanced only), `LastResortFloor()`
(minimal only), and `DefaultOwnerShares()` (4, 2, 1). `Profile.Lineage` (JSON `-`) replaces `Profile.DegradedAdmission`;
`BuiltinCatalog` sets it on each built-in, and `buildCatalog` already copies the parent profile, so a configured profile
inherits it through any depth of `extends` and no configuration key can set it. `Resolution.Lineage` (JSON `-`) is
copied by `Resolve`; `Resolution.DegradedAdmission` keeps its JSON field, now set from `Lineage.DegradedAdmission()`.
`Resolve` refuses a profile whose lineage is `LineageUnset`.

**Rules that change.**

- `Resolve`'s floor (`profiles.go`, line 266) reads `profile.Lineage.LastResortFloor()` instead of
  `current == profileMinimal`. A configured profile that extends `minimal`, directly or through another configured
  profile, and does not fit now resolves to itself with the relaxed admission memory and disk thresholds, where today it
  falls through to "resource profile has no usable fallback" and exit `125`. This is the D5 fix.
- The guard's owner-share default (`reservation.go`, lines 263–273) reads `resolution.Lineage.DefaultOwnerShares()`.
- `reservationCoordination`'s default share table (`config.go`, lines 117–121) is built from the built-ins'
  `DefaultOwnerShares()`, so the 4, 2, 1 table lives once.

**Conditions.** A configured profile named `minimal` without `extends` starts from the built-in and keeps the floor, as
its name gave it before. One named `minimal` with `extends: balanced` loses the floor; its inherited fallback then
reaches the configured `minimal` again, which `buildCatalog`'s fallback-cycle check already refuses at load. Strict
classes never take the floor, unchanged.

**Driver.** `tests/support` stops calling `policy.BuiltinCatalog()` (`driver.go`, lines 449, 477, and 2589;
`degraded_lineage.go`, lines 189 and 214). A helper resolves the scenario's configuration through `config.Load`, or the
defaults `config.Load` returns with no file, so every profile-sensitive scenario resolves the way the command line does.

**Tests.** `tests/unit/policy_test.go` (each lineage answer, and the floor for a `minimal`-derived catalog),
`tests/unit/reservation_test.go` (the owner-share default with an empty share map), and the scenarios in
[specification changes](003-specification-changes.md).

## Admission Path and the Single Decision (Unit 5)

**Types.** `policy.AdmissionPath`, a `uint8`: `AdmissionUnset` (zero), `AdmissionNormal`, `AdmissionDegraded`,
`AdmissionWait`, `AdmissionCleanup`, `AdmissionReplan`. `policy.EvidenceWindow`, a `uint8`: `WindowUnset` (zero),
`WindowSnapshot` (the two samples `status` takes), `WindowSampling` (the samples `run` collects).
`policy.AdmissionInput` is the struct `{Resolution, TaskClass, Samples, Policy, Window}`, and `exhaustruct_v5` refuses a
literal that omits a field. `Policy` is the resource policy the samples are assessed and admitted against, carried apart
from `Resolution` because callers set the two independently today: `run` admits against `RunConfig.Policy`, which the
behaviour driver sets to a fast or tuned policy beside a separate or zero `Resolution`, while `status` and the driver's
`assessAdmission` admit against `Resolution.Policy`. Reading `Resolution.Policy` inside the function would move
admission for the first case, which D7 forbids.

**Function.** `policy.DecideAdmission(AdmissionInput) (AdmissionPath, error)`, in order:

1. the resolution's decision `replan` gives `AdmissionReplan`, and `cleanup` gives `AdmissionCleanup`;
2. a storage-blocked assessment gives `AdmissionCleanup`;
3. under `WindowSnapshot`, a normal state gives `AdmissionNormal`, and anything else `AdmissionWait`;
4. under `WindowSampling`, `AdmissionReady` gives `AdmissionNormal`; an ephemeral task whose lineage allows degraded
   admission, with `WarningAdmissionReady`, gives `AdmissionDegraded`; anything else gives `AdmissionWait`;
5. `WindowUnset` is an error.

The snapshot rule is exactly today's `status` rule, so status JSON keeps `v0.8.4`'s decision for every host state;
`status` never reaches `AdmissionDegraded`, because two samples cannot fill the warning window. The stable-warning shed
exemption in the supervision loop (`run.go`, lines 1069–1071) calls
`policy.SparesStableWarning(class, resolution, samples, policy)`, which shares the eligibility rule with step 4 and
takes the policy for the same reason `AdmissionInput` does. No caller outside `internal/policy` calls `AdmissionReady`
or `WarningAdmissionReady` any more.

**Callers.**

- `run.go`'s sampling loop (lines 844–935) passes `config.Policy`, after the default it substitutes when unset (line
  536), and switches over the path: `AdmissionCleanup` takes today's storage-blocked branch, `AdmissionNormal` and
  `AdmissionDegraded` admit (the latter forcing concurrency `1` without reservation, as today), `AdmissionWait` keeps
  sampling until the deadline, `AdmissionReplan` stops with `ReasonReplanRequired`, and `AdmissionUnset` is a
  supervision failure. The `admitted` boolean goes.
- `withAssessmentDecision` (`development.go`, lines 94–106) passes `resolution.Policy` and becomes an exhaustive
  `switch` over the path: `AdmissionNormal` keeps `run`; `AdmissionWait` and `AdmissionDegraded` set `wait`,
  `ReasonCapacityDeferred`, and retryable, the documented "under any warning `decision` stays `wait`";
  `AdmissionCleanup` keeps a resolution already at `cleanup` and otherwise sets `cleanup` and `ReasonStorageBlocked`;
  `AdmissionReplan` keeps the resolution.
- The driver's `assessAdmission` (`driver.go`, lines 442–468) calls `DecideAdmission` with `WindowSampling` and
  `resolution.Policy` and keeps no rule of its own. The driver's guarded runs (`runGuardedShellUnder`,
  `superviseLineageChild`) reach the same decision through `guard.Run`, with the `Policy` they set in `RunConfig`.

**Tests.** A new `tests/unit/admission_decision_test.go` (every path under both windows, `WindowUnset` refused, and an
input whose `Policy` differs from its resolution's, where the input's alone decides), `internal/cli/development_test.go`
(each path's status decision, exit code, and retryability — AC-14), and the rebound admission scenarios.

## Strict and Tolerant Decoding (Unit 6)

**Strict.** `policy.TaskClass.UnmarshalText` accepts `ephemeral`, `service`, `transactional`, and `release` and refuses
anything else naming the value. The reservation ledger's owner and waiter classes (`reservation.go`, lines 112 and 130)
decode through it, so an unknown class fails at decode; `validReservationClass` (line 543) still refuses `release`
afterwards. The configuration's `coordination.mode` becomes an unexported closed type whose `UnmarshalText` accepts
exactly what `buildCoordination` accepts today, the empty string, meaning the default, and `reservation`, and refuses
every other value, still failing as `hippo.config.unreadable`; D7 keeps configuration compatibility, so a document with
`"mode": ""` or no `mode` key loads as it does today. This is the recorded answer to PW-5
([delivery](../delivery.md#post-write-gate)). The shedding cause decodes strictly from Unit 3.

**Tolerant.** `policy.RecordedTaskClass`, a struct of the parsed class and the recorded text, built like
`RecordedOutcome`: an empty or unknown class decodes without error and keeps its text. It types
`evidence.Summary.TaskClass` (`history.go`, line 38), `Query.Class` (line 55, parsed strictly from `--class`), and
`leaseOwner.Class` (`internal/guard/lease.go`, line 27), where an unknown class keeps making a session record invalid
(`validSessionRecord`, line 152) and an empty one keeps reading `unknown` in the lease message (line 324), as today. The
writer's `EvidenceSummary.TaskClass` (`evidence.go`, line 194) becomes `policy.TaskClass`.

**Ratchet end.** With these entries gone, the analysis allowlist is empty; this unit deletes it and the code that reads
it, as [the gates](002-gates-and-analysis.md#ratchet) describe.

**Tests.** `tests/unit/reservation_test.go` (an unknown ledger class refused at decode),
`internal/evidence/history_test.go` (an unknown recorded class listed as recorded), and
`tests/unit/config_schema2_errors_test.go` (an unknown coordination mode refused).
