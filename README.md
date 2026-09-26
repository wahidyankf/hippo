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
$ hippo run --class ephemeral --resource-tier standard --disk-path . -- make test
```

## ✨ Highlights

- **Cross-repository coordination.** Every checkout on one laptop shares one CPU-and-memory budget
  instead of each assuming it owns the machine.
- **Works with any build tool.** `--concurrency-env BUILD_WORKERS` writes HIPPO's allocation into the
  variable your tool already reads. No build system is compiled into HIPPO.
- **Invisible when healthy.** Your command keeps its stdin, stdout, stderr, and exit code. A healthy
  run prints nothing extra.
- **Safe by construction.** A guard signals only the process group it started. Pressure shedding
  works by marking a victim and waiting for that victim's own guard to act.
- **Fails closed.** Unreadable shared state fails without retry and keeps its bytes untouched.
- **A stable exit contract.** `124` a limit stopped the work, `125` HIPPO failed to start or
  supervise it, `126` and `127` the command cannot be run, `2` the invocation cannot be used — the
  numbers `timeout` and POSIX shells use. Each failure names a reason: `hippo: [hippo.area.reason]`.
  Child-owned codes pass through with task-failed evidence.
- **Visible admission.** `status`, `watch`, and `history` expose labeled owners, FIFO waiters,
  promotion state, and bounded run outcomes without exposing commands or paths.
- **Burst-safe activation.** Simultaneous repository clients wait through brief coordination
  contention after launch instead of cutting an already started payload.
- **No daemon.** One short-lived process per guarded command, plus files in a shared state root.

## 🤔 Why

Build tools commonly size themselves as if they own the host. Concurrent repositories then swap and
slow the editor too. Fixed low parallelism wastes an idle machine; high parallelism restores the
contention. `nice` and cgroups shape work already running but do not decide whether it should start.

HIPPO admits work against one shared ledger and exports its allocation through ordinary environment
variables, so build tools need no HIPPO-specific integration.

Longer version: [Why HIPPO exists](./docs/explanation/why-hippo-exists.md).

## 📦 Install

Download a tagged archive for `darwin` or `linux` on `amd64` or `arm64`, and verify it against the
release `checksums.txt`. **Pin both the tag and the expected SHA-256; never follow `main` at
runtime.**

```sh
VERSION=v0.8.2
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

The checksum command prints `hippo_v0.8.2_<os>_<arch>.tar.gz: OK`; `version --json` reports
`v0.8.2` and the exact release commit.

Working from a source checkout instead? The tracked `./hippo` bootstrap compiles the CLI once and
caches it. Full details: [How to install a pinned release](./docs/how-to/install-a-pinned-release.md).

## 🚀 Quick start

Ask what the host looks like:

```console
$ hippo status --disk-path .
state=normal reason=normal profile=balanced concurrency=11 swap=active availableGiB=15.36 diskFreeGiB=78.64 cpu=16.9% owners=0 waiters=0 ownerLimit=0 promotion=not-configured
```

Guard a command. Schema 3 requires a resource tier. Everything after `--` belongs to the child, so
its flags are never parsed as HIPPO's:

```console
$ hippo run --class ephemeral --resource-tier light --disk-path . -- sh -c 'echo build-started; echo build-finished'
build-started
build-finished
```

That is the entire output. HIPPO writes to stderr only when an operator needs to know about a
degraded admission, a deferral, a storage block, or a pressure shed.

Your exit code and your pipeline both survive:

```console
$ hippo run --resource-tier light --disk-path . -- sh -c 'exit 3'; echo $?
3

$ printf 'hello\n' | hippo run --resource-tier light --disk-path . -- sh -c 'read v; printf "%s-world\n" "$v"' | tr a-z A-Z
HELLO-WORLD
```

Hand the allocation to your build tool:

```console
$ hippo run --resource-tier standard --disk-path . --concurrency-env BUILD_WORKERS --concurrency-env TEST_JOBS -- sh -c 'echo "BUILD_WORKERS=$BUILD_WORKERS TEST_JOBS=$TEST_JOBS"'
BUILD_WORKERS=11 TEST_JOBS=11
```

Walk through it properly: [Guard your first command](./docs/tutorials/guard-your-first-command.md).

## ⚙️ How it works

**Admission.** HIPPO samples memory, disk, CPU, swap, the macOS compressor, and Linux PSI, then
resolves `balanced` → `constrained` → `minimal` from that evidence. Schema 3 registers one stable FIFO
waiter before launch and grants the largest safe launch-time vector between the chosen tier's minimum
and maximum. It never starts a hidden retry loop or resizes a running child. Every repository using
the same state root competes against the same pool.

**Supervision.** The admitted command runs as its own process group, holding `HIPPO_PROFILE`,
`HIPPO_CONCURRENCY`, and any variables you mapped. A private launcher holds the reservation identity
through complete group retirement, so a leader exiting early or forking a background descendant
cannot release capacity while the work is still running.

**Shedding.** Under critical pressure one locked evaluation marks a single victim — newest ephemeral
first, then newest service. Transactional work is protected during ordinary shedding and is eligible
last only at the configured emergency floor. A remote guard **never** signals another guard's process
group; it marks and waits for that owner to stop its own child. A live unresponsive victim blocks any
further selection, so pressure cannot cascade into emptying the ledger.

**Failure.** Exit `124` means a limit stopped the work; the reason says which. `storage-blocked`
needs cleanup, `capacity-deferred` may be requeued when its receipt says `never-started`, and
`pressure-shed` needs payload recovery. Exit `125` means HIPPO failed to start or supervise the
work: drain or upgrade an incompatible peer, change the request, or fix the configuration.
[Exit codes and error codes](./docs/reference/exit-codes.md) lists both closed vocabularies.

| Mode                     | Behavior                                                                                         |
| ------------------------ | ------------------------------------------------------------------------------------------------ |
| Schema 1 — `exclusive`   | One heavy task host-wide; services keep independent sessions. The default without configuration. |
| Schema 2 — `reservation` | Concurrent owners against a shared vector budget. Opt in per repository.                         |
| Schema 3 — `adaptive`    | Tiered FIFO admission, labeled status/history, and evidence-gated burst capacity.                |

The modes never mix within one state root. A v1 client meeting the other live protocol exits `125` naming `hippo.coordination.protocol-mismatch`
without changing state; drain the old epoch before retrying.

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

HIPPO's v1 behavior is pinned by an executable specification. Future public-contract changes follow
semantic versioning; see the [changelog](./CHANGELOG.md).

Released tags are immutable. A published release is never rebuilt or replaced.

**External contributions are currently closed** while the engineering patterns stabilize. Forks
remain welcome under the MIT license.

Contributor rules live in [`repo-governance/`](./repo-governance/README.md), one document each with
the reason it exists. `AGENTS.md` indexes them and states none itself, and `CLAUDE.md` holds one
import directive, so there is a single instruction body rather than two that drift.

## 🌙 Part of Open Sharia Enterprise

HIPPO is one of the five **OSE Code Repositories**, with `ose-public`, `rhino`, `beaver-nest`, and
one private operations repository this public one does not name. It supplies their host resource
coordination. The name is navigation, not coupling — see
[project context](./docs/README.md#project-context).

HIPPO has no OSE-specific defaults compiled into it and is designed to be used entirely on its own —
consumers supply their own commands, paths, ports, and health endpoints.

## 🛠️ Development

Source contributors need Go 1.26.1 and Node.js 24.

```sh
npm ci            # installs locked tooling and, via prepare, Git hooks
npm run test:quick # format, lint, unit, coverage, behavior adapters, artifact and repository policy
npm test           # the quick gate plus integration, end-to-end, race and vulnerability checks
```

Only `main` persists. Work reaches it through a pull request from a branch at
`{repository location}/worktrees/<task>`; sibling `*-worktrees` directories are forbidden. Direct
pushes are refused for every actor. One aggregate `Quality gate` check, defined in
`.github/workflows/pr-quality-gate.yml`, is required.

ShellCheck and documentation gates are in neither script: they run under
[RHINO](https://github.com/wahidyankf/rhino) on push and pull request, pinned by tag and
SHA-256 in `rhino.lock`; `repo-config.yml` supplies the lifecycle and three native harness
projections. Regenerate those projections only with `./rhino harness adapters generate` and prove
them with `./rhino harness adapters validate`. RHINO guards its own builds with a pinned `./hippo`.

Everything else — the coverage floor, the exemption boundaries, where a worktree may live, how a
release is cut — is in `repo-governance/`.

## 📄 License

HIPPO is available under the [MIT License](./LICENSE).
