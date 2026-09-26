# Exit codes and error codes

HIPPO answers in two layers, because one number cannot carry both meanings a caller needs.

The **exit status** is what a shell branches on. There are seven of them and the list is closed. The
**error code** is what a failure was: a namespaced `hippo.area.reason` that names the decision
precisely. Every failure carries both, and the pairing is fixed — a given error code always returns
the same status.

The split exists because the statuses have to stay few. A status is the only thing a `if [ $? -eq N ]`
can see, so a vocabulary with a dozen entries is a vocabulary nobody learns and every caller gets
wrong. The reasons have no such limit, so that is where the detail lives.

## Exit statuses

| Status  | Meaning                                                          | Retry?                                   |
| ------- | ---------------------------------------------------------------- | ---------------------------------------- |
| `0`     | The work ran and the answer is affirmative                       | n/a                                      |
| `1`     | The work ran and the answer is negative                          | No — the answer is empty                 |
| `2`     | The invocation could not be used, or HIPPO hit an internal fault | No — fix the command or report the fault |
| `124`   | A limit stopped the work                                         | Yes, once it lifts                       |
| `125`   | HIPPO failed before, while, or after starting the work           | No — read the reason                     |
| `126`   | The command exists and could not be executed                     | No — fix the permissions                 |
| `127`   | The command was not found                                        | No — fix the path                        |
| _other_ | A started child's own status, or `128+N` when a signal ended it  | Depends on the child or the signal       |

`124` and `125` are the statuses `timeout` returns for the same two situations, and `126` and `127`
are the ones every POSIX shell returns. A caller who has never read this page still reads them
correctly, which is the whole reason for choosing them.

### When a signal stops HIPPO

`SIGINT` or `SIGTERM` sent to HIPPO ends it with `128+N`, the status a shell reports for any process
a signal ended: `130` for `SIGINT`, `143` for `SIGTERM`. HIPPO catches the signal only to finish
cleanly, and writes no `hippo:` line and no JSON body, because an interruption is neither a result
nor HIPPO failing.

- `watch`, `monitor`, and `release monitor` run until they are stopped, so a signal is their usual
  end, and they end with `128+N` wherever it lands, including while a sample is being collected.
  `release monitor` still writes its summary first; ending at its own `--duration-ms` exits `0`.
- A `run` still waiting in the queue, or still sampling the host before its child starts, writes a
  `never-started` receipt with reason `admission-cancelled`, then exits `128+N`. One stopped while
  sampling also summarizes those samples under the outcome `admission-cancelled`.
- A `run` whose child already started stops that child, and the child's own status passes through
  as usual. A failure HIPPO hits while stopping, such as a receipt it cannot write, is still
  reported under its reason.

### Telling HIPPO's status from a child's

A started child's status passes through exactly as it arrived, and HIPPO writes nothing beside it.
So a bare `124` from `hippo run` may be HIPPO's own limit or a child that chose to exit `124`, and
the number alone will not say which.

The diagnostic says which. HIPPO's own failures always write a line beginning `hippo:` to stderr; a
child's status never produces one. A script that needs certainty can ask for
`--output json` and look for a body.

## Error codes

Every HIPPO failure names exactly one of these, on stderr as `hippo: [code] message`, and in the
`--output json` body as `error.code`. Nothing outside this list is ever returned.

| Error code                              | Status | Meaning                                                |
| --------------------------------------- | ------ | ------------------------------------------------------ |
| `hippo.args.invalid`                    | `2`    | The invocation could not be parsed or accepted         |
| `hippo.internal.failure`                | `2`    | A fault in HIPPO itself, including an unhandled panic  |
| `hippo.limit.capacity-deferred`         | `124`  | Admission deferred, or a bounded wait elapsed          |
| `hippo.limit.storage-blocked`           | `124`  | The disk floor stopped the work; free space first      |
| `hippo.limit.pressure-shed`             | `124`  | A started child was shed under host pressure           |
| `hippo.limit.release-envelope-exceeded` | `124`  | Release evidence left the release envelope             |
| `hippo.config.unreadable`               | `125`  | The resource configuration could not be read           |
| `hippo.config.unresolvable`             | `125`  | The configuration was read and is not usable           |
| `hippo.identity.invalid`                | `125`  | A run identity file is present and cannot be used      |
| `hippo.policy.replan-required`          | `125`  | No profile admits this request as asked for            |
| `hippo.coordination.protocol-mismatch`  | `125`  | Live peer state this client cannot safely join         |
| `hippo.host.unreadable`                 | `125`  | Host evidence could not be collected                   |
| `hippo.evidence.unwritable`             | `125`  | The evidence root refused a write HIPPO needs          |
| `hippo.evidence.unreadable`             | `125`  | Evidence HIPPO recorded earlier can no longer be read  |
| `hippo.supervision.failed`              | `125`  | HIPPO failed at a step it did not classify further     |
| `hippo.child.not-executable`            | `126`  | The command exists and cannot be executed              |
| `hippo.child.not-found`                 | `127`  | The command is not on `PATH` and not at the path given |

`hippo.host.unreadable` and `hippo.evidence.unwritable` are pre-launch reasons: HIPPO names them
only when nothing was started. The first is host evidence HIPPO cannot read, such as a denied
`/proc` or `sysctl` read or a `--disk-path` it cannot inspect, and `release check` returns it instead
of a deferral. The second is the evidence root refusing a write for lack of permission, a read-only
file system, or no space or quota: creating the state root, recording a sample or summary, or
writing a `never-started` receipt. A refused receipt outranks the signal that stopped a queued run,
so that run exits `125` rather than `128+N`. Once a child has started, either failure is HIPPO
losing supervision of work that began, and it names `hippo.supervision.failed`.

`error.retryable` in the body is `true` for `hippo.limit.capacity-deferred` and
`hippo.limit.pressure-shed`, and `false` for the rest. `hippo.limit.storage-blocked` is not
retryable although it is a limit: waiting does not free disk, and a caller that retries on it will
retry forever. `hippo.limit.release-envelope-exceeded` is not retryable either: it is a verdict on
evidence already collected, and the same summary is rejected again.

## The machine-readable body

`--output json` adds one JSON document to stderr beneath the diagnostic line:

```json
{
  "schemaVersion": 1,
  "command": "hippo run",
  "exitCode": 127,
  "error": {
    "code": "hippo.child.not-found",
    "message": "definitely-not-a-command: command not found",
    "retryable": false
  }
}
```

`schemaVersion`, `error.code` and `error.message` are always present. `command` names the command
that ran — `hippo history` for `hippo --output json history …` as for `hippo history … --output json`
— and is `hippo` itself only when no command was named or the one named does not exist.
`error.field` appears only when one named flag or field caused the failure. The body is never
coloured, whatever `--color` says, because an escape byte in it is a parse error rather than a
presentation choice.

Diagnostics go to stderr and results go to stdout, always. A failed invocation leaves stdout empty.

## Reading a status in a script

```sh
hippo run --class ephemeral --resource-tier light --disk-path . -- "$@"
case $? in
  0)   ;;                                    # it ran and it worked
  124) echo "shed against a limit; the reason says whether a retry can help" >&2; exit 1 ;;
  125) echo "hippo could not run this; see the diagnostic" >&2; exit 1 ;;
  126|127) echo "the command is wrong, not the host" >&2; exit 1 ;;
  *)   exit $? ;;                            # the child's own answer
esac
```

A retry on `124` belongs only to the retryable reasons above, and a pressure shed means the payload
ran, so recover its effects first. [Respond to exit codes](../how-to/respond-to-exit-codes.md) walks
each reason.

## What changed, and when

Before `v0.8.0` HIPPO returned `73`, `75`, `76` and `78` for its own refusals, and `1` for both an
empty result and a usage mistake. Those four numbers were in the range a child may return, carried
no reason, and no consumer branched on them. They are gone.

| Was  | Now   | Reason now carried                                             |
| ---- | ----- | -------------------------------------------------------------- |
| `73` | `124` | `hippo.limit.storage-blocked`                                  |
| `75` | `124` | `hippo.limit.capacity-deferred` or `hippo.limit.pressure-shed` |
| `76` | `125` | `hippo.coordination.protocol-mismatch`                         |
| `78` | `125` | `hippo.policy.replan-required` or a `hippo.config.*` reason    |
| `1`  | `2`   | `hippo.args.invalid`, when the invocation was the problem      |

`1` still means an empty result, which is the only thing it ever should have meant. The same rule
reached one more case in v0.8.2: activation contention that outlives its deadline after the child
started had kept `1`, and now returns `125` naming `hippo.supervision.failed`. Two cases moved to `2`
in the same release because the invocation was the problem: a `run` with no identity source had
returned `125` naming `hippo.policy.replan-required`, and a `history` filter value no run can carry
had returned `1` as an empty result.

v0.8.2 also added two reasons so that a status no longer names something that did not happen.
`release assess` over rejected evidence keeps `124` but names `hippo.limit.release-envelope-exceeded`,
not retryable, where it had named `hippo.limit.capacity-deferred` and invited a retry that could only
be rejected again; a summary it cannot read at all now returns `125` naming
`hippo.evidence.unreadable` with no verdict. A `run` whose `hippo.identity.json` is present and
invalid keeps `125` but names `hippo.identity.invalid` and the file, where it had named
`hippo.policy.replan-required`.

Also in v0.8.2, a signal to HIPPO itself stopped being reported as a result or as HIPPO's failure.
`watch`, `monitor`, and `release monitor` exited `0` when stopped between samples, and `watch` and
`monitor` exited `125` when stopped during one; a queued or sampling `run` exited `125` naming
`hippo.supervision.failed`. All of them now exit `128+N`.

v0.8.2 also made `hippo.host.unreadable` and `hippo.evidence.unwritable` reachable. Both were listed
here but never returned: an unreadable host and a refused evidence root before launch named
`hippo.supervision.failed`, and `release check` on an unreadable host exited `124` naming
`hippo.limit.capacity-deferred`. The status is still `125` in every case but that last one.
