# Business Requirements: Isolate Test Coordination State

## Business Goal

Make the test suite's verdict depend on the working tree, not on which HIPPO release happens to own the machine's
coordination state or which `HIPPO_*` variables the contributor's shell exports.

## Affected Roles

- Contributors running `npm test` on a workstation where other repositories run under the guard.
- Maintainers reading a local failure as evidence about the change under test.
- Consumers whose live coordination state a contributor's test run must never join.

## Desired Outcomes

- A local run and a CI run of the same head give the same answer.
- A test run never resolves the user's shared evidence root.
- Scenarios that set their own roots and helper flags keep working unchanged.

## Success Measures

- The recorded guarded reproduction passes with every `HIPPO_*` variable still exported.
- `npm run test:quick`, `npm test`, and the pull-request quality gate pass on the exact head.
- No product file, specification, public command, configuration key, or exit code changes.

## Non-Goals

- Guarding HIPPO's own gates.
- Changing how the product resolves its environment.
- Retry or rerun policy.

## Business Risks

- Scrubbing a helper process's own flags would break re-executed test binaries; the marker exists to prevent it.
- Removing `HIPPO_BIN` would break the adapters that name the binary to test; it stays a harness input.
- Removing `HIPPO_LOAD_SATURATED` would make the loaded gate reject the deferral it documents as correct; it stays a
  harness input.
