# Product Requirements: Strict Go Linting and Domain Modeling

## Product Overview

[Repo-grounded] HIPPO is one Go module: the `hippo` command line, its `internal/` packages, and a behaviour corpus of 11
feature files run through unit, integration, and end-to-end adapters. This plan changes how the module represents five
concepts and what its gates refuse. What a consumer observes stays as it is in `v0.8.4`, except that a configured
profile derived from `minimal` now receives the last-resort floor its base receives.

## Personas

- **A consumer's gate script** that branches on HIPPO's exit status, reason code, evidence, and status JSON, pinned to
  one release while another repository on the same host pins another.
- **A contributor** adding an outcome, a reason, a profile rule, or an admission branch.
- **A reviewer** who needs the gate, not their reading, to say a case was missed.

## User Stories

- As a contributor, I want a forgotten outcome or reason to fail compilation or lint, so it never reaches review.
- As a contributor, I want a comparison of a profile, outcome, or class with a literal to fail a gate, so a rule cannot
  key on a name again.
- As a consumer, I want a configured profile to behave as the built-in profile it extends, so tuning a profile does not
  silently forfeit a rule.
- As a consumer, I want my pinned release to keep reading evidence a newer release wrote, so mixed versions on one host
  do not break `history`.
- As a reviewer, I want `run`, `status`, and the test driver to share one admission decision, so a test cannot pass
  against a copy of the rule.

## Product Scope

The behaviours below, the gates that prove them, and the `v0.8.5` release. No exit status, error code, JSON field,
configuration key, or state-root format is added, removed, or renamed.

## Acceptance Criteria

```gherkin
Feature: Strict Go linting and domain modeling

  # Gates

  Scenario: [AC-01] The pinned NilAway passes on the delivered tree
    Given go.mod pins NilAway v0.0.0-20260918162853-acb8859b9031 as a tool
    When npm run test:quick runs
    Then the NilAway step exits 0

  Scenario: [AC-02] A nil dereference NilAway reports fails the quick gate
    Given a production function that reads a field of a result its callee may return as nil
    When npm run test:quick runs
    Then it exits non-zero with a NilAway diagnostic naming that file and line

  Scenario: [AC-03] A known-nil dereference fails lint
    Given a production function that dereferences a pointer inside the branch where it is nil
    When go tool golangci-lint run runs
    Then it exits non-zero naming govet and nilness

  Scenario: [AC-04] A map keyed by an enumerated type must name every member
    Given a map literal keyed by status.Code that omits status.CodeArgsInvalid
    When go tool golangci-lint run runs
    Then it exits non-zero naming exhaustive and status.CodeArgsInvalid

  Scenario: [AC-05] A domain value compared with a literal fails the analysis
    Given production code that compares a resolution's ResolvedProfile with the literal "balanced"
    When the domain literal analysis runs
    Then it fails naming that file, that line, and the literal-comparison rule

  Scenario: [AC-06] A domain-named field typed as a raw string fails the analysis
    Given a production struct field named Outcome declared as string
    When the domain literal analysis runs
    Then it fails naming that file, that line, and the raw-field rule

  Scenario: [AC-07] An allowlist entry whose violation is gone fails the analysis
    Given an allowlist entry for a file and symbol that no longer hold its violation
    When the domain literal analysis runs
    Then it fails naming the stale entry

  Scenario: [AC-08] The last code unit leaves no allowlist
    Given the head of the strict decoding unit
    When git grep -n domainLiteralAllowlist runs over tests and internal
    Then it prints nothing and the domain literal analysis reports no finding

  # Outcome

  Scenario Outline: [AC-09] Every run outcome keeps its v0.8.4 wire string
    Given a guarded run that ends because <ending>
    When its lifetime summary is written
    Then the summary's outcome reads "<wire>"

    Examples:
      | ending                                           | wire                  |
      | its child exits 0                                | passed                |
      | its child exits 3                                | task-failed           |
      | host evidence is lost after launch               | supervision-failed    |
      | warning pressure outlasts the class grace        | pressure-shed         |
      | disk falls below its reserve after launch        | storage-shed          |
      | available memory falls below the emergency floor | emergency-safety-stop |
      | admission never becomes safe before its deadline | capacity-deferred     |
      | the disk floor blocks admission                  | storage-blocked       |
      | a signal arrives while it samples the host       | admission-cancelled   |
      | host evidence becomes unreadable before launch   | admission-failed      |

  Scenario: [AC-10] A summary finalized with no outcome records a supervision failure
    Given a guarded run whose lifetime ends with its outcome still unset
    When its lifetime summary is finalized
    Then the summary's outcome reads "supervision-failed" and the run exits 125 naming hippo.supervision.failed

  Scenario: [AC-11] History lists an outcome this version does not know as recorded
    Given a current summary whose outcome is "future-outcome"
    When JSON history is requested for thirty days
    Then it exits 0 and that row's outcome reads "future-outcome"

  Scenario: [AC-12] History still refuses an outcome filter no run can carry
    Given any evidence root
    When history is filtered with --outcome future-outcome
    Then it exits 2 naming hippo.args.invalid and lists the ten known outcomes

  # Internal reasons

  Scenario Outline: [AC-13] Every internal reason keeps its v0.8.4 status and code
    Given a command that stops because <cause>
    When the command-line boundary reports it
    Then it exits <status> naming <code>

    Examples:
      | cause                                         | status | code                                 |
      | the disk floor blocks admission               | 124    | hippo.limit.storage-blocked          |
      | admission never becomes safe before deadline  | 124    | hippo.limit.capacity-deferred        |
      | pressure sheds a started child                | 124    | hippo.limit.pressure-shed            |
      | live coordination state uses another protocol | 125    | hippo.coordination.protocol-mismatch |
      | no profile admits a strict request            | 125    | hippo.policy.replan-required         |

  Scenario Outline: [AC-14] JSON status keeps its v0.8.4 decision and exit code
    Given a host whose samples show <state>
    When JSON status is requested
    Then profile.decision reads "<decision>" and profile.exitCode reads <exitCode>

    Examples:
      | state                                      | decision | exitCode |
      | normal pressure                            | run      | 0        |
      | a stable macOS warning                     | wait     | 75       |
      | free disk below the 256 MiB floor          | cleanup  | 73       |
      | a strict profile that does not fit         | replan   | 78       |

  Scenario: [AC-15] A ledger shedding code outside 73 and 75 is refused
    Given a reservation ledger whose shedding owner records sheddingExitCode 74
    When reservation state is decoded by coordination
    Then admission fails closed and preserves the ledger bytes

  Scenario Outline: [AC-16] A shed writes its v0.8.4 ledger code
    Given a reservation owner shed because of <cause>
    When the ledger is written
    Then that owner's sheddingExitCode reads <code>

    Examples:
      | cause            | code |
      | storage          | 73   |
      | memory pressure  | 75   |

  # Profile lineage

  Scenario Outline: [AC-17] The last-resort floor follows the minimal lineage
    Given a healthy 1 GiB machine without swap and <configuration>
    When development admission is assessed
    Then <result>

    Examples:
      | configuration                                | result                                                  |
      | no configuration                             | the minimal profile is selected with concurrency one    |
      | a default profile extending minimal          | the configured profile is selected with concurrency one |
      | a profile extending constrained, no fallback | exit 125 names hippo.policy.replan-required             |

  Scenario Outline: [AC-18] Automatic owner shares follow the profile lineage
    Given healthy host capacity and <profile> with no automatic owner share of its own
    When an automatic reservation is planned in the guard
    Then capacity is divided into <shares> owner shares

    Examples:
      | profile                                       | shares |
      | the built-in balanced profile                 | 4      |
      | a configured profile that extends balanced    | 4      |
      | a configured profile that extends constrained | 2      |
      | a configured profile that extends minimal     | 1      |

  Scenario Outline: [AC-19] Stable warning spares an ephemeral child of the balanced lineage
    Given an ephemeral child of <profile> admitted on healthy Darwin samples
    When the host then holds a stable macOS warning past the ephemeral grace
    Then the child finishes with its own exit code

    Examples:
      | profile                                    |
      | the built-in balanced profile              |
      | a configured profile that extends balanced |

  # One admission decision

  Scenario: [AC-20] Admission is decided in one place
    Given the delivered tree
    When git grep -nE "AdmissionReady\(" runs over internal/guard, internal/cli, and tests/support
    Then it prints nothing

  # Decoding

  Scenario: [AC-21] A reservation ledger naming an unknown class is refused at decode
    Given a reservation ledger whose owner class is "batch"
    When reservation state is decoded by coordination
    Then admission fails closed and preserves the class-corrupt ledger bytes

  Scenario: [AC-22] History lists a task class this version does not know as recorded
    Given a current summary whose taskClass is "batch"
    When JSON history is requested for thirty days
    Then it exits 0 and that row's taskClass reads "batch"

  Scenario: [AC-23] An unknown coordination mode is refused at load
    Given a schema-2 configuration whose coordination mode is "exclusive"
    When status is requested with that configuration
    Then it exits 125 naming hippo.config.unreadable

  # Release and record

  Scenario: [AC-24] v0.8.5 installs by the documented pinned commands
    Given the published v0.8.5 release
    When the commands in docs/how-to/install-a-pinned-release.md run for this platform
    Then the checksum line reads OK and version --json names v0.8.5

  Scenario: [AC-25] The plan is archived only after a permitting execution check
    Given every substantive delivery item is terminal
    When the execution check records its verdict
    Then a separate docs-only pull request moves the plan to plans/done/ and npm run test:quick exits 0 there
```

## Product Risks

- **A codec drifts.** A typed field could marshal differently from the string or integer it replaces; AC-09, AC-14,
  AC-16, and the existing contract scenarios pin each value.
- **A tolerant reader hides corruption.** An `unknown` outcome or class is listed as recorded, never counted as healthy:
  owner promotion still requires the `passed` member.
- **The analyzer misses a shape.** It reads the comparisons and declarations D9 names; a domain value laundered through
  an unnamed local escapes it, which the closed types and `exhaustive` cover instead.
- **The `minimal` fix surprises a consumer.** A configured profile derived from `minimal` that used to exit `125` now
  admits under the floor; the changelog names it under `Fixed`.
