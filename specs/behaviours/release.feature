Feature: Release resource ownership
  Release orchestration delegates host resource decisions and monitoring to the Go guard.
  The CPU ceiling is a proxy for routed responsiveness. A host with at most two CPUs cannot reserve two whole CPUs, so
  there the directly measured routed-journey budget is the guard instead, and only routed evidence from schema 4 or
  later can stand in for the ceiling. Swap pressure counts swap-outs only: a swap-in reads back a page that was already
  evicted and does not show the release forcing memory out, whereas a swap-out under low available memory does.

  @e2e-exempt
  Scenario: Release admission preserves the requested capacity envelope
    Given a release host below its requested balanced capacity
    When release admission is assessed
    Then the release requires replanning instead of automatic fallback

  @e2e-exempt
  Scenario Outline: A failed release stability check names its own reason in one line
    Given a release host whose <condition>
    When release admission is checked through the command line
    Then it exits <status> naming <code> in one diagnostic line that says <reason>

    Examples:
      | condition                                                        | status | code                          | reason                                               |
      | memory pressure never clears                                     | 124    | hippo.limit.capacity-deferred | memory pressure does not leave safe release headroom |
      | CPU use never settles                                            | 124    | hippo.limit.capacity-deferred | CPU use does not leave release and safety headroom   |
      | free disk is below the release reserve                           | 124    | hippo.limit.storage-blocked   | release disk reserve is unavailable                  |
      | sampling fails with an unclassified error after the first sample | 125    | hippo.supervision.failed      | injected host evidence failure                       |

  Scenario: Release overlap rejects failed health evidence
    Given a release summary with one health failure
    When release overlap evidence is assessed
    Then the release evidence is rejected

  Scenario: Release overlap rejects an unresponsive routed journey
    Given a release summary outside the routed responsiveness budget
    When release overlap evidence is assessed
    Then the release evidence is rejected

  Scenario: Release overlap accepts a two-CPU host with busy CPU and healthy routed journeys
    Given a two-CPU release summary with fully busy CPU and healthy routed journeys
    When release overlap evidence is assessed
    Then the release evidence is accepted

  Scenario: Release overlap rejects a two-CPU host whose routed journey is outside the budget
    Given a two-CPU release summary with busy CPU and a routed journey outside the routed responsiveness budget
    When release overlap evidence is assessed
    Then the release evidence is rejected

  Scenario: Release overlap keeps the CPU ceiling for a two-CPU host without routed evidence
    Given a two-CPU schema 3 release summary with busy CPU
    When release overlap evidence is assessed
    Then the release evidence is rejected

  Scenario: Release overlap keeps the CPU ceiling for a four-CPU host
    Given a four-CPU release summary with CPU use above its fifty percent ceiling
    When release overlap evidence is assessed
    Then the release evidence is rejected

  Scenario: Release overlap accepts swap-ins with low available memory
    Given a release summary with swap-ins and under 4 GiB available memory
    When release overlap evidence is assessed
    Then the release evidence is accepted

  Scenario: Release overlap rejects swap-outs with low available memory
    Given a release summary with swap-outs and under 4 GiB available memory
    When release overlap evidence is assessed
    Then the release evidence is rejected

  Scenario: Release overlap accepts swap-outs with 4 GiB available memory
    Given a release summary with swap-outs and 4 GiB available memory
    When release overlap evidence is assessed
    Then the release evidence is accepted
