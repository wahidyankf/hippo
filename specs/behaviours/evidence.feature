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

  Scenario: A refused evidence root stops a run before launch
    Given a state root HIPPO is not permitted to create
    When a guarded run is requested with that state root
    Then it exits 125 naming hippo.evidence.unwritable and no child starts

  Scenario: A refused never-started receipt is reported rather than hidden by the signal
    Given a queued run whose receipt directory refuses writes
    When the queued run receives SIGINT before it is admitted
    Then it exits 125 naming hippo.evidence.unwritable and no child starts
