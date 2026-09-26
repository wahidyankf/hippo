# How to enable reservation coordination

Reservation mode lets several guarded tasks run concurrently against one shared CPU-and-memory
budget. Without it, HIPPO uses the older exclusive mode, where one heavy task runs at a time
host-wide.

## Write a schema-2 configuration

The configuration file is machine-local and must not be committed. In a repository checkout,
`hippo.local.json` is already ignored and is discovered automatically by the `./hippo` bootstrap.

```sh
cat > hippo.local.json <<'JSON'
{
  "schemaVersion": 2,
  "coordination": { "mode": "reservation" }
}
JSON
```

Confirm it took effect:

```sh
./hippo status --json --disk-path . | grep -o '"mode":"[a-z]*"'
```

```console
"mode":"reservation"
```

A release binary started directly, rather than through the bootstrap, loads `hippo.local.json` from
its current directory only. To use the file from anywhere else, point at it with `--config` or
export `HIPPO_CONFIG`. See [Configuration](../reference/configuration.md#precedence).

## Migrate a host that is still in exclusive mode

The two modes cannot be mixed within one state root. While any exclusive session is live, a v0.7
reservation client reports protocol mismatch instead of taking over:

```console
$ hippo run --config hippo.local.json --disk-path . -- echo never-runs
HIPPO protocol mismatch: shared coordination protocol mismatch: exclusive mode has a live owner; drain or upgrade the incompatible client before retrying.
hippo: [hippo.coordination.protocol-mismatch] live peer coordination state this client cannot safely join
$ echo $?
125
```

This is not an error to work around. **Let the old sessions drain**, then retry. HIPPO refuses to
create a mixed epoch precisely so that two repositories can never disagree about which protocol is in
force.

Find out what is holding the root:

```sh
hippo status --json --disk-path . | grep -o '"coordination":.*'
```

Exclusive heavy work reports its holder directly in the deferral message, including the PID and task
class.

## Upgrade to adaptive schema 3

Upgrade every consumer binary and wrapper to a build that reports a protocol mismatch under its own
reason code, shows live exclusive owners in status, and bounds activation contention; the current
tagged baseline is v0.8.2. Verify each exact binary
with `version --json` and its release checksum instead of inferring capability from SemVer ordering.
Keep schema 2 active until `hippo status --json` reports no owners or waiters with `legacy: true`, then
atomically install the schema-3 policy. A schema-3 launch returns `125` naming `hippo.coordination.protocol-mismatch` before enqueue or child launch
when legacy ledger entries remain.

Schema 3 requires an identity and a resource tier:

```json
{
  "schemaVersion": 1,
  "source": "my-repository",
  "tags": { "group": "my-projects" }
}
```

```sh
hippo run --resource-tier standard --tag checkout=worktree -- make test
```

The complete recommended 8-CPU/16-GiB configuration, promotion gate, emergency floor, and tier
vectors are in [Configuration](../reference/configuration.md#adaptive-schema-3).

## Tighten the budget

The defaults give `balanced`, `constrained`, and `minimal` profiles four, two, and one automatic
owner respectively. To reserve less of the machine, add caps:

```json
{
  "schemaVersion": 2,
  "coordination": {
    "mode": "reservation",
    "maxCpu": 6,
    "maxMemoryMiB": 16384,
    "maxActiveOwners": 3
  }
}
```

With those caps on a 12-core, 32 GiB host, the automatic `balanced` share becomes a quarter of the
capped budget rather than a quarter of the machine:

```console
$ hippo run --config hippo.local.json --disk-path . -- sh -c 'echo "cpu=$HIPPO_CONCURRENCY mem=$HIPPO_RESERVED_MEMORY_BYTES"'
cpu=2 mem=4294967296
```

That is 6 CPU and 16 GiB divided four ways.

Caps may only _tighten_. `maxMemoryMiB` below 256 and `maxActiveOwners` above 20 are rejected at load
time with exit `125`:

```console
$ hippo status --config weakened.json --disk-path .
hippo: [hippo.config.unreadable] resource configuration: maximum memory weakens the immutable 256 MiB floor
$ echo $?
125
```

Remember that `maxActiveOwners` composes as a **minimum** across the shared root: the strictest live
participant sets the limit for everyone until it leaves.

## Give one task a fixed allocation

Automatic shares are right for ordinary work. When you have measured a task's real footprint, ask for
exactly that:

```sh
hippo run --reserve-cpu 2 --reserve-memory-mib 1024 -- make test
```

An explicit reservation may be smaller than the automatic share but never below one CPU or 256 MiB.
Asking for more than the host can safely provide returns `125` immediately rather than queueing.

## Handle a full budget

A temporarily exhausted schema-2 budget returns `124` after its bounded FIFO wait. To wait longer
before the single payload launch:

```sh
hippo run --wait-for-admission 10m -- make test
```

This creates one FIFO waiter and launches the payload at most once. A run with `--resource-tier`
rejects this flag with exit `2`, under schema 2 as under schema 3, and uses the tier's deadline
instead: the compiled 30 minutes, 90 minutes, or four hours under schema 2, and the configured
`queueDeadline` under schema 3.

## Related

- [Coordinate two tasks through one budget](../tutorials/coordinate-two-repositories.md)
- [Configuration](../reference/configuration.md)
- [The reservation model](../explanation/reservation-model.md)
