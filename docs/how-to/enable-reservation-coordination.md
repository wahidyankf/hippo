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

If you are invoking a release binary rather than the bootstrap, point at the file explicitly with
`--config`, or export `HIPPO_CONFIG`.

## Migrate a host that is still in exclusive mode

The two modes cannot be mixed within one state root. While any exclusive session is live, a
reservation client defers instead of taking over:

```console
$ hippo run --config hippo.local.json --disk-path . -- echo never-runs
HIPPO deferred task: shared coordination deferred admission: exclusive mode has a live or unverifiable owner.
$ echo $?
75
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
time with exit `78`:

```console
$ hippo status --config weakened.json --disk-path .
Error: resource configuration: maximum memory weakens the immutable 256 MiB floor
$ echo $?
78
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
Asking for more than the host can safely provide returns `78` immediately rather than queueing.

## Handle a full budget

A temporarily exhausted budget returns `75` after a bounded FIFO wait of five minutes. To retry
automatically instead:

```sh
hippo run --wait-for-admission 10m -- make test
```

This suits idempotent build and test commands. A retry can re-run a payload that had already started,
so keep the default of `0` for anything that is not safe to repeat.

## Related

- [Coordinate two tasks through one budget](../tutorials/coordinate-two-repositories.md)
- [Configuration](../reference/configuration.md)
- [The reservation model](../explanation/reservation-model.md)
