# Test-Driven Development

Red, green, refactor — with the evidence of each recorded.

## The Cycle

**RED.** Write the failing test and run it. Capture what it said. A test that was never observed failing is a test whose failure mode is unknown, and the two false passes in this repository's recent history were both exactly that.

**GREEN.** Write the smallest code that makes it pass. Run it again and capture that too.

**REFACTOR.** Improve the shape with the test still green. Nothing new; if a behaviour changes, that is a new RED.

## Evidence

"Tested locally" is not evidence. The name of the test, the boundary it ran at, and its output before and after are. A pull-request body carries them — see [pull request body](../conventions/pull-request-body.md).

Where the change is to a gate or a script rather than to the binary, the RED is a deliberate mutation: remove the fix, run the check, see it fail, restore. A mutation that leaves the check green means the check does not cover the change.

## Coverage

Deterministic production core coverage stays at or above **99%**, measured in the same run that executes the unit adapter. Measured once rather than twice, because two runs can disagree and the number that gates has to be the number the passing run produced.

Coverage is a floor, not a goal. Platform and process boundaries are proved through the integration and E2E adapters instead, which is a stronger claim than a line count — see [behaviour-driven development](behaviour-driven-development.md).
