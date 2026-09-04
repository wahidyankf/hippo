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

  @e2e-exempt
  Scenario: Invalid explicit configuration is actionable
    Given an explicit resource guard config with an unknown field
    When JSON status is requested with that config
    Then configuration fails with exit 78

  Scenario: Run validates its command boundary
    Given the compiled resource guard binary
    When run is requested without a command separator
    Then the command fails with a useful validation error

  Scenario: Release summary assessment accepts healthy evidence
    Given a healthy release summary file
    When release summary assessment is requested
    Then the release evidence is accepted

  @e2e-exempt
  Scenario: Release monitoring requires generic health inputs
    Given release monitor output paths without endpoint inputs
    When release monitoring is requested
    Then the command rejects a missing generic health URL
