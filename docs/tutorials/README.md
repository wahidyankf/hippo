# Tutorials

Hands-on lessons for someone new to HIPPO. Work through them in order; each one ends with something
running on your machine.

These are for learning. When you already know what you want to accomplish, the
[how-to guides](../how-to/README.md) are the faster route.

## Start here

1. [Guard your first command](./guard-your-first-command.md) — about five minutes. Run a command
   under supervision, read what HIPPO tells it about the machine, and confirm that guarding changes
   nothing about how the command behaves.
2. [Coordinate two tasks through one budget](./coordinate-two-repositories.md) — about ten minutes.
   Turn on reservation coordination, run two owners at once against one shared budget, and see an
   impossible request rejected.

## What you need

- macOS or Linux on `amd64` or `arm64`. Native Windows is not supported.
- Go 1.26.1, if you are running from a source checkout.

Neither tutorial requires a release download; both use the tracked `./hippo` bootstrap, which
compiles and caches the CLI on first use.

## Next steps

- [How-to guides](../how-to/README.md) for a goal you already have.
- [Explanation](../explanation/README.md) for why HIPPO is built this way.
- [Reference](../reference/README.md) for exact flags, codes, and schemas.
