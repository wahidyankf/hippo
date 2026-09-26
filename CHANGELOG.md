# Changelog

All notable changes to HIPPO are recorded here. This project follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html), and released tags are immutable — a
published release is never rebuilt or replaced.

Entries are reconstructed from the repository's own history. For the complete commit list of any
release, see its [comparison on GitHub](https://github.com/wahidyankf/hippo/releases).

## [v0.8.2] — 2026-09-26

### Changed

- A stalled reservation activation now exits `125` and names `hippo.supervision.failed`; it
  returned `1` with no diagnostic. This is the case where HIPPO launched the child, the shared
  coordination lock then stayed held past the two-second activation window, and HIPPO stopped the
  child. The v0.7.2 activation-contention behaviour chose `1`. The exit-status contract adopted in
  v0.8.0 reserves `1` for a result and gives every failure HIPPO owns before or while starting a
  child the status `125`, but its remap covered only the retired pre-launch numbers and missed this
  post-launch case. A caller that branched on `1` here should branch on `125` and read the reason.
  The failure is still not retryable: the owned cleanup, the `task-failed` summary and the
  `started-activation-failure` receipt are unchanged, so the receipt still says the payload ran.
- A child shed under host pressure other than storage now names `hippo.limit.pressure-shed`, as
  the exit-code reference always said. It exited `124` naming `hippo.limit.capacity-deferred`,
  the reason a deferral that never started anything gives, because the guard handed both to the
  command line as one internal status. The status is still `124` and receipts are unchanged. A
  consumer that branched on `capacity-deferred` will now see a shed under its own reason: the
  payload ran, so recover it before repeating anything.
- `run --resource-tier` beside `--wait-for-admission` under schema 2 now exits `2` naming
  `hippo.args.invalid`, before enqueue or child launch; schema 3 already refused it. The wait was
  silently ignored: a tier sets its own queue deadline, so the run waited up to that deadline
  instead of the one asked for. A caller that passes both should drop `--wait-for-admission` and
  rely on the tier's deadline, or drop the tier to keep the explicit wait.
- `hippo release` and `hippo completion` given no subcommand, or an unknown one, now exit `2`
  naming `hippo.args.invalid`, with the usage on stderr. They printed help to stdout and exited
  `0`, which told a script its work ran. `--help` still exits `0`.
- `run` refuses a flag value it cannot accept with exit `2` naming `hippo.args.invalid`, before it
  reads configuration or host evidence: an unknown `--class` or `--resource-tier`, a malformed
  `--tag` or `--source`, or a `--lease-port` outside `--lease-min`..`--lease-max` or 1–65535, or
  with an invalid `--lease-owner`. So does `monitor --interval` that is not positive. These exited
  `125` naming `hippo.supervision.failed` or `hippo.policy.replan-required`, and an unknown tier
  under schema 1 ran the payload. A caller that branched on `125` for them should branch on `2`.
- `--color` and `--output` accept only their documented values on every command: `always`,
  `never`, or `auto`, and `text` or `json`. Any other value now exits `2` naming
  `hippo.args.invalid`; it was silently ignored, so `--output JSON` quietly wrote no body. A caller
  passing another value should pass a documented one. `release monitor`'s own `--output` is a file
  path and is not checked this way.
- `run` refuses two more flag values before it reads configuration: a negative
  `--wait-for-admission`, and a `--lease-owner`, `--lease-min`, or `--lease-max` without
  `--lease-port`. Both exit `2` naming `hippo.args.invalid`; both were silently ignored and the
  payload ran as though they had not been given. A caller should drop them or add `--lease-port`.
- `run --wait-for-admission` under schema 1, with no reservation configuration, now exits `2`
  naming `hippo.args.invalid` before any payload starts. The flag bounds the schema-2 FIFO queue,
  which schema 1 does not have, so the run was admitted by the profile's own lease wait and the
  value asked for was silently ignored. A caller relying on it should drop the flag, or enable
  schema 2 reservation coordination to get a queue it can bound.
- `history` refuses a `--class`, `--resource-tier`, or `--outcome` value no recorded run can carry,
  with exit `2` naming `hippo.args.invalid` and the accepted values. It returned `1`, the empty
  result, so a misspelt filter read as "nothing matched". A caller that branched on `1` for such a
  value should correct the value; a valid filter that matches nothing still exits `1`.
- `status --tag` and `watch --tag` with a malformed filter now exit `2` naming
  `hippo.args.invalid`, before configuration is read, as `run` and `history` already did. They
  exited `125` naming `hippo.policy.replan-required`, which says no profile admits the request. A
  caller that branched on `125` here should branch on `2` and correct the filter.
- A `run` whose identity has no source — `--tag` given, or schema 3 in force, with no `--source` and
  no `hippo.identity.json` through `HIPPO_IDENTITY`, upward discovery, or `HIPPO_DEFAULT_IDENTITY` —
  now exits `2` naming `hippo.args.invalid`, and the diagnostic names `--source`. It exited `125`
  naming `hippo.policy.replan-required`, which says no profile admits the request, although the
  fix is one flag. A caller that branched on `125` here should branch on `2` and supply a source.
  An identity file that is present and invalid still exits `125`.
- `release monitor` reports a missing or malformed `--health-url` or `--routed-origin`, a missing
  output, summary or deployment root, a negative `--duration-ms`, or an out-of-range
  `--service-port` as a usage mistake: exit `2` naming `hippo.args.invalid`. Each exited `125`
  naming `hippo.supervision.failed`, although nothing had been supervised.
- `history` reports an archive it cannot read as `hippo.evidence.unreadable`, a new code, still
  exit `125`. It named `hippo.evidence.unwritable`, which means the evidence root refused a write,
  so a reader was sent to fix permissions for what is a corrupt file. The archive is left untouched
  for inspection. A consumer matching `evidence.unwritable` on `history` should match the new code.
- `--help` describes `125` as HIPPO failing before, while, or after starting the work, and `2` as
  also covering an internal fault, matching the exit-code reference. A test now holds every line of
  the help's exit block to that reference.

### Fixed

- HIPPO read `--output` and `--color` from the guarded command's own arguments. `hippo run -- tool
--output json` added HIPPO's failure body beneath its diagnostic, and a child's `--color always`
  coloured it, although the documentation says HIPPO interprets nothing after `--`. It now reads
  those flags only before `--`. A caller that wanted the body places `--output json` before `--`.
- `release monitor --output json` wrote a failure body, because the global flag was read from argv
  although the command's own `--output` — a raw sample path — had taken its place. It no longer
  does, wherever the flag is placed.
- A positional argument a command does not take, such as `hippo history extra` or `hippo status
foo`, printed the root `Usage: hippo [flags]` block beneath a diagnostic naming the right command.
  It prints that command's own usage; so does `run` without `--`. The status is still `2` naming
  `hippo.args.invalid`.
- The failure body's `command` field depended on where the global flags sat:
  `hippo --output json history --since nope` named `hippo`, while the same mistake with the flag
  after the command named `hippo history`, and `hippo release bogus` named `hippo release bogus`.
  It now names the command that ran — `hippo history`, `hippo release` — wherever the flags sit.
  A consumer that parsed `command` needs no change unless it matched those wrong values.
- `release assess` over rejected evidence printed the rejection on a bare line and then
  `hippo: [hippo.limit.capacity-deferred] capacity deferred this work; retry when the host is
quieter`, although nothing was deferred. The diagnostic is now one line that says what happened:
  `release evidence rejected: <why>`, and the failure body's message carries the same sentence. The
  status and reason are unchanged, `124` naming `hippo.limit.capacity-deferred`.
- A bounded wait that ran out on exhausted capacity could report the wrong reason and skip its
  receipt. On its last pass the wait asks for the shared root's coordination lock with almost no
  budget left, and it offered a free lock and an already-expired timer to the same `select`. Go
  chooses between ready cases at random, so HIPPO sometimes refused a lock nobody held: it still
  exited `124`, but named "another admission is updating the shared root" instead of the capacity
  deferral, and wrote no `never-started` receipt, which is what a consumer reads before requeueing.
  A free lock is now taken before any wait begins, and the timer only bounds a wait for a lock
  someone holds.
- `hippo-conformance` could never skip a capacity deferral against a current binary. An
  allow-capacity-skip check still required the retired exit `75` and a progress sentence the guard
  does not always print. It now requires exit `124`, the `hippo.limit.capacity-deferred` reason, and
  a new `never-started` receipt, as before. A pressure shed also exits `124` but writes no
  `never-started` receipt, so it is not skipped.
- The specification stated outcomes in the retired numbers. Its scenarios and architecture now use
  the current contract: `124` for a limit, `125` for HIPPO's own failure, and the reason code. The
  loaded-gate test harness had the same stale `75` and would have refused every real capacity
  deferral on a saturated host.
- Documentation caught up with v0.8.0's exit vocabulary. Several transcripts still showed the
  retired `Error:` prefix and the old numbers `75`, `76` and `78`. A malformed concurrency name was
  documented as exit `1` rather than `2`, and a malformed inherited value as exit `2` rather than
  `125`.

## [v0.8.1] — 2026-09-23

### Fixed

- `--help` named a diagnostic form HIPPO has never written. The exit block closed by telling a
  reader that every failure names its reason as `HIPPO error [hippo.area.reason]` on stderr; v0.8.0
  moved diagnostics to the GNU `program: message` form, so HIPPO writes `hippo: [hippo.area.reason]`
  and always had, once that release shipped. The one place a reader looks to learn what to match on
  named a string the tool never emits. Nothing asserted that text, which is why the full gate passed
  over it in every repository that runs one; it was found by reading the published help against the
  binary's own output.

## [v0.8.0] — 2026-09-23

### Changed — breaking

- The exit vocabulary is closed and renumbered. HIPPO's own refusals returned `73`, `75`, `76` and
  `78`; they now return `124` when a limit stopped the work and `125` when HIPPO could not do its
  job and started nothing. A usage mistake returns `2` rather than `1`, and `1` now means only that
  the work ran and the answer is empty. The four old numbers sat inside the range a child may
  return, so the number alone never said who chose it, and no consumer branched on them: the
  `./hippo` bootstrap shared across nine repositories reached `exit 78` from one helper in sixteen
  call sites and never asked which reason produced it.
- A command that cannot be run is now reported before admission: `127` when it is not found and
  `126` when it exists and cannot be executed, matching every POSIX shell. Both were `1`.
- A failed invocation no longer writes the usage block to stdout. Everything HIPPO says about a
  failure goes to stderr, and stdout stays empty, so a consumer reading the answer never meets a
  flag list instead.
- Diagnostics use the GNU `program: message` form: `hippo: [hippo.area.reason] message`.
- `history` exits `1` when nothing matched, rather than `0`.

### Added

- Every failure names a reason, a namespaced `hippo.area.reason` from a closed vocabulary published
  in [Exit codes and error codes](./docs/reference/exit-codes.md). It reaches the caller on stderr
  and, with `--output json`, as `error.code` in a machine-readable body carrying `schemaVersion`,
  `command`, `exitCode`, `error.message` and `error.retryable`.
- A `--version` flag on the root command, beside the existing `version` subcommand.
- A `--color` flag honouring `NO_COLOR` and `TERM=dumb`. The default is plain text, because HIPPO's
  output is read by scripts far more often than by people.
- `--help` publishes the exit statuses HIPPO can return.
- A top-level handler turns a fault in HIPPO into a declared status and a reason, rather than a
  stack trace and whatever the runtime chose.

### Fixed

- The end-to-end behaviour adapter inherited `HIPPO_BIN` from the guard that wraps the test script,
  so it measured the checksum-pinned release already installed rather than the working tree. It now
  builds and runs what is being tested.

## [v0.7.2] — 2026-09-17

### Fixed

- Reservation activation now waits up to two seconds for brief shared-root contention. Concurrent
  repository bursts no longer stop an already started payload merely because peer admission
  transactions occupied the previous 100 ms lifecycle window. A genuinely stalled activation
  still stops owned work boundedly, returns `1`, and records `started-activation-failure`.

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

[v0.8.1]: https://github.com/wahidyankf/hippo/releases/tag/v0.8.1
[v0.8.0]: https://github.com/wahidyankf/hippo/releases/tag/v0.8.0
[v0.7.2]: https://github.com/wahidyankf/hippo/releases/tag/v0.7.2
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
