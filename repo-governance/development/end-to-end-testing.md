# End-to-End Testing

The E2E adapter spawns the executable and drives it the way a caller does. It is the only boundary that sees the process
boundary itself: the argument vector, standard streams, the working directory, and the exit code.

## What Belongs Here

- Behaviour that only exists once a process exists: signal handling, controlling-terminal ownership, child process
  groups, the exit code a caller reads.
- Anything asserted about a relative path, because only here is the inspected directory also the process's working
  directory.
- The compiled artifact's own properties — that end-to-end binaries are temporary, that build caching behaves.

## What Does Not

Anything provable in process. A scenario driven at E2E because it was convenient costs minutes on every gate run and
proves nothing the unit adapter did not.

## Placement

Run the canonical executing E2E corpus and compiled-only public suites only through `npm run test:e2e`, which the `main`
surface runs after each merge. Ordinary quick support controls may acquire and use a temporary stamped candidate; that
fixture does not execute an E2E journey.

The E2E suite never runs per pull request. Keep local E2E selective to the touched flow. The nightly loaded-host
workflow runs the quick, integration, and E2E targets under synthetic load. `pre-push` runs only the leak scan, and it
must stay fast enough that nobody looks for a way around it — a slow hook is a hook that gets bypassed, and
[push hook verification](../conventions/push-hook-verification.md) exists because that bypass is restricted to its
worktree-to-PR exception.

This adapts the `ose-rules` end-to-end standard to HIPPO's targets and surfaces: private fixture controls retain
candidate acquisition while public journeys stay out of hooks and pull requests.

## Exemptions

A scenario exempt at E2E names the concrete boundary and the reason, in the reviewed inventory. The common honest reason
is that the subject is outside the compiled binary: a lint configuration, a hook file, a gate script's text. Those are
properties of the repository, not of the program, and the E2E adapter is driving the program.

Every such scenario still runs at the unit boundary. There is no unit exemption — see
[specification maintenance](specification-maintenance.md).
