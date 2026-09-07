# How to respond to a HIPPO exit code

HIPPO reserves five exit codes. Everything else came from your guarded command. This guide covers
what to actually do about each one.

For the full definitions see the [exit code reference](../reference/exit-codes.md).

## Decide quickly

| Code | Do this                                                                |
| ---- | ---------------------------------------------------------------------- |
| `1`  | Fix the command line. Nothing ran.                                     |
| `73` | Free disk space on the measured path, then retry.                      |
| `75` | **Retry.** Nothing was admitted.                                       |
| `78` | Change the request — smaller reservation, corrected value, or profile. |

Never respond to any of them by bypassing the guard or by changing `--class` to get admitted.
Changing a task to `transactional` so it cannot be shed does not make the host any bigger; it makes
the eventual failure worse.

## Handle `75` in a script

`75` is the only retryable code, and the invariant is unconditional: **an owner that receives `75`
holds no reservation**, whatever its payload printed before the deferral.

Let HIPPO do the retrying:

```sh
hippo run --wait-for-admission 10m -- make test
```

It re-attempts a deferred owner until the budget is spent, backing off from 100 ms to a 2 s ceiling,
and still reports `75` if capacity never frees.

Or handle it yourself when the payload is not safe to repeat:

```sh
if ! hippo run --disk-path . -- ./deploy.sh; then
  status=$?
  if [ "$status" -eq 75 ]; then
    echo "host is busy; not retrying a non-idempotent payload" >&2
    exit 75
  fi
  exit "$status"
fi
```

### Why `75` can arrive after your payload started

Activation records the supervised process group under the coordination lock, and it happens _after_
the child starts. A child that could not be recorded would be unsheddable under pressure, so HIPPO
stops it and reports `75` rather than supervising an untracked group.

This is why `--wait-for-admission` suits idempotent build and test commands, and why anything with
side effects should keep the default of `0`.

### When `75` means a mode conflict

```console
HIPPO deferred task: shared coordination deferred admission: exclusive mode has a live or unverifiable owner.
```

The shared root is in the other coordination mode. Do not delete state to force it. Let the existing
sessions drain and retry — see
[How to enable reservation coordination](./enable-reservation-coordination.md).

### When `75` means a heavy-work lease

In exclusive mode, the deferral names the holder:

```console
HIPPO deferred task: the heavy-work lease is held by pid 33413 (class transactional); it must exit before this work is admitted.
```

That is another repository's guarded work. Wait for it.

## Handle `73`

```console
$ hippo run --disk-path /Volumes/Small -- echo should-not-run
HIPPO decision=cleanup requested=balanced resolved=balanced.
$ echo $?
73
```

Free storage on the path you passed to `--disk-path`. The floor is 256 MiB and it is immutable.

**Do not point `--disk-path` at a roomier volume to get past the gate.** The flag names the volume
your work will actually write to; moving it elsewhere hides the problem until the build fails
halfway through with a partial artifact.

## Handle `78`

Three distinct causes, distinguished by the message.

**Impossible reservation** — ask for less:

```console
Error: reservation requires replanning: requested vector exceeds safe host capacity
Error: reservation requires replanning: reservations require at least one CPU and 256 MiB
```

**Invalid inherited mapping** — fix whatever exports the bad value:

```console
Error: concurrency environment "BUILD_WORKERS" must be a positive integer
```

**Rejected configuration** — the file weakens a compiled floor:

```console
Error: resource configuration: maximum memory weakens the immutable 256 MiB floor
Error: resource configuration: maxActiveOwners cannot exceed 20
```

Retrying any of these produces the same answer. The request has to change.

## Tell HIPPO's codes from your command's

A guarded command that itself exits `75` is indistinguishable by code alone from a HIPPO deferral.
HIPPO writes its own diagnostics to stderr and never mixes them into the child's streams, so read
stderr when the distinction matters:

```sh
stderr=$(hippo run --disk-path . -- ./task.sh 2>&1 >/dev/null)
status=$?
case "$stderr" in
  HIPPO*) echo "HIPPO decision: $stderr" >&2 ;;
  *)      echo "task exited $status" >&2 ;;
esac
```

## Related

- [Exit codes](../reference/exit-codes.md)
- [Why HIPPO fails closed](../explanation/failing-closed.md)
