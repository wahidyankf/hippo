---
description: >-
  Records how this repository adopted the shared quality-gate contract: its families, callers, deterministic tools,
  local owners, and every deviation from the copied text, with its reason.
when_to_use: >-
  Use when running, adopting, or changing a quality gate or propagation here, or when an adopted gate names an owner
  this repository does not hold.
---

# Quality Gate Adapter

The [Quality Gate Contract](quality-gate-contract.md), [Sole-Writer Propagation](sole-writer-propagation.md), and each
family's gate and propagation were copied from the shared catalog at its paths, so their links resolve unchanged. A
stronger local rule wins over an adopted one, and every difference is recorded here with its reason.

## Families

| Family      | Subject here                                                     |
| ----------- | ---------------------------------------------------------------- |
| `plan`      | one plan folder under `plans/`                                   |
| `docs`      | every `README.md`, `docs/`, `specs/`, and `CHANGELOG.md`         |
| `rules`     | `AGENTS.md`, `repo-governance/`, agents, and skills              |
| `harness`   | the Claude Code, Codex, and OpenCode bindings                    |
| `ci`        | `.github/workflows/`, `.husky/`, and the `repo-config.yml` gates |
| `pr-review` | one pull request                                                 |
| `specs`     | listed folders under `specs/`                                    |

Not adopted: `ui-web` and `api-http`, because the product is a command-line guard with no web or HTTP surface, and the
content, `pdf-to-md`, and tutorial families, because the repository publishes no such content.

## Local Owners

An adopted document links the local owner wherever the catalog linked its own: plans, documentation architecture,
directory maps (for indexes and word budgets), the [governance hierarchy](../../README.md) (for governance layers),
rules, the working tree (for temporary files), the coding-harness contract (for vendor-neutral governance and harness
adapters), decision gates, software quality enforcement, quality gates (for automated quality gates), behaviour-driven
development, end-to-end testing, test-driven development, architecture specifications, no destructive Git operations,
upstream tool defects, GitHub Actions storage, specification maintenance (for the specification tree), and minimal
sufficiency.

A catalog owner with no local counterpart stays named without a link: Bounded Convergence, Finding Criticality and
Confidence (the `assessing-criticality-confidence` skill carries its scales), the review disciplines, Web Research
Delegation, Deterministic and Judgement Validation, Preexisting Error Resolution, Related Repositories, the writing
conventions, and the principles this tree does not hold. Bounded Convergence has no local standard: each repeated
operation here states its own bound.

## Adopter Decisions

- **Callers.** `plan-planning` calls the plan gate and `release-cut` the docs gate. Rules grooming never calls the rules
  gate. Each gate's Entry lists only these callers.
- **Entry and exit check.** The `pre-push` surface in `repo-config.yml`, run with `./rhino gate run --surface pre-push`:
  the quick gate and every documentation validator.
- **Structure.** `./rhino governance quality-gates validate`, on the pre-push and pull-request surfaces, checks every
  gate and propagation for its headings, verdicts, retired inputs, and cycle bound against `repo-config.yml`.
- **Deterministic Boundary.** Each gate's last column names the tools those surfaces run. No validator checks front
  matter, heading levels, file names, or emoji here, so those rows left every table and the properties are judgeable. No
  plan structural validator runs here either, so the plan gate judges what Structural Validation lists.
- **Review surface.** A hosted pull request, under the merge preconditions.
- **Review route.** No scout or lens checker is adopted, so every pass takes the trivial tier: `pr-review-checker`
  reviews the change alone.
- **Agent fields.** Catalog `capabilities` become `requires` and `denies`, `read-only` becomes a denied
  `repository-write` with `inline-result-only`, and `tier: execution` is dropped, since only `plan` is declared. No
  harness here declares `network`, so `docs-checker` returns each outside-world research need to its caller.
- **Skills not adopted.** `authoring-documentation`, `propagating-rules`, `checking-harness-compatibility`,
  `applying-ci-standards`, `validating-specification-structure`, `resolving-review-threads`,
  `synthesizing-review-findings`, `producing-review-findings`, `validating-factual-accuracy`, `validating-links`,
  `validating-governance-rules`, and `understanding-governance-architecture`. Each executor works from its workflow and
  agent text.
- **Docs propagation.** A specification that disagrees with the implementation is asked through `grill-me`, under
  last-resort questions.
- **Rules propagation.** It stops at this repository's boundary, as the governance README records. It records no sibling
  obligation, and a catalog proposal is recorded for the owner, never delivered by the run.
- **Harness propagation.** It keeps its earlier automatic trigger, every canonical change, as its
  [Canonical Change](../../workflows/quality/harness-propagation/001-canonical-change.md) module.
- **Release cut.** A docs row where the binary breaks the documented contract is fixed in code before the tag.
- **Upstream tool defects.** A bug-fix plan executes after any plan gate verdict, since every verdict is advisory.
- **Shape.** Adopted workflows drop front matter, as every workflow here does, while these standards keep it. A table
  wider than the 120-column line limit becomes a list, and a module folder's table folds into its directory map.
- **Plan gate.** The plans convention it audits includes the local rules and specification changes `AGENTS.md` lists.
- **Single-pass reviews.** Exploratory and usability review, Gherkin implementation review, harness parity verification,
  and the leak review judge once and stay reviews.
