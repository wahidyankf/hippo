# Strict Go Linting and Domain Modeling

Status: Done (2026-10-07) — authored 2026-10-06 from the idea brief filed 2026-10-03; released as v0.8.5.

HIPPO carries several domain concepts as strings, integers, and booleans: a run's outcome, the five internal reasons
behind exit `124` and `125`, a profile's identity and lineage, and the path admission takes. No tool can say when a case
is missing, a value is forged, or a default silently applies. This plan turns each of them into a Go type, tightens the
lint configuration, and adds a repository analyzer, so the next defect of that class fails a gate instead of reaching a
consumer. The public contract does not move; one behaviour is fixed on the way.

## Context

[Repo-grounded] Everything below was measured on `origin/main` at `892c462` on 2026-10-06.

- **Outcome.** `internal/guard/run.go` starts every summary as `outcome := "capacity-deferred"` (line 810) and reassigns
  it on 22 lines; the deadline deferral at line 934 relies on that default. `"capacity-deferred"` and
  `"storage-blocked"` are bare literals, and `RunOutcomes` (lines 51–59) is a hand-kept list of ten strings whose only
  consumer is the `history --outcome` check (`internal/cli/history.go`, line 54). `internal/evidence/history.go`
  compares the recorded outcome with `"passed"` (line 212) and `budgetOutcome` with `"pressure-shed"` and
  `"storage-shed"` (line 217), values no writer in the repository's history has ever written: the only budget outcome
  written is `"admitted"` (`run.go`, line 808).
- **Internal reasons.** `73`, `74`, and `75` (`run.go`, lines 22–33) and `76` and `78` (`internal/policy/profiles.go`,
  lines 13 and 15) appear on 43 production lines in six files, plus a bare `73` in `Resolve` (`profiles.go`, line 260).
  The command-line boundary maps them through `internalReasons` (`internal/cli/status.go`, lines 26–32), and
  `reasonMessage` carries the repository's only `//nolint:exhaustive`. Two of them leave the process anyway: status JSON
  publishes `profile.exitCode` (`0`, `73`, `75`, or `78`), and the shared reservation ledger records `sheddingExitCode`
  (`73` or `75`, validated by `validSheddingExitCode`, `internal/guard/reservation.go`, line 1618).
- **Profile identity.** `Resolve` keeps its last-resort floor for the profile named `minimal` (`profiles.go`, line 266),
  so a configured profile that `extends: minimal` and does not fit falls through to "no usable fallback" and exit `125`
  naming `hippo.policy.replan-required`. The guard's automatic owner-share default switches on the names `balanced` and
  `constrained` (`reservation.go`, lines 263–273); configuration fills every catalog profile's share through
  `inheritShares` (`internal/config/config.go`, lines 256–276), so that switch is reached only by callers that bypass
  configuration. Lineage today is one inherited boolean, `Profile.DegradedAdmission` (`profiles.go`, lines 66–71), added
  by the v0.8.4 fix.
- **Admission.** `run` decides admission in its sampling loop (`run.go`, lines 844–935), `status` in
  `withAssessmentDecision`'s `if` chain (`internal/cli/development.go`, lines 94–106), and the test driver in a third
  copy, `assessAdmission` (`tests/support/driver.go`, lines 442–468), which resolves only `policy.BuiltinCatalog()`.
- **Decoding.** The four enums (`policy.State`, `policy.TaskClass`, `policy.Decision`, `status.Code`) are
  `type X string` with no `UnmarshalJSON` or `UnmarshalText` anywhere. The reservation ledger decodes `class` with
  `DisallowUnknownFields` and validates it afterwards (`reservation.go`, lines 735 and 585); lease records and evidence
  summaries carry the class as a plain string.
- **Gates.** `scripts/test-quick.sh` runs golangci-lint v2.13.2 (`default: all`, line 25). `govet` has no settings, and
  `nilness` is not among v2.13.2's default analyzers. `exhaustive` checks only `switch`; `exhaustruct` and
  `exhaustruct_v5` are disabled for fixture brittleness. A scratch probe with `nilness` and `exhaustive` map checking
  enabled reported one finding: the sparse `retryable` map in `internal/status/status.go`, line 179. A scratch NilAway
  run reported eight diagnostics, two in production (`internal/guard/exclusive_status.go`, line 60, and
  `internal/guard/lease.go`, line 315) and six in tests.
- **Size.** 13,161 production lines and 22,862 test lines; the latest release is `v0.8.4`.

[Judgment call] The brief's measurement of 58 classified fix commits found five defects that typing stops outright and
16 catch-all classifications that typing turns into explicit choices. That history is the case for doing this; this plan
does not re-argue it.

## Decision

Model the five concepts as closed Go types, route `run`, `status`, and the test driver through one admission decision,
key profile rules on lineage rather than names, decode decision-bearing enums strictly and evidence tolerantly, and gate
it all with `nilness`, `exhaustive` map checks, a pinned NilAway, a scoped `exhaustruct_v5`, and a repository analyzer
that refuses domain values compared with literals. Deliver it as six code units, a release, and an archival change. The
designs are in [the technical documents](tech-docs/README.md).

Rejected, from the brief: a rewrite in Rust (contract surface, mixed versions on one state root, cost); a golangci-lint
plugin for the analyzer (a private build of the linter for one check); publishing the admission path in status JSON (a
contract change needing its own release decision).

## Decision Gate Record

Pre-write gate, run with the owner on 2026-10-06. Each line records the choice and its reason.

- **D1 — One plan.** Domain types (outcome, internal reason, profile lineage, admission path, the single admission
  decision, strict decoding), the lint table, and configured-catalog scenarios. One argued change keeps the remedy from
  being applied path by path, which is how the background defect survived.
- **D2 — One pull request per delivery unit,** landed serially from this one worktree through worktree to pull request.
  The owner pre-authorized every commit, push, merge, and the release this plan names. Each unit leaves `main` coherent,
  so each is a [delivery boundary](../../../repo-governance/conventions/pull-request-boundaries.md).
- **D3 — NilAway is a gate outright,** a pinned `go tool` run from `scripts/test-quick.sh`, with real hazards fixed in
  code. [Lint strictness](../../../repo-governance/development/quality/checks/lint-strictness.md) admits no advisory
  tier, so a trial that reports without failing would be one.
- **D4 — The literal analyzer is a repository Go test** on the standard library's `go/parser`, `go/ast`, `go/types`, and
  `go/importer`, not a golangci-lint plugin and not `golang.org/x/tools`. It fits the existing scripted scans, and
  `depguard` admits no new test-support dependency without a selection argument.
- **D5 — Lineage, not names,** decides the `minimal` floor in `Resolve` and the guard's owner-share default, as it
  already decides degraded admission. A configured profile derived from `minimal` gains the floor: a fix, specified
  first, recorded under `Fixed` in the changelog. Configuration keys and JSON do not change.
- **D6 — Strict where it decides, tolerant where it records.** Enums that drive a decision (reservation ledger classes,
  configuration values) refuse unknown members at decode; evidence and history readers decode an unknown value to an
  explicit `unknown` member carrying the raw text, never a default and never a failure, because consumers pin different
  versions and share one state root.
- **D7 — One admission decision, no new JSON.** `run`, `status`, and the test driver call one function, and
  `withAssessmentDecision` becomes an exhaustive `switch`. Status JSON does not publish the path; exit statuses, codes,
  JSON bodies, configuration, and the state root stay as they are.
- **D8 — Typed reasons, narrowly.** Only the five internal integers become one closed type, mapped to `status.Code` and
  the exit status in one exhaustive `switch` at the boundary. The 16 boundary-classification fixes stay out of scope
  beyond what this forces, per [minimal sufficiency](../../../repo-governance/principles/minimal-sufficiency.md).
- **D9 — Analyzer reach.** It refuses (a) comparisons and `switch` cases of domain values against untyped literals, and
  (b) struct fields and parameters with domain names typed as raw `string`, `int`, or `bool` in production packages,
  against an auditable name list.
- **D10 — `exhaustruct_v5` on, scoped** by enforce patterns to the new domain types this plan creates, never globally,
  so the fixture brittleness that disabled it does not return.
- **D11 — NilAway false positives** use NilAway's documented exclusion mechanism, verified upstream at execution as a
  bounded checkpoint, each exclusion carrying its reason and recorded in the repository adapter.
- **D12 — Release `v0.8.5`,** a patch because the contract does not move, cut through release cut after the last code
  unit. Consumer repins are coordinated outside this repository and are not delivered here.
- **D13 — The analyzer ratchets from the first unit.** It lands with an explicit allowlist of current violations (file,
  symbol, rule); each domain unit deletes its entries; the last code unit proves the list empty and removes the
  mechanism. A new violation fails from unit 1, and the gate is green on the day it turns on.
- **D14 — Outcome's zero value is an invalid `unset`.** One exhaustive `switch` writes the wire string, `RunOutcomes`
  derives from the member list, and finalizing while `unset` records `supervision-failed` with
  `hippo.supervision.failed` — never a deferral, the default that produced `41bda15` and `529b506`.
- **D15 — Archival is its own docs-only pull request** after the last unit and the execution check, not part of the last
  unit's.
- **Plan pull request.** The pull request that lands this plan also carries its quality-gate repairs and its move to
  `plans/in-progress/`, so execution starts with the plan already in progress, in the same worktree.

Post-write gate: questions that only the draft revealed are recorded in [delivery](delivery.md#post-write-gate) with
their resolution.

## Scope

In scope: the closed types and their codecs, the single admission decision, lineage-keyed profile rules, strict and
tolerant decoding, the lint settings and the NilAway pin, the analyzer and its ratchet, configured-catalog scenario
outlines, the specification, documentation, adapter, and quality-gate updates each unit needs, the `v0.8.5` release, and
archival.

Out of scope: any change to exit statuses, error codes, JSON bodies, configuration keys, or the state root, beyond the
`minimal`-lineage fix; typed classification of untyped errors (the 16 catch-all fixes); concurrency and platform
defects; and repinning consumers.

## Approach Summary

1. Gates first: `nilness`, `exhaustive` maps, NilAway, and the analyzer with its allowlist.
2. One concept per unit: outcome, internal reasons, profile lineage, admission path and decision, strict decoding — each
   deleting its allowlist entries.
3. The last code unit empties the allowlist and removes the mechanism; then the release; then archival.

## Dependencies

- [Repo-grounded] The v0.8.4 fix
  ([its record](../2026-10-03__fix-configured-profiles-starve-under-macos-warning/README.md)) is on `main`; this plan
  builds on its lineage attribute and its configured-catalog scenarios.
- [Repo-grounded] NilAway publishes no tags, only pseudo-versions; the latest, `v0.0.0-20260918162853-acb8859b9031`,
  requires Go 1.26.0 or later, and `go.mod` names `go 1.26.1`.
- [Judgment call] Each consumer repins `v0.8.5` in its own repository through its own route; that follows this plan and
  is coordinated elsewhere.

## Directory Map

- [Business requirements](brd.md)
- [Product requirements and acceptance criteria](prd.md)
- [Technical design](tech-docs/README.md) holds the type designs, the gates, the specification changes, and the file
  impact.
- [Delivery checklist](delivery.md)
- [Execution learnings](learnings.md)
