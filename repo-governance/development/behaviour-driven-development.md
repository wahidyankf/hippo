# Behaviour-Driven Development

One corpus, three executing boundaries, and a static check over the mapping between them.

## The Boundaries

- **Unit** runs the corpus in process, against the packages directly. Every scenario runs here.
- **Integration** runs it against real directories, real files, and real child processes.
- **E2E** runs it against the spawned executable, which is the only boundary that sees what a caller sees.

A scenario is classified by the strongest real boundary its setup, subject, or assertions touch — not by the resources it is permitted to use.

## Strictness

Every adapter uses strict step resolution. An undefined step fails; an ambiguous pattern fails; a registered pattern with no handler fails; a handler no scenario reaches fails. All four are the same defect wearing different clothes: a corpus that does not correspond to the bindings that claim to run it.

## What the Compliance Check Does Not Do

`tests/bdd` resolves bindings. It does not execute scenarios. A change that satisfies it has proved that every step has a handler and nothing more — the executing adapters are where a scenario passes or fails on behaviour. Reading a green compliance run as a green suite has already produced a false pass in this repository once.

## Serial Adapters

The quick gate runs the three compliance adapters serially, and compiled end-to-end behaviour only in the full gate. Order is not incidental: a failure at the unit boundary is the cheapest one to read, and reaching it first is worth more than reaching all three at once.

See [specification maintenance](specification-maintenance.md) for the cycle, and [end-to-end testing](end-to-end-testing.md) for the outermost boundary.
