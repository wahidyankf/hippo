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

An operator, repository script, Git hook, or CI task invokes Resource Guard before compute-bearing local work. Resource Guard reads normalized host evidence, resolves a safe capacity profile, coordinates eligible work through a shared per-user lease, and supervises only the child process group it starts. Release monitoring may probe explicit local and routed health endpoints supplied by the caller. Resource Guard is repository-independent and does not know consumer project layouts, commands, or infrastructure defaults.

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

The Go CLI is the only long-running Resource Guard execution container. It reads an optional machine-local JSON configuration, collects host and process evidence through operating-system interfaces, and stores bounded lease and evidence records in a shared per-user state root. The configuration and state roots are runtime inputs; neither is committed to this repository. A guarded command runs as a distinct child process group so interruption and pressure shedding cannot target unrelated processes.

## Component View

```text
                              Container: Go CLI process
  +--------------------------------------------------------------------------------+
  |                                                                                |
  |  +----------------+      delegates      +------------------+                   |
  |  | Component      | ------------------> | Component        |                   |
  |  | Process entry  |                     | Command tree     |                   |
  |  +----------------+                     +----+------+------+------+             |
  |                                             |      |      |      |             |
  |                  +--------------------------+      |      |      |             |
  |                  | loads                    |      |      |      |             |
  |                  v                          |      |      |      |             |
  |       +----------+-------+          collects|      |      |      |             |
  |       | Component        |                  v      |      |      |             |
  |       | Config loader    |       +----------+---+  |      |      |             |
  |       | and profiles     |       | Component    |  |      |      |             |
  |       +------------------+       | Host         |  |      |      |             |
  |                                  | collector    |  |      |      |             |
  |                                  +--------------+  |      |      |             |
  |                                                    |      |      |             |
  |                     resolves +---------------------+      |      |             |
  |                              v                            |      |             |
  |                   +----------+-------+          supervises|      |             |
  |                   | Component        |                    v      |             |
  |                   | Policy engine    |       +------------+---+  |             |
  |                   | and profiles     |       | Component      |  |             |
  |                   +------------------+       | Execution      |  |             |
  |                                              | guard          |  |             |
  |                                              +----------------+  |             |
  |                                                                  |             |
  |                               release operations +---------------+             |
  |                                                  v                             |
  |                                       +----------+-------+                      |
  |                                       | Component        |                      |
  |                                       | Release guard    |                      |
  |                                       +------------------+                      |
  |                                                                                |
  +--------------------------------------------------------------------------------+
```

- **Process entry** maps the operating-system argument vector to the application's exit code.
- **Command tree** owns Cobra commands, flags, validation, output selection, and dependency injection.
- **Config loader and profiles** resolve configuration precedence, validate strict overrides, and preserve compiled safety floors.
- **Host collector** normalizes macOS, Linux, cgroup, swap, pressure, CPU, disk, and process evidence into portable samples.
- **Policy engine and profiles** classify evidence, choose an adaptive development profile, and preserve strict transaction and release envelopes.
- **Execution guard** owns shared sessions, port leases, child-process lifecycle, pressure monitoring, shedding, and bounded evidence retention.
- **Release guard** owns consecutive release admission, health sampling, summary schemas, and final overlap assessment.

The `internal/guard` package exposes the shared policy contracts used at process boundaries while their deterministic implementation remains in `internal/policy`. Platform collectors and release monitoring depend on those contracts rather than consumer-specific application types.

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
  |             | release and retain bounded evidence          |
  |             |----------------------------->|               |
  | child code or stable guard exit             |               |
  |<------------|              |               |               |
```

Admission failures return before child creation. An admitted child inherits the resolved profile and concurrency environment. Resource Guard preserves a normal child exit code; its own stable exit codes distinguish storage cleanup (`73`), retryable capacity or lease pressure (`75`), and configuration or strict-profile replanning (`78`).

## Architectural Constraints

- The public CLI and defaults remain generic across consuming repositories; consumers supply commands, paths, ports, health endpoints, and local policy inputs.
- Supported runtime collectors normalize macOS and Linux evidence, including effective cgroup limits where available, without assuming one machine shape.
- Machine-local configuration may tighten policy but cannot weaken compiled safety floors.
- Heavy work coordinates through a shared per-user lease. Long-lived services retain independent inheritable sessions and do not monopolize the heavy-work lease.
- Resource Guard signals only the process group it creates. It never sheds unrelated user, repository, proxy, or production processes.
- Evidence is bounded and excludes command arguments, repository origins, paths, credentials, and user payloads.
- Local configuration, runtime state, build caches, coverage, release artifacts, and scratch output remain untracked.
- Release monitoring requires explicit health inputs, emits generic bounded evidence, and keeps compatibility with supported retained summary schemas.
- Tagged release assets are immutable, built through the owned release script for the supported OS and architecture matrix, and published with SHA-256 checksums.

## Behaviour Traceability

Executable behavior is specified in [`behaviours/`](behaviours/README.md). The unit adapter executes the complete recursive corpus without exemptions. Local integration and compiled-binary E2E adapters remain strict; every boundary exemption is exact and records both the blocked boundary and its reason. C4 views describe structure and responsibility; Gherkin remains authoritative for observable behavior.
