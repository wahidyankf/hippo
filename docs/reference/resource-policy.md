# Resource policy

The thresholds and profiles HIPPO uses to classify host evidence and decide admission. These are
compiled defaults; local configuration may tighten them but never weaken them.

## Profiles

Ordinary work resolves `balanced` → `constrained` → `minimal` from effective memory, available
memory, disk, CPU, and swap capability. The first profile that fits is used.

| Profile       | Automatic reservation owners |
| ------------- | ---------------------------- |
| `balanced`    | 4                            |
| `constrained` | 2                            |
| `minimal`     | 1                            |

A resolved profile appears in `HIPPO_PROFILE`, in `status` output, and in every lifetime summary
alongside the `fallbackChain` that produced it.

## Task classes

| Class           | Strict? | Shed under critical pressure?                 |
| --------------- | ------- | --------------------------------------------- |
| `ephemeral`     | No      | Yes — selected first, newest owner first      |
| `service`       | No      | Yes — only when no eligible ephemeral remains |
| `transactional` | Yes     | **Never** after admission                     |
| `release`       | Yes     | Not applicable — release commands only        |

`transactional` and `release` are strict: they do not fall back to a safer profile. A misfit replans
with exit `78` instead.

## Default thresholds

| Threshold                       | Value   |
| ------------------------------- | ------- |
| Admission memory                | 9 GiB   |
| Warning-window admission memory | 8 GiB   |
| Critical memory                 | 4 GiB   |
| Disk warning                    | 30 GiB  |
| Disk critical                   | 20 GiB  |
| Immutable disk hard floor       | 256 MiB |
| Swap-out warning                | 128 MiB |
| Swap-out critical               | 512 MiB |
| Compressor warning payload      | 12 GiB  |
| Compressor warning growth       | 1 GiB   |
| Compressor critical payload     | 16 GiB  |
| Compressor critical growth      | 2 GiB   |
| Reserved CPU units              | 2       |
| Maximum CPU utilization         | 85%     |
| Consecutive CPU samples         | 3       |
| Trend window                    | 15 s    |
| Admission window                | 16 s    |
| Ephemeral warning grace         | 10 s    |
| Service warning grace           | 30 s    |
| Termination grace               | 10 s    |
| Bounded FIFO lease wait         | 5 min   |
| Sample interval                 | 1 s     |

The 5-minute lease wait is why a deferred owner can sit at the FIFO head for a long time before
returning `75`. It is not a hang.

## Reservation capacity

Reservation capacity in each dimension is:

- **CPU** — the host's available parallelism, minus one safety unit
- **Memory** — effective memory, minus the resolved profile's memory reserve

Optional schema-2 `maxCpu` and `maxMemoryMiB` caps tighten either dimension further.

Both dimensions must fit _together_, using checked subtraction, so integer overflow cannot turn an
exhausted vector into an admission.

Worked example from a 12-core host with 32 GiB of memory on the `balanced` profile:

```console
$ hippo status --config reservation.json --json --disk-path . | ...
"coordination":{"schemaVersion":4,"mode":"reservation","capacity":{"cpu":11,"memoryBytes":30064771072},...
```

`11` is 12 available parallelism minus one safety unit. `30064771072` is 28 GiB — 32 GiB effective
memory minus the 4 GiB balanced reserve. The automatic `balanced` share of that is one quarter:

```console
$ hippo run --config reservation.json --disk-path . -- sh -c 'echo "cpu=$HIPPO_CONCURRENCY mem=$HIPPO_RESERVED_MEMORY_BYTES"'
cpu=3 mem=7516192768
```

`7516192768` is exactly 7 GiB, one quarter of 28 GiB.

An explicit `--reserve-cpu` / `--reserve-memory-mib` may be smaller than the automatic share but
never below one CPU or 256 MiB.

## Host pressure remains authoritative

Fitting the reservation vector is necessary but not sufficient. After a vector fits, host pressure
thresholds still govern. A host under critical pressure can shed an owner that was admitted
legitimately.

## Degraded admission on macOS

Balanced **ephemeral** work on Darwin may admit after a full stable warning window when 25% of
effective memory — clamped to 4–8 GiB — remains available and the CPU, disk, OOM, swap-out, and
compressor-growth checks are all safe.

This degraded path forces canonical concurrency and every consumer-selected mapping to `1`, and
announces itself:

```console
$ hippo run --disk-path . -- sh -c 'sleep 10'
HIPPO admitting ephemeral child under stable macOS warning pressure with concurrency 1.
```

Services, fallback profiles, Linux PSI, transactional work, and releases cannot use this path.

## Supported evidence

| Platform | Capabilities reported                                         |
| -------- | ------------------------------------------------------------- |
| macOS    | `compressor`, `memory-pressure`, `swap`                       |
| Linux    | `cgroup-v2`, `memory-psi` where available, swap, OOM counters |

The `capabilities` array in every sample states what the host actually supplied. An unavailable
optional reading is `null` or omitted rather than guessed.

## Related

- [JSON schemas](./json-schemas.md)
- [The reservation model](../explanation/reservation-model.md)
- [Configuration](./configuration.md)
