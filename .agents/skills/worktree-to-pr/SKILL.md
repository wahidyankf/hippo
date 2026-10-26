---
name: worktree-to-pr
description: Take a change from a fresh worktree to a merged pull request in this repository, in the order the rules require.
when_to_use: >-
  Use when a change starts in a fresh worktree and must reach a merged pull request.
---

# Worktree to Pull Request

The authoritative procedure is [`repo-governance/workflows/maintenance/worktree-to-pull-request.md`][worktree-workflow].
Read it before acting; this skill exists to make sure it is the thing that gets read.

## Order

1. **Provision** below the repository location with the command in the workflow's Provision step, per
   [worktree location](../../../repo-governance/conventions/worktree-location.md). Then `npm ci`, which activates the
   hooks.
2. **Sync** with `git fetch origin && git rebase origin/main`. Never auto-stash, discard, or blindly auto-resolve. The
   [authorized task sync][task-sync] covers only the owned task branch: its owned, settled conflicts, a recovery copy of
   the pre-sync commits, and a rewrite pushed only with the recorded expected SHA as its lease plus
   `--force-if-includes`. A conflict or effect outside the contract stops the work.
3. **Work**, in thematic commits, each verified locally and each diff inspected for prohibited data.
4. **Push and open as a draft**, with a body carrying why, what was decided, and how it was proved.
5. **Mark ready**, then wait for the run that starts _because_ you marked it ready. The earlier run is not the required
   one.
6. **Merge** by squash, the only method the `main` ruleset allows, with
   `gh pr merge <number> --squash --match-head-commit <head>`, once every precondition holds.
7. **Land the next unit** in the same worktree, or delete the worktree, the local branch, and the remote branch.

## Traps

- A sibling `*-worktrees/` directory is forbidden, and `scripts/check-worktree-layout.sh` fails the quick gate when a
  worktree or a tracked instruction uses one.
- `pre-push` runs the whole quick gate and takes minutes. A push that returns quickly probably did not push; read the
  captured output rather than the exit code.
- Exit `124` from the guard means a limit stopped the work, and on a workstation that usually means it was busy. The
  reason on stderr says which limit. Retry the same invocation once it clears; it is not a failure of the change.
- `--no-verify` only under the [worktree-to-PR exception][hook-exception]: safety screens and the push leak review must
  pass, `commit-msg` stays on, and the PR gate must pass on the pushed head. A reported hook finding is still fixed at
  its cause.
- Check GitHub no more often than every three minutes, and never with a watching mode.

[worktree-workflow]: ../../../repo-governance/workflows/maintenance/worktree-to-pull-request.md
[task-sync]: ../../../repo-governance/conventions/no-destructive-git-operations/001-authorized-task-branch-sync.md
[hook-exception]: ../../../repo-governance/conventions/push-hook-verification.md#worktree-to-pr-exception
