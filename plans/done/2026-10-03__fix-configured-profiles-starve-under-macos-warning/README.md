# Fix: Configured Profiles Starve Under macOS Warning

Status: Done (2026-10-03)

Under a stable macOS memory-pressure warning, the degraded admission path admits ephemeral work only when the resolved
profile is literally named `balanced`. Every configured profile, including the configuration reference's own example and
one that `extends: balanced`, defers each attempt for as long as the warning lasts, while the built-in `balanced`
profile on the same host proceeds at concurrency 1.

A second, related inconsistency joined the plan on 2026-10-03 (D-06): once a child is running, stable warning is
forgiven only for a child that was itself admitted through the degraded path, so whether a child survives a stable
warning depends on when it was admitted rather than on the host's risk.

This is a [bug-fix plan](../../../repo-governance/conventions/plans/010-bug-fix-plan.md). It replaces the idea brief
filed 2026-10-03 at `plans/ideas/q2-not-urgent-important/degraded-admission-ignores-configured-profiles.md` (commit
`71a55b5`, never merged) by a consumer repository under the
[upstream tool defects](../../../repo-governance/development/upstream-tool-defects.md) standard.

**Deviation, recorded.** The convention reserves a bug-fix plan for a defect that blocks the work in hand with no
workaround, and this one has a workaround. On 2026-10-03 the owner classified it as a HIPPO bug and directed the fix and
its release, which is the owner's request the convention accepts in its place. Under this repository's upstream tool
defects standard, that request also directs this plan's quality gate, its execution, and its release.

**Decisions the owner made, 2026-10-03.** The fix has more than one correct shape, so the owner chose one before this
plan was written, from three options laid out to them (see [Alternatives rejected](#alternatives-rejected)):

- **D-01.** Degraded admission follows the profile's `extends` lineage: a profile derived from `balanced`, directly or
  through other configured profiles, qualifies. The lineage is an internal inherited attribute; no configuration key is
  added.
- **D-02.** `status` reports whether the degraded path is available to the resolved profile.
- **D-03.** `hippo.local.json.example` and `docs/reference/configuration.md` recommend the built-in `balanced` profile,
  and the exclusion of a profile outside `balanced`'s lineage is stated next to the profile example and in
  `docs/reference/resource-policy.md`.
- **D-04.** A regression test resolves a configured catalog against the synthetic warning window and fails today.
- **D-06** (added 2026-10-03, after the plan landed). Once warning pressure is stable by the same criteria degraded
  admission uses, it does not count toward the shed grace for any running ephemeral child whose resolved profile's
  lineage reaches `balanced`, however that child was admitted. Unstable warning (swap-out or compressor growth past its
  threshold), critical pressure, and storage blocks shed exactly as today. Service work keeps its current behaviour.
- **D-05.** The fix is released as `v0.8.4` through
  [release cut](../../../repo-governance/workflows/maintenance/release-cut.md) once its regression test and the full
  release gate pass on the exact revision. Repinning consumers is out of scope.

With the design settled by the owner, one delivery unit carries both fixes, and no interface a consumer relies on moves
(see [Public contract](#public-contract)), so the plan stays a bug-fix plan rather than expanding to six documents.

## Bug Report

**Description.** `Run` in `internal/guard/run.go` admits ephemeral work under a stable Darwin warning only when
`config.Resolution.ResolvedProfile == "balanced"` (lines 900–903). A configured profile resolves under its own name, so
it never qualifies and is deferred with `hippo.limit.capacity-deferred` until the kernel's pressure level leaves `2`.

**Steps to reproduce** on macOS, from a clean clone at `v0.8.3` or `main`:

1. Copy `hippo.local.json.example` as it stands at `209718c` (`git show 209718c:hippo.local.json.example`; the fix
   changes the file) to a scratch directory as `constrained.json`. It is a schema-3 reservation configuration with an 8
   CPU / 16 GiB pool whose `defaultProfile` is `local-constrained` (`extends: constrained`, `fallback: minimal`,
   `maxCpuUtilizationPercent: 90`), the same profile as the example in
   [configuration](../../../docs/reference/configuration.md#profiles).
2. Make two variants of it: `balanced.json`, with `defaultProfile: "balanced"` and `profiles: {}`; and
   `extends-balanced.json`, whose only profile `local-balanced` sets `extends: balanced`, `fallback: constrained`,
   `maxConcurrency: 2`, and `maxCpuUtilizationPercent: 90`, and is the `defaultProfile`.
3. Wait until `sysctl -n kern.memorystatus_vm_pressure_level` reads `2` while the compressor and swap are not growing.
   No synthetic way exists to hold the level for the compiled binary: `tests/contract/contract.go` exempts the degraded
   scenarios because synthetic samples "cannot be injected through the compiled binary". Apple's [memory-status
   notes][xnu-notify] give the warning level a falling threshold above its rising one, so a busy workstation can stay at
   `2` with no growth at all.
4. With `HIPPO_CONFIG` unset and `HIPPO_ROOT` pointing at an empty scratch directory, run each variant back to back:
   `./hippo run --config <scratch>/<variant>.json --class ephemeral --resource-tier light --disk-path . -- true`.
5. Run `./hippo status --json --config <scratch>/<variant>.json` for each variant.

**Expected behaviour.** [Resource policy](../../../docs/reference/resource-policy.md#degraded-admission-on-macos) says
"Balanced **ephemeral** work on Darwin may admit after a full stable warning window", and lists the work that cannot:
"Services, fallback profiles, Linux PSI, transactional work, and releases". Configured profiles are not on that list,
and nothing in [configuration](../../../docs/reference/configuration.md#profiles) says a profile override gives up the
path. `local-balanced`, which derives from `balanced`, should admit as `balanced` does, and `local-constrained` should
either admit or be documented as excluded. `status` should let an operator tell which applies.

**Actual behaviour.** On 2026-10-03, on `main` at `209718c`, three interleaved rounds gave the same result, with the
pressure level `2` before and after every run:

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

Each deferral took about 17 seconds. Once the level fell to `1`, all three variants admitted at once with exit 0.
`status --json` reported `"decision": "wait"` and `"retryable": true` under `profile` for every variant at level 2,
`balanced` included.

On the consumer's workstation the same `local-constrained` configuration, run through the pinned `v0.8.3` release,
deferred every attempt for well over an hour while the level stayed at `2` with zero compressor growth, zero swap-out,
about 14.7 GiB available, and CPU near 15%.

**Environment.** HIPPO `v0.8.3` (`6878c25`) for the consumer's runs; `main` at `209718c`, built with `./hippo`, for the
transcript above. `cmd/`, `internal/`, `go.mod`, and `go.sum` are identical between the two. macOS 15.5 on arm64 with 32
GiB of memory and 12 cores; Go 1.27.1.

**Workaround.** Set `defaultProfile` to the built-in `balanced` with no profile override, or pass `--profile balanced`.
It gives up every tuning field a configured profile could set, which is why the owner chose the fix.

### Second Defect: Stable Warning Sheds Only Normally Admitted Children

**Description.** [Resource policy](../../../docs/reference/resource-policy.md#host-pressure-remains-authoritative) says
warning pressure "sheds once it outlasts the guarded child's class grace — 10 s for `ephemeral`", and "for a child
admitted under degraded macOS warning, warning that stays stable does not count toward it". So an ephemeral child
admitted under normal pressure is shed after 10 s of a warning that is stable — no swap-out growth, no compressor
growth, ample available memory — while an identical child admitted a moment later under the same stable warning runs to
completion. Survival depends on the moment of admission, not on the host's risk.

**Steps to reproduce.** The compiled binary cannot hold a synthetic warning, so the reproduction is the regression
scenario this plan adds, with the fixture under [Specification Changes](#specification-changes): admit an ephemeral
`balanced` child on healthy Darwin samples, then feed the guard a stable warning (level `2`, 8 GiB available, flat
compressor payload and swap-outs) for longer than a shortened ephemeral grace, while the child sleeps past it. Run with
`HIPPO_BDD_ADAPTER=integration go test -count=1 ./tests/bdd`.

**Expected behaviour.** The child finishes with its own exit status, as a child admitted through the degraded path under
the same samples does.

**Actual behaviour.** The guard prints `HIPPO shedding ephemeral child after memory-warning.` and returns the pressure
shed. This is read from the code (below) and confirmed by the RED step of Phase 2.

**Must still shed.** On the consumer's workstation (32 GiB, macOS arm64, HIPPO `v0.8.3`), a child admitted through the
degraded path was shed when its own container build grew the compressor payload from about 11.1 GB to about 12.9 GB
while swap-outs stayed flat. That growth is past `CompressorWarningGrowthBytes` (1 GiB in the default policy) with the
payload past `CompressorWarningPayloadBytes` (12 GiB), so the warning was not stable and the shed was right. The fix
keeps that case, and the plan turns it into a regression row.

## Duplicate Check

Run 2026-10-03 against `main` at `209718c`:

- `gh issue list --state all` returned no issues; `--search` for `degraded`, `warning pressure`, `profile`,
  `capacity-deferred`, and `memory pressure` each returned nothing.
- `gh pr list --state open` returned no pull requests. `gh pr list --state all --search` for `degraded`,
  `warning pressure`, and `profile` returned #33, #63, #65, #87, #88, #89, #90, #91, #94, #98, #111, #112, #117
  (closed), and #118, all merged unless noted. None touches the profile condition on the degraded path; #118 rewrote the
  resource policy reference and specified the warning shed.
- `plans/in-progress/` and `plans/backlog/` held no plan, and every `plans/ideas/` quadrant held no brief.
- `grep -rli` over `plans/` for `degraded`, `warning admission`, `warning pressure`, `extends`, and `local-constrained`
  found one mention, in
  [isolate test coordination state](../../done/2026-09-26__isolate-test-coordination-state/README.md): a refused
  admission under `local-constrained` that plan attributed to a coordination-protocol mismatch, a different cause.
- `git log -i --grep=degraded` found only `abc9094` and `4f7b15c`, both test-support changes.
- A web search for the defect found no report against HIPPO. The closest report against another tool is [luchta
  #347][luchta-347], "macOS memory pressure pauses on the advisory WARN level by default", filed 2026-09-17.

The only match is the brief this plan replaces. No duplicate exists.

## Root Cause

The degraded path is gated on a name, and the name a configured profile resolves under never carries its lineage.

1. **The gate.** `internal/guard/run.go`, in `Run`'s admission loop:

   ```go
   if config.TaskClass == policy.TaskEphemeral &&
   	config.Resolution.ResolvedProfile == "balanced" &&
   	policy.WarningAdmissionReady(samples, config.Policy) {
   	admitted, degraded = true, true
   ```

2. **The name.** `Catalog.Resolve` (`internal/policy/profiles.go`, line 238) sets `ResolvedProfile` to the key it found
   in the catalog, and a configured profile's key is its own name, such as `local-balanced`.
3. **The lost lineage.** `buildCatalog` (`internal/config/config.go`, lines 435–536, inheritance closure 446–489) builds
   a configured profile by copying its resolved parent (lines 466–471), or the built-in of the same name when it has no
   `extends` (line 455), and `apply` (line 371) then sets `Name` and the overridden fields. `policy.Profile`
   (`internal/policy/profiles.go`, lines 51–66) has no field recording which built-in profile a configured one derives
   from, so nothing after `buildCatalog` can tell `local-balanced` from an unrelated profile.
4. **No threshold explains it.** `WarningAdmissionReady` (`internal/policy/policy.go`, line 357) takes no profile.
   `profilePolicy` derives `WarningAdmissionMemoryBytes` the same way for every profile (25% of memory capacity, clamped
   4–8 GiB, `profiles.go` line 179). The two profile-specific thresholds it reads, `DiskWarningBytes` and
   `MaxCPUUtilizationPercent` (`policy.go` lines 377–379 and 392–396), were equal or looser for the configured profiles:
   `constrained`'s disk reserve is smaller than `balanced`'s, and the 90% CPU ceiling was far above the measured 10–20%.
5. **Why no test caught it.** The binding of "Stable macOS warning admits degraded work" (`tests/support/driver.go`,
   `assessAdmission`, lines 441–467) resolves only `policy.BuiltinCatalog()` and repeats the name check at line 462. No
   scenario resolves a configured catalog under stable warning.
6. **Why `status` cannot show it.** `withAssessmentDecision` (`internal/cli/development.go`, lines 94–106) maps any
   non-normal state to `wait`, and no field says whether the resolved profile could take the degraded path.

The mechanism is confirmed by the reproduction: the three variants shared every reading, every threshold
`WarningAdmissionReady` checks was equal or looser for the configured profiles, and only the one resolved under the name
`balanced` was admitted. The name is the only failing condition.

**Second defect.** `internal/guard/run.go`, in `Run`'s supervision loop (lines 1065–1072):

```go
assessment := policy.ResourceAssessment(samples, config.Policy)
stableDegradedWarning := degraded && policy.WarningAdmissionReady(samples, config.Policy)
if assessment.State == policy.StateNormal || stableDegradedWarning {
	warningSince = nil
} else if assessment.State == policy.StateWarning && warningSince == nil {
```

`degraded` is a local set only where the degraded path admits (line 903). A child admitted by `AdmissionReady` under
normal pressure leaves it `false`, so `stableDegradedWarning` is never true for it, `warningSince` starts at its first
warning sample, and the shed at line 1083 follows one grace later. `WarningAdmissionReady` itself needs nothing from the
admission path: the supervision loop keeps the last `TrendWindow / SampleInterval + 2` samples (lines 1038–1041),
including the admission samples, so the same stability judgement is available to every running child. Service work never
reaches the degraded path (the gate requires `policy.TaskEphemeral`), so it never had the exemption.

A precedent for the first fix already exists one function away. Schema-3 `automaticOwnerShares` inherit through
`extends`: `inheritShares` (`internal/config/config.go`, lines 257–277) walks a configured profile's `extends` chain
until it finds a share. Degraded admission is the one profile behaviour that does not.

## Solution

Give `policy.Profile` an inherited attribute that says whether degraded admission applies, set it on the built-in
`balanced` profile only, carry it into `policy.Resolution`, and gate the degraded path on it instead of on the name.

**The change.**

- `internal/policy/profiles.go`: `Profile` gains `DegradedAdmission bool` with the JSON tag `-`, so it is not part of
  any document and cannot be configured. `BuiltinCatalog` sets it on `balanced` alone. `Resolution` gains
  `DegradedAdmission bool` with the JSON tag `degradedAdmission`, and `Resolve` copies it from the profile it resolved
  to.
- `internal/config/config.go`: no change. `buildCatalog` starts every configured profile from a copy of its resolved
  parent, and `apply` touches only `Name` and the fields the override sets, so the attribute is inherited through any
  depth of `extends`. The configuration schema has no key for it, so a file cannot set it.
- `internal/guard/run.go`: the gate reads `config.Resolution.DegradedAdmission` in place of the name comparison.
- `tests/support/driver.go`: `assessAdmission` reads `resolution.DegradedAdmission` in place of its copy of the name
  check, and `observeDegradedWarning`'s hand-built `balanced` resolution sets `DegradedAdmission: true`, as `Resolve`
  now would.
- `status --json` then reports `profile.degradedAdmission` with no change to `internal/cli`, because it already encodes
  the whole `Resolution`. The text line is unchanged.

**Why it removes the cause.** The gate no longer depends on a name. A profile's eligibility is decided once, where its
lineage is still known, and travels with the profile through `extends` the way every other inherited field does.

**Conditions beyond the reported one.**

- _A profile derived from a configured profile derived from `balanced`_: the copy is transitive, so it qualifies.
- _A configured profile named `balanced` that overrides the built-in without `extends`_: `buildCatalog` starts it from
  the built-in `balanced`, so it keeps the attribute, as its name did before.
- _A configured profile whose name and lineage disagree_: one named `balanced` with `extends: constrained` qualified by
  name before and no longer does; one named `constrained` with `extends: balanced` did not and now does. Lineage, not
  the name, decides, which is D-01; the changelog entry names the narrowing.
- _A `balanced`-derived profile that falls back_: `Resolve` returns the fallback profile's attribute. The built-in
  fallbacks `constrained` and `minimal` do not qualify, which keeps the documented "fallback profiles cannot use this
  path".
- _A profile derived from `constrained` or `minimal`_, including the documented `local-constrained`: it does not
  qualify, as today. The fix states this exclusion in the documentation and makes it visible in `status` instead.
- _Services, transactional work, releases, Linux_: unchanged; the task class and platform conditions stay in the gate
  and in `WarningAdmissionReady`.
- _Reserves and CPU ceiling_: a `balanced`-derived profile keeps its own; the degraded path's memory floor and its OOM,
  swap-out, and compressor checks are the same for every profile, and its disk and CPU checks use the profile's own
  reserve and ceiling.
- _Other name-keyed behaviour_, out of scope: the automatic owner-share default in `internal/guard/reservation.go`
  (lines 263–272) switches on the resolved name, but only when `automaticOwnerShares` has no entry, and `inheritShares`
  fills that entry through `extends` for every configured profile; and `Resolve` keeps its last-resort floor for the
  profile named `minimal` (`profiles.go` line 255). Neither starves work under warning. The second is routed through
  this plan's Learnings.

**`status`.** `profile.degradedAdmission` is `true` when the resolved profile may use the degraded path on macOS, and
`false` otherwise. It is a property of the profile, not a prediction: `status` takes two samples and cannot judge a full
stable window, so `decision` stays `wait` under warning. The reference says both.

**Documentation and example.** `hippo.local.json.example` sets `defaultProfile` to `balanced` and drops its `profiles`
entry, keeping every other key. [Configuration](../../../docs/reference/configuration.md#profiles) keeps documenting
profile overrides with an example derived from `balanced`, recommends the built-in `balanced` on macOS workstations, and
states that a profile outside `balanced`'s lineage never uses degraded admission, linking
[the resource policy section](../../../docs/reference/resource-policy.md#degraded-admission-on-macos). That section, the
degraded-admission note in [map concurrency into your build tool][map-note], and the status schema in
[JSON schemas](../../../docs/reference/json-schemas.md) say the same.

### Second Fix: Stable Warning Spares Every Eligible Child

**The change.** In `internal/guard/run.go`, replace `degraded &&` with the admission eligibility itself, and remove the
`degraded` local, which nothing else reads:

```go
stableWarning := config.TaskClass == policy.TaskEphemeral &&
	config.Resolution.DegradedAdmission &&
	policy.WarningAdmissionReady(samples, config.Policy)
```

A child admitted through the degraded path satisfies the first two conditions by construction, so its behaviour is
unchanged.

**Why it removes the cause.** Forgiveness of stable warning now depends on the child's class, its profile's lineage, and
the evidence, which are the same for both children in the report, and no longer on how the child was admitted.

**Conditions.**

- _Unstable warning_: swap-out growth at or past `SwapOutWarningBytes`, or compressor growth at or past
  `CompressorWarningGrowthBytes`, makes `WarningAdmissionReady` false, so the warning counts toward the grace as today.
- _Critical pressure_ sheds at once regardless of `warningSince` (line 1083), and _storage blocks_ make
  `WarningAdmissionReady` false (`policy.go` line 368), so both shed as today.
- _Memory below the warning-window floor_ (25% of capacity, clamped 4–8 GiB), or a sample that is not level `2`: not
  stable, counts as today. The existing scenario "Warning that outlasts the class grace sheds eligible work" holds
  memory at 6 GiB on level `1`, so it still sheds.
- _A young child_: stability needs a full `TrendWindow` of samples; warning in the child's first moments, before the
  window is full, counts toward the grace as today. This errs toward shedding.
- _Concurrency_: a normally admitted child keeps the concurrency it was admitted with; only admission through the
  degraded path forces `1`. Growth it causes makes the warning unstable and sheds it.
- _Service work_ is unchanged: the code never exempted it, and D-06 keeps it so.
- _Transactional work_ outside reservation coordination is never shed (line 1074), unchanged.

### Alternatives Rejected

Laid out to the owner on 2026-10-03, who chose D-01:

- _Let `constrained`-derived profiles use the degraded path too, under their own reserves._ It would fix the documented
  example without changing the documentation, but `constrained` is also `balanced`'s automatic fallback on a small host,
  so small and swapless hosts would start admitting under warning, and nothing recorded says why `constrained` was
  excluded. Rejected for now; a consumer that needs it can file it.
- _An explicit per-profile opt-in key, such as `degradedAdmission`._ Explicit and controllable, but it adds a
  configuration key, which the [public contract](../../../repo-governance/development/public-contract.md) protects, and
  the documented example would still starve unless set.
- _Documentation only._ It would leave `local-balanced` starving for a reason no reader would guess, and leave every
  tuning override silently disabling the path.

### Public Contract

No exit status, reason code, command path, flag, or configuration key changes. Which configurations reach exit 0 under a
stable macOS warning changes: profiles in `balanced`'s lineage now do, as the documentation already implied. The status
document gains one field, `profile.degradedAdmission`, in schema 5. The reference states that field order is not part of
any contract. HIPPO ships no production reader of the status document; its test reader, `requireStatus`
(`tests/support/driver.go`, lines 1888–1907), decodes with plain `json.Unmarshal` and ignores unknown fields.
`watch --json` emits the same schema-5 document, so it gains the field too. A consumer that rejects unknown fields in
that document would see the new one, which the changelog entry names for both commands. A configuration whose profile
name and lineage disagree changes as described under the conditions above, which the changelog entry also names. Under
D-06, a normally admitted ephemeral child whose lineage reaches `balanced` now finishes with its own status under a
stable macOS warning where it used to end in 124 naming `hippo.limit.pressure-shed`; the changelog entry names this too.
The release is a patch, `v0.8.4`.

### Specification Changes

- `specs/behaviours/admission.feature` `[E]`
  - `+` `Scenario: A configured profile derived from balanced admits degraded work`, `@e2e-exempt`: given a
    configuration whose default profile extends `balanced` and a full stable Darwin warning window with safe headroom,
    when the guard runs ephemeral work under that configuration, then it admits the child at concurrency 1 and says so
    on stderr.
  - `+` `Scenario: A configured profile outside balanced's lineage never uses degraded admission`, `@e2e-exempt`: the
    same with a profile that extends `constrained`, then the work is deferred naming `hippo.limit.capacity-deferred`.
  - = Preserve every other scenario; "Stable macOS warning admits degraded work" keeps its text and changes only its
    binding (above).
  - → Bindings: `tests/support/steps.go`, `tests/support/driver.go`, driving `guard.Run` with a `sequenceCollector` over
    `stableWarningSamples` and a resolution from `config.Load` on a temporary file. The end-to-end boundary cannot carry
    either, for the reason the sibling scenarios record, so `tests/contract/contract.go` gains an exact exemption for
    each with `hostEvidenceBoundary` and `syntheticHostPressureReason`.
- `specs/behaviours/public-cli.feature` `[E]`
  - `+` `Scenario Outline: JSON status says whether the resolved profile may use degraded admission`, with examples
    `balanced` and `constrained`: given a configuration whose default profile extends `<base>`, when JSON status is
    requested with that configuration, then `profile.degradedAdmission` is `true` exactly when the resolved profile is
    the one derived from `balanced`. Stated against the resolved profile so a host small enough to fall back still
    passes. It runs at all three boundaries. It reuses the existing step "JSON status is requested with that config",
    whose in-process binding `statusWithConfig` (`tests/support/driver.go`, lines 3404–3424) is changed to keep stdout
    in `driver.output`, as the end-to-end path already does, and adds a Given that writes the configuration and sets
    `driver.configPath`, and a Then that decodes `profile` as a map.
  - = Preserve every other scenario, including "Invalid explicit configuration is actionable", which uses the same step
    and does not read stdout.
- `specs/behaviours/execution.feature` `[E]`
  - `+` `Scenario: Stable warning spares a balanced ephemeral child admitted under normal pressure`, `@e2e-exempt`:
    given an ephemeral child of the built-in `balanced` profile admitted on healthy Darwin samples, when the host then
    holds a stable macOS warning past the ephemeral grace, then the child finishes with its own exit code.
  - `+` `Scenario Outline: Unsafe pressure still sheds a balanced ephemeral child admitted under normal pressure`,
    `@e2e-exempt`, with rows `growing compressor payload` (11.1 GB to 12.9 GB, swap-outs flat), `growing swap-outs` (128
    MiB within the window), `critical memory pressure`, each ending in exit 124 naming `hippo.limit.pressure-shed`, and
    `disk below its warning reserve`, ending in `hippo.limit.storage-blocked`.
  - `+` `Scenario Outline: Stable warning still sheds work outside the exemption`, `@e2e-exempt`, with rows
    `ephemeral child of a profile derived from constrained` and `service child of the built-in balanced profile`, each
    shed after its class grace naming `hippo.limit.pressure-shed`.
  - = Preserve every other scenario, including "Warning that outlasts the class grace sheds eligible work" and
    "Worsening warning sheds degraded work".
  - → Bindings: `tests/support/steps.go` and `tests/support/driver.go`. The fixture is fixed here so that stability
    never depends on host speed:
    - _Sample source._ A new `advancingCollector` in `tests/support/driver.go` returns 20 healthy Darwin samples
      (`healthySample`, level `1`, 12 GiB available, with its compressor payload set to 11.1 GB so the pressure samples
      add no growth), then the row's pressure sample forever. Every sample's `MeasuredAt` is `base + index × 1 ms`, so
      the synthetic clock advances by exactly one step per collection however long a tick takes, and the 17-sample
      supervision buffer always spans 16 ms. `sequenceCollector`, which repeats its last sample with the same timestamp,
      is not used for these scenarios.
    - _Policy._ `fastBehaviourPolicy()` (1 ms `SampleInterval`) with `TrendWindow` 15 ms and both
      `EphemeralWarningGrace` and `ServiceWarningGrace` 3 ms. The 3 ms graces are measured on the real clock
      (`Now: time.Now`); 15 synthetic steps take at least 15 real milliseconds, so any instability lasts longer than a
      grace.
    - _Pressure samples._ Stable warning: level `2`, 8 GiB available (the default warning-window floor), compressor
      payload 11.1 GB, swap-outs unchanged. Compressor row: the same, with the payload stepping once to 12.9 GB and
      holding, so the window shows 1.8 GB of growth for 15 steps. Swap-out row: swap-outs stepping once by 128 MiB and
      holding. Critical row: level `4` with 3 GiB available. Disk row: the stable warning with disk free below the
      default 30 GiB warning reserve and above the 20 GiB critical floor.
    - _Runner._ `runGuardedShellUnder` gains a sibling,
      `runGuardedShellAs(policy, resolution, taskClass, script, environment)`, which passes the `Resolution` and task
      class to `guard.Run`; `runGuardedShellUnder` delegates to it with the zero `Resolution` and `ephemeral`, unchanged
      for its callers. Resolutions come from `policy.BuiltinCatalog()` for `balanced`, and from `config.Load` on a
      temporary schema-2 file for the `constrained`-derived row.
    - _Child._ The spared scenario runs `sleep 0.5; printf d > "$CHILD_COMPLETED"` and requires exit `0` and the marker,
      so its runtime is the only cost. Every shedding row runs `sleep 10; printf d > "$CHILD_COMPLETED"`, as
      `observeLastingWarning` does, so the shed always lands before the child could finish on a loaded host; it requires
      the shed exit code, no marker, and, for the stable-warning rows, `memory-warning` named on stderr.
    - The end-to-end boundary cannot carry them, so each gains an exact exemption in `tests/contract/contract.go` with
      `processControlBoundary`, as its siblings do.
- `specs/architecture.md`: assessed; its "Policy engine and profiles" element describes profile resolution at a level
  this change does not alter. No change.

✓ Proof: `npm run test:quick`, whose behaviour adapters run the corpus and the contract check.

**References**, read 2026-10-03:

- HIPPO at `209718c`: [`internal/guard/run.go`][run-go], [`internal/policy/profiles.go`][profiles-go],
  [`internal/config/config.go`][config-go], [`internal/policy/policy.go`][policy-go], and
  [`tests/support/driver.go`][driver-go]. Primary source for every line cited above.
- [Apple XNU: memorystatus notifications][xnu-notify]: warning means the "device is beginning to experience memory
  pressure. Consider relaxing caching policy", and each level's falling threshold sits above its rising one. Primary
  source for why level 2 is ordinary and sticky.
- [luchta #347][luchta-347]: another admission tool paused on the advisory warning level by default and moved to pausing
  on critical. Background for why starving on warning is a defect rather than caution; HIPPO keeps its stricter
  stable-window checks.

### Acceptance Criteria

- **AC-01** Given a configured profile whose `extends` chain reaches `balanced` and a full stable Darwin warning window
  with safe headroom, when the guard runs ephemeral work, then it admits the child at concurrency 1 and prints the
  degraded-admission line.
- **AC-02** Given a configured profile whose `extends` chain reaches `constrained` or `minimal`, or a `balanced` request
  that falls back, under the same window, then the work is deferred naming `hippo.limit.capacity-deferred`, as today.
- **AC-03** Given any configuration, when `status --json` or `watch --json` reports a profile, then
  `profile.degradedAdmission` is `true` exactly when the resolved profile's lineage reaches the built-in `balanced`.
- **AC-04** Given the shipped `hippo.local.json.example`, then its `defaultProfile` is `balanced`, and the configuration
  and resource policy references state the lineage rule next to the profile example.
- **AC-05** Given a build of the fix on macOS at pressure level 2, when the reproduction runs, then `local-balanced`
  admits with the degraded-admission line and `local-constrained` is deferred. This is a manual check: the level cannot
  be held on demand, so it is attempted while the host sits at level 2, and recorded as not observable otherwise.
- **AC-06** Given the fix is merged, then a `v0.8.4` release publishes it with CI-built assets and `checksums.txt`.
- **AC-08** Given a running ephemeral child whose profile's lineage reaches `balanced`, admitted under normal pressure,
  when the host holds a stable macOS warning past the ephemeral grace, then the child is not shed and finishes with its
  own exit code.
- **AC-09** Given the same child, when the warning carries compressor growth (11.1 GB to 12.9 GB) or swap-out growth
  past its threshold, or pressure turns critical, or disk falls below its warning reserve, then it is shed exactly as
  before; and a `constrained`-derived ephemeral child or a service child under a stable warning is shed after its class
  grace as before.
- **AC-07** Given execution finishes, then this plan is gated, reconciled, and archived under `plans/done/`, and the
  worktree and its branches are gone.

## Delivery

**Execution checkout.** `~/ose-projects/hippo/worktrees/degraded-admission-profile-brief`, created from `origin/main`
for the brief and reused for every unit below, per
[worktree to pull request](../../../repo-governance/workflows/maintenance/worktree-to-pull-request.md). Unit 1 lands
from branch `worktree/degraded-admission-profile-brief`, and its D-06 amendment from
`worktree/fix-configured-profiles-starve-under-macos-warning-plan`; each later unit branches from `origin/main` in the
same directory, unit 2 as `worktree/fix-configured-profiles-starve-under-macos-warning`, unit 3 needs no branch, and
unit 4 as `worktree/fix-configured-profiles-starve-under-macos-warning-record`. HIPPO cannot guard its own gates, so
`npm run test:quick` and `npm test` run directly, per
[resource-aware development](../../../repo-governance/development/resource-aware-development.md).

**Commands** the items below name:

- _Focused tests_: `go test -count=1 -run 'DegradedAdmission' ./tests/unit` and
  `HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd`, then
  `HIPPO_BDD_ADAPTER=integration go test -count=1 ./tests/bdd`.
- _Reproduction_: steps 1–5 of the bug report with `./hippo` built from the branch, `HIPPO_CONFIG` unset, and
  `HIPPO_ROOT` set to an empty directory under `local-tmp/repro/`. Step 1's file comes from
  `git show 209718c:hippo.local.json.example`, never from the branch, whose example Phase 4 changes to `balanced`.

**Delivery units**, landed serially:

1. _Plan_ — this file alone, with the brief removed. Rollback: revert its merge.
2. _Fix_ — specification, tests, code, example, documentation, and `CHANGELOG.md`. Rollback: revert its merge; no
   release carries it until unit 3.
3. _Release_ — tag `v0.8.4` on unit 2's merge. A published tag is never replaced; a defect in it is fixed by `v0.8.5`.
4. _Record_ — this plan's results and its move to `plans/done/`. Rollback: revert its merge.

**Out of scope: repinning consumers.** Each consumer repins in its own repository through its own route, coordinated
outside this one ([plan lifecycle](../../../repo-governance/conventions/plan-lifecycle.md)). This plan records the
release's pin values that each repin uses.

**Pause safety.** Each item records its result when ticked. To resume, read the last ticked item, then
`git -C <worktree> log --oneline origin/main..HEAD` and `gh pr list --head <branch>`.

### Phase 1: Plan

- [x] `[AI]` Land this plan alone through a pull request, with the brief and its Q2 index line removed and the
      in-progress index updated; proof: the merge commit on `origin/main`. `[AC-07]` **Result:** #121 merged by rebase
      at `898c29d` after every check passed and a leak review posted `pass` for head `f85956e`.
- [x] `[AI]` Run the [plan quality gate](../../../repo-governance/workflows/quality/plan-quality-gate.md) on this folder
      in mode `normal`, at most three cycles per run, once on the landed plan and again after the D-06 amendment, a
      material change; proof: one terminal verdict line per run recorded here. `[AC-07]` **Result:** run 1, on
      `898c29d`: cycle 1 found one blocking HIGH row (the reproduction copied the example file this plan rewrites), two
      MEDIUM (a vacuous name-check grep, an unstated break test), and six LOW (an unstated lineage case, three line
      ranges, two overstated claims, `watch --json`, a missing preserve line); all were repaired, and cycle 2 found one
      new LOW (a line range), with no blocking row. Run 2, after D-06: cycle 1 found one blocking HIGH row (a sample
      source that repeated one timestamp could not keep a warning stable) and two LOW (the D-06 outcome missing from the
      public contract section, gate and amendment order); all were repaired, and cycle 2 found one new MEDIUM (the
      shedding rows' short child could finish before the shed on a loaded host). Both open rows were repaired in this
      amendment after their verdicts. Run 2 judged the uncommitted D-06 amendment draft, before its repairs were
      committed as `3f6949d`.
      `plan-quality-gate: PASS_WITH_FINDINGS (run 1: 2 cycles, 1 HIGH, 2 MEDIUM, 6 LOW resolved, 1 LOW open)`
      `plan-quality-gate: PASS_WITH_FINDINGS (run 2: 2 cycles, 1 HIGH, 2 LOW resolved, 1 MEDIUM open)`
- [x] `[AI]` Land the D-06 amendment, the gate's repairs, and the gate's verdict lines to this plan alone through a pull
      request, before any fix is committed; proof: the merge commit on `origin/main`. `[AC-07]` **Result:** #123 merged
      by rebase at `3a3afb0` after every check passed and a leak review posted `pass` for head `b70d02b`.

### Phase 2: Specification and Regression Tests

- [x] `[AI]` Add the two `admission.feature` scenarios and the `public-cli.feature` outline from
      [Specification Changes](#specification-changes), and the two exemptions to `tests/contract/contract.go`; proof:
      `HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd` reports the new steps undefined. `[AC-01]` `[AC-02]`
      `[AC-03]` **Result:** `HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd` reported the new steps undefined.
      **Deviation:** that command only checks the corpus against its bindings; the scenarios themselves run under
      `go test -count=1 -run TestUnitBehaviours ./tests/unit` and
      `go test -count=1 -run TestIntegrationBehaviours ./tests/integration`, which every later proof in this plan uses
      instead.
- [x] `[AI]` RED, behaviour: bind the new steps in `tests/support/steps.go` and `tests/support/driver.go`, decoding the
      status `profile` object as a map so a missing field fails by assertion; proof: the unit and integration adapters
      fail "A configured profile derived from balanced admits degraded work" because the guard deferred the work, and
      the outline because `profile.degradedAdmission` is absent, while "outside balanced's lineage" passes. `[AC-01]`
      `[AC-02]` `[AC-03]` **Result:** both adapters failed exactly the expected scenarios. "A configured profile derived
      from balanced admits degraded work" failed with
      `profile "local-balanced": exit=75 stderr="HIPPO deferred task: safe admission was not reached."`; both status
      rows failed with `status profile has no boolean degradedAdmission`; "outside balanced's lineage" passed on the
      integration adapter. The bindings live in `tests/support/degraded_lineage.go`. **Deviations:** the guard runs over
      an `advancingCollector` that stamps each repeated reading one step later, not a `sequenceCollector`, so the window
      stays stable through the admission wait; and the capacity-deferral Then holds the guard's internal deferral
      status, as the existing `requireDeferred` does, because the command line cannot shorten its admission window to
      reach the deferral within a test.
- [x] `[AI]` RED, unit: add `TestConfiguredProfilesInheritDegradedAdmission` to `tests/unit/adaptive_test.go`, loading a
      temporary schema-3 file with a `balanced`-derived profile, a profile derived from that one, a
      `constrained`-derived profile, and a configured `balanced` override, and resolving each, plus a `balanced` request
      that falls back on a small sample; proof: `go test -count=1 -run DegradedAdmission ./tests/unit` fails to build on
      the missing `Resolution.DegradedAdmission`. `[AC-01]` `[AC-02]` `[AC-03]` **Result:**
      `go test -count=1 -run DegradedAdmission ./tests/unit` failed to build:
      `resolution.DegradedAdmission undefined (type policy.Resolution has no field or method DegradedAdmission)`.
      **Deviation:** the temporary file is schema 2, not schema 3, because schema 3 refuses a file without adaptive
      coordination (`schema 3 requires adaptive coordination`) and the test concerns profiles only.
- [x] `[AI]` RED, shed: add the three `execution.feature` entries and their exemptions, and bind them; proof: the
      integration adapter fails "Stable warning spares a balanced ephemeral child admitted under normal pressure" with
      exit `74` (`guard.PressureShedExitCode`, which the command line reports as 124 naming `hippo.limit.pressure-shed`)
      and `HIPPO shedding ephemeral child after memory-warning.` on stderr, because `degraded` is false for a normally
      admitted child; every row of the two shedding outlines passes. With the advancing sample source the window is
      stable for the whole run, so no other cause can produce that shed. `[AC-08]` `[AC-09]` **Result:** the integration
      adapter failed "Stable warning spares a balanced ephemeral child admitted under normal pressure" with
      `exit=74 completed=false stderr="HIPPO shedding ephemeral child after memory-warning."`, and every row of both
      shedding outlines passed. Two unrelated scenarios failed in the same run only because they compile `./tests/unit`,
      which the unit RED above left unbuildable.

### Phase 3: Fix

- [x] `[AI]` GREEN: add the attribute to `Profile`, `BuiltinCatalog`, `Resolution`, and `Resolve` in
      `internal/policy/profiles.go`; gate `internal/guard/run.go` on it; update `assessAdmission` and
      `observeDegradedWarning` in `tests/support/driver.go`; proof: the _Focused tests_ pass, including every existing
      degraded-admission scenario. `[AC-01]` `[AC-02]` `[AC-03]` **Result:**
      `go test -count=1 -run DegradedAdmission ./tests/unit` passed, and
      `go test -count=1 -run TestUnitBehaviours ./tests/unit` and
      `go test -count=1 -run TestIntegrationBehaviours ./tests/integration` both exited `0`, including every existing
      degraded-admission scenario.
- [x] `[AI]` GREEN, shed: replace `degraded &&` in the supervision loop of `internal/guard/run.go` with the class and
      lineage conditions from [Second Fix](#second-fix-stable-warning-spares-every-eligible-child), and remove the
      `degraded` local; proof: the integration adapter passes every `execution.feature` scenario, including "Worsening
      warning sheds degraded work" and "Warning that outlasts the class grace sheds eligible work". `[AC-08]` `[AC-09]`
      **Result:** the same two adapter runs passed every `execution.feature` scenario, including "Worsening warning
      sheds degraded work" and "Warning that outlasts the class grace sheds eligible work"; the `degraded` local is
      gone.
- [x] `[AI]` REFACTOR: confirm the degraded gate no longer reads a profile name
      (`grep -n '"balanced"' internal/guard/run.go` prints nothing) and that the only other name-keyed paths are the two
      listed under the conditions above, and keep `internal/policy` and `internal/config` at the 99% coverage floor;
      proof: `npm run test:quick` exits `0`. `[AC-01]` **Result:** `grep -n '"balanced"' internal/guard/run.go` printed
      nothing; `npm run test:quick` exited `0` with `0 issues.` from the linter and
      `selected production line coverage: 99.30% (846/852 statements)`.
- [x] `[AI]` Run the
      [Gherkin implementation review](../../../repo-governance/workflows/quality/gherkin-implementation-review.md) on
      the new scenarios, the rebound "Stable macOS warning admits degraded work", and the rebound status step; proof:
      its status recorded here, with these break tests: reverting `run.go`'s gate to the name check fails "A configured
      profile derived from balanced admits degraded work"; removing the attribute from `BuiltinCatalog`'s `balanced`
      fails "Stable macOS warning admits degraded work"; dropping the `Resolution` field's JSON tag fails the status
      outline; and restoring `degraded &&` in the supervision loop fails "Stable warning spares a balanced ephemeral
      child admitted under normal pressure". `[AC-01]` `[AC-03]` `[AC-08]` **Result:** every new scenario and row, the
      rebound "Stable macOS warning admits degraded work", and the rebound status step are `implemented`; none is
      `untested`, `unimplemented`, or `drifted`. Each named break test failed its scenario and was reverted: the name
      gate failed "A configured profile derived from balanced admits degraded work" (`exit=75`); dropping the built-in
      attribute failed "Stable macOS warning admits degraded work" (`admitted=false`); dropping the JSON tag failed both
      status rows (`no boolean degradedAdmission`); and disabling the supervision exemption, as `degraded &&` does for a
      normally admitted child, failed "Stable warning spares a balanced ephemeral child admitted under normal pressure"
      (`exit=74`). Four more breaks hold the shedding rows: dropping the class condition failed the service row,
      dropping the lineage condition failed the `constrained` row, ignoring warning readiness failed the compressor,
      swap-out, and disk rows, and ignoring critical state failed the critical row.

### Phase 4: Example, Documentation, and Verification

- [x] `[AI]` Set `defaultProfile` to `balanced` in `hippo.local.json.example` and drop its `profiles` entry; proof:
      `./hippo status --config hippo.local.json.example` exits `0` naming `profile=balanced`. `[AC-04]` **Result:** a
      build of the branch printed `state=normal reason=normal profile=balanced concurrency=11 ...` for
      `status --config hippo.local.json.example` and exited `0`.
- [x] `[AI]` Run [docs propagation](../../../repo-governance/workflows/quality/docs-propagation.md): the configuration
      reference's `## profiles` example and lineage note, the resource policy's degraded-admission section and its "Host
      pressure remains authoritative" warning bullet (stable warning spares every eligible running ephemeral child, not
      only one admitted through the degraded path), the map-concurrency note, the status schema's
      `profile.degradedAdmission`, a `v0.8.4` `CHANGELOG.md` entry, and every document naming the current release;
      proof: `npm run format:check` exits `0` and `git grep -n -I 'local-constrained' -- ':!plans' ':!tests'` prints
      nothing. `[AC-04]` `[AC-06]` **Result:** updated the configuration reference (example now extends `balanced`, with
      the lineage note), the resource policy (both the warning bullet and the degraded-admission section), the
      map-concurrency note, the status schema (`profile.degradedAdmission`), `CHANGELOG.md` (`v0.8.4`), and the five
      pages naming the current release; `npm run format:check` exited `0` and the `git grep` printed nothing.
- [x] `[AI]` Run the _Reproduction_ with a build of the branch, once the host reads pressure level `2`; proof: the
      transcript recorded here, or a dated `Not observable` with the level read when the host never reached `2` during
      the unit. `[AC-05]` **Result:** `Not observable` on 2026-10-03: the host read pressure level `1` throughout the
      unit, so the live warning could not be reproduced. A build of the branch reported, at level `1`,
      `balanced degradedAdmission=true`, `local-balanced degradedAdmission=true`, and
      `local-constrained degradedAdmission=false` for the three reproduction configurations.
- [x] `[AI]` Run `npm test`; proof: the full gate exits `0` on the branch head. `[AC-01]` `[AC-02]` `[AC-03]` `[AC-08]`
      `[AC-09]` **Result:** `npm test` exited `0` on `59147f5`: the quick gate, the integration adapter, the end-to-end
      suite, the race-enabled run, and `No vulnerabilities found.` from `govulncheck`. The later commits change
      documentation only, and the rebase onto `091b4e4` brought only governance documents.

### Phase 5: Integrate and Release

- [x] `[AI]` Land the fix unit through a pull request, with the leak review posted for the exact head and every merge
      precondition holding; proof: the merge commit on `origin/main`. `[AC-01]` `[AC-02]` `[AC-03]` `[AC-04]` `[AC-08]`
      `[AC-09]` **Result:** #126 merged by rebase at `abde2ec` (fix commit `9a3c142`), after a rebase onto `091b4e4`,
      every check passing on head `7545895`, a leak review posted `pass` for that head, and merge state `CLEAN`. A
      Quality gate failure and a cancelled reproducibility job on that head belonged to run `37103066107`, superseded
      when the pull request was marked ready; run `37103077426` passed.
- [x] `[AI]` Run the [docs quality gate](../../../repo-governance/workflows/quality/docs-quality-gate.md) on subject
      `all` at that commit, as release cut requires; proof: one verdict line recorded here. `[AC-06]` **Result:** one
      cycle on the fix head `59147f5`; entry tooling clean; 44 documents and about 210 claims; no CRITICAL or HIGH row,
      so the run ended without a writer pass. **Deviation:** it ran on the pull request's head rather than the merge
      commit, so that rows within this plan's own documentation scope could land in the same unit; the commits on `main`
      since then change only governance, which is outside the gate's scope, but the repairs below changed six in-scope
      documents after the verdict and no cycle re-checked them before the tag; the execution check spot-checked them
      against `internal/guard/run.go` and found them consistent. This plan's docs propagation then repaired the rows it
      owns: the name-versus-lineage narrowing in the changelog (DQG-02), `extends` accepting a configured parent and the
      `balanced` recommendation (DQG-03, DQG-08), the stable-warning exception stated as a rule and in the shedding
      explanation (DQG-05, DQG-06), the `decision`-stays-`wait` note (DQG-08), and the exclusive-mode limit on forcing
      concurrency `1` (DQG-01). Left open: the first-command tutorial's claim that a warning does not affect it (DQG-04,
      MEDIUM, outside this fix), `profile.exitCode` carrying internal numbers (DQG-07, needs-decision), the
      degraded-admission message naming concurrency `1` under reservation coordination (DQG-01, needs-decision), and the
      dated idea brief's present tense (DQG-09), which the archival pull request repairs with a dated note that the fix
      shipped in v0.8.4. `docs-quality-gate: PASS_WITH_FINDINGS (1 cycle, 0 blocking, 4 MEDIUM and 5 LOW open)`
- [x] `[AI]` Cut `v0.8.4` on the merge commit through
      [release cut](../../../repo-governance/workflows/maintenance/release-cut.md), screening the generated notes and
      the tag name first; proof: the release's `checksums.txt` and the tag's peeled commit recorded here. `[AC-06]`
      **Result:** `scripts/test.sh` exited `0` on a clean detached `abde2ec`; the tag name and the notes
      `generate-notes` returned passed both outbound screens; `scripts/build-release.sh` and
      `tests/artifacts/release-assets.sh` passed; the annotated tag `v0.8.4` (tag object `4cd24db`) peels to
      `abde2ecbeff7513549cfd1c0805fd20b5d232c01`, and `release.yml` run `37105395978` published
      <https://github.com/wahidyankf/hippo/releases/tag/v0.8.4> with this `checksums.txt`: `darwin_amd64`
      `a9c976992dad8a7a8d116e85743dbc7cbc77b67eb139da9300a8c43e1fb8f6cc`, `darwin_arm64`
      `07b57189f6f4cc80d1913e55dcb386718bdc6aa3b0d34d8e0e8fedd089acc9f2`, `linux_amd64`
      `4247e0f91135a0becef83fdf45e10e66b440d0ae0b8fa30b66cf2b3d15031afc`, and `linux_arm64`
      `f1e6e02a2fa641c37555d10eae64450571ee70a9bd57268d9cfb914792bfc11b`, each for `hippo_v0.8.4_<os>_<arch>.tar.gz`.
      The build before tagging differed, because Go stamps the module version from the tags a checkout holds, as release
      cut says; rebuilt at the tag with the tag fetched, `scripts/build-release.sh` reproduced that `checksums.txt` byte
      for byte. No consumer was repinned.
- [x] `[AI]` Run the install commands in `docs/how-to/install-a-pinned-release.md` against the release; proof: the
      checksum line reads `OK` and `version --json` names `v0.8.4`. `[AC-06]` **Result:** in an empty directory the
      commands printed `hippo_v0.8.4_darwin_arm64.tar.gz: OK`, and `version --json` reported
      `{"schemaVersion":1,"version":"v0.8.4","commit":"abde2ecbeff7513549cfd1c0805fd20b5d232c01"}`; the `jq -e` check
      printed `true`.

### Phase 6: Close

- [x] `[AI]` Route each learning below to its durable owner, or discard it with a reason; proof: each entry names its
      owner or its reason. `[AC-07]` **Result:** two entries routed to idea briefs, three discarded with reasons.
- [x] `[AI]` Run the [execution check](../../../repo-governance/workflows/plan/plan-execution-check.md); proof: its
      verdict recorded here. `[AC-07]` **Result:** one run found scope, AC-01 to AC-06 and AC-08 to AC-09, the checklist
      evidence, the gates, and every learning sound, with seven LOW rows and none blocking. Four were repaired here: the
      docs gate deviation now says the repairs after its verdict were not re-gated, DQG-09's repair is recorded, the
      follow-up brief's status-schema claim is corrected, and plan gate run 2's revision is named. The other three close
      with this archival change: AC-07 and the clean-up it carries, the archived path the two briefs link to, and the
      routed briefs shipping in the same pull request.
      `plan-execution-check: PASS_WITH_FINDINGS (1 run, 7 LOW, 4 repaired, 3 closed by archival)`

### Archival

- [x] `[AI]` Move this folder to `plans/done/<completion date>__fix-configured-profiles-starve-under-macos-warning/`
      with both stage indexes updated, and land it through a pull request; proof: the merge commit on `origin/main`, and
      no copy left under `plans/in-progress/`. `[AC-07]` **Result:** moved with `git mv` to
      `plans/done/2026-10-03__fix-configured-profiles-starve-under-macos-warning/` on
      `worktree/fix-configured-profiles-starve-under-macos-warning-record`, with the in-progress and done indexes
      updated in the same change, and the strict-Go-linting brief's link repointed to the archived path. **Merge carried
      by the archival pull request (2026-10-03):** an archived file cannot record its own merge, so the merge commit is
      posted on that pull request.
- [ ] `[AI]` Run [dev artifact clean-up](../../../repo-governance/workflows/maintenance/dev-artifact-clean-up.md);
      proof: the worktree and every branch copy are gone and primary `main` equals `origin/main`. `[AC-07]` **Carried by
      the archival pull request (2026-10-03):** one worktree served every unit, so it goes once the archival unit lands.
      Classified before then:
  - _Remote branches_ `worktree/degraded-admission-profile-brief`, `...-plan`, and
    `worktree/fix-configured-profiles-starve-under-macos-warning`: scratch, already deleted on merge;
    `git ls-remote origin 'refs/heads/worktree/*'` printed nothing. `...-record` goes on its merge.
  - _Local branches_ `worktree/degraded-admission-profile-brief`, `...-plan`,
    `worktree/fix-configured-profiles-starve-under-macos-warning`, and `...-record`: scratch, their content landed
    through #121, #123, and #126; deleted once the archival merge lands.
  - _The worktree `worktrees/degraded-admission-profile-brief` and its ignored `local-tmp/`_ (the plan and docs gate
    ledgers, never committed by contract): scratch; removed with `git worktree remove` without `--force`.
  - _Session scratch outside the repository_ (reproduction configurations, release builds, test output): scratch,
    removed with the session.

  After the merge, the proof is posted on the archival pull request: `git worktree list` without this worktree, no local
  `worktree/*` branch of this plan, and `git rev-list --left-right --count main...origin/main` reading `0 0` after
  `git fetch --prune` and `git merge --ff-only origin/main`.

## Learnings

- **An outer workstation guard meets this repository's self-hosting rule at ad hoc commands.** The consumer workstation
  refuses bare `npm ci` and `npx` outside a HIPPO boundary, so this worktree's install and formatter ran under its own
  `./hippo` with the built-in `balanced` profile; with a `local-constrained` configuration they would have starved on
  this very defect. **Discarded:** the outer guard is the workstation's, not this repository's, and v0.8.4 removes the
  starvation itself.
- **The brief's first draft misread owner shares.** It claimed a configured profile's automatic owner shares ignore
  `extends`; `inheritShares` shows they follow it. Corrected here before landing. **Discarded:** a one-off reading
  error, corrected before the plan landed; no rule would have prevented it.
- **`Resolve`'s last-resort floor is keyed on the name `minimal`.** A configured profile derived from `minimal` that
  does not fit gets no last-resort floor (`internal/policy/profiles.go`, line 255), the same name-versus-lineage shape
  as this defect, found by the plan quality gate. **Routed** to the idea brief [Strict Go linting and domain
  modeling][linting-brief], whose profile-lineage item now names this path.
- **This plan's behaviour proofs named the binding-compliance command.**
  `HIPPO_BDD_ADAPTER=<adapter> go test ./tests/bdd` resolves bindings and runs no scenario; the executing adapters are
  `./tests/unit` and `./tests/integration`. Recorded as a deviation in Phase 2. **Discarded:** behaviour-driven
  development and software quality enforcement already say so; the plan, not the rule, was wrong.
- **The release-time docs gate found rows outside this fix.** The degraded-admission message under reservation
  coordination, `profile.exitCode` in status, and the first tutorial's claim about warning. **Routed** to the idea brief
  [Degraded-admission release audit follow-ups][followups-brief].

## Directory Map

This plan is one document, so this README has no siblings to map.

[followups-brief]: ../../ideas/q4-not-urgent-not-important/degraded-admission-release-audit-follow-ups.md
[linting-brief]: ../../ideas/q2-not-urgent-important/strict-go-linting-and-domain-modeling.md
[config-go]: https://github.com/wahidyankf/hippo/blob/209718c/internal/config/config.go
[driver-go]: https://github.com/wahidyankf/hippo/blob/209718c/tests/support/driver.go
[luchta-347]: https://github.com/dobesv/luchta/issues/347
[map-note]: ../../../docs/how-to/map-concurrency-into-your-build-tool.md#note-on-degraded-admission
[policy-go]: https://github.com/wahidyankf/hippo/blob/209718c/internal/policy/policy.go
[profiles-go]: https://github.com/wahidyankf/hippo/blob/209718c/internal/policy/profiles.go
[run-go]: https://github.com/wahidyankf/hippo/blob/209718c/internal/guard/run.go
[xnu-notify]: https://github.com/apple-oss-distributions/xnu/blob/main/doc/vm/memorystatus_notify.md
