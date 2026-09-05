Feature: Public HIPPO CLI
  Any repository can inspect and invoke the standalone HIPPO before its build system starts.

  Scenario: Version identifies the exact build
    Given the compiled HIPPO binary
    When JSON version is requested
    Then version schema identifies the release and commit

  Scenario: JSON status exposes the stable evidence schema
    Given the compiled HIPPO binary
    When JSON status is requested for an existing path
    Then status returns schema version 4 with profile capability and coordination evidence

  Scenario: JSON status fails closed on corrupt coordination state
    Given the compiled HIPPO binary with corrupt reservation coordination state
    When JSON status is requested for that coordination root
    Then status reports the coordination error instead of schema four zero totals

  @e2e-exempt
  Scenario: JSON status waits out a busy coordination root
    Given a coordination root whose lock is briefly held by a peer
    When JSON status is requested while that peer still holds the lock
    Then status reports its coordination totals instead of a contention error

  Scenario: JSON status gives active coordination markers precedence
    Given schema one and schema two configs paired with opposite active markers
    When JSON status is requested for each marked and empty root
    Then active marker modes win symmetrically and empty roots use configured modes

  Scenario: JSON status rejects overflowing queued demand
    Given the compiled HIPPO binary with individually valid overflowing reservation waiters
    When JSON status aggregates the queued reservation demand
    Then status reports a privacy-safe coordination error instead of wrapped totals

  Scenario: Command discovery uses Cobra help
    Given the compiled HIPPO binary
    When root command help is requested
    Then help lists the public command tree and exits successfully

  Scenario: Help identifies the HIPPO acronym
    Given the compiled HIPPO binary
    When root command help is requested
    Then help expands HIPPO as Host Infrastructure Pressure and Process Orchestrator

  Scenario: Release discovery uses grouped help
    Given the compiled HIPPO binary
    When release command help is requested
    Then help lists the release command tree and exits successfully

  Scenario: Shell completion is generated on demand
    Given the compiled HIPPO binary
    When Zsh completion is requested
    Then a Zsh completion script is emitted

  Scenario: Unknown commands use Cobra diagnostics
    Given the compiled HIPPO binary
    When an unknown command is requested
    Then Cobra reports the command and exits with code 1

  Scenario: Only usage errors print the command usage block
    Given the compiled HIPPO binary
    When a runtime failure and a usage error are requested
    Then only the usage error prints the command usage block

  Scenario: Invalid explicit configuration is actionable
    Given an explicit HIPPO config with an unknown field
    When JSON status is requested with that config
    Then configuration fails with exit 78

  Scenario: Run validates its command boundary
    Given the compiled HIPPO binary
    When run is requested without a command separator
    Then the command fails with a useful validation error

  Scenario: Commands without operands reject positional arguments
    Given the compiled HIPPO binary
    When operand-free commands are requested with positional arguments
    Then every command rejects the unexpected argument

  Scenario: Release summary assessment accepts healthy evidence
    Given a healthy release summary file
    When release summary assessment is requested
    Then the release evidence is accepted

  Scenario: Release summary assessment accepts standard input
    Given a healthy release summary on standard input
    When release summary assessment is requested from standard input
    Then the release evidence is accepted on standard output

  @e2e-exempt
  Scenario: Development monitor emits machine-readable transitions
    Given repeated and changing resource states
    When JSON development monitoring is requested
    Then one valid JSON record is emitted for each state transition

  Scenario: Release monitoring requires generic health inputs
    Given release monitor output paths without endpoint inputs
    When release monitoring is requested
    Then the command rejects a missing generic health URL

  @e2e-exempt
  Scenario: Release raw evidence streams to standard output
    Given a bounded release monitor with raw standard output
    When release monitoring completes
    Then raw JSON lines use standard output and the summary remains a file

  @e2e-exempt
  Scenario: Release summary streams to standard output
    Given a bounded release monitor with summary standard output
    When release monitoring completes
    Then the final summary uses standard output and raw evidence remains a file

  Scenario: Release output formats cannot share standard output
    Given release raw evidence and summary both target standard output
    When release monitoring is requested
    Then the command rejects mixed standard output before collecting evidence

  @e2e-exempt
  Scenario: Release streaming propagates downstream failure
    Given a release raw stream whose downstream writer fails
    When release monitoring writes its first sample
    Then monitoring fails without closing the caller-owned stream
