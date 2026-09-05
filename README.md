# HIPPO

**HIPPO** — **H**ost **I**nfrastructure **P**ressure & **P**rocess **O**rchestrator — is a standalone Go CLI that admits, supervises, and sheds local development work from host resource evidence. It supports macOS and Linux, coordinates concurrent repositories through a shared CPU-and-memory reservation ledger, and lets only the guard that owns a child signal and reap that child process group.

## v0.4 reservation contract

Version `v0.4` adds schema-2 reservation configuration while retaining schema-1 exclusive behavior for a staged rollout. Reservation mode gives every service, ephemeral, and transactional owner one fixed CPU-and-memory allocation. Admission is atomic, overflow-safe, and strict FIFO; host pressure thresholds remain authoritative after a vector fits. Automatic `balanced`, `constrained`, and `minimal` reservations use four, two, and one fair-share owners respectively. An explicit reservation may be smaller than its automatic share but never below one CPU or 256 MiB. The effective owner limit is the strictest limit contributed by any live owner or FIFO waiter in the shared root.

## v0.3.0 identity cutover

Version `v0.3.0` is a hard rename from Resource Guard to HIPPO. The repository, Go module,
executable, release archives, environment protocol, local configuration, cache, and state namespace
all use `hippo` or `HIPPO_*`; the former names are not aliases. Commands, flags, exit codes, JSON
schemas, and supported evidence readers remain unchanged. Existing pre-v0.3.0 releases stay immutable,
and HIPPO does not delete their local cache, configuration, or evidence.

## Install

Download a tagged archive for `darwin` or `linux` on `amd64` or `arm64`, then verify it against the release `checksums.txt`. Consumers should pin both the tag and the expected SHA-256; they must not follow `main` at runtime.

Source users can run the tracked bootstrap, which builds and retains a bounded local cache:

```sh
./hippo version --json
./hippo status --json --disk-path .
./hippo run --class ephemeral --disk-path . -- <command>

# Override either automatic dimension when a task has a measured smaller footprint.
./hippo run --reserve-cpu 2 --reserve-memory-mib 1024 -- <command>
```

## Command-line interface

Run `./hippo --help` to discover the `version`, `status`, `monitor`, `run`, and `release` commands. Release checks, summary assessment, and overlap monitoring are grouped under `release`. Guarded child commands must follow an explicit `--` boundary so their arguments are never interpreted as hippo flags.

HIPPO always exports `HIPPO_PROFILE` and `HIPPO_CONCURRENCY` to an admitted child. Reservation mode fixes `HIPPO_CONCURRENCY` to allocated CPU and also exports `HIPPO_RESERVED_MEMORY_BYTES`. A consumer can map the fixed concurrency into any tool-specific positive-integer variable without coupling HIPPO to that tool:

```sh
./hippo run \
  --concurrency-env BUILD_WORKERS \
  --concurrency-env TEST_JOBS \
  -- make test
```

`--concurrency-env` is repeatable. In reservation mode, a missing mapping receives allocated CPU, a positive lower value survives, and a higher value is clamped to the allocation. Zero, negative, or malformed mapped values require replanning with exit `78` before child execution. Schema-1 exclusive mode retains the v0.3.1 mapping behavior, including degraded admission at concurrency one. Names must be POSIX environment identifiers and cannot be HIPPO's protocol variables.

Usage output belongs to usage mistakes. A missing `--` boundary, an unknown command, or an unusable flag prints the command usage next to its diagnostic, while a failure that happens after the arguments were accepted prints only the diagnostic, so consumer logs keep the real cause instead of a flag list.

Cobra-powered completion scripts are generated on demand for Bash, Fish, PowerShell, and Zsh:

```sh
./hippo completion zsh
```

## What a running guard looks like

HIPPO is not a full-screen TUI. A healthy `run` is deliberately quiet: the child keeps its normal stdin, stdout, and stderr, so existing scripts and CI logs still look familiar.

```console
$ ./hippo status --disk-path .
state=normal reason=normal profile=balanced concurrency=7 swap=idle availableGiB=12.00 diskFreeGiB=40.00 cpu=18.4%

$ ./hippo run --class ephemeral --disk-path . -- sh -c 'echo build-started; echo build-finished'
build-started
build-finished
$ echo $?
0

$ printf 'hello\n' | ./hippo run -- sh -c 'read value; printf "%s-world\n" "$value"' | tr a-z A-Z
HELLO-WORLD
```

Admission can wait silently while the configured sample window fills. The guard writes a message to stderr only when an operator needs to know about a degraded admission, deferral, storage block, or pressure shed. `monitor` prints the initial state and then only state/profile transitions:

```console
$ ./hippo monitor --interval 1s --disk-path .
2026-01-02T03:04:05Z state=normal reason=normal profile=balanced swap=idle
2026-01-02T03:05:10Z state=warning reason=memory-psi profile=constrained swap=idle
^C
```

Use `monitor --json` for machine consumers. It emits one schema-1 JSON object per transition, one object per line, with `measuredAt`, `state`, `reason`, `profile`, and `swapState`.

Cancellation is propagated through the caller-owned context. A private, capability-authenticated HIPPO launcher owns the admitted command group and its reservation and port identities. Normal or nonzero leader exit is not group retirement: the launcher waits for the complete group to disappear. On cancellation or owner-side shedding, it signals only that group, applies independently bounded TERM and KILL observation, and releases ownership only after positive group retirement. An unconfirmed retirement returns boundedly while leaving accounting fail closed.

When inherited stdin is the caller's controlling terminal, HIPPO makes the guarded child group foreground before it can read and restores the original foreground group on every return path. Pipes, regular files, and non-controlling TTYs are unchanged. This preserves interactive reads without weakening process-group signal cleanup.

For a long-running pane, the same plain-text transcript can be captured without machine-specific tooling in the repository:

```sh
tmux capture-pane -p -t hippo:0.0 -S -200
```

## Resource policy

Ordinary work resolves `balanced` → `constrained` → `minimal` from effective memory, available memory, disk, CPU, and swap capability. Balanced ephemeral work on Darwin may admit after a full stable warning window when 25% of effective memory, clamped to 4–8 GiB, remains available and CPU, disk, OOM, swap-out, and compressor-growth checks remain safe. This degraded path forces canonical concurrency and every consumer-selected mapping to one. Services, fallback profiles, Linux PSI, transactions, and releases cannot use it.

Reservation capacity is the host's available parallelism minus one safety unit and effective memory minus the resolved profile reserve, optionally tightened by schema-2 caps. Both dimensions must fit together using checked subtraction, so integer overflow cannot turn an exhausted vector into an admission. An impossible vector returns exit `78`; temporary exhaustion remains at the FIFO head through the bounded lease interval and then returns exit `75`. Under critical pressure, one locked evaluation marks the newest ephemeral owner, then the newest service only when no eligible ephemeral remains. Transactional owners are never shed after admission. A remote selector never signals another owner's process group: it waits boundedly for the owning guard to observe its mark, terminate and reap its own child, and release the reservation. A live unresponsive selected owner remains the global no-cascade barrier. The mark preserves storage exit `73` versus retryable pressure exit `75` for the owner that performs termination.

Exit `73` requires storage cleanup. Exit `75` is retryable capacity, lease, or coordination pressure. If compatibility session inventory cannot be enumerated, a heavy owner or service session cannot be decoded, or positively stale heavy state cannot be removed, HIPPO deliberately leaves the existing bytes in place and defers reservation takeover. Inspect the private shared state, confirm that no owner remains, correct its filesystem accessibility, and retry; HIPPO never creates a mixed reservation/exclusive epoch. A malformed reservation marker, lock, or ledger also fails closed: admission and `status --json` return an error instead of rewriting state or reporting zero coordination totals. Exit `78` requires configuration, impossible reservation, invalid mapping, or strict-profile replanning. Never bypass the guard or change task class to obtain admission.

Every repository on a host shares one coordination root, so its lock is routinely held by a peer for a bounded transaction. That contention is never a supervision failure. Before a child starts it returns retryable exit `75`; during supervision the contended observation is skipped and the healthy child keeps running; and `status --json` waits the contention out before reporting. Ownership cleanup that cannot take the lock leaves a reconcilable owner mark behind, reports a deferred-cleanup note rather than a failure, and the next repository to take the lock completes the release. Malformed state still fails closed.

## Configuration and state

Copy [`hippo.local.json.example`](hippo.local.json.example) to ignored `hippo.local.json`. `--config` overrides `HIPPO_CONFIG`, which overrides the bootstrap default. Schema 2 enables reservation coordination and may cap `maxCpu`, `maxMemoryMiB`, and `maxActiveOwners` or set automatic shares. It cannot raise `maxActiveOwners` above 20 or weaken the one-CPU and 256 MiB floors. Within one shared root, every live owner and queued waiter contributes its configured maximum; HIPPO uses the minimum until that participant exits or times out and resets the effective limit when the ledger becomes idle. A retained schema-1 file deliberately selects the v0.3.1 exclusive mode.

`HIPPO_ROOT` overrides the shared coordination, lease, and evidence root. Defaults are `~/Library/Application Support/hippo` on macOS and `${XDG_STATE_HOME:-$HOME/.local/state}/hippo` on Linux. All repositories using the same root coordinate through the same protocol and evidence budget. At most 20 evidence streams may be live at once; each stream retains five rotating 400 KiB raw chunks (about 2 MiB total) while its summary covers the complete session. Inactive evidence is capped at 50 MiB, raw samples expire after seven days, and summaries expire after thirty days. Evidence never records command arguments, origins, paths, credentials, or user data.

Reservation mode holds `coordination.lock` only while it reconciles liveness, changes the FIFO queue, admits a complete vector, records a process group, marks one pressure victim, observes an owner mark, or releases an owner. Lifecycle acquisitions have bounded deadlines and an inode-keyed in-process gate in addition to cross-process advisory locking. If final ledger release cannot acquire the lock, HIPPO retains the identity evidence and retries atomically rather than making the unreleased record look stale; cancelled waiters use a fresh bounded cleanup attempt and retain verifiable FIFO evidence if that attempt fails. Decoded ledgers are rejected unless tokens, classes, monotonic sequences, nonnegative vectors, checked totals, owner/waiter structure, owner limits, and shedding state are internally consistent. A missing ledger is initialized only after the marker and identity directory positively prove an empty epoch. Sequence exhaustion remains byte-preserving while any participant is live or unverifiable and resets only after the epoch is positively empty. Per-token advisory identities include device and inode metadata, with a hard-link anchor preserving the live object when its primary path is corrupted. The private HIPPO launcher, never the arbitrary payload, holds both reservation and port identities through full group retirement, so supervisor death, payload descriptor closure, or a background descendant cannot release capacity early. Unconfirmed embedded returns abandon only the caller's local descriptor copies. New schema-1 ownership uses the same launcher lifetime; legacy zero-metadata PID records remain conservative. Inherited `HIPPO_SESSION` children reuse the existing fixed allocation and never create or expand an owner.

The compatibility protocol remains available: services own independent inheritable sessions, while ephemeral and transactional work serialize on `heavy.lock`. A schema-1 `coordination-mode.json` marker advertises `exclusive` while any compatibility session is live and is removed after the final session exits. Reservation mode refuses to replace live or unverifiable exclusive state. Conversely, a compatibility client defers every class when `reservation` is active. Both paths preserve state, start no child, and return `75`, so consumers can move only after old sessions drain.

Runtime integration uses `HIPPO_ROOT`, `HIPPO_SESSION`, `HIPPO_BIN`, `HIPPO_PROFILE`, `HIPPO_CONCURRENCY`, `HIPPO_BUILD_CACHE`, `HIPPO_HEALTH_URL`, and `HIPPO_ROUTED_ORIGIN`.

## JSON and evidence formats

`version --json` is the smallest public document:

```json
{
  "schemaVersion": 1,
  "version": "v1.2.3",
  "commit": "0123456789abcdef0123456789abcdef01234567"
}
```

`status --json` emits schema 4 with the latest host sample at the top level, the current assessment and resolved profile, and privacy-safe `coordination` totals. Coordination corruption is an error, never a synthetic zero-total success. Byte values are integer bytes; timestamps are UTC RFC 3339 with optional fractional seconds; unavailable optional readings are `null` or omitted according to the field's compatibility contract.

```json
{
  "schemaVersion": 4,
  "measuredAt": "2026-01-02T03:04:05Z",
  "platform": "linux",
  "capabilities": ["cgroup-v2", "memory-psi"],
  "effectiveMemoryLimitBytes": 17179869184,
  "availableMemoryBytes": 12884901888,
  "availableNonCompressedEstimateBytes": null,
  "memoryPressureLevel": 1,
  "compressorAvailable": null,
  "compressorPayloadBytes": null,
  "physicalMemoryBytes": 34359738368,
  "availableParallelism": 8,
  "cpuUtilizationPercent": 18.4,
  "diskFreeBytes": 42949672960,
  "diskTotalBytes": 549755813888,
  "pageSizeBytes": 4096,
  "compressorStoredPages": null,
  "compressorOccupiedPages": null,
  "swapIns": 0,
  "swapOuts": 0,
  "swapTotalBytes": 4294967296,
  "swapUsedBytes": 0,
  "swapFreeBytes": 4294967296,
  "swapState": "idle",
  "memoryPsiSomeAvg10": 0,
  "memoryPsiFullAvg10": 0,
  "oomEvents": 0,
  "oomKillEvents": 0,
  "resource": {
    "compressorGrowthWindowBytes": 0,
    "swapOutWindowBytes": 0,
    "reason": "normal",
    "state": "normal",
    "storageBlocked": false,
    "swapState": "idle"
  },
  "profile": {
    "requestedProfile": "balanced",
    "resolvedProfile": "balanced",
    "fallbackChain": ["balanced"],
    "strict": false,
    "concurrency": 7,
    "memoryReserveBytes": 2576980378,
    "diskReserveBytes": 21474836480,
    "decision": "run",
    "exitCode": 0,
    "retryable": false
  },
  "coordination": {
    "schemaVersion": 4,
    "mode": "reservation",
    "capacity": { "cpu": 7, "memoryBytes": 14602888806 },
    "allocated": { "cpu": 2, "memoryBytes": 1073741824 },
    "waiting": { "cpu": 0, "memoryBytes": 0 },
    "activeOwners": 1,
    "waitingOwners": 0,
    "ephemeral": 1,
    "service": 0,
    "transactional": 0
  },
  "configHash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
}
```

The complete host-sample field inventory is grouped below. The same sample object is written as one JSON object per line in development raw evidence.

| Group          | Fields                                                                                                                                   |
| -------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| Identity       | `schemaVersion`, `measuredAt`, `platform`, `capabilities`                                                                                |
| Memory         | `effectiveMemoryLimitBytes`, `availableMemoryBytes`, `availableNonCompressedEstimateBytes`, `memoryPressureLevel`, `physicalMemoryBytes` |
| Compressor     | `compressorAvailable`, `compressorPayloadBytes`, `compressorStoredPages`, `compressorOccupiedPages`                                      |
| CPU and disk   | `availableParallelism`, `cpuUtilizationPercent`, `diskFreeBytes`, `diskTotalBytes`                                                       |
| Swap           | `pageSizeBytes`, `swapIns`, `swapOuts`, `swapTotalBytes`, `swapUsedBytes`, `swapFreeBytes`, `swapState`                                  |
| Linux pressure | `memoryPsiSomeAvg10`, `memoryPsiFullAvg10`, `oomEvents`, `oomKillEvents`                                                                 |

A development lifetime summary uses schema 4. Its aggregate covers every sample even when older raw chunks have rotated away. Schema-3 fields retain their meaning; reservation sessions add only aggregate request, allocation, wait, peak-owner, and outcome fields. `peakOwnerCount` is raised atomically by every admission event during the child's lifetime, so even an owner admitted and released between host-sampling ticks is included.

| Summary group       | Fields                                                                                                                                                                                                                                                                    |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Identity and result | `schemaVersion`, `sampleCount`, `taskClass`, `outcome`                                                                                                                                                                                                                    |
| Capacity aggregate  | `availableParallelism`, `availableNonCompressedEstimateMinBytes`, `memoryPressureLevelMax`, `compressorAvailableAll`, `compressorPayloadPeakBytes`, `cpuUtilizationP95Percent`, `diskFreeMinBytes`, `swapInsDelta`, `swapOutsDelta`, `swapFreeMinBytes`, `healthFailures` |
| Source platform     | `platform`, `capabilities`                                                                                                                                                                                                                                                |
| Resolved policy     | `requestedProfile`, `resolvedProfile`, `fallbackChain`, `concurrency`, `configHash`                                                                                                                                                                                       |
| Reservation         | `requestedCpu`, `requestedMemoryBytes`, `allocatedCpu`, `allocatedMemoryBytes`, `reservationWaitMilliseconds`, `peakOwnerCount`, `budgetOutcome`                                                                                                                          |

Runtime files are private implementation data under the shared state root:

| File                                    | Purpose                                                             |
| --------------------------------------- | ------------------------------------------------------------------- |
| `coordination.lock`                     | Short advisory lock protecting coordination and session mutations   |
| `coordination-mode.json`                | Schema-1 active mode marker: `exclusive` or `reservation`           |
| `heavy.lock/owner.json`                 | Exclusive heavy-work owner retained for the guarded child lifecycle |
| `sessions/<token>.json`                 | Private inheritable live-session record                             |
| `reservations.json`                     | Schema-2 vectors, FIFO waiters, owners, and numeric shedding cause  |
| `reservation-identities/<token>.lock`   | Advisory liveness proof resistant to stale or reused PIDs           |
| `reservation-identities/<token>.anchor` | Same-inode recovery anchor for a live reservation identity          |
| `.coordination-mode-*.tmp`              | Protected atomic-write staging for the active mode marker           |
| `.reservations-*.tmp`                   | Protected atomic-write staging for the reservation ledger           |
| `<stream>.jsonl`                        | Newest raw samples for an active or completed stream                |
| `<stream>.1.jsonl` … `<stream>.4.jsonl` | Four progressively older raw chunks                                 |
| `<stream>.summary.json`                 | Complete lifetime aggregate for the stream                          |
| `<stream>.active.json`                  | Schema-1 live-owner marker containing only the writer PID           |
| `.writers.lock`                         | Cross-process lock protecting writer admission and cleanup          |

Coordination, lease, active-writer, and lock files are lifecycle internals, not supported evidence-reader APIs. Consumers should read the documented raw samples and summaries.

## Four-consumer conformance

`go run ./cmd/hippo-conformance <manifest.json>` runs a generic, manifest-driven adoption check. Start from [`conformance.manifest.json.example`](conformance.manifest.json.example). The manifest supplies exactly four consumer names and checkout paths, the pinned HIPPO binary and SHA-256, a shared root, argv-safe bootstrap and gate commands, and any coordination checks. Absolute, relative, and symlink-resolved paths must identify four different checkouts and cannot overlap the shared root in either direction. The harness freezes each checkout object plus the created shared-root object, revalidates both around every command, and snapshots every checkout's HEAD and complete dirty-path set before running any command. Bootstrap commands remain sequential within one consumer while all four consumer lanes run concurrently; any bootstrap failure is aggregated deterministically and blocks later phases. Coordination checks form the next barrier, then consumer gates run concurrently. Each command receives its own read-and-execute-only copy of one safely opened verified binary; the exact command copy is checked after execution, while the hidden master and manifest source are checked before later work and final reconciliation. Caller `HIPPO_SESSION` and fixed-allocation outputs plus the caller repository's bootstrap-only `HIPPO_DEFAULT_CONFIG` are removed from the consumer base environment; an explicit operator `HIPPO_CONFIG` remains available, and each consumer's own outer guard may establish the only session inherited by its nested work. Every started group must retire completely after normal, nonzero, or cancelled leader exit before its phase can finish; cancellation prevents the next sequential command from starting, applies bounded TERM/KILL observation, and reconciliation uses a fresh deadline. Cleanup and integrity failures remain fatal even beside an otherwise skippable capacity exit and join, rather than mask, execution or reconciliation failures. HIPPO compiles no consumer name, path, command, port, or product default, and its own validation, start, cancellation, and cleanup errors do not expose machine paths.

A live coordination check may set `allowCapacitySkip` when exit `75` means the current host cannot safely reproduce overlap. The harness records that check as an explicit capacity skip and continues deterministic integrity gates; other exits and all consumer-gate failures remain failures.

## Release monitoring

Strict release monitoring requires explicit local health and routed endpoints:

```sh
./hippo release monitor \
  --output samples.jsonl \
  --summary summary.json \
  --deployment-root /path/to/deployment \
  --health-url http://127.0.0.1:8080/health/ready \
  --routed-origin https://service.example \
  --service-port 8080 --service-port 8081
```

Without `--duration-ms`, monitoring continues until cancellation; a positive value makes the caller context end the capture after that many milliseconds. The current raw chunk remains at the requested `--output` path and older chunks use numbered suffixes.

Either release destination may use the Unix `-` convention, but not both in one invocation:

```sh
# Stream raw JSONL; retain the final summary as a file.
./hippo release monitor \
  --output - --summary summary.json \
  --deployment-root /path/to/deployment \
  --health-url http://127.0.0.1:8080/health/ready \
  --routed-origin https://service.example | jq -c .

# Retain rotating raw evidence; stream the final summary for assessment.
./hippo release monitor \
  --output samples.jsonl --summary - \
  --deployment-root /path/to/deployment \
  --health-url http://127.0.0.1:8080/health/ready \
  --routed-origin https://service.example |
  ./hippo release assess --summary -
```

File output remains exclusive, private, rotating, and retention-managed. Standard output is caller-owned: HIPPO neither closes nor retains it, applies normal pipe backpressure, and returns a failure if the downstream writer fails. Diagnostics remain on stderr. Raw JSONL and the final summary cannot both target stdout because their schemas must never be mixed.

New summaries use schema 5 with generic health fields. Assessment remains compatible with retained schema 2–4 summaries.

Each release raw JSONL record embeds the complete host sample and adds `oneMinuteLoad`, `serviceRssBytes`, `healthStatus`, `healthLatencyMs`, `routedJourneyStatus`, and `routedJourneyLatencyMs`. The schema-5 summary contains `schemaVersion`, `platform`, `capabilities`, `sampleCount`, `availableParallelism`, `availableNonCompressedEstimateMinBytes`, `memoryPressureLevelMax`, `compressorAvailableAll`, `compressorPayloadPeakBytes`, `physicalMemoryBytes`, `cpuUtilizationP95Percent`, `serviceRssPeakBytes`, `diskFreeMinBytes`, `swapInsDelta`, `swapOutsDelta`, `swapFreeMinBytes`, `healthLatencyP95Ms`, `healthFailures`, `routedJourneyLatencyP95Ms`, `routedJourneyLatencyMaxMs`, and `routedJourneyFailures`.

`release assess --summary <path>` prints `{"accepted":true,"schemaVersion":5}` when that evidence remains inside the release envelope; `--summary -` reads the same document from stdin. Rejected evidence returns exit `75` with `accepted` set to `false`.

## Development

The canonical specification includes the current [C4 architecture](specs/architecture.md) and executable [`specs/behaviours`](specs/behaviours/README.md). The shared behavior contract enforces strict unit, integration, and compiled-binary E2E adapters with complete step resolution. Unit runs the entire corpus; any integration or E2E exemption is exact and documents its boundary and reason. Executable scenarios cover canonical and mapped concurrency, child stream separation, JSON transitions, stdin assessment, release streaming, stdout conflicts, and downstream failures.

Source contributors need Go 1.26.1 and Node.js 24. Install the locked contributor tooling once; npm's prepare lifecycle installs the repository hooks:

```sh
npm ci
npm run test:quick
npm test
```

Commits follow Conventional Commits. Pre-commit formats supported staged Go, shell, Markdown, JSON, and YAML files; pre-push runs the direct no-Nx quick gate. The quick gate enforces at least 99% statement coverage over deterministic production policy, configuration, host-parsing, and fixed-memory evidence aggregation logic. Platform, filesystem, and process boundaries remain covered by strict integration and compiled-binary E2E adapters.

Release artifacts are built with `./scripts/build-release.sh <version> <commit> <output-dir>`. The version must be exact `vX.Y.Z`; the commit must be the full lowercase current `HEAD`, and the checkout must be clean before output begins. All four binaries are compiled from an isolated exact-commit materialization, so ignored or excluded checkout files cannot affect them. Validation requires exactly the four supported archives plus their checksums, one regular mode-755 `hippo` member per archive, matching clean VCS metadata, and matching native version JSON; symbolic links, hard links, and special archive members are rejected before extraction. Disposable scratch belongs in `local-tmp/`; requested non-authoritative reports belong in `generated-reports/`. Both directories, generated build output, and local configuration are enforced by the artifact suite and `.gitignore`.

## License

HIPPO is available under the [MIT License](LICENSE).
