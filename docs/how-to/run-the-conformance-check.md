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

```console
consumer-c bootstrap ok
consumer-b bootstrap ok
consumer-a bootstrap ok
consumer-d bootstrap ok
consumer-a overlap ok
consumer-b gate ok
consumer-d gate ok
consumer-a gate ok
consumer-c gate ok
four-consumer conformance passed with unchanged checkouts
```

Bootstrap output arrives out of order because the four consumer lanes run concurrently. That is
expected.

## Understand the phases

1. **Bootstrap** — sequential within one consumer, concurrent across all four lanes. Any bootstrap
   failure is aggregated deterministically and blocks later phases.
2. **Coordination checks** — a barrier. These are where you exercise genuine cross-repository
   overlap.
3. **Gates** — concurrent across consumers, once coordination has passed.

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

Exit `75` from that check is then recorded as an explicit capacity skip and deterministic integrity
gates continue. **Every other exit code, and every consumer-gate failure, remains a failure.**

Cleanup and integrity failures stay fatal even beside an otherwise skippable capacity exit — they are
joined with, rather than masked by, execution and reconciliation failures.

## Related

- [Command-line interface](../reference/cli.md)
- [The reservation model](../explanation/reservation-model.md)
