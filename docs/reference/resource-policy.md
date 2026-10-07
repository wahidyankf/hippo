# Resource policy

The thresholds and profiles HIPPO uses to classify host evidence and decide admission. These are compiled defaults.
Local configuration can never cross the immutable floors — one CPU, 256 MiB, and at most 20 owners — but within them a
profile override may move a compiled profile value either way, and under schema 3 the configuration file defines the
resource tiers.

## Profiles

Ordinary work resolves `balanced` → `constrained` → `minimal` from effective memory, available memory, disk, CPU, and
swap capability. The first profile that fits is used.

| Profile       | Automatic reservation owners |
| ------------- | ---------------------------- |
| `balanced`    | 4                            |
| `constrained` | 2                            |
| `minimal`     | 1                            |

A resolved profile appears in `HIPPO_PROFILE`, in `status` output, and in every lifetime summary alongside the
`fallbackChain` that produced it.

## Task classes

| Class           | Strict? | Shed under pressure?                                              |
| --------------- | ------- | ----------------------------------------------------------------- |
| `ephemeral`     | No      | Yes — selected first, newest owner first; 10 s warning grace      |
| `service`       | No      | Yes — only when no eligible ephemeral remains; 30 s warning grace |
| `transactional` | Yes     | Only as the last resort at the schema-3 emergency floor           |
| `release`       | Yes     | Not applicable — release commands only                            |

`transactional` and `release` are strict: they do not fall back to a safer profile. A misfit replans with exit `125`
naming `hippo.policy.replan-required` instead.

## Thresholds

Every command — `run`, `status`, `watch`, `monitor`, and `release check` — assesses host evidence against the resolved
profile's thresholds, which `profilePolicy` in `internal/policy/profiles.go` derives from the host itself. None is a
fixed byte count. Two capacities feed them:

- **Effective memory** — the cgroup or container limit where one applies, otherwise physical memory.
- **Disk capacity** — the total size of the filesystem holding `--disk-path`, or its free space when the total is
  unknown.

### Profile reserves

Each reserve is a percentage of its capacity, rounded up and then clamped between a minimum and a maximum
(`BuiltinCatalog` in `internal/policy/profiles.go`). The no-swap memory reserve replaces the memory reserve when the
sample's `swapState` is `unavailable`.

| Profile       | Memory reserve       | No-swap memory reserve | Disk reserve        | Concurrency cap | CPU ceiling |
| ------------- | -------------------- | ---------------------- | ------------------- | --------------- | ----------- |
| `balanced`    | 15%, 1–4 GiB         | 20%, 1–4 GiB           | 10%, 2–20 GiB       | none            | 85%         |
| `constrained` | 10%, 512 MiB – 2 GiB | 15%, 512 MiB – 2 GiB   | 5%, 1–8 GiB         | 2               | 92%         |
| `minimal`     | 5%, 128–512 MiB      | 10%, 256–768 MiB       | 2%, 256 MiB – 1 GiB | 1               | 98%         |

Concurrency is the available parallelism minus one, at least one, and never above the profile's cap.

### Derived thresholds

| Threshold                       | Value                                                                       |
| ------------------------------- | --------------------------------------------------------------------------- |
| Admission memory                | the memory reserve; less available memory is `memory-warning`               |
| Critical memory                 | half the memory reserve, at least 64 MiB; less is `memory-critical`         |
| Warning-window admission memory | 25% of effective memory, clamped to 4–8 GiB (macOS degraded admission)      |
| Disk warning                    | the disk reserve; less free space is `disk-warning`                         |
| Disk critical                   | the immutable 256 MiB floor; less free space is `disk-critical`             |
| Swap-out warning                | 0.4% of effective memory, clamped to 64–128 MiB, swapped out per window     |
| Swap-out critical               | 1.6% of effective memory, clamped to 256–512 MiB, swapped out per window    |
| Compressor warning              | payload at 37.5% of effective memory and growth of 3.125% per window        |
| Compressor critical             | payload at 50% of effective memory and growth of 6.25% per window           |
| CPU admission                   | utilization at or below the profile's CPU ceiling for 3 consecutive samples |

A swap or compressor threshold compares growth measured over the trend window. Swap pressure is not assessed when
`swapState` is `unavailable`.

Worked example: a 16 GiB host with swap, on `balanced`, reserves 15% of 16 GiB, 2.4 GiB. Below 2.4 GiB available it
reads `memory-warning`; below 1.2 GiB, `memory-critical`. A 32 GiB host reaches the 4 GiB maximum reserve, so its
warning starts at 4 GiB and its critical level at 2 GiB.

When no profile fits, ordinary work still resolves to the profile that ends its fallback chain, the one with no
`fallback`, when that profile's lineage reaches `minimal`: built-in `minimal`, or a configured profile whose `extends`
lineage reaches `minimal`. That profile's admission memory is lowered to its critical level and its disk warning to the
256 MiB floor. A `minimal`-lineage profile with a `fallback`, set or inherited, falls back to it instead. A chain that
ends at any other profile, such as a `constrained`-lineage profile with no `fallback`, has no usable fallback and fails
with `125` naming `hippo.policy.replan-required`. Strict classes replan instead.

### Fixed signals

Some evidence carries its own fixed level, independent of profile (`MemoryState` and `ResourceAssessment` in
`internal/policy/policy.go`):

- **macOS memory pressure** — level 2 is warning; level 4, or an unavailable compressor, is critical.
- **Linux PSI** — `some avg10` of 10 or more is warning; `some avg10` of 25 or more, or `full avg10` of 5 or more, is
  critical, reported as `memory-psi`.
- **OOM counters** — any increase in `oomEvents` or `oomKillEvents` between samples is critical, reported as
  `memory-oom`.

### Timing

| Setting                 | Value                                                                      |
| ----------------------- | -------------------------------------------------------------------------- |
| Sample interval         | 1 s                                                                        |
| Trend window            | 15 s                                                                       |
| Admission window        | 16 s                                                                       |
| Ephemeral warning grace | 10 s                                                                       |
| Service warning grace   | 30 s                                                                       |
| Termination grace       | 10 s                                                                       |
| Bounded FIFO lease wait | 5 min; a schema-3 tier's queue deadline; a schema-2 `--wait-for-admission` |

The 5-minute lease wait is why a deferred owner can sit at the FIFO head for a long time before returning `124`. It is
not a hang.

## Schema-3 resource tiers

| Tier       | Launch-time CPU | Launch-time memory | FIFO deadline |
| ---------- | --------------- | ------------------ | ------------- |
| `light`    | 1–2             | 1–2 GiB            | 30 minutes    |
| `standard` | 2–4             | 3–6 GiB            | 90 minutes    |
| `heavy`    | 4–8             | 8–16 GiB           | 4 hours       |

These are the compiled tiers, and the values the recommended schema-3 configuration sets. Under schema 3, each tier's
bounds and `queueDeadline` come from the configuration file, which may set any positive deadline and any bounds that fit
the pool — see [configuration](./configuration.md#adaptive-schema-3).

The minimum is the admission floor. When the FIFO head fits, HIPPO grants the largest vector up to the tier maximum that
is safe at that instant. The allocation is fixed for the payload lifetime, so a lighter period cannot silently enlarge
an existing job and a pressured period cannot resize it underneath the build tool.

The recommended workstation pool is 8 CPU and 16 GiB, with two base owners. A third owner is a burst slot, not
guaranteed capacity: it opens only after the configured healthy-evidence gate and closes to new work whenever live
pressure is not normal. All repositories may enqueue one plan at once; the pool deliberately admits only the safe subset
and keeps the rest visible in FIFO order.

## Reservation capacity

Reservation capacity in each dimension is:

- **CPU** — the host's available parallelism, minus one safety unit
- **Memory** — effective memory, minus the resolved profile's memory reserve

Optional schema-2, or required schema-3, `maxCpu` and `maxMemoryMiB` caps tighten either dimension further.

Both dimensions must fit _together_, using checked subtraction, so integer overflow cannot turn an exhausted vector into
an admission.

Worked example from a 12-core host with 32 GiB of memory on the `balanced` profile, captured while one owner was live.
An idle root reports zero capacity until an owner registers:

```console
$ hippo status --config reservation.json --json --disk-path . | ...
"coordination":{"schemaVersion":5,"mode":"reservation","capacity":{"cpu":11,"memoryBytes":30064771072},...
```

`11` is 12 available parallelism minus one safety unit. `30064771072` is 28 GiB — 32 GiB effective memory minus the 4
GiB balanced reserve. The automatic `balanced` share of that is one quarter, rounded up:

```console
$ hippo run --config reservation.json --disk-path . \
    -- sh -c 'echo "cpu=$HIPPO_CONCURRENCY mem=$HIPPO_RESERVED_MEMORY_BYTES"'
cpu=3 mem=7516192768
```

`7516192768` is exactly 7 GiB, one quarter of 28 GiB. CPU 3 is a quarter of 11 rounded up, so three automatic owners fit
in 11 CPU and a fourth waits.

An explicit `--reserve-cpu` / `--reserve-memory-mib` may be smaller than the automatic share but never below one CPU or
256 MiB.

Under schema 3, explicit values must remain inside the chosen tier. Prefer the tier defaults unless a known tool needs a
narrower fixed vector.

## Host pressure remains authoritative

Fitting the reservation vector is necessary but not sufficient. After a vector fits, host pressure thresholds still
govern, and an owner admitted legitimately can be shed:

- **Critical pressure** sheds at once.
- **Warning pressure** sheds once it outlasts the guarded child's class grace — 10 s for `ephemeral`, 30 s for `service`
  — counted from the first warning sample and reset by any normal one (`internal/application/run.go`). For a running
  `ephemeral` child whose profile may use [degraded admission](#degraded-admission-on-macos), a warning that would admit
  degraded work does not count toward it, however that child was admitted. Any other warning still counts — one that
  grows swap-outs or the compressor payload past their thresholds, leaves less than the warning-admission memory, or
  fails another check below — as do critical pressure and a disk below its warning reserve, and `service` children are
  not spared. The guard reports, for example, `HIPPO shedding ephemeral child after memory-warning.` and exits `124`
  naming `hippo.limit.pressure-shed`, or `hippo.limit.storage-blocked` when the warning is `disk-warning`.

Outside reservation coordination a `transactional` guard is never shed. Under reservation coordination either trigger
starts one victim selection, which picks ephemeral and then service work and reaches transactional work only at the
emergency described below.

The recommended schema-3 emergency floor is 6 GiB available memory. Ordinary shedding protects transactional work. At
the emergency floor, or equivalent critical non-storage pressure, transactional work becomes the last eligible victim
after ephemeral and service work. HIPPO writes a `started-safety-stop` receipt with reason `emergency-pressure`, records
the lifetime outcome `emergency-safety-stop`, and never auto-retries that payload.

## Degraded admission on macOS

**Ephemeral** work of the built-in `balanced` profile, or of a configured profile whose `extends` lineage reaches
`balanced`, may admit on Darwin after a full stable warning window when 25% of effective memory — clamped to 4–8 GiB —
remains available and the CPU, disk, OOM, swap-out, and compressor-growth checks are all safe. A profile that extends
`constrained` or `minimal` never uses this path; `status --json` reports `profile.degradedAdmission` for the resolved
profile.

Under schema-1 exclusive coordination this degraded path forces canonical concurrency and every consumer-selected
mapping to `1`; under reservation coordination the child keeps its reservation allocation. Either way it announces
itself:

```console
$ hippo run --disk-path . -- sh -c 'sleep 10'
HIPPO admitting ephemeral child under stable macOS warning pressure with concurrency 1.
```

Services, profiles outside `balanced`'s lineage (including a `constrained` or `minimal` fallback), Linux PSI,
transactional work, and releases cannot use this path.

## Supported evidence

| Platform | `capabilities` in every sample              |
| -------- | ------------------------------------------- |
| macOS    | `["compressor", "memory-pressure", "swap"]` |
| Linux    | `["cgroup-v2", "memory-psi"]`               |

The array is fixed per platform (`internal/adapters/host/collector_darwin.go` and
`internal/adapters/host/collector_linux.go`). It names the evidence the collector reads, not what a particular host
supplied: a Linux sample says `memory-psi` even when no pressure file was readable.

What a host could not supply shows in the readings instead. On Linux, `memoryPsiSomeAvg10` and `memoryPsiFullAvg10` are
omitted when neither the cgroup's `memory.pressure` nor `/proc/pressure/memory` parses, while `oomEvents` and
`oomKillEvents` are always present and read `0` when `memory.events` is unreadable. On macOS a reading the system did
not return, such as swap usage or CPU utilization, is `null`, and the Linux pressure fields are omitted. No unavailable
reading is guessed.

## Related

- [JSON schemas](./json-schemas.md)
- [The reservation model](../explanation/reservation-model.md)
- [Configuration](./configuration.md)
