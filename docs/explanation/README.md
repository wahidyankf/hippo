# Explanation

Why HIPPO works the way it does. These pages are for understanding, not for following along — read
them when a design decision seems surprising and you want the reasoning behind it.

## Pages

- [Why HIPPO exists](./why-hippo-exists.md) — the contention problem, why the obvious fixes fail, and
  what HIPPO is deliberately not.
- [The reservation model](./reservation-model.md) — why admission is a two-dimensional vector, why
  capacity is smaller than the machine, and why FIFO ordering is strict.
- [Process ownership and shedding](./process-ownership-and-shedding.md) — why a guard signals only
  its own child, how mark-and-observe replaces cross-signalling, and what happens to an abandoned
  group.
- [Why HIPPO fails closed](./failing-closed.md) — the asymmetry between deferring too much and
  admitting too much, and how it shapes every ambiguous case.

## Next steps

- [Reference](../reference/README.md) for the exact values behind these ideas.
- [How-to guides](../how-to/README.md) to act on them.
