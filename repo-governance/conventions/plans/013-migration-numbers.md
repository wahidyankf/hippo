# Migration Numbers

A migration takes its number when it is written, never from a number a document recorded earlier. The obligations below
bind every migration this repository governs. The repository holds no migration today, so they bind the first one it
adds.

## Obligations

Each obligation stands alone, and each is judged on its own observations.

1. **A new migration takes the next free number on current `origin/main`.** Fetch main, then read its migration
   directory and its known-migration catalog. Followed: the number is one above the highest number on `origin/main`.
   Violated: the number is already used there, or is more than one above that highest.
2. **A number named in a plan, brief, or other document is a hint.** It reserves no number and authorizes no migration
   to take it. Followed: the document calls the number a hint, or names none. Violated: the document says the number is
   reserved, claimed, or fixed, or a migration takes a number because a document named it.
3. **A migration whose documented number is taken on `origin/main`, or would leave a gap, takes the next free number and
   updates every stale reference in the same change.** Followed: the migration takes the next free number, and no
   reference in the change names the old one. Violated: the migration keeps a taken number or leaves a gap, or a
   reference still names the old number.
4. **An applied or merged migration is never renumbered or renamed.** Followed: its file name is unchanged in the
   change. Violated: a change renames or renumbers such a file.

The reason is one sequence. The migration journal, where a repository keeps one, requires a contiguous applied prefix,
and a plan written before the work starts cannot know which number will be free when the work merges. The first change
to merge takes the next free number, and every other change reads `origin/main` when it executes. A renamed applied
migration runs again, because the journal identifies it by name.

## Interaction With Renumbering

Obligation 4 holds even where [Renumbering Is Atomic](003-technical-shape-and-companions.md#renumbering-is-atomic)
renumbers a companion set. A migration takes a free number and moves no other, so the insertion that section describes
never reaches one.

## Enforcement

Obligations 1, 3, and 4 are unenforced by decision. A check would compare the change with `origin/main` and read the
migration directory. This repository declares no such check: [`repo-config.yml`](../../../repo-config.yml) has no gate
that reads a migration directory or compares migration names with `origin/main`, and the repository holds no migration
to check. A gate declared without an executor would fail or never run, so none is declared. Obligation 2 is unenforced
by decision: whether a sentence reserves a number is a judgement a reader makes.

## Adoption

Adopted by explicit copy from the shared `ose-rules` catalog at catalog head `81982e20c798d8af6dbb85ae446d39d0432b6362`
(`main`). Changed here, and why: the rule's home is this plans module, because this repository has no release cutover or
migration catalog, and plans are where numbers get named. The obligations are unchanged.
