# Rules Grooming

Keeping this tree true. Run on request, never automatically.

## What It Checks

**Correspondence.** Does each document still describe what the repository does? A rule about a file that moved, a gate that was renamed, or a hook that no longer exists is worse than no rule: it is followed, and it is wrong.

**Coverage.** Is every rule the repository actually enforces written down somewhere? A gate that fails for a reason no document names is a rule nobody agreed to.

**Placement.** Is each rule at the level that owns it? A principle written as a convention is a principle nobody weights properly; a workflow written as a convention is a procedure with no order.

**Duplication.** Is any rule stated twice? Two copies drift, and the reader cannot tell which one is current. Replace one with a link.

**Budget.** Is any document approaching its limit for the wrong reason — because it grew, rather than because it is genuinely one thing that takes that long to say? Split by reader task; never raise the limit.

## What It Produces

A list of findings, each naming the document and what makes it untrue. Fixes land as ordinary changes through [worktree to pull request](worktree-to-pull-request.md), one theme at a time.

Grooming never rewrites a rule's substance on its own authority. A rule that looks wrong is a finding to raise, not a paragraph to quietly improve — see [governance continuity](../principles/governance-continuity.md).

## When to Run It

After a structural change to the repository: a gate replaced, a workflow retired, a directory added. Also when a reader reports that a document did not match what they found, which is the strongest signal this tree produces.
