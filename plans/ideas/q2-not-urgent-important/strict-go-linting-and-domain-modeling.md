# Strict Go Linting and Domain Modeling

Carry HIPPO's domain concepts as Go types instead of strings, integers, and booleans, and tighten the lint
configuration, so that a missing case, a forged value, or a silent default fails a gate instead of reaching a consumer.

Filed 2026-10-03 at the owner's request, as an idea and nothing more: no code, no plan, no release. Measurements are
against `main` at `898c29d`. The goal is stricter linting and stricter domain modeling, in Go.

## Background

On a consumer workstation, every HIPPO-guarded invocation in one repository stalled for well over an hour under a stable
macOS memory-pressure warning. Its configuration used a configured profile, the way the configuration reference's own
example does. Under a stable warning, HIPPO's degraded admission path admits ephemeral work only when the resolved
profile is literally named `balanced`. A configured profile, even one that `extends: balanced`, never matches, so every
attempt was deferred with exit `124` and the advice to retry when the host is quieter.

Four gaps compounded:

1. **The rule keys on a name.** `internal/guard/run.go` lines 899–903 compare
   `config.Resolution.ResolvedProfile == "balanced"`. The same name decides automatic owner shares in a `switch` with a
   `default` arm (`internal/guard/reservation.go` lines 263–273).
2. **Lineage is discarded.** `apply` (`internal/config/config.go` line 371) starts by overwriting `base.Name` with the
   configured name. `buildCatalog` (line 435) resolves `extends` (lines 466–471) and keeps only the merged
   `policy.Profile`, which has no field recording where it came from (`internal/policy/profiles.go` lines 51–66).
3. **The admission path is two booleans.** `admitted, degraded := false, false` (`run.go` line 847). Nothing names the
   degraded path as a value that `status` could report.
4. **`status` cannot show the difference.** `withAssessmentDecision` (`internal/cli/development.go` lines 93–105) maps
   every non-normal state to `wait` in an `if` chain, so `balanced` and the starving profile both read `wait`.

Nothing caught it. The test driver repeats the production rule instead of calling it: `assessAdmission`
(`tests/support/driver.go` lines 441–467) resolves only `policy.BuiltinCatalog()` and copies the same name comparison.
The scenario "Stable macOS warning admits degraded work" is exempt from the compiled-binary adapter, because synthetic
pressure cannot be injected there (`tests/contract/contract.go`). No scenario resolves a configured catalog.

The consumer worked around it by switching to the built-in `balanced` profile. The fix is in Go, under the bug-fix plan
[`fix-configured-profiles-starve-under-macos-warning`][bug-fix-plan]. That plan is in progress, and its fix has not
merged as of this filing; it shipped in v0.8.4 on 2026-10-03, so code cited here as current describes the code before
it. Its decision D-01 adds lineage as an internal inherited attribute. This brief asks the wider question: which
modeling and lint changes would make this class of defect fail a gate everywhere in HIPPO, not only on this path?

## Problem and Evidence

**The class.** A domain concept carried as a string, an integer, or a flag rather than as a type, so no tool can say
when a case is missing, a value is forged, or a default silently applies.

HIPPO already models part of its domain this way. It has typed string constants (`policy.State`, `policy.TaskClass`,
`policy.Decision`, `status.Code`). `scripts/test-quick.sh` line 25 runs golangci-lint v2.13.2 with `default: all`
(`.golangci.yml`), and the confirmed list includes `exhaustive`, `gochecksumtype`, `forcetypeassert`, and `nilnil`.
`exhaustruct` is disabled because complete literals made fixtures brittle. `scripts/test.sh` line 20 runs the race
detector.

The gaps are where the domain is not typed:

- A run's outcome is a string that starts as a default: `outcome := "capacity-deferred"` (`run.go` line 810). Every path
  that forgets to overwrite it reports a deferral.
- `RunOutcomes` (`run.go` lines 51–59) is a hand-kept list of those strings.
- The v0.8.0 exit vocabulary ([#53](https://github.com/wahidyankf/hippo/pull/53)) kept the old statuses `73`, `74`, and
  `75` as internal integer reasons, mapped at the command-line boundary (`run.go` lines 22–33). Today 43 production
  lines still reference those five internal constants.
- Profile identity is a string, and lineage is not recorded at all.

`exhaustive` checks only `switch` statements over enumerated types. It cannot see an `==` comparison against a string,
an `if` chain, or a default assignment.

**The history.** `git log --grep='^fix' -E origin/main` at `898c29d` returns 64 commits. Six are not `fix` commits
(`ab12ea4`, `ec20d54`, `8687a3e`, `1132541`, `90f5fbc`, `ac76575`), which leaves 58. Each remaining fix is classified by
the question "would stricter Go modeling and linting have stopped this before review?":

| Class                                        | Fixes | Stricter Go modeling               |
| -------------------------------------------- | ----: | ---------------------------------- |
| Modeling: a concept as a string or integer   |     5 | stops it                           |
| Boundary classification: a catch-all reason  |    16 | forces a choice, not the right one |
| Concurrency and shared state                 |     4 | no; `-race` already caught one     |
| Logic, policy, contract wording              |    13 | no                                 |
| Platform and environment evidence            |     4 | no                                 |
| Repository tooling, test harness, governance |    15 | out of scope: not product code     |
| Unclassified (`fb04187`, no message body)    |     1 | unknown                            |

- **Modeling (5).** `41bda15` and `529b506`: a cancelled or failed admission was summarized as `capacity-deferred`
  because that string was the default. `e23151a`: `history` refused two real outcomes missing from the hand-kept list.
  `0efd802`: one internal integer meant both a pressure shed and a deferral, so the shed's own reason was never emitted.
  `be03781`: the remap of internal numbers missed one post-launch case. The background defect is a sixth of the same
  kind, not yet fixed.
- **Boundary classification (16).** `8fca1eb`, `f542f62`, `6fdd005`, `b1ab4b8`, `a9b0bc3`, `59adbab`, `3feb4d2`,
  `bd1e87f`, `11b6d06`, `c6c3157`, `b7e0687`, `8a11f9d`, `0f9b20a`, `64a31b7`, `f854d77`, and `88f776c`. An untyped
  `error` was classified late and fell into whichever code was nearest, often `hippo.supervision.failed`,
  `hippo.policy.replan-required`, or `capacity-deferred`. A typed failure kind with an exhaustive mapping makes the
  catch-all visible. Someone still has to pick the right kind.
- **Concurrency (4).** `7187870` was a data race, and the race detector caught it. `d9ab21a` and `7bf37f3` came from
  `select` picking randomly among ready cases. `311653a` was an inter-process lock window.
- **Logic (13)**, **platform (4)**, and **tooling (15)** cover help text, parsing order, environment precedence, a wrong
  model of the process tree (`5722854`), a conformance runner still expecting the retired exit `75` (`daaac15`), cgroup
  page cache (`475754a`), GNU versus BSD tools, and gate scripts.

Of the 42 product-code fixes, 5 (12%) plus the background defect are ones the measures below would have stopped. Another
16 (38%) would have become an explicit decision instead of a silent fallback. The remaining 21 (50%) are outside what
typing can reach.

## Why Now

Not urgent: a workaround holds and the bug-fix plan is underway. It is important because the class recurs. History holds
six modeling defects (five fixed, one in flight) and sixteen catch-all classifications in one month, and one of them
stalled a consumer. The bug-fix plan's lineage attribute is the first piece of this. Doing the rest as one argued change
keeps it from being applied path by path.

## Prior Art

Read 2026-10-03:

- **Duplicate search.** `gh issue list --state all --search` for `lint`, `exhaustive`, `sum type`, `enum`, `nilaway`,
  `rust`, and `rewrite` returned no issues; the repository has none. `gh pr list --state all --search` found these:
  - `lint`: 39 pull requests, none about lint strictness or domain types. The closest are #58, which corrected an
    exclusion comment, and #37, which made shell files lint clean.
  - `exhaustive`: #91 only, about consumers matching reasons exhaustively.
  - `rust`: #12 only, which mentions a sibling Rust tool in passing.
  - `rewrite`: governance and fix pull requests, none about modeling.
  - `sum type`, `enum`, and `nilaway`: nothing.
  - Open pull requests: none.

  `plans/` holds no brief or plan on lint strictness or domain types beyond the bug-fix plan above. This brief has no
  duplicate.

- **Exit vocabulary.** [#53](https://github.com/wahidyankf/hippo/pull/53) closed the public reasons, and deliberately
  kept the internal integers to avoid rewriting about 45 call sites.
- **Go analyzers.**
  - [exhaustive](https://github.com/nishanths/exhaustive) (`3d3f086`) checks expression `switch` statements over
    enumerated types.
  - [go-check-sumtype](https://github.com/alecthomas/go-check-sumtype) (`ae6904d`) needs a `//sumtype:decl` annotation
    on each sealed interface.
  - [NilAway](https://github.com/uber-go/nilaway) (`acb8859`) runs standalone or as a privately built golangci-lint
    plugin, but not as a built-in linter.

## Proposed Direction

A sketch. Each measure names what it would have caught.

**Domain types.**

1. **Outcome as a closed type.** Its zero value is an invalid `unset` member. A single exhaustive `switch` writes the
   wire string, `RunOutcomes` is derived from the member list, and writing a summary while `unset` is a supervision
   failure. Catches `41bda15`, `529b506`, and `e23151a`.
2. **Typed internal reasons.** A closed failure kind, mapped to `status.Code` and the exit status in one exhaustive
   `switch`, replaces the `73`/`74`/`75` integers. Catches `0efd802` and `be03781`. For the boundary class, it makes
   every unmapped error a visible decision.
3. **Profile lineage as a type.** Each resolved profile carries its built-in base as a closed enum, set in
   `buildCatalog`, and policy rules key on that base, never on a name. This covers the background defect, the
   owner-share `switch` in `reservation.go`, and `Resolve`'s last-resort floor, which still applies only to the profile
   named `minimal` (`internal/policy/profiles.go`), so a configured profile derived from `minimal` that does not fit
   gets none. The bug-fix plan found that path and routed it here.
4. **Admission path as a type.** A closed value (normal, degraded under warning, deferred, cleanup, replan) replaces the
   `admitted, degraded` booleans.
5. **One admission decision.** A single policy function returns that value. `run`, `status`, and the test driver all
   consume it, and `withAssessmentDecision` becomes an exhaustive `switch`. This would have made `status` report the
   difference, and kept the driver from copying the rule.
6. **Strict decoding.** An `UnmarshalJSON` on every enum that refuses unknown strings, so configuration and evidence
   cannot carry a member that does not exist. Typed constants in Go accept any string from JSON today.

**Lint configuration.**

| Measure                            | HIPPO today                    | Change                                      |
| ---------------------------------- | ------------------------------ | ------------------------------------------- |
| `exhaustive`                       | on, defaults (`switch` only)   | also check map literals                     |
| `gochecksumtype`                   | on, no annotated sum types yet | annotate the admission-decision interface   |
| `exhaustruct_v5`                   | off, fixture brittleness       | on, scoped to decision and resolution types |
| Literals compared to domain fields | nothing                        | a repository analyzer test refuses them     |
| NilAway                            | not run                        | trial as a standalone `go tool` gate        |
| `govet` nilness                    | not configured explicitly      | confirm it runs; enable it if not           |
| `-race`                            | full gate                      | unchanged                                   |

The analyzer test catches the background defect directly. It would refuse `ResolvedProfile == "balanced"` and an outcome
compared with an untyped string. A small analyzer test fits the repository's existing scripted scans better than a
golangci-lint plugin. NilAway matters because `policy.Sample` carries optional readings as pointers.

**Scenarios.** Every profile-sensitive rule gets an outline that runs against a built-in profile, a configured profile
that `extends` it, and a configured profile outside its lineage. The driver resolves through the configuration path
rather than `BuiltinCatalog()`. The bug-fix plan's regression test (its D-04) is the first such case.

## Scope and Non-Goals

In scope: domain types for outcome, internal reason, profile lineage, admission path, and the admission decision; the
lint settings above; and scenario coverage for configured catalogs. Not in scope: any change to exit statuses, reasons,
JSON bodies, configuration, or the state root; the in-flight bug fix, which proceeds as planned; and concurrency and
platform defects, which typing does not reach.

## Alternatives Considered

**Rewriting HIPPO in Rust**, for closed enums, exhaustive `match`, and no implicit zero values by default. The owner
decided against it, for three reasons:

- **Contract surface.** Every command, flag, exit status, reason, JSON body, configuration schema, and release asset
  name would have to be reproduced exactly.
- **Mixed versions.** Consumers pin releases independently and share one state root, so two implementations would have
  to interoperate on live coordination state.
- **Cost.** The rewrite covers 13,145 production lines, plus 15,892 lines of Go step bindings that would be rewritten.
  Only 54 of 268 scenarios test the compiled binary from outside.

The history above also shows that every modeling defect is reachable in Go.

## Risks and Open Questions

- Discipline decays where no tool looks. The analyzer test and scoped `exhaustruct_v5` are the guard. Is that enough, or
  should new domain fields be refused as strings outright?
- Does scoped `exhaustruct_v5` bring back fixture brittleness? Scoping it to a short type list is meant to prevent that.
  Unmeasured.
- Does NilAway's false-positive rate on this codebase justify a gate? The trial answers it.
- Should `status` publish the admission path in its JSON body? That is a public-contract change and needs its own
  decision.

## Success

- No string, integer, or boolean stands for an outcome, an internal reason, a lineage, or an admission path.
- `run`, `status`, and the test driver share one decision.
- A gate fails on the next `== "balanced"`.

Promote this brief to a formal plan once the bug-fix plan lands and the owner asks for it.

[bug-fix-plan]: ../../done/2026-10-03__fix-configured-profiles-starve-under-macos-warning/README.md
