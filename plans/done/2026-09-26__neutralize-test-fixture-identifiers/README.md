# Neutralize Test Fixture Identifiers

Status: Done (2026-09-26)

## Context

[Repo-grounded] `internal/identity/identity_test.go::TestOverrideSourceWithoutFile` passes a source override to
`identity.Load` and asserts it back (lines 42 and 46). The string it uses is not synthetic: a fixture string names a
private repository. [Data safety](../../../repo-governance/conventions/public-repository-data-safety.md) asks fixtures
to use `example.invalid`, `fixture`, and obviously synthetic values. Found during a cross-repository standards adoption.

The value is already published, and history is not rewritten here. The remedy is to stop carrying it forward.

## Decision

[Judgment call] Replace the string at both lines with the synthetic `fixture-source`, and leave every other identity
fixture alone.

Rejected alternatives:

- add the name to `scripts/public-safety/shape-terms.txt` — the term set is public, so listing the name there would
  publish it again;
- rewrite history — the ruleset refuses the force push, and data safety rules it out as a remedy.

## Decision Gate Record

- Filed on 2026-09-26 as a knowledge-capture follow-up.
- Activated on 2026-09-26: the owner approved the decision above and directed execution now, with the plan archived
  inside its delivery pull request rather than through a second one. The
  [quality gate](evidence/quality-gate.md) records the resulting delivery repair.

## Scope

In scope: the two lines in `TestOverrideSourceWithoutFile`. Out of scope: product code, other fixtures, the
public-safety term set, and history.

## Approach Summary

1. Prove that the assertion reads the value, using a deliberate mutation.
2. Replace the string at both sites and run the identity tests and repository gates.

## Dependencies

- [Repo-grounded] `npm run test:quick` runs `go test` over the module and the unit gate.

## Directory Map

- [Business requirements](brd.md)
- [Product requirements and acceptance criteria](prd.md)
- [Technical design](tech-docs.md)
- [Delivery checklist](delivery.md)
- [Execution learnings](learnings.md)
- [Evidence](evidence/README.md)
