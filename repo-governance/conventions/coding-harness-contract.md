# Coding Harness Contract

One canonical instruction body, expressed in each harness's own surface. Harnesses do not get their own rules; they get
their own spelling of the same rules.

`./rhino harness adapters validate` compares the complete native adapter tree with the canonical sources and four
profiles declared in [`repo-config.yml`](../../repo-config.yml). Generated catalog and provenance artifacts identify the
exact sources and their digests.

## The Canon

- **Instructions**: root [`AGENTS.md`](../../AGENTS.md). It is the only instruction body. Every other harness-facing
  instruction file is an adapter that routes to it and adds nothing.
- **Skills**: `.agents/skills/<name>/SKILL.md`. Codex, OpenCode, and Command Code read that path natively; Claude
  receives the generated native route.
- **Agents**: `.agents/agents/<name>.md`, carrying its own declaration — its tier, the capabilities it needs, the
  constraints it keeps, and any agents it dispatches.

## Adapters

An adapter exists only where a harness needs a native route or agent representation. It routes and declares; it never
restates the canonical body.

Where a harness translates a capability into its own vocabulary — a tool list or permission map — the translation is
declared in configuration and checked. A denial weakened in an adapter is a finding, not a local preference. Codex's
native TOML agent schema carries its supported name, description, and `developer_instructions` route; the canonical
boundary remains in that routed source when the schema has no agent-scoped permission field.

Adapters are generated from the canon only by `./rhino harness adapters generate`, never by a repository-local generator
or hand edit. The writer and judge stay apart: generation writes the declared transaction, and
`./rhino harness adapters validate` compares its desired output with the files on disk.

## Session Coordination

Command Code reads root `AGENTS.md` and `.agents/skills/` natively. Its profile selects only canonical leaf agents for
`.commandcode/agents/`. Run `swe-orchestrator` in the main session by reading its complete canonical definition and
following its named dispatch allowlist; nested native dispatch is unavailable. This is the exception to native
one-adapter-per-role coverage. It preserves the canonical role and its boundary without granting nested spawning.

Omit Command Code tier mappings and model, featureModels, effort, and reasoningEffort pins from repository profiles,
adapters, settings, shared global sources, and smoke commands. The active session supplies the model; omitted reasoning
fields use the harness default. Retain every documented native tool grant and denial for the selected leaves.

Keep `.commandcode/settings.local.json` and the entire `.commandcode/taste/` tree ignored and local, with taste learning
active. Generation owns only `.commandcode/agents/`, so it cannot replace those files or project settings.

Declared-profile parity verifies the projection; live discovery and main-session-to-leaf probes verify the vendor
behaviour. Semantic main-session compliance remains unenforced by decision because adapter validation cannot judge it.

When canonical agents change, update the explicit profile selection in the same change. It must equal every canonical
leaf, excluding exactly roles declaring `subagent` or nonempty `dispatches`; audit that equality after regeneration.

## Prohibited Instruction Sources

Files that would compete with the canon remain prohibited. A second instruction file does not add rules; it splits them,
and the reader has no way to know which half they got. The v0.4 adapter validator proves generated-adapter ownership; it
does not claim to scan arbitrary nested instruction filenames, so that broader product check must not be claimed as
adapter-validation evidence.

Changing any of this follows [harness propagation](../workflows/quality/harness-propagation.md).
