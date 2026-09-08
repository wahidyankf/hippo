# Software Quality Enforcement

A change is not done when it compiles. It is done when the gates that would have caught its failure have run and passed.

## Required Before Done

- The quick gate, locally, on the change as it will be pushed.
- The full gate before a release, and in CI on every supported platform.
- The scenario or test that was red before the change and is green after it, named — see [test-driven development](test-driven-development.md).

## Documentation Hygiene

`scripts/docs-check.sh` runs the pinned RHINO release over six independent questions: the configuration parses, word budgets hold, directory maps match the tree, internal links resolve, diagrams stay legible, and the harness roster is in parity with the canon.

What it enforces lives in [`repo-config.yml`](../../repo-config.yml), not in the tool. **Change the declaration, never the tool** — a repository that edits its validator to pass is a repository whose validator means nothing.

## Reading a Result

A gate that reports "checked 0 files, no findings" has not passed; it has looked at nothing. Every RHINO command reports what it inspected for exactly this reason, and a count that drops without explanation is a finding of its own.

A green compliance run is not a green suite. `tests/bdd` resolves bindings; the executing adapters run scenarios. Naming which one produced the green is part of reporting the result.

## Reporting

Report outcomes faithfully. If a gate failed, say so and include the output. If a step was skipped, say which and why. A completion claim that omits a skipped step is worse than an incomplete one, because it stops anyone looking.
