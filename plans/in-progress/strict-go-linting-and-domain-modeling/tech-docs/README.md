# Technical Design: Strict Go Linting and Domain Modeling

How the plan is built. Why it is worth doing is in [the business requirements](../brd.md), and what must be true when it
is done is in [the acceptance criteria](../prd.md#acceptance-criteria).

## Resulting Shape

```text
internal/policy      Reason, Lineage, ProfileName, AdmissionPath, AdmissionInput, DecideAdmission,
                     SparesStableWarning, TaskClass.UnmarshalText (strict), RecordedTaskClass (tolerant)
internal/evidence    Outcome (closed, zero = unset), BudgetOutcome, RecordedOutcome (tolerant reader)
internal/guard       writes typed outcomes; returns reasons as *policy.Stop; ShedCause with its 73/75 codec
internal/cli         one exhaustive switch: policy.Reason -> status.Code -> exit status;
                     withAssessmentDecision switches over AdmissionPath
internal/config      carries Lineage through extends; strict coordination mode
tests/support        domain literal analysis; the driver resolves through configuration and DecideAdmission
gates                govet nilness, exhaustive {switch, map}, exhaustruct_v5 (scoped), go tool nilaway,
                     the domain literal analysis scenario
```

## Shared Design Rules

- **No wire value moves.** Every typed value that reaches JSON, a ledger, a receipt, or stderr encodes the bytes
  `v0.8.4` writes, through one exhaustive `switch` per type, pinned by a test that names the `v0.8.4` value.
- **Closed means a zero value that is invalid.** Each new closed type is a defined `uint8` whose zero member is named
  `…Unset` and is refused by its encoder, so a forgotten assignment fails loudly instead of reading as a member.
- **Parse at the boundary.** Raw command-line text stays raw only in fields named `…Flag`, and is parsed into its domain
  type before any decision reads it, per
  [type and boundary safety](../../../../repo-governance/development/quality/code/type-and-boundary-safety.md).
- **Sealed interfaces are not used.** Every new type is a scalar enum: no member carries data the others lack, so
  `exhaustive` covers every `switch` and map over it. `gochecksumtype` stays enabled at its defaults with no
  `//sumtype:decl` target, and the adapter records why.
- **`exhaustruct_v5` targets structs.** It checks composite literals, so its enforce patterns name the plan's new struct
  types (`policy.AdmissionInput`, `evidence.RecordedOutcome`, `policy.RecordedTaskClass`); the scalar enums are covered
  by `exhaustive` instead.

## Rollback

Each unit is one pull request merged by rebase; reverting its commits restores the previous unit's state, because no
unit changes a wire value or a persisted format. The release is the exception: a published tag is never replaced, so a
defect in `v0.8.5` is fixed by `v0.8.6`.

## Directory Map

The companions, listed in their numeric reading order:

- [Domain types](001-domain-types.md) — each type, its members, its codec, and the call sites it replaces.
- [Gates and analysis](002-gates-and-analysis.md) — lint settings, NilAway and its selection argument, and the domain
  literal analysis with its ratchet.
- [Specification changes](003-specification-changes.md) — every Gherkin and C4 file the units edit.
- [File impact](004-file-impact.md) — every path each unit touches.
