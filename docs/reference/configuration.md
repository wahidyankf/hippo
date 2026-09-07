# Configuration

HIPPO's configuration file is optional, machine-local, and never committed. Without one, HIPPO runs
in schema-1 exclusive coordination.

## Precedence

Strongest first:

1. `--config <path>`
2. `HIPPO_CONFIG`
3. `HIPPO_DEFAULT_CONFIG` — set by the `./hippo` bootstrap to the repository-local
   `hippo.local.json`, if that file exists

Start from [`hippo.local.json.example`](../../hippo.local.json.example) and copy it to the ignored
path `hippo.local.json`.

## Schema versions

| `schemaVersion` | Coordination mode | Notes                                                         |
| --------------- | ----------------- | ------------------------------------------------------------- |
| `1`             | `exclusive`       | Retains v0.3.1 semantics. This is also the no-config default. |
| `2`             | `reservation`     | Shared CPU-and-memory reservation ledger                      |

Retaining a schema-1 file is a deliberate choice of the older mode, not an oversight. Any other
`schemaVersion` is rejected.

## Minimal reservation configuration

```json
{
  "schemaVersion": 2,
  "coordination": {
    "mode": "reservation"
  }
}
```

## `coordination`

| Field                  | Type    | Constraint                                             |
| ---------------------- | ------- | ------------------------------------------------------ |
| `mode`                 | string  | Must be `"reservation"` when set under schema 2        |
| `maxCpu`               | integer | Nonnegative. `0` means no extra cap                    |
| `maxMemoryMiB`         | integer | Nonnegative. If set, must be at least `256`            |
| `maxActiveOwners`      | integer | Nonnegative, at most `20`. `0` means the default of 20 |
| `automaticOwnerShares` | object  | Profile name → share count, each between `1` and `20`  |

These fields may only _tighten_ safety. `maxMemoryMiB` below 256 and `maxActiveOwners` above 20 are
rejected at load time with exit `78`, so a local file cannot weaken the compiled floors.

```console
$ hippo status --config weakened.json --disk-path .
Error: resource configuration: maximum memory weakens the immutable 256 MiB floor
$ echo $?
78

$ hippo status --config too-many-owners.json --disk-path .
Error: resource configuration: maxActiveOwners cannot exceed 20
$ echo $?
78
```

Default automatic owner shares divide capacity between four, two, and one owner:

| Profile       | Automatic owners |
| ------------- | ---------------- |
| `balanced`    | 4                |
| `constrained` | 2                |
| `minimal`     | 1                |

### The shared-root owner limit

Within one shared state root, every live owner and every queued waiter contributes its own configured
`maxActiveOwners`. HIPPO uses the **minimum** of those contributions until the contributing
participant exits or times out, and resets the effective limit when the ledger becomes idle. A single
strict repository therefore tightens the whole host, and cannot be loosened by a permissive peer.

## `profiles`

Profile overrides tune the resolved envelope. A profile `extends` one of the built-in profiles and
may set a `fallback`.

```json
{
  "schemaVersion": 2,
  "coordination": { "mode": "reservation" },
  "defaultProfile": "local-constrained",
  "profiles": {
    "local-constrained": {
      "extends": "constrained",
      "fallback": "minimal",
      "strict": false,
      "maxCpuUtilizationPercent": 90
    }
  }
}
```

| Field                        | Type    | Meaning                                                         |
| ---------------------------- | ------- | --------------------------------------------------------------- |
| `extends`                    | string  | Built-in profile this one derives from                          |
| `fallback`                   | string  | Profile to resolve to when this one does not fit                |
| `strict`                     | boolean | When true, no fallback is attempted; a misfit replans with `78` |
| `memoryReservePercent`       | number  | Share of effective memory held back                             |
| `memoryReserveMinMiB`        | integer | Lower clamp on the memory reserve                               |
| `memoryReserveMaxMiB`        | integer | Upper clamp on the memory reserve                               |
| `noSwapMemoryReservePercent` | number  | Memory reserve when the host has no usable swap                 |
| `noSwapMemoryReserveMinMiB`  | integer | Lower clamp, no-swap case. Never below 256 MiB                  |
| `noSwapMemoryReserveMaxMiB`  | integer | Upper clamp, no-swap case                                       |
| `diskReservePercent`         | number  | Share of disk held back                                         |
| `diskReserveMinMiB`          | integer | Lower clamp on the disk reserve                                 |
| `diskReserveMaxMiB`          | integer | Upper clamp on the disk reserve                                 |
| `maxConcurrency`             | integer | Ceiling on canonical concurrency                                |
| `maxCpuUtilizationPercent`   | number  | CPU utilization above which the profile does not fit            |

A profile override that weakens an immutable safety floor is rejected with
`profile weakens immutable safety floors`.

## Configuration identity

Both `status --json` and lifetime summaries carry a `configHash` — the SHA-256 of the loaded
configuration document. Two owners reporting the same `configHash` loaded byte-identical
configuration.

## Related

- [How to enable reservation coordination](../how-to/enable-reservation-coordination.md)
- [Resource policy](./resource-policy.md)
- [State root](./state-root.md)
