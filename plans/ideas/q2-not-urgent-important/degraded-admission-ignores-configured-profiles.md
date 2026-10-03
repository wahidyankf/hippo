# Degraded Admission Ignores Configured Profiles

Under a stable macOS memory-pressure warning, HIPPO's degraded path admits ephemeral work only when the resolved profile
is literally named `balanced`. A configured profile — even one that `extends: balanced` — never takes the degraded path,
so the configuration reference's own example profile defers every attempt for as long as the warning lasts, while the
built-in `balanced` profile on the same host proceeds at concurrency 1.

Filed 2026-10-03 by a consumer repository under the [upstream tool defects][utd] standard. The consumer has a
workaround, so this is filed in Q2 and not fixed yet: the defect is important, because a documented configuration
starves indefinitely with no hint why, but nothing is blocked while the workaround holds.

## Problem and Evidence

**Description.** The degraded path in the admission loop of `Run`, `internal/guard/run.go` lines 899–903, requires
`config.Resolution.ResolvedProfile == "balanced"`. `Catalog.Resolve` (`internal/policy/profiles.go` line 238) sets
`ResolvedProfile` to the configured profile's own name, never to its `extends` base, so `local-constrained` and
`local-balanced` both fail the comparison. Everything else the path checks is profile-independent:
`WarningAdmissionReady` (`internal/policy/policy.go` line 357) takes no profile, and `profilePolicy` derives the warning
floor the same way for every profile. The name is the only thing that differs.

**Steps to reproduce** on macOS, from a clean clone of this repository at `v0.8.3` or `main`:

1. Copy `hippo.local.json.example` to a scratch directory as `constrained.json`. It is a schema-3 reservation
   configuration with an 8 CPU / 16 GiB pool whose `defaultProfile` is `local-constrained` (`extends: constrained`,
   `fallback: minimal`, `maxCpuUtilizationPercent: 90`), the same profile as the example in
   [configuration](../../../docs/reference/configuration.md#profiles).
2. Make two variants of it: `balanced.json`, with `defaultProfile: "balanced"` and `profiles: {}`; and
   `extends-balanced.json`, whose `local-balanced` profile sets `extends: balanced`, `fallback: constrained`,
   `maxConcurrency: 2`, and `maxCpuUtilizationPercent: 90`.
3. Wait until `sysctl -n kern.memorystatus_vm_pressure_level` reads `2` while the compressor and swap are not growing.
   No synthetic way exists to hold the level for the compiled binary: `tests/contract/contract.go` exempts the degraded
   scenarios because synthetic samples "cannot be injected through the compiled binary". The level is sticky while the
   compressor holds a large payload, so a busy workstation often stays there with no growth at all.
4. With `HIPPO_CONFIG` unset and `HIPPO_ROOT` pointing at an empty scratch directory, run each variant back to back:
   `./hippo run --config <scratch>/<variant>.json --class ephemeral --resource-tier light --disk-path . -- true`.
5. Read `./hippo status --json --config <scratch>/<variant>.json` for each variant.

**Expected.** [Resource policy](../../../docs/reference/resource-policy.md#degraded-admission-on-macos) says "Balanced
**ephemeral** work on Darwin may admit after a full stable warning window", and lists the work that cannot: "Services,
fallback profiles, Linux PSI, transactional work, and releases". Configured profiles are not on that list, and nothing
in [configuration](../../../docs/reference/configuration.md#profiles) says a profile override gives up the path. A
reader would expect `local-balanced`, which derives from `balanced`, to admit as `balanced` does, and the more modest
`local-constrained` (concurrency 2) either to admit or to be named as excluded.

**Actual.** On 2026-10-03, three interleaved rounds on one host, with the pressure level read before and after each run:

```text
balanced           level 2  exit 0
  HIPPO admitting ephemeral child under stable macOS warning pressure with concurrency 1.
local-balanced     level 2  exit 124
  HIPPO deferred task: safe admission was not reached.
  hippo: [hippo.limit.capacity-deferred] capacity deferred this work; retry when the host is quieter
local-constrained  level 2  exit 124
  HIPPO deferred task: safe admission was not reached.
  hippo: [hippo.limit.capacity-deferred] capacity deferred this work; retry when the host is quieter
```

Every round gave the same result, and each deferral took about 17 seconds. Once the level fell to `1`, all three
variants admitted at once with exit 0. `status --json` reported `"decision": "wait"` and `"retryable": true` under
`profile` for every variant at level 2, including `balanced`, so `status` cannot tell an operator that one profile would
admit and the others would not (`withAssessmentDecision`, `internal/cli/development.go` lines 93–105, maps any
non-normal state to `wait`).

On the consumer's workstation the same `local-constrained` configuration, invoked through the pinned `v0.8.3` release,
deferred every attempt for well over an hour while the level stayed at `2` with zero compressor growth, zero swap-out,
about 14.7 GiB available, and CPU near 15%.

**Environment.** HIPPO `v0.8.3` (`6878c25`) for the consumer's runs; `main` at `209718c` built with `./hippo` for the
transcript above. `cmd/`, `internal/`, `go.mod`, and `go.sum` are identical between the two, so trunk behaves the same.
macOS 15.5 on arm64 with 32 GiB of memory and 12 cores; Go 1.27.1.

**Peculiarities.**

1. The profile that asks for less — concurrency 2 and a smaller reserve — is strictly worse under stable warning: it
   starves for as long as the level holds, while `balanced`, which asks for more, proceeds at concurrency 1.
2. The profile the configuration reference shows as its example is the one that starves, and neither page warns of it.
3. A profile that `extends: balanced` loses the path, so any tuning override, such as `maxCpuUtilizationPercent`,
   silently disables degraded admission.
4. `status` reports `wait` for every profile, so it cannot predict the difference.

## Why Now

Every consumer that follows the configuration reference on macOS is exposed, and a sticky level 2 is ordinary on a
memory-heavy workstation. The deferral message tells the reader to retry when the host is quieter, which sends them to
wait on a condition that may not change for hours.

## Prior Art

Read 2026-10-03:

- **Duplicate search.** `gh issue list --state all` returned no issues at all; `--search` for `degraded`,
  `warning pressure`, `profile`, `capacity-deferred`, and `memory pressure` each returned nothing.
  `gh pr list --state open` was empty. `gh pr list --state all --search` for `degraded`, `warning pressure`, and
  `profile` returned #33, #63, #65, #87, #88, #89, #90, #91, #94, #98, #111, #112, #117 (closed), and #118, all merged
  unless noted; none touches the profile condition on the degraded path (#118 rewrote the resource policy reference and
  specified the warning shed). `plans/in-progress/` and `plans/backlog/` hold no plan, and every `plans/ideas/` quadrant
  holds no brief. A search of `plans/` for `degraded`, `warning admission`, `warning pressure`, `extends`, and
  `local-constrained` found one mention, in
  [isolate test coordination state](../../done/2026-09-26__isolate-test-coordination-state/README.md): a refused
  admission under `local-constrained` that the plan attributed to a coordination-protocol mismatch, which is a different
  cause. `git log --grep` for `degraded` found only `abc9094` and `4f7b15c`, both test-support changes. No duplicate
  exists.
- **Specification.** `specs/behaviours/admission.feature` scenario "Stable macOS warning admits degraded work" is bound
  in `tests/support/driver.go` (`assessAdmission`, lines 441–467), which resolves only `policy.BuiltinCatalog()` and
  repeats the same `ResolvedProfile == profileBalanced` check at line 462. No scenario exercises a configured profile
  under stable warning, which is how the exclusion stayed invisible. `stableWarningSamples` (line 393) is the synthetic
  warning window a regression case can reuse.
- **Documentation.** [Resource policy](../../../docs/reference/resource-policy.md#degraded-admission-on-macos),
  [configuration](../../../docs/reference/configuration.md#profiles), and the degraded-admission note in [map
  concurrency into your build tool][map-note] all say "balanced" without saying whether a profile derived from it
  counts.

## Workaround

Set `defaultProfile` to the built-in `balanced` with no profile override, or pass `--profile balanced`. Degraded
admission then works as documented. The cost is giving up every tuning field a configured profile could set.

## Proposed Direction

Decide what the path should key on, then make code, specification, and documentation agree. Options, in rough order of
preference:

- Key it on the profile's `extends` chain: a profile whose chain reaches `balanced` qualifies, and its own reserves and
  CPU ceiling still apply.
- Add an explicit per-profile opt-in, such as `degradedAdmission: true`, defaulting to the built-in profile's behaviour.
- Keep the exclusion, but state it beside the profile example and in the resource policy's exclusion list, and let
  `status` say when the degraded path is or is not available to the resolved profile.

Any code change needs a regression case at the policy boundary that resolves a configured catalog against the synthetic
warning window and fails today.

## Scope and Non-Goals

In scope: which profiles the degraded path admits, its documentation, and what `status` reports about it. Not in scope:
the warning thresholds, the class rules (services, transactional work, and releases stay excluded), or Linux PSI.

## Risks and Open Questions

- Should `constrained`, or a profile derived from it, ever admit under warning? Its reserves are smaller than
  `balanced`'s; letting it in may be safe, or may be the reason it was excluded. Nothing recorded says which.
- Does admitting a configured profile change the
  [public contract](../../../repo-governance/development/public-contract.md)? Exit statuses and reasons stay the same;
  only which configurations reach exit 0 changes.
- `internal/guard/reservation.go` lines 263–273 also key automatic owner shares on the resolved name, so a configured
  profile absent from `automaticOwnerShares` gets one share whatever it extends. The same chain-or-name decision may
  apply there; its impact is unmeasured.

## Success

The reproduction above admits `local-balanced` at level 2, `local-constrained` either admits or is refused with
documentation that predicted it, and `status` shows which. The regression case fails without the change. Promote this
brief to a bug-fix plan if the workaround stops holding, for example for a consumer that needs a tuned profile on macOS.

[map-note]: ../../../docs/how-to/map-concurrency-into-your-build-tool.md#note-on-degraded-admission
[utd]: ../../../repo-governance/development/upstream-tool-defects.md
