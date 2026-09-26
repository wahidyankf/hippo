# How to run the four-consumer conformance check

`hippo-conformance` verifies that four independent repositories can adopt one pinned HIPPO binary and
coordinate through one shared state root without interfering with each other.

It is a manifest-driven adoption check. HIPPO compiles in no consumer name, path, command, port, or
product default — everything comes from the manifest.

## Write a manifest

Start from [`conformance.manifest.json.example`](../../conformance.manifest.json.example).

```json
{
  "schemaVersion": 1,
  "hippoBinary": "/path/to/pinned/hippo",
  "hippoSha256": "7df96ce966601c87d65c41f428689c5c9e88bdae797e21c39e289f48ec62ac2b",
  "sharedRoot": "/path/to/temporary/shared-state",
  "consumers": [
    {
      "name": "consumer-a",
      "path": "/path/to/consumer-a",
      "bootstrap": [{ "arguments": ["./bootstrap"] }],
      "gates": [{ "arguments": ["./run-gate"] }]
    }
  ],
  "coordinationChecks": [
    {
      "consumer": "consumer-a",
      "command": { "arguments": ["./check-shared-overlap"] },
      "allowCapacitySkip": true
    }
  ]
}
```

Exactly **four** consumers are required.

Get the pinned binary's digest with:

```sh
shasum -a 256 /path/to/pinned/hippo   # or sha256sum
```

### Path rules

Absolute, relative, and symlink-resolved paths must identify four **different** checkouts, and none
of them may overlap the shared root in either direction. The harness freezes each checkout object
plus the created shared-root object and revalidates both around every command, so a path that moves
mid-run is a failure rather than a silent substitution.

## Run it

```sh
go run ./cmd/hippo-conformance /path/to/manifest.json
```

The harness itself prints little: one line per passed deferral probe, one per capacity skip, and the
closing line. Everything else on the terminal is your consumer commands' own output, interleaved
because the four lanes run concurrently. A passing run ends with:

```console
four-consumer conformance passed with unchanged checkouts
```

## Understand the phases

1. **Bootstrap** — sequential within one consumer, concurrent across all four lanes. Any bootstrap
   failure is aggregated deterministically and blocks later phases.
2. **Deferral probes** — for each consumer that declares one, see below.
3. **Coordination checks** — a barrier. These are where you exercise genuine cross-repository
   overlap.
4. **Gates** — concurrent across consumers, once coordination has passed.

Before any command runs, the harness snapshots every checkout's `HEAD` and its complete dirty-path
set. The closing line `with unchanged checkouts` is that reconciliation passing.

## Understand the isolation

Each command receives its own read-and-execute-only copy of one safely opened verified binary. The
exact command copy is checked _after_ execution; the hidden master and the manifest source are
checked before later work and again at final reconciliation.

The consumer base environment is scrubbed: the caller's `HIPPO_SESSION`, its fixed-allocation
outputs, and the caller repository's bootstrap-only `HIPPO_DEFAULT_CONFIG` are all removed. An
explicit operator `HIPPO_CONFIG` survives, and each consumer's own outer guard may establish the only
session its nested work inherits.

Every started process group must retire completely — after normal, nonzero, or cancelled leader exit
— before its phase can finish. Cancellation prevents the next sequential command from starting,
applies bounded TERM/KILL observation, and reconciliation then runs on a fresh deadline.

## Prove a consumer retries a deferral

A consumer that reads a capacity deferral as an admission would oversubscribe the host, so the
harness can test that one contract directly. Give the consumer a `deferralRetryProbe` command:

```json
{
  "name": "consumer-a",
  "path": "/path/to/consumer-a",
  "deferralRetryProbe": { "arguments": ["./probe-deferral-retry"] },
  "gates": [{ "arguments": ["./run-gate"] }]
}
```

The harness runs the probe with `HIPPO_ROOT` pointing at a fresh root that it holds saturated for two
seconds, marked by a `conformance-capacity-held` file beside a `never-started` receipt in
`conformance-never-started-receipt.json`. The probe passes only if it exits `0` after the hold
lifts, and fails if it exits early or gives up on the deferral:

```console
consumer "consumer-a" retried a capacity deferral instead of reading it as an admission
```

A consumer without a probe is not checked for this, and nothing says so at run time.

## Allow a capacity skip

A live coordination check can legitimately fail on a host too small to reproduce real overlap. Set
`allowCapacitySkip` on that check:

```json
{
  "consumer": "consumer-a",
  "command": { "arguments": ["./check-shared-overlap"] },
  "allowCapacitySkip": true
}
```

Exit `124` is recorded as an explicit capacity skip only when the command emits the
`hippo: [hippo.limit.capacity-deferred]` diagnostic and creates a new schema-1 safety
receipt whose state is `never-started`. Bare `124`, a diagnostic without that receipt, a protocol
mismatch, every other exit code, and every consumer-gate failure remain failures.

Cleanup and integrity failures stay fatal even beside an otherwise skippable capacity exit. The
harness joins them with execution and reconciliation failures instead of masking them.

## Related

- [Command-line interface](../reference/cli.md)
- [The reservation model](../explanation/reservation-model.md)
