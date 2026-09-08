# Plan Execution

Use this only after an explicit direction to execute one formal plan. It exists to keep the plan's record true while the work moves.

## Start

1. Choose one plan in `backlogs/` or `in-progress/`. Require a current `PASS` from a [plan quality gate](plan-quality-gate.md) run the user directed. Authority to execute is not authority to run that gate: with no current `PASS`, stop and say so.
2. Enter the plan's worktree before any file or Git mutation, initializing it if new. It sits beside this checkout, never inside it: [worktree location](../conventions/worktree-location.md) gives the reason, which is the Go toolchain. Pass the [integration path](../conventions/integration-path.md) sync gate there, and read the whole incoming diff against the plan when the sync brings commits.
3. If the plan is queued, move it to `plans/in-progress/<slug>/` with its status and both stage maps in one change. Never copy it.
4. Read `learnings.md` before touching anything. Mirror every unchecked executable delivery item into the [task list](../conventions/task-tracking.md), keeping its wording, owner label, order, and references. Dormant recovery items stay dormant.

## Execute

1. Work in order, one active item at a time unless two are genuinely independent. Stop at any pending `[HUMAN]` input, and pass each phase checkpoint before the next starts.
2. Update `delivery.md` at start, at material progress, and at completion. Check an item only once its stated outcome and its stated proof both hold, with a dated note saying what proved it.
3. Keep the plan and the task list synchronized, activating a conditional item when its trigger fires. Add a discovered task to both only when it serves an outcome the plan already has; label it and explain it.
4. Capture learnings as they happen rather than reconstructing them afterwards. Search [`plans/ideas/`](../../plans/ideas/README.md) for overlap and either merge into an existing brief or write one.
5. Land work as delivery units through [worktree to pull request](worktree-to-pull-request.md), reusing the plan's single worktree for each. A unit is finished when its pull request has merged, not when its code is written.
6. Run the required automation for each unit and record only evidence [data safety](../conventions/public-repository-data-safety.md) permits.
7. Apply every applicable rule; a plan grants no authority, and neither does a task list. A failing gate stops the line: repair the cause under [push-hook verification](../conventions/push-hook-verification.md), update `delivery.md` and `learnings.md`, then resume.

## Complete and Archive

1. Require explicit direction for a fresh completion run of the quality gate, and continue only on `PASS`. Do not start it from here. Reconcile every item, criterion, learning, specification, document, rule, and test against `delivery.md`.
2. Tear down what the plan created — its worktree, its local branch, and that branch on `origin` — once every delivery unit that used the worktree has landed. A failed run keeps its worktree and says why. Record proof of each removal.
3. Give every dormant conditional a dated, evidenced `Not triggered` disposition. The plan stays in progress while any required outcome, activated conditional, gate, or human action remains.
4. Take the final checkpoint's local date for the README `Completed` field and for `plans/done/YYYY-MM-DD__<slug>/`. Refuse an existing destination: never merge, overwrite, or add a suffix.
5. In one change, set the status to Done, record outcomes, proof, and deviations, and move the folder with both stage maps. That completes the archival item.
6. Confirm one destination, no source, and no stale reference to the old path; the repository gate verifies links and maps. Committing and pushing need their own [authorization](../conventions/commit-authorization.md).

## Recovery

Interrupted work stays accurately in progress and resumes only after a directed fresh quality-gate `PASS`. If archival verification fails, restore the folder, its status, and both maps. Never leave a plan split across two stages, and never archive unfinished work.
