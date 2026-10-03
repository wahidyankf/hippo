---
description: >-
  Indexes this repository's canonical agent definitions, each declaring what it needs before any harness adapter
  translates that declaration.
when_to_use: >-
  Use when locating a canonical agent definition or deciding what a new one must declare.
---

# Agents

Canonical agent definitions. Each declares its identity, its tier, the capabilities it needs, the constraints it keeps,
and nothing about any particular harness. The per-harness wrappers under `.claude/`, `.codex/`, and `.opencode/` are
generated from these only by `./rhino harness adapters generate` and are never edited in place; see the
[coding harness contract](../../repo-governance/conventions/coding-harness-contract.md).

## Planning

- [plan-maker](plan-maker.md) — authors a plan, runs both decision gates, and submits it to the plan quality gate.
- [plan-checker](plan-checker.md) — audits a frozen draft and reports; changes nothing.
- [plan-fixer](plan-fixer.md) — repairs only the rows of a frozen plan quality ledger.
- [plan-execution-checker](plan-execution-checker.md) — audits finished execution and permits or blocks archival.

## Quality Gates

One checker and one fixer per gate family, under the
[quality gate contract](../../repo-governance/development/workflow/quality-gate-contract.md). Each checker reports and
changes nothing; each fixer runs its family's propagation on a frozen ledger.

- [docs-checker](docs-checker.md) and [docs-fixer](docs-fixer.md) — human-facing documents.
- [rules-checker](rules-checker.md) and [rules-fixer](rules-fixer.md) — the rules in `AGENTS.md` and `repo-governance/`.
- [harness-checker](harness-checker.md) and [harness-fixer](harness-fixer.md) — harness bindings against upstream
  conventions.
- [ci-checker](ci-checker.md) and [ci-fixer](ci-fixer.md) — hook and pipeline wiring.
- [specs-checker](specs-checker.md) and [specs-fixer](specs-fixer.md) — specification folders.
- [pr-review-checker](pr-review-checker.md) and [pr-review-fixer](pr-review-fixer.md) — one pull request.

## Software Development

Adopted from the shared catalog. Each loads a project's stack skill and standard on demand from the inventory; see the
[repository adapter](../../repo-governance/development/quality/stacks/repository-adapter.md).

- [swe-architect](swe-architect.md) — designing boundaries before a build, reviewing it after, and the architecture
  review lens
- [swe-debugger](swe-debugger.md) — repairing failing type checks, lint, and tests at the cause
- [swe-developer](swe-developer.md) — building behaviour test-first and applying re-validated findings
- [swe-orchestrator](swe-orchestrator.md) — decomposing a deterministic goal and dispatching the swe family until its
  checks pass
- [swe-releaser](swe-releaser.md) — cutting releases, deploying artifacts, and repinning tools through documented
  workflows
- [swe-reviewer](swe-reviewer.md) — auditing code, component source, and scenario bindings against adopted standards
- [swe-usability-tester](swe-usability-tester.md) — judging first use of a live web interface or command-line tool
  without its specifications

## Old-to-New Map

Each agent the swe family replaced, and where its work went. A reader holding an old name finds its replacement here.

| Replaced agent                    | New agent              | Mode or charter   |
| --------------------------------- | ---------------------- | ----------------- |
| `swe-code-maker`                  | `swe-developer`        | build             |
| `swe-ui-maker`                    | `swe-developer`        | build (UI skills) |
| `swe-code-fixer`, `swe-ui-fixer`  | `swe-developer`        | apply findings    |
| `ui-web-fixer`, `api-http-fixer`  | `swe-developer`        | apply findings    |
| `bugs-solver`                     | `swe-debugger`         | —                 |
| `swe-code-checker`                | `swe-reviewer`         | code              |
| `swe-ui-checker`                  | `swe-reviewer`         | interface         |
| `gherkin-implementation-reviewer` | `swe-reviewer`         | scenario trace    |
| `ui-web-checker`                  | `swe-web-tester`       | spec              |
| `web-design-tester`               | `swe-web-tester`       | design            |
| `web-exploratory-tester`          | `swe-web-tester`       | exploratory       |
| `web-usability-tester`            | `swe-usability-tester` | —                 |
| `api-http-checker`                | `swe-api-tester`       | contract          |
| `api-exploratory-tester`          | `swe-api-tester`       | exploratory       |
| `pr-review-architecture-checker`  | `swe-architect`        | lens              |
| `apps-*-deployer` (per app)       | `swe-releaser`         | deploy            |
