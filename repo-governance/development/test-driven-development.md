# Test-Driven Development

Red, green, refactor — with the evidence of each recorded.

## The Cycle

**RED.** Write the failing test and run it. Capture what it said. A test that was never observed failing is a test whose
failure mode is unknown, and the two false passes in this repository's recent history were both exactly that.

**GREEN.** Write the smallest code that makes it pass. Run it again and capture that too.

**REFACTOR.** Improve the shape with the test still green. Nothing new; if a behaviour changes, that is a new RED.

## Evidence

"Tested locally" is not evidence. The name of the test, the boundary it ran at, and its output before and after are. A
pull-request body carries them — see [pull request body](../conventions/pull-request-body.md).

Where the change is to a gate or a script rather than to the binary, the RED is a deliberate mutation: remove the fix,
run the check, see it fail, restore. A mutation that leaves the check green means the check does not cover the change.

## Time in Fixtures

A fixture that dates evidence the code under test ages against the wall clock takes its dates from the clock that code
reads, never from a fixed calendar date: a fixed date passes until that day is past, then fails on every runner at once.
Where the test needs a fixed instant, it hands the same instant to the code's clock. No gate checks this; it rests on
review.

## Focused Runs and Regression Gates

An item is one mini-cycle or checklist item; its check is the named case or smallest relevant test selection
demonstrating the behaviour, and the block is the whole automated suite except E2E. During a TDD cycle, run only the
check covering the current item to demonstrate Red and Green. Do not run the block to prove a single item. Run the named
affected behaviours and shared boundaries after each REFACTOR step, expanding the selection when the change affects more
code.

Record the exact command, test path, selected case identities, positive executed case count, exit status, and observed
result for each step. An unmatched filter, zero executed cases, or cached output that executes nothing is not proof.

Use the raw runner's selection when an aggregate enforces coverage over code deliberately omitted from a focused run. Do
not lower coverage floors or change their exclusions or full-coverage targets. Keep required build, code generation,
resource admission, fixture isolation, and cleanup around every selected run.

At the end of each plan phase, and before PR ready-for-review or session end as a safety net, run the whole automated
suite except full E2E, plus only the affected E2E flow. One recorded complete run may satisfy both boundaries when the
tested code, tests, configuration, and affected scope are unchanged; otherwise rerun the affected gates. Focused runs
replace no hook, CI, or delivery gate.

Followed evidence names relevant executed cases with positive counts and the complete boundary runs. A per-item whole
suite, a selection omitting an affected shared boundary, nonexecuting evidence, or a missing required complete gate
violates this rule. Existing TDD scope exceptions remain unchanged.

**Enforcement: unenforced by decision.** Selecting the meaningful affected scope and proving its recorded evidence
require judgement; static text checks do not establish semantic test execution.

## Coverage

Deterministic production core coverage stays at or above **99%**, measured in the same run that executes the unit
adapter. Measured once rather than twice, because two runs can disagree and the number that gates has to be the number
the passing run produced.

Coverage is a floor, not a goal. Platform and process boundaries are proved through the integration and E2E adapters
instead, which is a stronger claim than a line count — see
[behaviour-driven development](behaviour-driven-development.md).
