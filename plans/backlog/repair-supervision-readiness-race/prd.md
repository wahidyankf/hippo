# Product Requirements: Repair Supervision Readiness Race

## Product Overview

[Repo-grounded] The product contract already requires a supervision failure to reap its child before releasing the
resource lease. This plan changes only how the test fixture establishes the scenario's Given.

## Personas

- A maintainer diagnosing a first-run gate failure.
- A contributor changing process supervision or reservation cleanup.
- A CI runner scheduling a child and collector concurrently.

## User Stories

- As a maintainer, I want the fixture to observe the child's PID publication so a missing PID cannot mask the behavior
  under test.
- As a contributor, I want readiness failure to be bounded and explicit so a broken child setup cannot hang the suite.
- As a reviewer, I want the specification and product tree unchanged so the pull request is visibly test-only.

## Product Scope

The plan owns the collector-failure fixture, its bounded readiness synchronization, focused repetition, and repository
gates. It does not alter runtime supervision or public behavior.

## Acceptance Criteria

```gherkin
Feature: Deterministic supervision-failure fixture

  Scenario: [AC-01] Collector failure follows child PID readiness
    Given the guarded shell publishes its PID after a deterministic scheduling delay
    When the fixture reaches the injected host-evidence failure
    Then it observes the child-owned PID marker before returning that failure

  Scenario: [AC-02] Supervision cleanup remains the asserted behavior
    Given the collector failure occurs after child PID readiness
    When guarded supervision returns the injected failure
    Then the child is reaped before its resource lease becomes reacquirable

  Scenario: [AC-03] The repair is stable without product changes
    Given only test-support and plan files differ from current main
    When the focused scenario repeats and the repository completion gates run
    Then every invocation passes without retry and no product contract changes

  Scenario: [AC-04] Missing child readiness fails boundedly
    Given the guarded child never publishes its PID marker
    When the collector reaches the host-evidence failure boundary
    Then the fixture returns a readiness-specific error within its configured bound
```

## Product Risks

- A fixed sleep alone would not satisfy AC-01.
- Waiting after collector failure would still allow the wrong event order.
- Reusing a helper must preserve bounded timeout behavior at this call site.
