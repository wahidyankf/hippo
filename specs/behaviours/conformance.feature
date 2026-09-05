Feature: Generic consumer conformance
  A caller-supplied manifest verifies four HIPPO consumers without teaching HIPPO their identities.

  @e2e-exempt
  Scenario: Four manifest consumers preserve their checkout state
    Given a strict manifest containing four temporary consumer checkouts
    When the generic consumer conformance harness runs
    Then identity coordination gate and unchanged-checkout assertions pass without compiled product defaults

  @e2e-exempt
  Scenario: Checkout aliases cannot name one consumer twice
    Given four manifest consumers including absolute and symlink aliases of one checkout
    When the conformance manifest is validated
    Then canonical checkout identity is rejected without exposing its path

  @e2e-exempt
  Scenario: Empty HIPPO binary input cannot resolve from the manifest directory
    Given a conformance manifest with an empty raw HIPPO binary path
    When the raw manifest inputs are validated
    Then the empty binary input is rejected privately before path resolution

  @e2e-exempt
  Scenario: Empty shared-root input cannot resolve from the manifest directory
    Given a conformance manifest with an empty raw shared-root path
    When the raw manifest inputs are validated
    Then the empty shared-root input is rejected privately before path resolution

  @e2e-exempt
  Scenario: Empty consumer path cannot resolve to its manifest checkout
    Given a conformance manifest inside a valid checkout with one empty raw consumer path
    When the raw manifest inputs are validated
    Then the empty consumer path is rejected privately before checkout commands

  @e2e-exempt
  Scenario: Every checkout is reconciled after any command failure
    Given four clean consumers whose bootstrap coordination or gate command mutates a checkout and fails
    When each failure phase is exercised independently
    Then all checkouts were snapshotted before commands and every mutation is reported

  @e2e-exempt
  Scenario: Runtime inputs remain canonical across consumer commands
    Given a manifest with relative inputs inherited HIPPO keys and retargetable checkout aliases
    When consumer commands inspect their working directory environment and frozen checkout identity
    Then only canonical manifest inputs are used and shared-root checkout aliases are rejected privately

  @e2e-exempt
  Scenario: Consumer coordination never inherits the conformance caller session
    Given the conformance caller has one live reservation session and one repository default config
    When four consumer commands inspect their base environment and independent owners establish overlapping sessions
    Then no consumer receives the caller token or default config and independent owners still exhaust real capacity

  @e2e-exempt
  Scenario: A validated checkout directory cannot be replaced between phases
    Given a bootstrap replaces its validated checkout directory with a clean checkout at the same path
    When a later gate would run in the replacement with the same commit and clean status
    Then conformance rejects the changed checkout identity before the gate and reconciles conservatively

  @e2e-exempt
  Scenario Outline: The created shared root cannot be replaced between phases
    Given a bootstrap replaces the created shared root with <replacement>
    When a later coordination command would use the replacement root
    Then conformance rejects the changed shared-root identity privately before the command starts

    Examples:
      | replacement     |
      | an empty directory |
      | a symlink          |

  @e2e-exempt
  Scenario: Group retirement waits for the host to reap an already killed group
    Given an owned command group that is signalled after its members exited and reaped only after the stop grace
    When the conformance runner retires that group
    Then retirement reports no failure for a group the host had already stopped

  @e2e-exempt
  Scenario: Cancellation waits for a leader-first descendant group before reconciliation
    Given a conformance leader that exits on TERM while its TERM-ignoring descendant writes a late checkout mutation
    When the conformance caller is cancelled
    Then the full owned command group retires before fresh-deadline reconciliation reports the late mutation boundedly

  @e2e-exempt
  Scenario Outline: Completed commands retire background descendants before reconciliation
    Given a conformance leader that exits <leader_exit> while leaving a TERM-ignoring descendant to write a late checkout mutation
    When the leader exits before its descendant
    Then the full owned command group retires before the completed command is reconciled

    Examples:
      | leader_exit |
      | zero        |
      | nonzero     |

  @e2e-exempt
  Scenario: Force-stop reap failure remains bounded
    Given a cancelled conformance command whose force-stop reap cannot complete
    When cancellation reaches its post-kill observation deadline
    Then conformance returns a private bounded reap failure and still reconciles checkouts

  @e2e-exempt
  Scenario: Verified binary cleanup failures are reported
    Given a consumer makes verified binary storage temporarily unremovable
    When conformance completes command execution and checkout reconciliation
    Then cleanup failure joins the result without exposing the storage path

  @e2e-exempt
  Scenario: Cancellation before a sequential command prevents its start
    Given a bootstrap sequence cancelled after its first command
    When the next command reaches its start boundary
    Then the next command never starts and final identity and checkout reconciliation still run

  @e2e-exempt
  Scenario: A verified HIPPO binary cannot change between command phases
    Given a valid manifest whose bootstrap replaces the verified HIPPO binary identity
    When a later coordination or gate command would execute that binary
    Then conformance fails before changed bytes execute and still reconciles every checkout

  @e2e-exempt
  Scenario: Commands execute one pinned verified HIPPO identity
    Given a valid HIPPO source that changes after command verification
    When the consumer invokes HIPPO through its provided environment
    Then only pinned verified bytes execute and the changed source is reported

  @e2e-exempt
  Scenario: A consumer cannot replace its provided verified HIPPO binary between commands
    Given a valid manifest whose bootstrap overwrites its provided HIPPO binary while the source stays unchanged
    When a later gate would execute the provided binary
    Then conformance rejects the changed pinned object before unverified bytes execute

  @e2e-exempt
  Scenario: Concurrent consumers cannot replace a previously verified HIPPO binary
    Given one gate pauses after verification while another gate overwrites their provided HIPPO binary
    When the paused gate invokes its already-verified binary path
    Then only verified bytes execute and the replacement attempt remains observable

  @e2e-exempt
  Scenario Outline: Capacity skip never hides verified-binary integrity failures
    Given an allow-capacity-skip coordination check that exits 75 after <integrity_failure>
    When conformance classifies the joined coordination outcome
    Then the integrity failure remains fatal and private instead of being skipped

    Examples:
      | integrity_failure       |
      | pinned binary tamper    |
      | verified cleanup failure |

  @e2e-exempt
  Scenario: A HIPPO binary FIFO is rejected boundedly
    Given a manifest whose HIPPO binary identity is a FIFO
    When conformance validates the binary identity
    Then validation rejects the special file without blocking or exposing its path

  @e2e-exempt
  Scenario: A HIPPO binary directory is rejected
    Given a manifest whose HIPPO binary identity is a directory
    When conformance validates the binary identity
    Then validation rejects the directory without exposing its path

  @e2e-exempt
  Scenario: A HIPPO binary must be executable
    Given a manifest whose HIPPO binary is a non-executable regular file
    When conformance validates the binary identity
    Then validation rejects the non-executable file without exposing its path

  @e2e-exempt
  Scenario: Missing command errors remain private
    Given a consumer command with a missing absolute executable
    When conformance attempts to start the command
    Then the missing-command error names only the consumer phase and safe failure category

  @e2e-exempt
  Scenario: Invalid checkout command errors remain private
    Given a consumer command removes its checkout before the next command boundary
    When conformance revalidates the checkout identity after that command
    Then the invalid-checkout error names only the consumer phase and safe identity category

  @e2e-exempt
  Scenario Outline: Conformance setup filesystem errors remain private
    Given a conformance <surface> containing a unique private path sentinel
    When conformance attempts filesystem setup
    Then the setup error names only its safe category and never the private path

    Examples:
      | surface                 |
      | unavailable manifest   |
      | uncreatable shared root |

  @e2e-exempt
  Scenario: Independent bootstrap lanes run concurrently behind one phase barrier
    Given four consumers with sequential local bootstrap commands and multiple failing lanes
    When the generic harness executes the bootstrap phase
    Then consumer lanes overlap failures aggregate deterministically and no later phase starts
