# Execution

## Entry

A plan in `plans/in-progress/` with its checklist ready. A [quality-gate](../quality/plan-quality-gate.md) verdict, when
recorded, is advisory history: a `FAIL` or `BLOCKED` row keeps its owner. Execution starts with or without a verdict and
never runs the gate itself; a missing verdict is recorded as `plan-quality-gate: not run (manual-only)`. Before
starting, resolve every dependency the plan declares under
[Plan-Order Graph](../../conventions/plans/012-plan-order-graph.md): each satisfied, or stop and name the unmet one.
Silence reads as none. A mid-run graph change via reviewed merged PR is authoritative — adapt to it.

A queued plan reaches `plans/in-progress/` only by its own promotion: one pull request moving only it to `origin/main`,
updating its graph node in the same change, before Phase 0. Worktree from that fresh trunk; the promotion branch dies at
merge.

Inputs: `plan` (`directory`, required); `max-worktrees-per-plan-repo` (default `1`); `max-active-worktrees` (default
`1`). Only the caller raises a cap.

Mirror the run ledger from the ignored `local-tmp/` scratch (see [Task Tracking](../../conventions/task-tracking.md))
into one per-run Issue. Open it first and record its number as the claim; before worktree work, prove from lease,
branches, and open PRs that no other run holds the plan. A stale lease yields only to an explicit caller override naming
it.

## Sequence

1. **Read the plan before the checklist.** `delivery.md` is executable, not self-explanatory; the other five hold why.
2. **Confirm checkout, then access.** Which copy, branch or worktree, mode — synced from the latest `origin/main` before
   any change; a late-noticed wrong checkout is expensive. Then prove access from the plan's scope alone: every scope it
   needs, least privilege, infra secrets only for infra scope, portable versus machine-bound per need; missing access
   stops the run.
3. **Work in dependency order** per
   [Dependency Graph and Recovery](../../conventions/plans/011-dependency-graph-and-recovery.md). Outside blockers are
   recorded with their unblockers, never silently skipped.
4. **Work ready items under the worktree lock.** One `in_progress` per worktree; every compute node through one guard
   per [Resource-Aware Development](../../development/resource-aware-development.md). A deferred guard queues — retry,
   never bypass, never abandon. Failures block only downstream. Tick, record, move on — one step, outcome and proof
   held; no end-of-session batch ticks. Reorder only by revising `delivery.md` first; a material change re-passes the
   quality gate.
5. **Ship code in batched cycles.** One RED plus one GREEN per cohesive batch, selection frozen, per
   [Batched Cycles](../../development/test-driven-development/001-batched-cycles-in-planned-delivery.md); refactor only
   on green with the batch's proof held.
6. **Record results, not only ticks** — product, change, surprise — per
   [Verification Routing](../../development/planning-capabilities/006-verification-routing.md). **Fix every failure**,
   pre-existing included: still red at ship time means this plan's work, explanation or not.
7. **Route discoveries to `learnings.md`** (or a bug-fix plan's Learnings) as they arise.
8. **Land each delivery unit** through the [integration path](../../conventions/integration-path.md), refreshed against
   current `main` with gates re-run, per
   [Delivery Seams and Ownership](../../development/planning-capabilities/005-delivery-seams-and-ownership.md);
   independent units land in any order.
9. **Run [Execution Check](plan-execution-check.md)** once every substantive item is terminal.

## Goal and Termination

One end state; `pass` is the only success:

- **`pass`**: items terminal, check permits archival, delivery landed and verified, plan in `plans/done/`, worktree and
  branches gone, learnings routed or discarded.
- **`handed-off`**: `[AI]` work ready, only `[HUMAN]` merge or decision remains. No archive or cleanup; resume to `pass`
  after confirmation.
- **`partial`**: findings remain or work unexecuted. Plan stays in `plans/in-progress/` with its location; blockers and
  unblockers only.
- **`fail`**: technical error blocks completion; plan and work location stay for diagnosis.

## Exit

Only the Execution Check permits archival and `pass`. Cleanup may pend in `plans/done/`, but task completion awaits its
proof. Partial leaves recorded blockers and unblockers; the plan stays and the check waits. A failed run keeps its
worktree and records why.

## Status Cadence

Milestones carry ordinary progress. Independently send a user-visible five-line heartbeat every **20 minutes** —
weighted progress, steps left, blockers, decisions, next `[HUMAN]` timing — plus phase boundaries and terminal states.
Weights from `delivery.md`, else `1` each. A queued node is reported, never a stop. At each heartbeat, clear owned
subagents and processes of stuck work: `in_progress` with no progress since the last heartbeat and no guard or error
reason is stuck — list under blockers, nudge once, escalate if still stuck next.

## Pause Safety

At any pause the plan itself carries resume state: checkout, last terminal gate, next unresolved item, partly consumed
budget. A resumed session continues a budget; it does not reset one.
