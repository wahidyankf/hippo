# GitHub Actions storage

## Purpose

Keep HIPPO's GitHub Actions storage within free account allowances and make every storage-producing
workflow change reviewable. These rules cover Actions artifacts, GitHub Packages, Actions caches,
and the account-level control that prevents paid Actions overage.

GitHub Release assets are immutable release distribution files. They are distinct from Actions
artifacts, caches, and GitHub Packages, so the release workflow continues to manage them through its
tag and checksum guarantees.

## Standards

### Actions artifacts

Every `actions/upload-artifact` step must declare `retention-days`:

- Use `1` when an artifact only transfers data between jobs in one workflow run.
- Use at most `7` when an artifact exists for failure triage or manual inspection.
- A value greater than `7` requires an adjacent comment explaining why shorter retention cannot
  satisfy the use case.

Keep repository artifact/log retention at or below `7 days` as a backstop; explicit per-upload
retention remains the primary control.

A pull request violates this rule if it adds or changes an upload without the required retention,
uses more than seven days without an adjacent justification, or labels a long-lived deliverable as
an Actions artifact. Publish immutable release deliverables as GitHub Release assets instead.

### GitHub Packages

Every workflow that publishes to GitHub Packages must declare all of the following beside the
publishing job or in documentation linked directly from that job:

- the package-version cleanup or lifecycle policy;
- the steady-state repository storage estimate;
- reconciliation of that estimate with the repository owner's shared 500 MB GitHub Free allowance
  for Actions artifacts and GitHub Packages.

A package publisher without all three declarations violates this rule. GitHub Release assets are
outside this package lifecycle requirement.

### Actions caches

Treat Actions cache storage as a separate repository allowance:

- Keep the repository cache limit at or below `10 GB`.
- Keep cache retention at or below `7 days`.
- Forecast cache size multiplied by concurrently live cache scopes whenever a workflow changes a
  cache key, operating-system matrix, dependency set, or triggering ref.
- Use a restore-only cache action on pull requests and other non-default refs when the forecasted
  ref churn could exceed the limit. Only the default branch may save in that case.

A cache setting above either bound, or a cache-writing design forecast to exceed `10 GB` without
restore-only non-default refs, violates this rule.

### Paid-overage control

The personal-account owner must maintain a GitHub Actions budget of `$0`; GitHub user-level budgets
always hard-stop automatically and do not offer a stop-usage toggle. Organization owners using this
rule must also enable **Stop usage when budget limit is reached**. A repository administrator must
verify the applicable control in billing settings because repository files cannot inspect it.

This account-level control is explicitly **unenforced by repository automation**. Repository review
must not claim the no-paid-overage guarantee is complete without current external verification.

## Examples

Use one-day retention for an intra-run handoff:

```yaml
- uses: actions/upload-artifact@v4
  with:
    name: test-input
    path: dist/
    retention-days: 1
```

When pull-request cache churn could cross the repository limit, restore everywhere but save only on
the default branch:

```yaml
- uses: actions/cache/restore@v5
  if: github.ref != 'refs/heads/main'
  with:
    path: ~/.cache/example
    key: example-${{ runner.os }}-${{ hashFiles('package-lock.json') }}
- uses: actions/cache@v5
  if: github.ref == 'refs/heads/main'
  with:
    path: ~/.cache/example
    key: example-${{ runner.os }}-${{ hashFiles('package-lock.json') }}
```

## Validation

For every workflow change:

1. Search `.github/workflows/` and local actions for artifact uploads, package publishers, explicit
   caches, and setup-action caches.
2. Check artifact retention and any package lifecycle declarations against the standards above.
3. Calculate the cache forecast from cache-entry size, matrix width, and concurrently live ref/key
   scopes; require restore-only non-default refs if the forecast can exceed `10 GB`.
4. Confirm repository artifact/log retention is at most `7 days`, cache limit is at most `10 GB`,
   and cache retention is at most `7 days` in GitHub Actions settings.
5. Have a repository administrator record current external verification of the owner's `$0` Actions
   hard-stop budget in the pull request.
6. Run `npm run test:quick` before delivery.

The first two storage-producer standards are policy-reviewed until a relevant artifact or package
publisher exists. Cache compliance combines workflow forecast review with external repository
settings. The account budget remains a manual, external control.
