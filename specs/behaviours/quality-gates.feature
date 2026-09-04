Feature: Resource guard quality gates
  Repository Go diagnostics stay strict and reproducible through the standalone module boundary.

  @unit-exempt @e2e-exempt
  Scenario: Strict lint is exhaustive and module scoped
    Given the resource guard Go lint configuration
    When lint enforcement is inspected
    Then every available linter is enabled with documented exceptions
    And package documentation is enforced
    And lint remains scoped to the resource guard module

  @unit-exempt @e2e-exempt
  Scenario: Every behavior adapter has enforced compliance
    Given the resource guard Gherkin adapter contract
    When behavior coverage enforcement is inspected
    Then unit integration and end-to-end suites use strict step resolution
    And every behavior exemption has an approved adapter reason
    And behavior compliance runs serially for every adapter
    And full end-to-end behavior remains outside quick checks

  @unit-exempt @e2e-exempt
  Scenario: Contributor changes pass local and remote gates
    Given the resource guard contributor gate contract
    When contributor enforcement is inspected
    Then conventional commits are enforced locally and in CI
    And staged source and documentation files are formatted before commit
    And pushes run the quick gate without Nx
    And deterministic core coverage requires 99 percent
