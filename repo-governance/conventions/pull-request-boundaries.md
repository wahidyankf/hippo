# Pull Request Boundaries

One pull request per delivery boundary. A boundary is a change that leaves the repository coherent on its own: someone could merge it, stop, and the repository would still make sense.

## Requirements

- One theme, at a scale a reviewer can hold. [Thematic commits](thematic-commits.md) applies within the pull request; this applies to the pull request itself.
- A boundary is not a size limit. A large single-theme change is one pull request. Two small unrelated fixes are two.
- Land units serially from one worktree: land the first, sync from `origin/main`, branch the next in the same directory. See [integration path](integration-path.md).
- A change that must be split for review but cannot be split for correctness stays one pull request, and the body says why.

## What Belongs Together

A behaviour change and the specification that governs it. A gate change and the assertions that read the file it changed. A rename and every reference to the old name. Splitting any of those leaves `main` briefly inconsistent, which is exactly what a boundary is meant to prevent.

## What Does Not

Opportunistic cleanup found on the way. It is a second pull request, said out loud rather than folded in — a reviewer cannot approve a fix they cannot find in the diff. See [minimal sufficiency](../principles/minimal-sufficiency.md).
