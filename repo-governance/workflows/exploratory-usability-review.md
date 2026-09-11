# Exploratory and Usability Review

## Entry

A change with a user-facing surface, whose programmatic checks have already run. Running this first wastes a reviewer's
attention on defects a machine would have caught.

## Sequence

1. **Declare the tasks before starting.** State them as goals in the user's terms, and freeze the list. A task added
   mid-pass is chosen because of what was just seen, which is how a review turns into a search for confirmation.
2. **Choose a reviewer who did not build it**, where that is possible. Whoever built it cannot un-know where things are,
   and that knowledge is exactly what the review is trying to do without.
3. **Attempt each task once**, without hints, without the documentation, and without the author narrating.
4. **Record what happened** — including the tasks that went fine. A pass that records only problems cannot distinguish a
   flow that worked from one nobody tried.
5. **Record failures as behaviour, before explanation.** Stalls, guesses, backtracks, abandonment. "Took 40 seconds to
   find the setting" is a finding; "took a while, but they would learn it" is a defence of one.
6. **Note anything unexpected outside the tasks.** This is the part automation structurally cannot do, and it is worth
   more than the scripted portion.
7. **Record one terminal result** for the pass.

## Exit

Every declared task carries an outcome, every failure is recorded as behaviour, and the pass has one terminal result.

## Bounded

One pass over the declared tasks. Findings may be repaired within the quality gate's budget and the pass re-run once
against the **same** tasks.

Not re-run against new tasks until it comes back clean — that converges on a clean report rather than on a usable
interface, and the two are easy to confuse from the outside.

## What This Does Not Do

It does not verify correctness; that is the programmatic layer's. It does not check device-specific rendering; that is
its own layer. It does not fix anything it finds.
