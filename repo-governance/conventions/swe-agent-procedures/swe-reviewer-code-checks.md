---
description: >-
  Holds the six code checks of the swe-reviewer agent, moved verbatim from its definition so the definition fits its
  word budget.
when_to_use: >-
  Use when swe-reviewer audits code and its definition points here.
---

# SWE Reviewer Code Checks

Moved verbatim from [swe-reviewer](../../../.agents/agents/swe-reviewer.md), which links each section here, per Document
Word Budget.

## Code

1. **Placement and failure handling.** Hexagonal Architecture and Functional Core, Imperative Shell, with error fates,
   logging, and input validation judged as
   [Developing Applications](../../../.agents/skills/developing-applications/SKILL.md) teaches, and types per
   [Type and Boundary Safety](../../development/quality/code/type-and-boundary-safety.md).
2. **Clarity and cost.** [Code Clarity](../../development/code-clarity.md), Code as Liability,
   [Dependency Selection](../../development/dependency-selection.md), and
   [Shell Scripts](../../development/quality/code/shell-scripts.md) for any script in scope.
3. **Stack rules** from the stacks the project lists, read from the repository's local copies as
   [Stack Packs](../structure/stack-packs.md) resolves them. A stack with no recorded standard gets no stack rule, and
   the missing decision is reported.
4. **Test design.** Each test sits at its layer, per the test-boundary standard below; doubles follow Test Doubles, data
   follows Test Data Isolation, any git fixture follows Git Fixture Isolation, and a coverage number measures only what
   [Meaningful Coverage](../../development/quality/testing/meaningful-coverage.md) allows.
5. **Test-first evidence.** New or changed behaviour has a test, and the records
   [Cycle and Evidence](../../development/test-driven-development.md) requires exist wherever the work kept them.
   Behaviour shipped with no test is a finding.
6. **Regression tests.** Each bug fix carries the test [Regression Tests](../../development/test-driven-development.md)
   requires.
