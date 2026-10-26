Feature: Interactive terminal ownership
  A guarded process group can read inherited terminal input without losing group-isolated cleanup.

  Scenario: Interactive guarded child owns the terminal while it runs
    Given a guarded process with inherited controlling-terminal input
    When the child reads from the terminal in its own process group
    Then it completes without SIGTTIN and original foreground ownership is restored
