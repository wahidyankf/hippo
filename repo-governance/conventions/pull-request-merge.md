# Pull Request Merge

The preconditions for merging. All of them, every time; there is no bypass in the ruleset and none here.

## Preconditions

- The aggregate `Quality gate` check is **green on the head that will merge**. Not on an earlier head, and not on a run
  that a later push superseded.
- The pull request is out of draft. Marking it ready fires `ready_for_review`, which re-triggers the whole gate — so the
  run that matters is the one that follows, and merging before it reports leaves the base branch policy refusing the
  merge.
- No conversation is unresolved. The ruleset requires thread resolution.
- The branch is up to date with `main`. The ruleset requires it strictly, which is what keeps history linear.
- Every push of the branch passed the [push review](../workflows/quality/pr-leak-review/002-push-review.md) of each
  outgoing commit.
- One [leak review](../workflows/quality/pr-leak-review.md) by the harness's `leak-reviewer` agent, for the exact head
  being merged, reports `pass`. The harness enforces this before merging; no required status check reads it. An
  inspection nobody wrote down is not this precondition; a pass on a superseded head is not either.
- Merging is [authorized](commit-authorization.md).

## Method

Squash. The `main` ruleset's pull request rule allows only squash merges, so GitHub refuses a rebase or a merge commit.
Merge with `gh pr merge <number> --squash --match-head-commit <head>`, where `<head>` is the exact head the checks and
the leak review covered. The squash commit takes the pull request title as its subject when the branch has several
commits, so write that title as a Conventional Commit. The branch's [thematic commits](thematic-commits.md) stay in the
pull request; `main` receives one commit.

## Afterwards

Delete the branch on `origin`, the local branch, and the worktree — but only once every delivery unit that used that
worktree has landed. A squash leaves the local tip unmerged by SHA, so `git branch -d` refuses; the proof in
[dev artifact clean-up](../workflows/maintenance/dev-artifact-clean-up.md) step 3 permits `-D`. See
[integration path](integration-path.md).

Check status no more often than [three minutes](github-polling.md). A gate that takes eight minutes is not made faster
by being asked three times.
