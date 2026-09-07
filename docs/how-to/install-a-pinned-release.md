# How to install a pinned release

Use a tagged release binary rather than building from source when you want a reproducible,
verified HIPPO on a developer machine or a CI runner.

**Pin both the tag and the expected SHA-256. Never follow `main` at runtime.**

## Choose your asset

Releases publish four archives plus one checksum inventory:

```text
hippo_<version>_darwin_amd64.tar.gz
hippo_<version>_darwin_arm64.tar.gz
hippo_<version>_linux_amd64.tar.gz
hippo_<version>_linux_arm64.tar.gz
checksums.txt
```

## Download, verify, extract

```sh
VERSION=v0.5.1
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
[ "$ARCH" = x86_64 ] && ARCH=amd64
[ "$ARCH" = aarch64 ] && ARCH=arm64
ASSET="hippo_${VERSION}_${OS}_${ARCH}.tar.gz"
BASE="https://github.com/wahidyankf/hippo/releases/download/${VERSION}"

curl -fsSLO "${BASE}/${ASSET}"
curl -fsSLO "${BASE}/checksums.txt"

# Verify before extracting. Do not skip this step.
if command -v sha256sum >/dev/null 2>&1; then
  grep " ${ASSET}\$" checksums.txt | sha256sum -c -
else
  grep " ${ASSET}\$" checksums.txt | shasum -a 256 -c -
fi

tar -xzf "$ASSET"
```

```console
hippo_v0.5.1_darwin_arm64.tar.gz: OK
```

Each archive contains exactly one regular mode-755 member named `hippo`.

## Confirm what you got

```sh
./hippo version --json
```

```console
{"schemaVersion":1,"version":"v0.5.1","commit":"5722854fddfd68b1fc7ca9feca935fe3e7eec625"}
```

The reported `version` and `commit` are baked in at link time, so this is a positive identification
of the build — not a claim the binary reads from a file beside it.

Record that `commit` alongside the SHA-256 in whatever pins your toolchain.

## Put it on PATH

```sh
install -m 755 hippo /usr/local/bin/hippo
hippo version
```

On a machine where you cannot write to `/usr/local/bin`, keep the binary in a project-local tools
directory and invoke it by path. HIPPO does not care where it lives.

## Shell completion

```sh
# zsh
hippo completion zsh > "${fpath[1]}/_hippo"

# bash
hippo completion bash > /usr/local/etc/bash_completion.d/hippo
```

`fish` and `powershell` are also available.

## Building from source instead

If you are working inside the HIPPO checkout, use the tracked bootstrap:

```sh
./hippo version
```

That script content-addresses every production Go source file plus the module graph, builds once
under bounded compiler parallelism, publishes the binary atomically, and retains the current
generation plus two recent fallbacks. Subsequent runs hit the cache.

A source build reports `dev (unknown)` rather than a version, because version and commit are injected
only by `scripts/build-release.sh`. That is expected, and it is a useful signal: if a machine you
believed was running a pinned release reports `dev`, it is not.

`HIPPO_BUILD_CACHE` relocates the bootstrap's cache directory.

## Related

- [Command-line interface](../reference/cli.md)
- [Guard your first command](../tutorials/guard-your-first-command.md)
