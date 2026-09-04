# Resource Guard Architecture

This document is the canonical as-built C4 model for Resource Guard. It describes the public system boundary, runtime containers, internal responsibilities, and material constraints without prescribing any consuming repository's architecture.

The diagrams use ASCII so they remain readable in terminals and plain-text tooling. Every relationship and constraint also appears in searchable prose.

## System Context

```text
+----------------------+       invokes        +----------------------+
| Person               | -------------------> | Software system      |
| Operator/contributor |                      | Resource Guard       |
+----------------------+                      |                      |
                                              | Admits, supervises,  |
+----------------------+       invokes        | and sheds local work |
| External system      | -------------------> +----------+-----------+
| Hooks/automation     |                                 |
+----------------------+                                 |
                                      +------------------+------------------+
                                      |                  |                  |
                                      | reads            | supervises       | probes
                                      v                  v                  v
                           +----------+-------+ +---------+--------+ +-------+----------+
                           | External system  | | External system  | | External system  |
                           | Host OS          | | Guarded workload | | Health endpoints |
                           +------------------+ +------------------+ +------------------+
```

An operator, repository script, Git hook, or CI task invokes Resource Guard before compute-bearing local work. Resource Guard reads normalized host evidence, resolves a safe capacity profile, coordinates eligible work through a shared per-user lease, and supervises only the child process group it starts. Callers may select environment variables that receive resolved concurrency and may connect standard streams without teaching Resource Guard about a build ecosystem. Release monitoring may probe explicit local and routed health endpoints supplied by the caller. Resource Guard is repository-independent and does not know consumer project layouts, commands, or infrastructure defaults.

## Container View

```text
                                      Resource Guard system
  +--------------------------------------------------------------------------------+
  |                                                                                |
  |  +------------------+    builds/executes    +-------------------------------+  |
  |  | Container        | --------------------> | Container                     |  |
  |  | Shell bootstrap  |                       | Go CLI process                |  |
  |  +--------+---------+                       | Cobra command boundary        |  |
  |           |                                 +---+-----------+-----------+---+  |
  |           | caches                              |           |           |      |
  |           v                                     | reads     | owns      | writes
  |  +--------+---------+                           v           |           v      |
  |  | Temporary store  |                   +-------+------+    |   +-------+-----+|
  |  | Build cache      |                   | Input file   |    |   | Data store  ||
  |  +------------------+                   | Local config |    |   | Lease and   ||
  |                                         +--------------+    |   | evidence    ||
  |                                                             |   +-------------+|
  +-------------------------------------------------------------|------------------+
                                                                |
                                      +-------------------------+------------------+
                                      |                         |                  |
                                      | collects                | starts/signals   | probes
                                      v                         v                  v
                           +----------+-------+       +---------+--------+ +-------+----------+
                           | External system  |       | External system  | | External system  |
                           | Host OS          |       | Child process    | | Health endpoints |
                           +------------------+       | group            | +------------------+
                                                      +------------------+
```

The POSIX shell bootstrap hashes the Go sources and module metadata, serializes compilation, retains a bounded platform cache, and then replaces itself with the compiled executable. Tagged-release consumers may invoke a verified binary directly and bypass this source-build container.

The Go CLI is the only long-running Resource Guard execution container. It reads an optional machine-local JSON configuration, collects host and process evidence through operating-system interfaces, and stores bounded lease and evidence records in a shared per-user state root. Resource Guard instances launched by different repositories coordinate through that same root. The configuration and state roots are runtime inputs; neither is committed to this repository. A guarded command runs as a distinct child process group so interruption and pressure shedding cannot target unrelated processes.

## Component View

```text
Container boundary: Go CLI process

[Process entry] --delegates--> [Command tree]

[Command tree] --loads-------> [Config loader] --resolves--> [Policy engine]
[Command tree] --collects----> [Host collector] --samples---> [Policy engine]
[Command tree] --maps streams-> [Execution guard] <--assesses-- [Policy engine]
[Command tree] --selects sink-> [Release guard] <---assesses-- [Policy engine]

[Execution guard] --writes--+
                             +--> [Evidence store]
[Release guard] ----writes---+
```

- **Process entry** maps the operating-system argument vector to the application's exit code.
- **Command tree** owns Cobra commands, flags, validation, stdin/stdout selection, and dependency injection.
- **Config loader and profiles** resolve configuration precedence, validate strict overrides, and preserve compiled safety floors.
- **Host collector** normalizes macOS, Linux, cgroup, swap, pressure, CPU, disk, and process evidence into portable samples.
- **Policy engine and profiles** classify evidence, choose an adaptive development profile, and preserve strict transaction and release envelopes.
- **Execution guard** owns shared sessions, port leases, child-process lifecycle and streams, generic concurrency mapping, pressure monitoring, shedding, and bounded evidence retention.
- **Release guard** owns consecutive release admission, health sampling, file or caller-owned stream sinks, summary schemas, and final overlap assessment.
- **Evidence store** owns live-writer admission, raw chunk rotation, fixed-memory quantiles, expiry, and inactive-file retention.

The `internal/policy` package owns the shared typed samples, collectors, task classes, decisions, thresholds, and profile resolution. Execution, platform collection, and release monitoring depend directly on that package instead of a forwarding facade or consumer-specific application types. Long-running operations receive caller-owned contexts; only the process entry translates operating-system signals into cancellation.

## Guarded Execution Dynamic View

```text
Caller     CLI/config     Host/policy     Lease store     Child group
  |             |              |               |               |
  | run request |              |               |               |
  |------------>| collect      |               |               |
  |             |------------->|               |               |
  |             | resolve      |               |               |
  |             |<-------------|               |               |
  |             | acquire heavy-work lease     |               |
  |             |----------------------------->|               |
  |             | start admitted child         |               |
  |             |--------------------------------------------->|
  |             | sample and assess repeatedly |               |
  |             |------------->|               |               |
  |             | stop only on owned pressure boundary         |
  |             |--------------------------------------------->|
  |             | wait until the owned child is reaped         |
  |             |<---------------------------------------------|
  |             | finalize evidence, then release lease        |
  |             |----------------------------->|               |
  | child code or stable guard exit             |               |
  |<------------|              |               |               |
```

Admission failures return before child creation. An admitted child inherits the resolved profile, canonical concurrency, caller-selected concurrency mappings, and the caller's standard streams. Resource Guard has no built-in knowledge of ecosystem-specific environment variables. Cancellation, collector failure, evidence failure, or resource pressure terminates and reaps the owned child group before evidence finalization and lease release. Resource Guard preserves a normal child exit code; its own stable exit codes distinguish storage cleanup (`73`), retryable capacity or lease pressure (`75`), and configuration or strict-profile replanning (`78`).

## Architectural Constraints

- The public CLI and defaults remain generic across consuming repositories; consumers supply commands, paths, ports, health endpoints, and local policy inputs.
- Supported runtime collectors normalize macOS and Linux evidence, including effective cgroup limits where available, without assuming one machine shape.
- Machine-local configuration may tighten policy but cannot weaken compiled safety floors.
- Heavy work coordinates through a shared per-user lease. Long-lived services retain independent inheritable sessions and do not monopolize the heavy-work lease.
- Resource Guard signals only the process group it creates. It never sheds unrelated user, repository, proxy, or production processes.
- Child stdin, stdout, and stderr remain caller-owned and distinct from guard diagnostics. Consumers opt into tool concurrency mappings by valid environment name; no build system is compiled into the core.
- One shared state root admits at most 20 live evidence streams across all consuming repositories. Each live stream keeps five rotating 400 KiB raw chunks, for about 2 MiB per session and about 40 MiB at the maximum live count.
- Completed raw chunks and summaries share a 50 MiB inactive cap. Raw chunks expire after seven days, summaries after thirty days, and active streams are protected from cleanup by process-owned markers.
- Lifetime summaries use fixed-memory aggregates and remain complete even after older raw chunks rotate away. Evidence excludes command arguments, repository origins, paths, credentials, and user payloads.
- Local configuration, runtime state, build caches, coverage, release artifacts, and scratch output remain untracked.
- Release monitoring requires explicit health inputs, emits generic bounded file evidence or caller-owned stdout streams, rejects mixed raw and summary schemas on one stream, and keeps compatibility with supported retained summary schemas.
- Tagged release assets are immutable, built through the owned release script for the supported OS and architecture matrix, and published with SHA-256 checksums.

## Behaviour Traceability

Executable behavior is specified in [`behaviours/`](behaviours/README.md). The unit adapter executes the complete recursive corpus without exemptions. Local integration and compiled-binary E2E adapters remain strict; every boundary exemption is exact and records both the blocked boundary and its reason. C4 views describe structure and responsibility; Gherkin remains authoritative for observable behavior.
