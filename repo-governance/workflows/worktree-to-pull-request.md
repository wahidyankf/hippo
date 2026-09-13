# Worktree to Pull Request

The integration procedure, start to finish. The rules it enforces live in [integration path](../conventions/integration-path.md); this is the order to do them in.

## 1. Provision

```sh
git worktree add ../hippo-worktrees/<name> -b worktree/<name> origin/main
cd ../hippo-worktrees/<name> && npm ci
```

Beside the checkout, never inside it — see [worktree location](../conventions/worktree-location.md). `npm ci` activates the hooks; skip it and the branch pushes unverified.

One worktree per plan or task, reused for every delivery unit that work produces.

## 2. Sync

`git fetch origin && git rebase origin/main`, before starting and before resuming. Never auto-stash, discard, or auto-resolve. When the rebase brings in commits the branch lacked, read the whole incoming diff and reconcile the current task against it before continuing.

## 3. Work

[Thematic commits](../conventions/thematic-commits.md), each verified locally. Inspect every diff against [data safety](../conventions/public-repository-data-safety.md) before committing.

## 4. Push and Open

Push, then open the pull request **as a draft**, with a body that carries why, what was decided, and how it was proved — see [pull request body](../conventions/pull-request-body.md). Screen the title and body before opening it — see [data safety](../conventions/public-repository-data-safety.md).

## 5. Ready, Then Wait

Mark it ready. That fires `ready_for_review` and re-triggers the whole gate, so the run that matters is the one that starts now. Check no more often than [every three minutes](../conventions/github-polling.md).

## 6. Merge

When every [merge precondition](../conventions/pull-request-merge.md) holds, rebase-merge. Re-inspect the exact head being merged for data safety first.

## 7. Land the Next Unit, or Clean Up

More units: sync from `origin/main` and branch the next in the same directory.

Finished: run [dev artifact clean-up](dev-artifact-clean-up.md), which removes the worktree, both copies of the branch, and the build output this work produced, then reconciles the primary checkout.
