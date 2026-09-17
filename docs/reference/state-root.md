# Shared state root

Every repository on a host that uses the same state root coordinates through the same ledger and
shares one evidence budget. This is what makes cross-repository coordination work at all.

## Location

| Platform | Default                                       |
| -------- | --------------------------------------------- |
| macOS    | `~/Library/Application Support/hippo`         |
| Linux    | `${XDG_STATE_HOME:-$HOME/.local/state}/hippo` |

`HIPPO_ROOT` overrides both.

## Files

Everything below is private implementation data. **None of it is a supported API.** Consumers should
read the documented raw samples and summaries instead — see [JSON schemas](./json-schemas.md).

### Coordination

| File                                    | Purpose                                                             |
| --------------------------------------- | ------------------------------------------------------------------- |
| `coordination.lock`                     | Short advisory lock protecting coordination and session mutations   |
| `coordination-mode.json`                | Schema-1 active mode marker: `exclusive` or `reservation`           |
| `reservations.json`                     | Schema-2 vectors, FIFO waiters, owners, and numeric shedding cause  |
| `owner-metadata/<token>.json`           | Schema-1 safe labels, tier bounds, registration time, and deadline  |
| `reservation-identities/<token>.lock`   | Advisory liveness proof resistant to stale or reused PIDs           |
| `reservation-identities/<token>.anchor` | Same-inode recovery anchor for a live reservation identity          |
| `heavy.lock/owner.json`                 | Exclusive heavy-work owner retained for the guarded child lifecycle |
| `sessions/<token>.json`                 | Private inheritable live-session record                             |
| `.coordination-mode-*.tmp`              | Protected atomic-write staging for the active mode marker           |
| `.reservations-*.tmp`                   | Protected atomic-write staging for the reservation ledger           |

`reservations.json` and `coordination-mode.json` exist only while an epoch is live. An idle root
legitimately has neither.

### Evidence

Development streams are named `development-<class>-<epochMillis>-<pid>`.

| File                                    | Purpose                                                    |
| --------------------------------------- | ---------------------------------------------------------- |
| `<stream>.jsonl`                        | Newest raw samples for an active or completed stream       |
| `<stream>.1.jsonl` … `<stream>.4.jsonl` | Four progressively older raw chunks                        |
| `<stream>.summary.json`                 | Complete lifetime aggregate for the stream                 |
| `<stream>.active.json`                  | Schema-1 live-owner marker containing only the writer PID  |
| `.writers.lock`                         | Cross-process lock protecting writer admission and cleanup |
| `raw/*.jsonl.gz`                        | Daily-compacted completed raw streams                      |
| `history/YYYY-MM-DD.jsonl.gz`           | Daily-compacted queryable summary rows                     |
| `receipts/*.json`                       | Never-started and emergency safety-stop receipts           |

An idle root after a few guarded runs looks like this:

```console
$ ls "$HIPPO_ROOT"
.writers.lock
coordination.lock
development-ephemeral-1788757253723-12070.jsonl
development-ephemeral-1788757253723-12070.summary.json
development-service-1788757256450-12687.jsonl
development-service-1788757256450-12687.summary.json
development-transactional-1788757259105-13633.jsonl
development-transactional-1788757259105-13633.summary.json
reservation-identities
owner-metadata
raw
history
receipts
```

## Evidence budget

One shared root, across every repository using it:

| Bound                        | Value                             |
| ---------------------------- | --------------------------------- |
| Live evidence streams        | 20 maximum                        |
| Raw chunks per live stream   | 5 rotating chunks of 400 KiB each |
| Per live session             | about 2 MiB                       |
| All live sessions at maximum | about 40 MiB                      |
| Legacy inactive root cap     | 50 MiB                            |
| Compacted raw window         | 7 days and 512 MiB                |
| Summary/history window       | 30 days and 128 MiB               |
| Safety receipt window        | 30 days and 128 MiB               |

A lifetime summary stays complete even after its older raw chunks rotate away, because the aggregate
is maintained in fixed memory rather than recomputed from retained samples.

Active streams are protected from cleanup by process-owned markers. Recognized coordination and
reservation atomic-write temporary files are retention-exempt while their writers finalize them.
Completed prior-day raw streams move atomically into gzip files under `raw/`. Prior-day summaries
are deduplicated by run ID and compacted into daily gzip JSONL under `history/`. If the history cap
is reached before 30 days, the oldest day is aggregated by source, exact tags, class, tier, and
outcome before any already-aggregated oldest day is removed. Temporary files older than one hour
are pruned.

## Privacy

Evidence never records command arguments, repository origins, filesystem paths, credentials, or user
payload data. `reservations.json` records only capacity, vectors, classes, profiles, a monotonic
sequence, diagnostic PIDs, process groups, configuration hashes, and the numeric 73/75 shedding
cause. Metadata, summaries, and receipts use validated source/tag labels instead of paths.

## Related

- [JSON schemas](./json-schemas.md)
- [How to inspect evidence and abandoned groups](../how-to/inspect-evidence-and-abandoned-groups.md)
- [Why HIPPO fails closed](../explanation/failing-closed.md)
