# Complete Fixture Isolation and Executable Proofs

Preserve the unfinished fixture investigation as future work, separate from completed Command Code support.

Filed 2026-10-10 from the Command Code follow-up investigation. The owner explicitly deferred implementation to ideas
and considered the original Command Code task done. Q2 reflects the importance of the obligations without a deadline.

## Problem and Evidence

The owner accepted three standards during the investigation: **Test Doubles**, **Test Data Isolation**, and **Git
Fixture Isolation**. The resulting migration is unfinished. Focused passing tests establish useful local behaviour, but
do not establish compliance across every resource, executable producer, caller, or writable Git invocation. Deferral
changes the delivery scope; it does not waive those obligations or publish the partial adoption.

The local `fixture-cancellation-standards` branch at base `68dad7a` is retained in `worktrees/commandcode-support`. Its
184 changed or new files and ignored investigation evidence remain protected, unstaged WIP. A separately recorded
105-file source frame includes the fixture SDK, support producers, classifier, shell cases and controls. This idea
publishes none of that implementation.

Important focused results retained before deferral:

- Numeric and shell controls: 121 passing cases across 20 selected tops; the retained-reader dependency repair: 10
  passing cases across three tops.
- Compiler-consumer and namespace controls: 29 passes. Darwin scalar decoding: six passes, followed by one hostile
  payload control against the repaired decoder. Hostile payloads were never run against the old decoder.
- Inherited-environment SDK controls: 15 passes. Inventory snapshot transport: all four original tops passed after
  meaningful failures against the old behaviour. These checks preserved their original assertions and checked process
  retirement; they are not a whole-suite acceptance claim.

The retained sources include `tests/fixture/root.go`, `tests/architecture/fixture_git_test.go`,
`tests/support/release_v04.go`, `tests/support/release_retirement_darwin.go`, and their focused internal tests. Private
source inventories, review ledgers and native receipts remain under the retained worktree's
`local-tmp/classifier-lint-repair/`; they are investigation evidence, not repository authority.

## Why Now

Record the boundary before closing Command Code support so future work can reuse the evidence without mistaking partial
implementation for merged support. There is no deadline or current authorization to finish this migration. Disposition:
**keep**, in Q2, until an owner schedules a separate fixture-compliance task.

## Prior Art

Read 2026-10-10:

- [Command Code PR 164](https://github.com/wahidyankf/hippo/pull/164) is merged and handles local personal-state
  tracking. It is independent of this deferred fixture implementation.
- [Quality gates](../../../repo-governance/development/quality-gates.md) require the complete architecture,
  deterministic core coverage at 99%, and full gates on Ubuntu 24.04 and macOS 15.
- [Test-driven development](../../../repo-governance/development/test-driven-development.md) and
  [data safety](../../../repo-governance/conventions/public-repository-data-safety.md) govern future verification and
  publication. Focused receipts cannot replace those gates.

## Proposed Direction

Resume from a freshly reconciled source and caller inventory. Finish the three standards as one coherent migration, with
genuine positive controls before physical mutations and independent review before adopting permissions. Keep
actual-checkout readers separate from fixture writers and executable ownership separate from source identity.

## Scope and Non-Goals

In scope: fixture ownership, deterministic doubles, writable Git isolation, executable inputs, and their cleanup and
caller proofs. Preserve the existing public behaviour, process observations, and assertions. Command Code support,
managed compiler installation or upgrades, threshold changes, and blanket package or filename permissions are outside
this idea. This brief is not a promoted plan or a delivery checklist.

## Risks and Open Questions

- Whole architecture acceptance, full current-corpus verification, strict analysis and 99% coverage remain pending.
- The three remaining role proofs cover five-platform PTY execution, the genuine unregistered-compiler script refusal,
  and eight copied dependency files through 16 generic callers. Their complete SDK, caller and effect joins remain open.
- Registered-start producer proofs (G5) and compiler-provider provenance remain incomplete. The reviewed collector and
  official-distribution comparison have not run; source hashes or matching versions do not establish binary derivation.
- The new collector wrapper has a blocking cancellation finding: its original owner may exit while a separately grouped
  native payload remains alive, preventing reliable retirement and exact owned-root cleanup. It was never run.
- A bounded SDK envelope review remains unfinished. A same-named local receiver type may enter its canonical caller
  census without a declaration-Object join. This is an unexecuted potential finding, not a confirmed permission bypass.

## Success and Promotion Signal

Success means every retained obligation has current evidence, independent review, genuine refusal controls and passing
repository gates, with no skipped scope or unowned live resources. Promote only when an owner explicitly schedules the
separate migration and reconciles this protected WIP with current `main`. Retire if later repository work resolves the
obligations or the owner abandons the migration, recording which evidence supports that decision.
