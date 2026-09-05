# HIPPO Architecture

This document is the canonical as-built C4 model for HIPPO — Host Infrastructure Pressure &
Process Orchestrator. It describes the public system boundary, runtime containers, internal
responsibilities, and material constraints without prescribing any consuming repository's
architecture.

The diagrams use ASCII so they remain readable in terminals and plain-text tooling. Every relationship and constraint also appears in searchable prose.

## System Context

```text
+----------------------+       invokes        +----------------------+
| Person               | -------------------> | Software system      |
| Operator/contributor |                      | HIPPO                |
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

An operator, repository script, Git hook, or CI task invokes HIPPO before compute-bearing local work. HIPPO reads normalized host evidence, resolves a safe capacity profile, and atomically coordinates eligible work through a shared CPU-and-memory reservation ledger. Every service, ephemeral, and transactional owner participates. A schema-1 exclusive bridge remains for v0.3.1 consumers during rollout. Each guard supervises and signals only the child process group it owns; remote guards coordinate pressure shedding through an owner mark and never signal one another's groups. Callers may select environment variables that receive fixed allocated concurrency and may connect standard streams without teaching HIPPO about a build ecosystem. Release monitoring may probe explicit local and routed health endpoints supplied by the caller. HIPPO is repository-independent and does not know consumer project layouts, commands, or infrastructure defaults.

## Container View

```text
                                      HIPPO system
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
  |  +------------------+                   | Local config |    |   | Runtime     ||
  |                                         +--------------+    |   | state       ||
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

The Go CLI is the only long-running HIPPO execution container. It reads an optional machine-local JSON configuration, collects host and process evidence through operating-system interfaces, and stores coordination, lease, and bounded evidence records in a shared per-user state root. HIPPO instances launched by different repositories coordinate through that same root. The configuration and state roots are runtime inputs; neither is committed to this repository. A guarded command runs as a distinct child process group so interruption and pressure shedding cannot target unrelated processes.

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
- **Config loader and profiles** resolve configuration precedence, select schema-1 exclusive or schema-2 reservation coordination, validate caps and automatic owner shares, and preserve compiled safety floors.
- **Host collector** normalizes macOS, Linux, cgroup, swap, pressure, CPU, disk, and process evidence into portable samples.
- **Policy engine and profiles** classify evidence, choose an adaptive development profile, and preserve strict transaction and release envelopes.
- **Execution guard** owns coordination mode, atomic vector and FIFO mutations, liveness identities, compatibility and port leases, controlling-terminal ownership, child-process lifecycle and streams, fixed generic concurrency mapping, targeted pressure shedding, and bounded evidence retention.
- **Release guard** owns consecutive release admission, health sampling, file or caller-owned stream sinks, summary schemas, and final overlap assessment.
- **Evidence store** owns live-writer admission, raw chunk rotation, fixed-memory quantiles, expiry, and inactive-file retention.

The `internal/policy` package owns the shared typed samples, collectors, task classes, decisions, thresholds, and profile resolution. Execution, platform collection, and release monitoring depend directly on that package instead of a forwarding facade or consumer-specific application types. Long-running operations receive caller-owned contexts; only the process entry translates operating-system signals into cancellation.

## Guarded Execution Dynamic View

```text
Caller     CLI/config     Host/policy   Coordination store   Child group
  |             |              |               |               |
  | run request |              |               |               |
  |------------>| collect      |               |               |
  |             |------------->|               |               |
  |             | resolve      |               |               |
  |             |<-------------|               |               |
  |             | validate floors/total capacity               |
  |             |------------->|               |               |
  |             | lock, reconcile liveness/FIFO |               |
  |             |----------------------------->|               |
  |             | reserve CPU+memory atomically |               |
  |             |----------------------------->|               |
  |             | unlock coordination mutation |               |
  |             |----------------------------->|               |
  |             | foreground/start admitted child group        |
  |             |--------------------------------------------->|
  |             | observe own shedding mark before sampling    |
  |             |----------------------------->|               |
  |             | sample and assess repeatedly |               |
  |             |------------->|               |               |
  |             | mark newest ephemeral, then service + 73/75  |
  |             |----------------------------->|               |
  |             | remote selector waits; owner stops own group |
  |             |--------------------------------------------->|
  |             | wait until the owned child is reaped         |
  |             |<---------------------------------------------|
  |             | finalize evidence, then atomically release   |
  |             | vector/identity and clear idle mode marker   |
  |             |----------------------------->|               |
  | child code or stable guard exit             |               |
  |<------------|              |               |               |
```

Admission failures return before child creation. Reservation mode derives a safe vector from host parallelism and effective memory, applies optional caps, and divides automatic requests by profile shares of four, two, or one. Explicit dimensions may be smaller but cannot cross one CPU or 256 MiB. Checked subtraction verifies both dimensions without integer wrap. Impossible requests replan immediately; temporary exhaustion joins a strict FIFO bounded wait. The effective active-owner limit is the minimum contributed by every live owner and queued waiter. Inheritance reuses the token's fixed allocation without a second owner. Host thresholds remain authoritative after budget fit.

An admitted child inherits its fixed profile, allocated CPU, reserved memory, clamped caller-selected concurrency mappings, and caller-owned streams. For controlling-terminal input, HIPPO gives the isolated child group foreground ownership and restores the caller group on every exit path. Critical pressure marks at most one newest ephemeral owner per locked evaluation, then one newest service when no ephemeral is eligible; transactional owners are protected. The mark carries only the selected stable exit (`73` for storage or `75` for other pressure). A remote selector waits boundedly and never signals. The owning guard observes its own mark before collecting another host sample, performs bounded TERM-to-KILL cleanup, waits/reaps, and only then releases. A live unresponsive marked owner remains the global barrier against another election. HIPPO preserves a normal child exit code; stable exits remain storage cleanup (`73`), retryable capacity or coordination pressure (`75`), and configuration, impossible capacity, invalid mapping, or strict-profile replanning (`78`).

## Architectural Constraints

- The public CLI and defaults remain generic across consuming repositories; consumers supply commands, paths, ports, health endpoints, and local policy inputs.
- Supported runtime collectors normalize macOS and Linux evidence, including effective cgroup limits where available, without assuming one machine shape.
- Machine-local schema-2 configuration may tighten vector caps and owner counts but cannot weaken the one-CPU, 256 MiB, or 20-owner compiled floors and ceilings. The shared-root owner cap is the conservative minimum from all live owners and waiters and resets only when the ledger is idle. Schema 1 retains exclusive compatibility semantics.
- Every ledger, queue, victim, and compatibility mutation is serialized by the shared `coordination.lock`; that lock is never held while a child runs. Schema-2 `reservations.json` records only capacity, vectors, classes, profiles, monotonic sequence, diagnostic PIDs, process groups, configuration hashes, and the numeric 73/75 shedding cause.
- Decoded reservation state must satisfy strict token, class, sequence, nonnegative-vector, checked-total, owner/waiter, owner-limit, and shedding-state invariants. Corruption fails closed and preserves bytes; admission uses subtraction-based vector checks so aggregate arithmetic cannot wrap.
- A missing reservation ledger is an empty epoch only when mode and identity evidence positively prove it; unknown identity errors retain accounting. Lifecycle lock waits are bounded, failed release retains owner identity evidence, cancelled waiter cleanup receives a fresh bounded context, and exhausted FIFO sequences reset only after a positively empty epoch.
- Per-token advisory identities carry device and inode metadata plus a same-inode recovery anchor, rather than trusting diagnostic PID equality. A private capability-authenticated HIPPO launcher exclusively holds both reservation and port identities while supervising the complete command group; arbitrary payloads and descendants cannot inherit or forge those descriptors. Supervisor-only death, leader exit, or payload descriptor closure therefore retains accounting until positive group retirement. Unknown or unreaped retirement stays fail closed, while a positively stale identity is reclaimed even when the diagnostic PID has been reused. New schema-1 ownership follows the same lifetime; legacy zero-metadata PID records remain conservative. Inherited tokens must still identify a live locked owner.
- Reservation mode cannot replace active or unverifiable schema-1 exclusive state. Compatibility mode cannot replace reservation state. Both conflicts preserve the existing mode and return `75` before child creation.
- Compatibility heavy work continues to use one shared per-user `heavy.lock`; services retain independent inheritable compatibility sessions. Malformed heavy state, an unreadable session directory, malformed or unreadable service-session records, and failed positively-stale heavy-state removal stay byte-preserving and fail-closed during reservation takeover. A reservation marker is written only after positive empty/stale proof and successful cleanup, so mixed epochs cannot be created.
- An unreadable heavy-work owner, unsupported heavy-owner schema, unverifiable service session, or inaccessible compatibility inventory is never reclaimed automatically. Compatibility admission stays fail-closed until an operator confirms that no owner remains, restores filesystem accessibility, and corrects the private state.
- A guard signals only the process group of its own reservation or compatibility session. Remote selectors only mark and observe an owner; they never signal a foreign group and cannot select another victim while a live marked owner remains. HIPPO never sheds unrelated user, repository, proxy, or production processes, and it never selects an admitted transactional owner.
- Child stdin, stdout, and stderr remain caller-owned and distinct from guard diagnostics. Foreground terminal transfer applies only when inherited stdin is the controlling foreground TTY and is always restored. Consumers opt into tool concurrency mappings by valid environment name; no build system is compiled into the core.
- One shared state root admits at most 20 live evidence streams across all consuming repositories. Each live stream keeps five rotating 400 KiB raw chunks, for about 2 MiB per session and about 40 MiB at the maximum live count.
- Completed raw chunks and summaries share a 50 MiB inactive cap. Raw chunks expire after seven days, summaries after thirty days, and active streams are protected from cleanup by process-owned markers. Concurrent post-snapshot disappearance is benign only when the filesystem reports not-exist; every other inventory, metadata, or removal error propagates. Recognized coordination and reservation atomic-write temporary files are retention-exempt while writers finalize them.
- Lifetime summaries use fixed-memory aggregates and remain complete even after older raw chunks rotate away. Evidence excludes command arguments, repository origins, paths, credentials, and user payloads.
- Status and development summaries use schema 4 for privacy-safe reservation totals and lifetime request/allocation/wait aggregates. Status surfaces coordination corruption instead of fabricating zero totals, and every admission event atomically raises the owning development session's peak count even between host-sampling ticks. Raw host samples retain their existing schema and schema-3 development fields retain their meanings.
- The optional four-consumer conformance runner receives all checkout paths, names, commands, shared state, and pinned binary identity through one strict manifest. It freezes canonical absolute/symlink paths plus checkout and created shared-root filesystem identities, rejects checkout/shared-root ancestry overlap, revalidates those objects around every command, and snapshots all checkouts before any command. Bootstrap is sequential per consumer but concurrent across four lanes with a deterministic failure barrier; gates are concurrent after coordination. Every command receives a distinct read-and-execute-only copy of one safely opened verified executable; the command copy is checked after execution, and the hidden master plus manifest source are revalidated before later work and final reconciliation. Caller session/allocation outputs and the caller repository's bootstrap-only default configuration are removed before consumer commands, while an explicit configuration override remains and a consumer's own outer guard may establish nested inheritance. Every started process group must retire after normal, nonzero, or cancelled leader exit before its phase returns. Cancellation prevents later starts and owns bounded TERM/KILL/group-retirement observation; final identity plus checkout reconciliation uses a fresh bounded context. Integrity and cleanup errors remain fatal beside an otherwise skippable capacity exit and are joined privately. The runner has no product-specific defaults and does not expose paths in validation or execution errors.
- Local configuration, runtime state, build caches, coverage, and release artifacts remain untracked. Disposable scratch uses `local-tmp/`; requested non-authoritative reports use `generated-reports/`.
- Release monitoring requires explicit health inputs, emits generic bounded file evidence or caller-owned stdout streams, rejects mixed raw and summary schemas on one stream, and keeps compatibility with supported retained summary schemas.
- Tagged release assets are immutable and originate from the exact full lowercase `HEAD` commit in a clean checkout. The owned script builds from an isolated exact-commit materialization for the supported OS/architecture matrix. Validation requires exactly four checksummed archives, one regular mode-755 member each, matching clean VCS/native identity, and rejects link or special members before extraction. The workflow peels the tag and requires its commit to be reachable from `origin/main` without adding Actions artifact, package, or cache storage.

## Behaviour Traceability

Executable behavior is specified in [`behaviours/`](behaviours/README.md). The unit adapter executes the complete recursive corpus without exemptions. Local integration and compiled-binary E2E adapters remain strict; every boundary exemption is exact and records both the blocked boundary and its reason. C4 views describe structure and responsibility; Gherkin remains authoritative for observable behavior.
