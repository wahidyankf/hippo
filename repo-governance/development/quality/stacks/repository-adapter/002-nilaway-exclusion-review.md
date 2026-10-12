---
description: >-
  Records NilAway exclusion scope, hidden-read and stale-waiver review limitations, and the known false-positive cases.
when_to_use: >-
  Use when reviewing a changed NilAway exclusion or checking whether a colocated waiver remains necessary.
---

# NilAway Exclusion Review

Invocation, pins and exclusion directives follow [Go Analysis Gates](001-go-analysis-gates.md).

## Review Limitations

- Syntax-node scope: after `if`'s `{` misses that line; directly above covers it.
- One conflict per nil source hides later unguarded reads under exclusions. Review changed excluded sources for new
  reads; the gate stays silent.
- No stale-directive gate: `nolintlint` cannot judge continued need; review removes resolved waivers.

## Known Cases

Both false-positive exclusions escaped adjacent tests:

- [`ExclusiveStatus`](../../../../../internal/adapters/runtime/exclusive_status.go#L62), result 0 of
  `liveExclusiveHeavyOwner`: see the colocated waiver. Tests: `internal/adapters/runtime/exclusive_status_test.go`.
- [`DescribeHeavyLease`](../../../../../internal/adapters/runtime/lease.go#L305), result 0 of `readLeaseOwner`: see the
  colocated waiver. Tests: `tests/integration/lease_evidence_test.go`.
