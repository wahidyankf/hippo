# Coding Harness Contract Change

Changing the canonical instruction body, a skill, an agent, or any harness adapter. The contract itself is [coding harness contract](../conventions/coding-harness-contract.md).

## 1. Change the Canon First

`AGENTS.md`, `.agents/skills/<name>/SKILL.md`, or `.agents/agents/<name>.md`. Never an adapter first — an adapter edited ahead of the canon is a divergence that the validator will report as the adapter's fault.

## 2. Prove the Divergence

Run `rhino harness parity validate` and read the finding. A change to the canon that produces no finding either changed nothing a harness expresses, or the harness that should express it is not declared.

Capture that output. It is the RED — see [red green refactor](red-green-refactor.md).

## 3. Update Every Adapter

Each declared harness, in the same change. An adapter left behind is not a smaller problem than a missing one; the gate treats them identically, and so should the author.

## 4. Prove Parity

Run the validator again. It must report every declared harness reconciled, with a digest — the digest is what distinguishes "nothing changed" from "nothing was checked".

Then weaken one denial deliberately and confirm the validator reports it. A parity check that passes whatever the adapters say is not a check. See [coding harness parity verification](coding-harness-parity-verification.md).

## 5. Adding or Removing a Harness

A new harness is a `repo-config.yml` change plus its adapters, landed together. Declaring the harness without its adapters, or adapters without the declaration, both fail — deliberately, because a half-declared roster reconciles against nothing and says it is clean.

Establish a harness's real surface from current vendor documentation before declaring it. A path that was correct when a sibling repository adopted it may not be correct now.
