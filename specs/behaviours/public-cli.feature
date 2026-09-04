Feature: Public resource guard CLI
  Any repository can inspect and invoke the standalone guard before its build system starts.

  Scenario: Version identifies the exact build
    Given the compiled resource guard binary
    When JSON version is requested
    Then version schema identifies the release and commit

  Scenario: JSON status exposes the stable evidence schema
    Given the compiled resource guard binary
    When JSON status is requested for an existing path
    Then status returns schema version 3 with profile and capability evidence

  Scenario: Command discovery uses Cobra help
    Given the compiled resource guard binary
    When root command help is requested
    Then help lists the public command tree and exits successfully

  Scenario: Release discovery uses grouped help
    Given the compiled resource guard binary
    When release command help is requested
    Then help lists the release command tree and exits successfully

  Scenario: Shell completion is generated on demand
    Given the compiled resource guard binary
    When Zsh completion is requested
    Then a Zsh completion script is emitted

  Scenario: Unknown commands use Cobra diagnostics
    Given the compiled resource guard binary
    When an unknown command is requested
    Then Cobra reports the command and exits with code 1

  Scenario: Invalid explicit configuration is actionable
    Given an explicit resource guard config with an unknown field
    When JSON status is requested with that config
    Then configuration fails with exit 78

  Scenario: Run validates its command boundary
    Given the compiled resource guard binary
    When run is requested without a command separator
    Then the command fails with a useful validation error

  Scenario: Commands without operands reject positional arguments
    Given the compiled resource guard binary
    When operand-free commands are requested with positional arguments
    Then every command rejects the unexpected argument

  Scenario: Release summary assessment accepts healthy evidence
    Given a healthy release summary file
    When release summary assessment is requested
    Then the release evidence is accepted

  Scenario: Release monitoring requires generic health inputs
    Given release monitor output paths without endpoint inputs
    When release monitoring is requested
    Then the command rejects a missing generic health URL
