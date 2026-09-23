# Docs Quality Gate

An audit of the human-facing documents, run **only on explicit request** or by [release cut](release-cut.md). It never edits a document and never blocks a change on its own; a change or a [docs propagation](docs-propagation.md) run never authorizes it.

## Why It Is Requested Rather Than Automatic

Judging whether a document is still true, still needed, and still readable is a reading task. Wired into every change, it produces noise nobody reads or a pass nobody earned, and propagation already carries each change into its documents.

## Inputs

- **Scope**, `change` or `all`: the documents one change affects, or the whole document set [docs propagation](docs-propagation.md#the-document-set) defines.
- **Change**, required under `change`: the revision range or working-tree change.

Freeze the scope, the Git revision, and the dirty paths. A material change to them ends the audit as `input-changed`, with its ledger kept; it never restarts.

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

The verdict is `pass` when the ledger is clear and the gate passes, otherwise `needs-propagation` or `input-changed`. `needs-propagation` is a handoff, not a blocked result: the caller runs [docs propagation](docs-propagation.md) with the ledger without another request. A finding only the owner can decide, such as a specification that disagrees with the implementation, is asked through [grill-me](../../.agents/skills/grill-me/SKILL.md). A verdict authorizes neither commit nor push.

## After a Finding: Verdict Only

This repository records **verdict only**: the caller reports propagation's result, and the gate does not run again. A second audit needs a second request. That matches the [rules quality gate](rules-quality-gate.md), which hands its findings to its propagation workflow once.

The rejected alternative is repair to zero findings: propagation repairs, then the gate audits the effective state again while open findings strictly decrease, under a declared ceiling. It ends on a clean audit, but costs a repeated reading audit per round for a repository whose document set is small. Either way the gate itself never edits a document.
