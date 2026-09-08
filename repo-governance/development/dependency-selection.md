# Dependency Selection

Every dependency is a promise to keep: to update it, to audit it, and to answer for it when it breaks in a consumer's build.

## Before Adding One

- **Is it needed?** The standard library and the Go toolchain cover most of what this repository does. Reach for them first — see [minimal sufficiency](../principles/minimal-sufficiency.md).
- **What does it pull in?** A dependency's own tree is part of what is being adopted. `govulncheck` runs over all of it in the full gate.
- **What happens when it is abandoned?** Prefer something small enough to vendor or replace over something large enough to be unavoidable.
- **Does it reach the network, the filesystem, or the process table?** Those are the boundaries this repository is careful about; a dependency that crosses one crosses it on HIPPO's behalf.

## Tooling Dependencies

Contributor tooling is locked and installed with `npm ci`. A floating version is a gate that passes today and fails tomorrow for a reason nobody changed.

The pinned RHINO release in `rhino.lock` is pinned by version _and_ checksum, and resolved through a wrapper that verifies before executing. An unverified download in a gate is a supply chain this repository did not choose.

## Removing One

Removing a dependency is a change like any other: it lands with the code that stopped needing it, and the lock file moves in the same commit. A lock file updated separately is a diff nobody can review.
