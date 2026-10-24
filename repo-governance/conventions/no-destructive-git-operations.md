# No Destructive Git Operations

Git keeps no undo for uncommitted work, and a rewrite of pushed history reaches every clone that holds it. Assume
someone else is using the same machine and remote at the same moment.

Adapted from the catalog rule of the same name, with its bounded task sync as of catalog commit
`9e15c702ef01187f5a3fafca975d2891854c0495`. What changed: a secret in history follows
[data safety](public-repository-data-safety.md), which rules out rewriting here, an unused task branch is deleted under
the proofs of [dev artifact clean-up](../workflows/maintenance/dev-artifact-clean-up.md), and the sync module names this
repository's own gates.

## The Rule Is the Effect

Any Git invocation whose effect is to discard uncommitted changes, destroy work this actor did not create, rewrite
history others may hold, or remove the means of recovering any of those needs explicit approval for that one instance,
except the bounded [authorized task sync](no-destructive-git-operations/001-authorized-task-branch-sync.md). The
spelling is irrelevant: an alias, a script, or an unlisted flag with the same effect is covered. Ask what the command
destroys and who made it, not whether it appears below.

## Common Cases

- `git push --force`, or `--force-with-lease` without an expected value
  - Destroys: remote commits absent locally
  - Use instead: `--force-with-lease=<branch>:<expected-sha> --force-if-includes`
- **rebasing or amending commits already pushed**
  - Destroys: history others built on
  - Use instead: a new commit; for an authorized task's own branch, the sync rebase and the lease form above
- `git reset --hard`, `git checkout -f`, `git switch --discard-changes`
  - Destroys: uncommitted changes
  - Use instead: commit first, or `git stash push -- <path>` and keep the entry
- `git checkout -- <path>` or `git restore <path>` over edits
  - Destroys: the unstaged edits at those paths
  - Use instead: commit or stash first
- `git clean -fd` or `git clean -fdx`
  - Destroys: untracked and ignored files
  - Use instead: `git clean -n` to preview, then delete named paths
- `git stash drop`, `git stash clear`
  - Destroys: stash entries, which then become prunable
  - Use instead: leave the entries
- `git branch -D`, `git update-ref -d`
  - Destroys: a branch, skipping the merged check
  - Use instead: `git branch -d`; an unused task branch follows the section below
- **expiring the reflog and pruning at once**
  - Destroys: the recovery path itself
  - Use instead: let automatic maintenance run
- `git worktree remove --force`, deleting a worktree folder
  - Destroys: a working tree and everything uncommitted in it
  - Use instead: plain `git worktree remove`

The lease form still needs per-instance approval outside the authorized sync contract; its lease guard does not prove
task ownership or authorization.

## Unused Task Branches

Deleting the worktree and both copies of the branch once every unit that used them has landed is required cleanup, not
an operation this rule gates. A squash merge puts one new commit on `main` and never the branch's own commits, so
`git branch -d` refuses. `git branch -D` on a task branch is the one exception to the list above, and only under the
landed or stale proof and the preservation step of
[dev artifact clean-up](../workflows/maintenance/dev-artifact-clean-up.md) step 3. Any other `-D` still needs approval.

## Asking for Approval

Outside the authorized sync contract, first look for a route that destroys nothing: a new commit, a revert, a removal
without force. When none exists:

1. State the exact command as it will run.
2. State what it affects: the ref, the commits left unreachable where they can be determined, and the paths.
3. Ask a yes-or-no question and wait for the answer.
4. Run exactly what was approved. If a flag, ref, or target changes, ask again.

Such per-instance approval never carries forward. The repository can change between two operations, and the earlier
answer was about a state that no longer exists.

## Worktrees Share State

Worktrees share the object database and every ref, so pruning and forced removal reach state other worktrees depend on.
Never pass `--ignore-other-worktrees`. A destructive operation in a worktree this task does not use needs positive
evidence that the worktree is idle.

## Prefer Additive

When a destructive and an additive operation reach the same end state, take the one that leaves a trail. Before any bulk
deletion, run its dry-run form.

## Enforcement

No gate checks this. A hook cannot tell an approved or authorized force push from an unapproved one, so the rule depends
on attention. The `main` ruleset refuses force pushes and deletion of `main` itself, for every actor.
