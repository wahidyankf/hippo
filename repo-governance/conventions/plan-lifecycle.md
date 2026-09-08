# Plan Lifecycle

A plan proposes. [`specs/`](../../specs/README.md) records. A plan lives in exactly one stage under [`plans/`](../../plans/README.md) — `ideas/`, `backlogs/`, `in-progress/`, `done/` — and execution updates every affected specification alongside the implementation under [specification maintenance](../development/specification-maintenance.md).

## Authorization

Writing a plan into this repository takes an explicit request. Designing an approach in conversation, or in a harness planning mode, authorizes neither the folder nor the commit that would carry it. See [commit authorization](commit-authorization.md).

## Scope

This repository plans only what it can deliver alone. Work spanning repositories is planned where it is coordinated and arrives here as its own change with its own evidence, for the reason [rules propagation](../workflows/rules-propagation.md) gives: something that crosses a boundary is decided again on the other side, or it arrives without the argument that justified it.

## Ideas

A rough two-pager lives at `plans/ideas/<quadrant>/<slug>.md`, with q1 to q4 chosen from dated evidence of urgency and importance rather than from impression. Search the quadrants first and consolidate overlap. Leave out file-level design, Gherkin, and delivery checklists; those are what a formal plan is for.

## Formal Plans

Kebab-case folders at `plans/backlogs/<slug>/` when queued, `plans/in-progress/<slug>/` when running, and `plans/done/YYYY-MM-DD__<slug>/` when finished. Each holds:

- `README.md` — status, context, scope, approach, dependencies, navigation;
- `brd.md` — goal, roles, outcomes, non-goals, risks;
- `prd.md` — personas, stories, Gherkin acceptance criteria, scope, risks;
- `delivery.md` — ordered tasks, ownership, proof, checkpoints;
- `learnings.md` — approach and transient observations; and
- exactly one technical shape.

The shape is a single `tech-docs.md`, or `tech-docs/README.md` with mapped companions. Keep one document while it stays coherent, split when separate responsibilities each want their own reading order, and collapse a fragment doing no distinct job. Never both shapes, and never an empty companion. Length is a review signal and never a rule. Follow [minimal sufficiency](../principles/minimal-sufficiency.md).

Companions take a two-digit reading-order prefix such as `01-evidence-model.md`, with `README.md` first. Renumber on insertion and order every map by number.

Plans carry no word budget, but they obey [directory maps](directory-maps.md), [Mermaid](markdown-visualizations.md), and [data safety](public-repository-data-safety.md). Write for a junior. Name every affected path exactly, labelled `[E]` edited, `[N]` new, `[M]` moved, or `[D]` deleted; a directory or a glob is not a path.

PRD Gherkin accepts the plan, not the corpus. What becomes a durable scenario is decided under [plan specification changes](plan-specification-changes.md).

## Delivery Ownership

Every executable checkbox carries its acceptance labels and one owner. `[AI]` covers work within available authority and tools; `[HUMAN]` covers only a decision, credential, physical action, or authority no agent has. Prefer `[AI]`, and never use `[HUMAN]` to defer a settled decision or postpone a discovery someone could make now. Split a mixed task. Each names its input, action, outcome, and proof. Every phase ends in a blocking checkpoint.

A checkbox that ships code states its [red-green-refactor](../workflows/red-green-refactor.md) cycle as three checkboxes — RED, GREEN, REFACTOR — each naming the test path, the command, and the failure or pass expected. Never one checkbox, and never prose.

A recovery task carries an explicit trigger and stays dormant until it fires. At reconciliation it takes a dated, evidenced `Not triggered` disposition; a checkmark would claim something ran that did not.

Where execution may change a repository rule, `delivery.md` carries an `[AI]` task that applies [rules propagation](../workflows/rules-propagation.md) and records its result.

## Transitions

Move the folder, never copy it, and change its status and both stage indexes in the same change. Refuse a dated destination that already exists. Archive only once acceptance, verification, learnings, and conditional items are reconciled, then run the repository gate. [Plan execution](../workflows/plan-execution.md) owns the procedure.
