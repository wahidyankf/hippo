# Red Green Refactor

The cycle, and the evidence each step owes.

## RED

Write the failing test. **Run it, and read what it said.** Capture the output — the message, the boundary, the assertion that failed.

A test that was never observed failing is a test whose failure mode is unknown. This repository has produced two false passes for exactly that reason: a CI probe that passed because the aggregate did not depend on it, and a scenario that passed because the compliance runner resolves bindings rather than executing them. Both looked like green.

Where the change is to a script, a workflow, or a configuration rather than to code, the RED is a **deliberate mutation**: remove the fix, run the check, read the failure, restore. A mutation the check survives is a check that does not cover the change.

## GREEN

Write the smallest code that passes. Run it and capture that output too. Both captures go in the pull-request body.

## REFACTOR

Improve the shape with the test still green. Nothing new — if behaviour changes, that is a new RED.

## Where the Scenario Comes From

For anything the binary does, the RED begins in Gherkin, not in Go: write the scenario, see it undefined, bind it, see it fail at an **executing** adapter, then write the code. See [specification maintenance](../development/specification-maintenance.md).

## Reporting

Name the test, the boundary, and both outputs. "Tested locally" is not evidence — see [software quality enforcement](../development/software-quality-enforcement.md).
