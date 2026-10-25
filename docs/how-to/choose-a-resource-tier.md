# How to choose a resource tier

Under adaptive schema 3, every guarded `hippo run` names one tier with `--resource-tier`. The tier sets the CPU and
memory the run asks the shared ledger for, and how long the run may wait in the queue. This guide covers how to choose a
tier, how to add the optional `very-heavy` tier, and how to roll either change out to a machine without stopping any
guarded command.

The bounds in this guide are example values. Your configuration file sets them, and each tier must fit inside the pool
that file declares. The [configuration reference](../reference/configuration.md#adaptive-schema-3) shows a wider
recommended set, and a file may use either.

## The example tiers

| Tier         | CPU (min–max) | Memory in MiB (min–max) | Use it for                                          |
| ------------ | ------------- | ----------------------- | --------------------------------------------------- |
| `light`      | 1–1           | 512–1024                | Formatting, linting, and small checks               |
| `standard`   | 2–2           | 1024–2048               | Ordinary build and test plans                       |
| `heavy`      | 2–4           | 2048–6144               | Work whose measured use is above `standard`         |
| `very-heavy` | 4–6           | 6144–10240              | A last resort, declared only on a machine that fits |

The minimum is the run's admission floor. When the queue head fits, HIPPO grants the largest vector up to the tier's
maximum that is safe at that moment. The maximum is a ceiling, not a promise, and the allocation then stays fixed for
the life of the run.

## Pick the smallest tier that fits

Pick the smallest tier that fits the work, and move up only on measured need. A larger tier waits longer for room in the
pool and holds more of the shared budget while it runs, so a guess that a bigger tier is safer costs every other
repository on the machine.

Measure CPU from history. HIPPO records the child's CPU and wall time for each run it measured:

```sh
hippo history --since 30d --source my-repository --json
```

Each row carries `resourceTier`, `outcome`, `childCpuSeconds`, and `childWallSeconds`. The abridged row below is from a
run that kept one CPU busy for about nine seconds:

```console
{"schemaVersion":5,"source":"my-repository","resourceTier":"light","taskClass":"ephemeral","outcome":"passed", ...,
  "childCpuSeconds":9.608,"childWallSeconds":9.598}
```

Divide `childCpuSeconds` by `childWallSeconds` to get the average number of CPUs the run kept busy. That ratio is about
1.0 here, so `light` is enough. Move up a tier when the average is above the CPU maximum of the tier you use now. A run
that is shed or cancelled reports a lower bound, and a guard nested inside another guard counts its work in the outer
run.

History does not record what a run needed in memory. Measure a run's peak memory with your build tool or the operating
system, and move up only when that peak is above the memory maximum of the current tier. An outcome of `pressure-shed`
shows that the host was short of memory, not that the run needed more, so look at the host before you escalate.

## Add the very-heavy tier

Declare `very-heavy` only in a schema-3 file, and only on a machine with enough CPU and memory for its maximum. In
practice that is the desktop. Add the block beside the three required tiers under `coordination.tiers`:

```json
"very-heavy": {
  "minimumCpu": 4,
  "maximumCpu": 6,
  "minimumMemoryMiB": 6144,
  "maximumMemoryMiB": 10240,
  "queueDeadline": "4h"
}
```

HIPPO checks the block when it loads the file:

- Its bounds must fit inside the pool that `maxCpu` and `maxMemoryMiB` declare.
- A block that is `null` or `{}`, or whose `queueDeadline` is not a positive duration, refuses the whole file.
- A key spelled `very_heavy` is refused as an unknown tier.

Each refusal exits `125` naming `hippo.config.unreadable`. A run names the tier only where the file declares it:

```sh
hippo run --resource-tier very-heavy --disk-path . -- make test
```

On a file that does not declare the tier, the run exits `2` before any work starts. Long output lines below are wrapped
to fit, with continuation lines indented; each is one line in real output.

```console
$ hippo run --resource-tier very-heavy --disk-path . -- make test
hippo: [hippo.args.invalid] --resource-tier very-heavy: this configuration does not declare the very-heavy tier; choose
  light, standard, or heavy
$ echo $?
2
```

## Turn on swap counting on Linux

Swap counting is for a Linux host with schema 3 and swap that is not backed by zram. zram is carved from RAM, so
counting it counts the same memory twice. Add the key inside `coordination`:

```json
"coordination": {
  "countSwap": true
}
```

With the key on, unused swap counts as memory up to an allowance. The allowance is derived from swap total and the
memory limit, and [swap allowance](../reference/resource-policy.md#swap-allowance) gives the exact rule. Swap in use
above the allowance raises the `swap-over-allowance` advisory, and that advisory never sheds work on its own. On macOS
the allowance is zero, so admission and shedding decisions do not change. The key still adds output: the text status
line prints `swapAllowanceGiB=0.00`, and the [JSON fields](../reference/json-schemas.md) are present. Their
`swapAllowanceBytes` and `resource.swapHeadroomBytes` are zero, and `resource.countedAvailableMemoryBytes` equals the
raw available memory.

## Roll a change out to a machine

Any binary older than v0.11.0 refuses both settings. A v0.10.0 binary that reads a file declaring `very-heavy` exits
`125` naming `hippo.config.unreadable` on every guarded command that loads that file:

```console
$ hippo status --config hippo.local.json --disk-path .
hippo: [hippo.config.unreadable] resource configuration: schema 3 contains an unknown resource tier
$ echo $?
125
```

A file with `countSwap` gets the same exit, with `json: unknown field "countSwap"` in place of the tier message. A
machine's file that declares neither setting is unaffected by the upgrade. Roll out in this order:

1. Repin every consumer on the machine to v0.11.0, following [install a pinned release](./install-a-pinned-release.md).
2. Check each binary. `hippo version --json` must report `"version":"v0.11.0"`, with the commit that the release names.
   The line below shows the shape only; it is not a captured output:

   ```console
   $ hippo version --json
   {"schemaVersion":1,"version":"v0.11.0","commit":"<commit>"}
   ```

3. Only when every binary on the machine reports v0.11.0, add the `very-heavy` block or the `countSwap` key to that
   machine's file.

To roll back, delete the block or the key. The file then loads on every binary again.

## Related

- [Configuration: optional very-heavy tier](../reference/configuration.md#optional-very-heavy-tier)
- [Configuration: swap counting](../reference/configuration.md#swap-counting)
- [Resource policy: schema-3 resource tiers](../reference/resource-policy.md#schema-3-resource-tiers)
- [Command-line interface: hippo run](../reference/cli.md#hippo-run)
- [How to enable reservation coordination](./enable-reservation-coordination.md)
- [How to respond to a HIPPO exit code](./respond-to-exit-codes.md)
