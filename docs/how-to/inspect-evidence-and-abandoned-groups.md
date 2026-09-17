# How to inspect evidence and abandoned process groups

When a guarded run behaves unexpectedly, or a shared root starts deferring everything, this is how to
find out why.

## Look at the ledger first

```sh
hippo status --json --disk-path . | jq '{coordination, promotion}'
```

```console
{"coordination":{"schemaVersion":5,"mode":"reservation","activeOwners":1,"waitingOwners":7,"owners":[{"runId":"21e8...","source":"hippo","tier":"standard"}],"waiters":[{"runId":"31ad...","position":1,"source":"rhino","tier":"heavy"}]},"promotion":{"baseOwners":2,"maximumOwners":3,"effectiveOwners":2,"reason":"insufficient-overlap-runs"}}
```

Three questions this answers:

- **Is anything holding capacity?** `activeOwners` and `allocated`.
- **Is anything queued behind it?** `waitingOwners` and `waiting`.
- **Which mode is in force?** `mode`.
- **Why is owner three closed?** `promotion.reason` and `effectiveOwners`.
- **Which repository is next?** Privacy-safe `waiters[].source` and `position`.

All zeroes means an idle epoch, not a broken one.

If `status --json` returns an **error** instead of totals, the coordination state is corrupt. HIPPO
reports that rather than fabricating zero totals, because a zero total would silently license every
waiting task to start at once.

## Find the evidence for one run

Evidence lives under the state root — `~/Library/Application Support/hippo` on macOS,
`${XDG_STATE_HOME:-$HOME/.local/state}/hippo` on Linux, or wherever `HIPPO_ROOT` points.

```sh
ls "${HIPPO_ROOT:-$HOME/Library/Application Support/hippo}"
```

```console
.writers.lock
coordination.lock
development-ephemeral-1788757253723-12070.jsonl
development-ephemeral-1788757253723-12070.summary.json
development-service-1788757256450-12687.jsonl
development-service-1788757256450-12687.summary.json
development-transactional-1788757259105-13633.jsonl
development-transactional-1788757259105-13633.summary.json
reservation-identities
owner-metadata
raw
history
receipts
```

Streams are named `development-<class>-<epochMillis>-<pid>`.

## Read a lifetime summary

Prefer the public history command. It covers current summaries and daily gzip archives without
requiring knowledge of state-root filenames:

```sh
hippo history --since 7d --source hippo --tag checkout=worktree
hippo history --since 30d --resource-tier heavy --outcome pressure-shed --json
```

For a same-day run, the summary file covers the whole session even after older raw chunks rotated.

```sh
cat "$ROOT/development-transactional-1788757259105-13633.summary.json"
```

```json
{
  "schemaVersion": 5,
  "runId": "development-transactional-1788757259105-13633",
  "source": "hippo",
  "tags": { "checkout": "worktree" },
  "resourceTier": "standard",
  "sampleCount": 3,
  "taskClass": "transactional",
  "outcome": "passed",
  "cpuUtilizationP95Percent": 24.27,
  "requestedProfile": "balanced",
  "resolvedProfile": "balanced",
  "fallbackChain": ["balanced"],
  "concurrency": 3,
  "requestedCpu": 3,
  "requestedMemoryBytes": 7516192768,
  "allocatedCpu": 3,
  "allocatedMemoryBytes": 7516192768,
  "reservationWaitMilliseconds": 6,
  "peakOwnerCount": 1,
  "budgetOutcome": "admitted"
}
```

What to look at:

- **`reservationWaitMilliseconds`** — how long admission actually took. A large number means the host
  was contended, not that HIPPO was slow.
- **`peakOwnerCount`** — how many owners were live at the busiest moment during this run. Raised
  atomically by every admission event, so a peer admitted and released between sampling ticks is
  still counted.
- **`requestedCpu` vs `allocatedCpu`** — whether you got what you asked for.
- **`fallbackChain`** — the profiles HIPPO tried before settling. A chain longer than one entry means
  the host could not support the profile you requested.
- **`outcome`** and **`budgetOutcome`** — how the session ended.

Same-day raw `.jsonl` chunks hold one host sample per line for finer-grained analysis. Prior-day raw
streams are gzip-compressed under `raw/`; use `gzip -dc`. Prior-day summaries are under
`history/YYYY-MM-DD.jsonl.gz`. The summary/history view is complete without raw samples.

Safety receipts under `receipts/` answer the operationally important question after exit `75`:
`never-started` means no payload launched; `started-safety-stop` means emergency pressure stopped a
running payload. `started-activation-failure` accompanies exit `1` when ledger activation failed
after launch. Never blindly retry either started state.

## Investigate an abandoned process group

If `status --json` reports `abandonedProcessGroups`, a guard was killed outright while its child kept
running:

```json
"coordination": {
  "allocated": { "cpu": 1, "memoryBytes": 268435456 },
  "activeOwners": 1,
  "abandonedProcessGroups": [99252]
}
```

That reservation is held by work nothing is supervising any more. Nothing will shed it under
pressure, and nothing will release it while the payload runs.

**HIPPO reports these and never signals them.** Neither should you, until you have confirmed what the
group actually is. The record holds a bare process group with no start-time identity, so a group the
kernel has recycled would name an entirely unrelated process.

Confirm before acting:

```sh
ps -o pid=,pgid=,lstart=,command= -p 99252
pgrep -g 99252
```

Check three things:

1. **Does the process still exist?** If not, the report is already stale — see below.
2. **Is its start time consistent** with when you believe the guarded run began? A process that
   started _after_ the guard died is a recycled identifier, not your payload.
3. **Is the command what you expected to be guarding?** If it is your editor or an unrelated service,
   the identifier was reused. Do not signal it.

Only once all three check out is the group yours to terminate:

```sh
kill -TERM -99252     # note the leading '-': signals the group
```

## Wait rather than intervene, where you can

An abandoned group's reservation is reclaimed automatically once that group actually retires. The
next guard to take the coordination lock reconciles liveness and releases it:

```console
$ hippo status --config reservation.json --json --disk-path . | grep -o '"coordination":.*'
"coordination":{...,"allocated":{"cpu":0,"memoryBytes":0},"activeOwners":0,...}
```

So the leak is bounded by the payload's own lifetime. Manual intervention is only needed for a
payload that will never exit on its own.

## When the root will not admit anything

If admission fails with `1` and the ledger looks empty, shared state may be corrupt or inaccessible.
Exit `76` instead means the state is valid but uses an incompatible peer protocol. HIPPO preserves
both kinds of bytes instead of clearing them; exit `75` remains a capacity or safety result.

Recovery is manual and deliberate:

1. Inspect the state root.
2. Confirm that no owner remains — check every PID and process group it names.
3. For exit `1`, correct whatever made the files inaccessible or malformed. For exit `76`, drain
   the live epoch or upgrade every client that shares the root.
4. Retry.

Clearing state to "unstick" a root is how a live owner's record disappears and every waiting task
starts at once. See [Why HIPPO fails closed](../explanation/failing-closed.md).

## Related

- [Shared state root](../reference/state-root.md)
- [JSON schemas](../reference/json-schemas.md)
- [Process ownership and shedding](../explanation/process-ownership-and-shedding.md)
