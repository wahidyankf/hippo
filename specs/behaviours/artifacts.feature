Feature: Resource guard build artifacts
  Compiled development artifacts stay bounded and outside repository history.

  @e2e-exempt
  Scenario: Release builds stay outside repository history
    When build artifact tracking is inspected
    Then generated release binaries are ignored

  @e2e-exempt
  Scenario: End-to-end binaries are temporary
    When its compiled binary lifecycle is inspected
    Then the end-to-end binary is removed after the run

  @e2e-exempt
  Scenario: Bootstrap cache retention is bounded
    Given four historical bootstrap generations
    When the current bootstrap generation runs
    Then only the current and two recent generations remain

  @e2e-exempt
  Scenario: Machine-local configuration and binaries stay private
    When tracked and ignored paths are inspected
    Then local config and generated artifacts are ignored and untracked
    And the local config example remains tracked
    And the standalone layout has no Nx metadata
