Feature: Public HIPPO CLI
  Any repository can inspect and invoke the standalone HIPPO before its build system starts.

  Scenario: Version identifies the exact build
    Given the compiled HIPPO binary
    When JSON version is requested
    Then version schema identifies the release and commit

  Scenario: JSON status exposes the stable evidence schema
    Given the compiled HIPPO binary
    When JSON status is requested for an existing path
    Then status returns schema version 5 with profile capability and coordination evidence

  Scenario: Status exposes live exclusive compatibility owners
    Given a live exclusive compatibility owner in an isolated shared root
    When JSON status inspects that exclusive shared root
    Then status reports one legacy owner and preserves the compatibility state

  Scenario Outline: Status classifies invalid exclusive compatibility state
    Given a live exclusive compatibility owner with a <state> session document
    When JSON status inspects that invalid exclusive shared root
    Then status exits <code> without changing the invalid compatibility state

    Examples:
      | state     | code |
      | malformed | 125  |
      | future    | 125  |

  @e2e-exempt
  Scenario: Status exposes privacy-safe labeled owner rows
    Given a labeled reservation owner in the shared queue
    When JSON status is filtered by its source and worktree tag
    Then the matching owner row exposes tier and labels without private paths

  @e2e-exempt
  Scenario: History filters current labeled summaries
    Given current summaries from two labeled sources
    When JSON history is filtered to one source for thirty days
    Then only the matching privacy-safe summary is returned

  @e2e-exempt
  Scenario: An unreadable history archive is not reported as a refused write
    Given a history archive that is not valid gzip
    When history is queried for thirty days
    Then it exits 125 naming hippo.evidence.unreadable and leaves the archive bytes unchanged

  @e2e-exempt
  Scenario: Watch emits only changed admission snapshots
    Given stable host and queue state for watch
    When JSON watch observes two unchanged snapshots
    Then only one schema five status snapshot is emitted

  Scenario: JSON status fails closed on corrupt coordination state
    Given the compiled HIPPO binary with corrupt reservation coordination state
    When JSON status is requested for that coordination root
    Then status reports the coordination error instead of schema five zero totals

  @e2e-exempt
  Scenario: JSON status waits out a busy coordination root
    Given a coordination root whose lock is briefly held by a peer
    When JSON status is requested while that peer still holds the lock
    Then status reports its coordination totals instead of a contention error

  Scenario: JSON status gives active coordination markers precedence
    Given schema one and schema two configs paired with opposite active markers
    When JSON status is requested for each marked and empty root
    Then active marker modes win symmetrically and empty roots use configured modes

  Scenario: JSON status rejects overflowing queued demand
    Given the compiled HIPPO binary with individually valid overflowing reservation waiters
    When JSON status aggregates the queued reservation demand
    Then status reports a privacy-safe coordination error instead of wrapped totals

  Scenario: Command discovery uses Cobra help
    Given the compiled HIPPO binary
    When root command help is requested
    Then help lists the public command tree and exits successfully

  Scenario: Help identifies the HIPPO acronym
    Given the compiled HIPPO binary
    When root command help is requested
    Then help expands HIPPO as Host Infrastructure Pressure and Process Orchestrator

  Scenario: Help states the exit-code contract
    Given the compiled HIPPO binary
    When root command help is requested
    Then each line of the help's exit block matches the exit-code reference

  Scenario: Release discovery uses grouped help
    Given the compiled HIPPO binary
    When release command help is requested
    Then help lists the release command tree and exits successfully

  Scenario: Shell completion is generated on demand
    Given the compiled HIPPO binary
    When Zsh completion is requested
    Then a Zsh completion script is emitted

  Scenario: An unknown command names itself and exits as a usage mistake
    Given the compiled HIPPO binary
    When an unknown command is requested
    Then the diagnostic names the command and exits with code 2

  Scenario: A command group refuses an unknown or missing subcommand
    Given the compiled HIPPO binary
    When command groups are requested with an unknown or missing subcommand
    Then each exits 2 naming hippo.args.invalid with its usage on stderr and nothing on stdout

  Scenario: A flag value a command cannot accept is a usage mistake
    Given the compiled HIPPO binary
    When commands are requested with flag values they cannot accept
    Then each exits 2 naming hippo.args.invalid with only its diagnostic before any payload starts

  Scenario: Arguments after the separator belong to the guarded command
    Given the compiled HIPPO binary
    When run guards a missing command whose own arguments ask for JSON output and colour
    Then HIPPO reports its failure as one plain diagnostic line without a failure body

  Scenario: The release monitor output flag is not the global output format
    Given the compiled HIPPO binary
    When release monitoring is refused with its raw output flag set to json
    Then the refusal carries no machine-readable failure body

  Scenario: A failure body names the command wherever the global flags are placed
    Given the compiled HIPPO binary
    When history usage mistakes ask for JSON output before and after the command name
    Then every failure body names hippo history as the command

  Scenario: Only usage errors print the command usage block
    Given the compiled HIPPO binary
    When a runtime failure and a usage error are requested
    Then only the usage error prints the command usage block

  Scenario: Invalid explicit configuration is actionable
    Given an explicit HIPPO config with an unknown field
    When JSON status is requested with that config
    Then configuration fails with exit 125

  Scenario: Run validates its command boundary
    Given the compiled HIPPO binary
    When run is requested without a command separator
    Then the command fails with a useful validation error

  Scenario: Commands without operands reject positional arguments
    Given the compiled HIPPO binary
    When operand-free commands are requested with positional arguments
    Then every command rejects the unexpected argument with its own usage

  Scenario: Release summary assessment accepts healthy evidence
    Given a healthy release summary file
    When release summary assessment is requested
    Then the release evidence is accepted

  Scenario: Release summary assessment accepts standard input
    Given a healthy release summary on standard input
    When release summary assessment is requested from standard input
    Then the release evidence is accepted on standard output

  @e2e-exempt
  Scenario: Development monitor emits machine-readable transitions
    Given repeated and changing resource states
    When JSON development monitoring is requested
    Then one valid JSON record is emitted for each state transition

  Scenario: Release monitoring requires generic health inputs
    Given release monitor output paths without endpoint inputs
    When release monitoring is requested
    Then the command exits 2 naming hippo.args.invalid for the missing generic health URL

  @e2e-exempt
  Scenario: Release monitoring refuses missing health inputs before its deadline
    Given release monitor output paths without endpoint inputs
    And a monitoring deadline that has already passed
    When the release monitor starts
    Then the command exits 2 naming hippo.args.invalid for the missing generic health URL

  Scenario Outline: Release monitoring refuses a usage mistake before collecting evidence
    Given release monitor inputs with <mistake>
    When release monitoring is requested
    Then the command exits 2 naming hippo.args.invalid before collecting evidence

    Examples:
      | mistake                      |
      | a malformed health URL       |
      | a malformed routed origin    |
      | no routed origin             |
      | no output path               |
      | no summary path              |
      | no deployment root           |
      | a negative duration          |
      | an out-of-range service port |

  @e2e-exempt
  Scenario: Release raw evidence streams to standard output
    Given a bounded release monitor with raw standard output
    When release monitoring completes
    Then raw JSON lines use standard output and the summary remains a file

  @e2e-exempt
  Scenario: Release summary streams to standard output
    Given a bounded release monitor with summary standard output
    When release monitoring completes
    Then the final summary uses standard output and raw evidence remains a file

  Scenario: Release output formats cannot share standard output
    Given release raw evidence and summary both target standard output
    When release monitoring is requested
    Then the command rejects mixed standard output before collecting evidence

  @e2e-exempt
  Scenario: Release streaming propagates downstream failure
    Given a release raw stream whose downstream writer fails
    When release monitoring writes its first sample
    Then monitoring fails without closing the caller-owned stream
