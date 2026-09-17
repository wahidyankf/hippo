# Changelog

All notable changes to HIPPO are recorded here. This project follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html), and released tags are immutable — a
published release is never rebuilt or replaced.

Entries are reconstructed from the repository's own history. For the complete commit list of any
release, see its [comparison on GitHub](https://github.com/wahidyankf/hippo/releases).

## [v0.7.1] — 2026-09-17

### Fixed

- `status` and `watch` now report every live schema-1 exclusive compatibility session as a
  privacy-safe legacy owner. A heavy owner and its session record are deduplicated, class totals are
  accurate, and observation never mutates compatibility state. Malformed state still returns `1`;
  a future compatibility schema still returns `76`.

## [v0.7.0] — 2026-09-17

### Added

- Exit `76` uniquely reports a live incompatible peer coordination protocol or a supported protocol
  document with an unsupported schema. `status`, `watch`, and `run` preserve that classification.

### Changed

- **BREAKING (pre-stable):** coordination mode conflicts and live schema-2 entries seen by schema 3 now return
  `76`, not `75` or `78`. Consumers must drain or upgrade and must not retry `76` as capacity.
- Capacity exit `75` is skippable only with the documented diagnostic plus a new schema-1
  `never-started` receipt. Consumer conformance no longer uses a coordination mode conflict to
  synthesize capacity.
- A HIPPO-owned activation failure after payload launch returns `1`, records `task-failed`, and
  writes a `started-activation-failure` receipt instead of returning retryable `75`.
- Malformed or inaccessible coordination state returns `1` without mutation. Valid future schemas
  return `76` without mutation.
- Child-owned `75` and `76` still pass through unchanged, with `task-failed` evidence and no
  never-started receipt.

## [v0.6.1] — 2026-09-17

### Fixed

- History reads now accept both compact JSONL rows and the multiline JSON records produced when
  v0.6.0 compacted pretty-printed summaries. New archives always write one compact JSON object per
  line, so existing shared history remains queryable and future compaction is canonical.

## [v0.6.0] — 2026-09-17

### Added

- Schema-3 adaptive reservation policy with light, standard, and heavy launch-time resource tiers,
  per-tier FIFO deadlines, a two-owner base, and an evidence-gated third-owner burst.
- Privacy-safe repository identities and customizable run tags across live owner/waiter rows,
  lifetime summaries, history queries, and safety receipts.
- `status` schema 5 owner/waiter rows, `watch` changed-snapshot streaming, and `history` filters over
  current plus compacted summaries.
- Atomic daily gzip compaction with 7-day/512-MiB raw and 30-day/128-MiB summary windows, plus bounded
  never-started and emergency safety-stop receipts.

### Changed

- Admission now keeps one stable FIFO waiter until admission or deadline. The payload launches at
  most once; HIPPO no longer implements an outer payload retry loop.
- Tiered admission grants the largest vector that safely fits between the chosen minimum and maximum
  at launch time and never resizes a running payload.
- Transactional work remains protected during ordinary shedding but becomes the final eligible
  victim at the configured emergency memory floor. Emergency stops are explicit and never retried.
- Worktrees must live below `{repository location}/worktrees/`; the repository test gate rejects
  active instructions that prescribe sibling `*-worktrees` layouts.

## [v0.5.3] — 2026-09-09

### Fixed

- Duplicate environment keys now resolve to the **last** occurrence, matching what the guarded child
  observes. `os/exec` deduplicates its environment keeping the last entry, and HIPPO's own writer
  already produced that layout, but the reader returned the first match — so the guard could admit
  against one value while the process it launched read another. The trigger is ordinary:
  `append(os.Environ(), "HIPPO_SESSION="+token)` leaves a duplicate, and an ambient session from an
  outer guard shadowed the caller's explicit override, causing a spurious exit `75` instead of
  session inheritance. The same reader backs `--concurrency-env` mapping, the reservation clamp, and
  launcher identity detection.

## [v0.5.2] — 2026-09-07

### Changed

- `run --wait-for-admission` reports the deferral once instead of once per attempt. The notice reads
  the same every attempt and a long budget is hundreds of them, so repeating it buried the surrender
  line. The surrender still reports the total. Quieting is scoped to the deferral alone: storage
  refusals, warning-pressure admissions, and shedding notices stay audible on every attempt.

## [v0.5.1] — 2026-09-07

### Fixed

- The abandoned-payload report now actually fires. Because the launcher deliberately holds its
  identity lock for the whole group lifetime, the check must ask after the guard's own process rather
  than the lock. `status --json` reports `abandonedProcessGroups` for recorded groups still running
  after their guard has died.

## [v0.5.0] — 2026-09-07

### Added

- `run --wait-for-admission <duration>` retries a deferred owner until the budget is spent, backing
  off from 100 ms to a 2 s ceiling, and still reports `75` if capacity never frees. Zero, the
  default, reports the deferral immediately.

### Changed

- HIPPO is hardened for the contention it exists to manage. Coordination-lock contention is treated
  as a normal condition with a defined resolution in each window: retryable `75` before admission,
  retryable `75` that stops an already-started child at activation, and a skipped observation after
  activation.
- Ownership cleanup that cannot take the lock leaves a reconcilable owner mark and reports a
  deferred-cleanup note rather than a failure; the next repository to take the lock completes the
  release.

### Fixed

- A lifetime report is no longer lost when finalization races cancellation.
- The compiled end-to-end suite budgets for a deferred admission instead of assuming an idle host.
- The child-readiness TERM trap is installed before readiness is published.
- The bootstrap reads cache timestamps through a platform branch, because GNU `stat -f` and BSD
  `stat -f` mean different things.

## [v0.4.0] — 2026-09-05

### Added

- **Schema-2 reservation coordination.** Every service, ephemeral, and transactional owner claims one
  fixed CPU-and-memory vector from a ledger shared across repositories. Admission is atomic,
  overflow-safe, and strict FIFO; host pressure thresholds remain authoritative after a vector fits.
- Automatic `balanced`, `constrained`, and `minimal` reservations use four, two, and one fair-share
  owners respectively.
- `run --reserve-cpu` and `run --reserve-memory-mib` request an explicit vector. An explicit
  reservation may be smaller than its automatic share but never below one CPU or 256 MiB.
- `HIPPO_RESERVED_MEMORY_BYTES` is exported to children admitted in reservation mode.
- Schema-4 status and lifetime summaries carry reservation request, allocation, wait, peak-owner, and
  outcome aggregates.

### Changed

- The effective owner limit within a shared root is the strictest limit contributed by any live owner
  or FIFO waiter, and resets only when the ledger becomes idle.
- Schema-1 configuration continues to select the v0.3.1 exclusive behavior, so the rollout is
  staged. HIPPO never creates a mixed reservation/exclusive epoch.

### Fixed

- The release asset build and its fixtures are Linux-correct.

## [v0.3.1] — 2026-09-05

### Added

- A compatibility bridge between coordination modes. Services own independent inheritable sessions
  while ephemeral and transactional work serializes on `heavy.lock`. A schema-1
  `coordination-mode.json` marker advertises the active mode and is removed after the final session
  exits.

## [v0.3.0] — 2026-09-05

### Changed

- **BREAKING: renamed Resource Guard to HIPPO** — Host Infrastructure Pressure & Process
  Orchestrator. The repository, Go module, executable, release archives, environment protocol, local
  configuration, cache, and state namespace all use `hippo` or `HIPPO_*`. The former names are **not**
  aliases.

  Commands, flags, exit codes, JSON schemas, and supported evidence readers are unchanged. Releases
  published before v0.3.0 stay immutable, and HIPPO does not delete their local cache, configuration,
  or evidence.

### Fixed

- Generated reports use a standardized path.

## [v0.2.0] — 2026-09-04

### Added

- Generic Unix composition for the CLI: caller-owned standard streams, the mandatory `--` boundary
  for guarded commands, and `-` stream conventions.
- The canonical C4 architecture specification.

### Changed

- The command tree moved to Cobra.
- Linux pressure parsing was isolated from the rest of host collection.
- Shared runtime supervision was hardened.

## [v0.1.0] — 2026-09-04

### Added

- First standalone release, published as Resource Guard.

[v0.7.1]: https://github.com/wahidyankf/hippo/releases/tag/v0.7.1
[v0.7.0]: https://github.com/wahidyankf/hippo/releases/tag/v0.7.0
[v0.6.1]: https://github.com/wahidyankf/hippo/releases/tag/v0.6.1
[v0.6.0]: https://github.com/wahidyankf/hippo/releases/tag/v0.6.0
[v0.5.3]: https://github.com/wahidyankf/hippo/releases/tag/v0.5.3
[v0.5.2]: https://github.com/wahidyankf/hippo/releases/tag/v0.5.2
[v0.5.1]: https://github.com/wahidyankf/hippo/releases/tag/v0.5.1
[v0.5.0]: https://github.com/wahidyankf/hippo/releases/tag/v0.5.0
[v0.4.0]: https://github.com/wahidyankf/hippo/releases/tag/v0.4.0
[v0.3.1]: https://github.com/wahidyankf/hippo/releases/tag/v0.3.1
[v0.3.0]: https://github.com/wahidyankf/hippo/releases/tag/v0.3.0
[v0.2.0]: https://github.com/wahidyankf/hippo/releases/tag/v0.2.0
[v0.1.0]: https://github.com/wahidyankf/hippo/releases/tag/v0.1.0
