# JSON and evidence formats

Every JSON document HIPPO emits carries a `schemaVersion`. Byte values are integer bytes, timestamps
are UTC RFC 3339 with optional fractional seconds, and an unavailable optional reading is `null` or
omitted according to that field's compatibility contract. Field _order_ is not part of any contract.

| Document                     | Schema | Produced by                     |
| ---------------------------- | ------ | ------------------------------- |
| Version                      | 1      | `version --json`                |
| Monitor transition           | 1      | `monitor --json`                |
| Status                       | 5      | `status --json`, `watch --json` |
| History envelope             | 1      | `history --json`                |
| Run identity                 | 1      | `hippo.identity.json`           |
| Safety receipt               | 1      | `receipts/*.json`               |
| Host sample (development)    | 3      | raw development evidence        |
| Host sample (release)        | 3      | `release monitor --output`      |
| Development lifetime summary | 5      | `<stream>.summary.json`         |
| Release summary              | 5      | `release monitor --summary`     |

`release assess` accepts retained schema 2–5 release summaries.

## `version --json`

The smallest public document has three required fields: integer `schemaVersion` `1`, the release
string in `version`, and the exact source commit in `commit`. Release builds currently report
`v0.8.2`; source builds report `dev` and `unknown`.

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

Schema 5. The latest host sample at the top level, plus the current assessment, resolved profile,
privacy-safe `coordination` rows and totals, owner-promotion decision, and `configHash`.

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
  "schemaVersion": 5,
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
    "schemaVersion": 5,
    "mode": "reservation",
    "capacity": { "cpu": 11, "memoryBytes": 30064771072 },
    "allocated": { "cpu": 4, "memoryBytes": 6442450944 },
    "waiting": { "cpu": 0, "memoryBytes": 0 },
    "activeOwners": 1,
    "waitingOwners": 0,
    "ephemeral": 1,
    "service": 0,
    "transactional": 0,
    "owners": [
      {
        "runId": "21e8b2...",
        "state": "active",
        "class": "ephemeral",
        "profile": "balanced",
        "source": "hippo",
        "tags": { "checkout": "worktree", "plan": "maximize-hippo" },
        "tier": "standard",
        "requested": { "cpu": 4, "memoryBytes": 6442450944 },
        "allocated": { "cpu": 4, "memoryBytes": 6442450944 },
        "minimum": { "cpu": 2, "memoryBytes": 3221225472 },
        "maximum": { "cpu": 4, "memoryBytes": 6442450944 },
        "registeredAt": "2026-09-17T04:01:00Z",
        "deadline": "2026-09-17T05:31:00Z"
      }
    ]
  },
  "promotion": {
    "eligible": false,
    "qualifyingRuns": 12,
    "sources": 3,
    "reason": "insufficient-overlap-runs",
    "baseOwners": 2,
    "maximumOwners": 3,
    "effectiveOwners": 2
  },
  "configHash": "3e8ef8adada2d85bbb2750b80e25de0828f35bc4d032a58dd263522e02cf020b"
}
```

An idle coordination epoch reports zeroes for `capacity`, `allocated`, and every count. In exclusive
mode, each live compatibility session appears as an `active` owner with `legacy: true`; the heavy
lease and its matching session are one owner, not two. Exclusive mode has no registered waiter rows
and does not invent reservation vectors.

**Coordination corruption is an error, never a synthetic zero-total success.** If the reservation
marker, lock, or ledger cannot be decoded, `status --json` returns an error rather than reporting
totals it cannot substantiate.

`owners` and `waiters` are safe operational rows. They contain opaque run IDs, labels, class,
profile, tier, requested/allocated vectors, tier minimum/maximum, registration time, and deadline—never commands, arguments, working
directories, or repository paths. `legacyEntries` counts live compatibility owners and live schema-2
records without metadata; schema-3 launches refuse to mix with them until they drain.

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

## Run identity

A repository may track this small, customizable document:

```json
{
  "schemaVersion": 1,
  "source": "hippo",
  "tags": { "group": "ose-projects" }
}
```

`source` is a lowercase safe token. There may be at most eight tag pairs and the encoded document
may not exceed 512 bytes. Unknown fields, duplicate JSON fields, path-like values, and invalid
labels are rejected. Invocation `--source`/`--tag` overrides use the same validation.

## Development lifetime summary

Schema 5. The aggregate covers **every** sample taken during the session, even after older raw chunks
have rotated away.

```json
{
  "schemaVersion": 5,
  "runId": "development-transactional-1788757259105-13633",
  "startedAt": "2026-09-17T04:01:00Z",
  "finishedAt": "2026-09-17T04:03:20Z",
  "source": "hippo",
  "tags": { "checkout": "worktree", "plan": "maximize-hippo" },
  "resourceTier": "standard",
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
| Identity and result | `schemaVersion`, `runId`, `startedAt`, `finishedAt`, `source`, `tags`, `resourceTier`, `sampleCount`, `taskClass`, `outcome`                                                                                                                                              |
| Capacity aggregate  | `availableParallelism`, `availableNonCompressedEstimateMinBytes`, `memoryPressureLevelMax`, `compressorAvailableAll`, `compressorPayloadPeakBytes`, `cpuUtilizationP95Percent`, `diskFreeMinBytes`, `swapInsDelta`, `swapOutsDelta`, `swapFreeMinBytes`, `healthFailures` |
| Source platform     | `platform`, `capabilities`                                                                                                                                                                                                                                                |
| Resolved policy     | `requestedProfile`, `resolvedProfile`, `fallbackChain`, `concurrency`, `configHash`                                                                                                                                                                                       |
| Reservation         | `requestedCpu`, `requestedMemoryBytes`, `allocatedCpu`, `allocatedMemoryBytes`, `reservationWaitMilliseconds`, `peakOwnerCount`, `budgetOutcome`                                                                                                                          |

`outcome` is one of these values, and no other:

| `outcome`               | Child started? | Meaning                                                                              |
| ----------------------- | -------------- | ------------------------------------------------------------------------------------ |
| `passed`                | Yes            | The child exited `0`                                                                 |
| `task-failed`           | Yes            | The child exited nonzero, was stopped by a signal to HIPPO, or failed its activation |
| `pressure-shed`         | Yes            | HIPPO shed the child under host pressure other than storage                          |
| `storage-shed`          | Yes            | HIPPO shed the child because the disk floor was crossed                              |
| `emergency-safety-stop` | Yes            | HIPPO stopped transactional work past the emergency floor                            |
| `supervision-failed`    | Yes            | HIPPO lost supervision of a running child and stopped it                             |
| `capacity-deferred`     | No             | Safe host admission was not reached before the admission deadline                    |
| `storage-blocked`       | No             | The disk floor refused the run before launch                                         |
| `admission-cancelled`   | No             | A signal or other cancellation stopped the run while it sampled the host             |

A summary exists only for a run that reached host sampling. A run cancelled while it waited in the
reservation queue collected no host evidence, so it writes no summary; its `never-started` receipt
with reason `admission-cancelled` is its whole record. `admission-cancelled` is new in v0.8.2;
before it, a run cancelled during host sampling was summarized as `capacity-deferred`.

A run that HIPPO itself stops before launch after sampling began, because host evidence became
unreadable, an evidence write was refused, or the launch failed, is also summarized as
`capacity-deferred`. Its exit status and reason say which; the outcome does not.

Schema-4 fields keep their meanings; labels and timestamps are the schema-5 addition.
`peakOwnerCount` is raised atomically by every admission event during the child's lifetime, so an
owner that was admitted and released between host-sampling ticks is still counted.

## `history --json`

```json
{
  "schemaVersion": 1,
  "since": "30d",
  "rows": [
    {
      "schemaVersion": 5,
      "runId": "development-transactional-1788757259105-13633",
      "finishedAt": "2026-09-17T04:03:20Z",
      "source": "hippo",
      "resourceTier": "standard",
      "taskClass": "transactional",
      "outcome": "passed"
    }
  ]
}
```

The example row is abridged. A row carries every field below that has a value; empty fields are
omitted:

| Group        | Fields                                                                                                          |
| ------------ | --------------------------------------------------------------------------------------------------------------- |
| Identity     | `schemaVersion`, `runId`, `startedAt`, `finishedAt`, `source`, `tags`, `resourceTier`, `taskClass`              |
| Result       | `outcome`, `budgetOutcome`, `peakOwnerCount`, `aggregateCount`                                                  |
| Host summary | `availableNonCompressedEstimateMinBytes`, `memoryPressureLevelMax`, `cpuUtilizationP95Percent`, `swapOutsDelta` |

History rows are the queryable safe subset of lifetime summaries. Under the 128 MiB history cap,
the oldest daily archive is first aggregated by source, exact tags, class, tier, and outcome;
`aggregateCount` then reports how many original runs the row represents. Aggregates never qualify
for owner promotion.

## Safety receipt

```json
{
  "schemaVersion": 1,
  "runId": "21e8b2...",
  "recordedAt": "2026-09-17T04:03:20Z",
  "state": "never-started",
  "reason": "admission-deadline",
  "source": "hippo",
  "tags": { "plan": "maximize-hippo" },
  "resourceTier": "standard",
  "taskClass": "ephemeral"
}
```

`state` distinguishes `never-started` queue expiry/cancellation from `started-safety-stop`
emergency pressure and `started-activation-failure` after a launched child could not be activated in
the ledger. A `never-started` receipt names `admission-deadline` when the queue deadline passed,
`host-admission` when host sampling never became safe, and `admission-cancelled` when a signal
stopped the run first. Only `never-started` authorizes automatic requeue. Receipts contain no command or path
and are retained for 30 days under a 128 MiB cap.

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

A rejected summary reports `"accepted": false` and exits `124`.

## Related

- [Shared state root](./state-root.md)
- [Command-line interface](./cli.md)
- [How to monitor a release](../how-to/monitor-a-release.md)
