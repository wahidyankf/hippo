# Coding Harness Parity Verification

Proving the roster still reconciles — and that the check would notice if it did not.

## The Check

```sh
rhino harness parity validate
```

It reports the number of harnesses reconciled, the canon it read, and a digest over everything it saw. Read all three. `checked 0 harnesses, no findings` is not a pass; it is a roster that reconciles against nothing.

## Proving the Check Works

A validator that never fails proves nothing. At least once per contract change, break one thing deliberately and confirm the finding:

| Weakening                              | Expected finding                         |
| -------------------------------------- | ---------------------------------------- |
| grant a tool the canon denies          | the adapter grants what the canon denies |
| flip a permission from deny to allow   | the permission is not `deny`             |
| widen a sandbox mode                   | the sandbox mode is not read-only        |
| add a declaration to a closed wrapper  | beyond what a wrapper may declare        |
| copy the canonical body into a wrapper | the body is not the canonical route      |

Restore afterwards and confirm green.

## Prohibited Instruction Sources

The same run reports any file that would compete with the canon. Prove that too: add a nested instruction file, see it reported, remove it. A prohibition nobody has ever seen fire is a prohibition nobody knows is loaded.

## Narrowing

`--harness <name>` reconciles one declared harness. A name the repository does not declare is refused outright — narrowing asks a smaller question, never a quieter answer to the same one.

## When to Run It

On every contract change, and on every gate run: the check is part of [`scripts/docs-check.sh`](../development/software-quality-enforcement.md), so it runs before every push and in the pull-request gate.
