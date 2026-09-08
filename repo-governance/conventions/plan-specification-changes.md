# Plan Specification Changes

Use this when a formal plan changes observable behaviour, the command surface, an exit code, configuration compatibility, or the architecture. It makes the specification work concrete enough for a junior engineer to do before implementation starts, instead of discovering it in the middle.

## Where It Lives

In an unsplit plan, one section of `tech-docs.md` owns the planned specification work. In a split set, use a mapped `tech-docs/specification-changes.md` when that work is a distinct reader's job. Either way list every affected C4 or Gherkin file by exact repository-relative path, labelled `[E]` edited, `[N]` new, `[M]` moved, or `[D]` deleted.

## What Becomes a Contract

PRD Gherkin is plan-level acceptance language. It is not a standing request to copy every scenario into [`specs/`](../../specs/README.md). Before the file list, say which PRD outcomes become durable scenarios and which stay plan-only. Each plan-only outcome gets its reason and the exact `delivery.md` task that verifies it; each selected outcome gets its target specification file below.

Know what the change costs a consumer before proposing it. The exit codes, the evidence readers, the command surface, and configuration compatibility are [a public contract](../development/public-contract.md), and none of them moves without authorization. A plan proposing to move one is proposing a release decision, and it says so in those words.

## Per File

One heading per file with nested bullets, rather than a wide table. Put the planned delta in a fenced `diff` block — `-` for current or removed behaviour, `+` for resulting or added — and keep `= Preserve`, `→ Bindings`, and `✓ Proof` as ordinary bullets beneath it. A long scenario list goes into a collapsed `<details>` block directly below the diff. An `[N]` file uses `+` only, a `[D]` file `-` only, an `[M]` file shows both paths.

For each Gherkin file, state:

- every existing scenario to preserve, update, move, or delete, by name, and the observable behaviour that results;
- every new scenario by name, with its actor, preconditions, action, and expected outcome;
- the exact binding and support paths that change for it across the three executing boundaries of [behaviour-driven development](../development/behaviour-driven-development.md), or the specific boundary that cannot carry it and why; and
- the command that proves the changed corpus.

For each C4 file, name the exact view, node, relationship, or constraint that changes and why, under [architecture specifications](../development/architecture-specifications.md).

## Proposed Now, As-Built Later

The proposal stays in the plan. `specs/` receives only the final as-built result, during execution, under [specification maintenance](../development/specification-maintenance.md). The implementation phase of `delivery.md` carries an `[AI]` task naming that canonical path and the affected elements, whose outcome is synchronized as-built content and whose proof is the architecture and specification gates. Do not sweep every C4 update into one documentation task at the end: that is precisely how a model and the code it claims to describe come apart.

## File Impact

The plan lists every expected code, test, specification, documentation, and configuration path exactly. A directory, an ellipsis, a glob, or a described area is not a path. Where an unmade human decision blocks naming a file the work needs, make that decision a prerequisite and block execution on it rather than hiding it inside a tree.

Group a large tree into short area-specific blocks with aligned, concise annotations. Detailed behaviour belongs in the specification and architecture sections above, not in a horizontal list no one can read.
