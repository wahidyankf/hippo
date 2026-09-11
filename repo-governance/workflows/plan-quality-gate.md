# Quality Gate

## Entry

A complete draft, with both decision gates finished. The draft is frozen at a named commit for the duration of the gate.

## Sequence

1. **Freeze the snapshot.** Record the commit. A gate that re-reads a changing draft cannot state what it verified.
2. **Run structural validation** against the frozen snapshot. Structure is mechanical and is checked mechanically — see
   [Structural Validation](../conventions/plans/006-structural-validation.md).
3. **Review what validation cannot reach**: whether the acceptance criteria are testable and the right ones, whether
   `delivery.md` is genuinely executable by someone who was not present, whether the technical shape matches the work.
4. **Record one terminal verdict** — `PASS`, `PASS_WITH_FINDINGS`, or `FAIL` — with the command, the commit, the time,
   and the findings, sanitized.
5. **Repair within the budget.** At most two repair cycles. Each cycle repairs against the frozen finding list and
   re-verifies once.
6. **Decide at the ceiling.** If findings remain after the second cycle, choose between the repaired draft and the last
   known-good state on the criteria declared before the first cycle, and record the choice and its reasoning. Do not
   extend the budget because the next attempt looks close.

## Exit

A terminal verdict exists as a file, with its command, commit, timestamp, result, and sanitized findings.

## Why Bounded

An unbounded quality gate is a gate that always passes eventually. Each repair cycle costs judgement, and the third
cycle is usually spent defending the second rather than improving the plan.

The budget forces the more useful question: is this draft good enough to execute, or is the last known-good state better
than what two cycles produced? Either answer closes the gate.
