# How to respond to a HIPPO exit code

HIPPO answers in two parts, and both matter here. The **status** tells a shell what to do. The
**reason** on stderr tells you which case you are in, and two reasons under one status can need
opposite responses.

For the full definitions see the [exit code reference](../reference/exit-codes.md).

## Decide quickly

| Status | Do this                                                                     |
| ------ | --------------------------------------------------------------------------- |
| `1`    | Nothing matched. This is a result, not a failure.                           |
| `2`    | The invocation is wrong. Read the diagnostic and fix the command.           |
| `124`  | A limit stopped the work. Read the reason — see below; they differ.         |
| `125`  | HIPPO failed to start or supervise the work. Read the reason; do not retry. |
| `126`  | The command exists and cannot be executed. Fix its permissions.             |
| `127`  | The command is not there. Fix the path or the spelling.                     |

Never respond to any of them by bypassing the guard or by changing `--class` to get admitted.
Changing a task to `transactional` so it cannot be shed does not make the host any bigger; it makes
the eventual failure worse.

## Handle `124`

Three reasons share this status, and only one of them is a plain "wait and retry".

| Reason                          | Do this                                                      |
| ------------------------------- | ------------------------------------------------------------ |
| `hippo.limit.capacity-deferred` | Retry when the host is quieter, subject to the receipt below |
| `hippo.limit.pressure-shed`     | The payload ran. Recover it before repeating anything        |
| `hippo.limit.storage-blocked`   | Free disk on the measured path. Waiting will not do it       |

`error.retryable` in the `--output json` body says the same thing: it is `true` for the first two
and `false` for storage, because a caller that retries on a full disk retries forever.

### Tell never-started from started

A deferral before launch is safe to requeue once. A shed after launch is not.

If `HIPPO_ROOT` is not set in your shell, set it to your state root first; see
[Shared state root](../reference/state-root.md#location) for each platform's default. `receipts/`
exists only once a receipt has been written.

```sh
hippo history --since 1d --source my-repo --outcome emergency-safety-stop
ls "$HIPPO_ROOT/receipts"
```

Queue expiry or cancellation writes `state: "never-started"`. Emergency termination writes
`state: "started-safety-stop"`. Ordinary pressure shedding is recorded in the lifetime summary as
`pressure-shed` or `storage-shed`.

Let HIPPO wait before launch rather than looping yourself. A tier sets the deadline; under schema 2,
a run without a tier can set one instead:

```sh
hippo run --wait-for-admission 10m -- make test
```

It creates one stable FIFO waiter, reports position every 30 seconds, and starts the payload once at
most. HIPPO never runs a payload retry loop.

Or handle it yourself when the payload is not safe to repeat:

```sh
hippo run --disk-path . -- ./deploy.sh
status=$?
if [ "$status" -eq 124 ]; then
  echo "inspect the safety receipt before requeueing this payload" >&2
fi
exit "$status"
```

### When a deferral names a heavy-work lease

In exclusive mode, the deferral names the holder:

```console
HIPPO deferred task: the heavy-work lease is held by pid 33413 (class transactional); it must exit before this work is admitted.
```

That is another repository's guarded work. Wait for it.

### Storage

```console
$ hippo run --disk-path /Volumes/Small -- echo should-not-run
hippo: [hippo.limit.storage-blocked] the disk floor stopped this work; free space before retrying
$ echo $?
124
```

Free storage on the path you passed to `--disk-path`. The floor is 256 MiB and it is immutable.

**Do not point `--disk-path` at a roomier volume to get past the gate.** The flag names the volume
your work will actually write to; moving it elsewhere hides the problem until the build fails
halfway through with a partial artifact.

## Handle `125`

HIPPO could not do its job. The reason says which part failed, and all but one of them mean no
child was started.

**`hippo.coordination.protocol-mismatch`** — the shared root contains a live incompatible
coordination epoch. Do not send this through a capacity retry loop and do not delete state to force
takeover. Upgrade clients that share the root, let the existing sessions drain, and retry once — see
[How to enable reservation coordination](./enable-reservation-coordination.md).

**`hippo.policy.replan-required`** — ask for less:

```console
hippo: [hippo.policy.replan-required] reservation requires replanning: requested vector exceeds safe host capacity
```

**`hippo.config.unreadable`** — the configuration cannot be read, or weakens a compiled floor:

```console
hippo: [hippo.config.unreadable] resource configuration: maximum memory weakens the immutable 256 MiB floor
```

Retrying any of these produces the same answer. The request or the configuration has to change.

**`hippo.supervision.failed`** — HIPPO failed at a step it does not classify further, possibly after
the child started. When the shared coordination lock stays held past the two-second activation
window, HIPPO stops the child it just launched and writes a `started-activation-failure` receipt.
Read `receipts/` before running the payload again: the work may have begun — see
[How to inspect evidence](./inspect-evidence-and-abandoned-groups.md).

## Handle `2`

The invocation itself is unusable: an unknown flag, a missing argument, a value HIPPO cannot accept.
A fault inside HIPPO, `hippo.internal.failure`, also returns `2`: it is a failure to complete the
command, not a failure to start or supervise the work, and it is worth reporting as a bug.

```console
hippo: [hippo.args.invalid] concurrency environment name "HIPPO_ROOT" is reserved
```

stdout stays empty on every failed invocation, so a caller parsing stdout never has to skip a usage
block to find its answer.

## Tell HIPPO's status from your command's

A guarded command can itself exit `124` or `125`, and HIPPO passes that value through unchanged.
The number alone will not say who chose it — but the diagnostic will, because HIPPO's own failures
always write a line beginning `hippo:` to stderr and a child's status never produces one.

```sh
hippo run --output json --disk-path . -- ./task.sh 2>errors
status=$?
if grep -q '^hippo: ' errors; then
  echo "HIPPO decided this; the reason is on the line above" >&2
else
  echo "./task.sh chose $status for its own reasons" >&2
fi
exit "$status"
```

Evidence says the same thing independently: a child that ran and failed leaves a `task-failed`
summary and no never-started receipt.

## Related

- [Exit codes](../reference/exit-codes.md)
- [Why HIPPO fails closed](../explanation/failing-closed.md)
