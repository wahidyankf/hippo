# Rules Propagation

Where a rule change stops: at this repository's boundary.

## The Rule

A change to this tree propagates nowhere automatically. The sibling repositories hold their own governance, adapted from the same sources, and nothing keeps the copies synchronized.

That is the decision rather than an oversight. The only shared contract is the machine-checked one: each repository's own `repo-config.yml` and the validator it runs.

## When Something Should Move

Sometimes a rule discovered here genuinely applies elsewhere — a data-safety prohibition, a merge precondition. Then:

1. Change it here, with its reason.
2. Say, in the pull-request body, which other repositories may want it and why.
3. Let each of them decide, in its own change, with its own evidence.

A rule copied without that step arrives without the reason that justified it, and the first person to find it inconvenient will delete it correctly.

## When Something Should Not

A rule that fits one repository is not thereby owed to another. [Worktree location](../conventions/worktree-location.md) is the worked example: containment inside the repository is right for a Cargo project and breaks `go build`, and adopting it here because a sibling had it would have been exactly the blind adoption this workflow exists to prevent.

## Reading Divergence

Three different phrasings of the same rule across three repositories should be read as three repositories having decided, not as one having decayed. Where divergence _is_ decay, [rules grooming](rules-grooming.md) is where it is caught.
