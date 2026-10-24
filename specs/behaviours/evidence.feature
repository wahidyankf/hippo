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

  Scenario Outline: A refused coordination write stops a run before launch
    Given a state root whose <coordination state> refuses writes
    When a guarded run is requested with that state root
    Then it exits 125 naming hippo.evidence.unwritable and no child starts

    Examples:
      | coordination state             |
      | coordination lock              |
      | reservation identity directory |
      | session directory              |

  Scenario: Status reports a coordination lock it cannot open as a refused write
    Given a reservation state root whose coordination lock refuses writes
    When status is requested with that state root
    Then status exits 125 naming hippo.evidence.unwritable

  Scenario: A refused never-started receipt is reported rather than hidden by the signal
    Given a queued run whose receipt directory refuses writes
    When the queued run receives SIGINT before it is admitted
    Then it exits 125 naming hippo.evidence.unwritable and no child starts

  @e2e-exempt
  Scenario Outline: A run HIPPO stops before launch is summarized as admission-failed
    Given a guarded run that has begun sampling the host
    When <failure> before its child starts
    Then it exits 125 naming <reason>, its child never starts, and its lifetime summary records the outcome admission-failed
    And history filtered to the outcome admission-failed lists that run
    And its lifetime summary carries no child usage fields

    Examples:
      | failure                                                    | reason                    |
      | its host evidence becomes unreadable                       | hippo.host.unreadable     |
      | a signal stops it and its never-started receipt is refused | hippo.evidence.unwritable |
      | its command stops being executable                         | hippo.supervision.failed  |

  Scenario: A passed run records the CPU and wall time its child used
    Given an empty evidence root for guarded runs
    When a guarded child keeps one CPU busy for about one second
    And JSON history is requested for thirty days
    Then the history row records at least 0.8 seconds of child CPU and at least 1 second of child wall time
    And the history row carries no command, argument, working directory, or path

  Scenario: A waited grandchild counts toward the run's child CPU
    Given an empty evidence root for guarded runs
    When a guarded child waits for a grandchild that keeps one CPU busy for about one second
    And JSON history is requested for thirty days
    Then the history row records at least 0.8 seconds of child CPU

  Scenario: A failed run still records the CPU its child used
    Given an empty evidence root for guarded runs
    When a guarded child keeps one CPU busy for about one second and then exits 3
    And JSON history is requested for thirty days
    Then the history row outcome reads task-failed and records at least 0.8 seconds of child CPU

  @e2e-exempt
  Scenario: A shed run records only a lower bound of its child usage
    Given an empty evidence root for guarded runs
    When a guarded child starts a CPU-busy grandchild that the guard never waits for and critical pressure sheds it
    And JSON history is requested for thirty days
    Then the history row reads pressure-shed and records less than 0.5 seconds of child CPU

  Scenario: Compacting a summary keeps its child usage
    Given a passed run from the previous day that recorded its child usage
    When a later guarded run completes
    And JSON history is requested for thirty days
    Then the history row still records at least 0.8 seconds of child CPU and at least 1 second of child wall time

  Scenario: A summary written before child usage was recorded still reads
    Given a passed run whose summary predates child usage fields
    When JSON history is requested for thirty days
    Then the history row reads passed and carries no child usage fields
