---
description: >-
  Records the Go analysis gates beyond golangci-lint's defaults: nilness, exhaustive map checks, scoped exhaustruct_v5,
  the pinned NilAway run and its exclusions, the domain literal analysis and its no-finding rule, and gochecksumtype's
  target.
when_to_use: >-
  Use when changing `.golangci.yml`, the NilAway line in `scripts/test-quick.sh`, a `//nolint:nilaway` directive, or the
  domain literal analysis.
---

# Go Analysis Gates

The [repository adapter](../repository-adapter.md)'s `golang-standards.md` gates decision, in full. Each entry reads
decision: choice — reason. Commands live where they run, in [`.golangci.yml`](../../../../../.golangci.yml) and
[`scripts/test-quick.sh`](../../../../../scripts/test-quick.sh); versions live in [`go.mod`](../../../../../go.mod).

## golangci-lint Settings

- linters: every one enabled, after `gofumpt` and `goimports` — stronger than the standard; each disabled linter carries
  its reason in `.golangci.yml`
- `govet`: `nilness` enabled — it is not among govet's default analyzers, and a pointer dereferenced where it is known
  to be nil is a defect
- `exhaustive`: `switch` and `map` — a map keyed by an enumerated type names every member, so a new member needs a
  decision, as the `retryable` map in `internal/status/status.go` shows
- `exhaustruct_v5`: `explicit-mode: true` with one `enforce-patterns` regex per closed domain struct, matched against
  the full `import/path.TypeName` — a missing field in such a literal forges a value; without explicit mode every
  literal is checked, and the fixture brittleness that keeps `exhaustruct` disabled returns
- `gochecksumtype`: enabled with no `//sumtype:decl` target — every domain concept is a scalar enum, which `exhaustive`
  reaches; the first sealed interface is checked with no configuration change

## NilAway

- runner: `go.uber.org/nilaway/cmd/nilaway` in `go.mod`'s `tool` block, by pseudo-version because it publishes no tags,
  run directly after golangci-lint — it follows nil flow across functions and packages, which per-function `nilness`
  cannot, and it is not a golangci-lint linter
- pin changes: reviewed in `go.mod`'s diff — minimal version selection lets a tool pin raise shared modules, and the
  first NilAway pin raised the production `golang.org/x/sys`, per
  [Dependency Selection](../../../dependency-selection.md)
- output: plain, never `-json` — it exits `3` on a diagnostic and `1` when loading fails, and either stops the gate;
  `-json` exits `0` with findings
- scope: `-include-pkgs` names this module — dependencies are not this repository's to fix
- exclusions: an inline `//nolint:nilaway // <reason>`, never `-exclude-errors-in-files` — the waiver sits where it
  applies, per [Lint Strictness](../../checks/lint-strictness.md). `nolintlint` accepts it, and golangci-lint warns
  `Found unknown linters in //nolint directives: nilaway` on every run without failing.
  - NilAway scopes a directive to the syntax node it attaches to: a comment trailing an `if` line's `{` misses that
    line, and a comment line directly above the statement covers it.
  - NilAway reports one conflict per nil source, so an exclusion also hides every later unguarded read of that source. A
    change touching an excluded source is reviewed for new reads, because the gate stays quiet about them.
  - No gate reports a stale directive: `nolintlint` cannot tell whether NilAway still needs it, so review removes one
    whose finding is gone.

The two exclusions, both false positives that tests beside them failed to reach:

- `internal/guard/exclusive_status.go`, `ExclusiveStatus`, result 0 of `liveExclusiveHeavyOwner`: a nil owner comes only
  with `live` false or an error, and both return first; NilAway does not correlate the three results. Tests:
  `internal/guard/exclusive_status_test.go`.
- `internal/guard/lease.go`, `DescribeHeavyLease`, result 0 of `readLeaseOwner`: a nil owner comes only with an error,
  which the guarded condition tests first; NilAway loses that once `err` is reassigned. Tests:
  `tests/integration/lease_evidence_test.go`.

## Domain Literal Analysis

- form: a repository Go test on the standard library alone,
  [`domain_literals.go`](../../../../../tests/support/domain_literals.go), bound to "Production code compares no domain
  value with a literal" in [`quality-gates.feature`](../../../../../specs/behaviours/quality-gates.feature) — no private
  golangci-lint build and no new test-support dependency
- reach: non-test files under `cmd/` and `internal/` built on the running platform; its header comment states both rules
  and the name list — only `internal/host` has platform-only files, and it carries no domain name
- findings: none — the scenario fails on any; a violation is fixed where it stands, and no allowlist, waiver, or
  exemption exists
