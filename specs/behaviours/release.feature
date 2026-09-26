Feature: Release resource ownership
  Release orchestration delegates host resource decisions and monitoring to the Go guard.

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
      | condition                                   | status | code                          | reason                                              |
      | memory pressure never clears                | 124    | hippo.limit.capacity-deferred | memory pressure does not leave safe release headroom |
      | CPU use never settles                       | 124    | hippo.limit.capacity-deferred | CPU use does not leave release and safety headroom  |
      | free disk is below the release reserve      | 124    | hippo.limit.storage-blocked   | release disk reserve is unavailable                 |
      | host evidence stops after the first sample  | 125    | hippo.supervision.failed      | injected host evidence failure                      |

  Scenario: Release overlap rejects failed health evidence
    Given a release summary with one health failure
    When release overlap evidence is assessed
    Then the release evidence is rejected

  Scenario: Release overlap rejects an unresponsive routed journey
    Given a release summary outside the routed responsiveness budget
    When release overlap evidence is assessed
    Then the release evidence is rejected
