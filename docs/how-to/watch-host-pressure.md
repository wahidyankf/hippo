# How to watch host pressure

Use `hippo watch` for the complete operator view of host pressure, current owners, FIFO waiters, deadlines, and burst
promotion. Use `hippo monitor` when a resource-only transition stream is enough.

## Watch admission and queue transitions

```sh
hippo watch --interval 5s --disk-path .
```

`watch` prints the initial schema-5 status and then only changed snapshots. Each owner or waiter row shows its opaque
run ID, position, source, task class, resource tier, vector, and deadline. Filter a busy machine without hiding global
totals:

```sh
hippo watch --source ose-public --tag checkout=worktree --disk-path .
```

For a machine consumer, `--json` emits one complete schema-5 object per changed snapshot.

## Watch transitions in a terminal

```sh
hippo monitor --interval 1s --disk-path .
```

```console
2026-09-07T04:55:25.508138Z state=normal reason=normal profile=balanced swap=active
```

`monitor` prints the initial state and then **only transitions** — a quiet pane means the host is stable, not that
monitoring stopped. It runs until you stop it; `Ctrl-C` ends it with `130`, the status a shell reports for an interrupt,
and so does `watch`.

A busy host produces a trail:

```console
2026-01-02T03:04:05Z state=normal reason=normal profile=balanced swap=idle
2026-01-02T03:05:10Z state=warning reason=memory-psi profile=constrained swap=idle
```

The `reason` field names the specific evidence that moved the state — memory, PSI, disk, swap, CPU, or compressor —
which is usually the fastest way to identify what is actually saturated.

## Feed a machine consumer

```sh
hippo monitor --interval 1s --json --disk-path .
```

Long output lines below are wrapped to fit, with continuation lines indented; each is one line in real output.

```console
{"schemaVersion":1,"measuredAt":"2026-09-07T04:55:28.569948Z","state":"normal","reason":"normal","profile":"balanced",
  "swapState":"active"}
```

One schema-1 object per transition, one object per line. Pipe it anywhere that reads JSONL:

```sh
hippo monitor --json --disk-path . | jq -r 'select(.state != "normal") | "\(.measuredAt) \(.reason)"'
```

## Take a single reading instead

For a script that just needs the current answer, `status` is cheaper — one reading (two samples a second apart), then
exit:

```sh
hippo status --disk-path .
```

Long output lines below are wrapped to fit, with continuation lines indented; each is one line in real output.

```console
state=normal reason=normal profile=balanced concurrency=12 swap=active availableGiB=11.20 diskFreeGiB=96.33 cpu=23.5%
  owners=0 waiters=0 ownerLimit=0 promotion=not-configured
```

```sh
hippo status --json --disk-path . | jq -r '.resource.state'
```

## Keep a long-running pane

Both commands write plain text, so any terminal multiplexer can capture it without special tooling:

```sh
tmux capture-pane -p -t hippo:0.0 -S -200
```

## Point it at the right volume

`--disk-path` decides which filesystem's free space is measured. Point it at the directory the guarded work will
actually write to — a build output tree, a container store, a deployment root:

```sh
hippo monitor --interval 5s --disk-path ./target
```

Measuring a roomy volume while the work fills a small one is the most common way to be surprised by exit `124` naming
`hippo.limit.storage-blocked`.

## Read swap as memory on a Linux devbox

With `coordination.countSwap` on, `watch`, `status`, and `monitor` count unused swap as memory up to the swap allowance.
`status` and `watch` name the allowance in their text line, and their JSON carries the headroom and the counted memory.
`monitor` counts the swap in its transitions but prints no allowance. A `swap-over-allowance` advisory appears in the
JSON when swap use passes the allowance, and it never changes the state. Set the key only after every binary that reads
the file is `v0.11.0` or later, as the [configuration reference](../reference/configuration.md#swap-counting) explains.

## Account for a tuned swap gate

On a machine with `coordination.swapUsedMaxMultiple` set, `watch` and `monitor` skip the `swap-warning` and
`swap-critical` reasons while swap in use fits within the multiple of physical memory. A missing swap reason on a tuned
machine means the gate waived it, not that no swap-outs happened; PSI, pressure levels, and the compressor still report
as usual. See [swap gate](../reference/resource-policy.md#swap-gate).

## Related

- [Resource policy](../reference/resource-policy.md) — the thresholds behind each `reason`
- [Command-line interface](../reference/cli.md#hippo-monitor)
- [How to respond to a HIPPO exit code](./respond-to-exit-codes.md)
