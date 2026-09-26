# Docs Quality Gate

An audit of the human-facing documents, run **only on explicit request** or by [release cut](release-cut.md). It never edits a document and never blocks a change on its own; a change or a [docs propagation](docs-propagation.md) run never authorizes it.

## Why It Is Requested Rather Than Automatic

Judging whether a document is still true, still needed, and still readable is a reading task. Wired into every change, it produces noise nobody reads or a pass nobody earned, and propagation already carries each change into its documents.

## Inputs

- **Scope**, `change` or `all`: the documents one change affects, or the whole document set [docs propagation](docs-propagation.md#the-document-set) defines.
- **Change**, required under `change`: the revision range or working-tree change.

Freeze the scope, the Git revision, and the dirty paths for each audit. A material change to them during that audit ends it as `input-changed`, with its ledger kept; it never restarts. The next audit in the loop below takes a fresh snapshot.

## The Audit

Under `change`, audit the documents the change touches and every document citing what it changed; under `all`, the whole set. For each document, without editing it:

- **Is every claim true** to the implementation, and was every command shown executed against the current build or marked as not exercised? See [documentation architecture](../conventions/documentation-architecture.md#truth).
- **Does it still describe something the repository has?** If not, it is obsolete and its resolution is removal.
- **Does each fact have one home**, with a summary above its detail per [progressive disclosure](../principles/progressive-disclosure.md), and does each `docs/` page serve one Diátaxis mode?
- **Can a newcomer use it?** From the opening they learn what it is and why it matters, and they find the next step. Judged by reading, never by a score.
- **Can the setup be followed as written?** Under `all`, or when setup changed, a reader with no prior context follows it from a clean checkout, marking each step smooth, frustrating, or blocking.
- **Does it agree with its specification?** [`specs/`](../../specs/README.md) is canonical.

The machine-checkable part — budgets, links, maps, diagrams, harness parity — already runs on every gate under [documentation hygiene](../development/software-quality-enforcement.md#documentation-hygiene). The audit consumes that result rather than repeating it.

## Output

A finite ledger, written under `generated-reports/` per [working tree](../conventions/working-tree.md). Each row names the document, the gap, the resolution — update, move, or remove — the evidence, and a status: `open`, `resolved`, `not-applicable` with evidence, or `blocked`. Admit only a document that is wrong, obsolete, unreachable, or unusable by a newcomer; a wording preference is not a finding, per [minimal sufficiency](../principles/minimal-sufficiency.md).

An audit's verdict is `clear` when its ledger holds no open finding, otherwise `needs-propagation`. A finding only the owner can decide, such as a specification that disagrees with the implementation, is asked through [grill-me](../../.agents/skills/grill-me/SKILL.md). A verdict authorizes neither commit nor push.

## After a Finding: Repair to Zero Findings

A requested run is a bounded loop, not one verdict:

1. **Audit** read-only and record the ledger.
2. **Repair.** On `needs-propagation`, the caller runs [docs propagation](docs-propagation.md) with the ledger, without another request. Propagation stays the sole writer, and its repairs land like any other change.
3. **Re-audit** the effective state with the same scope and a fresh snapshot of revision and dirty paths.

The run ends:

- `pass` after **two consecutive clear audits**. One clear audit is not enough, because each deeper read of a document set has found what the previous one missed.
- `partial` at the ceiling of **seven audits**, or as soon as an audit's open findings fail to strictly decrease from the previous one's; the confirming clear audit is the one exception. Every remaining finding gets a durable owner: fixed, filed as an idea or backlog plan, or asked through grill-me.
- `input-changed` when the frozen inputs change for any reason other than this run's own repairs, with its ledgers kept.
- `fail` when an audit or a repair cannot be carried out.

The count is nonnegative and must strictly decrease, and the ceiling bounds it, so the loop terminates. Each audit's ledger and open-finding count are recorded under `generated-reports/`.

The rejected alternative was verdict only: one audit, one hand-off, no re-audit. Before a release, three successive requested runs found fifteen, four, and ten findings, and verdict only gave none of them an end. This gate therefore departs from the [rules quality gate](rules-quality-gate.md), which still hands its findings to propagation once.
