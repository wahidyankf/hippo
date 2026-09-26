# Command-line interface

Complete command and flag inventory for the `hippo` executable. Generated from the binary's own
help output; run `hippo <command> --help` to confirm against the version you have installed.

## Command tree

```mermaid
graph LR
    hippo["hippo"]
    version["version"]
    status["status"]
    watch["watch"]
    history["history"]
    monitor["monitor"]
    run["run"]
    release["release"]
    completion["completion"]
    check["check"]
    assess["assess"]
    rmonitor["monitor"]

    hippo --> version
    hippo --> status
    hippo --> watch
    hippo --> history
    hippo --> monitor
    hippo --> run
    hippo --> release
    hippo --> completion
    release --> check
    release --> assess
    release --> rmonitor

    classDef root fill:#DE8F05,stroke:#000000,color:#000000
    classDef command fill:#0173B2,stroke:#000000,color:#FFFFFF

    class hippo root
    class version,status,watch,history,monitor,run,release,completion,check,assess,rmonitor command
```

| Command                 | Does                                             |
| ----------------------- | ------------------------------------------------ |
| `hippo version`         | Print build version information                  |
| `hippo status`          | Inspect current resource evidence                |
| `hippo watch`           | Watch resource, admission, and queue transitions |
| `hippo history`         | Query bounded shared run summaries               |
| `hippo monitor`         | Monitor resource-state transitions               |
| `hippo run`             | Run a command under resource supervision         |
| `hippo release`         | Check and monitor release resource safety        |
| `hippo release check`   | Check release admission and stability            |
| `hippo release assess`  | Assess a release evidence summary                |
| `hippo release monitor` | Capture release overlap evidence                 |
| `hippo completion`      | Generate a shell autocompletion script           |

## Global flags

Every command accepts these, placed before or after the command name.

| Flag              | Default | Meaning                                                                                                    |
| ----------------- | ------- | ---------------------------------------------------------------------------------------------------------- |
| `--color <when>`  | `auto`  | Colour the diagnostic line: `always`, `never`, or `auto` (see [Colour](./environment-variables.md#colour)) |
| `--output <form>` | `text`  | Diagnostic format: `text`, or `json` to add the machine-readable failure body on stderr                    |

Any other value is a usage mistake: exit `2`, naming `hippo.args.invalid`.
HIPPO reads these flags only before `--`, so a guarded command's own `--output` or `--color` after
`run`'s `--` belongs to that command and changes nothing HIPPO writes.

`release monitor` defines its own `--output` (the raw sample destination), which takes the place of
the global flag for that command wherever it is placed, so that command never writes a failure body.
The failure body is described in [Exit codes](./exit-codes.md#the-machine-readable-body); its
`command` field names the command that ran, wherever the global flags sit.

The root command also accepts `--version`, which prints the same text as `hippo version` and exits.

## Shared flags

`status`, `watch`, `monitor`, `run`, and every `release` subcommand accept these.

| Flag               | Default | Meaning                                                                              |
| ------------------ | ------- | ------------------------------------------------------------------------------------ |
| `--config <path>`  | unset   | Strict local JSON configuration. Overrides `HIPPO_CONFIG` and the bootstrap default. |
| `--profile <name>` | unset   | Requested resource profile. Resolution may still fall back to a safer profile.       |

`--help` is available on every command. `version`, `history`, and `completion` take no shared flags.

## `hippo version`

| Flag     | Default | Meaning                            |
| -------- | ------- | ---------------------------------- |
| `--json` | `false` | Emit the schema-1 version document |

```console
$ hippo version
v0.8.2 (<commit>)

$ hippo version --json
{"schemaVersion":1,"version":"v0.8.2","commit":"<commit>"}
```

The text form reports the release followed by its exact source commit. The JSON form carries the
same values in `version` and `commit`. A binary built from source outside the release script reports
`dev (unknown)`, because both values are injected by `scripts/build-release.sh` at link time.

## `hippo status`

Takes one host sample, resolves a profile, and reports it. Read-only: it never admits work.

| Flag                 | Default | Meaning                                            |
| -------------------- | ------- | -------------------------------------------------- |
| `--json`             | `false` | Emit the schema-5 status document                  |
| `--disk-path <path>` | `.`     | Path whose free space is measured                  |
| `--source <label>`   | unset   | Filter owner and waiter rows by source             |
| `--tag <key=value>`  | none    | Filter rows by a label; repeatable, all must match |

```console
$ hippo status --disk-path .
state=normal reason=normal profile=balanced concurrency=11 swap=active availableGiB=16.64 diskFreeGiB=63.05 cpu=27.4% owners=2 waiters=1 ownerLimit=2 promotion=insufficient-overlap-runs
active run=a161efd79fbf2947e02cc76b1f84f865 position=0 source=hippo class=ephemeral tier=standard cpu=4 memoryMiB=6144 deadline=2026-09-26T08:50:52.086019Z
active run=e58306df9d9abcc729f132fc1a19b081 position=0 source=hippo class=ephemeral tier=light cpu=2 memoryMiB=2048 deadline=2026-09-26T07:50:54.096318Z
waiting run=fb5b47c58a0287aee14ca7c310499cd0 position=1 source=my-repo class=ephemeral tier=heavy cpu=4 memoryMiB=8192 deadline=2026-09-26T11:20:56.116166Z
```

The text form prints one privacy-safe row line under the summary for each owner and waiter. The JSON
form carries the same rows in full, plus the base, maximum, and currently effective owner limit. A
live exclusive compatibility session appears as a legacy owner, while exclusive waiters remain
unregistered. Filters change rows, not the global aggregate totals. It is documented in
[JSON schemas](./json-schemas.md#status---json).

## `hippo watch`

Runs `status` repeatedly and prints only changed snapshots. This is the operator view for owners,
FIFO position, admission deadline, and promotion state; `monitor` remains the lower-level resource
transition stream.

| Flag                    | Default | Meaning                                            |
| ----------------------- | ------- | -------------------------------------------------- |
| `--interval <duration>` | `5s`    | Minimum delay between status snapshots             |
| `--json`                | `false` | Emit one schema-5 object per changed snapshot      |
| `--disk-path <path>`    | `.`     | Path whose free space is measured                  |
| `--source <label>`      | unset   | Filter owner and waiter rows by source             |
| `--tag <key=value>`     | none    | Filter rows by a label; repeatable, all must match |

## `hippo history`

Reads current and daily compacted summaries from the shared root. It never emits commands,
arguments, working directories, or repository paths.

| Flag                     | Default | Meaning                                        |
| ------------------------ | ------- | ---------------------------------------------- |
| `--since <duration>`     | `30d`   | Positive rolling window, at most 30 days       |
| `--source <label>`       | unset   | Filter by source                               |
| `--tag <key=value>`      | none    | Filter by label; repeatable, all must match    |
| `--class <name>`         | unset   | Filter by task class                           |
| `--resource-tier <name>` | unset   | Filter by resource tier                        |
| `--outcome <name>`       | unset   | Filter by outcome                              |
| `--json`                 | `false` | Emit one schema-1 document with a `rows` array |
| `--jsonl`                | `false` | Emit one summary object per line               |

`--json` and `--jsonl` are mutually exclusive.

A filter value no recorded run can carry is a usage mistake (`2`, `hippo.args.invalid`), not an
empty result: `--class` takes `ephemeral`, `service`, `transactional`, or `release`;
`--resource-tier` takes `light`, `standard`, or `heavy`; and `--outcome` takes `passed`,
`task-failed`, `supervision-failed`, `pressure-shed`, `storage-shed`, `emergency-safety-stop`,
`capacity-deferred`, or `storage-blocked`. A valid filter that matches nothing exits `1`.

## `hippo monitor`

Prints the initial state, then one line per state or profile transition. Runs until cancelled.

| Flag                    | Default | Meaning                                      |
| ----------------------- | ------- | -------------------------------------------- |
| `--json`                | `false` | Emit one schema-1 JSON object per transition |
| `--disk-path <path>`    | `.`     | Path whose free space is measured            |
| `--interval <duration>` | `1s`    | Sample interval                              |

```console
$ hippo monitor --interval 1s --disk-path .
2026-09-07T04:55:25.508138Z state=normal reason=normal profile=balanced swap=active
^C

$ hippo monitor --interval 1s --json --disk-path .
{"schemaVersion":1,"measuredAt":"2026-09-07T04:55:28.569948Z","state":"normal","reason":"normal","profile":"balanced","swapState":"active"}
^C
```

## `hippo run`

Admits, supervises, and sheds one guarded command. The `--` boundary is mandatory so the guarded
command's own arguments are never parsed as hippo flags.

```text
hippo run [flags] -- <command> [arguments...]
```

| Flag                              | Default     | Meaning                                                                                                                                |
| --------------------------------- | ----------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| `--class <name>`                  | `ephemeral` | Task class: `ephemeral`, `service`, or `transactional`                                                                                 |
| `--cwd <path>`                    | unset       | Child working directory                                                                                                                |
| `--disk-path <path>`              | unset       | Path whose free space is measured                                                                                                      |
| `--reserve-cpu <n>`               | `0`         | Fixed CPU reservation; `0` selects the automatic fair share                                                                            |
| `--reserve-memory-mib <n>`        | `0`         | Fixed memory reservation in MiB; `0` selects the automatic fair share                                                                  |
| `--resource-tier <name>`          | unset       | `light`, `standard`, or `heavy`; required by schema 3                                                                                  |
| `--source <label>`                | identity    | Override the discovered `hippo.identity.json` source                                                                                   |
| `--tag <key=value>`               | identity    | Override/add a privacy-safe label; repeatable, last duplicate wins                                                                     |
| `--concurrency-env <NAME>`        | none        | Child variable that receives resolved concurrency; repeatable                                                                          |
| `--wait-for-admission <duration>` | `0`         | Schema-2 FIFO deadline without a tier; never negative; refused under schema 1, beside a tier under schema 2, and always under schema 3 |
| `--lease-port <n>`                | `0`         | Service port to lease; 1–65535, within `--lease-min`..`--lease-max`                                                                    |
| `--lease-owner <name>`            | unset       | Service port owner; lowercase letters, digits, and `-`; required with `--lease-port`, refused without it                               |
| `--lease-min <n>`                 | `0`         | Minimum allowed leased port; refused without `--lease-port`                                                                            |
| `--lease-max <n>`                 | `0`         | Maximum allowed leased port; refused without `--lease-port`                                                                            |

The child keeps the caller's stdin, stdout, and stderr. Guard diagnostics go to stderr only. A normal
child exit code is passed through unchanged. Capacity waiting creates one FIFO identity and does not
launch the payload until admitted. A heartbeat reports that run ID, queue position, and remaining
deadline every 30 seconds. Expiry returns `124` naming `hippo.limit.capacity-deferred`, with a `never-started` safety receipt; HIPPO never
retries a payload.

```console
$ hippo run --class ephemeral --resource-tier light --disk-path . -- sh -c 'echo build-started; echo build-finished'
build-started
build-finished

$ hippo run --resource-tier light --disk-path . -- sh -c 'exit 42'
$ echo $?
42
```

`--reserve-cpu` and `--reserve-memory-mib` apply in reservation mode. An explicit reservation may be
smaller than the automatic share but never below one CPU or 256 MiB; see
[Configuration](./configuration.md) to enable reservation mode.

Schema 3 grants the largest launch-time vector that fits between the selected tier's minimum and
maximum. It requires a valid identity from `HIPPO_IDENTITY`, upward `hippo.identity.json` discovery,
or `HIPPO_DEFAULT_IDENTITY`. An invocation override is useful for worktree context:

```sh
hippo run --resource-tier standard --tag checkout=worktree --tag plan=maximize-hippo -- make test
```

## `hippo release check`

Silent gate. Exits `0` when a release may proceed, and reports a stable exit code otherwise.

| Flag                 | Default | Meaning         |
| -------------------- | ------- | --------------- |
| `--disk-path <path>` | `.`     | Deployment path |

## `hippo release monitor`

Captures release overlap evidence. `--output`, `--summary`, `--deployment-root`, `--health-url`, and
`--routed-origin` are all required; without any of them the command exits `2` before it samples.

| Flag                       | Default               | Meaning                                                        |
| -------------------------- | --------------------- | -------------------------------------------------------------- |
| `--output <path>`          | unset                 | Raw JSONL sample destination; `-` streams to stdout (required) |
| `--summary <path>`         | unset                 | Final summary destination; `-` streams to stdout (required)    |
| `--deployment-root <path>` | unset                 | Deployment root (required)                                     |
| `--health-url <url>`       | `HIPPO_HEALTH_URL`    | Local health URL (required)                                    |
| `--routed-origin <origin>` | `HIPPO_ROUTED_ORIGIN` | Bare HTTPS routed origin (required)                            |
| `--service-port <n>`       | none                  | Service port included in RSS accounting; repeatable            |
| `--duration-ms <n>`        | `0`                   | Stop after this many milliseconds; `0` runs until cancelled    |

`--output -` and `--summary -` cannot both be used in one invocation, because raw and summary
schemas must never be mixed on one stream:

```console
$ hippo release monitor --output - --summary - ...
hippo: [hippo.args.invalid] raw evidence and summary cannot both use standard output
$ echo $?
2
```

## `hippo release assess`

Reads a release summary and decides whether the evidence stays inside the release envelope.

| Flag               | Default | Meaning                            |
| ------------------ | ------- | ---------------------------------- |
| `--summary <path>` | unset   | Summary JSON path; `-` reads stdin |

```console
$ hippo release assess --summary summary.json
{"accepted":true,"schemaVersion":5}
```

Rejected evidence prints `"accepted":false` and returns exit `124`, naming
`hippo.limit.capacity-deferred`:

```console
$ hippo release assess --summary summary.json
{"accepted":false,"schemaVersion":5}
release overlap exhausted resource or routed responsiveness headroom
hippo: [hippo.limit.capacity-deferred] capacity deferred this work; retry when the host is quieter
$ echo $?
124
```

Assessment accepts retained schema 2–5 summaries. New summaries are schema 5.

## `hippo completion`

Cobra-generated scripts for `bash`, `fish`, `powershell`, and `zsh`.

```console
$ hippo completion zsh > "${fpath[1]}/_hippo"
```

## Usage errors

A usage mistake returns exit `2`, naming `hippo.args.invalid`. A mistyped invocation — an unknown
flag or command, a positional argument a command does not take, a missing `--`, or a command group
such as `release` or `completion` given no subcommand or an unknown one — also prints the usage of
the command it named on stderr next to its diagnostic, and nothing on stdout. A value or
combination the command rejects after parsing, and any failure after the arguments were accepted,
print only the diagnostic, so consumer logs keep the real cause instead of a flag list.

```console
$ hippo run --disk-path . echo hi
hippo: [hippo.args.invalid] run requires -- followed by a command

Usage:
  hippo run -- <command> [arguments...] [flags]
...
$ echo $?
2
```

Every command refuses a `--color` other than `always`, `never`, or `auto`, and an `--output` other
than `text` or `json`, the same way. `run` checks its flag values before it reads configuration,
host evidence, or coordination state:
a `--class` other than `ephemeral`, `service`, or `transactional`; a `--resource-tier` other than
`light`, `standard`, or `heavy`; a malformed `--tag` or `--source`; a negative
`--wait-for-admission`; a `--lease-port` outside 1–65535, outside `--lease-min`..`--lease-max`, or
without a valid lowercase `--lease-owner`; and a `--lease-owner`, `--lease-min`, or `--lease-max`
without `--lease-port` are each usage errors (`2`, `hippo.args.invalid`, diagnostic only), as is a
non-positive `monitor --interval`. Once the configuration is read, `run` also refuses
`--wait-for-admission` under schema 1 the same way.
Invalid `--concurrency-env` _names_ are usage errors (`2`, `hippo.args.invalid`,
diagnostic only). Invalid mapped
_values_ inherited from the caller's environment are not: they return `125`,
`hippo.policy.replan-required`. The rules live in
[Environment variables](./environment-variables.md#name-rules); see also [Exit codes](./exit-codes.md).

## Related

- [Exit codes](./exit-codes.md)
- [Environment variables](./environment-variables.md)
- [Configuration](./configuration.md)
