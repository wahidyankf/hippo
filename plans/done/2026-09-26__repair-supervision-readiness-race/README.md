# Repair Supervision Readiness Race

Status: Done (2026-09-26)

## Context

[Repo-grounded] The scenario `A supervision failure reaps the guarded child before releasing ownership` has failed by
reading `child.pid` before the guarded shell writes it. Re-running passes because the fixture usually wins the race; the
product assertion is never reached on the failing run.

The behavior contract is already correct. The defect is test synchronization in `tests/support/driver.go`: collector
failure is scheduled by sample count, while child readiness is assumed rather than observed.

## Decision

[Judgment call] Repair only the fixture boundary. Keep the existing Gherkin scenario and product code unchanged, make
the race deterministic in RED, then place a bounded readiness barrier immediately before the injected collector
failure.

Rejected alternatives:

- retry the scenario — hides the first failure and preserves nondeterminism;
- increase a wall-clock delay — reduces probability but does not establish readiness;
- change guard supervision — alters product behavior without evidence of a product defect.

## Decision Gate Record

- Pre-write, 2026-09-16: the owner selected a separate Hippo backlog plan for the supervision-readiness race.
- Post-write, 2026-09-16: after the complete draft and cold-read repairs, the owner approved the plan as written and
  authorized its formal quality gate plus plan-only delivery.

## Scope

In scope:

- deterministically reproduce delayed PID publication;
- wait boundedly for the child-published PID marker before injecting host-evidence failure;
- fail with an explicit readiness diagnostic rather than a missing-file race;
- stress the repaired scenario and run repository completion gates.

Out of scope:

- changing supervision, lease, signal, or collector production behavior;
- changing the existing Gherkin outcome;
- changing retry policy or CI rerun policy;
- publishing a release.

## Approach Summary

1. Make the existing race deterministic with a delayed child PID write.
2. Gate injected collector failure on the child-owned readiness marker with a bounded wait.
3. Preserve the existing reaping and lease-reacquisition assertions.
4. Repeat the scenario enough times to show the synchronization no longer depends on scheduling luck.

## Dependencies

- [Repo-grounded] `tests/support/review_v04.go` already provides bounded marker-wait helpers in the same package.
- [Repo-grounded] `npm run test:quick` runs the quick gate, one of the gates `pre-push` runs, and `npm test` runs the full
  test gate; `./rhino gate list` names every gate on each surface.
- [Repo-grounded] HIPPO does not guard its own compute, so its repository commands run directly.

## Directory Map

- [Business requirements](brd.md)
- [Product requirements and acceptance criteria](prd.md)
- [Technical design](tech-docs.md)
- [Delivery checklist](delivery.md)
- [Execution learnings](learnings.md)
