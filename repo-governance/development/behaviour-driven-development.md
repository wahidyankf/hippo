# Behaviour-Driven Development

One corpus, three executing boundaries, and a static check over the mapping between them.

## The Boundaries

- **Unit** runs the corpus in process, against the packages directly. Every scenario runs here.
- **Integration** runs it against real directories, real files, and real child processes.
- **E2E** runs it against the spawned executable, which is the only boundary that sees what a caller sees.

A scenario is classified by the strongest real boundary its setup, subject, or assertions touch — not by the resources
it is permitted to use.

## Strictness

Every adapter uses strict step resolution. An undefined step fails; an ambiguous pattern fails; a registered pattern
with no handler fails; a handler no scenario reaches fails. All four are the same defect wearing different clothes: a
corpus that does not correspond to the bindings that claim to run it.

## What the Compliance Check Does Not Do

The required full-registry check is `TestFeatureCompliance` in `internal/adapters/tui`, composing the actual TUI and
neutral handlers. `tests/bdd` retains its negative validation tests. Compliance resolves bindings without executing
scenarios or starting a compiled candidate. It proves that each step has a handler, not that behaviour passes. The
executing adapters supply that proof.

## Suite Host

HIPPO adopts one external test package at `internal/adapters/tui` for the complete recursive canonical corpus, using
private TUI test exports and neutral support bindings. Validate the complete composed registry before filtering; never
copy or partition the corpus or substitute noop handlers. Existing ordinary tests remain in their directories.

Validate test-only `HIPPO_BDD_SUITE`: valid values are `unit`, `integration`, and `e2e`; empty defaults to `unit`.
Integration/E2E execution requires explicit selection; invalid values fail. Preserve it through test isolation.
`HIPPO_BDD_ADAPTER` independently selects static compliance, which must not start an executing entry. Mode selection
does not relax the genuine boundaries above.

Suite relocation, mode enforcement and selector/removal fixtures are present in source; their required execution and
final acceptance remain open. [Quality Gates](quality-gates.md) owns mandatory registration and the unchanged
coverage/race obligations. Reviewed [E2E exemptions](end-to-end-testing.md#exemptions) still require exact inventory
enforcement; an unsupported E2E handler must fail if its exemption is absent or removed, never pass through a model
fallback. Unit retains every scenario; native-I/O Integration retains every terminal-fault predicate. For this release,
the acceptance owner approved E2E deferral of exactly six forced fault rows in the reviewed inventory: four terminal
failures, restoration failure precedence over interruption, and global-JSON restoration-before-report. Unit/Integration
must freshly pass the original six rows/29 steps, and the compiled CLI must pass both ordinary readonly-output
regressions. The latter prove classified failure and post-Wait restoration, not the deferred public fault/order
predicates; deferred rows never count as PASS.

## Serial Adapters

The quick gate runs the three static compliance adapters serially. [E2E placement](end-to-end-testing.md#placement) owns
the full-only executing corpus and public suites. Order is not incidental: a failure at the unit boundary is the
cheapest one to read, and reaching it first is worth more than reaching all three at once.

See [specification maintenance](specification-maintenance.md) for the cycle, and
[end-to-end testing](end-to-end-testing.md) for the outermost boundary.
