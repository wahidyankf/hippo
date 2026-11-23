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

  # Exemption(e2e): a compiled binary run cannot plant, hold, or time the coordination records that owners and waiters share; alternative-proof: test:integration
  @e2e-exempt
  Scenario: Status exposes privacy-safe labeled owner rows
    Given a labeled reservation owner in the shared queue
    When JSON status is filtered by its source and worktree tag
    Then the matching owner row exposes tier and labels without private paths

  # Exemption(e2e): a compiled binary run cannot age, seed, or count evidence files in its shared root; alternative-proof: test:integration
  @e2e-exempt
  Scenario: History filters current labeled summaries
    Given current summaries from two labeled sources
    When JSON history is filtered to one source for thirty days
    Then only the matching privacy-safe summary is returned

  Scenario Outline: History lists a value this version does not know as recorded
    Given a current summary whose <field> is <value>
    When JSON history is requested for thirty days
    Then it exits 0 and that row's <field> reads <value>

    Examples:
      | field      | value          |
      | outcome    | future-outcome |
      | task class | batch          |

  # Exemption(e2e): a compiled binary run cannot age, seed, or count evidence files in its shared root; alternative-proof: test:integration
  @e2e-exempt
  Scenario: An unreadable history archive is not reported as a refused write
    Given a history archive that is not valid gzip
    When history is queried for thirty days
    Then it exits 125 naming hippo.evidence.unreadable and leaves the archive bytes unchanged

  # Exemption(e2e): requires injected stable host samples and deterministic cancellation timing; alternative-proof: test:quick
  @e2e-exempt
  Scenario: Watch emits only changed admission snapshots
    Given stable host and queue state for watch
    When JSON watch observes two unchanged snapshots and is then interrupted by SIGINT
    Then only one schema five status snapshot is emitted before watch exits 130

  Scenario Outline: A signal ends watch with the signal status
    Given watch has printed its first status snapshot
    When watch receives <signal> while it waits for the next snapshot
    Then watch exits <status> with no hippo diagnostic

    Examples:
      | signal  | status |
      | SIGINT  | 130    |
      | SIGTERM | 143    |

  # Exemption(e2e): requires an injected collector that is interrupted part way through one host sample; alternative-proof: test:quick
  @e2e-exempt
  Scenario Outline: A signal while an observer collects a sample ends it with the signal status
    Given stable host state for <command> sampling
    When <command> is interrupted by SIGINT while it collects a host sample
    Then <command> exits 130 with no hippo diagnostic

    Examples:
      | command |
      | watch   |
      | monitor |

  # Exemption(e2e): requires an injected release capture that observes its own cancellation instead of live health endpoints; alternative-proof: test:integration
  @e2e-exempt
  Scenario Outline: Release monitoring reports a signal but not its own duration
    Given a release capture that runs until it is stopped
    When the release capture is ended by <end>
    Then release monitoring finishes its capture and exits <status>

    Examples:
      | end          | status |
      | SIGTERM      | 143    |
      | its duration | 0      |

  Scenario: JSON status fails closed on corrupt coordination state
    Given the compiled HIPPO binary with corrupt reservation coordination state
    When JSON status is requested for that coordination root
    Then status reports the coordination error instead of schema five zero totals

  # Exemption(e2e): requires a peer holding the shared coordination lock for a bounded window, which no compiled binary invocation can arrange; alternative-proof: test:integration
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

  Scenario: Command discovery lists the public commands in help
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

  Scenario Outline: Requested help wins over every other flag
    Given the compiled HIPPO binary
    When hippo is invoked as "<arguments>"
    Then it prints the help of "<command>" to stdout, nothing to stderr, and exits 0

    Examples:
      | arguments                        | command       |
      | --help                           | hippo         |
      | help run                         | hippo run     |
      | --help --color bogus             | hippo         |
      | --output XML --help              | hippo         |
      | --help --no-such-flag            | hippo         |
      | --no-such-flag --help            | hippo         |
      | run --help --lease-port abc      | hippo run     |
      | run --lease-port abc -h          | hippo run     |
      | run --help --class bogus         | hippo run     |
      | history --since nope --help      | hippo history |
      | release --help --no-such-flag    | hippo release |

  Scenario Outline: A flag mistake without help remains a usage mistake
    Given the compiled HIPPO binary
    When hippo is invoked as "<arguments>"
    Then it exits 2 naming hippo.args.invalid with nothing on stdout

    Examples:
      | arguments                                 |
      | --no-such-flag                            |
      | --color bogus status                      |
      | run --lease-port abc                      |
      | run --lease-port abc -- /bin/sh -c --help |

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

  Scenario: An identity file that is present and invalid is named as invalid
    Given the compiled HIPPO binary
    When run is requested from a directory whose identity file has an unknown field
    Then it exits 125 naming hippo.identity.invalid and the identity file before any payload starts

  Scenario: A run label without any source is a usage mistake
    Given the compiled HIPPO binary
    When run is requested with a tag but no source and no identity file
    Then it exits 2 naming hippo.args.invalid and the --source flag before any payload starts

  Scenario: Only usage errors print the command usage block
    Given the compiled HIPPO binary
    When a runtime failure and a usage error are requested
    Then only the usage error prints the command usage block

  Scenario: Invalid explicit configuration is actionable
    Given an explicit HIPPO config with an unknown field
    When JSON status is requested with that config
    Then configuration fails with exit 125

  Scenario Outline: JSON status says whether the resolved profile may use degraded admission
    Given a configuration whose default profile extends <base>
    When JSON status is requested with that config
    Then status reports degraded admission only for the profile derived from balanced

    Examples:
      | base        |
      | balanced    |
      | constrained |

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

  Scenario: Release summary assessment names rejected evidence as its own limit
    Given a release summary with one health failure
    When release summary assessment is requested through the command line
    Then it exits 124 naming hippo.limit.release-envelope-exceeded, not retryable, and says the evidence was rejected

  Scenario: Release summary assessment reports an unusable summary as unreadable evidence
    Given a release summary file that is not a summary
    When release summary assessment is requested through the command line
    Then it exits 125 naming hippo.evidence.unreadable with nothing on standard output

  Scenario: Release summary assessment accepts standard input
    Given a healthy release summary on standard input
    When release summary assessment is requested from standard input
    Then the release evidence is accepted on standard output

  # Exemption(e2e): requires deterministic repeated and changing host states that cannot be injected through the compiled binary; alternative-proof: test:quick
  @e2e-exempt
  Scenario: Development monitor emits machine-readable transitions
    Given repeated and changing resource states
    When JSON development monitoring is requested
    Then one valid JSON record is emitted for each state transition

  Scenario: Release monitoring requires generic health inputs
    Given release monitor output paths without endpoint inputs
    When release monitoring is requested
    Then the command exits 2 naming hippo.args.invalid for the missing generic health URL

  # Exemption(e2e): requires a monitoring deadline that has already passed when the command starts, which cannot be injected through the compiled binary; alternative-proof: test:quick
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

  # Exemption(e2e): requires injected health probes and deterministic cancellation unavailable to the compiled binary fixture; alternative-proof: test:quick
  @e2e-exempt
  Scenario: Release raw evidence streams to standard output
    Given a bounded release monitor with raw standard output
    When release monitoring completes
    Then raw JSON lines use standard output and the summary remains a file

  # Exemption(e2e): requires injected health probes and deterministic cancellation unavailable to the compiled binary fixture; alternative-proof: test:quick
  @e2e-exempt
  Scenario: Release summary streams to standard output
    Given a bounded release monitor with summary standard output
    When release monitoring completes
    Then the final summary uses standard output and raw evidence remains a file

  Scenario: Release output formats cannot share standard output
    Given release raw evidence and summary both target standard output
    When release monitoring is requested
    Then the command rejects mixed standard output before collecting evidence

  # Exemption(e2e): requires an injected failing writer unavailable through the compiled binary process boundary; alternative-proof: test:integration
  @e2e-exempt
  Scenario: Release streaming propagates downstream failure
    Given a release raw stream whose downstream writer fails
    When release monitoring writes its first sample
    Then monitoring fails without closing the caller-owned stream

  Scenario: The removed TUI command is an unknown command
    Given the compiled HIPPO binary
    When the removed TUI command is requested
    Then the diagnostic names the removed command and exits with code 2

  # Exemption(e2e): requires an owned command/frame fixture that changes display annotations while retaining stable observation evidence; alternative-proof: test:quick
  @e2e-exempt
  Scenario: A display annotation adds no field to status
    Given stable observation evidence whose display annotation changes between requests
    When JSON status and watch inspect that evidence
    Then status retains schema version 5 without display annotation fields

  # Exemption(e2e): requires an owned command/frame fixture that changes display annotations while retaining stable observation evidence; alternative-proof: test:quick
  @e2e-exempt
  Scenario: A changed display annotation does not make watch emit another snapshot
    Given stable observation evidence whose display annotation changes between requests
    When JSON status and watch inspect that evidence
    Then watch emits only the first unchanged schema five snapshot

  # Behaviour: HI-CLI-003
  Scenario: A source build reports the dev version with an unknown commit
    Given a HIPPO build made from source without release stamps
    When JSON version is requested of that build
    Then the version reports dev with the commit unknown

  # Behaviour: HI-CLI-004
  Scenario: The root version flag prints what the version command prints
    Given the compiled HIPPO binary
    When the root version flag is requested
    Then its output matches the version command and exits 0

  # Behaviour: HI-CLI-014
  Scenario Outline: Shell completion is generated for bash, fish, and powershell
    Given the compiled HIPPO binary
    When <shell> completion is requested
    Then a <shell> completion script is emitted

    Examples:
      | shell      |
      | Bash       |
      | Fish       |
      | PowerShell |

  # Behaviour: HI-CLI-034
  Scenario: A history window longer than thirty days is refused with exit 2
    Given the compiled HIPPO binary
    When history is requested for a window of 31 days
    Then it exits 2 naming hippo.args.invalid with nothing on stdout

  # Behaviour: HI-CLI-036
  Scenario: History refuses --json together with --jsonl as a usage mistake
    Given the compiled HIPPO binary
    When history is requested with both --json and --jsonl
    Then it exits 2 naming hippo.args.invalid with nothing on stdout

  # Behaviour: HI-CLI-041
  # Behaviour: HI-EXT-002
  Scenario: A valid history filter that matches no run exits 1
    Given a current summary whose outcome is passed
    When history is requested for thirty days from a source that matches no run
    Then it exits 1 with nothing on standard output

  # Behaviour: HI-CLI-035
  Scenario: History JSON Lines output carries one summary object per line
    Given a current summary whose outcome is passed
    When JSON Lines history is requested for thirty days
    Then each history line is one summary object

  # Behaviour: HI-CLI-083
  Scenario Outline: The failure body names the one flag that caused the failure in error.field
    Given the compiled HIPPO binary
    When hippo is invoked as "<arguments>"
    Then the failure body's error field is <field>

    Examples:
      | arguments                          | field   |
      | --color bogus --output json status | --color |
      | --output json history --since nope | absent  |

  # Behaviour: HI-CLI-086
  # Behaviour: HI-ENV-015
  Scenario Outline: Colour auto colours a diagnostic only with HIPPO_COLOR=1, TERM set, and NO_COLOR absent
    Given the colour environment is <environment>
    When a usage mistake is reported with --color <colour>
    Then the diagnostic is <diagnostic>

    Examples:
      | environment                                | colour | diagnostic |
      | HIPPO_COLOR=1 and TERM set                 | auto   | coloured   |
      | HIPPO_COLOR=1, TERM set, and NO_COLOR set  | auto   | plain      |
      | HIPPO_COLOR=1 and TERM unset               | auto   | plain      |
      | HIPPO_COLOR unset and TERM set             | auto   | plain      |
      | HIPPO_COLOR=1 and TERM set                 | never  | plain      |
      | no colour variables                        | always | coloured   |

  # Behaviour: HI-CLI-087
  Scenario: The JSON failure body is never coloured whatever --color says
    Given the compiled HIPPO binary
    When hippo is invoked as "--color always --output json --no-such-flag"
    Then the JSON failure body parses and holds no escape byte

  # Behaviour: HI-CLI-078
  Scenario Outline: hippo-conformance refuses no argument or several arguments with exit 2
    Given the compiled hippo-conformance tool
    When hippo-conformance is invoked with <arguments>
    Then it exits 2 with the conformance usage line and nothing on stdout

    Examples:
      | arguments     |
      | no argument   |
      | two manifests |

  # Behaviour: HI-CLI-053
  Scenario: Run starts its child in the directory named by --cwd
    Given the compiled HIPPO binary
    When run starts a child in the directory named by --cwd
    Then the child reports the directory named by --cwd

  # Behaviour: HI-CLI-054
  Scenario: Run measures the disk path it is given and stops when that path cannot be measured
    Given the compiled HIPPO binary
    When run is requested with a disk path that cannot be measured
    Then it exits 125 naming hippo.host.unreadable and the child never starts

  # Behaviour: HI-CLI-020
  # Exemption(e2e): a compiled binary run cannot plant, hold, or time the coordination records that owners and waiters share; alternative-proof: test:integration
  @e2e-exempt
  Scenario: Text status prints a summary line and one privacy-safe row per owner and waiter
    Given a labeled reservation owner in the shared queue
    When text status is requested for that shared root
    Then text status prints a summary line and one row for the labeled owner without private paths

  # Behaviour: HI-CLI-031
  # Exemption(e2e): a compiled binary prints only changed snapshots, so a stable host shows no second snapshot whose wait can be timed from outside; alternative-proof: test:integration
  @e2e-exempt
  Scenario Outline: Watch pauses for its interval between snapshots
    Given stable host and queue state for watch
    When watch waits between snapshots with <interval>
    Then the watch pauses for <waited> before its next snapshot

    Examples:
      | interval           | waited |
      | --interval 2s      | 2s     |
      | no --interval flag | 5s     |

  # Behaviour: HI-CLI-042
  # Exemption(e2e): requires deterministic repeated and changing host states that cannot be injected through the compiled binary; alternative-proof: test:integration
  @e2e-exempt
  Scenario: Text monitor prints the initial state and one line per transition
    Given repeated and changing resource states
    When text development monitoring is requested
    Then monitoring prints the initial state and one state line per transition

  # Behaviour: HI-CFG-002
  Scenario Outline: A missing config named by --config or HIPPO_CONFIG exits 125; a missing default falls back
    Given the compiled HIPPO binary
    When JSON status is requested with a missing config named by <source>
    Then status exits <status>

    Examples:
      | source               | status |
      | --config             | 125    |
      | HIPPO_CONFIG         | 125    |
      | HIPPO_DEFAULT_CONFIG | 0      |

  # Behaviour: HI-CFG-001
  # Behaviour: HI-ENV-003
  # Behaviour: HI-ENV-004
  Scenario Outline: Configuration comes from --config, then HIPPO_CONFIG, then HIPPO_DEFAULT_CONFIG
    Given the compiled HIPPO binary
    And configuration files for <sources> each declare their own default profile
    When JSON status is requested with those configuration sources
    Then status reports the profile of the <winner> source

    Examples:
      | sources                                 | winner               |
      | every source                            | --config             |
      | HIPPO_CONFIG and HIPPO_DEFAULT_CONFIG   | HIPPO_CONFIG         |
      | HIPPO_DEFAULT_CONFIG alone              | HIPPO_DEFAULT_CONFIG |

  # Behaviour: HI-CFG-018
  # Behaviour: HI-ERR-008
  Scenario Outline: An unknown --profile exits 125 naming replan-required; an unknown profile in the file names config.unreadable
    Given the compiled HIPPO binary
    And a profile named by <source> that does not exist
    When JSON status is requested for that profile
    Then it exits 125 naming <code>

    Examples:
      | source                     | code                         |
      | --profile flag             | hippo.policy.replan-required |
      | defaultProfile in the file | hippo.config.unreadable      |

  # Behaviour: HI-EXT-004
  # Behaviour: HI-ERR-002
  # Exemption(e2e): a compiled binary cannot be made to fault inside its own sampler, so an unhandled fault cannot be injected through it; alternative-proof: test:integration
  @e2e-exempt
  Scenario: An internal fault exits 2 naming hippo.internal.failure
    Given a host sampler that faults during a reading
    When JSON status is requested with that sampler
    Then it exits 2 naming hippo.internal.failure

  # Behaviour: HI-CFG-020
  Scenario: JSON status reports the SHA-256 of the configuration file it loaded
    Given the compiled HIPPO binary
    And a schema two configuration file
    When JSON status is requested with that configuration file
    Then status reports the SHA-256 of that file's bytes as its configHash

  # Behaviour: HI-ENV-005
  Scenario: A run takes its identity from HIPPO_IDENTITY ahead of HIPPO_DEFAULT_IDENTITY
    Given the compiled HIPPO binary
    And HIPPO_IDENTITY names an identity from source fixture-identity and HIPPO_DEFAULT_IDENTITY names one from source default-identity
    When a guarded run of a passing command is requested without a source flag
    Then the history of that run names fixture-identity and does not name default-identity

  # Behaviour: HI-ENV-008
  # Exemption(e2e): a compiled monitor reports no endpoint it was given, and a live health service that would show them cannot be fixed by the scenario; alternative-proof: test:integration
  @e2e-exempt
  Scenario: HIPPO_HEALTH_URL and HIPPO_ROUTED_ORIGIN supply the release monitor flags they default
    Given HIPPO_HEALTH_URL names a health endpoint and HIPPO_ROUTED_ORIGIN names a routed origin
    When release monitoring is requested with output paths and no endpoint flags
    Then the release monitor is given the health endpoint and the routed origin those variables name

  # Behaviour: HI-CLI-076
  Scenario: Release monitoring counts a listed service port in its memory samples
    Given a service listening on a port the release monitor is asked to count
    When release monitoring is requested with that port as its service port
    Then the release summary reports the memory of that service as above zero

  # Behaviour: HI-ERR-020
  # Exemption(e2e): requires synthetic disk, capacity, and critical pressure evidence that the compiled binary cannot inject; alternative-proof: test:integration
  @e2e-exempt
  Scenario Outline: Only capacity deferral and pressure shed are retryable in the failure body
    Given <condition>
    When the run is refused and its failure body is read
    Then the failure body names <code> with retryable <retryable>

    Examples:
      | condition                                                    | code                          | retryable |
      | a host whose disk is below the storage floor                 | hippo.limit.storage-blocked   | false     |
      | a shared root whose capacity never frees for a waiting owner | hippo.limit.capacity-deferred | true      |
      | an admitted child that critical host pressure sheds          | hippo.limit.pressure-shed     | true      |
      | a mistyped flag on the run command                           | hippo.args.invalid            | false     |

  # Behaviour: HI-EVD-011
  # Exemption(e2e): requires synthetic disk and critical pressure samples that the compiled binary cannot inject; alternative-proof: test:integration
  @e2e-exempt
  Scenario Outline: Status JSON publishes the legacy integer exit code for a stopped profile
    Given <host>
    When JSON status is requested for that host
    Then the profile publishes exitCode <code> as an integer

    Examples:
      | host                                                          | code |
      | a status host with its disk below the storage floor           | 73   |
      | a status host whose capacity is deferred by critical pressure  | 75   |

  # Behaviour: HI-ENV-002
  # Exemption(e2e): the default root is observable only through evidence a run writes into the host's real home; alternative-proof: test:quick
  @e2e-exempt
  Scenario Outline: The default state root is under XDG_STATE_HOME or HOME, or the Library directory on macOS
    Given <state>
    When the default state root is resolved with HIPPO_ROOT unset
    Then the root is the hippo directory the host platform names for that state

    Examples:
      | state                                              |
      | the XDG_STATE_HOME variable names a state directory |
      | the XDG_STATE_HOME variable is unset               |
