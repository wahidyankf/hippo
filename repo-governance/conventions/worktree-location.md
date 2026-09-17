# Worktree Location

Every task worktree lives below this repository's location at `worktrees/<task>`. Sibling directories such as
`../hippo-worktrees/` are forbidden.

## The Rule

- From the primary checkout, create one task worktree with
  `git worktree add worktrees/<task> -b worktree/<task> origin/main`.
- Reuse that path for every delivery unit in the task. Initialize it with `npm ci` before a gate or Git mutation.
- Keep `/worktrees/` in `.gitignore` and the repository scanner exclusions.
- Run the worktree-local `./hippo`; never reach back to a primary-checkout wrapper or build output.
- After all units merge, follow [Dev Artifact Clean-Up](../workflows/dev-artifact-clean-up.md) and remove the registered
  worktree without `--force`.

## Go Provenance

Current Go releases can treat linked-worktree VCS discovery differently across layouts and versions. Development tests
do not publish their binaries. The release builder avoids ambiguity by cloning the exact commit into a temporary source
root, requiring `-buildvcs=true`, embedding the same version and commit, and checking the resulting artifacts. The
worktree layout must not weaken that release path.

`scripts/check-worktree-layout.sh` verifies registered paths and rejects tracked sibling-layout instructions. This is a
repository gate, not a convention left to memory.
