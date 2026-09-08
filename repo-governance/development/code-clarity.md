# Code Clarity

## Phase Separation

Separate distinct setup, validation, decision, mutation, and return phases in Go functions with blank lines. Automated formatters do not replace semantic grouping: `gofumpt` will not tell a reader where validation ended and mutation began, and that boundary is exactly what a reader is looking for when something went wrong.

A function whose phases cannot be separated is usually a function doing two things.

## Comments

Comment the non-obvious: shell safety invariants, lifecycle boundaries, and the reason a surprising line is the way it is. Avoid line-by-line narration — a comment restating the code beneath it is a second thing to keep true and the first to rot.

The comments worth writing are the ones a reader could not derive from the code:

- Why a `&&`/`||` chain is correct where a linter says it is not.
- Which process group a signal is allowed to reach, and which it must not.
- What an exit code will mean to the caller, at the point where it is chosen.
- Why a value is checked twice, when once looks sufficient.

## Naming

Names are part of the [public contract](public-contract.md) once they are exported. Choose the name a caller would search for, and then leave it alone; renaming for taste costs every consumer and buys nothing.

## Shape

Prefer the shape the reader expects over the shape that is shorter. This repository's code is read while something is failing on a machine the reader does not have, which is the least forgiving condition prose can be read in.
