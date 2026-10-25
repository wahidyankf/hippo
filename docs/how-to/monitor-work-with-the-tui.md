# How to monitor work with the TUI

Use `hippo tui` to inspect HIPPO's running jobs, registered queue, and machine capacity from a terminal. The TUI
observes work; quitting it does not stop guarded jobs. It is not a list of every process on the machine.

## Open the dashboard

Run from the directory whose filesystem you want to monitor:

```sh
hippo tui
```

Both input and output must be terminals, and `TERM` must be something other than `dumb`. Use
[status or watch](./watch-host-pressure.md) when you need redirected output or a scriptable view. Help works without
opening the dashboard:

```sh
hippo tui --help
```

**Verification note:** as of v0.9.0, the interactive launch examples on this page had not been exercised against the
final release build. `hippo tui --help` exits `0` on a binary built from the landed source, with the flags shown below,
and piped streams, `TERM=dumb`, and a sub-second `--interval` are each refused with exit `2` naming
`hippo.args.invalid`. As of v0.9.0, final interactive verification was pending.

The default refresh delay is **5 seconds after each collection finishes**, not every 5 seconds on a fixed clock.
Collection takes two host readings one second apart, so a complete refresh cycle takes longer than the delay. To wait 10
seconds between completed collections and measure the current directory's volume:

```sh
hippo tui --interval 10s --disk-path .
```

The minimum interval is `1s`. Point `--disk-path` at the filesystem where your work writes. Use `--config` to select a
local configuration or `--profile` to request a resource profile; see [configuration](../reference/configuration.md).
The active shared coordination state determines which running and waiting work is visible.

## Find a job and inspect its resources

Start in **Running**. Press `n` or `Tab` to switch to **Queue**, then use `j`/`k` or Down/Up to select a row. Press
Enter to open its detail and `b` to return.

Running rows show allocated reservations. Queue rows show requested resources and `WAIT #` with their original FIFO
position. Detail includes the full run ID, label, source, class, profile, tier, resource bounds, registration time,
deadline, elapsed time, and tags when available. Missing evidence is shown as `unavailable`.

Selection follows the full run ID through refreshes and resizes. If a selected row disappears, the nearest surviving row
is selected. An already open detail instead says `job no longer active`; press `b` to continue browsing.

Press `c` on the dashboard for **Capacity detail**. Scroll with `j`/`k` to compare host CPU, RAM, swap, disk space,
pressure and its reason with reservation capacity, allocated resources, waiting demand, and owner limits. An allocation
is a reservation, not measured process CPU usage or RSS. A missing reading is not a zero reading.

Reservation mode exposes the registered FIFO queue. Exclusive compatibility mode reports the queue as `unregistered`; it
cannot tell you how many unregistered callers are waiting. Legacy jobs may have unavailable labels or metadata while
their known accounting remains visible.

## Reduce a busy list

Filter rows by a source and one or more tags:

```sh
hippo tui --source ose-public --tag checkout=worktree
```

Replace the source and tag with values your jobs actually use. Repeat `--tag` for additional requirements. A row must
match the source and every supplied tag. Filters change the visible Running and Queue lists, while global capacity,
allocated resources, waiting demand, and owner totals remain unfiltered. Queue positions retain their original numbers;
a filtered list can start at `WAIT #3`.

Labels help distinguish jobs without exposing command arguments or working directories. When launching your own guarded
work, use `run`'s `--label` for a short, public description such as `build-web`. Otherwise HIPPO derives a label from
the executable basename. Labels allow 1–64 safe ASCII characters, must start with a letter or digit, and cannot contain
path separators or `..`. Keep private data out of labels, sources, and tags. A missing or unusable label does not remove
the live job or its accounting.

## Refresh and recognize old data

Press `r` from any view to request a refresh. Navigation and resize remain usable during collection. Repeated refresh
requests while collection is busy become one immediate follow-up, rather than overlapping collections.

The state line shows `refreshing` while collecting and the age of the last host sample. If refresh fails, the last
successful data remains visible with `STALE` and `ERROR Observation unavailable`. Treat it as old evidence and press `r`
to retry. Before any successful observation, values remain unavailable instead of inventing an empty machine. A
successful refresh replaces the data and clears those markers. `age unavailable` means the sample time is unknown.

## Use a narrow or mobile terminal

The dashboard needs at least **40 columns by 16 rows**. At 40×16 or 60×20, the compact layout shows the active list; use
`n`/`p` to reach the other list. At 80×24 or larger, both lists are visible. Long rows fit the available width; open
detail for wrapped fields and scroll to read more.

At 39×15, or whenever either minimum dimension is missing, a fitted notice asks for 40×16. Enlarge or rotate the
terminal, reduce the font size, or expand the pane. Returning to a supported size preserves a still-live selection. `q`,
`r`, and `?` remain available while the terminal is too small; enlarge it to read the full help.

## Keep keyboard control

All TUI actions have keys without modifiers, so mobile keyboards can reach them. Press `?` for scrollable help.

| Where                  | Key                    | Action                                          |
| ---------------------- | ---------------------- | ----------------------------------------------- |
| Dashboard              | `n` or Tab             | Next section                                    |
| Dashboard              | `p` or Shift-Tab       | Previous section                                |
| Dashboard              | `j` or Down; `k` or Up | Select next or previous job                     |
| Dashboard              | Enter                  | Open selected job detail                        |
| Dashboard              | `c`                    | Open capacity detail                            |
| Detail, capacity, help | `j` or Down; `k` or Up | Scroll down or up one row                       |
| Detail or capacity     | `b`                    | Return to the dashboard                         |
| Help                   | `b`                    | Return to the previous view and scroll position |
| Every view             | `r`                    | Refresh                                         |
| Every view             | `?`                    | Open help                                       |
| Every view             | `q`                    | Quit normally, exit `0`                         |
| Every view             | Ctrl-C                 | Interrupt, exit `130`                           |

SIGINT also exits `130`; SIGTERM exits `143`. Exiting cancels observation and restores the terminal without cancelling
guarded jobs. A terminal failure is reported as exit `125`; see [exit codes](../reference/exit-codes.md).

HIPPO does not bind Ctrl-Space, Ctrl-h/j/k/l, Ctrl-backslash, or Escape to job actions. Keep using host shortcuts for
pane navigation. For setups using Ctrl-Space as the tmux prefix or Cmd-B as the WezTerm leader, those prefixes belong to
the host. In a Neovim terminal configured with double Escape to leave terminal mode, use that sequence to reach normal
mode and `i` to send keys to HIPPO again. Use plain Enter for job detail; a host's Shift-Enter mapping is not a HIPPO
shortcut. Host mappings must be checked in your own configuration.

To disable color styling, launch with `--color never`. Text markers such as `>`, `WAIT`, `STALE`, and `ERROR` still
distinguish selection and state.

## Related

- [Command-line reference](../reference/cli.md)
- [Resource policy](../reference/resource-policy.md)
- [Watch host pressure](./watch-host-pressure.md)
