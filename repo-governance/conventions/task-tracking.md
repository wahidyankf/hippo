# Task Tracking

Task state lives in the repository, not in a session. A list held only in a conversation is lost at the first compaction, and the work it described is rediscovered from scratch — see [governance continuity](../principles/governance-continuity.md).

## Requirements

- Break work into items small enough that each has one visible outcome. "Fix the guard" is not an item; "prove the exit-75 retry path in the integration adapter" is.
- Mark an item complete only when it is fully done and verified. A partially done item left ticked is worse than one left open, because it removes the reason anyone would look again.
- When an item turns out to be blocked, leave it open and record what blocks it. A blocked item that is quietly closed is a decision nobody made.
- Keep the list current as the work proceeds rather than at the end. The value is in the state during the work, not in the record afterwards.
- Where work follows a plan, the plan's own delivery list is the tracked list, and it is updated in the same change as the work it describes.

## What Belongs in the Repository

Anything the next reader needs to resume: the plan, its delivery units, the evidence each unit produced, and the decisions taken along the way. Scratch work belongs in ignored `local-tmp/`; a report someone asked for belongs in ignored `generated-reports/`. Neither is authoritative, and neither is a plan.

## New Direction Mid-Task

New, follow-on, or changed direction reaches the list before it reaches the work. Read it against every open item first: some are now wrong, some are superseded, some are unaffected, and the new direction is usually more than one item. Record that reconciliation, then continue.

Acting first and updating afterwards produces a list that describes the task as it was requested rather than as it is being done, which is the state the list exists to prevent. The reconciliation is also where a contradiction between old and new direction becomes visible; carrying both silently resolves it by accident.

## Discovered Work

Work found on the way is a new item, said out loud. Folding it into the current one hides both: the reviewer cannot find the fix, and the discovery has no record of why it was needed. See [pull request boundaries](pull-request-boundaries.md).
