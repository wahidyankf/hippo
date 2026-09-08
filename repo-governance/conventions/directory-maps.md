# Directory Maps

Every directory in a mapped tree carries a `README.md` with a `## Directory Map` section listing each direct sibling exactly once, by a relative path that resolves.

The mapped trees are declared in [`repo-config.yml`](../../repo-config.yml) and checked by `rhino governance directory-map validate`.

## What Is Mapped

`specs/`, `repo-governance/`, and `plans/`. All three are trees a reader navigates by structure: a specification directory whose contents are not listed is a directory whose scenarios are found by luck, and a plan stage whose folders are not listed is a stage nobody can tell is empty.

## What Is Not

`docs/` is deliberately excluded, and the exclusion is a decision rather than an omission. Its landing pages link into sections by name under [Diátaxis](documentation-architecture.md) — a reader arrives wanting a tutorial, not an inventory. Requiring every page to be named by its parent would replace a contract that serves the reader with one that serves the checker.

Adding `docs/` to the mapped trees is therefore a change to this document first.

## Requirements

- One entry per direct sibling, no more and no fewer. A stale entry and a missing entry fail the same way, and both mean the map lied.
- Each entry says what the sibling is for, in a clause. A list of filenames is something the reader could have got from `ls`.
- Relative paths, resolving inside the repository. [Markdown links](markdown-links.md) governs the link itself.
- A directory with no siblings still carries the section, stating that it holds only its own document. Silence is indistinguishable from an unmapped directory.
