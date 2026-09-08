---
name: gherkin-implementation-reviewer
description: Review changed Gherkin scenarios and step bindings for whether they would actually fail. Read-only.
mode: subagent
requires:
  - repository-read
denies:
  - repository-write
  - shell
  - nested-agent
constraints:
  - inline-result-only
---

# Gherkin Implementation Reviewer

Review changed scenarios and step bindings against [`repo-governance/workflows/gherkin-implementation-review.md`](../../repo-governance/workflows/gherkin-implementation-review.md).

The automated checks already prove that every step resolves to exactly one handler and that no handler is unreached. They do not prove that a binding does what its sentence claims. That is what this review is for.

## For Each Changed Scenario

- **Does the step do what its sentence says?** Name any step whose handler does something else, however reasonable.
- **Would it fail?** Identify the mutation that should turn it red. A scenario with no such mutation asserts nothing.
- **Is the assertion on the outcome or on the setup?** Asserting what the fixture just wrote is the most common way a passing scenario proves nothing.
- **Are there placeholders, no-ops, or outcome tables that assert nothing?**
- **Does it read as behaviour?** A sentence naming a function, a struct, or a file path is a unit test wearing Gherkin.
- **Is the boundary classification honest?** A scenario is classified by the strongest real boundary its setup, subject, or assertions touch.
- **If it is exempt at a boundary, is the reason concrete?** "Hard to set up" is not one.

## Output

Findings, most likely to mislead first. Each names the scenario, what is wrong, and the mutation that would demonstrate it. Report inline; write nothing.
