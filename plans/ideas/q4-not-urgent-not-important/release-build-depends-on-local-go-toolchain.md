# Release Build Depends on the Local Go Toolchain

`scripts/build-release.sh` produces different archive bytes on a workstation whose Go toolchain differs from the one
`release.yml` uses, although [release cut](../../../repo-governance/workflows/maintenance/release-cut.md) says the
archives "do not depend on the machine that produced them".

Filed 2026-10-02 from the v0.8.3 release, whose bug-fix plan routed this learning here. Nothing published is wrong:
`release.yml` is the only publisher, and consumers pin its `checksums.txt`.

## Problem and Evidence

Before tagging v0.8.3, `scripts/build-release.sh v0.8.3 6878c25… <dir>` and `tests/artifacts/release-assets.sh` passed
locally, but all four archive checksums differed from the published `checksums.txt`. `go version` on the extracted
binaries reported `go1.27.1` for the local build and `go1.26.1` for the published one. `go.mod` declares `go 1.26.1` and
no `toolchain` line, and `release.yml` installs Go from `go-version-file: go.mod`, so CI builds with 1.26.1 while a
newer local Go builds with itself. The script clones at the exact commit and disables CGO, which removes the checkout
and the C toolchain from the build, but not the Go toolchain.

## Why Now

It does not block anything. It matters only if someone uses a local build to check a published asset, and then the
mismatch looks like tampering.

## Prior Art

Read 2026-10-02: Go's [toolchain documentation](https://go.dev/doc/toolchain) describes `GOTOOLCHAIN` and the
`toolchain` line in `go.mod`, which select the Go version a build uses. No brief or plan here mentions the toolchain.

## Proposed Direction

Either have the script build with the `go.mod` version (for example `GOTOOLCHAIN=go1.26.1`, or a `toolchain` line), and
assert that local and published checksums match, or narrow the release-cut sentence to what the script guarantees.

## Scope and Non-Goals

In scope: the build script or the release-cut wording. Not in scope: changing the Go version, or the published assets.

## Risks and Open Questions

- Archive bytes may also depend on other inputs, such as `tar` or `gzip` metadata, so matching toolchains alone might
  not make them byte-identical.

## Success

A local `scripts/build-release.sh` run at a release commit reproduces the published `checksums.txt`, or the release-cut
text no longer promises that it does.
