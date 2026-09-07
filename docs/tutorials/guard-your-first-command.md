# Guard your first command

In this tutorial we will run a command under HIPPO's supervision, see what HIPPO tells that command
about the machine, and confirm that guarding changes nothing about how the command behaves.

It takes about five minutes. By the end you will have run five commands and seen HIPPO admit work,
pass through an exit code, and leave your shell pipeline intact.

## Before we start

You need:

- **macOS or Linux** on `amd64` or `arm64`. Native Windows is not supported.
- **Go 1.26.1** if you are running from a source checkout, which is what we will do here.

We are going to work inside the HIPPO repository itself, using the tracked `./hippo` bootstrap. That
script compiles the CLI once, caches it, and then hands off to the compiled binary — so the first
command is slower than the rest.

```sh
cd hippo
```

## Step 1: ask HIPPO what it sees

Before guarding anything, let's look at the machine through HIPPO's eyes.

```sh
./hippo status --disk-path .
```

The first run compiles the binary. When it finishes you will see a single line:

```console
state=normal reason=normal profile=balanced concurrency=11 swap=active availableGiB=15.36 diskFreeGiB=78.64 cpu=16.9%
```

Read it left to right. The host is in the `normal` state, HIPPO resolved the `balanced` profile, and
it would tell a guarded command it may use `11` parallel workers.

**Your numbers will differ**, and that is the point — `concurrency` is derived from your machine, not
from a constant. If your `state` says `warning` instead of `normal`, that is fine too; the rest of
this tutorial still works.

## Step 2: guard a command

Now run something under supervision. Note the `--` before the command: everything after it belongs to
the child, so HIPPO never tries to interpret the child's own flags.

```sh
./hippo run --class ephemeral --disk-path . -- sh -c 'echo build-started; echo build-finished'
```

```console
build-started
build-finished
```

That is the whole output. A healthy guarded run is deliberately quiet — HIPPO writes to stderr only
when an operator needs to know about a degraded admission, a deferral, a storage block, or a pressure
shed.

Notice what did **not** happen: no progress bar, no wrapper banner, no reformatting of the child's
output. The child kept your stdout exactly as it found it.

## Step 3: see what the child was told

HIPPO always exports two variables into an admitted child. Let's read them.

```sh
./hippo run --disk-path . -- sh -c 'echo "profile=$HIPPO_PROFILE concurrency=$HIPPO_CONCURRENCY"'
```

```console
profile=balanced concurrency=11
```

This is the whole integration surface for most consumers. A build script reads `HIPPO_CONCURRENCY`
and sizes itself accordingly, instead of asking the operating system how many cores exist and
assuming it owns all of them.

Compare that number with the `concurrency=` you saw in Step 1. They match, because both came from the
same policy applied to the same host.

## Step 4: confirm the exit code survives

A guard that swallowed exit codes would be useless in a script. Let's prove it does not.

```sh
./hippo run --disk-path . -- sh -c 'exit 3'
echo $?
```

```console
3
```

HIPPO passed the child's own exit status straight through. It reserves only five codes for itself —
`0`, `1`, `73`, `75`, and `78` — and every other code you see came from your command. The
[exit code reference](../reference/exit-codes.md) covers the five.

## Step 5: confirm your pipeline survives

Guarding also has to be invisible to shell plumbing. Let's put a guarded command in the middle of a
pipe, reading from stdin and writing to stdout.

```sh
printf 'hello\n' | ./hippo run --disk-path . -- sh -c 'read value; printf "%s-world\n" "$value"' | tr a-z A-Z
```

```console
HELLO-WORLD
```

The child read our `hello` from the pipe, wrote `hello-world`, and `tr` upcased it. Standard input,
standard output, and standard error all stayed connected to the caller. HIPPO's own diagnostics never
enter that stream — they go to stderr — so existing scripts and CI logs keep looking familiar.

## What we did

In five commands you:

- read HIPPO's assessment of the host with `status`;
- ran a guarded command with `run --` and saw it stay quiet;
- read the `HIPPO_PROFILE` and `HIPPO_CONCURRENCY` a child receives;
- confirmed exit codes pass through unchanged;
- confirmed stdin and stdout stay caller-owned.

So far HIPPO has been arbitrating between exactly one task, which is not very interesting. The real
job starts with a second one.

## Next steps

- [Coordinate two repositories](./coordinate-two-repositories.md) — watch two guarded tasks share one
  budget, which is what HIPPO is actually for.
- [Why HIPPO exists](../explanation/why-hippo-exists.md) — the problem behind all of this.
- [How to map concurrency into your build tool](../how-to/map-concurrency-into-your-build-tool.md) —
  wire `HIPPO_CONCURRENCY` into a real build.
