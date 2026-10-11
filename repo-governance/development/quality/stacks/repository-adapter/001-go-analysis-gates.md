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
    `tests/support/tui_public_interactive_inputs.go`, `tests/support/tui_transport_native.go`, and
    `tests/support/tui_transport_native_darwin.go`: real terminal/FD state requires the pinned kernel API. Their strict
    binding retains the support allowlist; other support, x/sys packages, Charm and production permissions stay
    unchanged.
- `ireturn`: only exact `Update`/Tea.Model in `internal/adapters/tui/model.go` is excluded because the pinned external
  consumer requires that interface. Helpers/constructors/other files stay concrete:
  [Lint Strictness](../../checks/lint-strictness.md).
- `G702`: exactly three inline `#nosec G702` waivers under [Lint Strictness](../../checks/lint-strictness.md); other
  calls remain checked:
  - [`invokeCompiledTUI`](../../../../../tests/e2e/tui_contract_test.go): stamped harness selection.
  - [Old executable](../../../../../internal/adapters/tui/bdd_public_mixed_owners_external_test.go): authenticated
    absolute regular executable; pinned SHA/strict release receipt; separate argv/context.
  - [PS wrapper](../../../../../internal/adapters/tui/bdd_public_ps_startup_diagnostic_external_test.go): probe-owned
    0700 wrapper; quoted authenticated helper, fixed argv, finite context/Wait.

## NilAway

- runner: `go.uber.org/nilaway/cmd/nilaway` in `go.mod`'s `tool` block, pseudo-versioned because it has no tags. Run
  directly after golangci-lint. Unlike per-function `nilness`, it follows nil flow across functions/packages; it is not
  a golangci-lint linter.
- pins: review `go.mod` diffs. Minimal version selection can raise shared modules; the first pin raised production
  `golang.org/x/sys`: [Dependency Selection](../../../dependency-selection.md).
- output: plain; diagnostic exit `3`/loading failure `1` stop the gate. Never `-json`, which exits `0` with findings.
- scope: `-include-pkgs` names this module; dependencies are not ours to fix.
- exclusions: inline `//nolint:nilaway // <reason>`, never `-exclude-errors-in-files`, colocates waivers:
  [Lint Strictness](../../checks/lint-strictness.md). `nolintlint` accepts them; golangci-lint's
  `Found unknown linters in //nolint directives: nilaway` warning never fails the run.
  - Syntax-node scope: trailing an `if` line's `{` misses that line; directly above the statement covers it.
  - One conflict per nil source means exclusions hide later unguarded reads. Review changed excluded sources for new
    reads; the gate stays silent.
  - No stale-directive gate exists: `nolintlint` cannot judge continued need; review removes resolved waivers.

Both false-positive exclusions escaped adjacent tests:

- `internal/adapters/runtime/exclusive_status.go`, `ExclusiveStatus`, result 0 of `liveExclusiveHeavyOwner`: nil means
  `live` false or error; both return first. NilAway cannot correlate three results. Tests:
  `internal/adapters/runtime/exclusive_status_test.go`.
- `internal/adapters/runtime/lease.go`, `DescribeHeavyLease`, result 0 of `readLeaseOwner`: nil requires error, checked
  first by the guard. Reassigning `err` loses NilAway's correlation. Tests: `tests/integration/lease_evidence_test.go`.

## Domain Literal Analysis

- form: standard-library repository Go test [`domain_literals.go`](../../../../../tests/support/domain_literals.go),
  bound to "Production code compares no domain value with a literal" in
  [`quality-gates.feature`](../../../../../specs/behaviours/quality-gates.feature). No private golangci-lint build or
  new test-support dependency.
- reach: running-platform non-test `cmd/`/`internal/` files. Its header records both rules/name list. Platform-specific
  collection in `internal/adapters/host` contains no domain name.
- findings: zero required; fix violations in place. No allowlist, waiver or exemption.
