# Coding Harness Parity Verification

Proving the roster still reconciles — and that the check would notice if it did not.

## The Check

```sh
./rhino harness adapters validate
```

It validates the full declared adapter tree without writing it. The generated catalog and provenance artifacts record the canonical sources and their digests. A configuration with fewer than the required three profiles is refused before validation.

## Proving the Check Works

A validator that never fails proves nothing. At least once per contract change, break one thing deliberately and confirm the finding:

| Weakening                              | Expected finding    |
| -------------------------------------- | ------------------- |
| grant a tool the canon denies          | `divergent-adapter` |
| flip a permission from deny to allow   | `divergent-adapter` |
| add an undeclared native adapter       | `stale-adapter`     |
| copy canonical content into an adapter | `divergent-adapter` |

Restore afterwards and confirm green.

## Prohibited Instruction Sources

The adapter validator reports undeclared files inside a native adapter root as `stale-adapter`. It does not scan arbitrary nested instruction filenames in v0.4, so do not represent an adapter-validation run as proof of that separate product-level prohibition.

## Narrowing

Validation is whole-roster only: every declared profile and generated artifact is compared in the same read-only run.

## When to Run It

On every contract change, and on every gate run: the check is a [declared gate](../development/software-quality-enforcement.md) on the `pre-push`, `pull-request`, and `main` surfaces.
