Feature: Bounded runtime evidence
  Raw evidence retains a recent window while summaries cover the complete monitored lifetime.

  @e2e-exempt
  Scenario: Active evidence rotates without truncating its lifetime summary
    Given a guarded evidence stream with small test chunks
    When the stream exceeds all retained raw chunks
    Then only the bounded newest raw chunks remain
    And the evidence summary counts every recorded sample

  @e2e-exempt
  Scenario: A shared root admits at most twenty live evidence streams
    Given twenty live evidence streams in one shared root
    When another evidence stream starts
    Then the new stream is rejected before raw evidence is created

  @e2e-exempt
  Scenario: Inactive evidence is pruned to the shared storage budget
    Given inactive evidence above the shared storage budget
    When evidence retention is enforced
    Then the oldest inactive evidence is removed below the budget
