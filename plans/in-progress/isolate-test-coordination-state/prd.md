# Product Requirements: Isolate Test Coordination State

## Product Overview

[Repo-grounded] The product reads `HIPPO_ROOT`, `HIPPO_CONFIG`, `HIPPO_DEFAULT_CONFIG`, `HIPPO_IDENTITY`,
`HIPPO_DEFAULT_IDENTITY`, `HIPPO_SESSION`, and `HIPPO_COLOR`, and the guard exports further `HIPPO_*` values to its
child. This plan changes only the environment the test packages give the binary under test.

## Personas

- A contributor running the complete gate beneath a workstation guard.
- A maintainer triaging a local-only failure.
- A test author adding a scenario that sets its own `HIPPO_ROOT`.

## User Stories

- As a contributor, I want the suite to ignore my shell's coordination variables so a local failure is about my change.
- As a maintainer, I want a test run never to resolve the shared evidence root.
- As a test author, I want my scenario's explicit root and helper flags to survive the isolation.

## Product Scope

The test-support isolation helper, the four test-package entry points, and their proof. Nothing the product does
changes.

## Acceptance Criteria

```gherkin
Feature: Test runs own their coordination state

  Scenario: [AC-01] Inherited coordination variables are removed
    Given the invoking environment exports HIPPO_CONFIG, HIPPO_SESSION, and HIPPO_PROFILE
    When a test package starts
    Then none of them is visible to the binary under test
    And HIPPO_ROOT names a directory created for this run

  Scenario: [AC-02] Harness inputs survive
    Given the invoking environment sets HIPPO_BIN, HIPPO_BDD_ADAPTER, HIPPO_E2E_TEMP_PARENT, HIPPO_GO_BINARY, and HIPPO_LOAD_SATURATED
    When a test package starts
    Then each keeps its value

  Scenario: [AC-03] A re-executed helper keeps its parent's environment
    Given a test re-executes its own binary with a helper flag
    When the helper process starts
    Then the isolation step leaves its environment unchanged

  Scenario: [AC-04] The guarded complete gate passes
    Given the end-to-end runner starts beneath the workstation guard with every HIPPO variable exported
    When the behaviour suite and the command-line contract run
    Then both pass without retry

  Scenario: [AC-05] The change is test-only
    Given only test-support, test entry points, and plan files differ from main
    When the repository gates run
    Then they pass and no product or specification path changed
```

## Product Risks

- A package without the `TestMain` stays exposed; the file-impact list names all four that start the product.
- A per-run root that is not removed would accumulate in the temporary directory; the helper removes it after `m.Run`.
