# Rules

How a rule is written, and where it lives.

## Shape

A rule states what must be true and why. The reason is not decoration: a rule without one survives only until following it is inconvenient, and then it is discarded by someone who could not tell whether it mattered.

Write rules as constraints on outcomes, not as instructions for a particular tool. "Every internal link resolves" outlives the validator that checks it; "run the link checker" does not.

## Placement

Every rule lives in exactly one document, at the level that owns it:

- A durable constraint that shapes other rules is a [principle](../principles/README.md).
- A repository-wide choice is a [convention](../conventions/README.md).
- An engineering standard for changing code is a [development](../development/README.md) rule.
- A repeatable procedure with steps and an order is a [workflow](../workflows/README.md).

Root [`AGENTS.md`](../../AGENTS.md) links to it and states nothing itself. A rule written in two places drifts in one of them, and the reader has no way to tell which.

## Changing One

Change the document. A rule discovered to be wrong is evidence, and working around it silently leaves the next reader to rediscover the same problem with less to go on.

Where a rule is adapted from another repository, say what changed and why, in the document. [Rules propagation](../workflows/rules-propagation.md) stops at this repository's boundary, so divergence is expected; undocumented divergence is not.

## Enforcement

Prefer a rule a gate can check. Where the check exists, name it in the document so a reader can see what would catch a violation. Where none exists, say that too — an unenforced rule is a rule that depends on attention, and the document should not pretend otherwise.
