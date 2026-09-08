# Rules Quality Gate

A review of this tree, run **only on explicit request**. It is not part of any automated gate and never blocks a change on its own.

## Why It Is Requested Rather Than Automatic

Judging whether a rule is well-placed, well-reasoned, and still true is a reading task. Wiring it into every pull request would either produce noise nobody reads, or produce a check that passes on documents nobody read either.

## The Review

For each document:

- **Does it state a rule, or describe a practice?** A description with no obligation belongs in `docs/` or in a specification.
- **Does it give the reason?** A rule without one is followed until it is inconvenient.
- **Is it at the right level?** See [rules](../conventions/rules.md).
- **Does anything below it contradict it?** A lower level may not contradict a higher one.
- **Is it reachable from [`AGENTS.md`](../../AGENTS.md)?** An unreachable rule is an unfollowed rule.
- **Is it enforced, and does it say so?** Where a gate checks it, name the gate. Where none does, say that too.
- **Is it within budget for the right reason?**

## Output

Findings, ordered by how likely each is to cause someone to do the wrong thing. Each names the document, what is wrong, and what would make it right.

Fixes land through [worktree to pull request](worktree-to-pull-request.md) like anything else. The review produces findings; it does not produce commits.

The machine-checkable part — budgets, links, maps, diagrams, harness parity — already runs on every gate. This gate is for what a validator cannot read.
