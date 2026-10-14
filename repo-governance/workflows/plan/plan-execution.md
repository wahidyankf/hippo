# Execution

## Entry

A plan in `plans/in-progress/` with its checklist ready. A [quality-gate](../quality/plan-quality-gate.md) verdict,
when recorded, is advisory history: a `FAIL` or `BLOCKED` row keeps the owner the gate gave it. Execution starts with
or without a verdict, records a missing one as `plan-quality-gate: not run (manual-only)`, and never runs the gate
itself.

## Sequence

1. **Read the plan before the checklist.** `delivery.md` is written to be executable, not to be self-explanatory. The
   other five documents hold why each item exists.
2. **Confirm the execution checkout.** Which working copy, which branch or worktree, which delivery mode. Executing in
   the wrong checkout is recoverable; noticing late is expensive. Before starting plan execution or creating a new
   worktree, sync the execution checkout from the latest origin/main.
3. **Work items in order.** An item blocked by something outside the plan is recorded as blocked, with what would
   unblock it, rather than skipped silently.
4. **Resolve each item atomically.** Tick the checkbox, record the result, and move on — in that order, as one step. A
   batch of ticks applied at the end of a session cannot say which item produced which result.
5. **Record results, not only ticks.** What was produced, what changed, what was surprising. A tick says an action
   happened; it does not say what it found. See
   [Verification Routing](../../development/planning-capabilities/006-verification-routing.md).
6. **Fix what fails, including what was already failing.** A check that was red before the plan started is still red
   because of this plan's work by the time it ships. Pre-existing is an explanation, not an exemption.
7. **Route discoveries to `learnings.md`**, or a bug-fix plan's Learnings section, as they happen, not from memory
   afterwards.
8. **Run [Execution Check](plan-execution-check.md)** once every substantive item is terminal.

## Exit

Every substantive checklist item is terminal, `learnings.md` holds what execution discovered, and the execution check
has recorded a verdict. Never require a full E2E run as a completion condition for plan execution or a plan phase; full
E2E is a separately batched set.

## Pause Safety

Execution stops at arbitrary moments. At any pause the plan itself must carry enough state to resume: the current
checkout, the last terminal gate, the next unresolved item, and any bounded budget already partly consumed.

A resumed session continues a budget; it does not reset one.
