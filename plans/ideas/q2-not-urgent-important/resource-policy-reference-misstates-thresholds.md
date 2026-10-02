# Resource Policy Reference Misstates Thresholds

The resource policy reference presents fixed default thresholds that no command uses, misdescribes the `capabilities`
array, and omits the shed that sustained warning triggers, so a reader cannot predict when HIPPO admits, warns, or
sheds.

Filed 2026-10-02 as the owner of the open rows of the docs quality gate that
[release cut](../../../repo-governance/workflows/maintenance/release-cut.md) ran on subject `all` before v0.8.3. Every
row predates v0.8.3; none concerns that release's change.

## Problem and Evidence

Read against `main` at `6878c25`:

- **Thresholds (gate row DQG-01, HIGH).** `docs/reference/resource-policy.md` shows a "Default thresholds" table built
  from `DefaultPolicy()`: fixed memory and disk figures such as 9/8/4 GiB and 30/20 GiB.
  `docs/explanation/reservation-model.md` and `docs/reference/configuration.md` send readers there for the exact values.
  Every command (`run`, `status`, `watch`, `monitor`, `release check`) assesses with the resolved profile's policy
  instead (`profilePolicy` in `internal/policy/profiles.go`), whose thresholds are relative to the host. The `balanced`
  admission reserve, for example, is 15% of effective memory clamped to 1–4 GiB, and its critical level is half of that.
  The only `DefaultPolicy` path, `release.Check` in `internal/release/release.go`, has no production caller. The page
  also contradicts itself: its own warning-window line gives 25% clamped to 4–8 GiB. Read by the table, the 3 GiB
  reading in the v0.8.3 changelog entry would be critical, but HIPPO reported `memory-warning`.
- **Capabilities (DQG-02, MEDIUM).** The page says `capabilities` "states what the host actually supplied", with
  `memory-psi` on Linux "where available". `docs/reference/json-schemas.md` says the Linux pressure group appears only
  where the host supplies it. In fact the Linux collector always emits `["cgroup-v2","memory-psi"]`, and always sets
  `oomEvents` and `oomKillEvents`, which read `0` when `memory.events` is unreadable. Only the PSI pair is omitted.
- **Warning shed (DQG-03, MEDIUM).** `README.md`, `docs/explanation/process-ownership-and-shedding.md`, and the
  reference describe shedding only under critical pressure. The guard also sheds an ephemeral or service child once
  warning outlasts its class grace, 10 s or 30 s (`internal/guard/run.go`). The v0.8.3 consumer saw exactly this shed:
  `HIPPO shedding ephemeral child after memory-warning.`. `specs/behaviours/execution.feature` has no scenario for it.

## Why Now

The threshold table is the page a reader opens to learn why HIPPO deferred or shed their work, and it gives wrong
numbers on every host. Nothing is broken at run time, and no release is blocked, so this is important but not urgent.

## Prior Art

Read 2026-10-02: no open issue or pull request covers these pages, and no brief or plan under `plans/` mentions the
threshold table, `capabilities`, or the warning shed.

## Proposed Direction

Replace the table with the profile-relative formulas and each built-in profile's values, keeping the time-based rows,
which are accurate. State the fixed per-platform `capabilities` lists, or decide that the collector should report what
it read. Say that warning past the class grace sheds non-transactional work, and give that shed a scenario.

## Scope and Non-Goals

In scope: the three pages above and the missing scenario. Not in scope: changing thresholds, profiles, or graces.

## Risks and Open Questions

- Making `capabilities` truthful in the collector would change a published sample field, which the
  [public contract](../../../repo-governance/development/public-contract.md) governs; documenting today's fixed lists
  would not.

## Success

A reader can compute, from the reference alone, the thresholds HIPPO applies on their host, and predict a warning shed.
The next docs quality gate on subject `all` reports none of these rows.
