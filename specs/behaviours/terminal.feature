Feature: Interactive terminal ownership
  A guarded process group can read inherited terminal input without losing group-isolated cleanup.

  Scenario: Interactive guarded child owns the terminal while it runs
    Given a guarded process with inherited controlling-terminal input
    When the child reads from the terminal in its own process group
    Then it completes without SIGTTIN and original foreground ownership is restored

  @tui-terminal
  Scenario Outline: TUI refuses unsupported terminal surfaces before observation
    Given TUI has <surface>
    When TUI is requested without help
    Then it exits 2 naming hippo.args.invalid without entering terminal mode
    And no configuration host or coordination observation is requested

    Examples:
      | surface                    |
      | non-terminal input         |
      | non-terminal output        |
      | TERM set to dumb           |

  @tui-terminal
  Scenario: Terminal capability inspection failures are supervision failures
    Given TUI terminal capability inspection fails
    When TUI is requested
    Then it exits 125 with a privacy-safe diagnostic before terminal entry or observation

  @tui-terminal
  Scenario Outline: Quit works from every TUI surface
    Given TUI is displaying <surface> in an isolated terminal
    When the user presses q
    Then TUI exits 0 after cancelling and joining its observation work
    And the original terminal modes and foreground ownership are restored
    And no guarded job is cancelled or retired

    Examples:
      | surface             |
      | dashboard           |
      | job detail          |
      | capacity detail     |
      | help                |
      | minimum-size notice |

  @tui-terminal
  Scenario Outline: TUI interruption preserves the signal status and terminal state
    Given TUI is observing in an isolated terminal
    When TUI receives <interruption>
    Then TUI exits <code> without a HIPPO failure diagnostic
    And observation is cancelled and joined before the runner returns
    And the original terminal modes and foreground ownership are restored
    And no guarded job is cancelled or retired

    Examples:
      | interruption | code |
      | raw Ctrl-C   | 130  |
      | SIGINT       | 130  |
      | SIGTERM      | 143  |

  @e2e-exempt
  @tui-terminal
  Scenario Outline: Terminal failures restore ownership before reporting failure
    Given TUI encounters a <failure> failure in an isolated terminal
    When the terminal session finishes
    Then it exits 125 with one privacy-safe HIPPO diagnostic
    And terminal restoration is attempted before the diagnostic is written
    And its observation work and timers do not survive the runner

    Examples:
      | failure         |
      | startup         |
      | input read      |
      | output write    |
      | restoration     |

  @e2e-exempt
  @tui-terminal
  Scenario: A classified terminal failure takes precedence over an interruption
    Given an interrupted TUI whose terminal restoration fails
    When the terminal session finishes
    Then it exits 125 for the restoration failure instead of the signal status
    And restoration is attempted before the diagnostic is written
