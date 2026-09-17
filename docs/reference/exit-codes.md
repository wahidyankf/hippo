# Exit codes

HIPPO's exit codes are a stable contract. A caller can branch on them without parsing diagnostics.

| Code    | Name             | Meaning                                                                       | Retry?                  |
| ------- | ---------------- | ----------------------------------------------------------------------------- | ----------------------- |
| `0`     | Success          | The command completed, or the guarded child exited `0`                        | n/a                     |
| `1`     | Usage            | Arguments or flags were rejected before any work started                      | No — fix the invocation |
| `73`    | Cleanup required | Storage is below the immutable floor; free space before retrying              | No — free disk first    |
| `75`    | Deferred/stopped | Admission expired, release rejected, or a supervised child was safety-stopped | Inspect receipt first   |
| `78`    | Replan required  | Configuration, impossible reservation, invalid mapping, or strict profile     | No — change the request |
| _other_ | Child exit code  | A guarded child's own exit code is passed through unchanged                   | Depends on the child    |

Any exit code other than the five above came from the guarded command itself, not from HIPPO.

## `0` — success

`hippo run` returns the child's exit status. A child that exits `0` produces `0`.

## Child exit codes pass through

```console
$ hippo run --disk-path . -- sh -c 'exit 42'
$ echo $?
42
```

Because arbitrary child codes pass through, a child that itself exits `73`, `75`, or `78` is
indistinguishable by code alone from a HIPPO decision. HIPPO writes its own diagnostics to stderr
and never mixes them into the child's streams, so a caller that must tell the two apart should read
stderr rather than infer from the code.

## `1` — usage

The invocation was rejected before anything ran. Usage errors print the command usage next to the
diagnostic.

Known causes:

- A missing `--` boundary before the guarded command.
- An unknown command or unusable flag.
- A `--class` value other than `ephemeral`, `service`, or `transactional`.
- A `--concurrency-env` name that is not a POSIX identifier, or that collides with one of HIPPO's own
  protocol variables.
- `release monitor` without `--health-url` or without `--routed-origin`.
- `release monitor` with both `--output -` and `--summary -`.

```console
$ hippo run --disk-path . --concurrency-env HIPPO_CONCURRENCY -- true
Error: concurrency environment name "HIPPO_CONCURRENCY" is reserved
$ echo $?
1

$ hippo run --disk-path . --concurrency-env '1BAD-NAME' -- true
Error: concurrency environment name "1BAD-NAME" is not a POSIX identifier
$ echo $?
1
```

## `73` — cleanup required

Free space on the measured `--disk-path` is below the immutable 256 MiB hard floor, or the reading is
unavailable. No child is started.

```console
$ hippo status --disk-path /Volumes/Tiny
state=critical reason=disk-critical profile=balanced concurrency=11 swap=active availableGiB=11.84 diskFreeGiB=0.02 cpu=23.0%

$ hippo run --disk-path /Volumes/Tiny -- echo should-not-run
HIPPO decision=cleanup requested=balanced resolved=balanced.
$ echo $?
73
```

**Response:** free storage on the measured path, then retry. Do not point `--disk-path` somewhere
roomier to get past the gate — the gate is measuring the volume the work will actually write to.

## `75` — deferred or safety-stopped

Exit `75` is transient only when HIPPO records `state: "never-started"`. A supervised payload may
also return `75` after ordinary pressure shedding or an emergency safety stop. Never infer retry
safety from the number alone; inspect stderr and the bounded receipt/history record.

Known causes:

- Reservation capacity is temporarily exhausted, and the bounded FIFO wait expired.
- A coordination lock was held by a peer repository through the bounded window.
- The shared root is in the other coordination mode: a reservation client meets a live exclusive
  epoch, or a compatibility client meets a live reservation epoch.
- `release assess` rejected the summary as outside the release envelope.

```console
$ hippo run --config reservation.json --disk-path . -- echo never-runs
HIPPO deferred task: shared coordination deferred admission: exclusive mode has a live or unverifiable owner.
$ echo $?
75
```

**Response:** schema 3 already waits with one FIFO identity until the chosen tier deadline. Schema 2
can opt into the same single-waiter behavior with `--wait-for-admission`. A 30-second heartbeat shows
the run ID, current position, and remaining time:

```console
$ hippo run --wait-for-admission 10m --disk-path . -- make test
HIPPO waiting for admission run=6f... position=3 remaining=9m29s
HIPPO admission deadline expired before payload start.
$ echo $?
75
```

No payload is launched while queued, and HIPPO never auto-retries one. Receipts distinguish
`never-started` admission deadline/cancellation from `started-safety-stop` emergency pressure. An
ordinary `pressure-shed` outcome likewise means the payload started and must not be blindly retried.

## `78` — replan required

The request cannot be satisfied as written. No amount of retrying changes the answer.

Known causes:

- The requested reservation vector exceeds safe host capacity.
- The requested reservation is below the one-CPU or 256 MiB floor.
- Schema 3 lacks `--resource-tier`, uses an unknown tier, or receives a vector outside that tier.
- Schema 3 sees legacy schema-2 owners or waiters that have not drained.
- The identity document or invocation labels are missing or invalid under schema 3.
- A mapped `--concurrency-env` variable already holds a zero, negative, or malformed value.
- A strict profile (`transactional` or `release` class) has no usable fallback under current pressure.

```console
$ hippo run --config reservation.json --disk-path . --reserve-cpu 9999 -- true
Error: reservation requires replanning: requested vector exceeds safe host capacity
$ echo $?
78

$ hippo run --config reservation.json --disk-path . --reserve-memory-mib 1 -- true
Error: reservation requires replanning: reservations require at least one CPU and 256 MiB
$ echo $?
78

$ BUILD_WORKERS=0 hippo run --config reservation.json --concurrency-env BUILD_WORKERS -- true
Error: concurrency environment "BUILD_WORKERS" must be a positive integer
$ echo $?
78
```

**Response:** change the request — a smaller reservation, a corrected environment value, or a
different profile. Never bypass the guard or change task class to obtain admission.

## Related

- [How to respond to a HIPPO exit code](../how-to/respond-to-exit-codes.md)
- [Why HIPPO fails closed](../explanation/failing-closed.md)
- [Command-line interface](./cli.md)
