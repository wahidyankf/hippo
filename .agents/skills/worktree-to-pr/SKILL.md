---
name: worktree-to-pr
description: Take a change from a fresh worktree to a merged pull request in this repository, in the order the rules require.
---

# Worktree to Pull Request

The authoritative procedure is [`repo-governance/workflows/worktree-to-pull-request.md`](../../../repo-governance/workflows/worktree-to-pull-request.md). Read it before acting; this skill exists to make sure it is the thing that gets read.

## Order

1. **Provision** at `../hippo-worktrees/<name>`, beside the checkout — never inside it. Then `npm ci`, which activates the hooks.
2. **Sync** with `git fetch origin && git rebase origin/main`. Never auto-stash, discard, or auto-resolve.
3. **Work**, in thematic commits, each verified locally and each diff inspected for prohibited data.
4. **Push and open as a draft**, with a body carrying why, what was decided, and how it was proved.
5. **Mark ready**, then wait for the run that starts _because_ you marked it ready. The earlier run is not the required one.
6. **Merge** by rebase, once every precondition holds.
7. **Land the next unit** in the same worktree, or delete the worktree, the local branch, and the remote branch.

## Traps

- A worktree inside the repository breaks `go build`. The reason is in [worktree location](../../../repo-governance/conventions/worktree-location.md), and it is not negotiable.
- `pre-push` runs the whole quick gate and takes minutes. A push that returns quickly probably did not push; read the captured output rather than the exit code.
- Exit `75` from the guard means the workstation was busy. Retry the same invocation once it clears; it is not a failure of the change.
- Never `--no-verify`. Fix the cause.
- Check GitHub no more often than every three minutes, and never with a watching mode.
