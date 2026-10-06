# File Impact

Every path each unit is expected to touch: `[N]` new, `[E]` edited, `[D]` deleted, `[M]` moved. A path a unit finds it
must also touch is recorded in [learnings](../learnings.md) and in the unit's pull-request body.

## Unit 1 — Gates

```text
.golangci.yml                                                     [E] govet nilness; exhaustive check: [switch, map]
go.mod                                                            [E] NilAway tool directive and its modules
go.sum                                                            [E] NilAway tool directive and its modules
scripts/test-quick.sh                                             [E] go tool nilaway line after golangci-lint
internal/status/status.go                                         [E] retryable map names all 18 codes
internal/guard/exclusive_status.go                                [E] NilAway hazards fixed or excluded
internal/guard/lease.go                                           [E] NilAway hazards fixed or excluded
internal/guard/exclusive_status_test.go                           [E] NilAway hazard test for exclusive_status.go
internal/guard/run_test.go                                        [E] NilAway finding in test code
tests/support/release_v04.go                                      [E] NilAway findings in test code
tests/support/isolation_test.go                                   [E] NilAway findings in test code
tests/integration/lease_evidence_test.go                          [E] NilAway findings in test code; lease attempt tests
tests/integration/run_test.go                                     [E] NilAway finding in test code, added during Unit 1
tests/support/domain_literals.go                                  [N] the analysis
tests/support/domain_literals_internal_test.go                    [N] fixture tests of the analysis
tests/support/domain_literals_allowlist.go                        [N] the ratchet allowlist
tests/support/lint_wiring_internal_test.go                        [N] mutations of the lint-wiring checks (Unit 1)
tests/unit/vocabulary_test.go                                     [E] per-code retryable test (Unit 1)
tests/support/steps.go                                            [E] wiring and analysis steps
tests/support/driver.go                                           [E] nilness, map, and NilAway wiring checks (Unit 1)
tests/contract/contract.go                                        [E] end-to-end exemption for the analysis scenario
specs/behaviours/quality-gates.feature                            [E] wiring steps; analysis scenario
specs/behaviours/README.md                                        [E] feature index names NilAway and the analysis
repo-governance/development/quality/stacks/repository-adapter.md  [E] gates entry links its new module
repo-governance/development/quality/stacks/repository-adapter/001-go-analysis-gates.md [N] gates record
repo-governance/development/quality/stacks/repository-adapter/README.md [N] module index
repo-governance/development/quality/stacks/README.md              [E] indexes the adapter's modules
repo-governance/development/quality-gates.md                      [E] quick-gate order names NilAway and the analysis
```

## Unit 2 — Outcome

```text
internal/evidence/outcome.go                                      [N] Outcome, BudgetOutcome, RecordedOutcome, codecs
internal/evidence/outcome_test.go                                 [N] codec, parse, and tolerant round-trip tests
internal/evidence/history.go                                      [E] typed Summary, Query, and promotion check
internal/evidence/history_test.go                                 [E] typed fixtures
internal/guard/run.go                                             [E] outcome; deferral; finalOutcome; promoteFinalize
internal/guard/evidence.go                                        [E] typed Finalize, reservation context, fields
internal/guard/run_test.go                                        [E] finalOutcome, promoteFinalize, deferral tests
internal/cli/history.go                                           [E] outcomeFlag parsed by evidence.ParseOutcome
internal/cli/commands.go                                          [E] outcomeFlag parsed by evidence.ParseOutcome
internal/cli/history_test.go                                      [E] typed rows
tests/unit/outcome_vocabulary_test.go                             [N] documented outcome list equals evidence.Outcomes()
tests/support/interruption_v082.go                                [E] typed outcome reads; unknown-outcome writer
tests/support/history_v05.go                                      [E] typed outcome reads; unknown-outcome writer
tests/support/steps.go                                            [E] unknown-outcome history steps
tests/support/domain_literals_allowlist.go                        [E] outcome entries removed
.golangci.yml                                                     [E] exhaustruct_v5 on for evidence.RecordedOutcome
specs/behaviours/public-cli.feature                               [E] unknown-outcome history scenario
docs/reference/json-schemas.md                                    [E] history lists an unknown outcome as recorded
repo-governance/development/quality/stacks/repository-adapter/001-go-analysis-gates.md [E] exhaustruct_v5 scope
```

## Unit 3 — Internal Reasons

```text
internal/policy/reason.go              [N] Reason, Stop, Stopped, legacy integer codec
internal/policy/profiles.go            [E] Resolution.Reason; constants 76 and 78 and bare 73 removed
internal/guard/run.go                  [E] constants 73, 74, 75 and callerShedCode removed; Stop returns
internal/guard/reservation.go          [E] ShedCause and its strict 73/75 codec
internal/cli/status.go                 [E] reasonCode and reasonMessage switches; nolint removed
internal/cli/status_test.go            [N] every reason to code and status
internal/cli/development.go, internal/cli/release.go [E] Stop returns; withAssessmentDecision sets Reason
internal/cli/development_test.go       [E] typed expectations
internal/guard/run_test.go             [E] Stop expectations; finalize precedence test
tests/unit/reason_test.go              [N] legacy integers per member
tests/unit/adaptive_test.go            [E] typed expectations; ledger codec cases
tests/unit/reservation_test.go         [E] typed expectations; ledger codec cases
tests/integration/reservation_test.go  [E] typed expectations
tests/integration/run_test.go          [E] typed expectations
tests/support/blockers_v04.go          [E] typed expectations
tests/support/degraded_lineage.go      [E] typed expectations
tests/support/driver.go                [E] typed expectations
tests/support/history_v05.go           [E] typed expectations
tests/support/loaded_gate.go           [E] typed expectations
tests/support/pending_v04.go           [E] typed expectations
tests/support/review_v04.go            [E] typed expectations
specs/architecture.md                  [E] only if the shedding-cause bullet no longer reads true
```

## Unit 4 — Profile Lineage

```text
internal/policy/profiles.go                 [E] ProfileName, Lineage, floor by lineage
internal/config/config.go                   [E] ProfileName keys; default shares from lineage
internal/guard/reservation.go               [E] owner-share default by lineage; typed profile fields
internal/guard/owner_metadata.go            [E] typed profile fields and parameters
internal/guard/evidence.go                  [E] typed profile fields and parameters
internal/guard/run.go                       [E] typed profile reads
internal/cli/commands.go                    [E] requestedProfileFlag; typed monitor profile
internal/cli/development.go                 [E] requestedProfileFlag; typed monitor profile
internal/cli/release.go                     [E] typed profile reads
tests/unit/policy_test.go                   [E] lineage and owner-share default tests
tests/unit/reservation_test.go              [E] lineage and owner-share default tests
tests/support/driver.go                     [E] resolve through config.Load; fallback override
tests/support/degraded_lineage.go           [E] resolve through config.Load; fallback override
tests/support/steps.go                      [E] outline steps
tests/contract/contract.go                  [E] exemptions for the three outlines
tests/support/domain_literals_allowlist.go  [E] profile entries removed
specs/behaviours/admission.feature          [E] minimal-lineage outline
specs/behaviours/reservations.feature       [E] owner-share outline
specs/behaviours/execution.feature          [E] stable-warning spares outline
specs/architecture.md                       [E] lineage clause
docs/reference/resource-policy.md           [E] the floor follows the minimal lineage
docs/reference/configuration.md             [E] lineage note covers the floor and default owner shares
```

## Unit 5 — Admission Path and Decision

```text
internal/policy/admission.go                                      [N] AdmissionPath, EvidenceWindow, AdmissionInput,
                                                         DecideAdmission, SparesStableWarning
internal/guard/run.go                                             [E] sampling loop and supervision exemption use policy
internal/cli/development.go                                       [E] withAssessmentDecision switches over the path
internal/cli/development_test.go                                  [E] path-to-decision table
tests/unit/admission_decision_test.go                             [N] every path under both windows
tests/support/driver.go                                           [E] assessAdmission calls DecideAdmission
.golangci.yml                                                     [E] exhaustruct_v5 adds policy.AdmissionInput
specs/behaviours/admission.feature                                [E] bindings only (no text change)
specs/architecture.md                                             [E] admission clause
repo-governance/development/quality/stacks/repository-adapter/001-go-analysis-gates.md [E] exhaustruct_v5 scope
```

## Unit 6 — Strict Decoding and the End of the Ratchet

```text
internal/policy/profiles.go                                       [E] TaskClass.UnmarshalText; RecordedTaskClass
internal/guard/reservation.go                                     [E] strict ledger class decode
internal/guard/lease.go                                           [E] leaseOwner.Class as RecordedTaskClass
internal/guard/evidence.go                                        [E] EvidenceSummary.TaskClass typed
internal/evidence/history.go                                      [E] Summary.TaskClass and Query.Class typed
internal/evidence/history_test.go                                 [E] unknown class listed as recorded
internal/config/config.go                                         [E] closed coordination mode
internal/cli/commands.go                                          [E] classFlag parsed strictly
internal/cli/history.go                                           [E] classFlag parsed strictly
tests/unit/reservation_test.go                                    [E] unknown class refused at decode
tests/unit/config_schema2_errors_test.go                          [E] unknown coordination mode refused
tests/support/history_v05.go                                      [E] unknown-class history steps
tests/support/steps.go                                            [E] unknown-class history steps
tests/support/domain_literals.go                                  [E] allowlist reading removed
tests/support/domain_literals_allowlist.go                        [D] the allowlist
specs/behaviours/quality-gates.feature                            [E] the analysis reports no finding
specs/behaviours/public-cli.feature                               [E] unknown-class history scenario
.golangci.yml                                                     [E] exhaustruct_v5 adds policy.RecordedTaskClass
docs/reference/json-schemas.md                                    [E] history lists an unknown class as recorded
repo-governance/development/quality/stacks/repository-adapter/001-go-analysis-gates.md [E] ratchet closed; final scope
```

## Release and Archival

```text
CHANGELOG.md                                              [E] v0.8.5 entry
README.md                                                 [E] current release v0.8.5
docs/how-to/install-a-pinned-release.md                   [E] current release v0.8.5
docs/how-to/enable-reservation-coordination.md            [E] current tagged baseline v0.8.5
docs/reference/cli.md                                     [E] version examples v0.8.5
docs/reference/json-schemas.md                            [E] version examples v0.8.5
plans/in-progress/strict-go-linting-and-domain-modeling/  [M] to its dated plans/done/ folder
plans/in-progress/README.md                               [E] stage indexes
plans/done/README.md                                      [E] stage indexes
```
