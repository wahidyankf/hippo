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
- `depguard`: versioned Tea/Lipgloss roots and Bubbles components stay TUI-only:
  [Code Clarity](../../../code-clarity.md). Exact x/ansi root serves only TUI text and
  `tests/host/evidence-verifier/*.go` native cells. Other restrictions remain.
  - Native support permits exact `golang.org/x/sys/unix` only in `tests/support/tui_arguments.go`,
    `tests/support/tui_public_interactive_inputs.go`, `tests/support/tui_transport_native.go`,
    `tests/support/tui_transport_native_darwin.go` and `tests/support/tui_transport_native_linux.go`: real terminal/FD
    state on both platforms requires the pinned kernel API. Their strict binding retains the support allowlist; other
    support, x/sys packages, Charm and production permissions stay unchanged.
  - Native fault test `internal/adapters/tui/bdd_native_faults_internal_test.go` permits exact `golang.org/x/sys/unix`
    in its separate strict binding, alongside only the standard library and HIPPO module. Atomic replacement of its
    distinct retained descriptors needs the pinned kernel API on Linux arm64; other TUI tests retain their restrictions.
- `ireturn`: only exact `Update`/Tea.Model in `internal/adapters/tui/model.go` is excluded because the pinned external
  consumer requires that interface. Helpers/constructors/other files stay concrete:
  [Lint Strictness](../../checks/lint-strictness.md).
- `G702`: exactly five inline `#nosec G702` waivers under [Lint Strictness](../../checks/lint-strictness.md); other
  calls remain checked:
  - [`invokeCompiledTUI`](../../../../../tests/e2e/tui_contract_test.go): stamped harness selection.
  - [Old executable](../../../../../internal/adapters/tui/bdd_public_mixed_owners_external_test.go): authenticated
    absolute regular executable; pinned SHA/strict release receipt; separate argv/context.
  - [PS wrapper](../../../../../internal/adapters/tui/bdd_public_ps_startup_diagnostic_external_test.go): probe-owned
    0700 wrapper; quoted authenticated helper, fixed argv, finite context/Wait.
  - [Mixed owner](../../../../../tests/integration/mixed_version_v085_test.go): trusted supervisor verifies v0.8.5
    provenance and builds/selects current; leaf identity checks; separate argv, fixed payload shell, finite
    context/Wait.
  - [Mixed status](../../../../../tests/integration/mixed_version_v085_test.go): same selection/identity checks; fixed
    separate status argv, finite context/Run.
  - Review judges continued need; taint analysis cannot authenticate the supervisor (**unenforced by decision**).
    Untrusted selection, interpolated shell source, removed provenance or changed status argv invalidates waivers.
    Leaf/integration TestMain does not authenticate current stamp.

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
