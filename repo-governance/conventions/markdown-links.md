# Markdown Links

Every local Markdown link resolves to something inside the repository. `rhino md internal-link validate` checks it on every gate run, and a broken link fails the build rather than waiting to be found by a reader.

## Requirements

- Use relative paths. An absolute path to this repository's own file breaks the moment the tree moves or is read from a fork.
- Link the thing, not the URL. `[worktree location](worktree-location.md)` tells a reader where they are going; a bare path does not.
- A link to a directory points at its `README.md`, so the reader lands on something written rather than on a listing.
- Links with a scheme and pure fragments are outside this rule: they address something the validator is not looking at. That is also why an external link is never proof of anything — it can rot without failing anything here.

## When a Target Moves

Move the target and fix every link in the same change. A change that leaves a link dangling has not finished, and the gate will say so before the pull request can merge.

Where a document is deleted rather than moved, delete the links too. A link to a deleted document is not a smaller problem than a link to a missing one; it is the same problem with a story attached.
