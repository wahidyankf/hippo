# Product Requirements: Neutralize Test Fixture Identifiers

## Product Overview

[Repo-grounded] `identity.Load` takes an optional source override that wins when no identity file exists. The test
for that behaviour stays; only its fixture value changes.

## Personas

- A maintainer reviewing what the public tree publishes.
- A contributor copying a fixture into a new test.

## User Stories

- As a maintainer, I want every fixture to be obviously synthetic so none publishes a private name.
- As a contributor, I want the test I copy to model the synthetic-value rule.

## Product Scope

Two lines of one unit test. No product behaviour changes.

## Acceptance Criteria

```gherkin
Feature: Identity fixtures are synthetic

  Scenario: [AC-01] The source override is proved with a synthetic value
    Given TestOverrideSourceWithoutFile passes the override "fixture-source"
    When go test runs the identity package
    Then the loaded identity reports source "fixture-source" and group "local"

  Scenario: [AC-02] The removed string is gone from the tree
    Given the string the test used before this change
    When git grep searches every tracked file for it
    Then it finds no match

  Scenario: [AC-03] The change is test-only
    Given only the test file and plan files differ from main
    When the repository gates run
    Then they pass and no product path changed
```

## Product Risks

- Nothing in this plan's text may quote the removed string; the executor reads it from line 42 before editing.
