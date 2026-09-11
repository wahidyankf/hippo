# Exploratory and Usability Review

One workflow, not several. A repository with two overlapping versions of this review has two places for a finding to be
missed, and each assumes the other covered it.

The canonical procedure is
[Exploratory and Usability Review](../../workflows/exploratory-usability-review.md). This module states what
it must record.

## What Is Recorded

| Element      | Requirement                                                    |
| ------------ | -------------------------------------------------------------- |
| tasks        | what the reviewer was asked to accomplish, in the user's terms |
| observations | what actually happened, including what was unremarkable        |
| failures     | where the reviewer stalled, guessed, backtracked, or gave up   |
| result       | one terminal verdict for the pass as a whole                   |

Tasks are stated as goals, not steps. "Change your notification settings" is a task; "click Settings, then
Notifications, then toggle" is a script, and a script cannot discover that the path was impossible to find.

## Record What Went Right

Observations that found nothing are evidence too. A review recording only problems cannot distinguish a flow that worked
from one nobody tried, and the second is what a rushed pass produces.

## Failures Are the Product

The stall, the guess, the backtrack — these are what the review exists to find, and they are exactly what a reviewer who
also built the thing will explain away.

Record the behaviour before the explanation. "Took 40 seconds to find the setting" is a finding; "took a while, but they
would learn it" is a defence of a finding.

## Bounded

The review is one pass over the declared tasks, not a search continued until something turns up or until nothing does.

Both unbounded directions fail the same way: the first finds problems in whatever the reviewer grew tired of, and the
second stops the moment the result is acceptable.

## Terminal Result

The pass ends with one verdict. Findings may be repaired within the quality gate's budget and the pass re-run once
against the same tasks — not re-run repeatedly against new tasks until it comes back clean.
