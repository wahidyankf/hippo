# Software Quality Enforcement

A change is not done when it compiles. It is done when the gates that would have caught its failure have run and passed.

## Required Before Done

- `npm run test:quick` locally, and the pull-request surface over the final head before the pull request opens
  ([landing gate](quality-gates.md#landing-and-ruleset)).
- The `main` surface before a release: its post-merge run passes for the release commit, on both operating systems
  ([In CI](quality-gates.md#in-ci)).
- The scenario or test that was red before the change and is green after it, named — see
  [test-driven development](test-driven-development.md).

## Documentation Hygiene

The pinned RHINO release answers six independent questions: the configuration parses, word budgets hold, directory maps
match the tree, internal links resolve, Mermaid diagrams meet the label and colour rules, and the harness adapter tree
matches the canon. All six are declared gates on the `pull-request` surface, beside `markdown-line-length`, which holds
every Markdown line to 120 characters.

What it enforces lives in [`repo-config.yml`](../../repo-config.yml), not in the tool. **Change the declaration, never
the tool** — a repository that edits its validator to pass is a repository whose validator means nothing.

## Reading a Result

A gate that reports "checked 0 files, no findings" has not passed; it has looked at nothing. Every RHINO command reports
what it inspected for exactly this reason, and a count that drops without explanation is a finding of its own.

A green compliance run is not a green suite. The
[compliance check](behaviour-driven-development.md#what-the-compliance-check-does-not-do) resolves bindings; the
executing adapters run scenarios. Naming which one produced the green is part of reporting the result.

## Reporting

Report outcomes faithfully. If a gate failed, say so and include the output. If a step was skipped, say which and why. A
completion claim that omits a skipped step is worse than an incomplete one, because it stops anyone looking.
