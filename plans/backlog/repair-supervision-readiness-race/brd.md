# Business Requirements: Repair Supervision Readiness Race

## Business Goal

Restore trust in HIPPO's supervision gate by ensuring a passing result does not depend on whether a child writes its PID
before a synthetic collector failure wins the scheduler race.

## Affected Roles

- Maintainers need a first-run gate result to be evidence, not a lucky rerun.
- Contributors need failure diagnostics to identify the violated premise.
- Consumers rely on the supervision scenario to prove cleanup before lease release.

## Desired Outcomes

- The fixture observes child readiness before it injects the failure under test.
- A child that never becomes ready fails boundedly and explicitly.
- Existing product behavior and the Gherkin contract remain unchanged.
- Repeated focused runs and complete gates pass without retries.

## Success Measures

- The current race is reproduced deterministically before the repair.
- The repaired scenario passes twenty consecutive focused executions on the authoring platform.
- `npm run test:quick`, `npm test`, and the pull-request quality gate pass on the exact head.
- No production file, public command, configuration key, or exit code changes.

## Non-Goals

- General test-runner retry support.
- Product-level readiness signaling.
- New command-line or configuration surface.
- Release publication.

## Business Risks

- An unbounded wait could replace a flake with a hang.
- A marker written by the test rather than the child would prove the wrong premise.
- Broad helper refactoring could disturb unrelated v0.4 fixtures; the change stays at the one failure boundary.
