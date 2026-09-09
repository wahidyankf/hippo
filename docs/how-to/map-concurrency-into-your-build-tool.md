# How to map concurrency into your build tool

Your build tool already reads some environment variable to decide how many workers to start. HIPPO
can write its allocation into that variable, so the tool sizes itself to what the host can spare
instead of to the core count.

HIPPO does not know about any build tool. You supply the name.

## Map one variable

```sh
hippo run --concurrency-env BUILD_WORKERS -- ./build.sh
```

Inside `build.sh`, `BUILD_WORKERS` now holds the resolved concurrency.

## Map several

`--concurrency-env` is repeatable, so one guarded command can feed several tools at once:

```sh
hippo run \
  --concurrency-env BUILD_WORKERS \
  --concurrency-env TEST_JOBS \
  -- make test
```

```console
$ hippo run --disk-path . --concurrency-env BUILD_WORKERS --concurrency-env TEST_JOBS -- sh -c 'echo "BUILD_WORKERS=$BUILD_WORKERS TEST_JOBS=$TEST_JOBS HIPPO_CONCURRENCY=$HIPPO_CONCURRENCY"'
BUILD_WORKERS=11 TEST_JOBS=11 HIPPO_CONCURRENCY=11
```

## Common target variables

You do not need HIPPO's permission to use any of these — they are just names your tools already read.

| Tool          | Variable                                          |
| ------------- | ------------------------------------------------- |
| GNU Make      | Consume `$MAKE_JOBS` in `make -j$MAKE_JOBS`       |
| Cargo         | `CARGO_BUILD_JOBS`                                |
| Go test       | Consume it in `go test -p "$GO_TEST_JOBS"`        |
| Jest          | Consume it in `jest --maxWorkers="$JEST_WORKERS"` |
| Nx            | `NX_PARALLEL`                                     |
| Ninja / CMake | Consume it in `ninja -j"$NINJA_JOBS"`             |

Where a tool has no native variable, pass the value through on the command line:

```sh
hippo run --concurrency-env JOBS -- sh -c 'make -j"$JOBS" all'
```

## Understand how your existing value is treated

A value you already exported is treated differently in each coordination mode, so establish which
mode you are in before relying on either behavior. Reservation mode reconciles the value against the
allocation; exclusive mode passes it through untouched.

### Reservation mode reconciles it

Under a fixed reservation allocation the existing value is read as a request and reconciled against
the allocation.

| Your value                   | Result                            |
| ---------------------------- | --------------------------------- |
| unset                        | Receives the allocated CPU        |
| positive, below allocation   | Survives unchanged                |
| positive, above allocation   | Clamped down to the allocation    |
| zero, negative, or malformed | Exit `78` before the child starts |

```console
$ BUILD_WORKERS=1 hippo run --config reservation.json --reserve-cpu 4 --concurrency-env BUILD_WORKERS -- sh -c 'echo "BUILD_WORKERS=$BUILD_WORKERS HIPPO_CONCURRENCY=$HIPPO_CONCURRENCY"'
BUILD_WORKERS=1 HIPPO_CONCURRENCY=4

$ BUILD_WORKERS=64 hippo run --config reservation.json --reserve-cpu 2 --concurrency-env BUILD_WORKERS -- sh -c 'echo "BUILD_WORKERS=$BUILD_WORKERS HIPPO_CONCURRENCY=$HIPPO_CONCURRENCY"'
BUILD_WORKERS=2 HIPPO_CONCURRENCY=2
```

A deliberately low value is respected; an optimistic one is capped. This means you can keep an
existing `BUILD_WORKERS=2` in a `.env` and HIPPO will not raise it.

### Exclusive mode leaves it alone

Schema 1 — the default when you supply no configuration — retains the v0.3.1 mapping behavior. A
mapped variable is written only when it is missing, so a value already in the environment reaches
the child unchanged. Nothing is clamped and nothing is rejected, and an optimistic value therefore
survives even though `HIPPO_CONCURRENCY` reports the smaller allocation beside it.

```console
$ BUILD_WORKERS=64 hippo run --disk-path . --concurrency-env BUILD_WORKERS -- sh -c 'echo "BUILD_WORKERS=$BUILD_WORKERS HIPPO_CONCURRENCY=$HIPPO_CONCURRENCY"'
BUILD_WORKERS=64 HIPPO_CONCURRENCY=11
```

Two consequences are worth planning around. A stale `BUILD_WORKERS=64` inherited from a shell
profile or a `.env` oversubscribes the host despite the guard, and a `0` or a typo reaches your
build tool rather than being caught. If you want the reconciliation, enable
[reservation coordination](./enable-reservation-coordination.md); otherwise keep the variable out of
the environment and let HIPPO supply it.

Degraded admission is the one exclusive-mode case that does overwrite an existing value — see
[the note below](#note-on-degraded-admission).

## Fix a rejected mapping

Two different failures look similar. Check the exit code.

**Exit `1` — the name is wrong.** Rejected before anything runs.

```console
$ hippo run --concurrency-env '1BAD-NAME' -- true
Error: concurrency environment name "1BAD-NAME" is not a POSIX identifier

$ hippo run --concurrency-env HIPPO_CONCURRENCY -- true
Error: concurrency environment name "HIPPO_CONCURRENCY" is reserved
```

Use a POSIX identifier that is not one of HIPPO's own `HIPPO_*` protocol variables.

**Exit `78` — the inherited value is wrong.** Reservation mode only; exclusive mode does not
inspect the value.

```console
$ BUILD_WORKERS=0 hippo run --config reservation.json --concurrency-env BUILD_WORKERS -- true
Error: concurrency environment "BUILD_WORKERS" must be a positive integer
```

Something upstream is exporting `0`, an empty string, or a non-number. Fix the source rather than
dropping the mapping — a `0` reaching a build tool is usually a bug on its own.

## Note on degraded admission

Balanced ephemeral work on macOS can admit under a stable warning window at concurrency `1`. When
that happens, **every** mapped variable is forced to `1` as well, and HIPPO says so:

```console
HIPPO admitting ephemeral child under stable macOS warning pressure with concurrency 1.
```

A build that suddenly runs single-threaded on a busy Mac is doing that on purpose.

## Related

- [Environment variables](../reference/environment-variables.md)
- [Exit codes](../reference/exit-codes.md)
