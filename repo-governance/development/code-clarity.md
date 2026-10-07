# Code Clarity

## Phase Separation

Separate distinct setup, validation, decision, mutation, and return phases in Go functions with blank lines. Automated
formatters do not replace semantic grouping: `gofumpt` will not tell a reader where validation ended and mutation began,
and that boundary is exactly what a reader is looking for when something went wrong.

A function whose phases cannot be separated is usually a function doing two things.

## Dependency Direction

HIPPO keeps policy, identity, status values and domain decisions pure. These packages depend only inward and on portable
value-processing libraries; they do not perform filesystem, process, signal, network or terminal effects.

The application layer owns use-case orchestration and declares the ports it consumes. Bounded retries, deadlines,
supervision and finalization stay there. Adapters implement effects and indivisible runtime transactions through those
ports; they do not own complete application workflows. Lease and workload handles crossing inward expose portable
capabilities rather than operating-system files, process objects or signals.

Bootstrap constructs concrete adapters and connects them to application services. Command entry points invoke that
wiring. Cobra remains in the CLI adapter and Charm TUI dependencies remain in the TUI adapter. The as-built package map
lives in [specs/architecture.md](../../specs/architecture.md).

The production import checker enforces dependency direction and framework ownership across every supported platform
source file, including files selected by build tags. Negative fixtures prove that forbidden edges are rejected.
Application port traces and runtime integration tests enforce orchestration and lifetime ownership; review checks the
remaining semantic boundary.

## Comments

Comment the non-obvious: shell safety invariants, lifecycle boundaries, and the reason a surprising line is the way it
is. Avoid line-by-line narration — a comment restating the code beneath it is a second thing to keep true and the first
to rot.

The comments worth writing are the ones a reader could not derive from the code:

- Why a `&&`/`||` chain is correct where a linter says it is not.
- Which process group a signal is allowed to reach, and which it must not.
- What an exit code will mean to the caller, at the point where it is chosen.
- Why a value is checked twice, when once looks sufficient.

## Naming

Names are part of the [public contract](public-contract.md) once they are exported. Choose the name a caller would
search for, and then leave it alone; renaming for taste costs every consumer and buys nothing.

## Shape

Prefer the shape the reader expects over the shape that is shorter. This repository's code is read while something is
failing on a machine the reader does not have, which is the least forgiving condition prose can be read in.
