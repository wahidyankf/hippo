# Specification Changes

Planned under [plan specification changes](../../../../repo-governance/conventions/plan-specification-changes.md).
`specs/` receives the as-built result in the unit named beside each file, scenario first, per
[specification maintenance](../../../../repo-governance/development/specification-maintenance.md).

## What Becomes a Contract

Durable scenarios: AC-11, AC-17, AC-18, AC-19, and AC-22, plus the analysis scenario behind AC-05 to AC-08 and the lint
wiring behind AC-01, AC-03, and AC-04. AC-12 and AC-21 are already durable and are preserved as regressions.

Plan-only, each with its reason and the delivery item that verifies it:

| Criterion | Why it stays in the plan                               | Verified by                           |
| --------- | ------------------------------------------------------ | ------------------------------------- |
| AC-02     | a planted defect, not product behaviour                | Unit 1 NilAway mutation item          |
| AC-09     | a codec test and a documentation test pin it           | Unit 2 codec RED and GREEN items      |
| AC-10     | unreachable through the binary unless a path forgets   | Unit 2 finalize and promotion items   |
| AC-13     | existing contract scenarios pin each status and code   | Unit 3 boundary-switch items          |
| AC-14     | status samples the real host; a table test injects it  | Unit 5 `withAssessmentDecision` items |
| AC-15     | ledger bytes are private; a codec test names the value | Unit 3 shedding-cause items           |
| AC-16     | as AC-15                                               | Unit 3 shedding-cause items           |
| AC-20     | a property of the source tree                          | Unit 5 single-decision proof item     |
| AC-23     | configuration loading is covered by its own unit tests | Unit 6 coordination-mode items        |
| AC-24     | a release, not a behaviour                             | Release unit install item             |
| AC-25     | a record, not a behaviour                              | Archival items                        |

## `specs/behaviours/quality-gates.feature` `[E]` — Unit 1, then Unit 6

```diff
   Scenario: Lint gate wiring is exhaustive and module scoped
     When lint gate wiring is inspected
     Then the configuration enables all linters and unlimited findings
     And exported documentation diagnostics remain errors
+    And govet runs nilness and exhaustive checks switch statements and map literals
     And the quick gate invokes module-local lint
+    And the quick gate invokes the pinned NilAway over the module

+  @e2e-exempt
+  Scenario: Production code compares no domain value with a literal
+    When the domain literal analysis runs over production code
+    Then every finding is on the ratchet allowlist and every allowlist entry holds a finding
```

- As built in Unit 1 (2026-10-06): the step order and the analysis steps' wording above are the ones `specs/` carries.

- In Unit 6 the new scenario's `Then` becomes `Then it reports no finding`.
- = Preserve every other scenario in the file.
- → Bindings: `tests/support/steps.go` registers the four new steps; the wiring steps read `.golangci.yml` and
  `scripts/test-quick.sh` beside the existing lint-wiring steps; the analysis steps call
  `tests/support/domain_literals.go`. The end-to-end boundary cannot carry either, because lint configuration and source
  text are outside the compiled binary; `tests/contract/contract.go` gains an exact exemption for the new scenario with
  `repositoryConfigBoundary`.
- ✓ Proof: `HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd` and the integration adapter.

## `specs/behaviours/public-cli.feature` `[E]` — Unit 2, then Unit 6

```diff
+  Scenario: History lists an outcome this version does not know as recorded
+    Given a current summary whose outcome is future-outcome
+    When JSON history is requested for thirty days
+    Then it exits 0 and that row's outcome reads future-outcome

+  Scenario: History lists a task class this version does not know as recorded
+    Given a current summary whose task class is batch
+    When JSON history is requested for thirty days
+    Then it exits 0 and that row's task class reads batch
```

- The outcome scenario lands in Unit 2 (AC-11); the task-class scenario in Unit 6 (AC-22).
- = Preserve "History filters current labeled summaries" and "A flag value a command cannot accept is a usage mistake",
  whose `history --outcome` and `history --class` rows keep exiting `2` (AC-12).
- → Bindings: `tests/support/steps.go` and `tests/support/history_v05.go`, which write the summary into the scenario's
  evidence root. Both run at all three boundaries, because the end-to-end adapter reads the same evidence root.
- ✓ Proof: the unit and integration adapters, then `npm test` for the end-to-end adapter.

## `specs/behaviours/admission.feature` `[E]` — Unit 4, then Unit 5

```diff
-  Scenario: Minimal work still runs on a tiny machine
-    Given a healthy 1 GiB machine without swap
-    When development admission is assessed
-    Then the minimal profile is selected with concurrency one
+  Scenario Outline: The last-resort floor follows the minimal lineage
+    Given a healthy 1 GiB machine without swap and <configuration>
+    When development admission is assessed
+    Then <result>
+
+    Examples:
+      | configuration                                | result                                                  |
+      | no configuration                             | the minimal profile is selected with concurrency one    |
+      | a default profile extending minimal          | the configured profile is selected with concurrency one |
+      | a profile extending constrained, no fallback | exit 125 names hippo.policy.replan-required             |
```

- The outline keeps the `@e2e-exempt` tag the replaced scenario had. Its first row is the replaced scenario.
- = Preserve every other scenario's text. In Unit 5 the scenarios whose `When` is "development admission is assessed"
  ("Healthy consecutive samples admit work", "Stable macOS warning admits degraded work", "Growing pressure defers
  degraded work", "Strict work never uses degraded admission", "Balanced work falls back on a small runner", "Exhausted
  storage requires cleanup", and "A strict transaction does not silently downgrade") change binding only: the step calls
  `policy.DecideAdmission`.
- → Bindings: `tests/support/steps.go` and `tests/support/degraded_lineage.go`, whose configuration writer gains the
  `fallback` override; the resolution comes from `config.Load`. `tests/contract/contract.go` replaces the exemption for
  "Minimal work still runs on a tiny machine" with one for the outline, `hostEvidenceBoundary`, "requires synthetic host
  capacity that cannot be injected through the compiled binary".
- ✓ Proof: the unit and integration adapters; the outline's second row fails before the floor change (RED).

## `specs/behaviours/reservations.feature` `[E]` — Unit 4

```diff
+  @e2e-exempt
+  Scenario Outline: Automatic owner shares follow the profile lineage
+    Given healthy host capacity and <profile> with no automatic owner share of its own
+    When an automatic reservation is planned in the guard
+    Then capacity is divided into <shares> owner shares
+
+    Examples:
+      | profile                                       | shares |
+      | the built-in balanced profile                 | 4      |
+      | a configured profile that extends balanced    | 4      |
+      | a configured profile that extends constrained | 2      |
+      | a configured profile that extends minimal     | 1      |
```

- = Preserve "Automatic reservations divide capacity by profile owner shares" and every other scenario.
- → Bindings: `tests/support/steps.go` and `tests/support/degraded_lineage.go`: the resolution comes from `config.Load`,
  and the guard's reservation planning receives settings with an empty `OwnerShares` map, so the lineage default is what
  decides. Exempt at the end-to-end boundary with `reservationCapacityBoundary`, because the binary always plans with
  the configuration's filled share map.
- ✓ Proof: the unit and integration adapters; the extends-balanced row fails before the lineage change (RED).

## `specs/behaviours/execution.feature` `[E]` — Unit 4

```diff
-  Scenario: Stable warning spares a balanced ephemeral child admitted under normal pressure
+  Scenario Outline: Stable warning spares an ephemeral child of the balanced lineage
+    Given an ephemeral child of <profile> admitted on healthy Darwin samples
+    When the host then holds a stable macOS warning past the ephemeral grace
+    Then the child finishes with its own exit code
+
+    Examples:
+      | profile                                    |
+      | the built-in balanced profile              |
+      | a configured profile that extends balanced |
```

- The outline keeps the replaced scenario's `@e2e-exempt` tag and its fixture (`advancingCollector`,
  `fastBehaviourPolicy()`); its first row is the replaced scenario.
- = Preserve "Unsafe pressure still sheds a balanced ephemeral child admitted under normal pressure", "Stable warning
  still sheds work outside the exemption", "Warning that outlasts the class grace sheds eligible work", and "Worsening
  warning sheds degraded work".
- → Bindings: `tests/support/steps.go` and `tests/support/driver.go`'s `runGuardedShellAs`, with the configured row's
  resolution from `config.Load`. `tests/contract/contract.go` renames the exemption, keeping `processControlBoundary`.
- ✓ Proof: the unit and integration adapters.

## `specs/architecture.md` `[E]` — Units 4 and 5

```diff
-- **Policy engine and profiles** classify evidence, choose an adaptive development profile, and preserve strict
-  transaction and release envelopes.
+- **Policy engine and profiles** classify evidence, choose an adaptive development profile, key every profile rule on
+  the built-in lineage a profile inherits through `extends`, decide the admission path once for `run`, `status`, and
+  the behaviour driver, and preserve strict transaction and release envelopes.
```

- Unit 4 adds the lineage clause; Unit 5 adds the admission clause. The ledger bullet's "internal shedding cause:
  storage (73) or other pressure (75)" stays true, because the ledger keeps those integers.
- ✓ Proof: `./rhino md internal-link validate` and `npm run test:quick`.

## Assessed, No Change

`evidence.feature`, `conformance.feature`, `artifacts.feature`, `portability.feature`, `release.feature`, and
`terminal.feature`: their scenarios assert wire values this plan preserves. Units 2, 3, and 6 run them unchanged as
regressions.
