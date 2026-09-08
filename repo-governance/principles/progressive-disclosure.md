# Progressive Disclosure

Root [`AGENTS.md`](../../AGENTS.md) is an index. It names where each rule lives and states none of its own. Every rule lives in exactly one document in this tree, and every document is reached by following a link from the index.

## Why

An instruction file that holds every rule is read once and skimmed thereafter. The rules that matter most are the ones a reader is least likely to reach, because they are the ones furthest down. Word budgets exist to make that failure visible — [`repo-config.yml`](../../repo-config.yml) fails the gate at 650 words for the index and 750 for any document here.

The budget is not a suggestion to write tersely. It is a structural claim: a document that cannot say what it means in 750 words is describing more than one thing, and splitting it gives each part a reader who wants it.

## Requirements

- The index links; it does not restate. A rule written in two places drifts in one of them.
- A document states its rule and its reason. A rule without a reason is followed until it is inconvenient and then discarded.
- Link to a higher-level document rather than repeating it, so the hierarchy in [the tree README](../README.md) stays real.
- When a document outgrows the budget, split it by reader task rather than by size. Never raise the limit, and never cut substance to fit.

## What This Costs

More files, and one more hop to reach any given rule. That is the trade: the index stays short enough to be read completely, and a rule that is read completely is a rule that is followed.
