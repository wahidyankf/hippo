---
description: >-
  Holds the confidence-to-status table of the applying-maker-checker-fixer skill, moved verbatim from its definition so
  the definition fits its word budget.
when_to_use: >-
  Use when a fixer rates a finding's confidence and sets the row's ledger status.
---

# Confidence Decides the Row's Status

Moved verbatim from [applying-maker-checker-fixer](../SKILL.md), which links each section here, per Document Word
Budget.

## Confidence Decides the Row's Status

| Confidence       | The fixer                                     | Status                        |
| ---------------- | --------------------------------------------- | ----------------------------- |
| `HIGH`           | applies the repair, then verifies the row     | `resolved`, or `not-resolved` |
| `MEDIUM`         | leaves it with the evidence that kept it open | `needs-decision`              |
| `FALSE_POSITIVE` | records the disproof and what would stop it   | `not-applicable`              |

A row whose target state already holds is `resolved` with no edit. A row the fixer did not reach stays `open`.
