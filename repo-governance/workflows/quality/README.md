# Quality Workflows

Every quality gate with its propagation, and every single-pass review.

## Directory Map

- [Docs propagation](docs-propagation.md) — carrying a change into every human-facing document it affects.
- [Docs quality gate](docs-quality-gate.md) — auditing the human-facing documents, on request or at a release.
- [Exploratory and usability review](exploratory-usability-review.md) — a bounded exploration, recorded with its
  evidence.
- [Gherkin implementation review](gherkin-implementation-review.md) — the manual review a changed scenario or adapter
  requires.
- [Harness parity verification](harness-parity-verification.md) — proving the roster still reconciles.
- [Harness propagation](harness-propagation.md) — changing the canon or a harness adapter.
- [Plan quality gate](plan-quality-gate.md) — one plan's readiness, on explicit request only; for a bug-fix plan, an
  adopted [upstream tool defects](../../development/upstream-tool-defects.md) standard is that request.
- [PR leak review](pr-leak-review.md) — the private review every push requires, and the posted, current-head review a
  merge requires.
- [PR leak review modules](pr-leak-review/README.md) — leak classes, the push review, and how both are enforced.
- [Red green refactor](red-green-refactor.md) — the cycle, with its evidence.
- [Rules propagation](rules-propagation.md) — carrying a rule change through this repository, and where it stops.
- [Rules quality gate](rules-quality-gate.md) — reviewing this tree on request.
