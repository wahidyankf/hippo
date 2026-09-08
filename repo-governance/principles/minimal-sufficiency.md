# Minimal Sufficiency

The smallest change that fully solves the problem, and no smaller. Both halves are load-bearing: a partial fix that leaves the defect reachable is not minimal, it is unfinished.

## The Cycle

**Understand.** Read the failing behaviour, the specification that governs it, and the code that implements it before proposing anything. A change written from the symptom alone fixes the symptom.

**Reuse.** Prefer an existing function, an existing convention, an existing document. A second way to do something already done is a second thing to keep true.

**Minimize.** Add nothing the problem did not require. No option nobody asked for, no abstraction with one caller, no configuration key with one value. Every one of them is a promise to keep.

**Verify.** Prove the change with the gate that would have caught the defect. A change verified only by reading it is a change nobody verified.

## Applied Here

HIPPO holds no defaults about the work it guards, and that is this principle rather than an accident. Every product-specific default — a task runner's flag, a repository's layout, a build tool's name — would be a value this tool cannot verify and cannot retract. See [the vision](../vision/README.md).

The same applies to the exit codes. `73`, `75`, and `78` mean three things and no more; a fourth meaning wedged into an existing code costs every consumer their ability to branch on it. See [public contract](../development/public-contract.md).

## Stopping

Stop after the minimal verified change. Adjacent improvements that the task did not require belong to a later task, stated plainly rather than folded in silently — a pull request that fixes one thing and improves four others cannot be reviewed as either.

Where a genuinely smaller change would leave a rule unenforced or a boundary untested, say so and do the larger one. Minimal is measured against the problem, not against the diff.
