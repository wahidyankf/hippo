---
name: spec-impact-assessment
description: Assess what a proposed change does to specs/behaviours and specs/architecture.md before writing any code.
---

# Specification Impact Assessment

Required before **every** repository change. The rule is [`repo-governance/development/specification-maintenance.md`](../../../repo-governance/development/specification-maintenance.md).

## Ask

1. **Does this change what the binary does?** If yes, a Gherkin scenario changes, and it changes _first_.
2. **Which feature owns it?** `specs/behaviours/` holds the corpus. A behaviour with no obvious feature is usually a behaviour described at the wrong level.
3. **Does it move a boundary or a responsibility?** If yes, `specs/architecture.md` changes in the same pull request, to the final as-built shape rather than the intended one.
4. **If neither changes, say so.** A verified no-op is a result. Record it; do not churn an unaffected specification to look thorough.

## Then

Write the scenario. Run the adapters and watch it be reported undefined. Bind it. Run again and watch it **fail at an executing adapter** — `tests/bdd` resolves bindings and does not execute, so a green there is not a red anywhere.

Only then write the code.

## Rejected

Placeholder steps. No-op assertions. Outcome tables that assert nothing. Each passes and each leaves the behaviour unproven while looking proven.

Every scenario runs at the unit boundary; there is no unit exemption. An integration or E2E exemption names the concrete boundary and the reason, in the reviewed inventory.
