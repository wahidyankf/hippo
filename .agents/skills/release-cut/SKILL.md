---
name: release-cut
description: Cut and publish a HIPPO release, with the immutability rules that make a published tag safe to pin.
---

# Release Cut

The authoritative procedure is [`repo-governance/workflows/release-cut.md`](../../../repo-governance/workflows/release-cut.md).

## Before Anything

- On `main`, synced, in a worktree. This repository's clone is bare, so there is no primary checkout to prefer.
- Working tree clean **including untracked files** — the build script verifies this itself and refuses otherwise.
- `scripts/test.sh` passes.
- `CHANGELOG.md`, `README.md`, and `docs/` are true to the binary being cut.
- The cut is authorized.

## Build

```sh
./scripts/build-release.sh <version> <commit> <output-dir>
./tests/artifacts/release-assets.sh <output-dir> <version> <commit>
```

Only through that script. It clones, detaches at the exact commit, and builds the whole platform matrix with CGO disabled, so the archives do not depend on the machine that produced them.

The commit must equal checkout HEAD and be a real 40-character object.

## Never

Replace an existing tag. Weaken checksum verification. Hand-build an asset.

Consumers pin by version **and** checksum. A replaced tag makes every one of those pins a lie, and does it silently, because the version string did not change. A mistake in a published release is fixed by publishing the next version.
