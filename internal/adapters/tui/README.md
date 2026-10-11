# Terminal UI Adapter

This package owns the Bubble Tea boundary for interactive resource observation. Application snapshots and complete run
IDs cross inward-facing ports; framework messages, display state and terminal effects remain in this adapter.

The model processes completion and clock messages without collecting host data. It coalesces busy refresh requests,
schedules from completion time, preserves stale snapshots, tracks age independently, and selects rows by complete ID.
Pure behavior contracts record its request, schedule and cancellation effects.

The dashboard keeps CPU, memory, disk and reservation summaries visible. At 80x24 and larger it shows Running and Queue
together; at 40x16 and larger it shows the selected panel. Smaller windows show fitted size and quit/help text. Job
detail, capacity and help use the Bubbles viewport with only vertical single-row keys enabled. Back restores the
previous help destination and scroll position. Resize keeps complete run-ID selection and the selected row visible.

Tab/n and Shift-Tab/p change panels; Down/j and Up/k select or scroll. Enter opens the selected job, c opens capacity, b
goes back, r refreshes, ? opens help and q quits. Raw Ctrl-C records an interruption and cancels owned work. Host
prefixes and unrelated keys are ignored. These navigation actions work even while observation is outstanding.

Untrusted text passes through pinned x/ansi sequence stripping before UTF-8/control normalization. Lipgloss measures and
wraps terminal cells; x/ansi truncates sanitized rows with an ASCII marker. Plain-text states retain meaning when
styling is disabled. Allocations are reservations and do not imply process CPU or RSS measurements.

The runner and its worker, scheduler, event pump and terminal-lifecycle tests are a separate implementation boundary.
Model/view tests use immutable observations and effect traces; they do not count as real terminal restoration proof.

For the owner-approved terminal-fault acceptance scope and its public evidence gaps, see
[compiled E2E tests](../../../tests/e2e/README.md) and
[BDD boundaries](../../../repo-governance/development/behaviour-driven-development.md). Model or Integration receipts
are not public-binary observations.
