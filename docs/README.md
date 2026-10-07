# HIPPO documentation

HIPPO admits, supervises, and sheds local development work from host resource evidence, so several repositories can
build and test on one machine without thrashing it.

## Start here

New to HIPPO? [Guard your first command](./tutorials/guard-your-first-command.md) takes about five minutes and ends with
a real command running under supervision.

Already know what you want to do? Jump to the [how-to guides](./how-to/README.md).

## Find the right kind of help

This documentation follows the [Diátaxis framework](https://diataxis.fr/), so you can pick material that matches the
question you have right now.

| Section                                | Use it when                                          |
| -------------------------------------- | ---------------------------------------------------- |
| [Tutorials](./tutorials/README.md)     | You are new and want to learn by doing               |
| [How-to guides](./how-to/README.md)    | You have a specific goal and need the steps          |
| [Reference](./reference/README.md)     | You need an exact flag, exit code, field, or default |
| [Explanation](./explanation/README.md) | You want to understand why HIPPO is built this way   |

The distinction that matters most: a **tutorial** teaches you something you did not know how to want, while a **how-to
guide** helps you do something you already decided to do. If a page feels like the wrong shape for your question, the
other section probably has the right one.

## The short version

- **What it is** — a standalone Go CLI for macOS and Linux. No daemon, no agent, no service.
- **What it does** — reads host evidence, resolves a safe profile, claims a CPU-and-memory reservation from a ledger
  shared by every repository on the machine, then runs your command in its own process group with the allocated
  concurrency in its environment.
- **How you use it** — under adaptive schema 3, `hippo run --resource-tier <tier> -- <command>`, plus one environment
  variable your build tool already reads.
- **How you observe it** — `hippo status`, `hippo watch`, and `hippo history` show labeled admission and bounded
  outcomes without recording commands or paths.
- **What it will not do** — signal a process group it did not start, or guess when shared state is unreadable.

## Specifications

The [specifications tree](../specs/README.md) is canonical. [`specs/architecture.md`](../specs/architecture.md) holds
the as-built C4 model, and [`specs/behaviours/`](../specs/behaviours/README.md) holds the executable Gherkin corpus that
every test adapter runs. Where this documentation and the corpus disagree about observable behavior, the corpus wins —
and that disagreement is a bug worth reporting.

## Project context

HIPPO is one of the seven **`ose-projects`** repositories — the repositories
[Open Sharia Enterprise](https://github.com/wahidyankf/ose-public) is built and maintained in. Each entry gives the
repository's role, then how it relates to HIPPO:

- **[`hippo`](https://github.com/wahidyankf/hippo)** — resource coordination. This repository.
- [`ose-public`](https://github.com/wahidyankf/ose-public) — the OSE product platform and research. Upstream consumption
  both ways: it pins HIPPO releases, and HIPPO's contributor gates pin its FERRET release.
- _(unnamed, private)_ — authorized operations. Upstream consumption: it pins HIPPO releases.
- [`rhino`](https://github.com/wahidyankf/rhino) — repository hygiene. Upstream consumption both ways: HIPPO's
  contributor gates pin RHINO releases, and RHINO guards its own builds with a pinned HIPPO.
- [`beaver-nest`](https://github.com/wahidyankf/beaver-nest) — an independent family product. Upstream consumption: it
  pins HIPPO releases.
- [`ose-rules`](https://github.com/wahidyankf/ose-rules) — the reference catalog of governance, planning, agent, and
  skill artifacts. Knowledge sharing: HIPPO adopts its artifacts by explicit one-off copy and owns each copy. It also
  pins HIPPO releases.
- [`py-typekit`](https://github.com/wahidyankf/py-typekit) — typed functional primitives for Python. None: it is named
  here only because it is a member.

Each consumer reaches HIPPO through a checksum-pinned bootstrap. Nothing crosses the other way at runtime: the HIPPO
binary depends on none of them, and cannot guard itself.

The private one is left unnamed here on purpose: this repository is public, and a public document naming a private
repository publishes the fact that it exists and what it is called. Anyone authorized to work in it already knows its
name.

**That label is navigation, not coupling.** `ose-projects` is a routing label only — not an organization, a parent
repository, a parity group, or a shared release. The seven are developed, versioned, gated, and released independently,
with no shared version number, release cadence, or monorepo. Membership obliges each member only to name the others, so
a reader who finds one can find the other six, and nothing more. HIPPO is usable entirely on its own and has no
OSE-specific defaults compiled into it.

External contributions are currently closed. See the [repository README](../README.md#-project-status) for the current
status.
