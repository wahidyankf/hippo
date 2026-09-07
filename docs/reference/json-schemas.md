# JSON and evidence formats

Every JSON document HIPPO emits carries a `schemaVersion`. Byte values are integer bytes, timestamps
are UTC RFC 3339 with optional fractional seconds, and an unavailable optional reading is `null` or
omitted according to that field's compatibility contract. Field _order_ is not part of any contract.

| Document                     | Schema | Produced by                 |
| ---------------------------- | ------ | --------------------------- |
| Version                      | 1      | `version --json`            |
| Monitor transition           | 1      | `monitor --json`            |
| Status                       | 4      | `status --json`             |
| Host sample (development)    | 3      | raw development evidence    |
| Host sample (release)        | 3      | `release monitor --output`  |
| Development lifetime summary | 4      | `<stream>.summary.json`     |
| Release summary              | 5      | `release monitor --summary` |

`release assess` accepts retained schema 2–5 release summaries.

## `version --json`

The smallest public document.

```json
{
  "schemaVersion": 1,
  "version": "v0.5.1",
  "commit": "5722854fddfd68b1fc7ca9feca935fe3e7eec625"
}
```

## `monitor --json`

One object per line, one line per state or profile transition.

```json
{
  "schemaVersion": 1,
  "measuredAt": "2026-09-07T04:55:28.569948Z",
  "state": "normal",
  "reason": "normal",
  "profile": "balanced",
  "swapState": "active"
}
```

## `status --json`

Schema 4. The latest host sample at the top level, plus the current assessment, the resolved
profile, privacy-safe `coordination` totals, and the `configHash`.

```json
{
  "measuredAt": "2026-09-07T05:08:11.686333Z",
  "platform": "darwin",
  "capabilities": ["compressor", "memory-pressure", "swap"],
  "effectiveMemoryLimitBytes": 34359738368,
  "availableMemoryBytes": 14087492730,
  "availableNonCompressedEstimateBytes": 14087492730,
  "memoryPressureLevel": 1,
  "compressorAvailable": true,
  "compressorPayloadBytes": 8608522560,
  "physicalMemoryBytes": 34359738368,
  "availableParallelism": 12,
  "cpuUtilizationPercent": 20.049999999999994,
  "diskFreeBytes": 84500033536,
  "diskTotalBytes": 494384795648,
  "pageSizeBytes": 16384,
  "compressorStoredPages": 1808315,
  "compressorOccupiedPages": 581039,
  "swapIns": 6335194,
  "swapOuts": 11273290,
  "swapTotalBytes": 10737418240,
  "swapUsedBytes": 9621859205,
  "swapFreeBytes": 1115559034,
  "swapState": "active",
  "schemaVersion": 4,
  "resource": {
    "compressorGrowthWindowBytes": 0,
    "swapOutWindowBytes": 0,
    "reason": "normal",
    "state": "normal",
    "storageBlocked": false,
    "swapState": "active"
  },
  "profile": {
    "requestedProfile": "balanced",
    "resolvedProfile": "balanced",
    "fallbackChain": ["balanced"],
    "strict": false,
    "concurrency": 11,
    "memoryReserveBytes": 4294967296,
    "diskReserveBytes": 21474836480,
    "decision": "run",
    "exitCode": 0,
    "retryable": false
  },
  "coordination": {
    "schemaVersion": 4,
    "mode": "reservation",
    "capacity": { "cpu": 11, "memoryBytes": 30064771072 },
    "allocated": { "cpu": 2, "memoryBytes": 1073741824 },
    "waiting": { "cpu": 0, "memoryBytes": 0 },
    "activeOwners": 1,
    "waitingOwners": 0,
    "ephemeral": 1,
    "service": 0,
    "transactional": 0
  },
  "configHash": "3e8ef8adada2d85bbb2750b80e25de0828f35bc4d032a58dd263522e02cf020b"
}
```

An idle ledger reports zeroes for `capacity`, `allocated`, and every count. That is an empty epoch,
not a corrupt one.

**Coordination corruption is an error, never a synthetic zero-total success.** If the reservation
marker, lock, or ledger cannot be decoded, `status --json` returns an error rather than reporting
totals it cannot substantiate.

### `coordination.abandonedProcessGroups`

Present only when HIPPO has recorded a supervised process group that is still running while the guard
process that owned it is gone.

```json
"coordination": {
  "mode": "reservation",
  "allocated": { "cpu": 1, "memoryBytes": 268435456 },
  "activeOwners": 1,
  "ephemeral": 1,
  "abandonedProcessGroups": [99252]
}
```

HIPPO reports these and **never signals them**. See
[How to inspect evidence and abandoned groups](../how-to/inspect-evidence-and-abandoned-groups.md).

## Host sample

The same object written one per line into raw development evidence, and embedded in every release
raw record. Schema 3.

| Group          | Fields                                                                                                                                   |
| -------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| Identity       | `schemaVersion`, `measuredAt`, `platform`, `capabilities`                                                                                |
| Memory         | `effectiveMemoryLimitBytes`, `availableMemoryBytes`, `availableNonCompressedEstimateBytes`, `memoryPressureLevel`, `physicalMemoryBytes` |
| Compressor     | `compressorAvailable`, `compressorPayloadBytes`, `compressorStoredPages`, `compressorOccupiedPages`                                      |
| CPU and disk   | `availableParallelism`, `cpuUtilizationPercent`, `diskFreeBytes`, `diskTotalBytes`                                                       |
| Swap           | `pageSizeBytes`, `swapIns`, `swapOuts`, `swapTotalBytes`, `swapUsedBytes`, `swapFreeBytes`, `swapState`                                  |
| Linux pressure | `memoryPsiSomeAvg10`, `memoryPsiFullAvg10`, `oomEvents`, `oomKillEvents`                                                                 |

The Linux pressure group appears only where the host supplies it.

## Development lifetime summary

Schema 4. The aggregate covers **every** sample taken during the session, even after older raw chunks
have rotated away.

```json
{
  "schemaVersion": 4,
  "sampleCount": 3,
  "taskClass": "transactional",
  "outcome": "passed",
  "availableParallelism": 12,
  "availableNonCompressedEstimateMinBytes": 12025908428,
  "memoryPressureLevelMax": 1,
  "compressorAvailableAll": true,
  "compressorPayloadPeakBytes": 9980680128,
  "cpuUtilizationP95Percent": 24.27,
  "diskFreeMinBytes": 84461486080,
  "swapInsDelta": 0,
  "swapOutsDelta": 0,
  "swapFreeMinBytes": 1107097026,
  "healthFailures": 0,
  "platform": "darwin",
  "capabilities": ["compressor", "memory-pressure", "swap"],
  "requestedProfile": "balanced",
  "resolvedProfile": "balanced",
  "fallbackChain": ["balanced"],
  "concurrency": 3,
  "configHash": "3e8ef8adada2d85bbb2750b80e25de0828f35bc4d032a58dd263522e02cf020b",
  "requestedCpu": 3,
  "requestedMemoryBytes": 7516192768,
  "allocatedCpu": 3,
  "allocatedMemoryBytes": 7516192768,
  "reservationWaitMilliseconds": 6,
  "peakOwnerCount": 1,
  "budgetOutcome": "admitted"
}
```

| Summary group       | Fields                                                                                                                                                                                                                                                                    |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Identity and result | `schemaVersion`, `sampleCount`, `taskClass`, `outcome`                                                                                                                                                                                                                    |
| Capacity aggregate  | `availableParallelism`, `availableNonCompressedEstimateMinBytes`, `memoryPressureLevelMax`, `compressorAvailableAll`, `compressorPayloadPeakBytes`, `cpuUtilizationP95Percent`, `diskFreeMinBytes`, `swapInsDelta`, `swapOutsDelta`, `swapFreeMinBytes`, `healthFailures` |
| Source platform     | `platform`, `capabilities`                                                                                                                                                                                                                                                |
| Resolved policy     | `requestedProfile`, `resolvedProfile`, `fallbackChain`, `concurrency`, `configHash`                                                                                                                                                                                       |
| Reservation         | `requestedCpu`, `requestedMemoryBytes`, `allocatedCpu`, `allocatedMemoryBytes`, `reservationWaitMilliseconds`, `peakOwnerCount`, `budgetOutcome`                                                                                                                          |

Schema-3 fields keep their meanings; the reservation group is the schema-4 addition.
`peakOwnerCount` is raised atomically by every admission event during the child's lifetime, so an
owner that was admitted and released between host-sampling ticks is still counted.

## Release raw record

Schema 3. The complete host sample plus six release-specific fields.

```json
{
  "oneMinuteLoad": 4.79,
  "serviceRssBytes": 22511616,
  "healthStatus": 200,
  "healthLatencyMs": 0.916,
  "routedJourneyStatus": 200,
  "routedJourneyLatencyMs": 106.944
}
```

## Release summary

Schema 5, with generic health fields.

```json
{
  "schemaVersion": 5,
  "platform": "darwin",
  "capabilities": ["compressor", "memory-pressure", "swap"],
  "sampleCount": 2,
  "availableParallelism": 12,
  "availableNonCompressedEstimateMinBytes": 14431090114,
  "memoryPressureLevelMax": 1,
  "compressorAvailableAll": true,
  "compressorPayloadPeakBytes": 7857545152,
  "physicalMemoryBytes": 34359738368,
  "cpuUtilizationP95Percent": 16.69,
  "serviceRssPeakBytes": 22528000,
  "diskFreeMinBytes": 84479193088,
  "swapInsDelta": 0,
  "swapOutsDelta": 0,
  "swapFreeMinBytes": 1107097026,
  "healthLatencyP95Ms": 1,
  "healthFailures": 0,
  "routedJourneyLatencyP95Ms": 107,
  "routedJourneyLatencyMaxMs": 106.944
}
```

`routedJourneyFailures` is present when any routed probe failed.

## Assessment result

```json
{ "accepted": true, "schemaVersion": 5 }
```

A rejected summary reports `"accepted": false` and exits `75`.

## Related

- [Shared state root](./state-root.md)
- [Command-line interface](./cli.md)
- [How to monitor a release](../how-to/monitor-a-release.md)
