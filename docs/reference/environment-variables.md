# Environment variables

HIPPO reads a small set of variables from its caller and exports a small set to the guarded child.
Both sets are fixed: HIPPO compiles in no build-tool or product-specific names.

## Read by HIPPO

| Variable               | Purpose                                                                                                                        |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `HIPPO_ROOT`           | Shared coordination, lease, and evidence root. See [State root](./state-root.md).                                              |
| `HIPPO_CONFIG`         | Configuration path. Overridden by `--config`; overrides the bootstrap default.                                                 |
| `HIPPO_DEFAULT_CONFIG` | Bootstrap-only fallback set by the `./hippo` script to the repository-local `hippo.local.json`. Lowest precedence.             |
| `HIPPO_BUILD_CACHE`    | Overrides the `./hippo` bootstrap's compiled-binary cache directory.                                                           |
| `HIPPO_SESSION`        | Inherited session token. A child that inherits one reuses the existing fixed allocation and never creates or expands an owner. |
| `HIPPO_BIN`            | Path to the HIPPO executable, for nested invocations.                                                                          |
| `HIPPO_HEALTH_URL`     | Default for `release monitor --health-url`.                                                                                    |
| `HIPPO_ROUTED_ORIGIN`  | Default for `release monitor --routed-origin`.                                                                                 |

Configuration precedence, strongest first: `--config`, then `HIPPO_CONFIG`, then
`HIPPO_DEFAULT_CONFIG`.

Where an environment carries the same variable twice, the **last** occurrence wins. That matches what
the child observes, because Go's `os/exec` deduplicates its environment keeping the last entry. It
matters because `append(os.Environ(), "HIPPO_SESSION="+token)` — the ordinary way to override one
variable — produces exactly such a duplicate, and reading the first would let an ambient value from an
outer guard shadow the caller's explicit override.

## Exported to a guarded child

| Variable                      | Always?               | Value                                                                        |
| ----------------------------- | --------------------- | ---------------------------------------------------------------------------- |
| `HIPPO_PROFILE`               | Yes                   | The resolved profile: `balanced`, `constrained`, or `minimal`                |
| `HIPPO_CONCURRENCY`           | Yes                   | Canonical concurrency. In reservation mode this is exactly the allocated CPU |
| `HIPPO_RESERVED_MEMORY_BYTES` | Reservation mode only | Allocated memory in bytes                                                    |

```console
$ hippo run --disk-path . -- sh -c 'echo "profile=$HIPPO_PROFILE concurrency=$HIPPO_CONCURRENCY reserved=${HIPPO_RESERVED_MEMORY_BYTES:-<unset>}"'
profile=balanced concurrency=11 reserved=<unset>
```

That run used the default schema-1 exclusive mode, so no memory reservation was exported. With
reservation mode enabled:

```console
$ hippo run --config reservation.json --disk-path . -- sh -c 'echo "profile=$HIPPO_PROFILE cpu=$HIPPO_CONCURRENCY mem=$HIPPO_RESERVED_MEMORY_BYTES"'
profile=balanced cpu=3 mem=7516192768
```

## Caller-selected concurrency mappings

`--concurrency-env <NAME>` copies the resolved concurrency into a name your build tool already
understands, without HIPPO learning about that tool. The flag is repeatable.

```console
$ hippo run --disk-path . --concurrency-env BUILD_WORKERS --concurrency-env TEST_JOBS -- sh -c 'echo "BUILD_WORKERS=$BUILD_WORKERS TEST_JOBS=$TEST_JOBS HIPPO_CONCURRENCY=$HIPPO_CONCURRENCY"'
BUILD_WORKERS=11 TEST_JOBS=11 HIPPO_CONCURRENCY=11
```

### Name rules

A mapped name must be a POSIX environment identifier and must not be one of HIPPO's own protocol
variables. Violations are usage errors (exit `1`), rejected before anything runs.

### Value rules in reservation mode

The caller's existing value, if any, is taken as a ceiling request and reconciled against the
allocation:

| Caller's value               | Result                            |
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

$ BUILD_WORKERS=abc hippo run --config reservation.json --concurrency-env BUILD_WORKERS -- true
Error: concurrency environment "BUILD_WORKERS" must be a positive integer
$ echo $?
78
```

Schema-1 exclusive mode retains the v0.3.1 mapping behavior, including degraded admission at
concurrency one.

## Related

- [How to map concurrency into your build tool](../how-to/map-concurrency-into-your-build-tool.md)
- [Command-line interface](./cli.md)
- [Configuration](./configuration.md)
