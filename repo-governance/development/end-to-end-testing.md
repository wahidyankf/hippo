# End-to-End Testing

The E2E adapter spawns the executable and drives it the way a caller does. It is the only boundary that sees the process boundary itself: the argument vector, standard streams, the working directory, and the exit code.

## What Belongs Here

- Behaviour that only exists once a process exists: signal handling, controlling-terminal ownership, child process groups, the exit code a caller reads.
- Anything asserted about a relative path, because only here is the inspected directory also the process's working directory.
- The compiled artifact's own properties — that end-to-end binaries are temporary, that build caching behaves.

## What Does Not

Anything provable in process. A scenario driven at E2E because it was convenient costs minutes on every gate run and proves nothing the unit adapter did not.

## Placement

Compiled end-to-end behaviour runs in the full gate, never in the quick gate. `pre-push` must stay fast enough that nobody looks for a way around it — a slow hook is a hook that gets bypassed, and [push hook verification](../conventions/push-hook-verification.md) exists because that bypass is prohibited.

## Exemptions

A scenario exempt at E2E names the concrete boundary and the reason, in the reviewed inventory. The common honest reason is that the subject is outside the compiled binary: a lint configuration, a hook file, a gate script's text. Those are properties of the repository, not of the program, and the E2E adapter is driving the program.

Every such scenario still runs at the unit boundary. There is no unit exemption — see [specification maintenance](specification-maintenance.md).
