# Agents

Canonical agent definitions. Each declares its identity, the capabilities it requires, the ones it denies itself, and
nothing about any particular harness. The per-harness wrappers under `.claude/`, `.codex/`, and `.opencode/` are
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

Adopted from the shared catalog. The maker loads each project's stack skill and standard on demand from the inventory;
see the [repository adapter](../../repo-governance/development/quality/stacks/repository-adapter.md).

- [swe-code-maker](swe-code-maker.md) — builds behaviour test-first in the projects named.
- [swe-code-checker](swe-code-checker.md) — audits code against the adopted standards and reports; changes nothing.
- [swe-code-fixer](swe-code-fixer.md) — applies re-validated checker findings and records every disposition.

## This Repository

- [gherkin-implementation-reviewer](gherkin-implementation-reviewer.md) — reviews whether a changed scenario would
  actually fail.
