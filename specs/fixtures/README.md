# Fixture Corpora

Shared fixtures this repository verifies against rather than authors.

## Corpora

[plan-structure/](plan-structure/README.md) — the plan-structure corpus. More than one implementation validates plan structure, and the claim that they agree only means something if they read the same bytes. This copy is byte-identical to `ose-rules`, pinned by `CORPUS-DIGEST`, and regenerated nowhere.

[cli-conformance/](cli-conformance/README.md) — the command-line interface assertion manifest. Unlike the corpus above it carries no digest and is not pinned: the convention it encodes is portable prose, while the probe that exercises each assertion is local to this executable, so a copy that has diverged is a question for review rather than a checksum.

## Verifying

```bash
cd specs/fixtures/plan-structure
shasum -a 256 -c SHA256SUMS
shasum -a 256 SHA256SUMS | cut -d' ' -f1   # must equal CORPUS-DIGEST
```

A mismatch means the corpus moved. The fix is upstream in the catalog, then a fresh copy and a new digest — never a local edit to make the check pass.
