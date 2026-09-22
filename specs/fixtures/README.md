# Fixture Corpora

Shared fixtures this repository verifies against rather than authors.

## Corpora

[plan-structure/](plan-structure/README.md) — the plan-structure corpus. More than one implementation validates plan structure, and the claim that they agree only means something if they read the same bytes. This copy was adopted from `ose-rules` and is owned here.

[cli-conformance/](cli-conformance/README.md) — the command-line interface assertion manifest. It carries no digest either: the convention it encodes is portable prose, while the probe that exercises each assertion is local to this executable.

## Ownership

Neither copy carries an obligation upstream. There is no digest pinning it to the catalog, no pin check, and no drift ledger, because that is the catalog's own adoption model: adoption is explicit, scoped, and one-off, and nothing in `ose-rules` reaches into an adopting repository.

Parity is therefore not a standing guarantee. When the upstream convention changes, a named re-adoption task copies the new corpus in and re-runs the suite. Between such passes the copies may legitimately diverge, and this repository is entitled to that divergence.

## Verifying

```bash
cd specs/fixtures/plan-structure
shasum -a 256 -c SHA256SUMS
```

`SHA256SUMS` is a local integrity check over this copy, not a comparison against the catalog. A mismatch means these bytes changed since the digests were written, which for fixture data is a stop: whatever the validator then reports was measured against something other than the corpus under test. Regenerate it in the same change that edits the corpus.
