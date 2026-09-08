# Specification Maintenance

`specs/` is canonical. Behaviour changes are written there first, and the code follows.

## The Cycle

1. **Assess.** Before every repository change, assess both `specs/behaviours/` and `specs/architecture.md` for impact. Record a verified no-op rather than churning an unaffected specification — "I checked and nothing changed" is a result.
2. **Write the scenario.** Gherkin first, in the feature that owns the behaviour.
3. **Prove the binding failure.** Run the adapters and see the scenario reported as undefined. A scenario that was never red proves only that it is bound.
4. **Bind it.** Register the steps in the driver.
5. **Prove it fails at an executing boundary.** Binding compliance is not execution: `tests/bdd` resolves patterns, and the unit, integration, and E2E adapters run them. A mutation that leaves the first green and the second red is the difference this step exists to catch.
6. **Write the code.** Then green.
7. **Synchronize the C4 views** with the final as-built boundaries — see [architecture specifications](architecture-specifications.md).

## Rejected Outright

Placeholder steps, no-op assertions, and outcome tables that assert nothing. Each of them passes, and each of them means the behaviour is unproven while looking proven.

## Exemptions

Every scenario runs through the unit adapter. There is no unit exemption tag and no unit inventory entry; a claim that cannot be made at the unit boundary is a claim about the harness rather than the product.

An integration or E2E exemption must be exact, and must name both the concrete boundary and the reason the scenario cannot execute there. "Hard to set up" is not a reason; "gate script text is outside the compiled binary boundary" is.
