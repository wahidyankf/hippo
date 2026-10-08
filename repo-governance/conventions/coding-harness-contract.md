# Coding Harness Contract

One canonical instruction body, expressed in each harness's own surface. Harnesses do not get their own rules; they get
their own spelling of the same rules.

`./rhino harness adapters validate` compares the complete native adapter tree with the canonical sources and four
profiles declared in [`repo-config.yml`](../../repo-config.yml). Generated catalog and provenance artifacts identify the
exact sources and their digests.

## The Canon

- **Instructions**: root [`AGENTS.md`](../../AGENTS.md). It is the only instruction body. Every other harness-facing
  instruction file is an adapter that routes to it and adds nothing.
- **Skills**: `.agents/skills/<name>/SKILL.md`. Codex and OpenCode read that path natively; Claude receives the
  generated native route.
- **Agents**: `.agents/agents/<name>.md`, carrying its own declaration — its tier, the capabilities it needs, the
  constraints it keeps, and any agents it dispatches.

## Adapters

An adapter exists only where a harness needs a native route or agent representation. It routes and declares; it never
restates the canonical body.

Where a harness translates a capability into its own vocabulary — a tool list or permission map — the translation is
declared in configuration and checked. A denial weakened in an adapter is a finding, not a local preference. Codex 0.161
retains its existing guards; Serena and new destination routing are deferred. Its [release role loader][role] omits MCP
overrides, and native shell hooks omit the requested working directory. No role-local MCP capability enforcement is
claimed.

[role]: https://github.com/openai/codex/blob/979011409de0a60b52f179721948e65531d26144/codex-rs/core/src/agent/role.rs

Adapters are generated from the canon only by `./rhino harness adapters generate`, never by a repository-local generator
or hand edit. The writer and judge stay apart: generation writes the declared transaction, and
`./rhino harness adapters validate` compares its desired output with the files on disk.

## Prohibited Instruction Sources

Files that would compete with the canon remain prohibited. A second instruction file does not add rules; it splits them,
and the reader has no way to know which half they got. The v0.4 adapter validator proves generated-adapter ownership; it
does not claim to scan arbitrary nested instruction filenames, so that broader product check must not be claimed as
adapter-validation evidence.

Changing any of this follows [harness propagation](../workflows/quality/harness-propagation.md).

## Repository Policy

Native policy bindings invoke [`scripts/agent-policy-hook.sh`](../../scripts/agent-policy-hook.sh) for this physical
checkout. Claude Code, OpenCode, and Command Code cross-repository operations use neutral destination routing. OpenCode
Serena registration is deferred; Claude Code and Command Code retain semantic integration. Codex retains its existing
guards; new routing and Serena are deferred. Protected-path data lives in
[`.agents/agent-policy.json`](../../.agents/agent-policy.json); semantic indexing exclusions live in the tracked
[Serena project config](../../.serena/project.yml). Shared MCP and capture integrations remain user-global.
