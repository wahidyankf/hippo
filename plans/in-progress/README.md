# In Progress

This stage holds only what is being executed now. A plan's status, its `delivery.md` checklist, and its `learnings.md`
log are kept true to what has actually happened, as it happens. A checklist reconciled only at the end was never a
checklist; it was a summary written afterwards.

Work starts by moving one folder out of [`../backlog/`](../backlog/README.md) without renaming it, and proceeds under
[plan execution](../../repo-governance/workflows/plan/plan-execution.md).

Work finishes only when every required outcome, acceptance condition, verification, learning, and triggered conditional
has been reconciled against the record. A recovery task that never fired is given a dated, evidenced `Not triggered`
disposition rather than a checkmark, because a checkmark on something that never ran is the one entry a record cannot
survive. The same workflow then performs the dated move into [`../done/`](../done/README.md), refusing a destination
that already exists, with both stage indexes updated in that one change.

## Directory Map

- [Fix: Cancelled waiter cleanup flakes under load](fix-cancelled-waiter-cleanup-flake/README.md) — the cancelled
  waiter's cleanup never refuses a free coordination lock, and its scenario tolerates a loaded host.
- [Fix: Degraded-lineage admission scenario flakes under load](fix-degraded-lineage-scenario-flake/README.md) — the
  degraded-lineage scenarios time their admission window on a logical clock, so a loaded runner no longer defers them.
- [Fix: Distinct-root coordination lock test flakes under load](fix-distinct-root-lock-test-flake/README.md) — the
  distinct-root lock test judges serialization by a zero-wait refusal, not the wall clock, so a slow runner no longer
  fails it.
- [Strict Go linting and domain modeling](strict-go-linting-and-domain-modeling/README.md) — carry outcomes, internal
  reasons, profile lineage, and the admission decision as Go types, and gate the result.
