# Quality Gate Results

A gate runs against a frozen snapshot and returns exactly one result:

| Result               | Means                                                         |
| -------------------- | ------------------------------------------------------------- |
| `PASS`               | nothing outstanding                                           |
| `PASS_WITH_FINDINGS` | findings exist, each has an explicit disposition, none blocks |
| `FAIL`               | at least one finding blocks proceeding                        |

## No Polling State

There is no fourth value. "Running", "pending a fix", "almost passing", and "re-run and see" are not results — they are
the absence of one, and every one of them lets work continue past a gate that never closed.

The gate is not re-run until it agrees. It runs once, findings are repaired within the budget, and the outcome is
decided.

## The Snapshot Is Frozen

A gate that re-reads a changing draft is measuring a moving target and cannot state what it verified. Record the commit;
that commit is what the result is about.

## Bounded Repair

At most two repair-and-verification cycles. Each cycle repairs against the frozen finding list, and the count of open
findings must strictly decrease — a cycle that changes which findings exist without reducing how many is not progress.

At the ceiling, or at the first cycle that makes no progress, choose between the repaired state and the last known-good
one on criteria declared before the first cycle, and record the choice with its reasoning.

## An Empty Report Is Not a Completion Predicate

The temptation is to iterate until the checker returns nothing. That state is always reachable, by repair or by
attrition, and reaching it says nothing about quality.

What the budget forces instead is the useful question: is this good enough to proceed, or is the previous state better
than what two cycles produced? Either answer is terminal.

## `PASS_WITH_FINDINGS` Requires Dispositions

It is admissible only when each residual finding carries an explicit disposition — accepted with a reason, deferred to a
named owner, or judged not applicable.

Without that, it becomes a way to record `FAIL` in a form that reads like success.
