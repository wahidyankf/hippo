# HIPPO documentation

HIPPO admits, supervises, and sheds local development work from host resource evidence, so several
repositories can build and test on one machine without thrashing it.

## Start here

New to HIPPO? [Guard your first command](./tutorials/guard-your-first-command.md) takes about five
minutes and ends with a real command running under supervision.

Already know what you want to do? Jump to the [how-to guides](./how-to/README.md).

## Find the right kind of help

This documentation follows the [Diátaxis framework](https://diataxis.fr/), so you can pick material
that matches the question you have right now.

| Section                                | Use it when                                          |
| -------------------------------------- | ---------------------------------------------------- |
| [Tutorials](./tutorials/README.md)     | You are new and want to learn by doing               |
| [How-to guides](./how-to/README.md)    | You have a specific goal and need the steps          |
| [Reference](./reference/README.md)     | You need an exact flag, exit code, field, or default |
| [Explanation](./explanation/README.md) | You want to understand why HIPPO is built this way   |

The distinction that matters most: a **tutorial** teaches you something you did not know how to want,
while a **how-to guide** helps you do something you already decided to do. If a page feels like the
wrong shape for your question, the other section probably has the right one.

## The short version

- **What it is** — a standalone Go CLI for macOS and Linux. No daemon, no agent, no service.
- **What it does** — reads host evidence, resolves a safe profile, claims a CPU-and-memory
  reservation from a ledger shared by every repository on the machine, then runs your command in its
  own process group with the allocated concurrency in its environment.
- **How you use it** — `hippo run -- <your command>`, plus one environment variable your build tool
  already reads.
- **What it will not do** — signal a process group it did not start, or guess when shared state is
  unreadable.

## Specifications

The [specifications tree](../specs/README.md) is canonical.
[`specs/architecture.md`](../specs/architecture.md) holds the as-built C4 model, and
[`specs/behaviours/`](../specs/behaviours/README.md) holds the executable Gherkin corpus that every
test adapter runs. Where this documentation and the corpus disagree about observable behavior, the
corpus wins — and that disagreement is a bug worth reporting.

## Project context

HIPPO is one of the five **OSE Code Repositories** — the repositories
[Open Sharia Enterprise](https://github.com/wahidyankf/ose-public) is built and maintained in:

| Repository                                                 | What it does                            |
| ---------------------------------------------------------- | --------------------------------------- |
| [`ose-public`](https://github.com/wahidyankf/ose-public)   | The OSE product platform and research   |
| `ose-private`                                              | Authorized operations, private          |
| [`rhino`](https://github.com/wahidyankf/rhino)             | Repository hygiene                      |
| **`hippo`**                                                | Resource coordination — this repository |
| [`beaver-nest`](https://github.com/wahidyankf/beaver-nest) | An independent family product           |

HIPPO coordinates real work across the other four, which consume its published releases through
their own checksum-pinned bootstraps. Nothing crosses the other way: HIPPO consumes none of them,
and cannot guard itself.

**That name is navigation, not coupling.** The five are developed, versioned, and released
independently — no shared version number, no shared release cadence, no monorepo, and no parent
repository above them. Membership means only that a reader who finds one can find the other four.
HIPPO is usable entirely on its own and has no OSE-specific defaults compiled into it.

External contributions are currently closed. See the
[repository README](../README.md#-project-status) for the current status.
