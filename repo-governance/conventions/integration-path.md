# Integration Path

This repository practises trunk-based development. `main` is the trunk, every branch off it is short-lived, and history stays linear. It takes the scaled variant, where work reaches the trunk through a pull request rather than a direct commit, so the shorthand "commit to trunk" never applies here.

Local `main` has no executable path to `origin/main`: the `main` ruleset refuses direct pushes, force pushes, and branch deletion for every actor, including the repository owner. There is no bypass.

## Requirements

- Work on a branch dedicated to it, in a Git worktree at `hippo-worktrees/<name>/` beside this checkout. Never inside it — see [worktree location](worktree-location.md) for the reason, which is the Go toolchain rather than taste.
- Initialize a new worktree from its own root, before any Git mutation or gate run, with `npm ci`. That activates its hooks; a worktree whose hooks never ran pushes unverified work.
- Sync before starting and before resuming: `git fetch origin`, then `git rebase origin/main`. Never auto-stash, discard, or auto-resolve — an unclean tree or a conflict stops the work and goes to the user. When the sync brings in commits the branch lacked, read the whole incoming diff and reconcile the current task against it before continuing. A rebase of a branch already pushed needs a force push, which follows [no destructive Git operations](no-destructive-git-operations.md).
- Provision at most one worktree per plan or task and reuse it for every delivery unit that work produces. A second `git worktree add` for the same work is a defect. Units land serially: land one, sync from `origin/main`, branch the next in the same directory.
- Open one pull request per [delivery boundary](pull-request-boundaries.md), as a draft, and merge only when the [merge preconditions](pull-request-merge.md) hold.
- Reconcile local `main` after every merge. The pull request lands from the worktree, so `origin/main` moves and the primary checkout's `main` does not. Git reports no error; the checkout is simply behind until someone notices. Run `git fetch origin`, then `git merge --ff-only origin/main` there, and prove it with `git rev-list --left-right --count HEAD...origin/main` reading `0 0`. Where a clone has no primary checkout, `git fetch origin main:main` does the same job without one — never against a branch checked out somewhere, which moves the ref while leaving that tree pinned to the old commit. No hook can enforce this step: it runs after the merge, in a checkout the merge never touched.
- Keep history linear; never merge `main` into a task branch.
- A task branch is short-lived in a measurable sense: merge it the day it is created where possible, one to two days maximum, and past two days rebase or abandon it.
- Delete all three artifacts the work created — the worktree, the local branch, and that branch on `origin` — once every unit that used the worktree has landed. Confirm nothing is unpushed and nothing is running first. Retain a worktree whose run failed, and say so, rather than deleting the evidence.
- Cut a release from a worktree on `main` like any other work. A release exception would have to name a checkout by topology, and topology is a per-clone property this repository does not fix. See [release cut](../workflows/release-cut.md).

## Why the Server Enforces It

A pull request can be opened from any checkout, including one whose hooks never ran, so [`pr-quality-gate.yml`](../../.github/workflows/pr-quality-gate.yml) mirrors the `pre-commit`, `commit-msg`, and `pre-push` contracts as merge-blocking checks. Repairing a failure there follows [push-hook verification](push-hook-verification.md): fix the cause.

This convention chooses a path; it authorizes nothing. See [commit authorization](commit-authorization.md).
