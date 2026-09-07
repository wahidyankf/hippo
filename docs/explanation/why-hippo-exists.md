# Why HIPPO exists

## The problem

A developer working across several repositories ends up running several heavy things at once. A
build in one checkout, a test suite in another, a dev server in a third, an agent-driven task in a
fourth. Each of them is individually reasonable. Each of them, on its own, sizes itself to the
machine: `make -j$(nproc)`, a test runner that defaults to one worker per core, a bundler that
assumes it owns the box.

Collectively they are not reasonable at all. Every one of them sees twelve cores and takes twelve
cores. The machine starts swapping, the compressor fills, and everything slows down together —
including the interactive work the developer is actually watching. On macOS the usual outcome is a
long, silent stall; on Linux it is more often the OOM killer picking a victim that had nothing to do
with the overload.

The failure mode is not that any single task is too big. It is that **nothing is arbitrating between
them**. Each process makes a locally correct decision using a global resource it does not own.

## Why the obvious fixes do not work

**Turn the parallelism down everywhere.** This is the common answer, and it is wrong in both
directions. Pinned low, a single build on an idle machine now takes three times longer for no reason.
Pinned high, the contention returns the moment a second task starts. A fixed constant cannot be right
for a variable number of concurrent tasks.

**Use the operating system's tools.** `nice`, `cpulimit`, and cgroups shape how work is scheduled
once it is running. They do not decide whether it should start. A machine that is already out of
memory does not need its tenth build de-prioritized; it needs that build to wait. And none of these
tools knows that the ten processes belong to four different repositories that should each get a
share.

**Let the build tools coordinate.** They cannot. Cargo does not know about your Gradle daemon. Jest
does not know about your Next.js dev server. There is no shared vocabulary between ecosystems, and
adding one to each of them is not a maintainable path.

## What HIPPO does instead

HIPPO puts a small, generic arbiter between the developer's intent and the work starting. Before a
compute-bearing command runs, it:

1. Reads normalized host evidence — memory, disk, CPU, swap, compressor, and Linux pressure.
2. Resolves a safe profile from that evidence.
3. Atomically claims a fixed CPU-and-memory reservation from a ledger shared by every repository on
   the host.
4. Starts the command in its own process group with the allocated concurrency in its environment.
5. Keeps sampling, and sheds the work if the host crosses into critical pressure.

The command itself does not change. It reads a number out of an environment variable it already
understands, and that number now reflects what the machine can actually spare, given everyone else.

## The design commitments that follow

Three commitments fall out of that job, and most of HIPPO's apparent complexity is downstream of
them.

**It has to be generic.** The moment HIPPO knows about `make` or `npm` or `cargo`, it becomes a tool
for those ecosystems and useless for the next one. So consumers supply commands, paths, ports, and
health endpoints, and select their own environment-variable names with `--concurrency-env`. HIPPO
compiles in no product defaults.

**It has to be safe against itself.** A resource guard that kills the wrong process is worse than no
guard. HIPPO therefore signals only process groups it started and owns; a guard under pressure
elsewhere on the host marks a victim and waits for that victim's own guard to act, rather than
reaching across and sending a signal itself. See
[Process ownership and shedding](./process-ownership-and-shedding.md).

**It has to fail closed.** When HIPPO cannot prove that shared state is consistent, the safe answer
is to defer, not to guess and rewrite. A wrongly-cleared ledger lets everyone in at once — precisely
the failure HIPPO exists to prevent. See [Why HIPPO fails closed](./failing-closed.md).

## What HIPPO is not

- It is not a scheduler. It admits or defers; it does not queue your work for later or reorder it
  beyond strict FIFO among waiters.
- It is not a sandbox. It does not restrict what a guarded command can do, only how much of the host
  it is told to assume.
- It is not a production supervisor. The evidence budget, retention windows, and shedding policy are
  sized for a developer machine.
- It is not a monitoring product. `monitor` and the evidence files exist to explain HIPPO's own
  decisions, not to be a metrics pipeline.

## Related

- [The reservation model](./reservation-model.md)
- [Process ownership and shedding](./process-ownership-and-shedding.md)
- [Guard your first command](../tutorials/guard-your-first-command.md)
