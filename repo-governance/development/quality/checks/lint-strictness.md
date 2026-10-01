---
description: >-
  Makes every lint gate fail at warning severity and above, turns a gate on only after its backlog is clean, admits only
  waivers documented where they apply, and requires automatic fixes to be reviewed.
when_to_use: >-
  Use when adding a linter, choosing its failure threshold, suppressing a rule, or applying a linter's automatic fixes.
---

# Lint Strictness

A lint finding either matters or it does not. A tier that prints findings without failing gets read for a week and then
scrolls past forever, so a gate that reports a finding also blocks on it.

This standard implements Automation Over Manual, Explicit Over Implicit, and Root Cause Orientation. Where each gate
runs is owned by [Quality Gates](../../quality-gates.md).

## Warning and Above Fails

Every lint gate fails on a finding of warning severity or higher, in every language and artifact type the repository
ships.

| The tool reports  | The gate fails on                  |
| ----------------- | ---------------------------------- |
| severity levels   | warning or above                   |
| no severity       | any finding                        |
| compiler warnings | every warning, treated as an error |

There is no advisory tier. One threshold everywhere means quality does not depend on which kind of file a change happens
to touch, and nobody learns a separate bar per language.

Each gate runs in the local hooks, and in continuous integration as well wherever the repository has it.

## Clean, Then Gate

A new gate is introduced in order:

1. **Clean.** Fix the existing violations, or waive them under the rule below.
2. **Verify.** Run the linter over its full scope and confirm it exits cleanly.
3. **Turn it on.** Only then wire it into the hooks and the pipeline.

A gate switched on over an existing backlog fails the first unrelated change that runs it. The author of that change
inherits someone else's debt, and the quickest way out is to disable the gate. Cleaning first means the gate is green on
the day it goes live, and every later failure belongs to the change that caused it.

## Waivers Are Documented Where They Apply

Suppress a rule only at the spots where following it would cost clarity or reproducibility while buying no real safety.
Every waiver sits at the point of suppression, as an inline directive or a commented entry in the tool's configuration,
and states its reason.

Never suppress silently, and never disable a rule across the repository to quiet one file. A waiver nobody can find is
indistinguishable from a rule nobody enforces.

## Applying a Linter's Fixes

Review every automatic fix before keeping it. Never apply fixes a tool labels unsafe across a whole tree unattended: a
fix that deletes code a rule considers unreachable can take the tests for that code with it, and every gate stays green
over the loss.

Fix only the construct the rule reported. A similar expression on the next line is a separate decision with its own
reason, because a rule that named one site has said nothing about whether anything depends on the one beside it.

## The Inventory Is Local

Each repository records its own gates: the artifact type, the tool, the threshold or configuration, and where the gate
runs. The inventory differs between repositories; the threshold, the rollout order, and the waiver rule do not.

Where a stack has both a formatter and a linter, the linter's layout rules are disabled so the two never disagree.

An adopter enforces the threshold in the linter configuration its own hooks and pipeline read.
