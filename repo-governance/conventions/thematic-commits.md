# Thematic Commits

One theme per commit. A commit is the smallest coherent change that leaves the repository working, and its message explains why that change exists.

## Requirements

- Follow [Conventional Commits](https://www.conventionalcommits.org/). The `commit-msg` hook and the [quality gate](../development/quality-gates.md) both enforce it, so a non-conforming message never reaches `main`.
- One theme. A commit that fixes a defect and reformats four files is two commits, and the defect is invisible in the diff of the second.
- The subject says what changed. The body says why, and what a reader would otherwise have to reconstruct — the failing case, the rejected alternative, the constraint that forced the shape.
- Never describe the process. "Address review feedback" tells a future reader nothing they can act on; name the change.
- Split by theme before pushing, not by size. A large single-theme commit is fine; a small two-theme commit is not.

## Before Committing

Inspect the diff and remove anything [data safety](public-repository-data-safety.md) prohibits. The check is on the diff rather than on memory: a file added three commits ago is still being published by this one.

## Why It Matters Here

This repository's history is linear and its releases are cut from it. A commit is the unit a bisect lands on, the unit a revert removes, and the unit a release note is written from. Each of those reads a commit as one idea; a commit holding two serves none of them.

Committing and pushing require [authorization](commit-authorization.md) separately from this convention.
