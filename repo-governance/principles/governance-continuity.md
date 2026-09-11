# Governance Continuity

Rules survive context loss. A session that has been compacted, resumed, or handed to a different reader must arrive at the same rules as the session that started the work.

## Requirements

- Every rule lives in a file in this repository. A rule that exists only in a conversation is not a rule; it is a memory, and memories are the first thing a compaction discards.
- Root [`AGENTS.md`](../../AGENTS.md) is the single entry point, and it is complete: every rule in this tree is reachable from it. A rule nobody can find is a rule nobody follows.
- Retain unfamiliar in-flight changes under `plans/` and this tree rather than reverting them. Work that a reader does not recognize is more often work they have not yet read than work that is wrong.
- Record a decision where the decision applies. A choice explained only in a pull-request description is lost the moment the branch is deleted.
- When a rule is discovered to be wrong, change the document. Working around a rule in silence leaves the next reader to rediscover the same problem with less evidence.

## Why This Level

This is a principle rather than a convention because it constrains how every other document must be written, not what any of them say. A convention that assumed the reader remembered the last session would be unenforceable the first time one ended.

## Applied Here

The [plans convention](../conventions/plans.md) and [task tracking](../conventions/task-tracking.md) exist for the same reason: they move state out of a session and into the repository, where the next reader — human or otherwise — starts from the same place the last one left.
