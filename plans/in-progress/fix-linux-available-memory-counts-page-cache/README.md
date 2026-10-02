# Fix: Linux Available Memory Counts Page Cache

Status: In progress (2026-10-02)

On Linux, HIPPO subtracts the job cgroup's whole `memory.current` from its limit. That figure includes clean,
reclaimable page cache, so a healthy host that has read a lot of files looks starved, and HIPPO defers or sheds work the
kernel could run without trouble.

This is a [bug-fix plan](../../../repo-governance/conventions/plans/010-bug-fix-plan.md). It replaces the idea brief
filed 2026-10-02 at `plans/ideas/q1-urgent-important/linux-available-memory-counts-page-cache.md` under the `ose-public`
[upstream tool defects][ose-utd] standard. That brief recorded a consumer workaround and deferred the fix; on 2026-10-02
the owner decided to fix it here and release it, which is the owner's request the bug-fix plan convention accepts in
place of a blocking defect. Under this repository's
[upstream tool defects](../../../repo-governance/development/upstream-tool-defects.md) standard, that request also
directs this plan's quality gate, its execution, and its release.

## Bug Report

**Description.** `collect` in `internal/host/collector_linux.go` sets
`available = min(MemAvailable, effective - memory.current)` whenever the process's cgroup has a `memory.current` file.
`memory.current` counts reclaimable page cache as used, so the reading falls toward zero while the kernel still reports
plenty of memory available, and HIPPO defers or sheds guarded work.

**Steps to reproduce** with Docker on Linux (Docker Desktop on arm64 here) and the released `hippo_v0.8.2_linux_arm64`
binary:

1. `docker run --rm --memory 6g --memory-swap 6g -v <dir-with-hippo>:/hippo:ro alpine:latest sh -c '...'`
2. In the container, write and read back a 7 GiB file (`dd if=/dev/zero of=/work/big bs=1M count=7168; sync;`
   `cat /work/big >/dev/null`), then run `hippo run --class ephemeral --disk-path /work -- sleep 40`.
3. Alternatively, run the same file write and read as the payload of `hippo run --class ephemeral`.

**Expected behaviour.** The [cgroup v2 documentation][cgv2] describes `inactive_file` as file data "on the internal
memory management lists used by the page reclaim algorithm", and `MemAvailable` in the [procfs documentation][procfs]
already counts "the size of the file LRU lists" as available. File data that can be reclaimed at once therefore does not
count against available memory, and admission and shedding follow the memory the payload could still use.

**Actual behaviour.** After step 2, `memory.stat` showed `anon 122880` and `inactive_file 6438256640`, and
`MemAvailable` was about 22.7 GiB. HIPPO's own sample reported `availableMemoryBytes: 1310720` with reason
`memory-critical`, and it deferred admission with `hippo.limit.capacity-deferred`. In step 3, HIPPO admitted the payload
and then shed it.

**Error output** from step 3, the same message the consumer saw:

```text
HIPPO shedding ephemeral child after memory-critical.
hippo: [hippo.limit.pressure-shed] host pressure shed this work after the payload ran; recover it before \
repeating anything
```

**Environment.** HIPPO v0.8.2 (`5f21ca2`) on Linux arm64 under Docker Desktop, and on GitHub-hosted `ubuntu-latest`
runners with 16 GiB of memory. `internal/host` and `internal/policy` are unchanged on `main` at `d07782f`, so trunk
behaves the same.

**Observed** 2026-10-01 to 2026-10-02 in `ose-public`'s `pr-quality-gate.yml`, `Repository policy` job. Before the
guarded RHINO surface, that job installs Node, .NET, Rust, Flutter, Java, Go, and Python. About one minute into the
step, HIPPO printed `HIPPO shedding ephemeral child after memory-warning.` and exited 124 before RHINO reported
anything. This happened in pull request run 36982227277 on both attempts and in `main` runs 36865789763, 36866720303,
36871882821, 36940844040, and 36977510904. Other runs of the same job passed. With no configuration file, HIPPO used the
compiled `balanced` profile, whose reserve is 15% of memory, about 2.4 GiB.

Run 36986287304 of the same job (`ose-public` pull request #629) recorded this just before the guarded step:

```text
before: MemAvailable:   14810480 kB, cgroup memory.current 13352443904, inactive_file 11884974080
after: MemAvailable:   15182540 kB, cgroup memory.current 377806848, inactive_file 32817152
```

Before the drop, the kernel reported 14.1 GiB available, but `memory.current` was 12.4 GiB, of which 11.1 GiB was
`inactive_file`. HIPPO therefore saw roughly 3 GiB on a 16 GiB runner, one gate's worth of file reads above the reserve.
After `drop_caches`, `memory.current` fell to 360 MiB, and the guarded surface passed.

**Workaround.** Before the guarded step, `sync; echo 3 | sudo tee /proc/sys/vm/drop_caches`. It needs root and does
nothing for cache the payload itself creates, which is why the owner chose the fix over the workaround.

## Duplicate Check

Run 2026-10-02 against `main` at `d07782f`:

- `gh issue list --state all` returned no issues.
- `gh pr list --state open` returned no pull requests.
- `grep -rli` over `plans/` outside `done/` for `cgroup`, `page cache`, `memory.current`, `inactive_file`,
  `pressure-shed`, and `memory-warning` found only this defect's own idea brief and its quadrant index.
- `git log -i --grep` for `inactive_file`, `page cache`, `memory.current`, and `memory.stat` found nothing. The brief's
  earlier search of the same terms found only `68cae05`, which does not touch this reading.

The only match is the brief this plan replaces. No duplicate exists.

## Root Cause

`internal/host/collector_linux.go`, in `collect`, with its `if` line wrapped to fit here:

```go
available := memory.Available
if current, finite := ParseCgroupLimit(readOptional(read, filepath.Join(cgroup, "memory.current")));
	finite && effective > 0 {
	available = min(available, max(int64(0), effective-current))
}
```

The [cgroup v2 documentation][cgv2] defines `memory.current` as "the total amount of memory currently being used by the
cgroup and its descendants". Under its memory ownership rules, "a memory area is charged to the cgroup which
instantiated it and stays charged to the cgroup until the area is released", and that includes page cache: `memory.stat`
reports it as `file`, "memory used to cache filesystem data", split across the `active_file` and `inactive_file` lists
"used by the page reclaim algorithm". So `effective - memory.current` treats every cached file page as spoken for.

Two shapes follow, both checkable in the evidence above:

- **No `memory.max`**, as with a hosted runner's service cgroup: `effective` is `MemTotal`. The kernel reclaims cache
  only under global pressure, so `memory.current` climbs toward physical memory while `MemAvailable` stays high. In run
  36986287304, `effective - memory.current` was about 3 GiB while `MemAvailable` was 14.1 GiB; the `min` picked the
  cgroup term.
- **A finite `memory.max`**, as in the container reproduction: cache fills the cgroup to its limit, and the reading
  approaches zero. There, `6442450944 - memory.current` gave the reported `1310720` while 6438256640 bytes were
  `inactive_file`.

`MemAvailable` alone is right on the first shape and blind to the second, which is why the cgroup term exists. The
defect is only that the cgroup term counts reclaimable cache as usage.

## Solution

Measure the cgroup's usage as its working set: `memory.current` less the `inactive_file` that `memory.stat` reports, and
never below zero. The Linux reading becomes:

```text
working set = max(0, memory.current - inactive_file)
available   = min(MemAvailable, max(0, effective - working set))
```

**Why it removes the cause.** `inactive_file` is exactly the cache the kernel reclaims first, so it no longer reads as
usage, while anonymous memory, `active_file`, kernel memory, and everything else in `memory.current` still does. The
`min` with `MemAvailable` stays, so host-wide pressure still governs a cgroup with no limit of its own.

**Conditions beyond the reported one.**

- `inactive_file` larger than `memory.current`, which per-CPU counter batching can produce briefly: the working set
  floors at zero, so the reading never exceeds `effective`.
- `memory.current` over `effective`: the reading floors at zero, as today.
- `memory.stat` unreadable, or without a well-formed `inactive_file` line: no cache is subtracted, which is today's
  reading. The fallback is the conservative one; it never reports more memory than v0.8.2 did.
- No `memory.current`: the root cgroup has none, since the [cgroup v2 documentation][cgv2] says both `memory.current`
  and `memory.stat` exist "on non-root cgroups". cgroup v1 and hybrid hosts expose no `memory.current` beneath the path
  HIPPO reads. All three keep `MemAvailable` alone, unchanged. cgroup v1's `total_inactive_file` is out of scope because
  HIPPO reads no v1 file today.
- `memory.stat` is parsed by key, never by position, because the documentation warns that "new entries can show up in
  the middle".

**Alternatives rejected.**

- _Subtract `active_file` too._ It would match `MemAvailable` more closely, but active pages are reclaimed only after
  being demoted, so counting them available risks admitting work the cgroup cannot hold. Excluding them is the choice
  the kubelet makes, and stays conservative.
- _Drop the cgroup term and trust `MemAvailable`._ It is host-wide, so a container with a 6 GiB `memory.max` on a 24 GiB
  host would report 22.7 GiB.
- _Drop caches from HIPPO._ It needs root, discards other work's cache, and is the consumer workaround this replaces.

**Public contract.** `availableMemoryBytes` keeps its meaning, the memory available to new work; only its accuracy on
Linux changes. No exit status, reason code, schema, or configuration moves, so the
[public contract](../../../repo-governance/development/public-contract.md) is unchanged and the release is a patch,
`v0.8.3`.

**References**, read 2026-10-02:

- [Linux kernel: Control Group v2][cgv2], the `memory.current`, `memory.stat`, and Memory Ownership sections. Primary
  source for every kernel term above.
- [Linux kernel: The /proc Filesystem][procfs], the `MemAvailable` entry: "Calculated from MemFree, SReclaimable, the
  size of the file LRU lists, and the low watermarks in each zone".
- [Kubernetes: Node-pressure Eviction][k8s-eviction]: "The kubelet excludes inactive_file (the number of bytes of
  file-backed memory on the inactive LRU list) from its calculation, as it assumes that memory is reclaimable under
  pressure." This plan's working-set formula is copied from it.
- [Kubernetes: `memory-available-cgroupv2.sh`][k8s-script], the kubelet's calculation as a script: `memory.current` less
  `inactive_file` from `memory.stat`, floored at zero when the cache exceeds usage.

### Acceptance Criteria

- **AC-01** Given a Linux cgroup whose `memory.current` is mostly `inactive_file`, when HIPPO samples the host, then
  `availableMemoryBytes` is `effective - (memory.current - inactive_file)`, capped by `MemAvailable`.
- **AC-02** Given `memory.stat` is unreadable or has no well-formed `inactive_file`, when HIPPO samples the host, then
  `availableMemoryBytes` is `effective - memory.current`, capped by `MemAvailable`, as in v0.8.2.
- **AC-03** Given `inactive_file` exceeds `memory.current`, or `memory.current` exceeds `effective`, then
  `availableMemoryBytes` lies between zero and `effective`.
- **AC-04** Given no `memory.current`, then `availableMemoryBytes` is `MemAvailable`, as in v0.8.2.
- **AC-05** Given the container reproduction above, when a build of the fix runs it, then HIPPO admits the payload and
  it finishes with its own exit status.
- **AC-06** Given the fix is merged, then a `v0.8.3` release publishes it with CI-built assets and `checksums.txt`.

## Delivery

**Execution checkout.** `~/ose-projects/hippo/worktrees/fix-linux-available-memory-counts-page-cache`, branch
`worktree/fix-linux-available-memory-counts-page-cache`, created from `origin/main` and reused for every unit below, per
[worktree to pull request](../../../repo-governance/workflows/maintenance/worktree-to-pull-request.md). HIPPO cannot
guard its own gates, so `npm run test:quick` and `npm test` run directly, per
[resource-aware development](../../../repo-governance/development/resource-aware-development.md).

**Delivery units**, landed serially:

1. _Plan_ — this file alone. Rollback: revert its merge.
2. _Fix_ — specification, tests, collector change, `CHANGELOG.md`, and documentation. Rollback: revert its merge; no
   release carries it until unit 3.
3. _Release_ — tag `v0.8.3` on unit 2's merge. A published tag is never replaced; a defect in it is fixed by `v0.8.4`.
4. _Record_ — this plan's results and its move to `plans/done/`. Rollback: revert its merge.

**Pause safety.** Each item records its result when ticked. To resume, read the last ticked item, then
`git -C <worktree> log --oneline origin/main..HEAD` and `gh pr list --head <branch>`.

### Phase 1: Plan

- [ ] `[AI]` Land this plan alone through a pull request, with the brief removed from `plans/ideas/` and both stage
      indexes updated; proof: the merge commit on `origin/main`. `[AC-06]`
- [ ] `[AI]` Run the [plan quality gate](../../../repo-governance/workflows/quality/plan-quality-gate.md) on this folder
      in mode `normal`; proof: one terminal verdict line recorded here. `[AC-01]`

### Phase 2: Specification and Regression Tests

- [ ] `[AI]` Assess `specs/behaviours/` and `specs/architecture.md`; add the scenario
      `Linux reclaimable file cache stays available` to `specs/behaviours/portability.feature`, `@e2e-exempt` with an
      exact `tests/contract/contract.go` entry; proof: `HIPPO_BDD_ADAPTER=unit go test -count=1 ./tests/bdd` reports it
      undefined. `[AC-01]`
- [ ] `[AI]` RED: bind its steps in `tests/support/steps.go` and `tests/support/driver.go`, and add
      `TestCgroupAvailableMemoryExcludesInactiveFileCache` to `tests/unit/host_test.go` and a `memory.stat` fixture case
      to `tests/unit/linux_collector_test.go`; proof: `go test -count=1 ./tests/unit ./tests/bdd` fails to compile on
      the missing helper, and the Linux collector case run in a `golang` container reports the v0.8.2 reading where it
      expects the working-set one. `[AC-01]` `[AC-02]` `[AC-03]` `[AC-04]`

### Phase 3: Fix

- [ ] `[AI]` GREEN: add the working-set helper to `internal/host/linux_parsers.go` and read `memory.stat` in
      `internal/host/collector_linux.go`; proof: the Phase 2 tests pass on macOS and in the Linux container. `[AC-01]`
      `[AC-02]` `[AC-03]` `[AC-04]`
- [ ] `[AI]` REFACTOR: reuse the existing flat-keyed parser rather than a second one, and keep the helper at the 99%
      coverage floor; proof: `npm run test:quick` exits `0`. `[AC-01]`

### Phase 4: Verify and Document

- [ ] `[AI]` Re-run the container reproduction with a Linux build of the branch and of `origin/main`; proof: the branch
      admits and finishes the payload, and `origin/main` defers or sheds it. `[AC-05]`
- [ ] `[AI]` Run the
      [Gherkin implementation review](../../../repo-governance/workflows/quality/gherkin-implementation-review.md) on
      the new scenario; proof: its status recorded here. `[AC-01]`
- [ ] `[AI]` Run [docs propagation](../../../repo-governance/workflows/quality/docs-propagation.md): a `v0.8.3`
      `CHANGELOG.md` entry, and every document naming the current release; proof: `npm run format:check` exits `0`.
      `[AC-06]`
- [ ] `[AI]` Run `npm test`; proof: the full gate exits `0` on the branch head. `[AC-01]` `[AC-05]`

### Phase 5: Integrate and Release

- [ ] `[AI]` Land the fix unit through a pull request, with the leak review posted for the exact head and every merge
      precondition holding; proof: the merge commit on `origin/main`. `[AC-01]`
- [ ] `[AI]` Run the [docs quality gate](../../../repo-governance/workflows/quality/docs-quality-gate.md) on subject
      `all` at that commit, as [release cut](../../../repo-governance/workflows/maintenance/release-cut.md) requires;
      proof: one verdict line recorded here. `[AC-06]`
- [ ] `[AI]` Cut `v0.8.3` on the merge commit through
      [release cut](../../../repo-governance/workflows/maintenance/release-cut.md), screening the generated notes first;
      proof: the release's `checksums.txt` and the tag's peeled commit recorded here. `[AC-06]`

### Phase 6: Close

- [ ] `[AI]` Run the [execution check](../../../repo-governance/workflows/plan/plan-execution-check.md); proof: its
      verdict recorded here. `[AC-06]`
- [ ] `[AI]` Route each learning below to its durable owner, or discard it with a reason. `[AC-06]`

### Archival

- [ ] `[AI]` Move this folder to `plans/done/2026-10-02__fix-linux-available-memory-counts-page-cache/` with both stage
      indexes updated, and land it through a pull request. `[AC-06]`
- [ ] `[AI]` Run [dev artifact clean-up](../../../repo-governance/workflows/maintenance/dev-artifact-clean-up.md);
      proof: the worktree and both branch copies are gone and primary `main` equals `origin/main`. `[AC-06]`

## Learnings

None yet.

## Directory Map

This plan is one document, so this README has no siblings to map.

[cgv2]: https://docs.kernel.org/admin-guide/cgroup-v2.html
[procfs]: https://docs.kernel.org/filesystems/proc.html
[k8s-eviction]: https://kubernetes.io/docs/concepts/scheduling-eviction/node-pressure-eviction/
[k8s-script]:
  https://github.com/kubernetes/website/blob/main/content/en/examples/admin/resource/memory-available-cgroupv2.sh
[ose-utd]:
  https://github.com/wahidyankf/ose-public/blob/main/repo-governance/development/workflow/upstream-tool-defects.md
