# Quality Workflows

Every quality gate with its propagation, and every single-pass review.

## Directory Map

- [CI propagation](ci-propagation.md) — the `ci` family's writer: pipeline definitions and hook wiring.
- [CI quality gate](ci-quality-gate.md) — judges pipeline definitions and hook wiring against the pipeline standards.
- [Docs propagation](docs-propagation.md) — the `docs` family's writer, run automatically with each change a document
  describes.
- [Docs quality gate](docs-quality-gate.md) — judges documents for stale, obsolete, misplaced, and unreadable content.
- [Exploratory and usability review](exploratory-usability-review.md) — a bounded exploration, recorded with its
  evidence.
- [Gherkin implementation review](gherkin-implementation-review.md) — semantic review of what a binding actually
  asserts.
- [Harness parity verification](harness-parity-verification.md) — the read-only audit of the coding-harness contract.
- [Harness propagation](harness-propagation.md) — the `harness` family's writer, and every canonical change.
- [Harness propagation modules](harness-propagation/README.md) — the canonical change procedure.
- [Harness quality gate](harness-quality-gate.md) — judges the harness bindings for upstream drift.
- [Plan propagation](plan-propagation.md) — the `plan` family's writer, inside one plan folder.
- [Plan quality gate](plan-quality-gate.md) — judges a complete plan draft for what structural validation cannot reach.
- [PR leak review](pr-leak-review.md) — the private review before every push and the posted, current-head review a merge
  requires.
- [PR leak review modules](pr-leak-review/README.md) — leak classes, the push review, and enforcement, in reading order.
- [PR review](pr-review.md) — one semantic review pass of a pinned pull-request head.
- [PR review propagation](pr-review-propagation.md) — the `pr-review` family's writer: answers every blocking finding.
- [PR review quality gate](pr-review-quality-gate.md) — iterative review with repairs on one pull request.
- [PR review quality gate modules](pr-review-quality-gate/README.md) — review surface, answering findings, and the
  ceiling.
- [Red, green, refactor](red-green-refactor.md) — the executable form of the test-driven cycle.
- [Rules propagation](rules-propagation.md) — the `rules` family's writer, run automatically for every rule change.
- [Rules propagation modules](rules-propagation/README.md) — statement and conflict, placement, enforcement.
- [Rules quality gate](rules-quality-gate.md) — judges one proposed or effective rule state.
- [Specs propagation](specs-propagation.md) — the `specs` family's writer, inside the listed folders.
- [Specs quality gate](specs-quality-gate.md) — judges listed specification folders for structure and alignment.
