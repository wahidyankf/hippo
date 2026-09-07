# How to watch host pressure

Use `hippo monitor` when you want to know _why_ work is being deferred or shed, rather than what a
single sample said.

## Watch transitions in a terminal

```sh
hippo monitor --interval 1s --disk-path .
```

```console
2026-09-07T04:55:25.508138Z state=normal reason=normal profile=balanced swap=active
```

`monitor` prints the initial state and then **only transitions** — a quiet pane means the host is
stable, not that monitoring stopped. It runs until you cancel it.

A busy host produces a trail:

```console
2026-01-02T03:04:05Z state=normal reason=normal profile=balanced swap=idle
2026-01-02T03:05:10Z state=warning reason=memory-psi profile=constrained swap=idle
```

The `reason` field names the specific evidence that moved the state — memory, PSI, disk, swap, CPU,
or compressor — which is usually the fastest way to identify what is actually saturated.

## Feed a machine consumer

```sh
hippo monitor --interval 1s --json --disk-path .
```

```console
{"schemaVersion":1,"measuredAt":"2026-09-07T04:55:28.569948Z","state":"normal","reason":"normal","profile":"balanced","swapState":"active"}
```

One schema-1 object per transition, one object per line. Pipe it anywhere that reads JSONL:

```sh
hippo monitor --json --disk-path . | jq -r 'select(.state != "normal") | "\(.measuredAt) \(.reason)"'
```

## Take a single reading instead

For a script that just needs the current answer, `status` is cheaper — one sample, then exit:

```sh
hippo status --disk-path .
```

```console
state=normal reason=normal profile=balanced concurrency=11 swap=active availableGiB=13.76 diskFreeGiB=78.67 cpu=15.5%
```

```sh
hippo status --json --disk-path . | jq -r '.resource.state'
```

## Keep a long-running pane

`monitor` writes plain text, so any terminal multiplexer can capture it without special tooling:

```sh
tmux capture-pane -p -t hippo:0.0 -S -200
```

## Point it at the right volume

`--disk-path` decides which filesystem's free space is measured. Point it at the directory the
guarded work will actually write to — a build output tree, a container store, a deployment root:

```sh
hippo monitor --interval 5s --disk-path ./target
```

Measuring a roomy volume while the work fills a small one is the most common way to be surprised by
exit `73`.

## Related

- [Resource policy](../reference/resource-policy.md) — the thresholds behind each `reason`
- [Command-line interface](../reference/cli.md#hippo-monitor)
- [How to respond to a HIPPO exit code](./respond-to-exit-codes.md)
