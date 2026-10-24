Feature: Portable host evidence
  Host-specific signals are normalized without requiring unavailable capabilities.

  @e2e-exempt
  Scenario: Linux cgroup memory limits host capacity
    Given Linux host and cgroup memory evidence
    When the Linux evidence is collected
    Then effective memory is 4 GiB

  @e2e-exempt
  Scenario: Linux reclaimable file cache stays available
    Given a Linux cgroup whose usage is mostly inactive file cache
    When the Linux evidence is collected
    Then available memory excludes the inactive file cache

  @e2e-exempt
  Scenario: Linux without swap remains usable
    Given Linux reports no usable swap
    When development pressure is assessed
    Then swap is unavailable without causing critical pressure

  @e2e-exempt
  Scenario: Linux PSI detects active memory contention
    Given Linux memory PSI some average is 10 percent
    When development pressure is assessed
    Then the state is warning because of memory PSI

  @e2e-exempt
  Scenario: Linux swap counts toward the allowance when counting is on
    Given a Linux host with 7618 MiB of memory and 1 GiB of available memory
    And that host has 8 GiB of swap of which 1 GiB is in use
    And swap counting is on
    When development pressure is assessed
    Then the swap allowance is 8 GiB
    And the swap headroom is 7 GiB

  @e2e-exempt
  Scenario Outline: The swap allowance is the smaller of memory and swap, rounded up to whole GiB
    Given a Linux host with <memory> of memory and 1 GiB of available memory
    And that host has <swap> of swap
    And swap counting is on
    When development pressure is assessed
    Then the swap allowance is <allowance>

    Examples:
      | memory   | swap  | allowance |
      | 7618 MiB | 2 GiB | 2 GiB     |
      | 7618 MiB | 8 GiB | 8 GiB     |
      | 4 GiB    | 8 GiB | 4 GiB     |

  @e2e-exempt
  Scenario: A Linux host without swap has no swap allowance
    Given a Linux host with 4 GiB of memory and 1 GiB of available memory
    And that host has no swap
    And swap counting is on
    When development pressure is assessed
    Then the swap allowance is 0 bytes

  @e2e-exempt
  Scenario: A Linux host whose swap cannot be read has no swap allowance
    Given a Linux host with 4 GiB of memory and 1 GiB of available memory
    And that host has swap that is unreadable
    And swap counting is on
    When development pressure is assessed
    Then the swap allowance is 0 bytes

  @e2e-exempt
  Scenario: A macOS host never counts swap
    Given a macOS host with 4 GiB of memory and 1 GiB of available memory
    And that host has 8 GiB of swap of which 1 GiB is in use
    And swap counting is on
    When development pressure is assessed
    Then the swap allowance is 0 bytes

  @e2e-exempt
  Scenario: Swap is not counted when the setting is off
    Given a Linux host with 4 GiB of memory and 1 GiB of available memory
    And that host has 8 GiB of swap of which 1 GiB is in use
    When development pressure is assessed
    Then the swap allowance is absent
