# Resource policy

The thresholds and profiles HIPPO uses to classify host evidence and decide admission. These are
compiled defaults; local configuration may tighten them but never weaken them. The resource tiers
are the one exception: under schema 3 the configuration file defines them.

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

| Class           | Strict? | Shed under pressure?                                    |
| --------------- | ------- | ------------------------------------------------------- |
| `ephemeral`     | No      | Yes — selected first, newest owner first                |
| `service`       | No      | Yes — only when no eligible ephemeral remains           |
| `transactional` | Yes     | Only as the last resort at the schema-3 emergency floor |
| `release`       | Yes     | Not applicable — release commands only                  |

`transactional` and `release` are strict: they do not fall back to a safer profile. A misfit replans
with exit `125` naming `hippo.policy.replan-required` instead.

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
returning `124`. It is not a hang.

## Schema-3 resource tiers

| Tier       | Launch-time CPU | Launch-time memory | FIFO deadline |
| ---------- | --------------- | ------------------ | ------------- |
| `light`    | 1–2             | 1–2 GiB            | 30 minutes    |
| `standard` | 2–4             | 3–6 GiB            | 90 minutes    |
| `heavy`    | 4–8             | 8–16 GiB           | 4 hours       |

These are the compiled tiers, and the values the recommended schema-3 configuration sets. Under
schema 3, each tier's bounds and `queueDeadline` come from the configuration file, which may set any
positive deadline and any bounds that fit the pool — see [configuration](./configuration.md#adaptive-schema-3).

The minimum is the admission floor. When the FIFO head fits, HIPPO grants the largest vector up to
the tier maximum that is safe at that instant. The allocation is fixed for the payload lifetime, so
a lighter period cannot silently enlarge an existing job and a pressured period cannot resize it
underneath the build tool.

The recommended workstation pool is 8 CPU and 16 GiB, with two base owners. A third owner is a burst
slot, not guaranteed capacity: it opens only after the configured healthy-evidence gate and closes to
new work whenever live pressure is not normal. All repositories may enqueue one plan at once; the
pool deliberately admits only the safe subset and keeps the rest visible in FIFO order.

## Reservation capacity

Reservation capacity in each dimension is:

- **CPU** — the host's available parallelism, minus one safety unit
- **Memory** — effective memory, minus the resolved profile's memory reserve

Optional schema-2, or required schema-3, `maxCpu` and `maxMemoryMiB` caps tighten either dimension
further.

Both dimensions must fit _together_, using checked subtraction, so integer overflow cannot turn an
exhausted vector into an admission.

Worked example from a 12-core host with 32 GiB of memory on the `balanced` profile, captured while
one owner was live. An idle root reports zero capacity until an owner registers:

```console
$ hippo status --config reservation.json --json --disk-path . | ...
"coordination":{"schemaVersion":5,"mode":"reservation","capacity":{"cpu":11,"memoryBytes":30064771072},...
```

`11` is 12 available parallelism minus one safety unit. `30064771072` is 28 GiB — 32 GiB effective
memory minus the 4 GiB balanced reserve. The automatic `balanced` share of that is one quarter, rounded up:

```console
$ hippo run --config reservation.json --disk-path . -- sh -c 'echo "cpu=$HIPPO_CONCURRENCY mem=$HIPPO_RESERVED_MEMORY_BYTES"'
cpu=3 mem=7516192768
```

`7516192768` is exactly 7 GiB, one quarter of 28 GiB. CPU 3 is a quarter of 11 rounded up, so three
automatic owners fit in 11 CPU and a fourth waits.

An explicit `--reserve-cpu` / `--reserve-memory-mib` may be smaller than the automatic share but
never below one CPU or 256 MiB.

Under schema 3, explicit values must remain inside the chosen tier. Prefer the tier defaults unless a
known tool needs a narrower fixed vector.

## Host pressure remains authoritative

Fitting the reservation vector is necessary but not sufficient. After a vector fits, host pressure
thresholds still govern. A host under critical pressure can shed an owner that was admitted
legitimately.

The recommended schema-3 emergency floor is 6 GiB available memory. Ordinary shedding protects
transactional work. At the emergency floor, or equivalent critical non-storage pressure,
transactional work becomes the last eligible victim after ephemeral and service work. HIPPO writes
a `started-safety-stop` receipt with reason `emergency-pressure`, records the lifetime outcome
`emergency-safety-stop`, and never auto-retries that payload.

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
