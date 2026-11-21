Feature: HIPPO quality gates
  Repository Go diagnostics stay strict and reproducible through the standalone module boundary.
  # Exemption(e2e): workflow text is outside the compiled binary boundary; alternative-proof: test:quick
  @e2e-exempt
  Scenario: Full gate runs after merge on main, not on pull requests
    When post-merge gate wiring is inspected
    Then the pull request gate runs no full gate matrix
    And the post-merge workflow runs the full gate on push to main on both operating systems
    And the post-merge workflow is never cancelled by a later commit
    And the macOS release reproducibility check runs after merge only

  # Exemption(e2e): gate script and workflow text are outside the compiled binary boundary; alternative-proof: test:quick
  @e2e-exempt
  Scenario: Stable lifecycle wiring is complete
    When stable lifecycle wiring is inspected
    Then the hooks and pull request caller dispatch only closed v0.4 surfaces
    And the declared main and pull request runners retain every owned policy validator
    And the harness declaration owns non-overlapping Claude Codex and OpenCode projections
