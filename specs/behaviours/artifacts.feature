Feature: HIPPO build artifacts
  Compiled development artifacts stay bounded and outside repository history.
  # Exemption(e2e): cache retention is owned by the wrapper outside the compiled binary boundary; alternative-proof: test:quick
  @e2e-exempt
  Scenario: Bootstrap cache retention is bounded
    Given four historical bootstrap generations
    When the current bootstrap generation runs
    Then only the current and two recent generations remain
  # Exemption(e2e): a compiled binary run cannot be given an isolated Git history with a chosen commit, tag, or dirty state; alternative-proof: test:integration
  @e2e-exempt
  Scenario: Release versions use exact semantic syntax
    Given malformed and exact semantic release versions
    When each release version is validated independently
    Then only exact v-prefixed semantic versions pass

  # Exemption(e2e): requires isolated valid and invalid Git object identities; alternative-proof: test:integration
  @e2e-exempt
  Scenario: Release commits are full lowercase real commits
    Given short uppercase nonexistent and full real release commits
    When each release commit is validated independently
    Then only a full lowercase real commit passes

  # Exemption(e2e): requires isolated history with a real non-HEAD commit; alternative-proof: test:integration
  @e2e-exempt
  Scenario: Release commit equals checkout HEAD
    Given a clean fixture repository with a real non-HEAD commit
    When a release is requested for the non-HEAD commit
    Then the release is rejected before output

  # Exemption(e2e): a compiled binary run cannot be given an isolated checkout with tracked or untracked dirt; alternative-proof: test:integration
  @e2e-exempt
  Scenario Outline: A release checkout with tracked or untracked changes is rejected
    Given a fixture repository with <change>
    When its release identity is validated
    Then <dirt> dirt rejects the release

    Examples:
      | change             | dirt      |
      | one tracked change | tracked   |
      | one untracked file | untracked |

  # Exemption(e2e): a compiled binary run cannot be given an isolated Git history with a chosen commit, tag, or dirty state; alternative-proof: test:integration
  @e2e-exempt
  Scenario Outline: Every invalid release identity creates no output
    Given invalid release identity <identity>
    When its output boundary is inspected after rejection
    Then no release output directory exists

    Examples:
      | identity           |
      | version syntax     |
      | commit syntax      |
      | nonexistent commit |
      | non-HEAD commit    |
      | tracked dirt       |
      | untracked dirt     |
      | git status failure |

  # Exemption(e2e): requires an injected Git status failure in isolated release history; alternative-proof: test:integration
  @e2e-exempt
  Scenario: Release cleanliness inspection fails closed
    Given a clean release identity whose git status command fails
    When release cleanliness is inspected
    Then the release fails before creating output

  # Exemption(e2e): a compiled binary run cannot be given an isolated Git history with a chosen commit, tag, or dirty state; alternative-proof: test:integration
  @e2e-exempt
  Scenario: Release builds use only exact committed source
    Given clean committed source shadowed by ignored global and repository-excluded Go files
    When release binaries are built from the checkout identity
    Then only exact committed source affects assets and isolated build state is removed

  # Exemption(e2e): requires a recording Go toolchain and host Go settings around fixture release builds; alternative-proof: test:integration
  @e2e-exempt
  Scenario: Release builds use the Go release go.mod names
    Given host Go settings that name another toolchain and other build flags
    When its release binaries are built
    Then every build runs with the go.mod Go release and none of the host's Go settings

  # Exemption(e2e): requires two fixture release builds from binaries stamped at controlled times; alternative-proof: test:integration
  @e2e-exempt
  Scenario: Rebuilding a release commit reproduces its archives
    Given one release commit built twice from binaries stamped at different times
    When the two release outputs are compared
    Then their archives and checksums are byte-identical

  # Exemption(e2e): requires controlled release staging success and failure paths; alternative-proof: test:integration
  @e2e-exempt
  Scenario Outline: Release temporary materialization is always removed
    Given a controlled temporary directory and a release build that <result>
    When the release build finishes
    Then no source or binary staging directory remains

    Examples:
      | result                    |
      | succeeds                  |
      | build fails               |
      | second allocation fails   |
  # Exemption(e2e): requires a complete cross-platform archive build and checksum inventory; alternative-proof: test:integration
  @e2e-exempt
  Scenario: Release assets have one exact platform set
    Given one clean exact release identity
    When its platform asset names and checksums are validated
    Then exactly four supported archives and four matching checksums exist

  # Exemption(e2e): a compiled binary run cannot build or open a set of cross-platform release archives on demand; alternative-proof: test:integration
  @e2e-exempt
  Scenario: Every release archive has one executable member
    Given one release archive for every supported platform
    When each archive member and mode is inspected
    Then each archive contains only an executable hippo member

  # Exemption(e2e): a compiled binary run cannot build or open a set of cross-platform release archives on demand; alternative-proof: test:integration
  @e2e-exempt
  Scenario: Release archives have normalized ownership metadata
    Given release archives with nonzero numeric or noncanonical named ownership
    When each archive header is inspected without extraction
    Then ownership must be uid zero gid zero and root root

  # Exemption(e2e): a compiled binary run cannot build or open a set of cross-platform release archives on demand; alternative-proof: test:integration
  @e2e-exempt
  Scenario: Release archive members cannot redirect through unsafe types
    Given one-member release archives whose hippo entries are symbolic links hard links and special files
    When archive metadata is validated before extraction
    Then every unsafe member is rejected without following its host target

  # Exemption(e2e): a compiled binary run cannot build or open a set of cross-platform release archives on demand; alternative-proof: test:integration
  @e2e-exempt
  Scenario: Release binaries carry one clean source identity
    Given cross-platform release binaries from one clean commit
    When VCS metadata and the native version command are inspected
    Then every binary has that revision and clean state and the native JSON matches

  # Exemption(e2e): requires executing a native archive member from a controlled spaced path; alternative-proof: test:integration
  @e2e-exempt
  Scenario: Native release identity works from a path containing spaces
    Given valid release assets in a directory whose path contains spaces
    When the native version command is executed
    Then its quoted binary path emits the exact release identity

  # Behaviour: HI-DOC-003
  # Behaviour: HI-ENV-006
  # Exemption(e2e): the bootstrap wrapper that compiles the binary runs outside the compiled binary boundary; alternative-proof: test:integration
  @e2e-exempt
  Scenario: The bootstrap reuses its compiled binary from HIPPO_BUILD_CACHE on a second run
    Given an empty build cache named by HIPPO_BUILD_CACHE
    When the bootstrap is run twice in a row
    Then both runs exit 0 and only the first run compiles the binary
