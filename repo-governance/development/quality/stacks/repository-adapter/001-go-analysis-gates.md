---
description: >-
  Records nilness, exhaustive maps, scoped exhaustruct_v5, pinned NilAway/exclusions, no-finding domain literal
  analysis, and gochecksumtype targets beyond golangci-lint defaults.
when_to_use: >-
  Use when changing `.golangci.yml`, the NilAway line in `scripts/test-quick.sh`, a `//nolint:nilaway` or `#nosec G702`
  directive, or the domain literal analysis.
---

# Go Analysis Gates

Go choices/reasons under [repository adapter](../repository-adapter.md). Commands:
[`.golangci.yml`](../../../../../.golangci.yml), [`scripts/test-quick.sh`](../../../../../scripts/test-quick.sh);
versions: [`go.mod`](../../../../../go.mod).

## golangci-lint Settings

- linters: all enabled after `gofumpt`/`goimports`, stronger than the standard; disabled linters state reasons in
  `.golangci.yml`.
- `govet`: nondefault `nilness` detects dereferences of known-nil pointers.
- `exhaustive`: `switch`/`map` require every enum member, forcing decisions on additions; see `retryable` in
  `internal/status/status.go`.
- `exhaustruct_v5`: `explicit-mode: true`; one `enforce-patterns` regex per closed domain struct matches full
  `import/path.TypeName`. Missing fields forge values; checking every literal would restore the fixture brittleness
  behind disabled `exhaustruct`.
- `gochecksumtype`: enabled without `//sumtype:decl` targets: domain concepts are scalar enums covered by `exhaustive`.
  The first sealed interface needs no configuration change.
- `depguard`: production stays within the standard library, Cobra, the HIPPO module, and `golang.org/x/sys`; test
  support additionally admits the Godog harness. No Charm, Bubbles, or terminal framework remains after the TUI removal.
- `ireturn`: no exclusions; helpers, constructors, and all other files return concrete types:
  [Lint Strictness](../../checks/lint-strictness.md).
- `G702`: no inline `#nosec G702` waivers remain after the TUI removal; every `exec.Command` call stays checked under
  [Lint Strictness](../../checks/lint-strictness.md).

## NilAway

- runner: `go.uber.org/nilaway/cmd/nilaway` in `go.mod`'s `tool` block; no tags, so pseudo-versioned. Run directly after
  golangci-lint. This separate tool tracks cross-function/package nil flow beyond per-function `nilness`.
- pins: review `go.mod` diffs: minimal version selection can raise shared modules. The first pin raised production
  `golang.org/x/sys`: [Dependency Selection](../../../dependency-selection.md).
- output: plain; diagnostic `3`/loading failure `1` stop the gate. Never `-json`: findings exit `0`.
- scope: `-include-pkgs` selects this module, excluding dependency repairs.
- exclusions: colocated `//nolint:nilaway // <reason>`, never `-exclude-errors-in-files`, per
  [Lint Strictness](../../checks/lint-strictness.md). `nolintlint` accepts them; golangci-lint's
  `Found unknown linters in //nolint directives: nilaway` warning does not fail.

[NilAway Exclusion Review](002-nilaway-exclusion-review.md) owns the full review limitations and known cases.

## Domain Literal Analysis

- form: standard-library repository Go test [`domain_literals.go`](../../../../../tests/support/domain_literals.go),
  bound to "Production code compares no domain value with a literal" in
  [`quality-gates.feature`](../../../../../specs/behaviours/quality-gates.feature). No private golangci-lint build or
  new test-support dependency.
- reach: running-platform non-test `cmd/`/`internal/` files. Its header records both rules/name list. Platform-specific
  collection in `internal/adapters/host` contains no domain name.
- findings: zero required; fix violations in place. No allowlist, waiver or exemption.
