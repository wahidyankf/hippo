# Integration Path

This repository practises trunk-based development. `main` is the trunk, every branch off it is short-lived, and history stays linear. It takes the scaled variant, where work reaches the trunk through a pull request rather than a direct commit, so the shorthand "commit to trunk" never applies here.

Local `main` has no executable path to `origin/main`: the `main` ruleset refuses direct pushes, force pushes, and branch deletion for every actor, including the repository owner. There is no bypass.

## Requirements

- Work on a branch dedicated to it, in a Git worktree at `hippo-worktrees/<name>/` beside this checkout. Never inside it — see [worktree location](worktree-location.md) for the reason, which is the Go toolchain rather than taste.
- Initialize a new worktree from its own root, before any Git mutation or gate run, with `npm ci`. That activates its hooks; a worktree whose hooks never ran pushes unverified work.
- Sync before starting and before resuming: `git fetch origin`, then `git rebase origin/main`. Never auto-stash, discard, or auto-resolve — an unclean tree or a conflict stops the work and goes to the user. When the sync brings in commits the branch lacked, read the whole incoming diff and reconcile the current task against it before continuing.
- Provision at most one worktree per plan or task and reuse it for every delivery unit that work produces. A second `git worktree add` for the same work is a defect. Units land serially: land one, sync from `origin/main`, branch the next in the same directory.
- Open one pull request per [delivery boundary](pull-request-boundaries.md), as a draft, and merge only when the [merge preconditions](pull-request-merge.md) hold.
- Keep history linear; never merge `main` into a task branch.
- A task branch is short-lived in a measurable sense: merge it the day it is created where possible, one to two days maximum, and past two days rebase or abandon it.
- Delete all three artifacts the work created — the worktree, the local branch, and that branch on `origin` — once every unit that used the worktree has landed. Confirm nothing is unpushed and nothing is running first. Retain a worktree whose run failed, and say so, rather than deleting the evidence.
- Cut a release from a worktree on `main` like any other work. This repository has no primary checkout to prefer: its clone is bare, so every checkout is a worktree and a release exception would name a directory that does not exist. See [release cut](../workflows/release-cut.md).

## Why the Server Enforces It

A pull request can be opened from any checkout, including one whose hooks never ran, so [`pr-quality-gate.yml`](../../.github/workflows/pr-quality-gate.yml) mirrors the `pre-commit`, `commit-msg`, and `pre-push` contracts as merge-blocking checks. Repairing a failure there follows [push-hook verification](push-hook-verification.md): fix the cause.

This convention chooses a path; it authorizes nothing. See [commit authorization](commit-authorization.md).
