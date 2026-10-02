# Linux Available Memory Counts Page Cache

On Linux, HIPPO subtracts the job cgroup's whole `memory.current` from its limit. That figure includes clean,
reclaimable page cache, so a healthy host that has read a lot of files looks starved. HIPPO then defers or sheds work
the kernel could run without trouble.

Filed 2026-10-02 under the `ose-public` [upstream tool defects][ose-utd] standard. A consumer CI workaround exists, so
this is filed here and not fixed yet.

## Problem and Evidence

**Description.** `collect` in `internal/host/collector_linux.go` sets
`available = min(MemAvailable, effective - memory.current)` whenever the process's cgroup has a `memory.current` file.
The kernel's [cgroup v2 documentation](https://docs.kernel.org/admin-guide/cgroup-v2.html) defines `memory.current` as
"the total amount of memory currently being used by the cgroup and its descendants". Under memory ownership, a memory
area "is charged to the cgroup which instantiated it", which includes page cache. `memory.stat` reports that cache as
`inactive_file` ("cached filesystem data on the internal memory management lists used by the page reclaim algorithm")
and `active_file`. HIPPO counts all of it as used. `MemAvailable` already treats reclaimable cache as available, but the
`min` lets the cgroup reading override it.

When the cgroup has no `memory.max`, as with a GitHub-hosted runner's service cgroup, the kernel reclaims cache only
under global pressure. `memory.current` can therefore climb toward physical memory while `MemAvailable` stays high. When
the cgroup does set `memory.max`, the cache fills to the limit and the reading approaches zero.

**Steps to reproduce** with Docker on Linux (here Docker Desktop on arm64) and the released `hippo_v0.8.2_linux_arm64`
binary:

1. `docker run --rm --memory 6g --memory-swap 6g -v <dir-with-hippo>:/hippo:ro alpine:latest sh -c '...'`
2. In the container, write and read back a 7 GiB file (`dd if=/dev/zero of=/work/big bs=1M count=7168; sync;`
   `cat /work/big >/dev/null`), then run `hippo run --class ephemeral --disk-path /work -- sleep 40`.
3. Alternatively, run the same file write and read as the payload of `hippo run --class ephemeral`.

**Expected.** File data that can be reclaimed at once does not count against available memory. Admission and shedding
follow the memory the payload could still use.

**Actual.** After step 2, `memory.stat` showed `anon 122880` and `inactive_file 6438256640`, and `MemAvailable` was
about 22.7 GiB. HIPPO's own sample reported `availableMemoryBytes: 1310720` with reason `memory-critical`, and it
deferred admission with `hippo.limit.capacity-deferred`. In step 3, HIPPO admitted the payload and then shed it with the
same message the consumer saw:

```text
HIPPO shedding ephemeral child after memory-critical.
hippo: [hippo.limit.pressure-shed] host pressure shed this work after the payload ran; recover it before \
repeating anything
```

**Environment.** HIPPO v0.8.2 (`5f21ca2`). `internal/host` and `internal/policy` are unchanged on `main` at `a67904a`,
so trunk behaves the same.

**Observed** 2026-10-01 to 2026-10-02 in `ose-public`'s `pr-quality-gate.yml`, `Repository policy` job. Before the
guarded RHINO surface, that job installs Node, .NET, Rust, Flutter, Java, Go, and Python on a 16 GiB `ubuntu-latest`
runner. About one minute into the step, HIPPO printed `HIPPO shedding ephemeral child after memory-warning.` and exited
124 before RHINO reported anything. This happened in pull request run 36982227277 on both attempts and in `main` runs
36865789763, 36866720303, 36871882821, 36940844040, and 36977510904. Other runs of the same job passed. With no
configuration file, HIPPO used the compiled `balanced` profile, whose reserve is 15% of memory, about 2.4 GiB. The
miscounted reading crossed that reserve and stayed under it past the 10-second ephemeral grace.

The job's own reading confirms the cause. Run 36986287304 of the same job (`ose-public` pull request #629) recorded this
just before the guarded step:

```text
before: MemAvailable:   14810480 kB, cgroup memory.current 13352443904, inactive_file 11884974080
after: MemAvailable:   15182540 kB, cgroup memory.current 377806848, inactive_file 32817152
```

Before the drop, the kernel reported 14.1 GiB available, but `memory.current` was 12.4 GiB, of which 11.1 GiB was
`inactive_file`. HIPPO therefore saw roughly 3 GiB on a runner with 16 GiB of memory, one gate's worth of file reads
above the reserve. After `drop_caches`, `memory.current` fell to 360 MiB, and the guarded surface passed.

## Why Now

Every consumer that runs HIPPO in hosted CI after a toolchain install is exposed. Whether a run passes depends on how
much cache earlier steps left behind, so the failure looks like flakiness. The shed message tells the reader to recover
the work before repeating it, which sends them toward their own payload, not toward HIPPO's reading.

## Prior Art

Read 2026-10-02:

- **Duplicate search.** `gh issue list --state all` returned no issues. The open pull request list was empty. A search
  of `plans/` for `cgroup`, `page cache`, `memory.current`, `inactive_file`, `pressure-shed`, and `memory-warning` found
  nothing in ideas, backlog, in-progress, or done. `git log --grep` for the same terms found only `68cae05` ("test(e2e):
  fix the host evidence a compiled run samples on Linux"), which does not touch this reading. No duplicate exists.
- **Kernel definitions.** [cgroup v2](https://docs.kernel.org/admin-guide/cgroup-v2.html) covers `memory.current`,
  `memory.stat` (`file`, `active_file`, `inactive_file`), and memory ownership.
- **Kubernetes.** For
  [node-pressure eviction](https://kubernetes.io/docs/concepts/scheduling-eviction/node-pressure-eviction/), the kubelet
  "excludes inactive_file (the number of bytes of file-backed memory on the inactive LRU list) from its calculation, as
  it assumes that memory is reclaimable under pressure".

## Workaround

Before the guarded step, drop clean page cache: `sync; echo 3 | sudo tee /proc/sys/vm/drop_caches`. This releases the
cache charged to the job cgroup. `ose-public` does this in `pr-quality-gate.yml` and prints `MemAvailable`,
`memory.current`, and `inactive_file` before and after the drop. The step needs root and does nothing for cache the
payload itself creates.

## Proposed Direction

Subtract the cgroup's reclaimable file cache, at least `inactive_file`, from `memory.current` before taking the limit
difference. Keep `min` with `MemAvailable`. Cover it with a regression case that feeds the collector a `memory.stat`
holding a large `inactive_file` and requires the sample to report near-full availability.

## Scope and Non-Goals

In scope: the Linux available-memory reading and its regression test. Not in scope: thresholds, profiles, grace periods,
the macOS collector, or shed selection.

## Risks and Open Questions

- Should `active_file` count as reclaimable too? Excluding it is conservative. Including it matches `MemAvailable` more
  closely.
- `memory.stat` is missing on the root cgroup. The reading should fall back to `MemAvailable` alone there, as it does
  today when `memory.current` is absent.
- Does a changed sample value move the [public contract](../../../repo-governance/development/public-contract.md)? The
  field's meaning stays "available memory"; only its accuracy changes.

## Success

The container reproduction admits and finishes its payload, and the regression case fails without the fix. Consumers can
drop the cache-dropping step on their next repin. Promote this brief to a bug-fix plan if the workaround stops holding
or a consumer cannot run it, for example without root.

[ose-utd]:
  https://github.com/wahidyankf/ose-public/blob/main/repo-governance/development/workflow/upstream-tool-defects.md
