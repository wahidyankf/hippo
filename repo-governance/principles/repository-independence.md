# Repository Independence

HIPPO knows nothing about the repositories it guards. Not their layout, not their commands, not their task runner, not their build tool. It takes a command and runs it.

## The Rule

**No product-specific default may enter this tool.** Not a default command, not a known directory, not a special case for a framework, not a heuristic that recognizes one ecosystem's flags.

Where a caller must supply something, HIPPO requires it and refuses without it. Where the host cannot be measured, HIPPO refuses with a stable exit code rather than estimating.

## Why This Is a Principle

A default is a value the tool cannot verify and cannot retract. It is correct in the repository it was taken from and silently wrong everywhere else, and the failure is quiet: the guard admits work under an assumption nobody stated.

It is also self-defeating. The whole claim of a guard is that its clean run means something. A guard that guessed at capacity, or at what a command was going to do, has a clean run that means only that the guess was not obviously wrong.

## What This Permits

Generic mechanisms that a caller parameterizes: environment variables that receive an allocated concurrency, streams the caller connects, health endpoints the caller supplies, a class the caller chooses. Each of those is a shape rather than a value, and the value stays with the repository that knows it.

## Consequences Elsewhere

This is why the [public contract](../development/public-contract.md) is three exit codes and a command surface rather than a configuration schema for other people's builds; why [dependency selection](../development/dependency-selection.md) treats anything reaching the filesystem or process table as significant; and why HIPPO must never become load-bearing for the work it guards.
