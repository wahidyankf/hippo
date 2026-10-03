# Task Tracking

Task state lives in the repository, not in a session. A list held only in a conversation is lost at the first
compaction, and the work it described is rediscovered from scratch — see
[governance continuity](../principles/governance-continuity.md).

## Requirements

- Break work into items small enough that each has one visible outcome. "Fix the guard" is not an item; "prove the
  never-started exit-75 receipt in the integration adapter" is.
- Mark an item complete only when it is fully done and verified. A partially done item left ticked is worse than one
  left open, because it removes the reason anyone would look again.
- When an item turns out to be blocked, leave it open and record what blocks it. A blocked item that is quietly closed
  is a decision nobody made.
- Keep the list current as the work proceeds rather than at the end. The value is in the state during the work, not in
  the record afterwards.
- Where work follows a plan, the plan's own delivery list is the tracked list and the only written record of its
  progress, and it is updated in the same change as the work it describes.
- Away from a plan, a progress file in `local-tmp/` is the written progress record. Open it before the task's first
  action; record the goal, every active rule decision, and each item with its status; and update it as items resolve, so
  a session that breaks off resumes from it. It stays until the whole task has ended, delivery and clean-up in every
  repository included, and is then removed under
  [dev artifact clean-up](../workflows/maintenance/dev-artifact-clean-up.md).

## What Belongs in the Repository

Anything the next reader needs to resume: the plan, its delivery units, the evidence each unit produced, and the
decisions taken along the way. Scratch work belongs in ignored `local-tmp/`; a report someone asked for belongs in
ignored `generated-reports/`. Neither is authoritative, and neither is a plan. Scratch holds what a run needs and then
discards — scripts, assets, logs, the touched-path ledger, and away from a plan the progress record — never a copy of a
delivery list, its ticks, or its status.

## New Direction Mid-Task

New, follow-on, or changed direction reaches the list before it reaches the work. Read it against every open item first:
some are now wrong, some are superseded, some are unaffected, and the new direction is usually more than one item.
Record that reconciliation, then continue.

Acting first and updating afterwards produces a list that describes the task as it was requested rather than as it is
being done, which is the state the list exists to prevent. The reconciliation is also where a contradiction between old
and new direction becomes visible; carrying both silently resolves it by accident.

## Discovered Work

Work found on the way is a new item, said out loud. Folding it into the current one hides both: the reviewer cannot find
the fix, and the discovery has no record of why it was needed. See
[pull request boundaries](pull-request-boundaries.md).
