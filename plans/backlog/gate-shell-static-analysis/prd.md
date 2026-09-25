# Product Requirements: Gate Shell Static Analysis

## Product Overview

[Repo-grounded] Shell here is POSIX `sh` for the bootstrap wrapper and gate scripts, and Bash for `rhino`, `ferret`,
`scripts/format-staged.sh`, and the adopted scanner. This plan adds analysis over all of it without changing what any
script does.

## Personas

- A contributor changing a gate script.
- A reviewer who needs the analyser's verdict rather than their own reading.
- A CI runner on Linux and a workstation on macOS running the same pin.

## User Stories

- As a contributor, I want a shell defect at warning level to stop my push so it never reaches review.
- As a reviewer, I want the analyser's version pinned so a green result means the same thing on every machine.
- As a maintainer, I want the adapter to stop recording a gap that no longer exists.

## Product Scope

The pinned analyser, its wrapper, the shared file list, the 14 fixes, the registry entry, and the adapter and
quality-gate documentation.

## Acceptance Criteria

```gherkin
Feature: Shell scripts are statically analysed

  Scenario: [AC-01] The pinned analyser verifies before it runs
    Given the lock file names a version and a digest for this platform
    When the wrapper runs with a cached archive whose digest differs
    Then it refuses before executing the analyser

  Scenario: [AC-02] The repository is clean at the warning threshold
    Given every enumerated shell file
    When the gate runs the analyser at warning severity
    Then it exits 0

  Scenario: [AC-03] A warning-level defect fails the gate
    Given an enumerated script with an unguarded variable in a recursive removal
    When the gate runs
    Then it exits non-zero and names the file and rule

  Scenario: [AC-04] Formatter and analyser read one file list
    Given the enumerated shell file list
    When the format check and the analyser run
    Then both cover the same files, including every root wrapper and hook

  Scenario: [AC-05] The gate runs locally and in CI
    Given the registry declares the gate on pre-push, pull-request, and main
    When a pull request is opened
    Then the repository-contract job runs it, and the adapter records static analysis as a gate
```

## Product Risks

- A new script outside the enumerated list would escape; the list is derived from `git ls-files` plus the named
  entrypoints rather than written by hand.
- A platform without a digest in the lock refuses rather than falling back to `PATH`.
