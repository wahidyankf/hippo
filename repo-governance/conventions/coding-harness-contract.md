# Coding Harness Contract

One canonical instruction body, expressed in each harness's own surface. Harnesses do not get their own rules; they get their own spelling of the same rules.

`rhino harness parity validate` reconciles the canon against every harness declared in [`repo-config.yml`](../../repo-config.yml), and reports a digest so "nothing changed" is distinguishable from "nothing was checked".

## The Canon

- **Instructions**: root [`AGENTS.md`](../../AGENTS.md). It is the only instruction body. Every other harness-facing instruction file is an adapter that routes to it and adds nothing.
- **Skills**: `.agents/skills/<name>/SKILL.md`. Codex and OpenCode read that path natively, so for them the canon _is_ the surface and no adapter exists.
- **Agents**: `.agents/agents/<name>.md`, carrying its own capability declaration — what it requires, what it denies, what constrains it.

## Adapters

An adapter exists only where a harness cannot read the canon. It routes and declares; it never restates. A wrapper that copied the canonical body would be a second copy to keep true, which is the failure this contract exists to prevent, so the validator refuses it.

Where a harness translates a capability into its own vocabulary — a tool list, a permission map, a sandbox mode — the translation is declared in configuration and checked. A denial weakened in an adapter is a finding, not a local preference.

## Prohibited Instruction Sources

Files that would compete with the canon are prohibited by name, and the prohibition is checked. A second instruction file does not add rules; it splits them, and the reader has no way to know which half they got.

Changing any of this follows [the harness contract change workflow](../workflows/coding-harness-contract-change.md).
