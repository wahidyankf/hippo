# How-to guides

Directions for a goal you already have. Each guide assumes you know what you want and gets you
there.

If you are still learning what HIPPO does, start with the
[tutorials](../tutorials/README.md) instead.

## Set up

- [How to install a pinned release](./install-a-pinned-release.md) — download, verify a checksum, and
  put a tagged binary on PATH.
- [How to enable reservation coordination](./enable-reservation-coordination.md) — turn on the shared
  CPU-and-memory budget, cap it, and migrate a host that is still in exclusive mode.

## Integrate

- [How to map concurrency into your build tool](./map-concurrency-into-your-build-tool.md) — wire
  HIPPO's allocation into the variable your build tool already reads.
- [How to respond to a HIPPO exit code](./respond-to-exit-codes.md) — handle `73`, `75`, and `78`
  correctly in scripts.

## Operate

- [How to watch host pressure](./watch-host-pressure.md) — see state transitions as they happen and
  identify what is saturated.
- [How to inspect evidence and abandoned process groups](./inspect-evidence-and-abandoned-groups.md)
  — read the ledger, read a lifetime summary, and safely investigate an orphaned payload.
- [How to monitor a release](./monitor-a-release.md) — capture and assess deployment-window evidence.

## Verify

- [How to run the four-consumer conformance check](./run-the-conformance-check.md) — prove four
  repositories can adopt one pinned binary and coordinate through one shared root.

## Next steps

- [Reference](../reference/README.md) for exact flags, codes, and schemas.
- [Explanation](../explanation/README.md) for why HIPPO behaves this way.
