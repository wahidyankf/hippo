# Environment variables

HIPPO reads a small set of variables from its caller and exports a small set to the guarded child. Both sets are fixed:
HIPPO compiles in no build-tool or product-specific names.

## Read by HIPPO

- `HIPPO_ROOT` — Shared coordination, lease, and evidence root. See [State root](./state-root.md).
- `HIPPO_CONFIG` — Configuration path. Overridden by `--config`; overrides the bootstrap default.
- `HIPPO_DEFAULT_CONFIG` — Bootstrap-only fallback set by `./hippo` to the repository-local `hippo.local.json`. Skips
  the current-directory file.
- `HIPPO_IDENTITY` — Explicit schema-1 run identity path. Stronger than discovery and the bootstrap default.
- `HIPPO_DEFAULT_IDENTITY` — Bootstrap-only fallback identity path after upward `hippo.identity.json` discovery.
- `HIPPO_BUILD_CACHE` — Overrides the `./hippo` bootstrap's compiled-binary cache directory.
- `HIPPO_SESSION` — Inherited session token. A child that inherits one reuses the existing fixed allocation and never
  creates or expands an owner.
- `HIPPO_HEALTH_URL` — Default for `release monitor --health-url`.
- `HIPPO_ROUTED_ORIGIN` — Default for `release monitor --routed-origin`.
- `HIPPO_COLOR` — Set to `1` to let `--color auto` colour diagnostics and decorate the TUI. See [Colour](#colour).
- `NO_COLOR` — Present with any value, even empty: `--color auto` disables diagnostic colour and TUI decoration.
- `TERM` — Unset or `dumb`: `--color auto` never colours or decorates. `dumb` also refuses TUI entry.
- `XDG_STATE_HOME` — Linux only, when `HIPPO_ROOT` is unset: picks the default state root, and so the shared ledger
  HIPPO joins. See [State root](./state-root.md#location).
- `HOME` — When `HIPPO_ROOT` is unset: the base of the default state root. See [State root](./state-root.md#location).
- `TMPDIR` — Base of the port-lease root `hippo-port-leases` that `run --lease-port` uses; unset means `/tmp`. See
  [State root](./state-root.md#coordination).

### Test builds only

A build that carries the repository's test identity, `v0.0.0-test`, which the repository's own test harness stamps at
build time, also reads `HIPPO_TEST_LINUX_EVIDENCE_ROOT`. When it names an absolute directory, the Linux collector reads
`/proc` and `/sys/fs/cgroup` evidence beneath it instead of from the live host. Compiled end-to-end runs use it to
sample a fixed idle host rather than the runner's load. A release build carries its tag and a plain `go build` carries
`dev`, so neither reads it.

Configuration precedence, strongest first: `--config`, then `HIPPO_CONFIG`, then `HIPPO_DEFAULT_CONFIG`, then
`hippo.local.json` in the current directory. See [Configuration](./configuration.md#precedence).

Identity precedence is `HIPPO_IDENTITY`, then the nearest `hippo.identity.json` found by walking upward from `--cwd` or
the current directory, then `HIPPO_DEFAULT_IDENTITY`. Upward discovery lets a contained repository worktree use its own
tracked identity while sharing the same machine root and configuration. `--source` and repeatable `--tag` values
override the loaded identity for one run. A run that needs an identity — one given `--tag`, or any run under schema 3 —
and finds no source in any of these or in `--source` exits `2` naming `hippo.args.invalid`, and the diagnostic names
`--source`.

Where an environment carries the same variable twice, the **last** occurrence wins. That matches what the child
observes, because Go's `os/exec` deduplicates its environment keeping the last entry. It matters because
`append(os.Environ(), "HIPPO_SESSION="+token)` — the ordinary way to override one variable — produces exactly such a
duplicate, and reading the first would let an ambient value from an outer guard shadow the caller's explicit override.

### Colour

`--color auto`, the default, colours the diagnostic line only when all three hold: `NO_COLOR` is absent, `TERM` is set
and is not `dumb`, and `HIPPO_COLOR` is exactly `1`. HIPPO does not test whether stderr is a terminal, so colour under
`auto` is always something the caller opts into. `--color always` and `--color never` ignore all three variables. The
machine-readable body is never coloured.

The TUI uses that same policy for optional decoration, including colour and emphasis. Disabling decoration keeps
required cursor/alternate-screen controls and text markers such as `WAIT`, `STALE`, and `ERROR`. Explicit
`--color always` does not bypass interactive entry requirements: stdin and stdout must be terminals, and `TERM=dumb` is
refused. Global `--output json` selects only failure diagnostics after terminal restoration, never JSON frames. See the
[TUI command reference](./cli.md#hippo-tui).

```console
$ TERM=xterm HIPPO_COLOR=1 hippo --config missing.json status 2>&1 | cat -v
^[[31mhippo: [hippo.config.unreadable] resource configuration: open missing.json: no such file or directory^[[0m
```

## Exported to a guarded child

- `HIPPO_PROFILE`
  - Always?: Yes
  - Value: The resolved profile's name: a built-in `balanced`, `constrained`, or `minimal`, or a
    [configured profile](./configuration.md#profiles) under its own name
- `HIPPO_CONCURRENCY`
  - Always?: Yes
  - Value: Canonical concurrency. In reservation mode this is exactly the allocated CPU
- `HIPPO_RESERVED_MEMORY_BYTES`
  - Always?: Reservation mode only
  - Value: Allocated memory in bytes
- `HIPPO_SESSION`
  - Always?: Yes
  - Value: The run's session token, which a nested invocation inherits
- `HIPPO_BIN`
  - Always?: Yes
  - Value: Absolute path of the running HIPPO executable, for nested invocations

```console
$ hippo run --disk-path . \
    -- sh -c \
    'echo "profile=$HIPPO_PROFILE concurrency=$HIPPO_CONCURRENCY reserved=${HIPPO_RESERVED_MEMORY_BYTES:-<unset>}"'
profile=balanced concurrency=12 reserved=<unset>
```

That run used the default schema-1 exclusive mode, so no memory reservation was exported. With reservation mode enabled:

```console
$ hippo run --config reservation.json --disk-path . \
    -- sh -c 'echo "profile=$HIPPO_PROFILE cpu=$HIPPO_CONCURRENCY mem=$HIPPO_RESERVED_MEMORY_BYTES"'
profile=balanced cpu=3 mem=7516192768
```

## Caller-selected concurrency mappings

`--concurrency-env <NAME>` copies the resolved concurrency into a name your build tool already understands, without
HIPPO learning about that tool. The flag is repeatable.

```console
$ hippo run --disk-path . --concurrency-env BUILD_WORKERS --concurrency-env TEST_JOBS \
    -- sh -c 'echo "BUILD_WORKERS=$BUILD_WORKERS TEST_JOBS=$TEST_JOBS HIPPO_CONCURRENCY=$HIPPO_CONCURRENCY"'
BUILD_WORKERS=12 TEST_JOBS=12 HIPPO_CONCURRENCY=12
```

### Name rules

A mapped name must be a POSIX environment identifier and must not begin with `HIPPO_`: every `HIPPO_*` name is reserved
for HIPPO, whether or not HIPPO sets it today. Violations are usage errors (exit `2`, `hippo.args.invalid`), rejected
before anything runs.

### Value rules in reservation mode

The caller's existing value, if any, is taken as a ceiling request and reconciled against the allocation:

| Caller's value               | Result                                                              |
| ---------------------------- | ------------------------------------------------------------------- |
| unset or empty               | Receives the allocated CPU                                          |
| positive, below allocation   | Survives unchanged                                                  |
| positive, above allocation   | Clamped down to the allocation                                      |
| zero, negative, or malformed | Exit `125`, `hippo.policy.replan-required`, before the child starts |

```console
$ BUILD_WORKERS=1 hippo run --config reservation.json --reserve-cpu 4 --concurrency-env BUILD_WORKERS \
    -- sh -c 'echo "BUILD_WORKERS=$BUILD_WORKERS HIPPO_CONCURRENCY=$HIPPO_CONCURRENCY"'
BUILD_WORKERS=1 HIPPO_CONCURRENCY=4

$ BUILD_WORKERS=64 hippo run --config reservation.json --reserve-cpu 2 --concurrency-env BUILD_WORKERS \
    -- sh -c 'echo "BUILD_WORKERS=$BUILD_WORKERS HIPPO_CONCURRENCY=$HIPPO_CONCURRENCY"'
BUILD_WORKERS=2 HIPPO_CONCURRENCY=2

$ BUILD_WORKERS=abc hippo run --config reservation.json --concurrency-env BUILD_WORKERS -- true
hippo: [hippo.policy.replan-required] concurrency environment "BUILD_WORKERS" must be a positive integer
$ echo $?
125
```

Schema-1 exclusive mode retains the v0.3.1 mapping behavior, including degraded admission at concurrency one.

## Related

- [How to map concurrency into your build tool](../how-to/map-concurrency-into-your-build-tool.md)
- [Command-line interface](./cli.md)
- [Configuration](./configuration.md)
