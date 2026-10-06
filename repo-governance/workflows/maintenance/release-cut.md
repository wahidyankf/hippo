# Release Cut

Publishing a version. A released tag is permanent: it is never rebuilt, never replaced, and never moved.

## Preconditions

- On `main`, synced with `origin/main`, in a worktree like any other work. A release is not a reason to prefer one
  checkout over another, and "synced" means reconciled by the [integration path](../../conventions/integration-path.md),
  not assumed.
- The working tree is clean **including untracked files**. `scripts/build-release.sh` verifies this itself and refuses
  otherwise.
- `scripts/test.sh` — the full gate — passes.
- A [docs quality gate](../quality/docs-quality-gate.md) verdict on subject `all` is recorded, so `CHANGELOG.md`,
  `README.md`, and `docs/` are checked against the binary being cut. A documented contract the binary breaks is fixed in
  code before the tag. Output they quote, diagnostics included, is measured on a binary built with the Go release
  `go.mod` names, as `scripts/build-release.sh` builds it: another local Go can print different text.
- The tag name and the notes the release will publish pass the screen in
  [data safety](../../conventions/public-repository-data-safety.md). `release.yml` generates those notes from merged
  pull requests, so screen the text
  `gh api repos/<owner>/<repo>/releases/generate-notes -f tag_name=<version> --jq .body` returns before tagging.
- The cut is [authorized](../../conventions/commit-authorization.md). An adopted
  [upstream tool defects](../../development/upstream-tool-defects.md) standard authorizes releasing a merged HIPPO
  defect fix, whether a consumer filed it or this repository found it.

## Building

```sh
./scripts/build-release.sh <version> <commit> <output-dir>
./tests/artifacts/release-assets.sh <output-dir> <version> <commit>
```

Assets are built **only** through that script. It clones the checkout, detaches at the exact commit, and builds the full
platform matrix from that one commit with CGO disabled, on the Go release that commit's `go.mod` names — fetched when
the local Go differs — and without the host's Go settings. `scripts/release-archive` then writes each archive with the
commit's time in place of the build's, so the archives do not depend on the machine that produced them: a rebuild at a
release commit reproduces that release's `checksums.txt` byte for byte. The pull-request gate proves it by rebuilding
every change on macOS and requiring the Linux checksums. A hand-built asset is an asset nobody can reproduce.

Go also stamps the module version into each binary from the tags the checkout holds, so a rebuild matches only with the
same tags present. To check a published release, build its tag's commit in a checkout that has fetched its tags, into an
empty directory, and `diff` the result's `checksums.txt` against the published one. Releases up to v0.8.3 predate the
pinned toolchain and archiver: their archives carry the time they were built and never reproduce, so compare the
extracted binaries instead, built with that release's Go.

The commit must equal checkout HEAD and must be a real 40-character object. The script enforces both rather than
trusting the caller.

## Immutability

**Never replace an existing release tag, and never weaken checksum verification.** Consumers pin by version _and_
checksum; a replaced tag makes every one of those pins a lie, and silently, because the version string did not change.

A mistake in a published release is fixed by publishing the next version.

## Afterwards

The pull-request gate builds the same matrix on every pull request, so a break in the asset build is found before the
tag exists rather than during the cut.
