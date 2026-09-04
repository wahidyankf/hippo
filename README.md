# Resource Guard

Resource Guard is a standalone Go CLI that admits, supervises, and sheds local development work from host resource evidence. It supports macOS and Linux, coordinates heavy work across repositories through a shared per-user lease, and only signals the child process group it created.

## Install

Download a tagged archive for `darwin` or `linux` on `amd64` or `arm64`, then verify it against the release `checksums.txt`. Consumers should pin both the tag and the expected SHA-256; they must not follow `main` at runtime.

Source users can run the tracked bootstrap, which builds and retains a bounded local cache:

```sh
./resource-guard version --json
./resource-guard status --json --disk-path .
./resource-guard run --class ephemeral --disk-path . -- <command>
```

## Command-line interface

Run `./resource-guard --help` to discover the `version`, `status`, `monitor`, `run`, and `release` commands. Release checks, summary assessment, and overlap monitoring are grouped under `release`. Guarded child commands must follow an explicit `--` boundary so their arguments are never interpreted as resource-guard flags.

Resource Guard always exports `RESOURCE_GUARD_PROFILE` and `RESOURCE_GUARD_CONCURRENCY` to an admitted child. A consumer can map the resolved concurrency into any tool-specific positive-integer variable without coupling Resource Guard to that tool:

```sh
./resource-guard run \
  --concurrency-env BUILD_WORKERS \
  --concurrency-env TEST_JOBS \
  -- make test
```

`--concurrency-env` is repeatable. Missing mapped variables receive resolved concurrency, existing caller values remain unchanged during ordinary admission, and degraded admission forces every selected mapping to `1`. Names must be POSIX environment identifiers and cannot be Resource Guard's protocol variables.

Cobra-powered completion scripts are generated on demand for Bash, Fish, PowerShell, and Zsh:

```sh
./resource-guard completion zsh
```

## What a running guard looks like

Resource Guard is not a full-screen TUI. A healthy `run` is deliberately quiet: the child keeps its normal stdin, stdout, and stderr, so existing scripts and CI logs still look familiar.

```console
$ ./resource-guard status --disk-path .
state=normal reason=normal profile=balanced concurrency=7 swap=idle availableGiB=12.00 diskFreeGiB=40.00 cpu=18.4%

$ ./resource-guard run --class ephemeral --disk-path . -- sh -c 'echo build-started; echo build-finished'
build-started
build-finished
$ echo $?
0

$ printf 'hello\n' | ./resource-guard run -- sh -c 'read value; printf "%s-world\n" "$value"' | tr a-z A-Z
HELLO-WORLD
```

Admission can wait silently while the configured sample window fills. The guard writes a message to stderr only when an operator needs to know about a degraded admission, deferral, storage block, or pressure shed. `monitor` prints the initial state and then only state/profile transitions:

```console
$ ./resource-guard monitor --interval 1s --disk-path .
2026-01-02T03:04:05Z state=normal reason=normal profile=balanced swap=idle
2026-01-02T03:05:10Z state=warning reason=memory-psi profile=constrained swap=idle
^C
```

Use `monitor --json` for machine consumers. It emits one schema-1 JSON object per transition, one object per line, with `measuredAt`, `state`, `reason`, `profile`, and `swapState`.

Cancellation is propagated through the caller-owned context. For an admitted command, the guard signals only its child process group, waits up to the termination grace, force-stops it if necessary, reaps it, finalizes evidence, and only then releases ownership.

For a long-running pane, the same plain-text transcript can be captured without machine-specific tooling in the repository:

```sh
tmux capture-pane -p -t resource-guard:0.0 -S -200
```

## Resource policy

Ordinary work resolves `balanced` → `constrained` → `minimal` from effective memory, available memory, disk, CPU, and swap capability. Balanced ephemeral work on Darwin may admit after a full stable warning window when 25% of effective memory, clamped to 4–8 GiB, remains available and CPU, disk, OOM, swap-out, and compressor-growth checks remain safe. This degraded path forces canonical concurrency and every consumer-selected mapping to one. Services, fallback profiles, Linux PSI, transactions, and releases cannot use it.

Exit `73` requires storage cleanup. Exit `75` is retryable capacity pressure or a held heavy-work lease. Exit `78` requires configuration or strict-profile replanning. Never bypass the guard or change task class to obtain admission.

## Configuration and state

Copy [`resource-guard.local.json.example`](resource-guard.local.json.example) to ignored `resource-guard.local.json`. `--config` overrides `RESOURCE_GUARD_CONFIG`, which overrides the bootstrap default. Local configuration can make policy stricter but cannot weaken compiled floors.

`RESOURCE_GUARD_ROOT` overrides the shared evidence and lease root. Defaults are `~/Library/Application Support/resource-guard` on macOS and `${XDG_STATE_HOME:-$HOME/.local/state}/resource-guard` on Linux. All repositories using the same root coordinate through the same leases and evidence budget. At most 20 evidence streams may be live at once; each stream retains five rotating 400 KiB raw chunks (about 2 MiB total) while its summary covers the complete session. Inactive evidence is capped at 50 MiB, raw samples expire after seven days, and summaries expire after thirty days. Evidence never records command arguments, origins, paths, credentials, or user data.

Runtime integration uses `RESOURCE_GUARD_ROOT`, `RESOURCE_GUARD_SESSION`, `RESOURCE_GUARD_BIN`, `RESOURCE_GUARD_PROFILE`, `RESOURCE_GUARD_CONCURRENCY`, `RESOURCE_GUARD_BUILD_CACHE`, `RESOURCE_GUARD_HEALTH_URL`, and `RESOURCE_GUARD_ROUTED_ORIGIN`.

## JSON and evidence formats

`version --json` is the smallest public document:

```json
{
  "schemaVersion": 1,
  "version": "v1.2.3",
  "commit": "0123456789abcdef0123456789abcdef01234567"
}
```

`status --json` emits the latest host sample at the top level, plus the current assessment and resolved profile. Byte values are integer bytes; timestamps are UTC RFC 3339 with optional fractional seconds; unavailable optional readings are `null` or omitted according to the field's compatibility contract.

```json
{
  "schemaVersion": 3,
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

A development lifetime summary uses schema 3. Its aggregate covers every sample even when older raw chunks have rotated away.

| Summary group       | Fields                                                                                                                                                                                                                                                                    |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Identity and result | `schemaVersion`, `sampleCount`, `taskClass`, `outcome`                                                                                                                                                                                                                    |
| Capacity aggregate  | `availableParallelism`, `availableNonCompressedEstimateMinBytes`, `memoryPressureLevelMax`, `compressorAvailableAll`, `compressorPayloadPeakBytes`, `cpuUtilizationP95Percent`, `diskFreeMinBytes`, `swapInsDelta`, `swapOutsDelta`, `swapFreeMinBytes`, `healthFailures` |
| Source platform     | `platform`, `capabilities`                                                                                                                                                                                                                                                |
| Resolved policy     | `requestedProfile`, `resolvedProfile`, `fallbackChain`, `concurrency`, `configHash`                                                                                                                                                                                       |

Runtime files are private implementation data under the shared state root:

| File                                    | Purpose                                                    |
| --------------------------------------- | ---------------------------------------------------------- |
| `<stream>.jsonl`                        | Newest raw samples for an active or completed stream       |
| `<stream>.1.jsonl` … `<stream>.4.jsonl` | Four progressively older raw chunks                        |
| `<stream>.summary.json`                 | Complete lifetime aggregate for the stream                 |
| `<stream>.active.json`                  | Schema-1 live-owner marker containing only the writer PID  |
| `.writers.lock`                         | Cross-process lock protecting writer admission and cleanup |

The active marker and lock are lifecycle internals, not supported evidence-reader APIs. Consumers should read the documented raw samples and summaries.

## Release monitoring

Strict release monitoring requires explicit local health and routed endpoints:

```sh
./resource-guard release monitor \
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
./resource-guard release monitor \
  --output - --summary summary.json \
  --deployment-root /path/to/deployment \
  --health-url http://127.0.0.1:8080/health/ready \
  --routed-origin https://service.example | jq -c .

# Retain rotating raw evidence; stream the final summary for assessment.
./resource-guard release monitor \
  --output samples.jsonl --summary - \
  --deployment-root /path/to/deployment \
  --health-url http://127.0.0.1:8080/health/ready \
  --routed-origin https://service.example |
  ./resource-guard release assess --summary -
```

File output remains exclusive, private, rotating, and retention-managed. Standard output is caller-owned: Resource Guard neither closes nor retains it, applies normal pipe backpressure, and returns a failure if the downstream writer fails. Diagnostics remain on stderr. Raw JSONL and the final summary cannot both target stdout because their schemas must never be mixed.

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

Release artifacts are built with `./scripts/build-release.sh <version> <commit> <output-dir>`. Generated outputs and local configuration are enforced by the artifact suite and `.gitignore`.

## License

Resource Guard is available under the [MIT License](LICENSE).
