# Thematic Commits

One theme per commit. A commit is the smallest coherent change that leaves the repository working, and its message
explains why that change exists.

## Requirements

- Follow [Conventional Commits](https://www.conventionalcommits.org/). The `commit-msg` hook and the
  [quality gate](../development/quality-gates.md) both enforce it, so a non-conforming message never reaches `main`.
- One theme. A commit that fixes a defect and reformats four files is two commits, and the defect is invisible in the
  diff of the second.
- The subject says what changed.
- The body explains why the change was needed, using context a reader cannot infer from the diff — the failing case,
  rejected alternative, or constraint that forced the shape.
- Do not narrate what the code does in the body.
- Never describe the process. "Address review feedback" tells a future reader nothing they can act on; name the change.
- Split by theme before pushing, not by size. A large single-theme commit is fine; a small two-theme commit is not.

## Before Committing

Inspect the diff and remove anything [data safety](public-repository-data-safety.md) prohibits. The check is on the diff
rather than on memory: a file added three commits ago is still being published by this one.

## Why It Matters Here

This repository's history is linear and its releases are cut from it. A commit is the unit a bisect lands on, the unit a
revert removes, and the unit a release note is written from. Each of those reads a commit as one idea; a commit holding
two serves none of them.

Committing and pushing require [authorization](commit-authorization.md) separately from this convention.

## Enforcement

The hook and quality gate check Conventional Commits. Review judges the reason in the body and whether its context adds
value; these rules are unenforced by decision because a parser cannot distinguish useful context from narration.
