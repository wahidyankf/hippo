# Pull Request Merge

The preconditions for merging. All of them, every time; there is no bypass in the ruleset and none here.

## Preconditions

- The aggregate `Quality gate` check is **green on the head that will merge**. Not on an earlier head, and not on a run that a later push superseded.
- The pull request is out of draft. Marking it ready fires `ready_for_review`, which re-triggers the whole gate — so the run that matters is the one that follows, and merging before it reports leaves the base branch policy refusing the merge.
- No conversation is unresolved. The ruleset requires thread resolution.
- The branch is up to date with `main`. The ruleset requires it strictly, which is what keeps history linear.
- The diff has been inspected against [data safety](public-repository-data-safety.md) at the exact head being merged.
- Merging is [authorized](commit-authorization.md).

## Method

Rebase. Linear history is required by the ruleset, so a merge commit is refused; squashing is available but loses the [thematic commits](thematic-commits.md) the branch was built from.

## Afterwards

Delete the branch on `origin`, the local branch, and the worktree — but only once every delivery unit that used that worktree has landed. See [integration path](integration-path.md).

Check status no more often than [three minutes](github-polling.md). A gate that takes eight minutes is not made faster by being asked three times.
