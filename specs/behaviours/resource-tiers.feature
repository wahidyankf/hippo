Feature: Optional very-heavy resource tier
  A schema-3 configuration may declare a fourth, last-resort resource tier. A run may name that tier only where its
  loaded configuration declares it, and every other run keeps its behaviour.

  Scenario: A declared very-heavy tier admits a run and records that tier
    Given a schema three configuration that declares the very-heavy tier within the pool
    When a guarded run requests the very-heavy tier with a command that exits zero
    Then the tier run exits 0
    And the tier history row reads very-heavy with outcome passed
    And the very-heavy tier queue deadline is four hours

  Scenario: A declared very-heavy tier refuses an explicit CPU outside its bounds
    Given a schema three configuration that declares the very-heavy tier within the pool
    When a guarded run requests the very-heavy tier with an explicit CPU of three and a command that exits zero
    Then the tier run exits 125 naming hippo.policy.replan-required with "explicit CPU is outside the very-heavy tier"
    And the tier payload did not run and no history row was written

  Scenario Outline: An undeclared very-heavy tier is refused before any work starts
    Given <configuration>
    When a guarded run requests the very-heavy tier with output json and a command that exits zero
    Then the tier run exits 2 naming hippo.args.invalid with "does not declare the very-heavy tier"
    And the tier run reports exit code 2 and retryable false in its JSON error
    And the tier payload did not run and no history row was written

    Examples:
      | configuration                                             |
      | a schema three configuration without the very-heavy tier |
      | a schema two configuration                               |
      | a schema one configuration                               |
      | no configuration file                                    |

  Scenario: A misspelled very-heavy tier name is refused with the closed set of tiers
    Given a schema three configuration that declares the very-heavy tier within the pool
    When a guarded run requests the tier very_heavy with a command that exits zero
    Then the tier run exits 2 naming hippo.args.invalid with "light, standard, heavy, or very-heavy"
    And the tier payload did not run and no history row was written

  Scenario Outline: A declared very-heavy tier that the pool cannot hold leaves the configuration unreadable
    Given a schema three configuration whose very-heavy tier is <declaration>
    When a guarded run requests the light tier with a command that exits zero
    Then the tier run exits 125 naming hippo.config.unreadable with "<message>"
    And the tier payload did not run and no history row was written

    Examples:
      | declaration                             | message                                               |
      | a maximum CPU above the pool            | schema 3 very-heavy resource tier is outside the pool |
      | null                                    | schema 3 very-heavy resource tier is outside the pool |
      | an empty object                         | schema 3 very-heavy resource tier is outside the pool |
      | a queue deadline that is not a duration | schema 3 very-heavy queue deadline is invalid         |
      | a key spelled with an underscore        | schema 3 contains an unknown resource tier            |

  Scenario: A configuration without the very-heavy tier admits its required tiers as before
    Given a schema three configuration without the very-heavy tier
    When a guarded run requests the light tier with a command that exits zero
    Then the tier run exits 0

  Scenario Outline: The history filter accepts the very-heavy tier whatever the configuration declares
    Given <configuration>
    When history is filtered by the very-heavy tier
    Then the tier history filter is not refused as a usage mistake

    Examples:
      | configuration                                             |
      | a schema three configuration without the very-heavy tier |
      | a schema three configuration that declares the very-heavy tier within the pool |
      | no configuration file                                    |
