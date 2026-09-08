# Plan Quality Gate

Run this only when the user names this gate or unambiguously directs its semantic audit. Authorization is never inferred from creating, editing, reviewing, or executing a plan, from a harness planning mode, or another workflow. One instruction may authorize several named checkpoints; otherwise it authorizes one run.

A run returns one terminal result — `PASS`, or one `BLOCKED_*` variant — for one plan's readiness at the directed pre-execution, post-material-change, or completion checkpoint. It never recurses and never starts another run.

## What It Judges

Meaning, consistency, safety, executability, and proof. `PASS` means good enough for the authorized scope, its known risks, and applicable rules — not perfect, and not future-proof. Do not block on style, speculative hardening, or an improvement that could wait without leaving execution unsafe or ambiguous. Apply [minimal sufficiency](../principles/minimal-sufficiency.md).

Machine-decidable questions belong to the tooling: links, directory maps, word budgets, Mermaid, harness parity. Do not re-derive them by reading or second-guess a verdict they returned; run them once, in verification. Where the plan _delivers_ a check, confirm `delivery.md` has a task that builds it and one that proves it, rather than simulating a tool that does not exist yet; at completion it must exist and pass.

## Snapshot and Ledger

Freeze the plan path and stage, Git revision and dirty paths, scope, relevant specification and governance paths, unresolved decisions, and cycle `1`. Carry it through compaction or handoff under [governance continuity](../principles/governance-continuity.md). A material external input change ends the run as `BLOCKED_INPUT_CHANGED`; it never restarts one.

Audit before editing. Build one finite ledger whose rows carry an ID, canonical rule, location, material gap, required repair, proof, and a status of `OPEN`, `FIXED`, `NOT_APPLICABLE`, or `BLOCKED`. A row exists only where a gap violates a rule or leaves scoped execution unsafe, ambiguous, or unprovable. A mandatory finding cannot be waived, and `NOT_APPLICABLE` needs evidence.

## Procedure

1. Inventory and read the plan, its assets, relevant implementation and specifications, and the governance behind them. Do not audit a machine-owned concern.
2. Complete one semantic audit, editing nothing. Check the [plan lifecycle](../conventions/plan-lifecycle.md) contract — one stage, required documents, one technical shape, truthful status; a route from BRD and PRD through the technical set into delivery that a junior could walk; necessary, non-placeholder artifacts; architecture, Gherkin, file impact, and dependencies synchronized against [software quality enforcement](../development/software-quality-enforcement.md); ownership, acceptance traceability, RED, GREEN, and REFACTOR tasks, checkpoints, evidence; the [specification-change](../conventions/plan-specification-changes.md) contract; and conflicts with current specifications, governance, implementation, or another live plan.
3. Freeze the ledger. Repair only its rows, in dependency and safety order, each closing one `OPEN` row without widening product scope. A missing decision, missing authority, or irreconcilable rule becomes `BLOCKED`; never invent the answer.
4. Verify semantically in read-only mode, reviewing only repaired meaning and its cross-document effects. Then run the gate:

   ```sh
   npm run test:quick
   ```

   It runs unguarded, because [HIPPO cannot guard HIPPO](../development/resource-aware-development.md).

5. Return `PASS` when no row is `OPEN` or `BLOCKED`, the gate passes, no new semantic gap appeared, and the snapshot moved only through recorded repairs.
6. Otherwise take exactly one stabilization cycle: add only repair-caused semantic gaps and deterministic findings, set cycle `2`, repair them once, and repeat step 4. A fixed finding cannot reopen without changed input, which yields `BLOCKED_INPUT_CHANGED`.
7. After cycle `2`, return `PASS` if step 5 now holds. Otherwise return `BLOCKED_NON_CONVERGENT` with the remaining rows and evidence. Do not repair again, restart, or invoke this workflow automatically.

Where the gate reaches no deterministic verdict, return `BLOCKED_TOOLING` with the failure evidence. Never simulate the check or retry it unbounded.

## Terminal Contract

`PASS` authorizes neither execution nor commit. Every `BLOCKED_*` result names its reason, remaining rows, and the external change needed. Resume only when new input and an explicit direction authorize a fresh run. [Plan execution](plan-execution.md) consumes this result and never starts it.
