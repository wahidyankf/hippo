# Workflow Standards

The contracts every quality gate and every gate's writer follow, adopted from the shared catalog at its paths, and the
adapter that maps them onto this repository.

## Directory Map

- [Quality gate adapter](quality-gate-adapter.md) — this repository's families, callers, tools, and deviations.
- [Quality gate contract](quality-gate-contract.md) — one bounded, advisory contract for every `<family>-quality-gate`.
- [Quality gate contract modules](quality-gate-contract/README.md) — inputs and scoring, sequence and termination,
  verdicts and ledger, in reading order.
- [Sole-writer propagation](sole-writer-propagation.md) — the one writer per family and the rules every writer shares.
