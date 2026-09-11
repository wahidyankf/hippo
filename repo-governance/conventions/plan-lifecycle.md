# Plan Lifecycle — Local Rules

The plan contract itself is the [plans convention](plans.md) and its [modules](plans/README.md): lifecycle folders, the six documents, the technical shape, the delivery grammar, validation, evidence, and archival. That document is portable on purpose and names no repository.

This file holds the part that is HIPPO's, and would not be true of another repository.

## A Plan Proposes; `specs/` Records

[`specs/`](../../specs/README.md) is the canonical description of the tool. Where a plan and a specification disagree the specification is right, and execution updates every affected specification alongside the implementation under [specification maintenance](../development/specification-maintenance.md).

## Authorization

Writing a plan into this repository takes an explicit request. Designing an approach in conversation, or in a harness planning mode, authorizes neither the folder nor the commit that would carry it. See [commit authorization](commit-authorization.md).

## Scope

This repository plans only what it can deliver alone. Work spanning repositories is planned where it is coordinated and arrives here as its own change with its own evidence, for the reason [rules propagation](../workflows/rules-propagation.md) gives: something that crosses a boundary is decided again on the other side, or it arrives without the argument that justified it.

## Ideas Are Filed by Quadrant

A rough two-pager lives at `plans/ideas/<quadrant>/<slug>.md`, with q1 to q4 chosen from dated evidence of urgency and importance rather than from impression. Search the quadrants first and consolidate overlap. The plans convention leaves the idea folder's internal shape to the repository; this is the shape HIPPO chose, and it is the one difference between the two.

## Delivery Items That Ship Code

A checkbox that ships code states its [red-green-refactor](../workflows/red-green-refactor.md) cycle as three checkboxes — RED, GREEN, REFACTOR — each naming the test path, the command, and the failure or pass expected. Never one checkbox, and never prose.

A recovery task carries an explicit trigger and stays dormant until it fires. At reconciliation it takes a dated, evidenced `Not triggered` disposition; a checkmark would claim something ran that did not.

Where execution may change a repository rule, `delivery.md` carries an `[AI]` task that applies [rules propagation](../workflows/rules-propagation.md) and records its result.
