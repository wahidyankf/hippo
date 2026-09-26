# Configuration

HIPPO's configuration file is optional, machine-local, and never committed. Without one, HIPPO runs
in schema-1 exclusive coordination.

## Precedence

Strongest first:

1. `--config <path>`
2. `HIPPO_CONFIG`
3. `HIPPO_DEFAULT_CONFIG` — set by the `./hippo` bootstrap to the repository-local
   `hippo.local.json`
4. `hippo.local.json` in the process's current directory — not `--cwd` — when none of the above
   is set

The first source that is set wins, and no later one is consulted. A missing file named by
`--config` or `HIPPO_CONFIG` fails with `hippo.config.unreadable` (exit `125`); a missing file at 3
or 4 falls back to the no-config default. The bootstrap always sets `HIPPO_DEFAULT_CONFIG`, so 4
applies only to a binary started directly, such as a release binary: started in a directory that
holds `hippo.local.json`, it loads that file without being asked.

Start from [`hippo.local.json.example`](../../hippo.local.json.example) and copy it to the ignored
path `hippo.local.json`.

## Schema versions

| `schemaVersion` | Coordination mode | Notes                                                                  |
| --------------- | ----------------- | ---------------------------------------------------------------------- |
| `1`             | `exclusive`       | Retains v0.3.1 semantics. This is also the no-config default.          |
| `2`             | `reservation`     | Shared CPU-and-memory reservation ledger                               |
| `3`             | `reservation`     | Adaptive tiers, labeled FIFO admission, and evidence-gated owner burst |

Retaining a schema-1 file is a deliberate choice of the older mode, not an oversight. Any other
`schemaVersion` is rejected.

## Adaptive schema 3

Schema 3 is intentionally explicit. All pool, owner, promotion, emergency, and tier fields are
required so a partially copied machine policy fails closed.

```json
{
  "schemaVersion": 3,
  "coordination": {
    "mode": "reservation",
    "maxCpu": 8,
    "maxMemoryMiB": 16384,
    "baseActiveOwners": 2,
    "maxActiveOwners": 3,
    "emergencyAvailableMemoryMiB": 6144,
    "promotion": {
      "completedRuns": 25,
      "minimumSources": 3,
      "minimumAvailableMemoryMiB": 10240,
      "maximumCpuP95Percent": 75
    },
    "tiers": {
      "light": {
        "minimumCpu": 1,
        "maximumCpu": 2,
        "minimumMemoryMiB": 1024,
        "maximumMemoryMiB": 2048,
        "queueDeadline": "30m"
      },
      "standard": {
        "minimumCpu": 2,
        "maximumCpu": 4,
        "minimumMemoryMiB": 3072,
        "maximumMemoryMiB": 6144,
        "queueDeadline": "90m"
      },
      "heavy": {
        "minimumCpu": 4,
        "maximumCpu": 8,
        "minimumMemoryMiB": 8192,
        "maximumMemoryMiB": 16384,
        "queueDeadline": "4h"
      }
    }
  }
}
```

The third owner opens only when the newest 25 overlapping completed runs include at least three
sources and every run stayed at or above 10 GiB available memory, at or below 75% CPU p95, at normal
memory pressure, without swap-out or shedding. Current non-normal pressure closes that optional slot
immediately for new admissions; it does not kill an already admitted owner.

Every schema-3 `run` must select `--resource-tier`. Queue deadlines come from the tier, so combining
schema 3 with `--wait-for-admission` is rejected; under schema 2 the flag is rejected beside `--resource-tier` too. Upgrade every consumer first and let schema-2
owners and waiters drain before activating schema 3; a schema-3 launch returns protocol-mismatch
exit `125` naming `hippo.coordination.protocol-mismatch` before enqueue or child launch when a mixed legacy ledger remains.

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
rejected at load time with exit `125` naming `hippo.config.unreadable`, so a local file cannot weaken the compiled floors.

Schema 3 additionally requires positive `maxCpu`, `maxMemoryMiB`, `baseActiveOwners`, and
`maxActiveOwners`; exactly the `light`, `standard`, and `heavy` tiers; a positive deadline per tier;
and minimum/maximum vectors that fit inside the pool.

```console
$ hippo status --config weakened.json --disk-path .
hippo: [hippo.config.unreadable] resource configuration: maximum memory weakens the immutable 256 MiB floor
$ echo $?
125

$ hippo status --config too-many-owners.json --disk-path .
hippo: [hippo.config.unreadable] resource configuration: maxActiveOwners cannot exceed 20
$ echo $?
125
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

| Field                        | Type    | Meaning                                                          |
| ---------------------------- | ------- | ---------------------------------------------------------------- |
| `extends`                    | string  | Built-in profile this one derives from                           |
| `fallback`                   | string  | Profile to resolve to when this one does not fit                 |
| `strict`                     | boolean | When true, no fallback is attempted; a misfit replans with `125` |
| `memoryReservePercent`       | number  | Share of effective memory held back                              |
| `memoryReserveMinMiB`        | integer | Lower clamp on the memory reserve                                |
| `memoryReserveMaxMiB`        | integer | Upper clamp on the memory reserve                                |
| `noSwapMemoryReservePercent` | number  | Memory reserve when the host has no usable swap                  |
| `noSwapMemoryReserveMinMiB`  | integer | Lower clamp, no-swap case. Never below 256 MiB                   |
| `noSwapMemoryReserveMaxMiB`  | integer | Upper clamp, no-swap case                                        |
| `diskReservePercent`         | number  | Share of disk held back                                          |
| `diskReserveMinMiB`          | integer | Lower clamp on the disk reserve                                  |
| `diskReserveMaxMiB`          | integer | Upper clamp on the disk reserve                                  |
| `maxConcurrency`             | integer | Ceiling on canonical concurrency                                 |
| `maxCpuUtilizationPercent`   | number  | CPU utilization above which the profile does not fit             |

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
