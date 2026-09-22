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

| Status  | Meaning                                                         | Retry?                   |
| ------- | --------------------------------------------------------------- | ------------------------ |
| `0`     | The work ran and the answer is affirmative                      | n/a                      |
| `1`     | The work ran and the answer is negative                         | No — the answer is empty |
| `2`     | The invocation could not be used                                | No — fix the command     |
| `124`   | A limit stopped the work                                        | Yes, once it lifts       |
| `125`   | HIPPO could not do its job and started nothing                  | No — read the reason     |
| `126`   | The command exists and could not be executed                    | No — fix the permissions |
| `127`   | The command was not found                                       | No — fix the path        |
| _other_ | A started child's own status, or `128+N` when a signal ended it | Depends on the child     |

`124` and `125` are the statuses `timeout` returns for the same two situations, and `126` and `127`
are the ones every POSIX shell returns. A caller who has never read this page still reads them
correctly, which is the whole reason for choosing them.

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

| Error code                             | Status | Meaning                                                |
| -------------------------------------- | ------ | ------------------------------------------------------ |
| `hippo.args.invalid`                   | `2`    | The invocation could not be parsed or accepted         |
| `hippo.internal.failure`               | `2`    | A fault in HIPPO itself, including an unhandled panic  |
| `hippo.limit.capacity-deferred`        | `124`  | Admission deferred, or a bounded wait elapsed          |
| `hippo.limit.storage-blocked`          | `124`  | The disk floor stopped the work; free space first      |
| `hippo.limit.pressure-shed`            | `124`  | A started child was shed under host pressure           |
| `hippo.config.unreadable`              | `125`  | The resource configuration could not be read           |
| `hippo.config.unresolvable`            | `125`  | The configuration was read and is not usable           |
| `hippo.policy.replan-required`         | `125`  | No profile admits this request as asked for            |
| `hippo.coordination.protocol-mismatch` | `125`  | Live peer state this client cannot safely join         |
| `hippo.host.unreadable`                | `125`  | Host evidence could not be collected                   |
| `hippo.evidence.unwritable`            | `125`  | The evidence root refused a write HIPPO needs          |
| `hippo.supervision.failed`             | `125`  | HIPPO failed at a step it did not classify further     |
| `hippo.child.not-executable`           | `126`  | The command exists and cannot be executed              |
| `hippo.child.not-found`                | `127`  | The command is not on `PATH` and not at the path given |

`error.retryable` in the body is `true` for `hippo.limit.capacity-deferred` and
`hippo.limit.pressure-shed`, and `false` for the rest. `hippo.limit.storage-blocked` is not
retryable although it is a limit: waiting does not free disk, and a caller that retries on it will
retry forever.

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

`schemaVersion`, `error.code` and `error.message` are always present. `error.field` appears only
when one named flag or field caused the failure. The body is never coloured, whatever `--color`
says, because an escape byte in it is a parse error rather than a presentation choice.

Diagnostics go to stderr and results go to stdout, always. A failed invocation leaves stdout empty.

## Reading a status in a script

```sh
hippo run --class ephemeral --resource-tier light --disk-path . -- "$@"
case $? in
  0)   ;;                                    # it ran and it worked
  124) echo "shed against a limit; retrying later" ;;
  125) echo "hippo could not run this; see the diagnostic" >&2; exit 1 ;;
  126|127) echo "the command is wrong, not the host" >&2; exit 1 ;;
  *)   exit $? ;;                            # the child's own answer
esac
```

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

`1` still means an empty result, which is the only thing it ever should have meant.
