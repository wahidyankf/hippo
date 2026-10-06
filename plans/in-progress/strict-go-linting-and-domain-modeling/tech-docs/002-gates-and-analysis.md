# Gates and Analysis

Unit 1 lands every gate; later units only narrow the analysis allowlist and widen `exhaustruct_v5`'s enforce patterns.
Each gate is clean before it turns on, per
[lint strictness](../../../../repo-governance/development/quality/checks/lint-strictness.md).

## Lint Configuration

| Setting in `.golangci.yml`   | At `892c462`                    | After                               | Unit |
| ---------------------------- | ------------------------------- | ----------------------------------- | ---- |
| `govet`                      | no settings; `nilness` not run  | `enable: [nilness]`                 | 1    |
| `exhaustive`                 | defaults (`switch` only)        | `check: [switch, map]`              | 1    |
| `gochecksumtype`             | defaults, no annotated type     | unchanged: no sealed interface      | —    |
| `exhaustruct_v5`             | disabled                        | `enforce-patterns`: the new structs | 2    |
| `exhaustruct`                | disabled                        | unchanged; v5 supersedes it         | —    |
| `nolintlint`                 | `allow-unused: false`, specific | unchanged                           | —    |
| `-race` in `scripts/test.sh` | full gate                       | unchanged                           | —    |

**Baseline.** A scratch probe of the first two rows on 2026-10-06 at `892c462` (a copy of `.golangci.yml` passed with
`-c`) reported nothing for `nilness` and one `exhaustive` finding: `internal/status/status.go`, line 179, the
`retryable` map, which names two of the 18 codes. Unit 1 spells out all 18 keys, each `false` but those two, so a new
code cannot land without a retryability decision.

**`exhaustruct_v5` scope.** golangci-lint v2.13.2 reads `enforce-patterns`, `ignore-patterns`, and related keys
(`pkg/config/linters_settings.go`, `ExhaustructV5Settings`). It is enabled in Unit 2, the first unit that creates a
struct type, with one pattern per new struct: `evidence.RecordedOutcome` (Unit 2), `policy.AdmissionInput` (Unit 5), and
`policy.RecordedTaskClass` (Unit 6). The exact pattern syntax (full import path or package-qualified) is confirmed from
`dev.gaijin.team/go/exhaustruct/v5` at Unit 2 by planting an incomplete literal and seeing it refused. Its disabling
reason in `.golangci.yml` becomes a scoping comment.

As built in Unit 2 (2026-10-06): `dev.gaijin.team/go/exhaustruct/v5@v5.0.3` reads each pattern as a regex over the full
`import/path.TypeName`, for example `^github\.com/wahidyankf/hippo/internal/evidence\.RecordedOutcome$`, and in its
default implicit mode checks every struct literal whatever the patterns say, so `enforce-patterns` alone would not scope
it. Unit 2 therefore also sets `explicit-mode: true`. It adds a second pattern, for `evidence.RecordedBudgetOutcome`,
the other struct the unit creates; Units 5 and 6 add theirs the same way.

**`gochecksumtype`.** It checks `switch` statements over interfaces annotated `//sumtype:decl`. This plan models every
new concept as a scalar enum, because no member carries data the others lack, so `exhaustive` is the checker that
reaches them. The repository adapter records that `gochecksumtype` stays enabled with no annotated target, so the first
sealed interface someone adds is checked without a configuration change.

## NilAway

**Selection argument,** per [dependency selection](../../../../repo-governance/development/dependency-selection.md):

- _Needed._ `policy.Sample` carries optional readings as pointers, and nil-flow across functions is beyond `nilness`,
  which reasons within one function. No standard-library or toolchain analyzer covers it.
- _What it pulls in._ A tool-only dependency: `go.uber.org/nilaway` with `golang.org/x/tools`, `golang.org/x/exp`,
  `golang.org/x/sync`, `golang.org/x/mod`, and `github.com/klauspost/compress` (the modules a scratch `go run` fetched
  on 2026-10-06). None is imported by production code; `depguard` keeps it that way, and `govulncheck` covers the tree.
  The pin is not tool-only in one respect: by minimal version selection it raised shared modules, among them the
  production dependency `golang.org/x/sys` from v0.47.0 to v0.48.0, with `x/tools` v0.50.0, `x/mod` v0.41.0, `x/sync`
  v0.23.0, and newer `x/exp/typeparams` and `x/telemetry` pseudo-versions (Unit 1, 2026-10-06). Build, lint, and the
  quick gate pass under them, and the full gate's `govulncheck` covers the raised tree.
- _If abandoned._ It is a gate, not a library: removing the tool line and its `go.mod` directive retires it with no code
  change.
- _Boundaries._ It reads source only; it touches no network, process table, or file outside the module at run time.
- _Licence and version._ Apache-2.0. No tags exist, only pseudo-versions; the latest,
  `v0.0.0-20260918162853-acb8859b9031` (2026-09-18, <https://proxy.golang.org/go.uber.org/nilaway/@latest>), requires Go
  1.26.0 or later (<https://github.com/uber-go/nilaway/pull/520>). Known open issues: generics false negatives
  (<https://github.com/uber-go/nilaway/issues/103>) and an `iter.Pull` false positive
  (<https://github.com/uber-go/nilaway/issues/317>).

**Pin and run.** `go get -tool go.uber.org/nilaway/cmd/nilaway@v0.0.0-20260918162853-acb8859b9031` adds it to `go.mod`'s
`tool` block, as golangci-lint, `goimports`, `govulncheck`, `gofumpt`, and `shfmt` are. `scripts/test-quick.sh` runs,
directly after `go tool golangci-lint run`:

```sh
go tool nilaway -include-pkgs=github.com/wahidyankf/hippo -pretty-print=false ./...
```

It exits `0` when clean, `3` with diagnostics, and `1` when loading fails, so `set -eu` stops the gate on either
failure. `-json` always exits `0` and is never used. A scratch run took about 11 seconds.

**Baseline.** The scratch run on 2026-10-06 at `892c462` reported eight diagnostics: two in production —
`internal/guard/exclusive_status.go`, line 60 (a `nil` returned by `liveExclusiveHeavyOwner` at line 125, then
dereferenced), and `internal/guard/lease.go`, line 315 (the result of `readLeaseOwner` read unguarded, with two more
sites at lines 322 and 327) — and six in test code: `tests/support/release_v04.go`, line 1416;
`internal/guard/run_test.go`, line 1909; `tests/integration/lease_evidence_test.go`, lines 41, 281, and 324; and
`tests/support/isolation_test.go`, line 119. Unit 1 classifies each as a hazard, fixed test-first, or a false positive,
excluded with its reason.

**Exclusions.** NilAway honours an inline `//nolint:nilaway` scoped to the node it is attached to
(<https://github.com/uber-go/nilaway/pull/320>), and also `-exclude-pkgs`, `-exclude-errors-in-files`, and
`-exclude-file-docstrings`. golangci-lint's `nolintlint` (`allow-unused: false`, `require-specific: true`) may report
`//nolint:nilaway` as unused, because NilAway is not a golangci-lint linter. Unit 1 runs that as a bounded checkpoint:
one exclusion is written inline and `go tool golangci-lint run` is run once. If it exits `0`, inline directives are the
mechanism. If `nolintlint` reports the directive, the fallback is predeclared: each excluded file is named in
`-exclude-errors-in-files` on the `scripts/test-quick.sh` line, and nothing is retried. Either way every exclusion's
reason is recorded in the repository adapter. Both production findings look unreachable by construction
(`liveExclusiveHeavyOwner` returns `nil` only with `live` false, and `readLeaseOwner` never returns a `nil` owner
without an error); the recorded answer to PW-7 ([delivery](../delivery.md#post-write-gate)) keeps the two-attempt
fallback rather than classifying them in advance.

## Domain Literal Analysis

**Form.** A behaviour scenario in `specs/behaviours/quality-gates.feature`, bound in `tests/support/steps.go` to
`tests/support/domain_literals.go`, which the unit and integration adapters run and the end-to-end adapter exempts, as
the other repository-configuration scenarios are. Its own behaviour is pinned by fixture tests in
`tests/support/domain_literals_internal_test.go`, run by the quick gate's `go test ./tests/support` line.

**Loading.** Standard library only (D4): `go list -export -json ./cmd/... ./internal/...` names each package's files and
export data; `go/parser` parses the non-test files; `go/types` checks each package with `go/importer.ForCompiler` in
`gc` mode reading that export data. No module is added, and `depguard`'s test-support list is unchanged. Bounded
checkpoint: if type checking cannot load every production package in one attempt, the predeclared fallback is a
syntax-only analysis keyed by the name list alone, which keeps rule (a)'s name-based reach and rule (b) whole, and the
adapter records the narrower reach.

**Rules.**

- **(a) Literal comparison.** An `==` or `!=` whose operands are an untyped string or integer constant written as a
  literal and a domain value, or a `case` clause listing such a literal in a `switch` over a domain value. A domain
  value is an expression whose type is a defined type declared in this module with a string or integer underlying type,
  or a field, variable, or parameter whose name is on the list. The empty string and `0` are not refused: they test
  presence, not identity (the recorded answer to PW-2, in [delivery](../delivery.md#post-write-gate)).
- **(b) Raw domain field.** A struct field or function parameter whose name is on the list, declared as `string`, `int`,
  or `bool`, in a non-test file under `cmd/` or `internal/`.

**Name list,** matched as whole identifiers, case-insensitively (the recorded answer to PW-1, in
[delivery](../delivery.md#post-write-gate)): `Outcome`, `BudgetOutcome`, `Profile`, `ResolvedProfile`,
`RequestedProfile`, `Class`, `TaskClass`, `Decision`, `Lineage`, and `AdmissionPath`. The brief's `Path`, `Reason`, and
`State` are left off: at `892c462` they match 49 declarations and comparisons — 34 file-path parameters and fields,
eight reasons such as the human-readable `Assessment.Reason` and the receipt reason, and the receipt, owner-row, and
conformance state words — none of which this plan types. The admission path is named `AdmissionPath` so it never
collides with a file path. Raw command-line text stays in fields named `…Flag` (`outcomeFlag`, `classFlag`), which the
list does not match, and is parsed before use.

**Findings** print as `<path>:<line>: <rule>: <identifier>`, sorted by path and line, so a failure names what to fix.

## Ratchet

**Allowlist.** `tests/support/domain_literals_allowlist.go` holds `domainLiteralAllowlist`, one entry per current
violation: path, enclosing symbol, rule, and the unit that removes it. The scenario fails on a finding with no entry,
and on an entry with no finding, so the list can only shrink. A scratch syntax scan with the name list above found 31
candidates at `892c462` (21 fields, 5 parameters, 5 comparisons); Unit 1 records the analysis's own count.

**Removal by unit.**

| Unit | Removes entries for                                                                                |
| ---- | -------------------------------------------------------------------------------------------------- |
| 2    | outcomes and budget outcomes in `internal/guard/evidence.go`, `internal/evidence/history.go`, CLI  |
| 4    | profile names in `internal/policy`, `internal/guard`, `internal/cli`, and the owner-share `switch` |
| 6    | task classes in `internal/guard`, `internal/evidence`, and the CLI flags                           |

Because the scenario fails on an entry with no finding, each unit deletes an entry in the same delivery item that
removes its violation, never in a later one. After Unit 6's last removal the list is empty; Unit 6 then deletes
`tests/support/domain_literals_allowlist.go` and the code that reads it, so the analysis reports every finding as a
failure.

## Where Each Gate Runs

All of them run in `scripts/test-quick.sh`, which the `pre-push` hook and the pull-request workflow already run, so no
hook, registry entry, or workflow changes. [Quality gates](../../../../repo-governance/development/quality-gates.md)
gains NilAway and the analysis in its quick-gate order, and the
[repository adapter](../../../../repo-governance/development/quality/stacks/repository-adapter.md) records the pin, the
exclusions, the scoped `exhaustruct_v5`, and `gochecksumtype`'s empty target. Unit 1 placed that record in the adapter's
first module,
[Go analysis gates](../../../../repo-governance/development/quality/stacks/repository-adapter/001-go-analysis-gates.md),
because the adapter had no room for it under its word budget.
