# Docs Quality Gate

This gate follows the [Quality Gate Contract](../../development/workflow/quality-gate-contract.md): a read-only checker,
a frozen ledger, one separate writer, at most three cycles, and an advisory verdict. This file states only what is
specific to documents.

## Entry

The gate starts only on an explicit request that names it, or from [Release Cut](../maintenance/release-cut.md), which
runs it with subject `all` before publishing. An adopting repository lists only the callers it has, chosen from this
set. A change or a propagation run never starts it alone.

## Inputs

| Input        | Type    | Values                                              | Default  |
| ------------ | ------- | --------------------------------------------------- | -------- |
| `subject`    | string  | `all`, or one revision range or working-tree change | required |
| `mode`       | enum    | `lax`, `normal`, `strict`, `all`                    | `normal` |
| `max-cycles` | integer | 1, 2, or 3                                          | 3        |

A change subject covers the documents it touches and every document citing what it changed; `all` covers the whole scope
[Docs Propagation](docs-propagation.md) defines. Any other `max-cycles` value, or a missing subject, refuses to start.

## Deterministic Boundary

The checker reports none of these properties. The entry and exit checks run their owners instead.

| Property                            | Owned by                  | This repository runs                        |
| ----------------------------------- | ------------------------- | ------------------------------------------- |
| Markdown formatting and line length | the formatter and linter  | `prettier`, `markdownlint-cli2`             |
| Internal links and anchors          | the link validator        | `./rhino md internal-link validate`         |
| Directory maps                      | the map validator         | `./rhino governance directory-map validate` |
| Word budgets                        | the word-budget validator | `./rhino governance word-budget validate`   |
| Mermaid diagrams                    | the Mermaid validator     | `./rhino md mermaid validate`               |

A property no tool of its own owns leaves the table and becomes judgeable.

## Cycle

Each cycle is one full audit by `docs-checker` and one repair by [Docs Propagation](docs-propagation.md), run by
`docs-fixer`, per
[Sequence and Termination](../../development/workflow/quality-gate-contract/002-sequence-and-termination.md). The audit
may delegate reading to the repository's documentation checkers. It asks of each document whether:

1. every claim is true to the implementation, per [Factual Validation](../../conventions/documentation-architecture.md),
   and every command shown was run or is marked not exercised, per
   [Only What Was Run](../../conventions/documentation-architecture.md#truthfulness);
2. it still describes something the repository has; if not, it is obsolete and its resolution is removal;
3. each fact has one home, a summary sits above its detail per
   [Progressive Disclosure](../../principles/progressive-disclosure.md), and a page serves one mode per
   [Documentation Architecture](../../conventions/documentation-architecture.md);
4. a newcomer learns from the opening what it is and why it matters, and finds the next step, per README Quality and
   Content Quality, judged by reading, never by a score;
5. under `all`, or when setup changed, a reader with no prior context can follow the setup as written from a clean
   checkout; and
6. it agrees with its specification, which is canonical.

Only a document that is wrong, obsolete, unreachable, or unusable by a newcomer is a finding; wording preference is not.
A row only the owner can settle, such as a specification that disagrees with the implementation, is `needs-decision`.

## Termination

The contract's
[termination table](../../development/workflow/quality-gate-contract/002-sequence-and-termination.md#termination)
applies unchanged. This gate adds no row.

## Verdict

| Verdict              | The caller                                                                          |
| -------------------- | ----------------------------------------------------------------------------------- |
| `PASS`               | records the verdict and continues                                                   |
| `PASS_WITH_FINDINGS` | records the verdict and the open non-blocking rows, and continues                   |
| `FAIL`               | gives each open blocking row an owner (idea brief, plan item, or issue), continues  |
| `BLOCKED`            | records the cause (tooling, input-changed, or unavailable), then acts as for `FAIL` |

No verdict stops the caller, and none authorizes a commit or a push.

## Ledger

`local-tmp/quality/docs/<subject-slug>__<YYYYMMDDTHHMMZ>.md`, with the columns and closing verdict block in
[the contract](../../development/workflow/quality-gate-contract/003-verdicts-ledger-and-relations.md#ledger). It is
never committed.

## Example Usage

```text
Run docs-quality-gate on subject all.
```

## Related Workflows

- [Docs Propagation](docs-propagation.md) repairs every blocking row, removals included.
- [Release Cut](../maintenance/release-cut.md) runs this gate before publishing.
