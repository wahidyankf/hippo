# Rust for Stricter Domain Modeling

Should HIPPO move from Go to Rust for closed enums, exhaustive `match`, and no implicit zero values, or can it get most
of that by modeling more strictly in Go?

Filed 2026-10-03 at the owner's request, as an idea and nothing more: no code, no plan, no release. Measurements are
against `main` at `898c29d`. The motivation is stricter linting and stricter domain modeling. Speed is not a factor. The
defect history below argues for hardening the Go first. Rust stays a triggered option, not the next step.

## Background

On a consumer workstation, every HIPPO-guarded invocation in one repository stalled for well over an hour under a stable
macOS memory-pressure warning. Its configuration used a configured profile, the way the configuration reference's own
example does. Under a stable warning, HIPPO's degraded admission path admits ephemeral work only when the resolved
profile is literally named `balanced`. A configured profile, even one that `extends: balanced`, never matches, so every
attempt was deferred with exit `124` and the advice to retry when the host is quieter.

Three gaps compounded:

1. **The rule keys on a name.** `internal/guard/run.go` lines 899–903 compare
   `config.Resolution.ResolvedProfile == "balanced"`. The same name decides automatic owner shares in a `switch` with a
   `default` arm (`internal/guard/reservation.go` lines 263–273).
2. **Lineage is discarded.** `apply` (`internal/config/config.go` line 371) starts by overwriting `base.Name` with the
   configured name. `buildCatalog` (line 435) resolves `extends` (lines 466–471) and keeps only the merged
   `policy.Profile`, which has no field recording where it came from (`internal/policy/profiles.go` lines 51–66).
3. **`status` cannot show the difference.** `withAssessmentDecision` (`internal/cli/development.go` lines 93–105) maps
   every non-normal state to `wait`, so `balanced` and the starving profile both read `wait`.

Nothing caught it. The test driver repeats the production rule instead of calling it: `assessAdmission`
(`tests/support/driver.go` lines 441–467) resolves only `policy.BuiltinCatalog()` and copies the same name comparison.
The scenario "Stable macOS warning admits degraded work" is also exempt from the compiled-binary adapter, because
synthetic pressure cannot be injected there (`tests/contract/contract.go`).

The consumer worked around it by switching to the built-in `balanced` profile. The fix is in Go, under the bug-fix plan
[`fix-configured-profiles-starve-under-macos-warning`][bug-fix-plan]. The plan is in progress and its fix has not merged
as of this filing. Its decision D-01 adds lineage as an internal inherited attribute.

That raised this question: would a language with algebraic data types and stricter default lints have made this class of
defect structurally unlikely?

## Problem and Evidence

**The class.** A domain concept carried as a string, an integer, or a flag rather than as a type, so the compiler cannot
say when a case is missing, a value is forged, or a default silently applies. HIPPO already uses typed string constants
(`policy.State`, `policy.TaskClass`, `policy.Decision`, `status.Code`), and `.golangci.yml` runs every golangci-lint
linter (`default: all`, v2.13.2). The confirmed list includes `exhaustive`, `gochecksumtype`, `forcetypeassert`, and
`nilnil`. `exhaustruct` is disabled as brittle. HIPPO is already close to the strictest Go lint setup available. What
remains are the gaps a linter cannot see:

- A run's outcome is a string that starts as a default: `outcome := "capacity-deferred"` (`run.go` line 810). Every path
  that forgets to overwrite it reports a deferral.
- `RunOutcomes` (`run.go` lines 51–59) is a hand-kept list of those strings.
- The v0.8.0 exit vocabulary ([#53](https://github.com/wahidyankf/hippo/pull/53)) kept the old statuses `73`, `74`, and
  `75` as internal integer reason identifiers, mapped to public codes at the command-line boundary (`run.go` lines
  22–33).

**The history.** `git log --grep='^fix' -E origin/main` at `898c29d` returns 64 commits. Six are not `fix` commits
(`ab12ea4`, `ec20d54`, `8687a3e`, `1132541`, `90f5fbc`, `ac76575`). Each remaining commit is classified by the question
"would Rust's defaults have stopped this before review?":

| Class                                        | Fixes | Rust by default                    |
| -------------------------------------------- | ----: | ---------------------------------- |
| Modeling: a concept as a string or integer   |     5 | prevents, or forces the decision   |
| Boundary classification: a catch-all reason  |    16 | forces a choice, not the right one |
| Concurrency and shared state                 |     4 | 1 at compile time; 3 not           |
| Logic, policy, contract wording              |    13 | no                                 |
| Platform and environment evidence            |     4 | no                                 |
| Repository tooling, test harness, governance |    15 | out of scope: not product code     |
| Unclassified (`fb04187`, no message body)    |     1 | unknown                            |

- **Modeling (5).** `41bda15` and `529b506`: a cancelled or failed admission summarized as `capacity-deferred` because
  that string was the default. `e23151a`: `history` refused two real outcomes missing from the hand-kept list.
  `0efd802`: one internal integer meant both a pressure shed and a deferral, so the shed's own reason was never emitted.
  `be03781`: the remap of internal numbers missed one post-launch case. The in-flight profile defect is a sixth of the
  same kind. A closed `enum` with no default and a compiler-checked `match` blocks each one, and so would a Go type used
  with the same discipline. The difference is that Rust enforces it and Go needs a reviewer to insist.
- **Boundary classification (16).** `8fca1eb`, `f542f62`, `6fdd005`, `b1ab4b8`, `a9b0bc3`, `59adbab`, `3feb4d2`,
  `bd1e87f`, `11b6d06`, `c6c3157`, `b7e0687`, `8a11f9d`, `0f9b20a`, `64a31b7`, `f854d77`, and `88f776c`. An untyped
  `error` was classified late and fell into whichever code was nearest, often `hippo.supervision.failed`,
  `hippo.policy.replan-required`, or `capacity-deferred`. A closed error enum with an exhaustive mapping makes the
  catch-all visible, but someone still has to pick the right variant. The two invalid-flag-combination fixes (`f854d77`,
  `88f776c`) are library features in either language.
- **Concurrency (4).** `7187870`, an unsynchronized writer shared with an `os/exec` goroutine, is a compile error in
  safe Rust. Go's race detector already caught it in the gate. `d9ab21a` and `7bf37f3` come from `select` picking
  randomly among ready cases. `tokio::select!` does the same unless told `biased;`. `311653a` is an inter-process lock
  window. Rust stops none of the three. Two related non-`fix` cases also fall outside it: the test-only `ec20d54`, a
  garbage-collected `flock` (Rust's deterministic drop would make that one fail every time instead of now and then), and
  the archived plan
  [repair supervision readiness race](../../done/2026-09-26__repair-supervision-readiness-race/README.md), a race
  between processes.
- **Logic (13)**, **platform (4)**, and **tooling (15)**: help text, parsing order, environment precedence, a wrong
  model of the process tree (`5722854`), a conformance runner still expecting the retired exit `75` (`daaac15`, a
  child's exit status is an integer in any language), cgroup page cache (`475754a`), GNU versus BSD tools, and gate
  scripts. A language change does not touch these.

Of the 42 product-code fixes, 6 (14%) are ones Rust's defaults would have stopped. Another 16 (38%) are ones a closed
error type would have turned into an explicit decision. The remaining 20 (48%) no language prevents. The five modeling
fixes are also within reach of Go typed modeling, the linters already enabled, and one shared decision type. Go's race
detector caught the sixth, the data race, before it merged.

## Why Now

Not urgent. A workaround holds and the bug-fix plan is underway. It is important because the class recurs: six modeling
defects (five fixed, one in flight) and sixteen catch-all classifications in one month of history, one of which stalled
a consumer. Writing this down now stops the "rewrite it in Rust" suggestion from being re-argued from scratch at the
next such defect.

## Strictness: Rust's Defaults Against Go's Reach

| Concern               | Rust, by default                | Go, at best                                   |
| --------------------- | ------------------------------- | --------------------------------------------- |
| Closed sets           | `enum` is closed                | typed constants; any literal or JSON converts |
| Missing cases         | `match` must cover all (E0004)  | `exhaustive`: `switch` only, not `if` or `==` |
| Variants with data    | enum payloads                   | sealed interface plus `gochecksumtype`        |
| Complete construction | every field named (E0063)       | zero values; `exhaustruct` opt-in, brittle    |
| Every path assigns    | definite initialization (E0381) | every variable starts at its zero value       |
| Distinct identities   | newtype, no implicit conversion | named type; untyped constants convert freely  |
| Absence               | `Option`, no null in safe code  | pointers; `nilaway` only as a plugin          |
| Data races            | `Send`/`Sync` at compile time   | `-race` at test time                          |
| Lint strictness       | clippy `pedantic` via `[lints]` | golangci-lint `default: all`, already on      |

Go can match closed handling of a sum type, typed decision values, and validated construction behind unexported fields,
type by type and with discipline. Four things it cannot match structurally:

- It has no closed enum: any value of the underlying type is a member.
- Every type has a zero value.
- It does not check that each path assigns a variable before use.
- Exhaustiveness is checked only where a `switch` is written. `withAssessmentDecision`, an `if` chain, is invisible to
  `exhaustive`.

## Prior Art

Read 2026-10-03:

- **Duplicate search.** `gh issue list --state all --search` for `rust`, `rewrite`, `port`, `migrate`, `sum type`, and
  `exhaustive` returned no issues. The repository has none. `gh pr list --state all --search` returned these:
  - `rust`: #12 only, which mentions a sibling Rust command-line tool in passing.
  - `migrate`: #50, which adopted the RHINO lifecycle.
  - `exhaustive`: #91, about consumers matching reasons exhaustively.
  - `rewrite` and `port`: governance and fix pull requests, none about the language.
  - Open pull requests: none.
  - `plans/` and `docs/` searched for `rust` and `rewrite`: nothing about the implementation language.

  This brief has no duplicate.

- **Exit vocabulary.** [#53](https://github.com/wahidyankf/hippo/pull/53) closed the public reasons and kept the
  internal integers "so the ~45 call sites below the boundary keep their meaning without a rewrite". Today 43 production
  lines still reference the five internal status constants.
- **RHINO**, already Rust, at
  [`3610737`](https://github.com/wahidyankf/rhino/tree/361073763dc3dfb505c3364ead3d9e0ec12792d5):
  - one crate plus an `xtask` workspace member (`test-quick`, `self-validate`, `schema`, `dist`, `checksums`);
  - `#![forbid(unsafe_code)]`, `clippy -D warnings` on the default groups, and a cognitive-complexity threshold of 20;
  - `cargo-deny`, and a toolchain pinned to 1.95.0;
  - a release matrix with one native runner per platform, because "a cross-built Darwin binary has never started on
    Darwin" (`.github/workflows/release.yml` lines 10–12).

  Its library header calls its validators "read-only, network-free, and process-free". It holds no process supervision
  HIPPO could share.

- **Go analyzers.** [exhaustive](https://github.com/nishanths/exhaustive) (`3d3f086`) checks expression `switch`
  statements over enumerated types. [go-check-sumtype](https://github.com/alecthomas/go-check-sumtype) (`ae6904d`) needs
  a `//sumtype:decl` annotation per sealed interface. [NilAway](https://github.com/uber-go/nilaway) (`acb8859`) runs in
  golangci-lint only as a privately built plugin.

## Options

**1. Stay in Go and model strictly.**

The changes:

- A typed `Outcome` with no valid zero value, written to its wire string in one exhaustive `switch`.
- Typed profile lineage, which the bug-fix plan starts.
- One admission-decision sum type, produced by one policy function and consumed by `run`, `status`, and the test driver.
- Internal integer reasons replaced by `status.Code`-typed failures.
- Strict `UnmarshalJSON` on every enum, so unknown strings are refused at the boundary.
- `exhaustive` extended to map literals.
- `exhaustruct_v5` scoped to decision types only.
- A trial of NilAway.

What it prevents: all five modeling fixes and the in-flight defect, plus the catch-all half of the boundary class.
Effort comes from the number of string and integer sites, from 268 Gherkin scenarios that must keep passing, and from
lint rules that need justification. Risk: discipline decays. Go will compile the next `== "balanced"` without complaint,
so a custom rule or review must catch it.

**2. Incremental Rust behind the identical command-line contract.**

Go↔Rust FFI means cgo:

- It ends `CGO_ENABLED=0` static cross-builds (`scripts/build-release.sh` line 105).
- It puts two runtimes in one supervising process.
- It would only serve the pure policy core, which is the cheapest part to harden in Go.

The realistic shape is a second binary built in the same repository and ported module by module, in this order: policy
and configuration, then evidence formats, then coordination and reservations, then supervision. One release switches to
it once parity holds. Each archive must still hold exactly one member named `hippo`, so nothing ships half-ported.

What it prevents: the six stoppable fixes by construction, plus compile-time data races. Effort comes from 13,145
production lines and 15,892 lines of Go step bindings in `tests/support/`. Those bindings call Go internals and would be
rewritten.

The risk is the test oracle. Of 268 scenarios, 214 are exempt from the compiled-binary adapter (lease ownership 47,
process control 41, consumer harness 34, host evidence 25, and smaller boundaries). Only 54 scenarios, the 52 CLI
conformance assertions, and the `hippo-conformance` harness test a binary from outside. The exempt boundaries are where
HIPPO's hardest behaviour lives, so the port's parity would rest on newly written Rust bindings.

**3. Full rewrite.** It has the same constraints as option 2, with no parity period and no running comparison. Its
benefits are option 2's, and the risk is greater.

## Constraints Any Port Must Keep

A port must keep all of these:

- every command and flag ([CLI](../../../docs/reference/cli.md));
- exit statuses and reasons ([exit codes](../../../docs/reference/exit-codes.md));
- JSON bodies ([JSON schemas](../../../docs/reference/json-schemas.md));
- configuration schemas 1–3 ([configuration](../../../docs/reference/configuration.md));
- the [public contract](../../../repo-governance/development/public-contract.md);
- the [state-root](../../../docs/reference/state-root.md) format and coordination protocol.

The state root is the hardest. Consumers pin releases independently and share one state root, so Go and Rust clients
would hold live epochs side by side. Both must lock the same files with `flock`, read each other's ledgers, and agree
wherever bytes are compared. `encoding/json` and `serde_json` differ in HTML escaping and float formatting by default.

Releases are a further constraint. Each publishes four archives named `hippo_<version>_<os>_<arch>.tar.gz` for darwin
and linux on amd64 and arm64, plus `checksums.txt`, all reproducible from any host
([install a pinned release](../../../docs/how-to/install-a-pinned-release.md)). Rust names its targets with triples,
links glibc unless built for musl, and needs native Darwin runners, so this one costs Rust more. The consumer `./hippo`
bootstrap downloads those exact asset names. This repository's own `./hippo` builds from Go source.

## Beyond Defects

The case for one toolchain:

- It would match RHINO: one `rustup` pin, one clippy policy, and one `xtask` pattern.

Shared crates are thin: release packaging and checksums at most. Sharing a crate between independent repositories would
add coupling that [repository independence](../../../repo-governance/principles/repository-independence.md) argues
against.

The case for Go, which fits this job well:

- `SysProcAttr` covers process groups, foreground terminal ownership, and the controlling terminal.
- Goroutines and `select` cover supervision timers.
- One host cross-compiles all four static targets.
- Production depends only on the standard library, `cobra`, and `x/sys`.

A Rust port needs `unsafe` `pre_exec`, or `nix`/`rustix` for `tcsetpgrp`, `flock`, and signals. It also needs `clap`,
`serde`, and `flate2`, each a [dependency to select](../../../repo-governance/development/dependency-selection.md).

## Scope and Non-Goals

In scope: whether the implementation language should change, and what Go hardening would make it unnecessary. Not in
scope: changing any public contract, performance, the in-flight bug fix (it proceeds in Go as planned), and any
consumer.

## Risks and Open Questions

- Would option 1's discipline hold without a compiler behind it? A rule that refuses comparisons of profile names
  against string literals, and of outcomes against untyped strings, is the cheapest test.
- Can the compiled-binary oracle reach the exempt boundaries? Synthetic host evidence through the binary would help any
  option, and is a precondition for a credible option 2.
- Is anything compared byte for byte across implementations, such as history compaction keys or configuration hashes?
  Unmeasured.

## Recommendation and Success

**Recommendation: option 1, and keep option 2 parked behind triggers.**

- Every modeling defect in the history is reachable in Go with typed modeling and the linters already running.
- Rust's compile-time race freedom would have moved one concurrency fix from test time to compile time.
- 48% of product fixes are ones no language prevents.
- A port's parity oracle is mostly code that has not been written.

Success for option 1 has three signals:

- No string or integer stands for an outcome, a lineage, or an admission decision.
- `run`, `status`, and the test driver share one decision.
- A lint fails on the next `== "balanced"`.

Promote option 1 to a formal plan once the bug-fix plan lands. Promote option 2 only when one of these happens:

- two modeling-class defects of a kind Go cannot catch reach tagged releases after option 1 lands;
- a data race escapes `-race` into a release;
- a concrete, recorded need for HIPPO and RHINO to share code appears;
- the compiled-binary adapter covers the exempt boundaries, so parity becomes provable.

[bug-fix-plan]: ../../in-progress/fix-configured-profiles-starve-under-macos-warning/README.md
