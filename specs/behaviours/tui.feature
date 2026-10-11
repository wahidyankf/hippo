Feature: Responsive read-only HIPPO monitoring
  Operators inspect shared work and machine pressure without changing admission or controlling jobs.

  @tui-snapshot-01
  Scenario: TUI observation preserves running identities and FIFO demand
    Given two running jobs and three registered FIFO waiters with known reservation vectors
    When one TUI observation completes
    Then both running identities and all three original FIFO positions retain their resource values

  @tui-snapshot-02
  Scenario: TUI filtering retains global accounting and original queue positions
    Given two running jobs and three FIFO waiters with only the third waiter matching source and tag filters
    When the filtered TUI observation completes
    Then only the matching row is visible at position 3 and global capacity and usage remain unfiltered

  @tui-snapshot-03
  Scenario: TUI observation uses the current shared status evidence
    Given a host collector with two readings and a shared root whose active mode differs from configuration
    When one TUI observation completes
    Then two host readings one second apart and the active coordination mode determine the displayed snapshot

  @tui-snapshot-04
  Scenario: An idle TUI does not create coordination ownership
    Given an empty reservation root with no active epoch
    When one TUI observation completes
    Then Running and Queue are empty with zero known demand and unavailable epoch capacity
    And the observation creates no owner waiter annotation or coordination epoch

  @tui-snapshot-05
  Scenario: Exclusive monitoring names its unregistered queue limitation
    Given an exclusive compatibility root with a live owner and no registered FIFO queue
    When one TUI observation completes
    Then the owner is visible and reservation vectors and unregistered waiting demand are unavailable

  @tui-snapshot-06
  Scenario: Legacy reservation rows retain known allocation vectors
    Given a reservation owner whose optional metadata and display annotation are missing
    When one TUI observation completes
    Then its live run ID and ledger allocation remain known while its label is unavailable

  @e2e-exempt
  @tui-snapshot-07
  Scenario: Completed observations remain unchanged when source values are reused
    Given a completed TUI observation with host metrics row tags and ordered jobs
    When the collector and coordination provider reuse and change their previous values
    Then the completed observation retains its original values and independently owned row tags

  @tui-label-01
  Scenario: Annotations join only the complete live job identity
    Given live job IDs sharing a short prefix and annotations for one live ID and one absent ID
    When one TUI observation completes
    Then only the exact matching live ID receives its label without adding or reordering jobs

  @tui-label-02
  Scenario Outline: Unusable annotations leave live accounting intact
    Given a live job whose display annotation is <state>
    When one TUI observation completes
    Then its label is unavailable and the live identity vectors and global accounting remain unchanged
    And the annotation advisory exposes no unchecked payload or private path

    Examples:
      | state                  |
      | missing                |
      | malformed              |
      | mismatched run ID      |
      | unsupported schema     |
      | larger than 512 bytes  |
      | unreadable             |
      | a symlink              |

  @tui-refresh-01
  Scenario: Default refresh starts five seconds after completion
    Given a TUI whose previous observation has just completed
    When five seconds elapse after that completion
    Then exactly one new observation starts and no observation starts before that deadline

  @tui-refresh-02
  Scenario: Repeated refresh requests coalesce while observation is busy
    Given one blocked TUI observation
    When refresh is requested three times before that observation completes
    Then no second observation overlaps it and exactly one immediate follow-up starts after completion

  @tui-refresh-03
  Scenario: Navigation and resize remain usable during collection
    Given one blocked TUI observation and a selected running job
    When the operator switches section and resizes to 40 by 16 cells
    Then the section and compact layout change before the observation is released

  @tui-refresh-04
  Scenario: Initial observation failure does not invent empty accounting
    Given a TUI without a successful observation
    When its first observation fails
    Then the display shows unavailable data and a safe error summary with refresh still accessible

  @tui-refresh-05
  Scenario: Failed refresh preserves the last good snapshot as stale
    Given a successful TUI snapshot with one owner and known allocated resources
    When the next observation fails
    Then the same owner and allocation remain visible with STALE sample age and a safe error summary

  @tui-refresh-06
  Scenario: Successful refresh clears stale and error markers
    Given a stale TUI snapshot after an observation failure
    When a later observation succeeds
    Then the new snapshot replaces the old data and STALE and ERROR markers disappear

  @tui-refresh-07
  Scenario: Sample age advances without collecting again
    Given a successful observation whose second host reading was measured ten seconds ago
    When one lightweight age tick elapses before the next refresh deadline
    Then the displayed sample age advances and the host collector is not called again

  @e2e-exempt
  @tui-refresh-08
  Scenario Outline: Unknown sample time remains unavailable
    Given a successful observation whose second host measurement time is <time>
    When the TUI displays its sample age
    Then sample age is unavailable instead of using refresh completion time

    Examples:
      | time      |
      | absent    |
      | malformed |

  @tui-refresh-09
  Scenario: Exiting a busy TUI ignores late observations
    Given one blocked TUI observation
    When the operator quits and the collector receives cancellation
    Then the TUI exits after its observation and timing work finish and no late result updates its display

  @e2e-exempt
  @tui-resources-01
  Scenario: Missing host readings are distinct from accounting zero
    Given known zero reservation demand and unavailable optional CPU memory swap and disk readings
    When capacity detail is displayed
    Then reservation demand says zero and the missing host readings say unavailable

  @tui-resources-02
  Scenario: Reservation resources are named separately from host utilization
    Given a job allocated two CPUs and one GiB with host CPU utilization at twenty percent
    When job and capacity details are inspected
    Then reservation allocation and machine utilization have distinct labels without claiming process CPU or RSS

  @tui-views-01
  Scenario Outline: Each viewport uses a bounded appropriate layout
    Given a populated TUI at <width> columns and <height> rows
    When its current view is rendered
    Then every line fits <width> terminal cells and the complete view fits <height> rows
    And the display uses the <layout> layout

    Examples:
      | width | height | layout       |
      | 39    | 15     | minimum-size |
      | 39    | 16     | minimum-size |
      | 40    | 15     | minimum-size |
      | 40    | 16     | compact      |
      | 60    | 20     | compact      |
      | 79    | 24     | compact      |
      | 80    | 23     | compact      |
      | 80    | 24     | wide         |
      | 100   | 30     | wide         |
      | 120   | 40     | wide         |

  @tui-views-02
  Scenario: Too-small view recovers without losing live selection
    Given a selected live job at 40 columns and 16 rows
    When the terminal shrinks to 39 by 15 cells and returns to 40 by 16 cells
    Then the fitted minimum-size message gives way to the compact view with the same selected run ID

  @tui-views-03
  Scenario Outline: Untrusted display text cannot escape its cell budget
    Given a TUI row and detail containing <text>
    When they are rendered at 40 columns and 16 rows
    Then controls are not displayed and each row and detail fits its terminal-cell and row budget

    Examples:
      | text                       |
      | an overlong unbroken value |

  @e2e-exempt @tui-views-03 @tui-synthetic-text
  Scenario Outline: Synthetic unsafe display text cannot escape its cell budget
    Given a TUI row and detail containing <text>
    When they are rendered at 40 columns and 16 rows
    Then controls are not displayed and each row and detail fits its terminal-cell and row budget

    Examples:
      | text                          |
      | wide Unicode characters       |
      | combining Unicode characters  |
      | ANSI escapes and control bytes |

  @tui-views-04
  Scenario: Large lists scroll while retaining the selected identity
    Given one thousand running and queued jobs with a selected live run ID
    When the operator navigates and repeatedly resizes between compact and wide views
    Then the selected run ID stays visible and each list shows its bounded position and count

  @tui-views-05
  Scenario: A disappearing selection moves to the nearest surviving row
    Given three visible jobs with the middle run ID selected
    When a refresh removes that selected job
    Then the nearest surviving row is selected by its prior position and remains visible

  @tui-views-06
  Scenario: A disappeared detail remains navigable
    Given job detail is open for a live run ID
    When a refresh removes that run ID
    Then detail reports job no longer active and back returns to a usable section

  @tui-views-07
  Scenario: Empty panels have no false selection
    Given a TUI whose active job panel is empty
    When the operator requests job detail
    Then the explicit empty panel remains usable with no selected job

  @tui-keys-01
  Scenario Outline: Dashboard keyboard actions reach their declared destination
    Given a populated TUI with the middle running job selected
    When the operator presses <key> on the dashboard
    Then the TUI performs <action>

    Examples:
      | key       | action                      |
      | Tab       | select the next section     |
      | n         | select the next section     |
      | Shift-Tab | select the previous section |
      | p         | select the previous section |
      | Down      | select the next job         |
      | j         | select the next job         |
      | Up        | select the previous job     |
      | k         | select the previous job     |
      | Enter     | open the selected job detail |
      | c         | open capacity detail        |
      | ?         | open keyboard help          |

  @tui-keys-02
  Scenario Outline: Detail and help scroll and return without a focus trap
    Given the TUI <view> is open with scrollable content
    When the operator presses <key>
    Then the TUI performs <action>

    Examples:
      | view            | key  | action                              |
      | job detail      | Down | scroll one row down                 |
      | job detail      | j    | scroll one row down                 |
      | job detail      | Up   | scroll one row up                   |
      | job detail      | k    | scroll one row up                   |
      | capacity detail | Down | scroll one row down                 |
      | capacity detail | j    | scroll one row down                 |
      | capacity detail | Up   | scroll one row up                   |
      | capacity detail | k    | scroll one row up                   |
      | help            | Down | scroll one row down                 |
      | help            | j    | scroll one row down                 |
      | help            | Up   | scroll one row up                   |
      | help            | k    | scroll one row up                   |
      | job detail      | b    | restore the previous section and selection |
      | capacity detail | b    | restore the previous section and selection |
      | help            | b    | restore the previous view and selection    |

  @tui-keys-03
  Scenario Outline: Manual refresh is reachable from every view
    Given the TUI <view> is open
    When the operator presses r
    Then one refresh is requested without leaving that view

    Examples:
      | view            |
      | dashboard       |
      | job detail      |
      | capacity detail |
      | help            |
      | minimum-size    |

  @tui-keys-04
  Scenario: Help explains every product function using modifier-free keys
    Given a populated TUI
    When keyboard help is opened
    Then every section selection detail capacity back refresh help and quit action has a documented usable key

  @tui-keys-05
  Scenario Outline: Unrelated keys do not capture host navigation
    Given a populated TUI with a selected run ID
    When the operator presses <key>
    Then no job action or TUI quit occurs and selection is unchanged

    Examples:
      | key            |
      | x              |
      | Ctrl-Space     |
      | Ctrl-h         |
      | Ctrl-j         |
      | Ctrl-k         |
      | Ctrl-l         |
      | Ctrl-backslash |
      | Escape         |

  @tui-keys-06
  Scenario: No-color output retains every state distinction
    Given color is disabled and the TUI has selected waiting stale and error states
    When the dashboard and details are rendered
    Then text markers distinguish those states without ANSI styling
