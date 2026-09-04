Feature: Guarded process execution
  The guard serializes heavy work, keeps long-lived services outside that lease, and owns only the process it launches.

  @e2e-exempt
  Scenario: A live heavy lease defers a second owner
    Given another live process owns the heavy lease
    When a second owner waits for the lease
    Then the second owner is deferred with exit 75
    And the deferral names the process holding the lease

  @e2e-exempt
  Scenario: A long-lived service never holds the heavy-work lease
    Given a live service session owns its resource lease
    When heavy work requests the lease
    Then the heavy owner acquires the lease immediately

  @e2e-exempt
  Scenario: Concurrent services keep their own inheritable sessions
    Given two live service sessions on separate ports
    When each service child validates its inherited session
    Then both inherited sessions remain valid

  @e2e-exempt
  Scenario: An inherited session runs without reacquiring the lease
    Given a valid inherited resource session
    When a guarded child exits successfully
    Then the child exits successfully and the inherited session remains owned

  @e2e-exempt
  Scenario: A failed child keeps its own exit code
    Given an admitted guarded command
    When the guarded child exits with code 17
    Then the guard exits with code 17

  @e2e-exempt
  Scenario: Canonical concurrency remains ecosystem neutral
    Given an admitted command without consumer concurrency mappings
    When the guarded child inspects its environment
    Then only canonical resource guard concurrency variables are added

  @e2e-exempt
  Scenario: Consumers select their concurrency environment mappings
    Given an admitted command with explicit consumer concurrency mappings
    When the guarded child inspects its environment
    Then missing mappings receive resolved concurrency and caller values remain unchanged

  @e2e-exempt
  Scenario: Invalid concurrency mappings fail before child execution
    Given invalid and reserved consumer concurrency mappings
    When each mapped guarded command is requested
    Then every command is rejected before its child starts

  @e2e-exempt
  Scenario: Guarded child streams remain composable
    Given an admitted guarded child with piped input and separate output streams
    When the guarded child copies all three standard streams
    Then stdin reaches the child and stdout and stderr remain separate

  @e2e-exempt
  Scenario: An interrupted guard signals once and then force-stops the child
    Given an admitted child that ignores termination
    When the guard is interrupted
    Then the child is signalled once and force-stopped within the grace

  @e2e-exempt
  Scenario: A supervision failure reaps the guarded child before releasing ownership
    Given an admitted child that ignores termination during a collector failure
    When guarded supervision loses host evidence
    Then the child is reaped before its resource lease is released

  @e2e-exempt
  Scenario: Critical pressure sheds eligible work
    Given an admitted ephemeral child encounters critical pressure
    When the guard observes the critical sample
    Then the guard terminates its child and exits with code 75

  @e2e-exempt
  Scenario: Worsening warning sheds degraded work
    Given an admitted degraded ephemeral child encounters growing compressor pressure
    When the guard observes warning through the grace
    Then the degraded child starts and is terminated with exit 75
