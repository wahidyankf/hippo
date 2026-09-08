# Release Cut

Publishing a version. A released tag is permanent: it is never rebuilt, never replaced, and never moved.

## Preconditions

- On `main`, synced with `origin/main`, in a worktree like any other work. A release is not a reason to prefer one checkout over another, and "synced" means reconciled by the [integration path](../conventions/integration-path.md), not assumed.
- The working tree is clean **including untracked files**. `scripts/build-release.sh` verifies this itself and refuses otherwise.
- `scripts/test.sh` — the full gate — passes.
- `CHANGELOG.md`, `README.md`, and `docs/` are true to the binary being cut.
- The cut is [authorized](../conventions/commit-authorization.md).

## Building

```sh
./scripts/build-release.sh <version> <commit> <output-dir>
./tests/artifacts/release-assets.sh <output-dir> <version> <commit>
```

Assets are built **only** through that script. It clones the checkout, detaches at the exact commit, and builds the full platform matrix from that one commit with CGO disabled — so the archives do not depend on the machine that produced them. A hand-built asset is an asset nobody can reproduce.

The commit must equal checkout HEAD and must be a real 40-character object. The script enforces both rather than trusting the caller.

## Immutability

**Never replace an existing release tag, and never weaken checksum verification.** Consumers pin by version _and_ checksum; a replaced tag makes every one of those pins a lie, and silently, because the version string did not change.

A mistake in a published release is fixed by publishing the next version.

## Afterwards

The pull-request gate builds the same matrix on every pull request, so a break in the asset build is found before the tag exists rather than during the cut.
