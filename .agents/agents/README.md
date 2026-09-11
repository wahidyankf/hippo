# Agents

Canonical agent definitions. Each declares its identity, the capabilities it requires, the ones it denies itself, and
nothing about any particular harness. The per-harness wrappers under `.claude/`, `.codex/` and `.opencode/` are
generated from these by `node scripts/generate-adapters.mjs` and are never edited in place.

## Planning

- [plan-maker](plan-maker.md) — authors a plan, runs both decision gates, and repairs its own draft.
- [plan-checker](plan-checker.md) — audits a frozen draft and reports; changes nothing.
- [plan-execution-checker](plan-execution-checker.md) — audits finished execution and permits or blocks archival.

There is no fixer. The maker validates the checker's findings and applies the ones that hold.

## This Repository

- [gherkin-implementation-reviewer](gherkin-implementation-reviewer.md) — reviews whether a changed scenario would
  actually fail.
