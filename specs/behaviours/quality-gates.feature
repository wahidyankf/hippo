Feature: Resource guard quality gates
  Repository Go diagnostics stay strict and reproducible through the standalone module boundary.

  @unit-exempt @e2e-exempt
  Scenario: Lint gate wiring is exhaustive and module scoped
    When lint gate wiring is inspected
    Then the configuration enables all linters and unlimited findings
    And exported documentation diagnostics remain errors
    And the quick gate invokes module-local lint

  @unit-exempt @e2e-exempt
  Scenario: Behavior adapter wiring is complete
    When behavior adapter wiring is inspected
    Then each registered adapter uses strict step resolution
    And each registered pattern owns a function handler
    And every configured behavior exemption has a nonempty reason
    And the quick gate invokes each compliance adapter serially
    And compiled end-to-end behavior runs only in the full gate

  @unit-exempt @e2e-exempt
  Scenario: Contributor gate wiring is complete
    When contributor gate wiring is inspected
    Then the commit hook and CI invoke conventional commit validation
    And pre-commit invokes staged formatting for supported files
    And pre-push invokes the direct quick gate without Nx
    And the quick gate invokes deterministic core coverage at 99 percent
