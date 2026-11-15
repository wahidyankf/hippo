# Consumer Correctness

When options differ, choose the one that most helps the consumer be correct. A terse or ambiguous surface that lets
misuse pass quietly is the wrong choice even when it is shorter.

## The Rule

**Prefer the surface a consumer cannot misuse silently.** Where two designs serve, the explicit one wins over the short
one: an error that names the cause and the fix, a name spelled the same on every surface, a failure that is signalled
rather than swallowed.

- **Descriptive errors.** A failure names what caused it and what fixes it. The diagnosis states what is wrong; the
  remedy follows as advice, never mistaken for the diagnosis — see [Errors and Diagnostics][errors-and-diagnostics].
- **Exact names across surfaces.** A command, flag, code, or field keeps one spelling on the command line, in
  machine-readable output, in documentation, and in specifications. A rename is a [contract change][changing-it], not a
  spelling correction.
- **No silent behavior.** A failure path signals: a non-zero status, a named `hippo.area.reason` code, or both. The only
  permitted silence is the declared [harness callback exemption][harness-callback-exemption], whose failures are
  recorded where a maintainer can find them.

[errors-and-diagnostics]: ../conventions/command-line-interface/006-errors-and-diagnostics.md
[changing-it]: ../development/public-contract.md#changing-it
[harness-callback-exemption]:
  ../conventions/command-line-interface/006-errors-and-diagnostics.md#the-harness-callback-exemption
[code-clarity]: ../development/code-clarity.md

## Why This Is a Principle

Consumers cannot read maintainer intent. An ambiguous surface does not stay ambiguous; it becomes misuse, and misuse
becomes incidents. A descriptive surface fails loudly at the point of error, where the fix is cheapest.

Brevity is a maintainer convenience paid for by every consumer. This repository's code is read while something is
failing on a machine the reader does not have — see [Code Clarity][code-clarity] — and its surfaces are consumed under
the same condition.

## Followed and Violated

Followed: of two candidate surfaces, the chosen one names the cause and the fix in its error, spells every name
identically on every surface, and signals failure instead of passing quietly.

Violated: a chosen surface is terser but lets misuse pass quietly — an error that omits the cause, a name that differs
between surfaces, or a failure path that reports success outside the declared exemption.

## Disposition

Unenforced by decision. Choosing between candidate surfaces is a design judgement no mechanical check can settle; review
judges it against the followed and violated observations above.

Entry is automatic through the existing index: this document is linked from the [principles index](README.md), which
[AGENTS.md](../../AGENTS.md) reaches through [`repo-governance/`](README.md). No new trigger is declared.
