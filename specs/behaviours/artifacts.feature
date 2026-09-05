Feature: HIPPO build artifacts
  Compiled development artifacts stay bounded and outside repository history.

  @e2e-exempt
  Scenario: Release builds stay outside repository history
    When build artifact tracking is inspected
    Then generated release binaries are ignored

  @e2e-exempt
  Scenario: End-to-end binaries are temporary
    When its compiled binary lifecycle is inspected
    Then the end-to-end binary is removed after the run

  @e2e-exempt
  Scenario: Bootstrap cache retention is bounded
    Given four historical bootstrap generations
    When the current bootstrap generation runs
    Then only the current and two recent generations remain

  @e2e-exempt
  Scenario: Machine-local configuration and binaries stay private
    When tracked and ignored paths are inspected
    Then local config, generated artifacts, and generated-reports are ignored and untracked
    And the local config example remains tracked
    And the standalone layout has no Nx metadata

  @e2e-exempt
  Scenario: Release versions use exact semantic syntax
    Given malformed and exact semantic release versions
    When each release version is validated independently
    Then only exact v-prefixed semantic versions pass

  @e2e-exempt
  Scenario: Release commits are full lowercase real commits
    Given short uppercase nonexistent and full real release commits
    When each release commit is validated independently
    Then only a full lowercase real commit passes

  @e2e-exempt
  Scenario: Release commit equals checkout HEAD
    Given a clean fixture repository with a real non-HEAD commit
    When a release is requested for the non-HEAD commit
    Then the release is rejected before output

  @e2e-exempt
  Scenario: Release checkout has no tracked changes
    Given a fixture repository with one tracked change
    When its release identity is validated
    Then tracked dirt rejects the release

  @e2e-exempt
  Scenario: Release checkout has no untracked changes
    Given a fixture repository with one untracked file
    When its release identity is validated
    Then untracked dirt rejects the release

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

  @e2e-exempt
  Scenario: Release cleanliness inspection fails closed
    Given a clean release identity whose git status command fails
    When release cleanliness is inspected
    Then the release fails before creating output

  @e2e-exempt
  Scenario: Release builds use only exact committed source
    Given clean committed source shadowed by ignored global and repository-excluded Go files
    When release binaries are built from the checkout identity
    Then only exact committed source affects assets and isolated build state is removed

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

  @e2e-exempt
  Scenario: Release policy inventories every version-four source
    Given the version-four runtime source and conformance inventory
    When repository artifact policy is executed
    Then every version-four source and example is explicitly protected

  @e2e-exempt
  Scenario: Release assets have one exact platform set
    Given one clean exact release identity
    When its platform asset names and checksums are validated
    Then exactly four supported archives and four matching checksums exist

  @e2e-exempt
  Scenario: Every release archive has one executable member
    Given one release archive for every supported platform
    When each archive member and mode is inspected
    Then each archive contains only an executable hippo member

  @e2e-exempt
  Scenario: Release archives have normalized ownership metadata
    Given release archives with nonzero numeric or noncanonical named ownership
    When each archive header is inspected without extraction
    Then ownership must be uid zero gid zero and root root

  @e2e-exempt
  Scenario: Release archive members cannot redirect through unsafe types
    Given one-member release archives whose hippo entries are symbolic links hard links and special files
    When archive metadata is validated before extraction
    Then every unsafe member is rejected without following its host target

  @e2e-exempt
  Scenario: Release binaries carry one clean source identity
    Given cross-platform release binaries from one clean commit
    When VCS metadata and the native version command are inspected
    Then every binary has that revision and clean state and the native JSON matches

  @e2e-exempt
  Scenario: Native release identity works from a path containing spaces
    Given valid release assets in a directory whose path contains spaces
    When the native version command is executed
    Then its quoted binary path emits the exact release identity

  @e2e-exempt
  Scenario: Release workflow peels annotated and lightweight tags
    Given reachable annotated and lightweight release tags
    When the release workflow resolves each tag identity
    Then each tag is compared by its peeled commit

  @e2e-exempt
  Scenario: Release workflow requires the peeled tag to equal the event commit
    Given a reachable release tag and a different reachable event commit
    When the release workflow compares their identities
    Then the mismatched event commit is rejected before publishing

  @e2e-exempt
  Scenario: Release workflow requires origin main ancestry
    Given reachable and unreachable peeled release commits
    When the release workflow applies its ancestry gate
    Then only the origin-main-reachable commit passes

  @e2e-exempt
  Scenario: Release workflow adds no Actions storage
    Given the repository Actions storage policy
    When release workflow actions and permissions are inspected
    Then release publishing adds no artifact package or cache storage

  @e2e-exempt
  Scenario: Release workflows disable implicit Go module caching
    Given release asset jobs that configure Go
    When their setup-go storage configuration is inspected
    Then each release asset job explicitly disables Go caching

  @e2e-exempt
  Scenario: CI validates assets against the exact build commit
    Given the CI release asset build and validation steps
    When their command arguments are compared
    Then the validator receives the same exact commit as the builder
