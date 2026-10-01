# Enforcement

A precondition stated only in prose is a convention someone forgets. The leak review is enforced in three layers; each
fails closed.

## The Screen

[`scripts/public-safety/check.sh`](../../../scripts/public-safety/README.md) runs as the `public-safety-range` gate in
[`repo-config.yml`](../../../repo-config.yml): at `pre-push` over each pushed range, with `origin/main` as the fallback
base, and on the pull request's base-to-head range. It screens the range commit by commit: each commit's added lines at
the line numbers they occupy, its file names, its message, and the ref names. A merge contributes what it resolved
beyond the automatic merge. Content the range did not add is not screened again, so the screen binds from adoption
onward. Its shapes include absolute home paths, private addresses, and internal hostnames, beside a credential scanner.
Case `160-range-history` proves it.

The screen matches shapes; the review reads context. Neither replaces the other.

## Hosted Checks

- **Range screen.** The `Repository contract` job of
  [`pr-quality-gate.yml`](../../../.github/workflows/pr-quality-gate.yml) replays the `pull-request` surface over the
  pull request's base-to-head range, because a local hook can be skipped and a hosted check cannot. It reports through
  the aggregate `Quality gate` check.
- **Record check.** [`leak-review.yml`](../../../.github/workflows/leak-review.yml) runs
  [`record-status.sh`](../../../scripts/leak-review/README.md), which publishes the `leak-review` commit status on the
  live head: `success` only when the repository owner's latest undismissed review on that head carries a `pass` record
  naming this repository, the pull request, and the head. It runs when the pull request changes and when a review is
  submitted, so posting the record turns it green without a new commit. The `leak-review-tests` gate runs its offline
  suite.

Both `Quality gate` and `leak-review` are required on `main`, through its ruleset. A waiver of other gates never covers
either.

## Adopter Decisions

- **Marker:** `ose-pr-leak-review`, which never changes; records posted before adoption stay valid.
- **Reviewer identity:** the repository owner.
- **Required checks:** `Quality gate` for the range screen, and `leak-review` for the record.
- **Integration path:** pull request, per [integration path](../../conventions/integration-path.md).
