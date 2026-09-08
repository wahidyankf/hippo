# 🦛 HIPPO

**Host Infrastructure Pressure & Process Orchestrator** — stop several repositories from thrashing
one developer machine.

[![Quality gate](https://github.com/wahidyankf/hippo/actions/workflows/pr-quality-gate.yml/badge.svg)](https://github.com/wahidyankf/hippo/actions/workflows/pr-quality-gate.yml)
[![Release](https://img.shields.io/github/v/release/wahidyankf/hippo?sort=semver)](https://github.com/wahidyankf/hippo/releases)
[![Go](https://img.shields.io/badge/go-1.26.1-00ADD8)](https://go.dev/)
[![Platforms](https://img.shields.io/badge/platforms-macOS%20%7C%20Linux-lightgrey)](#-install)
[![License](https://img.shields.io/badge/license-MIT-blue)](./LICENSE)

HIPPO is a standalone Go CLI that admits, supervises, and sheds local development work based on what
the host can actually spare. It coordinates concurrent repositories through a shared CPU-and-memory
reservation ledger, and only the guard that owns a child may signal that child's process group.

```console
$ hippo run --class ephemeral --disk-path . -- make test
```

## ✨ Highlights

- **Cross-repository coordination.** Four checkouts on one laptop share one CPU-and-memory budget
  instead of each assuming it owns the machine.
- **Works with any build tool.** `--concurrency-env BUILD_WORKERS` writes HIPPO's allocation into the
  variable your tool already reads. No build system is compiled into HIPPO.
- **Invisible when healthy.** Your command keeps its stdin, stdout, stderr, and exit code. A healthy
  run prints nothing extra.
- **Safe by construction.** A guard signals only the process group it started. Pressure shedding
  works by marking a victim and waiting for that victim's own guard to act.
- **Fails closed.** Unreadable shared state defers admission and preserves bytes rather than guessing
  and rewriting.
- **A stable exit contract.** `73` cleanup, `75` retry, `78` replan — everything else is your
  command's own exit code.
- **No daemon.** One short-lived process per guarded command, plus files in a shared state root.

## 🤔 Why

Run a build in one checkout, a test suite in another, and a dev server in a third, and each one sizes
itself to the machine: `make -j$(nproc)`, one test worker per core, a bundler that assumes it owns the
box. Individually reasonable, collectively ruinous. The machine starts swapping and everything slows
down together — including the editor you are actually looking at.

Turning parallelism down everywhere is wrong in both directions: too low wastes an idle machine, too
high brings the contention straight back. `nice` and cgroups shape work that is already running; they
do not decide whether it should start. And your bundler will never know about your Gradle daemon.

HIPPO puts a small, generic arbiter in front of the work. Before a heavy command runs, it reads host
evidence, claims a fixed share from a ledger every repository on the machine can see, and tells the
command how much of the host it may actually use. The command does not change — it reads a number out
of an environment variable it already understands.

Longer version: [Why HIPPO exists](./docs/explanation/why-hippo-exists.md).

## 📦 Install

Download a tagged archive for `darwin` or `linux` on `amd64` or `arm64`, and verify it against the
release `checksums.txt`. **Pin both the tag and the expected SHA-256; never follow `main` at
runtime.**

```sh
VERSION=v0.5.1
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m); [ "$ARCH" = x86_64 ] && ARCH=amd64; [ "$ARCH" = aarch64 ] && ARCH=arm64
BASE="https://github.com/wahidyankf/hippo/releases/download/${VERSION}"

curl -fsSLO "${BASE}/hippo_${VERSION}_${OS}_${ARCH}.tar.gz"
curl -fsSLO "${BASE}/checksums.txt"

# Verify before extracting. sha256sum on Linux, shasum on macOS.
grep " hippo_${VERSION}_${OS}_${ARCH}.tar.gz\$" checksums.txt |
  { command -v sha256sum >/dev/null 2>&1 && sha256sum -c - || shasum -a 256 -c -; }

tar -xzf "hippo_${VERSION}_${OS}_${ARCH}.tar.gz"
./hippo version --json
```

```console
hippo_v0.5.1_darwin_arm64.tar.gz: OK
{"schemaVersion":1,"version":"v0.5.1","commit":"5722854fddfd68b1fc7ca9feca935fe3e7eec625"}
```

Working from a source checkout instead? The tracked `./hippo` bootstrap compiles the CLI once and
caches it. Full details: [How to install a pinned release](./docs/how-to/install-a-pinned-release.md).

## 🚀 Quick start

Ask what the host looks like:

```console
$ hippo status --disk-path .
state=normal reason=normal profile=balanced concurrency=11 swap=active availableGiB=15.36 diskFreeGiB=78.64 cpu=16.9%
```

Guard a command. Everything after `--` belongs to the child, so its flags are never parsed as HIPPO's:

```console
$ hippo run --class ephemeral --disk-path . -- sh -c 'echo build-started; echo build-finished'
build-started
build-finished
```

That is the entire output. HIPPO writes to stderr only when an operator needs to know about a
degraded admission, a deferral, a storage block, or a pressure shed.

Your exit code and your pipeline both survive:

```console
$ hippo run --disk-path . -- sh -c 'exit 3'; echo $?
3

$ printf 'hello\n' | hippo run --disk-path . -- sh -c 'read v; printf "%s-world\n" "$v"' | tr a-z A-Z
HELLO-WORLD
```

Hand the allocation to your build tool:

```console
$ hippo run --disk-path . --concurrency-env BUILD_WORKERS --concurrency-env TEST_JOBS -- sh -c 'echo "BUILD_WORKERS=$BUILD_WORKERS TEST_JOBS=$TEST_JOBS"'
BUILD_WORKERS=11 TEST_JOBS=11
```

Walk through it properly: [Guard your first command](./docs/tutorials/guard-your-first-command.md).

## ⚙️ How it works

**Admission.** HIPPO samples memory, disk, CPU, swap, the macOS compressor, and Linux PSI, then
resolves `balanced` → `constrained` → `minimal` from that evidence. It claims a fixed CPU-and-memory
vector from a ledger shared by every repository using the same state root. Capacity is the host's
available parallelism minus one safety unit, and effective memory minus the profile's reserve — never
the whole machine.

**Supervision.** The admitted command runs as its own process group, holding `HIPPO_PROFILE`,
`HIPPO_CONCURRENCY`, and any variables you mapped. A private launcher holds the reservation identity
through complete group retirement, so a leader exiting early or forking a background descendant
cannot release capacity while the work is still running.

**Shedding.** Under critical pressure one locked evaluation marks a single victim — newest ephemeral
first, then newest service, never a transactional owner. A remote guard **never** signals another
guard's process group; it marks and waits for that owner to stop its own child. A live unresponsive
victim blocks any further selection, so pressure cannot cascade into emptying the ledger.

**Failure.** Exit `73` needs storage cleanup, `75` is retryable pressure and holds no reservation,
`78` needs a changed request. Corrupt or unreadable shared state returns an error and preserves the
bytes rather than reporting a synthetic zero that would let everyone in at once.

| Mode                     | Behavior                                                                                         |
| ------------------------ | ------------------------------------------------------------------------------------------------ |
| Schema 1 — `exclusive`   | One heavy task host-wide; services keep independent sessions. The default without configuration. |
| Schema 2 — `reservation` | Concurrent owners against a shared vector budget. Opt in per repository.                         |

The two never mix within one state root; a client meeting the other mode defers with `75` until the
old sessions drain.

## 📚 Documentation

Full documentation lives in [`docs/`](./docs/README.md) and follows the
[Diátaxis framework](https://diataxis.fr/).

| Section                                     | Use it when                                          |
| ------------------------------------------- | ---------------------------------------------------- |
| [Tutorials](./docs/tutorials/README.md)     | You are new and want to learn by doing               |
| [How-to guides](./docs/how-to/README.md)    | You have a specific goal and need the steps          |
| [Reference](./docs/reference/README.md)     | You need an exact flag, exit code, field, or default |
| [Explanation](./docs/explanation/README.md) | You want to understand why HIPPO works this way      |

The [specifications tree](./specs/README.md) is canonical: `specs/architecture.md` holds the as-built
C4 model and [`specs/behaviours/`](./specs/behaviours/README.md) holds the executable Gherkin corpus
that every test adapter runs.

## 📋 Project status

HIPPO is in active development. Its behavior is pinned by an executable specification, but versions
below `1.0.0` may still make breaking changes — see the [changelog](./CHANGELOG.md).

Released tags are immutable. A published release is never rebuilt or replaced.

**External contributions are currently closed.** Issues and pull requests from outside the project
are not being accepted while the engineering patterns stabilize. You are welcome to fork the
repository under the MIT license and use it however you like.

Contributor rules live in [`repo-governance/`](./repo-governance/README.md), one document each with
the reason it exists. `AGENTS.md` indexes them and states none itself, and `CLAUDE.md` holds one
import directive, so there is a single instruction body rather than two that drift.

## 🌙 Part of Open Sharia Enterprise

HIPPO is one of the five **OSE Code Repositories**, with `ose-public`, `ose-private`, `rhino`, and
`beaver-nest`. It supplies their host resource coordination. The name is navigation, not coupling —
see [project context](./docs/README.md#project-context).

HIPPO has no OSE-specific defaults compiled into it and is designed to be used entirely on its own —
consumers supply their own commands, paths, ports, and health endpoints.

## 🛠️ Development

Source contributors need Go 1.26.1 and Node.js 24.

```sh
npm ci            # installs locked tooling; the prepare lifecycle installs Git hooks
npm run test:quick # format, lint, unit, coverage, behavior adapters, artifact and documentation policy
npm test           # the full release gate, including race detection and vulnerability scan
```

Only `main` persists. Work reaches it through a pull request from a branch in a worktree beside this
checkout; direct pushes are refused for every actor, with no bypass. One aggregate `Quality gate`
check, defined in `.github/workflows/pr-quality-gate.yml`, is required, and it is a superset of the
Git hooks.

Documentation hygiene runs under [RHINO](https://github.com/wahidyankf/rhino), pinned by tag and
SHA-256 in `rhino.lock`, through `scripts/docs-check.sh` — one definition the quick gate and the
pull-request gate both call. What it enforces is declared in `repo-config.yml` rather than compiled
into the tool. The two repositories pin each other: RHINO guards its own builds with a pinned
`./hippo`.

Everything else — the coverage floor, the exemption boundaries, where a worktree may live, how a
release is cut — is in `repo-governance/`.

## 📄 License

HIPPO is available under the [MIT License](./LICENSE).
