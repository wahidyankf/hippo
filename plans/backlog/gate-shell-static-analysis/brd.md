# Business Requirements: Gate Shell Static Analysis

## Business Goal

Close the recorded shell static-analysis gap so a quoting, dialect, or dangerous-expansion defect in the bootstrap
wrappers, hooks, and gate scripts fails before it is pushed rather than after a consumer runs it.

## Affected Roles

- Contributors editing a wrapper, hook, or gate script.
- Maintainers who today review by eye what an analyser would catch.
- Consumers who run the `hippo` bootstrap wrapper this repository ships.

## Desired Outcomes

- One pinned analyser runs locally and in CI with the same version and the same file list.
- The repository is clean at the warning threshold before the gate turns on.
- The adapter records the shell stack's static analysis as a gate, not a gap.

## Success Measures

- `shell-lint` exits `0` on the delivered head and non-zero on a deliberate mutation.
- `npm run test:quick`, `npm test`, and the pull-request quality gate pass on the exact head.
- No behaviour of any script changes.

## Non-Goals

- Style-level or note-level findings.
- Editing the adopted public-safety scanner.
- A shell test framework.

## Business Risks

- An unverified download would add a supply chain nobody chose; the pin carries a digest per platform.
- A rewritten `CDPATH` assignment that changed behaviour would break every script entry; each fix is behaviour-neutral
  and the full gate proves it.
