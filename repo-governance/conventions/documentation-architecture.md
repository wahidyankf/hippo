# Documentation Architecture

`docs/` follows [Diátaxis](https://diataxis.fr/). Every page belongs to exactly one of `tutorials/`, `how-to/`, `reference/`, or `explanation/`.

## The Four Modes

- **Tutorials** teach by doing. The reader is new and follows along; the page succeeds if they reach a working result without deciding anything.
- **How-to guides** solve one stated problem for a reader who already knows what they want.
- **Reference** describes what is, exactly and exhaustively: commands, flags, exit codes, configuration keys.
- **Explanation** gives the reasoning. Why the exit codes mean what they mean, why the guard owns only its own process group.

A page serving two modes serves neither. Split it.

## Truth

`specs/` is canonical. `README.md`, `docs/`, and `CHANGELOG.md` must be true to the shipped binary, and where any of them contradicts the specification, the specification wins and the document changes.

**Never publish a command or a transcript that has not been executed against the current build.** An invented transcript is indistinguishable from a real one to every reader, and it stays wrong long after the behaviour it describes has changed. Where a path cannot be exercised safely — a destructive operation, a host state that cannot be arranged — say so plainly instead.

## Structure

`docs/` is deliberately outside the [directory map](directory-maps.md) requirement: its landing pages link into sections by name, which is a different contract from naming every direct sibling once.

Rules for contributors do not live in `docs/`. They live in this tree, indexed from [`AGENTS.md`](../../AGENTS.md); `docs/` is written for the people who use the binary.
