# Docs Propagation

Carrying one change into every human-facing document it affects, in one bounded pass: stale facts corrected, obsolete documents removed, each fact kept in its one home, and the result readable by someone new to the repository.

Apply it automatically before committing any change that alters what a document's reader relies on, when a document is added, moved, or deleted, or when an explicitly requested [docs quality gate](docs-quality-gate.md) hands over findings. No separate request is needed; whoever makes the change runs it as part of the work. Edits made inside one run do not start another.

Propagation is the sole writer of documents. The gate finds; only this workflow edits, so every document edit is made in one bounded place.

## The Document Set

Every human-facing document: [`README.md`](../../README.md) and every other README outside this governance tree, [`docs/`](../../docs/README.md), [`specs/`](../../specs/README.md), [`CHANGELOG.md`](../../CHANGELOG.md), and a plan's documents where they describe the repository.

Governance and agent instructions — this tree, [`AGENTS.md`](../../AGENTS.md), and `.agents/` — stay with [rules propagation](rules-propagation.md). Formatting, links, maps, diagrams, and word budgets stay with the gates that already check them; this workflow runs those gates and adds none.

## Inputs

Freeze the change (a revision range or working-tree change), any handed-over findings, the Git revision, and the dirty paths. A material change to them ends the run as `input-changed`; it never restarts it.

## Procedure

1. **Find what went stale.** Search the whole document set for every name, path, command, flag, exit status, error code, and configuration key the change removed, renamed, or redefined. Each handed-over finding is an item too.
2. **Remove what is obsolete.** A document describing something the repository no longer has is deleted, together with every link and index entry pointing at it. Meaning that is unique and still true moves to its canonical home first.
3. **Keep each fact in its one home.** The root README orients; a `docs/` page serves exactly one Diátaxis mode, per [documentation architecture](../conventions/documentation-architecture.md); a summary links one level down to its detail, per [progressive disclosure](../principles/progressive-disclosure.md). A fact that already has a home is linked, never copied — a copy drifts, and the reader cannot tell which one is right.
4. **Write for a newcomer.** Each affected document says what it is and why it matters from its opening, shows the next step without assuming the layout, and leaves no undefined term or skipped prerequisite. Judge that by reading, never by a readability score. A sparing semantic emoji may aid scanning; decoration never does.
5. **Run what is safe to run.** Execute every command and transcript an affected document shows against the current build, per [documentation architecture](../conventions/documentation-architecture.md#truth). Never run one that touches a production or shared system, publishes, spends, needs a secret, or cannot be undone; the document says plainly that it was not exercised.
6. **Treat specifications as canonical.** Refresh their readability, navigation, and links; their behaviour changes only under [specification maintenance](../development/specification-maintenance.md). Where one disagrees with the implementation, the partial outcome below applies.
7. **Change only what is stale, missing, or obsolete.** Never rewrite accurate prose, invent behaviour, or fold in unrelated work. A released `CHANGELOG.md` entry is history, not a stale fact.
8. **Verify once.** Run the quick gate, which the `pre-push` hook runs too — see [quality gates](../development/quality-gates.md):

   ```sh
   npm run test:quick
   ```

   Repair only failures this run caused, and rerun only while their count strictly decreases and no new failure class appears. The count is nonnegative and decreasing, so the repair terminates.

9. **Land it with the change it explains,** per [thematic commits](../conventions/thematic-commits.md). Repairs for a handed-over ledger land as their own commit. This workflow authorizes neither commit nor push — see [commit authorization](../conventions/commit-authorization.md).

## Terminal Contract

The only results are `no-change`, `landed`, `partial`, and `input-changed`, reported with the documents updated, the documents removed, and each command left unexecuted with the reason.

`partial`: where the code, a specification, or the intended reader is ambiguous or they disagree, that document stays unchanged and the owner is asked through [grill-me](../../.agents/skills/grill-me/SKILL.md); the rest lands. With unchanged inputs and repository state, another run produces no diff.
