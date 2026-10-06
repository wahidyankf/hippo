# Done

This stage preserves completed plans as delivery records. A finished folder is named `YYYY-MM-DD__<slug>`, the date
being when the work completed, so the directory sorts by delivery rather than by intent.

What is here is history. It describes the repository as it was on purpose, and it may name paths that have since moved.
That is why [`repo-config.yml`](../../repo-config.yml) excludes `plans/done/**` as a link _source_ while leaving it a
valid target: an archived plan's outward links are allowed to have aged, and reporting them would make the act of
finishing a plan generate findings. For what the tool does today, read [`specs/`](../../specs/README.md).

Reconcile required and conditional delivery, acceptance, verification, and learnings before archiving.
[Plan execution](../../repo-governance/workflows/plan/plan-execution.md) owns the move itself: it refuses an existing
destination rather than merging into it, overwriting it, or adding a suffix; records completion metadata and outcomes;
moves the folder out of [`../in-progress/`](../in-progress/README.md); updates both stage indexes in one change; and
then verifies the archive rather than assuming it.

## Completed Plans

- 2026-10-07 —
  [Fix: Corrupt-Waiter Identity Test Flakes Under Load](2026-10-07__fix-corrupt-waiter-identity-test-flake/README.md):
  the corrupt-identity test parks its waiter outside the coordination lock, so its competing admission always reaches
  the ledger; test-only, carried by v0.8.5.
- 2026-10-07 —
  [Fix: Degraded-Lineage Admission Scenario Flakes Under Load](2026-10-07__fix-degraded-lineage-scenario-flake/README.md):
  the degraded-lineage scenarios time their admission window on a logical clock, so a loaded runner no longer defers
  them; test-only, carried by v0.8.5.
- 2026-10-03 —
  [Fix: Configured profiles starve under macOS warning](2026-10-03__fix-configured-profiles-starve-under-macos-warning/README.md):
  degraded admission and its stable-warning shed exemption follow the `balanced` lineage, not the name; released as
  v0.8.4.
- 2026-10-02 —
  [Fix: Linux available memory counts page cache](2026-10-02__fix-linux-available-memory-counts-page-cache/README.md):
  the Linux reading subtracts the cgroup's inactive file cache from its usage; released as v0.8.3.
- 2026-09-26 — [Repair supervision readiness race](2026-09-26__repair-supervision-readiness-race/README.md): the
  collector-failure fixture waits for the child-published PID before it injects the failure.
- 2026-09-26 — [Gate shell static analysis](2026-09-26__gate-shell-static-analysis/README.md): shell scripts are gated
  on a checksum-pinned ShellCheck at warning severity over one list the formatter shares.
- 2026-09-26 — [Neutralize test fixture identifiers](2026-09-26__neutralize-test-fixture-identifiers/README.md): the
  source-override fixture uses a synthetic value instead of a string that named a private repository.
- 2026-09-26 — [Isolate test coordination state](2026-09-26__isolate-test-coordination-state/README.md): each test
  package that starts the product removes inherited `HIPPO_*` variables and owns a run-scoped evidence root.

## Directory Map

- [Fix: Configured profiles starve under macOS warning](2026-10-03__fix-configured-profiles-starve-under-macos-warning/README.md)
  is the delivery record for the v0.8.4 degraded-admission lineage fix.
- [Fix: Corrupt-Waiter Identity Test Flakes Under Load](2026-10-07__fix-corrupt-waiter-identity-test-flake/README.md) is
  the delivery record for the corrupt-waiter identity test flake fix.
- [Fix: Degraded-Lineage Admission Scenario Flakes Under Load](2026-10-07__fix-degraded-lineage-scenario-flake/README.md)
  is the delivery record for the degraded-lineage scenario flake fix.
- [Fix: Linux available memory counts page cache](2026-10-02__fix-linux-available-memory-counts-page-cache/README.md) is
  the delivery record for the v0.8.3 page-cache fix.
- [Gate shell static analysis](2026-09-26__gate-shell-static-analysis/README.md) is the delivery record for the
  shell-lint gate.
- [Isolate test coordination state](2026-09-26__isolate-test-coordination-state/README.md) is the delivery record for
  the test-isolation helper.
- [Neutralize test fixture identifiers](2026-09-26__neutralize-test-fixture-identifiers/README.md) is the delivery
  record for the synthetic identity fixture.
- [Repair supervision readiness race](2026-09-26__repair-supervision-readiness-race/README.md) is the delivery record
  for the PID readiness barrier.
